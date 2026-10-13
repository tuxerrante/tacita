package backtest

import (
	"cmp"
	"context"
	"fmt"

	"github.com/tuxerrante/tacita/internal/mining"
)

// ReasonIdenticalBaseline marks the unweighted comparison of a unit
// configuration, which is its own unweighted baseline.
const ReasonIdenticalBaseline Reason = "identical_baseline"

// Outcome is one configuration's evaluation over one event ordering.
type Outcome struct {
	Configuration mining.Configuration
	// Displays holds the configuration's display at each partition.
	Displays []Display
	// Model scores the displayed candidates.
	Model Evaluation
	// FrequentConsequent scores the same antecedent slots with each consequent
	// replaced by the most prevalent other training component.
	FrequentConsequent Evaluation
}

// EvaluateOrdering folds transactions over the partitions once, displays every
// configuration at each cutoff, and scores each configuration and its
// frequent-consequent baseline on the evaluation windows, in configuration
// order.
//
// The chronological model and the hash-shuffled control are the same call
// over different orderings. ctx is checked before each cutoff's displays and
// before each configuration's evaluation, so a cancellation or deadline stops
// the work within one cutoff's displays.
func EvaluateOrdering(
	ctx context.Context,
	transactions []Transaction,
	partitions []Partition,
	configurations []mining.Configuration,
) ([]Outcome, error) {
	outcomes := make([]Outcome, len(configurations))
	modelRules := make([][]CutoffRules, len(configurations))
	frequentRules := make([][]CutoffRules, len(configurations))
	for index, configuration := range configurations {
		outcomes[index].Configuration = configuration
	}

	err := Fold(transactions, partitions, func(_ Partition, aggregate mining.Aggregate) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		displays, err := DisplayCandidates(aggregate, configurations)
		if err != nil {
			return err
		}
		prevalence := mostPrevalent(aggregate)
		for index, display := range displays {
			rules := display.Rules()
			outcomes[index].Displays = append(outcomes[index].Displays, display)
			modelRules[index] = append(modelRules[index], rules)
			frequentRules[index] = append(frequentRules[index], prevalence.replaceConsequents(rules))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	windows, err := Windows(transactions, partitions)
	if err != nil {
		return nil, err
	}
	for index := range outcomes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if outcomes[index].Model, err = Evaluate(windows, modelRules[index]); err != nil {
			return nil, fmt.Errorf("evaluating %s: %w", configurations[index].ID(), err)
		}
		if outcomes[index].FrequentConsequent, err = Evaluate(windows, frequentRules[index]); err != nil {
			return nil, fmt.Errorf("evaluating %s baseline: %w", configurations[index].ID(), err)
		}
	}

	return outcomes, nil
}

// prevalence holds the two training components with the greatest raw
// prevalence, ties broken by ascending component bytes. Two are enough: the
// most prevalent component other than A is the first unless A is the first.
type prevalence struct {
	first  string
	second string
}

func mostPrevalent(aggregate mining.Aggregate) prevalence {
	var top [2]*mining.Component
	for index := range aggregate.Components {
		component := &aggregate.Components[index]
		switch {
		case top[0] == nil || morePrevalent(component, top[0]):
			top[1] = top[0]
			top[0] = component
		case top[1] == nil || morePrevalent(component, top[1]):
			top[1] = component
		}
	}

	var result prevalence
	if top[0] != nil {
		result.first = top[0].Name
	}
	if top[1] != nil {
		result.second = top[1].Name
	}

	return result
}

func morePrevalent(left, right *mining.Component) bool {
	return cmp.Or(
		cmp.Compare(right.RawOpportunity, left.RawOpportunity),
		cmp.Compare(left.Name, right.Name),
	) < 0
}

// replaceConsequents keeps every displayed antecedent slot and replaces its
// consequent with the most prevalent other training component. A displayed
// candidate A -> B proves a second component exists, so the replacement is
// always defined.
func (p prevalence) replaceConsequents(cutoff CutoffRules) CutoffRules {
	rules := make([]Rule, len(cutoff.Rules))
	for index, rule := range cutoff.Rules {
		consequent := p.first
		if consequent == rule.Antecedent {
			consequent = p.second
		}
		rules[index] = Rule{Antecedent: rule.Antecedent, Consequent: consequent}
	}

	return CutoffRules{Abstained: cutoff.Abstained, Rules: rules}
}

// UnweightedCounterpart returns the configuration whose evaluation is the
// unweighted co-change baseline of configuration: the same thresholds with
// unit size weight. It reports false for a unit configuration, which is its
// own baseline and whose comparison is omitted as identical_baseline.
func UnweightedCounterpart(configuration mining.Configuration) (mining.Configuration, bool) {
	if configuration.SizeWeight == mining.WeightUnit {
		return mining.Configuration{}, false
	}
	configuration.SizeWeight = mining.WeightUnit

	return configuration, true
}

// NoHistoryCandidates counts the inferred candidates that configurations
// display from an empty training partition. The frozen no-history control
// requires zero.
func NoHistoryCandidates(configurations []mining.Configuration) (int, error) {
	displays, err := DisplayCandidates(mining.Aggregate{}, configurations)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, display := range displays {
		count += len(display.Candidates)
	}

	return count, nil
}
