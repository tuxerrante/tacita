package main

import (
	"github.com/tuxerrante/tacita/internal/gitlog"
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
}

func newObservations() *observations {
	return &observations{
		paths:      make(map[string]struct{}),
		components: make(map[string]uint32),
		pairs:      make(map[[2]uint32]struct{}),
	}
}

// transaction records one eligible transaction. Ingestion has already bounded
// every identity budget, so these sets cannot outgrow the frozen limits.
func (o *observations) transaction(transaction gitlog.Transaction) {
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
	count := int64(len(ids))
	o.pairObservations += count * (count - 1)
	for _, antecedent := range ids {
		for _, consequent := range ids {
			if antecedent != consequent {
				o.pairs[[2]uint32{antecedent, consequent}] = struct{}{}
			}
		}
	}
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
