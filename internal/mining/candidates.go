package mining

import (
	"cmp"
	"fmt"
	"math"
	"slices"
)

const (
	// minEligibleTransactions is the frozen inference-abstention floor: no
	// configuration derives candidates below this many eligible transactions.
	minEligibleTransactions = 100
	// minRawSupport is the frozen raw-support floor applied before any grid
	// configuration filter.
	minRawSupport = 3
)

// WeightMode names one of the three frozen descriptive size-weight modes.
type WeightMode string

// The frozen size-weight modes, in their canonical simplicity order.
const (
	WeightUnit             WeightMode = "unit"
	WeightInverseComponent WeightMode = "inverse-component"
	WeightPairNormalized   WeightMode = "pair-normalized"
)

// weightModes lists the frozen size-weight modes in grid order.
var weightModes = []WeightMode{WeightUnit, WeightInverseComponent, WeightPairNormalized}

// minimumOpportunityValues, minimumConfidenceValues, and minimumLiftValues are
// the frozen candidate configuration grid values for the remaining three
// parameters, in grid order.
var (
	minimumOpportunityValues = []uint64{5, 10, 20}
	minimumConfidenceValues  = []float64{0.70, 0.80, 0.90}
	minimumLiftValues        = []float64{1.25, 1.50, 2.00}
)

// Configuration is one point in the frozen 81-point candidate configuration
// grid: a size-weight mode and the three post-aggregation filter thresholds.
type Configuration struct {
	SizeWeight           WeightMode
	MinimumOpportunities uint64
	MinimumConfidence    float64
	MinimumLift          float64
}

// ID returns the canonical configuration ID:
//
//	weight=<name>;opportunities=<integer>;confidence=<two decimals>;lift=<two decimals>
func (c Configuration) ID() string {
	return fmt.Sprintf(
		"weight=%s;opportunities=%d;confidence=%.2f;lift=%.2f",
		c.SizeWeight, c.MinimumOpportunities, c.MinimumConfidence, c.MinimumLift,
	)
}

// validate reports whether c is one of the 81 frozen grid points. Any other
// value is rejected rather than silently treated as always-ineligible.
func (c Configuration) validate() error {
	if !slices.Contains(weightModes, c.SizeWeight) {
		return &ConfigurationError{Field: "SizeWeight", Value: string(c.SizeWeight)}
	}
	if !slices.Contains(minimumOpportunityValues, c.MinimumOpportunities) {
		return &ConfigurationError{
			Field: "MinimumOpportunities",
			Value: fmt.Sprintf("%d", c.MinimumOpportunities),
		}
	}
	if !slices.Contains(minimumConfidenceValues, c.MinimumConfidence) {
		return &ConfigurationError{
			Field: "MinimumConfidence",
			Value: fmt.Sprintf("%v", c.MinimumConfidence),
		}
	}
	if !slices.Contains(minimumLiftValues, c.MinimumLift) {
		return &ConfigurationError{
			Field: "MinimumLift",
			Value: fmt.Sprintf("%v", c.MinimumLift),
		}
	}

	return nil
}

// AllConfigurations returns all 81 frozen candidate configuration grid points,
// in ascending canonical ID order. Each call returns a freshly built,
// detached slice.
func AllConfigurations() []Configuration {
	configurations := make(
		[]Configuration,
		0,
		len(weightModes)*len(minimumOpportunityValues)*len(minimumConfidenceValues)*len(minimumLiftValues),
	)

	for _, weight := range weightModes {
		for _, opportunities := range minimumOpportunityValues {
			for _, confidence := range minimumConfidenceValues {
				for _, lift := range minimumLiftValues {
					configurations = append(configurations, Configuration{
						SizeWeight:           weight,
						MinimumOpportunities: opportunities,
						MinimumConfidence:    confidence,
						MinimumLift:          lift,
					})
				}
			}
		}
	}

	slices.SortFunc(configurations, func(left, right Configuration) int {
		return cmp.Compare(left.ID(), right.ID())
	})

	return configurations
}

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
	// Rank is the candidate's 1-based position under the frozen ranking order
	// among the eligible candidates returned for this configuration.
	Rank int

	// antecedentName and consequentName cache the two byte-order tie-break
	// keys alongside each eligible candidate so ranking never needs a
	// map[ComponentID]Component lookup per comparison. They are cleared
	// before DeriveCandidates returns, so they never leak into the public
	// result.
	antecedentName, consequentName string
}

// forMode selects the weighted value for the requested size-weight mode.
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

// DeriveCandidates derives ranked eligible candidates for one configuration
// from one completed Aggregate snapshot. It never mutates aggregate.
//
// It abstains when aggregate has fewer than 100 eligible transactions,
// returning (nil, nil); abstention is not an error. A non-abstaining call
// always returns a non-nil slice, even when zero pairs qualify, so callers can
// distinguish "abstained" (nil) from "evaluated, nothing eligible" (non-nil,
// empty) by a nil check alone. An invalid configuration or a structurally
// malformed aggregate (a duplicate component identity or byte-identical name,
// an unknown component reference, a self-pair, or a duplicate directional pair
// identity) is a typed error. A pair whose derived metrics are not all finite
// and positive where required is excluded rather than treated as an error.
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

	// candidates is explicitly non-nil (unlike a nil-valued var) so that a
	// valid, eligible aggregate with zero qualifying pairs is distinguishable
	// from abstention: callers can rely on nil meaning "abstained" and a
	// non-nil empty slice meaning "evaluated, nothing qualified". Growth is
	// proportional to eligible output, not preallocated to the pair ceiling.
	candidates := make([]Candidate, 0)
	var (
		previousPair pairKey
		havePrevious bool
		seenPairs    map[pairKey]bool
	)

	for pairIndex, pair := range aggregate.Pairs {
		// Every pair reference is resolved before any raw-support or
		// opportunity filtering, so a malformed reference always produces the
		// same typed error regardless of whether the pair would otherwise
		// have been filtered out.
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

		identity := pairKey{antecedent: pair.Antecedent, consequent: pair.Consequent}
		if seenPairs != nil && seenPairs[identity] {
			return nil, &DuplicatePairIdentityError{
				Antecedent: pair.Antecedent,
				Consequent: pair.Consequent,
			}
		}
		if seenPairs == nil && havePrevious {
			switch comparePairKeys(identity, previousPair) {
			case 0:
				return nil, &DuplicatePairIdentityError{
					Antecedent: pair.Antecedent,
					Consequent: pair.Consequent,
				}
			case -1:
				// Snapshot pairs are strictly sorted, so valid production
				// aggregates need no duplicate-tracking map. Allocate one
				// only for an out-of-order hand-built aggregate, seeding it
				// with the already validated prefix.
				seenPairs = make(map[pairKey]bool, len(aggregate.Pairs))
				for _, previous := range aggregate.Pairs[:pairIndex] {
					seenPairs[pairKey{
						antecedent: previous.Antecedent,
						consequent: previous.Consequent,
					}] = true
				}
				if seenPairs[identity] {
					return nil, &DuplicatePairIdentityError{
						Antecedent: pair.Antecedent,
						Consequent: pair.Consequent,
					}
				}
			}
		}
		if seenPairs != nil {
			seenPairs[identity] = true
		}
		previousPair = identity
		havePrevious = true

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
			// antecedent and consequent are already resolved above, so
			// caching their names here costs no extra lookup; it only saves
			// one at sort time, repeated across every pairwise comparison.
			antecedentName: antecedent.Name,
			consequentName: consequent.Name,
		})
	}

	// compareCandidates is a direct, map-free comparator: every key it reads
	// is a plain field already carried on Candidate, and it returns as soon
	// as an earlier key decides the order instead of unconditionally
	// evaluating all six keys the way cmp.Or's eagerly evaluated arguments
	// would.
	slices.SortFunc(candidates, compareCandidates)

	for index := range candidates {
		candidates[index].Rank = index + 1
		// The name tie-break keys are sort-only scratch state; clear them so
		// they never leak into the public result.
		candidates[index].antecedentName = ""
		candidates[index].consequentName = ""
	}

	return candidates, nil
}

func comparePairKeys(left, right pairKey) int {
	return cmp.Or(
		cmp.Compare(left.antecedent, right.antecedent),
		cmp.Compare(left.consequent, right.consequent),
	)
}

// compareCandidates orders candidates by the frozen ranking precedence:
// weighted confidence, then weighted lift, then raw support, then raw
// opportunity, then antecedent name bytes, then consequent name bytes, all
// descending except the two name keys, which are ascending. It short-circuits
// on the first deciding key instead of computing all six unconditionally.
func compareCandidates(left, right Candidate) int {
	if c := cmp.Compare(right.WeightedConfidence, left.WeightedConfidence); c != 0 {
		return c
	}
	if c := cmp.Compare(right.WeightedLift, left.WeightedLift); c != 0 {
		return c
	}
	if c := cmp.Compare(right.RawSupport, left.RawSupport); c != 0 {
		return c
	}
	if c := cmp.Compare(right.RawOpportunity, left.RawOpportunity); c != 0 {
		return c
	}
	if c := cmp.Compare(left.antecedentName, right.antecedentName); c != 0 {
		return c
	}

	return cmp.Compare(left.consequentName, right.consequentName)
}

// indexComponents builds a safe, identity- and name-checked lookup from an
// exported Aggregate's component slice. It never assumes ComponentID values
// index the slice directly, so a malformed or hand-built Aggregate cannot
// cause an out-of-range panic. Production Snapshot state interns components by
// name, so distinct ComponentID values with byte-identical Name values can
// only occur in a hand-built Aggregate; that is rejected as malformed rather
// than silently accepted, because the frozen ranking order's byte-level name
// tie-breakers assume distinct names.
func indexComponents(components []Component) (map[ComponentID]Component, error) {
	index := make(map[ComponentID]Component, len(components))
	names := make(map[string]bool, len(components))
	for _, component := range components {
		if _, exists := index[component.ID]; exists {
			return nil, &DuplicateComponentIdentityError{ComponentID: component.ID}
		}
		if names[component.Name] {
			return nil, &DuplicateComponentNameError{Name: component.Name}
		}
		names[component.Name] = true
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
