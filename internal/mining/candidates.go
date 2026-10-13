package mining

import "math"

const (
	minEligibleTransactions = 100
	minRawSupport           = 3
)

// Candidate is one eligible directional descriptive candidate for one
// configuration, derived from one completed Aggregate.
type Candidate struct {
	Antecedent         ComponentID
	Consequent         ComponentID
	RawSupport         uint64
	RawOpportunity     uint64
	WeightedSupport    float64
	WeightedExposure   float64
	WeightedConfidence float64
	WeightedPrevalence float64
	WeightedLift       float64
}

func (w Weights) forMode(mode WeightMode) float64 {
	switch mode {
	case WeightUnit:
		return w.Unit
	case WeightInverseComponent:
		return w.InverseComponent
	case WeightPairNormalized:
		return w.PairNormalized
	default:
		return math.NaN()
	}
}

// DeriveCandidates derives eligible candidates for one configuration from one
// completed Aggregate snapshot. It never mutates aggregate.
//
// It abstains below 100 eligible transactions by returning (nil, nil). A
// non-abstaining call returns a non-nil slice even when no pairs qualify. An
// invalid configuration or malformed component reference returns a typed
// error. A pair with non-finite or non-positive weighted inputs is ineligible.
func DeriveCandidates(aggregate Aggregate, configuration Configuration) ([]Candidate, error) {
	if err := configuration.validate(); err != nil {
		return nil, err
	}
	if aggregate.EligibleTransactions < minEligibleTransactions {
		return nil, nil
	}

	components, err := indexComponents(aggregate.Components)
	if err != nil {
		return nil, err
	}

	eligibleWeight := aggregate.EligibleWeight.forMode(configuration.SizeWeight)
	candidates := make([]Candidate, 0)
	for _, pair := range aggregate.Pairs {
		antecedent, ok := components[pair.Antecedent]
		if !ok {
			return nil, &UnknownComponentError{ComponentID: pair.Antecedent}
		}
		consequent, ok := components[pair.Consequent]
		if !ok {
			return nil, &UnknownComponentError{ComponentID: pair.Consequent}
		}
		if pair.Antecedent == pair.Consequent {
			return nil, &SelfPairError{ComponentID: pair.Antecedent}
		}

		if pair.RawSupport < minRawSupport {
			continue
		}
		if antecedent.RawOpportunity < configuration.MinimumOpportunities {
			continue
		}

		weightedSupport := pair.Support.forMode(configuration.SizeWeight)
		weightedExposure := antecedent.Occurrence.forMode(configuration.SizeWeight)
		if !isFinitePositive(weightedSupport) || !isFinitePositive(weightedExposure) {
			continue
		}

		weightedConfidence := weightedSupport / weightedExposure
		if !isFinite(weightedConfidence) || weightedConfidence < configuration.MinimumConfidence {
			continue
		}

		weightedConsequentOccurrence := consequent.Occurrence.forMode(configuration.SizeWeight)
		if !isFinitePositive(weightedConsequentOccurrence) || !isFinitePositive(eligibleWeight) {
			continue
		}

		weightedPrevalence := weightedConsequentOccurrence / eligibleWeight
		if !isFinitePositive(weightedPrevalence) {
			continue
		}

		weightedLift := weightedConfidence / weightedPrevalence
		if !isFinite(weightedLift) || weightedLift < configuration.MinimumLift {
			continue
		}

		candidates = append(candidates, Candidate{
			Antecedent:         pair.Antecedent,
			Consequent:         pair.Consequent,
			RawSupport:         pair.RawSupport,
			RawOpportunity:     antecedent.RawOpportunity,
			WeightedSupport:    weightedSupport,
			WeightedExposure:   weightedExposure,
			WeightedConfidence: weightedConfidence,
			WeightedPrevalence: weightedPrevalence,
			WeightedLift:       weightedLift,
		})
	}

	return candidates, nil
}

func indexComponents(components []Component) (map[ComponentID]Component, error) {
	index := make(map[ComponentID]Component, len(components))
	for _, component := range components {
		if _, exists := index[component.ID]; exists {
			return nil, &DuplicateComponentIdentityError{ComponentID: component.ID}
		}
		index[component.ID] = component
	}

	return index, nil
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func isFinitePositive(v float64) bool {
	return isFinite(v) && v > 0
}
