package backtest

import (
	"cmp"
	"math"
	"slices"

	"github.com/tuxerrante/tacita/internal/mining"
)

// GateOutcome is a frozen engineering gate outcome.
type GateOutcome string

// The frozen outcomes. Both fail and inconclusive are non-passing.
const (
	GatePass         GateOutcome = "pass"
	GateFail         GateOutcome = "fail"
	GateInconclusive GateOutcome = "inconclusive"
)

// The development gate identifiers, one per frozen development engineering
// requirement that a single repository's calibration decides.
const (
	GateAdjacentJaccard          = "adjacent_jaccard"
	GateCoverage                 = "coverage"
	GateCutoffDisplay            = "cutoff_display"
	GateEvaluationWindowsCovered = "evaluation_windows_covered"
	GateFrequentConsequentGain   = "frequent_consequent_gain"
	GateNoHistoryControl         = "no_history_control"
	GateShuffledControlGain      = "shuffled_control_gain"
	GateUnweightedGain           = "unweighted_gain"
)

// The frozen development thresholds.
const (
	minimumCoverage               = 0.05
	minimumFrequentConsequentGain = 0.10
	minimumShuffledControlGain    = 0.05
	minimumUnweightedGain         = 0
	minimumAdjacentJaccard        = 0.50
	minimumDisplayedPerCutoff     = 1
	minimumCoveredPerWindow       = 1
	maximumNoHistoryCandidates    = 0
	operatorAtLeast               = ">="
	operatorAtMost                = "<="
)

// Gate is one decided engineering dimension. Metric is the value compared with
// Threshold; for a gate over micro and macro adherence it is the smaller of
// the two differences. Reason explains an inconclusive outcome or a failure
// without a defined metric.
type Gate struct {
	ID        string
	Outcome   GateOutcome
	Metric    Metric
	Operator  string
	Threshold float64
	Reason    Reason
}

// ConfigurationGates holds one configuration's development gates on one
// repository, sorted by gate ID, and their conjunction.
type ConfigurationGates struct {
	Configuration mining.Configuration
	Gates         []Gate
	Outcome       GateOutcome
}

// DevelopmentGates decides every development engineering dimension for each
// configuration of one repository from its chronological and hash-shuffled
// outcomes, which must cover the same configurations in the same order.
//
// The unweighted gate is omitted for a `unit` configuration, whose baseline is
// itself. Comparisons use unrounded finite values.
func DevelopmentGates(chronological, shuffled []Outcome) ([]ConfigurationGates, error) {
	if len(chronological) != len(shuffled) {
		return nil, &OutcomeMismatchError{Chronological: len(chronological), Shuffled: len(shuffled)}
	}
	byConfiguration := make(map[mining.Configuration]Outcome, len(chronological))
	for index, outcome := range chronological {
		if shuffled[index].Configuration != outcome.Configuration {
			return nil, &OutcomeMismatchError{Chronological: len(chronological), Shuffled: len(shuffled)}
		}
		byConfiguration[outcome.Configuration] = outcome
	}

	decided := make([]ConfigurationGates, len(chronological))
	for index, outcome := range chronological {
		model := outcome.Model
		gates := []Gate{
			cutoffDisplayGate(outcome.Displays),
			atLeast(GateCoverage, model.Coverage, minimumCoverage),
			windowsCoveredGate(outcome.Displays, model),
			atLeast(GateFrequentConsequentGain,
				gain(model, outcome.FrequentConsequent), minimumFrequentConsequentGain),
			atLeast(GateShuffledControlGain,
				gain(model, shuffled[index].Model), minimumShuffledControlGain),
			atLeast(GateAdjacentJaccard, model.MinimumJaccard, minimumAdjacentJaccard),
		}
		if counterpart, ok := UnweightedCounterpart(outcome.Configuration); ok {
			unweighted, found := byConfiguration[counterpart]
			if !found {
				return nil, &MissingCounterpartError{Configuration: counterpart}
			}
			gates = append(gates, atLeast(GateUnweightedGain,
				gain(model, unweighted.Model), minimumUnweightedGain))
		}
		slices.SortFunc(gates, func(left, right Gate) int { return cmp.Compare(left.ID, right.ID) })

		decided[index] = ConfigurationGates{
			Configuration: outcome.Configuration,
			Gates:         gates,
			Outcome:       conjunction(gates),
		}
	}

	return decided, nil
}

// NoHistoryGate decides the repository's no-history control, which requires
// zero inferred candidates from an empty training partition.
func NoHistoryGate(candidates int) Gate {
	gate := Gate{
		ID:        GateNoHistoryControl,
		Metric:    defined(float64(candidates)),
		Operator:  operatorAtMost,
		Threshold: maximumNoHistoryCandidates,
		Outcome:   GatePass,
	}
	if candidates > maximumNoHistoryCandidates {
		gate.Outcome = GateFail
	}

	return gate
}

// atLeast passes a defined metric at or above threshold, fails one below it,
// and is inconclusive when the metric is undefined: the corpus or partition
// did not provide the evidence to compute it.
func atLeast(id string, metric Metric, threshold float64) Gate {
	gate := Gate{ID: id, Metric: metric, Operator: operatorAtLeast, Threshold: threshold}
	switch {
	case !metric.Defined():
		gate.Outcome = GateInconclusive
		gate.Reason = metric.Reason
	case metric.Value >= threshold:
		gate.Outcome = GatePass
	default:
		gate.Outcome = GateFail
	}

	return gate
}

// gain is the smaller of the micro and macro differences between model and
// baseline. It is undefined, with the first missing reason, when any of the
// four values is undefined.
func gain(model, baseline Evaluation) Metric {
	for _, metric := range []Metric{model.Micro, model.Macro, baseline.Micro, baseline.Macro} {
		if !metric.Defined() {
			return metric
		}
	}

	return defined(math.Min(
		model.Micro.Value-baseline.Micro.Value,
		model.Macro.Value-baseline.Macro.Value,
	))
}

// cutoffDisplayGate requires every cutoff at or above the inference floor to
// display at least one candidate; the display limit already caps it at 10. A
// cutoff above the floor that displays nothing fails the gate even when
// another cutoff is below the floor, because that is valid contrary evidence.
func cutoffDisplayGate(displays []Display) Gate {
	gate := Gate{ID: GateCutoffDisplay, Operator: operatorAtLeast, Threshold: minimumDisplayedPerCutoff}
	fewest := math.Inf(1)
	abstained := false
	for _, display := range displays {
		if display.Abstained {
			abstained = true
			continue
		}
		fewest = math.Min(fewest, float64(len(display.Candidates)))
	}

	switch {
	case !math.IsInf(fewest, 1) && fewest < minimumDisplayedPerCutoff:
		gate.Metric = defined(fewest)
		gate.Outcome = GateFail
	case abstained:
		gate.Metric = undefined(ReasonBelowInferenceFloor)
		gate.Reason = ReasonBelowInferenceFloor
		gate.Outcome = GateInconclusive
	default:
		gate.Metric = defined(fewest)
		gate.Outcome = GatePass
	}

	return gate
}

// windowsCoveredGate requires at least one covered transaction in every
// evaluation window. A window after a cutoff above the floor that covers
// nothing is a failure; a window after an abstaining cutoff, or with no
// eligible transaction, cannot be judged.
func windowsCoveredGate(displays []Display, model Evaluation) Gate {
	gate := Gate{ID: GateEvaluationWindowsCovered, Operator: operatorAtLeast, Threshold: minimumCoveredPerWindow}
	fewest := math.Inf(1)
	var reason Reason
	for index, covered := range model.Covered {
		switch {
		case displays[index].Abstained:
			reason = cmp.Or(reason, ReasonBelowInferenceFloor)
		case model.Transactions[index] == 0:
			reason = cmp.Or(reason, ReasonNoEligibleEvaluationEvents)
		default:
			fewest = math.Min(fewest, float64(covered))
		}
	}

	switch {
	case !math.IsInf(fewest, 1) && fewest < minimumCoveredPerWindow:
		gate.Metric = defined(fewest)
		gate.Outcome = GateFail
	case reason != "":
		gate.Metric = undefined(reason)
		gate.Reason = reason
		gate.Outcome = GateInconclusive
	default:
		gate.Metric = defined(fewest)
		gate.Outcome = GatePass
	}

	return gate
}

// conjunction fails when any gate fails, is inconclusive when any other gate
// is inconclusive, and passes only when every gate passes.
func conjunction(gates []Gate) GateOutcome {
	outcome := GatePass
	for _, gate := range gates {
		switch gate.Outcome {
		case GateFail:
			return GateFail
		case GateInconclusive:
			outcome = GateInconclusive
		}
	}

	return outcome
}
