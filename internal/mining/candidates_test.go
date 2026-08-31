package mining

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"
)

func twoComponentAggregate(
	eligibleTransactions uint64,
	eligibleWeight float64,
	rawOpportunity uint64,
	exposure float64,
	rawSupport uint64,
	support float64,
	consequentOccurrence float64,
) Aggregate {
	return Aggregate{
		EligibleTransactions: eligibleTransactions,
		EligibleWeight: Weights{
			Unit: eligibleWeight, InverseComponent: eligibleWeight, PairNormalized: eligibleWeight,
		},
		Components: []Component{
			{
				ID: 0, Name: "a", RawOpportunity: rawOpportunity,
				Occurrence: Weights{Unit: exposure, InverseComponent: exposure, PairNormalized: exposure},
			},
			{
				ID: 1, Name: "b",
				Occurrence: Weights{
					Unit:             consequentOccurrence,
					InverseComponent: consequentOccurrence,
					PairNormalized:   consequentOccurrence,
				},
			},
		},
		Pairs: []Pair{{
			Antecedent: 0, Consequent: 1, RawSupport: rawSupport,
			Support: Weights{Unit: support, InverseComponent: support, PairNormalized: support},
		}},
	}
}

func looseConfiguration() Configuration {
	return Configuration{
		SizeWeight: WeightUnit, MinimumOpportunities: 5,
		MinimumConfidence: 0.70, MinimumLift: 1.25,
	}
}

func TestDeriveCandidatesEvaluationBoundaries(t *testing.T) {
	tests := []struct {
		name                 string
		eligibleTransactions uint64
		rawSupport           uint64
		wantCandidates       int
		wantNil              bool
	}{
		{"below transaction floor abstains", 99, 10, 0, true},
		{"transaction floor evaluates", 100, 10, 1, false},
		{"evaluated empty result is non-nil", 100, 2, 0, false},
		{"raw support floor is inclusive", 100, 3, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregate := twoComponentAggregate(
				tt.eligibleTransactions, 100, 20, 100, tt.rawSupport, 90, 10,
			)
			got, err := DeriveCandidates(aggregate, looseConfiguration())
			if err != nil {
				t.Fatalf("DeriveCandidates() error = %v", err)
			}
			if len(got) != tt.wantCandidates {
				t.Errorf("len(candidates) = %d, want %d", len(got), tt.wantCandidates)
			}
			if (got == nil) != tt.wantNil {
				t.Errorf("candidates == nil = %v, want %v", got == nil, tt.wantNil)
			}
		})
	}
}

func TestDeriveCandidatesAppliesConfigurationThresholds(t *testing.T) {
	tests := []struct {
		name                 string
		configuration        Configuration
		rawOpportunity       uint64
		exposure             float64
		support              float64
		consequentOccurrence float64
		eligibleWeight       float64
		wantCandidates       int
	}{
		{
			name: "opportunities below threshold",
			configuration: Configuration{
				SizeWeight: WeightUnit, MinimumOpportunities: 10,
				MinimumConfidence: 0.70, MinimumLift: 1.25,
			},
			rawOpportunity: 9, exposure: 100, support: 95,
			consequentOccurrence: 10, eligibleWeight: 100,
		},
		{
			name: "opportunities at threshold",
			configuration: Configuration{
				SizeWeight: WeightUnit, MinimumOpportunities: 10,
				MinimumConfidence: 0.70, MinimumLift: 1.25,
			},
			rawOpportunity: 10, exposure: 100, support: 95,
			consequentOccurrence: 10, eligibleWeight: 100, wantCandidates: 1,
		},
		{
			name: "confidence below threshold",
			configuration: Configuration{
				SizeWeight: WeightUnit, MinimumOpportunities: 5,
				MinimumConfidence: 0.80, MinimumLift: 1.25,
			},
			rawOpportunity: 20, exposure: 100, support: 79,
			consequentOccurrence: 50, eligibleWeight: 100,
		},
		{
			name: "confidence at threshold",
			configuration: Configuration{
				SizeWeight: WeightUnit, MinimumOpportunities: 5,
				MinimumConfidence: 0.80, MinimumLift: 1.25,
			},
			rawOpportunity: 20, exposure: 100, support: 80,
			consequentOccurrence: 50, eligibleWeight: 100, wantCandidates: 1,
		},
		{
			name:           "lift below threshold",
			configuration:  looseConfiguration(),
			rawOpportunity: 20, exposure: 64, support: 45,
			consequentOccurrence: 10, eligibleWeight: 16,
		},
		{
			name:           "lift at threshold",
			configuration:  looseConfiguration(),
			rawOpportunity: 20, exposure: 64, support: 45,
			consequentOccurrence: 9, eligibleWeight: 16, wantCandidates: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregate := twoComponentAggregate(
				100, tt.eligibleWeight, tt.rawOpportunity, tt.exposure,
				10, tt.support, tt.consequentOccurrence,
			)
			got, err := DeriveCandidates(aggregate, tt.configuration)
			if err != nil {
				t.Fatalf("DeriveCandidates() error = %v", err)
			}
			if len(got) != tt.wantCandidates {
				t.Errorf("len(candidates) = %d, want %d", len(got), tt.wantCandidates)
			}
		})
	}
}

func TestDeriveCandidatesUsesEachWeightModeSource(t *testing.T) {
	aggregate := Aggregate{
		EligibleTransactions: 100,
		EligibleWeight:       Weights{Unit: 48, InverseComponent: 40, PairNormalized: 12},
		Components: []Component{
			{
				ID: 0, Name: "a", RawOpportunity: 42,
				Occurrence: Weights{Unit: 64, InverseComponent: 128, PairNormalized: 32},
			},
			{
				ID: 1, Name: "b",
				Occurrence: Weights{Unit: 6, InverseComponent: 20, PairNormalized: 3},
			},
		},
		Pairs: []Pair{{
			Antecedent: 0, Consequent: 1, RawSupport: 77,
			Support: Weights{Unit: 56, InverseComponent: 96, PairNormalized: 30},
		}},
	}

	tests := []struct {
		mode                                            WeightMode
		support, exposure, confidence, prevalence, lift float64
	}{
		{WeightUnit, 56, 64, 0.875, 0.125, 7},
		{WeightInverseComponent, 96, 128, 0.75, 0.5, 1.5},
		{WeightPairNormalized, 30, 32, 0.9375, 0.25, 3.75},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			configuration := Configuration{
				SizeWeight: tt.mode, MinimumOpportunities: 5,
				MinimumConfidence: 0.70, MinimumLift: 1.25,
			}
			got, err := DeriveCandidates(aggregate, configuration)
			if err != nil {
				t.Fatalf("DeriveCandidates() error = %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("len(candidates) = %d, want 1", len(got))
			}
			want := Candidate{
				Antecedent: 0, Consequent: 1, RawSupport: 77, RawOpportunity: 42,
				WeightedSupport: tt.support, WeightedExposure: tt.exposure,
				WeightedConfidence: tt.confidence, WeightedPrevalence: tt.prevalence,
				WeightedLift: tt.lift,
			}
			if got[0] != want {
				t.Errorf("candidate = %+v, want %+v", got[0], want)
			}
		})
	}
}

func TestDeriveCandidatesExcludesInvalidWeightedValues(t *testing.T) {
	type field int
	const (
		supportField field = iota
		exposureField
		consequentField
		eligibleField
	)
	values := []struct {
		name  string
		value float64
	}{
		{"zero", 0},
		{"negative", -1},
		{"NaN", math.NaN()},
		{"positive infinity", math.Inf(1)},
	}

	for _, target := range []field{supportField, exposureField, consequentField, eligibleField} {
		for _, value := range values {
			t.Run(fmt.Sprintf("field-%d/%s", target, value.name), func(t *testing.T) {
				support, exposure, consequent, eligible := 90.0, 100.0, 10.0, 100.0
				switch target {
				case supportField:
					support = value.value
				case exposureField:
					exposure = value.value
				case consequentField:
					consequent = value.value
				case eligibleField:
					eligible = value.value
				}
				aggregate := twoComponentAggregate(100, eligible, 20, exposure, 10, support, consequent)
				got, err := DeriveCandidates(aggregate, looseConfiguration())
				if err != nil {
					t.Fatalf("DeriveCandidates() error = %v", err)
				}
				if len(got) != 0 {
					t.Errorf("len(candidates) = %d, want 0", len(got))
				}
			})
		}
	}
}

func TestDeriveCandidatesRejectsMalformedComponentReferences(t *testing.T) {
	tests := []struct {
		name       string
		components []Component
		pair       Pair
		want       error
	}{
		{
			name: "unknown component even below raw support floor",
			components: []Component{
				{ID: 0, Name: "a", RawOpportunity: 20, Occurrence: Weights{Unit: 100}},
			},
			pair: Pair{Antecedent: 0, Consequent: 7, RawSupport: 1, Support: Weights{Unit: 90}},
			want: ErrUnknownComponent,
		},
		{
			name: "duplicate component identity",
			components: []Component{
				{ID: 0, Name: "a", RawOpportunity: 20, Occurrence: Weights{Unit: 100}},
				{ID: 0, Name: "b", RawOpportunity: 20, Occurrence: Weights{Unit: 100}},
			},
			pair: Pair{Antecedent: 0, Consequent: 0, RawSupport: 10, Support: Weights{Unit: 90}},
			want: ErrDuplicateComponentIdentity,
		},
		{
			name: "self pair",
			components: []Component{
				{ID: 3, Name: "a", RawOpportunity: 20, Occurrence: Weights{Unit: 100}},
			},
			pair: Pair{Antecedent: 3, Consequent: 3, RawSupport: 1, Support: Weights{Unit: 1}},
			want: ErrSelfPair,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregate := Aggregate{
				EligibleTransactions: 100, EligibleWeight: Weights{Unit: 100},
				Components: tt.components, Pairs: []Pair{tt.pair},
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

func TestDeriveCandidatesReturnsAllDetachedAndDeterministic(t *testing.T) {
	const pairCount = 15
	components := make([]Component, 0, pairCount*2)
	pairs := make([]Pair, 0, pairCount)
	for index := 0; index < pairCount; index++ {
		antecedent := ComponentID(index * 2)
		consequent := antecedent + 1
		components = append(components,
			Component{
				ID: antecedent, Name: fmt.Sprintf("a%d", index), RawOpportunity: 20,
				Occurrence: Weights{Unit: 100},
			},
			Component{
				ID: consequent, Name: fmt.Sprintf("b%d", index),
				Occurrence: Weights{Unit: 20},
			},
		)
		pairs = append(pairs, Pair{
			Antecedent: antecedent, Consequent: consequent,
			RawSupport: 10, Support: Weights{Unit: 90},
		})
	}
	aggregate := Aggregate{
		EligibleTransactions: 100, EligibleWeight: Weights{Unit: 100},
		Components: components, Pairs: pairs,
	}
	pairsBefore := slices.Clone(aggregate.Pairs)
	componentsBefore := slices.Clone(aggregate.Components)

	first, err := DeriveCandidates(aggregate, looseConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	second, err := DeriveCandidates(aggregate, looseConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != pairCount {
		t.Fatalf("len(candidates) = %d, want %d", len(first), pairCount)
	}
	if !slices.Equal(first, second) {
		t.Errorf("repeated derivations differ")
	}
	if !slices.Equal(aggregate.Pairs, pairsBefore) ||
		!slices.Equal(aggregate.Components, componentsBefore) {
		t.Errorf("DeriveCandidates() mutated aggregate")
	}

	first[0].RawSupport = 0
	third, err := DeriveCandidates(aggregate, looseConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if third[0].RawSupport == 0 {
		t.Errorf("result is not detached")
	}
}
