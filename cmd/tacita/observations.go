package main

import (
	"github.com/tuxerrante/tacita/internal/gitlog"
	"github.com/tuxerrante/tacita/internal/mining"
	"github.com/tuxerrante/tacita/internal/report"
)

// Gate and control outcomes the command writes.
const (
	outcomePass = "pass"
	outcomeFail = "fail"
)

// observations counts the deterministic work a calibration run performed, for
// the report's budget records.
type observations struct {
	paths            map[string]struct{}
	components       map[string]uint32
	pairs            map[[2]uint32]struct{}
	pairObservations int64
	eventPaths       int
	eventComponents  int
	observationLimit int64
	pairLimit        int
}

func newObservations() *observations {
	return &observations{
		paths:      make(map[string]struct{}),
		components: make(map[string]uint32),
		pairs:      make(map[[2]uint32]struct{}),
		// The frozen pair budgets, enforced as transactions arrive.
		observationLimit: maxPairObservations,
		pairLimit:        maxPairIdentities,
	}
}

// Frozen pair budgets, enforced here as the transactions arrive so the pair
// set cannot outgrow them before the miner sees the history.
const (
	maxPairObservations = 20_000_000
	maxPairIdentities   = 2_000_000
)

// transaction records one eligible transaction. Ingestion bounds path and
// component identities; this records pairs only within the frozen pair
// budgets and returns the miner's typed limit error at the first breach,
// leaving the transaction unrecorded.
func (o *observations) transaction(transaction gitlog.Transaction) error {
	count := int64(len(transaction.Components))
	if o.pairObservations+count*(count-1) > o.observationLimit {
		return &mining.PairObservationLimitError{Observed: int(o.observationLimit) + 1, Limit: int(o.observationLimit)}
	}

	o.eventPaths = max(o.eventPaths, len(transaction.Paths))
	o.eventComponents = max(o.eventComponents, len(transaction.Components))
	for _, path := range transaction.Paths {
		o.paths[path] = struct{}{}
	}

	ids := make([]uint32, len(transaction.Components))
	for index, component := range transaction.Components {
		id, ok := o.components[component]
		if !ok {
			id = uint32(len(o.components)) //nolint:gosec // bounded by the 50,000-component budget
			o.components[component] = id
		}
		ids[index] = id
	}
	var added [][2]uint32
	for _, antecedent := range ids {
		for _, consequent := range ids {
			key := [2]uint32{antecedent, consequent}
			if antecedent == consequent {
				continue
			}
			if _, seen := o.pairs[key]; !seen {
				added = append(added, key)
			}
		}
	}
	if len(o.pairs)+len(added) > o.pairLimit {
		return &mining.PairIdentityLimitError{Observed: o.pairLimit + 1, Limit: o.pairLimit}
	}
	for _, key := range added {
		o.pairs[key] = struct{}{}
	}
	o.pairObservations += count * (count - 1)

	return nil
}

// budgets renders every frozen budget record. Observed budgets pass when the
// observation is within the limit; unobserved ones carry null.
func (o *observations) budgets(events int, rows int) []report.Budget {
	observed := map[string]int64{
		"distinct_components": int64(len(o.components)),
		"distinct_pairs":      int64(len(o.pairs)),
		"distinct_paths":      int64(len(o.paths)),
		"event_components":    int64(o.eventComponents),
		"event_paths":         int64(o.eventPaths),
		"first_parent_events": int64(events),
		"pair_observations":   o.pairObservations,
		"report_rows":         int64(rows),
	}

	specs := report.FrozenBudgets()
	records := make([]report.Budget, len(specs))
	for index, spec := range specs {
		records[index] = report.Budget{ID: spec.ID, Unit: spec.Unit, Limit: spec.Limit}
		if spec.Unobserved {
			continue
		}
		value := observed[spec.ID]
		outcome := outcomePass
		if value > spec.Limit {
			outcome = outcomeFail
		}
		records[index].Observed = &value
		records[index].Outcome = &outcome
	}

	return records
}
