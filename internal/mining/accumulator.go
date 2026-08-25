package mining

import (
	"cmp"
	"slices"
)

const (
	maxTransactionComponents = 100
	maxComponentIdentities   = 50_000
	maxPairObservations      = 20_000_000
	maxPairIdentities        = 2_000_000
)

// ComponentID is the run-local integer identity of one canonical component.
type ComponentID uint32

// Weights holds the three frozen size-weight modes for one value.
type Weights struct {
	Unit             float64
	InverseComponent float64
	PairNormalized   float64
}

// Component is one component's accumulated descriptive state.
type Component struct {
	ID             ComponentID
	Name           string
	RawOpportunity uint64
	Occurrence     Weights
}

// Pair is one directional pair's accumulated descriptive state.
type Pair struct {
	Antecedent ComponentID
	Consequent ComponentID
	RawSupport uint64
	Support    Weights
}

// Aggregate is a detached snapshot of one ordered accumulation fold.
type Aggregate struct {
	EligibleTransactions uint64
	PairObservations     int
	EligibleWeight       Weights
	Components           []Component
	Pairs                []Pair
}

// Accumulator folds canonical component sets into bounded descriptive state.
//
// The zero value is ready to use with the frozen production limits. Observe
// expects the distinct components of one eligible transaction in canonical
// input order.
type Accumulator struct {
	initialized            bool
	componentIdentityLimit int
	pairObservationLimit   int
	pairIdentityLimit      int
	pairObservations       int
	eligibleTransactions   uint64
	eligibleWeight         Weights
	nextComponentID        ComponentID
	componentIDs           map[string]ComponentID
	components             []componentState
	pairs                  map[pairKey]pairState
}

type componentState struct {
	id             ComponentID
	name           string
	rawOpportunity uint64
	occurrence     Weights
}

type pairKey struct {
	antecedent ComponentID
	consequent ComponentID
}

type pairState struct {
	rawSupport uint64
	support    Weights
}

// NewAccumulator returns an empty accumulator using the frozen production
// limits.
func NewAccumulator() *Accumulator {
	return newAccumulator(limits{
		components:       maxComponentIdentities,
		pairObservations: maxPairObservations,
		pairs:            maxPairIdentities,
	})
}

type limits struct {
	components       int
	pairObservations int
	pairs            int
}

func newAccumulator(limit limits) *Accumulator {
	return &Accumulator{
		initialized:            true,
		componentIdentityLimit: limit.components,
		pairObservationLimit:   limit.pairObservations,
		pairIdentityLimit:      limit.pairs,
		componentIDs:           make(map[string]ComponentID),
		pairs:                  make(map[pairKey]pairState),
	}
}

// Observe adds one eligible transaction atomically. Invalid transactions and
// transactions that would cross a state budget contribute no state.
func (a *Accumulator) Observe(components []string) error {
	a.initialize()

	ids, pending, err := a.planComponents(components)
	if err != nil {
		return err
	}

	componentIdentities := len(a.components) + len(pending)
	if componentIdentities > a.componentIdentityLimit {
		return &ComponentIdentityLimitError{
			Observed: a.componentIdentityLimit + 1,
			Limit:    a.componentIdentityLimit,
		}
	}

	pairCount := len(ids) * (len(ids) - 1)
	observed := a.pairObservations + pairCount
	if observed > a.pairObservationLimit {
		return &PairObservationLimitError{
			Observed: a.pairObservationLimit + 1,
			Limit:    a.pairObservationLimit,
		}
	}

	newPairs := a.countNewPairs(ids)
	pairIdentities := len(a.pairs) + newPairs
	if pairIdentities > a.pairIdentityLimit {
		return &PairIdentityLimitError{
			Observed: a.pairIdentityLimit + 1,
			Limit:    a.pairIdentityLimit,
		}
	}

	a.commitComponents(pending)

	weights := weightsFor(len(ids))
	a.eligibleTransactions++
	a.pairObservations = observed
	a.eligibleWeight.add(weights)

	for _, id := range ids {
		component := &a.components[id]
		component.rawOpportunity++
		component.occurrence.add(weights)
	}

	for _, antecedent := range ids {
		for _, consequent := range ids {
			if antecedent == consequent {
				continue
			}

			key := pairKey{antecedent: antecedent, consequent: consequent}
			pair := a.pairs[key]
			pair.rawSupport++
			pair.support.add(weights)
			a.pairs[key] = pair
		}
	}

	return nil
}

// Snapshot returns a detached, deterministically ordered copy of the current
// aggregate. The accumulator may continue observing transactions afterward.
func (a *Accumulator) Snapshot() Aggregate {
	a.initialize()

	components := make([]Component, len(a.components))
	for id, state := range a.components {
		components[id] = Component{
			ID:             state.id,
			Name:           state.name,
			RawOpportunity: state.rawOpportunity,
			Occurrence:     state.occurrence,
		}
	}

	pairs := make([]Pair, 0, len(a.pairs))
	for key, state := range a.pairs {
		pairs = append(pairs, Pair{
			Antecedent: key.antecedent,
			Consequent: key.consequent,
			RawSupport: state.rawSupport,
			Support:    state.support,
		})
	}
	slices.SortFunc(pairs, func(left, right Pair) int {
		return cmp.Or(
			cmp.Compare(left.Antecedent, right.Antecedent),
			cmp.Compare(left.Consequent, right.Consequent),
		)
	})

	return Aggregate{
		EligibleTransactions: a.eligibleTransactions,
		PairObservations:     a.pairObservations,
		EligibleWeight:       a.eligibleWeight,
		Components:           components,
		Pairs:                pairs,
	}
}

func (a *Accumulator) initialize() {
	if a.initialized {
		return
	}

	a.initialized = true
	a.componentIdentityLimit = maxComponentIdentities
	a.pairObservationLimit = maxPairObservations
	a.pairIdentityLimit = maxPairIdentities
	a.componentIDs = make(map[string]ComponentID)
	a.pairs = make(map[pairKey]pairState)
}

func (a *Accumulator) planComponents(
	components []string,
) ([]ComponentID, []string, error) {
	if len(components) == 0 {
		return nil, nil, ErrEmptyTransaction
	}
	if len(components) > maxTransactionComponents {
		return nil, nil, &TransactionComponentLimitError{
			Observed: maxTransactionComponents + 1,
			Limit:    maxTransactionComponents,
		}
	}

	ids := make([]ComponentID, len(components))
	pending := make([]string, 0, len(components))
	firstIndex := make(map[string]int, len(components))
	nextID := a.nextComponentID

	for index, component := range components {
		if first, ok := firstIndex[component]; ok {
			return nil, nil, &DuplicateComponentError{
				First:     first,
				Duplicate: index,
			}
		}
		firstIndex[component] = index

		if id, ok := a.componentIDs[component]; ok {
			ids[index] = id
			continue
		}
		id := nextID
		nextID++
		pending = append(pending, component)
		ids[index] = id
	}

	return ids, pending, nil
}

func (a *Accumulator) countNewPairs(ids []ComponentID) int {
	count := 0
	for _, antecedent := range ids {
		for _, consequent := range ids {
			if antecedent == consequent {
				continue
			}
			if _, ok := a.pairs[pairKey{
				antecedent: antecedent,
				consequent: consequent,
			}]; !ok {
				count++
			}
		}
	}

	return count
}

func (a *Accumulator) commitComponents(components []string) {
	for _, component := range components {
		id := a.nextComponentID
		a.nextComponentID++
		a.componentIDs[component] = id
		a.components = append(a.components, componentState{id: id, name: component})
	}
}

func weightsFor(componentCount int) Weights {
	pairCount := componentCount * (componentCount - 1)

	return Weights{
		Unit:             1,
		InverseComponent: 1 / float64(max(1, componentCount-1)),
		PairNormalized:   1 / float64(max(1, pairCount)),
	}
}

func (w *Weights) add(other Weights) {
	w.Unit += other.Unit
	w.InverseComponent += other.InverseComponent
	w.PairNormalized += other.PairNormalized
}
