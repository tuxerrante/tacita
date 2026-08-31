package mining

import (
	"errors"
	"testing"
)

func TestRankCandidatesUsesFrozenPrecedence(t *testing.T) {
	base := Candidate{
		WeightedConfidence: 0.8,
		WeightedLift:       2,
		RawSupport:         8,
		RawOpportunity:     10,
		antecedentName:     "\x02",
		consequentName:     "\x02",
	}
	tests := []struct {
		name   string
		winner func(Candidate) Candidate
		loser  func(Candidate) Candidate
	}{
		{
			name: "confidence beats higher lift",
			winner: func(c Candidate) Candidate {
				c.WeightedConfidence, c.WeightedLift = 0.9, 1.5
				return c
			},
			loser: func(c Candidate) Candidate {
				c.WeightedConfidence, c.WeightedLift = 0.8, 10
				return c
			},
		},
		{
			name: "lift beats higher raw support",
			winner: func(c Candidate) Candidate {
				c.WeightedLift, c.RawSupport = 3, 3
				return c
			},
			loser: func(c Candidate) Candidate {
				c.WeightedLift, c.RawSupport = 2, 30
				return c
			},
		},
		{
			name: "raw support beats higher opportunity",
			winner: func(c Candidate) Candidate {
				c.RawSupport, c.RawOpportunity = 9, 5
				return c
			},
			loser: func(c Candidate) Candidate {
				c.RawSupport, c.RawOpportunity = 8, 20
				return c
			},
		},
		{
			name: "opportunity beats lower antecedent bytes",
			winner: func(c Candidate) Candidate {
				c.RawOpportunity, c.antecedentName = 20, "\xff"
				return c
			},
			loser: func(c Candidate) Candidate {
				c.RawOpportunity, c.antecedentName = 10, "\x01"
				return c
			},
		},
		{
			name: "antecedent bytes beat lower consequent bytes",
			winner: func(c Candidate) Candidate {
				c.antecedentName, c.consequentName = "\x01", "\xff"
				return c
			},
			loser: func(c Candidate) Candidate {
				c.antecedentName, c.consequentName = "\x02", "\x00"
				return c
			},
		},
		{
			name: "consequent bytes break final tie",
			winner: func(c Candidate) Candidate {
				c.consequentName = "\x01"
				return c
			},
			loser: func(c Candidate) Candidate {
				c.consequentName = "\xff"
				return c
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			winner := tt.winner(base)
			winner.Antecedent = 1
			loser := tt.loser(base)
			loser.Antecedent = 2
			candidates := []Candidate{loser, winner}

			rankCandidates(candidates)

			if candidates[0].Antecedent != 1 || candidates[1].Antecedent != 2 {
				t.Errorf("order = %+v, want designated winner first", candidates)
			}
			if candidates[0].Rank != 1 || candidates[1].Rank != 2 {
				t.Errorf("ranks = [%d %d], want [1 2]", candidates[0].Rank, candidates[1].Rank)
			}
			if candidates[0].antecedentName != "" || candidates[0].consequentName != "" {
				t.Errorf("private sort keys leaked into returned candidate")
			}
		})
	}
}

func TestDeriveCandidatesRanksUsingComponentNameBytes(t *testing.T) {
	aggregate := Aggregate{
		EligibleTransactions: 100,
		EligibleWeight:       Weights{Unit: 100},
		Components: []Component{
			{ID: 0, Name: "\xff", RawOpportunity: 20, Occurrence: Weights{Unit: 10}},
			{ID: 1, Name: "shared", Occurrence: Weights{Unit: 20}},
			{ID: 2, Name: "\x01", RawOpportunity: 20, Occurrence: Weights{Unit: 10}},
		},
		Pairs: []Pair{
			{Antecedent: 0, Consequent: 1, RawSupport: 5, Support: Weights{Unit: 8}},
			{Antecedent: 2, Consequent: 1, RawSupport: 5, Support: Weights{Unit: 8}},
		},
	}

	got, err := DeriveCandidates(aggregate, looseConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len(candidates) = %d, want 2", len(got))
	}
	if got[0].Antecedent != 2 || got[1].Antecedent != 0 {
		t.Errorf("order = %+v, want lower-byte antecedent first", got)
	}
}

func TestDeriveCandidatesRejectsAmbiguousCandidateIdentities(t *testing.T) {
	tests := []struct {
		name       string
		components []Component
		pairs      []Pair
		want       error
	}{
		{
			name: "duplicate component name",
			components: []Component{
				{ID: 0, Name: "same"}, {ID: 1, Name: "same"},
			},
			want: ErrDuplicateComponentName,
		},
		{
			name: "adjacent duplicate pair",
			components: []Component{
				{ID: 0, Name: "a"}, {ID: 1, Name: "b"},
			},
			pairs: []Pair{
				{Antecedent: 0, Consequent: 1},
				{Antecedent: 0, Consequent: 1},
			},
			want: ErrDuplicatePairIdentity,
		},
		{
			name: "out of order duplicate pair",
			components: []Component{
				{ID: 0, Name: "a"}, {ID: 1, Name: "b"},
			},
			pairs: []Pair{
				{Antecedent: 0, Consequent: 1},
				{Antecedent: 1, Consequent: 0},
				{Antecedent: 0, Consequent: 1},
			},
			want: ErrDuplicatePairIdentity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregate := Aggregate{
				EligibleTransactions: 100,
				EligibleWeight:       Weights{Unit: 100},
				Components:           tt.components,
				Pairs:                tt.pairs,
			}
			got, err := DeriveCandidates(aggregate, looseConfiguration())
			if got != nil {
				t.Errorf("candidates = %v, want nil", got)
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestDeriveCandidatesReportsEarlierUnknownReferenceBeforeLaterDuplicatePair(t *testing.T) {
	aggregate := Aggregate{
		EligibleTransactions: 100,
		EligibleWeight:       Weights{Unit: 100},
		Components: []Component{
			{ID: 0, Name: "a"}, {ID: 1, Name: "b"},
		},
		Pairs: []Pair{
			{Antecedent: 0, Consequent: 7},
			{Antecedent: 0, Consequent: 1},
			{Antecedent: 0, Consequent: 1},
		},
	}

	got, err := DeriveCandidates(aggregate, looseConfiguration())
	if got != nil {
		t.Errorf("candidates = %v, want nil", got)
	}
	if !errors.Is(err, ErrUnknownComponent) {
		t.Errorf("error = %v, want ErrUnknownComponent", err)
	}
}

func BenchmarkDeriveCandidates(b *testing.B) {
	accumulator := NewAccumulator()
	for index := 0; index < 1000; index++ {
		components := make([]string, 10)
		for component := range components {
			components[component] = string(rune('a' + (index+component)%26))
		}
		if err := accumulator.Observe(components); err != nil {
			b.Fatal(err)
		}
	}
	aggregate := accumulator.Snapshot()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := DeriveCandidates(aggregate, looseConfiguration()); err != nil {
			b.Fatal(err)
		}
	}
}
