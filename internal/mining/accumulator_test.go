package mining

import (
	"errors"
	"fmt"
	"slices"
	"testing"
)

func TestAccumulatorFoldsAllWeightModes(t *testing.T) {
	accumulator := NewAccumulator()
	for _, components := range [][]string{
		{"a"},
		{"a", "b"},
		{"a", "b", "c"},
	} {
		if err := accumulator.Observe(components); err != nil {
			t.Fatalf("Observe(%q) error = %v", components, err)
		}
	}

	oneSixth := 1.0 / 6.0
	want := Aggregate{
		EligibleTransactions: 3,
		PairObservations:     8,
		EligibleWeight: Weights{
			Unit:             3,
			InverseComponent: 2.5,
			PairNormalized:   1.5 + oneSixth,
		},
		Components: []Component{
			{
				ID:             0,
				Name:           "a",
				RawOpportunity: 3,
				Occurrence: Weights{
					Unit:             3,
					InverseComponent: 2.5,
					PairNormalized:   1.5 + oneSixth,
				},
			},
			{
				ID:             1,
				Name:           "b",
				RawOpportunity: 2,
				Occurrence: Weights{
					Unit:             2,
					InverseComponent: 1.5,
					PairNormalized:   0.5 + oneSixth,
				},
			},
			{
				ID:             2,
				Name:           "c",
				RawOpportunity: 1,
				Occurrence: Weights{
					Unit:             1,
					InverseComponent: 0.5,
					PairNormalized:   oneSixth,
				},
			},
		},
		Pairs: []Pair{
			{
				Antecedent: 0,
				Consequent: 1,
				RawSupport: 2,
				Support: Weights{
					Unit:             2,
					InverseComponent: 1.5,
					PairNormalized:   0.5 + oneSixth,
				},
			},
			{
				Antecedent: 0,
				Consequent: 2,
				RawSupport: 1,
				Support: Weights{
					Unit:             1,
					InverseComponent: 0.5,
					PairNormalized:   oneSixth,
				},
			},
			{
				Antecedent: 1,
				Consequent: 0,
				RawSupport: 2,
				Support: Weights{
					Unit:             2,
					InverseComponent: 1.5,
					PairNormalized:   0.5 + oneSixth,
				},
			},
			{
				Antecedent: 1,
				Consequent: 2,
				RawSupport: 1,
				Support: Weights{
					Unit:             1,
					InverseComponent: 0.5,
					PairNormalized:   oneSixth,
				},
			},
			{
				Antecedent: 2,
				Consequent: 0,
				RawSupport: 1,
				Support: Weights{
					Unit:             1,
					InverseComponent: 0.5,
					PairNormalized:   oneSixth,
				},
			},
			{
				Antecedent: 2,
				Consequent: 1,
				RawSupport: 1,
				Support: Weights{
					Unit:             1,
					InverseComponent: 0.5,
					PairNormalized:   oneSixth,
				},
			},
		},
	}

	assertAggregate(t, accumulator.Snapshot(), want)
}

func TestAccumulatorRejectsPairBudgetExhaustionAtomically(t *testing.T) {
	tests := []struct {
		name                 string
		pairObservationLimit int
		pairIdentityLimit    int
		wantErr              error
	}{
		{
			name:                 "observations",
			pairObservationLimit: 1,
			pairIdentityLimit:    10,
			wantErr:              ErrPairObservationLimit,
		},
		{
			name:                 "identities",
			pairObservationLimit: 10,
			pairIdentityLimit:    1,
			wantErr:              ErrPairIdentityLimit,
		},
		{
			name:                 "observation precedence",
			pairObservationLimit: 1,
			pairIdentityLimit:    1,
			wantErr:              ErrPairObservationLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accumulator := newAccumulator(limits{
				pairObservations: tt.pairObservationLimit,
				pairs:            tt.pairIdentityLimit,
			})

			err := accumulator.Observe([]string{"a", "b"})

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Observe() error = %v, want %v", err, tt.wantErr)
			}
			assertAggregate(t, accumulator.Snapshot(), Aggregate{})

			if errors.Is(tt.wantErr, ErrPairObservationLimit) {
				var limitErr *PairObservationLimitError
				if !errors.As(err, &limitErr) {
					t.Fatalf("Observe() error type = %T, want *PairObservationLimitError", err)
				}
				if limitErr.Observed != 2 || limitErr.Limit != tt.pairObservationLimit {
					t.Errorf("PairObservationLimitError = %+v, want observed 2 limit %d",
						limitErr, tt.pairObservationLimit)
				}
			} else {
				var limitErr *PairIdentityLimitError
				if !errors.As(err, &limitErr) {
					t.Fatalf("Observe() error type = %T, want *PairIdentityLimitError", err)
				}
				if limitErr.Observed != 2 || limitErr.Limit != tt.pairIdentityLimit {
					t.Errorf("PairIdentityLimitError = %+v, want observed 2 limit %d",
						limitErr, tt.pairIdentityLimit)
				}
			}
		})
	}
}

func TestAccumulatorPreservesStateWhenLaterTransactionExceedsPairLimit(t *testing.T) {
	accumulator := newAccumulator(limits{
		pairObservations: 10,
		pairs:            2,
	})
	if err := accumulator.Observe([]string{"a", "b"}); err != nil {
		t.Fatalf("first Observe() error = %v", err)
	}
	before := accumulator.Snapshot()

	err := accumulator.Observe([]string{"a", "c"})

	if !errors.Is(err, ErrPairIdentityLimit) {
		t.Fatalf("second Observe() error = %v, want %v", err, ErrPairIdentityLimit)
	}
	assertAggregate(t, accumulator.Snapshot(), before)
}

func TestAccumulatorAcceptsExactPairLimits(t *testing.T) {
	accumulator := newAccumulator(limits{
		pairObservations: 2,
		pairs:            2,
	})

	if err := accumulator.Observe([]string{"a", "b"}); err != nil {
		t.Fatalf("Observe() at exact limits error = %v", err)
	}

	got := accumulator.Snapshot()
	if got.PairObservations != 2 {
		t.Errorf("PairObservations = %d, want 2", got.PairObservations)
	}
	if len(got.Pairs) != 2 {
		t.Errorf("Pairs = %d, want 2", len(got.Pairs))
	}
}

func TestAccumulatorRejectsInvalidTransactionsAtomically(t *testing.T) {
	tests := []struct {
		name       string
		components []string
		wantErr    error
	}{
		{
			name:    "empty",
			wantErr: ErrEmptyTransaction,
		},
		{
			name:       "duplicate",
			components: []string{"a", "b", "a"},
			wantErr:    ErrDuplicateComponent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accumulator := NewAccumulator()

			err := accumulator.Observe(tt.components)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Observe() error = %v, want %v", err, tt.wantErr)
			}
			assertAggregate(t, accumulator.Snapshot(), Aggregate{})
		})
	}
}

func TestAccumulatorReportsFirstPairBudgetBreach(t *testing.T) {
	tests := []struct {
		name                 string
		pairObservationLimit int
		pairIdentityLimit    int
		second               []string
		wantErr              error
	}{
		{
			name:                 "observations",
			pairObservationLimit: 3,
			pairIdentityLimit:    10,
			second:               []string{"a", "b", "c"},
			wantErr:              ErrPairObservationLimit,
		},
		{
			name:                 "identities",
			pairObservationLimit: 10,
			pairIdentityLimit:    3,
			second:               []string{"a", "c"},
			wantErr:              ErrPairIdentityLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accumulator := newAccumulator(limits{
				pairObservations: tt.pairObservationLimit,
				pairs:            tt.pairIdentityLimit,
			})
			if err := accumulator.Observe([]string{"a", "b"}); err != nil {
				t.Fatalf("first Observe() error = %v", err)
			}
			before := accumulator.Snapshot()

			err := accumulator.Observe(tt.second)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("second Observe() error = %v, want %v", err, tt.wantErr)
			}
			if errors.Is(tt.wantErr, ErrPairObservationLimit) {
				var limitErr *PairObservationLimitError
				if !errors.As(err, &limitErr) {
					t.Fatalf("error type = %T, want *PairObservationLimitError", err)
				}
				if limitErr.Observed != 4 || limitErr.Limit != 3 {
					t.Errorf("PairObservationLimitError = %+v, want observed 4 limit 3",
						limitErr)
				}
			} else {
				var limitErr *PairIdentityLimitError
				if !errors.As(err, &limitErr) {
					t.Fatalf("error type = %T, want *PairIdentityLimitError", err)
				}
				if limitErr.Observed != 4 || limitErr.Limit != 3 {
					t.Errorf("PairIdentityLimitError = %+v, want observed 4 limit 3",
						limitErr)
				}
			}
			assertAggregate(t, accumulator.Snapshot(), before)
		})
	}
}

func TestAccumulatorSnapshotIsDetachedAndRepeatable(t *testing.T) {
	var accumulator Accumulator
	if err := accumulator.Observe([]string{"b", "a"}); err != nil {
		t.Fatalf("Observe() error = %v", err)
	}

	first := accumulator.Snapshot()
	second := accumulator.Snapshot()
	assertAggregate(t, second, first)

	first.Components[0].Name = "changed"
	first.Pairs[0].RawSupport = 99

	third := accumulator.Snapshot()
	if third.Components[0].Name != "b" {
		t.Errorf("detached component name = %q, want b", third.Components[0].Name)
	}
	if third.Pairs[0].RawSupport != 1 {
		t.Errorf("detached pair support = %d, want 1", third.Pairs[0].RawSupport)
	}

	if err := accumulator.Observe([]string{"b"}); err != nil {
		t.Fatalf("later Observe() error = %v", err)
	}
	if first.EligibleTransactions != 1 {
		t.Errorf("earlier snapshot transactions = %d, want 1", first.EligibleTransactions)
	}
}

func BenchmarkAccumulatorObserve(b *testing.B) {
	transactions := make([][]string, 1_000)
	for index := range transactions {
		components := make([]string, 10)
		for component := range components {
			components[component] = fmt.Sprintf("%02d", (index+component)%100)
		}
		transactions[index] = components
	}

	b.ReportAllocs()
	for b.Loop() {
		accumulator := NewAccumulator()
		for _, components := range transactions {
			if err := accumulator.Observe(components); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func assertAggregate(t *testing.T, got, want Aggregate) {
	t.Helper()

	if got.EligibleTransactions != want.EligibleTransactions {
		t.Errorf("EligibleTransactions = %d, want %d",
			got.EligibleTransactions, want.EligibleTransactions)
	}
	if got.PairObservations != want.PairObservations {
		t.Errorf("PairObservations = %d, want %d",
			got.PairObservations, want.PairObservations)
	}
	if got.EligibleWeight != want.EligibleWeight {
		t.Errorf("EligibleWeight = %+v, want %+v", got.EligibleWeight, want.EligibleWeight)
	}
	if !slices.Equal(got.Components, want.Components) {
		t.Errorf("Components = %+v, want %+v", got.Components, want.Components)
	}
	if !slices.Equal(got.Pairs, want.Pairs) {
		t.Errorf("Pairs = %+v, want %+v", got.Pairs, want.Pairs)
	}
}
