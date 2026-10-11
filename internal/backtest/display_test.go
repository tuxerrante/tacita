package backtest

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/tuxerrante/tacita/internal/mining"
)

// pairedAggregate holds pairs disjoint directory pairs that always change
// together ten times, padded with single-directory transactions to total.
func pairedAggregate(t *testing.T, pairs int, total int) mining.Aggregate {
	t.Helper()

	accumulator := mining.NewAccumulator()
	observed := 0
	for pair := range pairs {
		for range 10 {
			components := []string{fmt.Sprintf("a%02d", pair), fmt.Sprintf("b%02d", pair)}
			if err := accumulator.Observe(components); err != nil {
				t.Fatalf("Observe() error = %v", err)
			}
			observed++
		}
	}
	for ; observed < total; observed++ {
		if err := accumulator.Observe([]string{"padding"}); err != nil {
			t.Fatalf("Observe() error = %v", err)
		}
	}
	return accumulator.Snapshot()
}

func unitConfiguration() mining.Configuration {
	return mining.Configuration{
		SizeWeight:           mining.WeightUnit,
		MinimumOpportunities: 5,
		MinimumConfidence:    0.70,
		MinimumLift:          1.25,
	}
}

func TestDisplayCandidatesKeepsTheFirstTenRanked(t *testing.T) {
	aggregate := pairedAggregate(t, 12, 150)
	configuration := unitConfiguration()

	ranked, err := mining.DeriveCandidates(aggregate, configuration)
	if err != nil {
		t.Fatalf("DeriveCandidates() error = %v", err)
	}
	displays, err := DisplayCandidates(aggregate, []mining.Configuration{configuration})
	if err != nil {
		t.Fatalf("DisplayCandidates() error = %v", err)
	}

	if len(displays) != 1 {
		t.Fatalf("returned %d displays, want 1", len(displays))
	}
	display := displays[0]
	if display.Abstained || display.Eligible != 24 || len(display.Candidates) != displayLimit {
		t.Fatalf("display = abstained %t eligible %d shown %d, want false 24 %d",
			display.Abstained, display.Eligible, len(display.Candidates), displayLimit)
	}
	names := make(map[mining.ComponentID]string)
	for _, component := range aggregate.Components {
		names[component.ID] = component.Name
	}
	for index, shown := range display.Candidates {
		if shown.Candidate != ranked[index] {
			t.Errorf("candidate %d = %+v, want rank %d %+v", index, shown.Candidate, index+1, ranked[index])
		}
		if shown.Antecedent != names[ranked[index].Antecedent] ||
			shown.Consequent != names[ranked[index].Consequent] {
			t.Errorf("candidate %d names = %q -> %q, want %q -> %q", index,
				shown.Antecedent, shown.Consequent,
				names[ranked[index].Antecedent], names[ranked[index].Consequent])
		}
	}
}

func TestDisplayCandidatesShowsEveryCandidateBelowTheLimit(t *testing.T) {
	aggregate := pairedAggregate(t, 2, 120)

	displays, err := DisplayCandidates(aggregate, []mining.Configuration{unitConfiguration()})
	if err != nil {
		t.Fatalf("DisplayCandidates() error = %v", err)
	}

	if got := displays[0]; got.Abstained || got.Eligible != 4 || len(got.Candidates) != 4 {
		t.Errorf("display = abstained %t eligible %d shown %d, want false 4 4",
			got.Abstained, got.Eligible, len(got.Candidates))
	}
}

func TestDisplayCandidatesDistinguishesAbstentionFromNoCandidates(t *testing.T) {
	tests := []struct {
		name      string
		aggregate mining.Aggregate
		abstained bool
	}{
		{name: "below the inference floor", aggregate: pairedAggregate(t, 2, 99), abstained: true},
		{name: "no eligible pair", aggregate: pairedAggregate(t, 0, 100), abstained: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			displays, err := DisplayCandidates(tt.aggregate, []mining.Configuration{unitConfiguration()})
			if err != nil {
				t.Fatalf("DisplayCandidates() error = %v", err)
			}

			got := displays[0]
			if got.Abstained != tt.abstained || got.Eligible != 0 || len(got.Candidates) != 0 {
				t.Fatalf("display = abstained %t eligible %d shown %d, want %t 0 0",
					got.Abstained, got.Eligible, len(got.Candidates), tt.abstained)
			}
			if (got.Candidates == nil) != tt.abstained {
				t.Errorf("Candidates nil = %t, want %t", got.Candidates == nil, tt.abstained)
			}
		})
	}
}

func TestDisplayCandidatesCoversTheGridInOrder(t *testing.T) {
	configurations := mining.AllConfigurations()

	displays, err := DisplayCandidates(pairedAggregate(t, 3, 130), configurations)
	if err != nil {
		t.Fatalf("DisplayCandidates() error = %v", err)
	}

	if len(displays) != len(configurations) {
		t.Fatalf("returned %d displays, want %d", len(displays), len(configurations))
	}
	for index, display := range displays {
		if display.Configuration != configurations[index] {
			t.Errorf("display %d configuration = %s, want %s",
				index, display.Configuration.ID(), configurations[index].ID())
		}
	}
}

func TestDisplayCandidatesRejectsAnInvalidConfiguration(t *testing.T) {
	invalid := unitConfiguration()
	invalid.MinimumLift = 1.75

	_, err := DisplayCandidates(pairedAggregate(t, 2, 120), []mining.Configuration{invalid})

	if !errors.Is(err, mining.ErrInvalidConfiguration) {
		t.Fatalf("DisplayCandidates() error = %v, want mining.ErrInvalidConfiguration", err)
	}
}

func TestFoldAndDisplayAreRepeatable(t *testing.T) {
	const eventCount = 600
	var transactions []Transaction
	for position := 2; position <= eventCount; position++ {
		group := position % 7
		components := []string{fmt.Sprintf("a%d", group), fmt.Sprintf("b%d", group)}
		if position%5 == 0 {
			components = []string{fmt.Sprintf("a%d", group)}
		}
		transactions = append(transactions, Transaction{Position: position, Components: components})
	}
	partitions, err := Partitions(eventCount)
	if err != nil {
		t.Fatalf("Partitions() error = %v", err)
	}
	run := func() [][]Display {
		var cutoffs [][]Display
		err := Fold(transactions, partitions, func(_ Partition, aggregate mining.Aggregate) error {
			displays, err := DisplayCandidates(aggregate, mining.AllConfigurations())
			cutoffs = append(cutoffs, displays)
			return err
		})
		if err != nil {
			t.Fatalf("Fold() error = %v", err)
		}
		return cutoffs
	}

	first, second := run(), run()

	if !reflect.DeepEqual(first, second) {
		t.Error("two folds over identical input displayed different candidates")
	}
	shown := 0
	for _, displays := range first {
		for _, display := range displays {
			shown += len(display.Candidates)
		}
	}
	if shown == 0 {
		t.Error("the repeated fold displayed no candidate, so the comparison proves nothing")
	}
}
