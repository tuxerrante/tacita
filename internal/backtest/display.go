package backtest

import (
	"fmt"

	"github.com/tuxerrante/tacita/internal/mining"
)

// displayLimit is the frozen number of candidates each cutoff displays.
const displayLimit = 10

// Display is one configuration's displayed candidates at one cutoff.
type Display struct {
	Configuration mining.Configuration
	// Abstained reports a training partition below the inference floor.
	Abstained bool
	// Eligible counts every eligible candidate before the display limit.
	Eligible int
	// Candidates holds at most the first 10 ranked candidates.
	Candidates []DisplayedCandidate
}

// DisplayedCandidate is a ranked candidate with its canonical component bytes.
// Component IDs are local to one accumulation, so later stages compare names.
type DisplayedCandidate struct {
	Candidate  mining.Candidate
	Antecedent string
	Consequent string
}

// DisplayCandidates derives every configuration from one training aggregate
// and keeps the first 10 ranked candidates of each, in configuration order.
func DisplayCandidates(
	aggregate mining.Aggregate,
	configurations []mining.Configuration,
) ([]Display, error) {
	names := make(map[mining.ComponentID]string, len(aggregate.Components))
	for _, component := range aggregate.Components {
		names[component.ID] = component.Name
	}

	displays := make([]Display, 0, len(configurations))
	for _, configuration := range configurations {
		candidates, err := mining.DeriveCandidates(aggregate, configuration)
		if err != nil {
			return nil, fmt.Errorf("deriving %s: %w", configuration.ID(), err)
		}

		display := Display{
			Configuration: configuration,
			Abstained:     candidates == nil,
			Eligible:      len(candidates),
		}
		shown := candidates[:min(len(candidates), displayLimit)]
		if !display.Abstained {
			display.Candidates = make([]DisplayedCandidate, 0, len(shown))
		}
		for _, candidate := range shown {
			display.Candidates = append(display.Candidates, DisplayedCandidate{
				Candidate:  candidate,
				Antecedent: names[candidate.Antecedent],
				Consequent: names[candidate.Consequent],
			})
		}
		displays = append(displays, display)
	}

	return displays, nil
}

// Rules returns what this display shows, in rank order, for evaluation.
func (d Display) Rules() CutoffRules {
	rules := make([]Rule, len(d.Candidates))
	for index, candidate := range d.Candidates {
		rules[index] = Rule{Antecedent: candidate.Antecedent, Consequent: candidate.Consequent}
	}

	return CutoffRules{Abstained: d.Abstained, Rules: rules}
}
