package main

import (
	"errors"
	"testing"

	"github.com/tuxerrante/tacita/internal/gitlog"
	"github.com/tuxerrante/tacita/internal/mining"
)

func TestObservationsEnforceThePairBudgets(t *testing.T) {
	transaction := func(components ...string) gitlog.Transaction {
		return gitlog.Transaction{Components: components, Paths: components}
	}

	pairs := newObservations()
	pairs.pairLimit = 2
	if err := pairs.transaction(transaction("a", "b")); err != nil {
		t.Fatalf("transaction(a b) error = %v", err)
	}
	err := pairs.transaction(transaction("a", "c"))
	var identity *mining.PairIdentityLimitError
	if !errors.As(err, &identity) || identity.Limit != 2 || !errors.Is(err, mining.ErrPairIdentityLimit) {
		t.Fatalf("transaction(a c) error = %v, want PairIdentityLimitError", err)
	}
	if len(pairs.pairs) != 2 || pairs.pairObservations != 2 {
		t.Errorf("a refused transaction was recorded: %d pairs, %d observations", len(pairs.pairs), pairs.pairObservations)
	}
	if failure, ok := failureFor(err); !ok || failure.Code != "budget_unique_pairs" {
		t.Errorf("failureFor(pair limit) = %+v, %t", failure, ok)
	}

	observations := newObservations()
	observations.observationLimit = 5
	if err := observations.transaction(transaction("a", "b")); err != nil {
		t.Fatalf("transaction(a b) error = %v", err)
	}
	err = observations.transaction(transaction("a", "b", "c"))
	var observation *mining.PairObservationLimitError
	if !errors.As(err, &observation) || observation.Limit != 5 {
		t.Fatalf("transaction(a b c) error = %v, want PairObservationLimitError", err)
	}
	if failure, ok := failureFor(err); !ok || failure.Code != "budget_pair_observations" {
		t.Errorf("failureFor(observation limit) = %+v, %t", failure, ok)
	}

	frozen := newObservations()
	if frozen.observationLimit != 20_000_000 || frozen.pairLimit != 2_000_000 {
		t.Errorf("frozen limits = %d, %d", frozen.observationLimit, frozen.pairLimit)
	}
}
