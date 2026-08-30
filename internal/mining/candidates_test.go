package mining

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"
)

// twoComponentAggregate builds a minimal Aggregate with exactly one directional
// pair A -> B, using the same numeric value for all three weight-mode fields
// unless a test needs to isolate one mode.
func twoComponentAggregate(
	eligibleTransactions uint64,
	eligibleWeight float64,
	antecedentName, consequentName string,
	rawOpportunity uint64,
	exposure float64,
	rawSupport uint64,
	support float64,
	consequentOccurrence float64,
) Aggregate {
	return Aggregate{
		EligibleTransactions: eligibleTransactions,
		EligibleWeight: Weights{
			Unit:             eligibleWeight,
			InverseComponent: eligibleWeight,
			PairNormalized:   eligibleWeight,
		},
		Components: []Component{
			{
				ID:             0,
				Name:           antecedentName,
				RawOpportunity: rawOpportunity,
				Occurrence: Weights{
					Unit:             exposure,
					InverseComponent: exposure,
					PairNormalized:   exposure,
				},
			},
			{
				ID:             1,
				Name:           consequentName,
				RawOpportunity: rawOpportunity,
				Occurrence: Weights{
					Unit:             consequentOccurrence,
					InverseComponent: consequentOccurrence,
					PairNormalized:   consequentOccurrence,
				},
			},
		},
		Pairs: []Pair{
			{
				Antecedent: 0,
				Consequent: 1,
				RawSupport: rawSupport,
				Support: Weights{
					Unit:             support,
					InverseComponent: support,
					PairNormalized:   support,
				},
			},
		},
	}
}

// looseConfiguration is the least restrictive frozen grid point: it isolates
// whichever single dimension a test wants to exercise.
func looseConfiguration() Configuration {
	return Configuration{
		SizeWeight:           WeightUnit,
		MinimumOpportunities: 5,
		MinimumConfidence:    0.70,
		MinimumLift:          1.25,
	}
}

func TestConfigurationIDFormat(t *testing.T) {
	got := Configuration{
		SizeWeight:           WeightInverseComponent,
		MinimumOpportunities: 10,
		MinimumConfidence:    0.80,
		MinimumLift:          1.50,
	}.ID()

	want := "weight=inverse-component;opportunities=10;confidence=0.80;lift=1.50"
	if got != want {
		t.Errorf("ID() = %q, want %q", got, want)
	}
}

func TestAllConfigurationsGrid(t *testing.T) {
	weights := []WeightMode{WeightUnit, WeightInverseComponent, WeightPairNormalized}
	opportunities := []uint64{5, 10, 20}
	confidences := []float64{0.70, 0.80, 0.90}
	lifts := []float64{1.25, 1.50, 2.00}

	wantIDs := make(map[string]bool)
	for _, weight := range weights {
		for _, opportunity := range opportunities {
			for _, confidence := range confidences {
				for _, lift := range lifts {
					id := Configuration{
						SizeWeight:           weight,
						MinimumOpportunities: opportunity,
						MinimumConfidence:    confidence,
						MinimumLift:          lift,
					}.ID()
					wantIDs[id] = true
				}
			}
		}
	}
	if len(wantIDs) != 81 {
		t.Fatalf("independently generated grid has %d distinct IDs, want 81", len(wantIDs))
	}

	got := AllConfigurations()
	if len(got) != 81 {
		t.Fatalf("AllConfigurations() returned %d configurations, want 81", len(got))
	}

	gotIDs := make(map[string]bool, len(got))
	for _, configuration := range got {
		id := configuration.ID()
		if gotIDs[id] {
			t.Errorf("AllConfigurations() repeated ID %q", id)
		}
		gotIDs[id] = true
		if !wantIDs[id] {
			t.Errorf("AllConfigurations() produced unexpected ID %q", id)
		}
		if err := configuration.validate(); err != nil {
			t.Errorf("configuration %q failed self-validation: %v", id, err)
		}
	}
	for id := range wantIDs {
		if !gotIDs[id] {
			t.Errorf("AllConfigurations() is missing expected ID %q", id)
		}
	}

	if !slices.IsSortedFunc(got, func(left, right Configuration) int {
		if left.ID() < right.ID() {
			return -1
		}
		if left.ID() > right.ID() {
			return 1
		}
		return 0
	}) {
		t.Errorf("AllConfigurations() is not sorted by ascending canonical ID")
	}
}

func TestAllConfigurationsReturnsDetachedSlices(t *testing.T) {
	first := AllConfigurations()
	first[0].MinimumOpportunities = 999

	second := AllConfigurations()
	if second[0].MinimumOpportunities == 999 {
		t.Errorf("AllConfigurations() shares backing state across calls")
	}
}

func TestConfigurationValidateRejectsOutsideGrid(t *testing.T) {
	tests := []struct {
		name          string
		configuration Configuration
		wantField     string
	}{
		{
			name: "unknown weight mode",
			configuration: Configuration{
				SizeWeight: "bogus", MinimumOpportunities: 5,
				MinimumConfidence: 0.70, MinimumLift: 1.25,
			},
			wantField: "SizeWeight",
		},
		{
			name: "opportunities outside grid",
			configuration: Configuration{
				SizeWeight: WeightUnit, MinimumOpportunities: 7,
				MinimumConfidence: 0.70, MinimumLift: 1.25,
			},
			wantField: "MinimumOpportunities",
		},
		{
			name: "confidence outside grid",
			configuration: Configuration{
				SizeWeight: WeightUnit, MinimumOpportunities: 5,
				MinimumConfidence: 0.75, MinimumLift: 1.25,
			},
			wantField: "MinimumConfidence",
		},
		{
			name: "lift outside grid",
			configuration: Configuration{
				SizeWeight: WeightUnit, MinimumOpportunities: 5,
				MinimumConfidence: 0.70, MinimumLift: 1.0,
			},
			wantField: "MinimumLift",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregate := twoComponentAggregate(100, 100, "a", "b", 20, 100, 10, 90, 10)

			candidates, err := DeriveCandidates(aggregate, tt.configuration)

			if candidates != nil {
				t.Errorf("DeriveCandidates() candidates = %v, want nil", candidates)
			}
			if !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf("DeriveCandidates() error = %v, want ErrInvalidConfiguration", err)
			}
			var configErr *ConfigurationError
			if !errors.As(err, &configErr) {
				t.Fatalf("error type = %T, want *ConfigurationError", err)
			}
			if configErr.Field != tt.wantField {
				t.Errorf("ConfigurationError.Field = %q, want %q", configErr.Field, tt.wantField)
			}
		})
	}
}

func TestDeriveCandidatesAbstainsBelowEligibleTransactionFloor(t *testing.T) {
	// 99 is abstention: DeriveCandidates returns the literal nil slice, not
	// merely a zero-length one, so callers can distinguish "abstained" from
	// "evaluated, nothing eligible" (see
	// TestDeriveCandidatesEvaluatedZeroEligibleCandidatesReturnsNonNilEmptySlice)
	// with a nil check alone.
	tests := []struct {
		name                 string
		eligibleTransactions uint64
		wantCandidates       int
		wantNil              bool
	}{
		{name: "99 is below the floor", eligibleTransactions: 99, wantCandidates: 0, wantNil: true},
		{name: "100 meets the floor", eligibleTransactions: 100, wantCandidates: 1, wantNil: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregate := twoComponentAggregate(
				tt.eligibleTransactions, 100, "a", "b", 20, 100, 10, 90, 10,
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

func TestDeriveCandidatesEvaluatedZeroEligibleCandidatesReturnsNonNilEmptySlice(t *testing.T) {
	// The aggregate meets the eligible-transaction floor (it is evaluated,
	// not abstained), but its only pair fails the raw-support floor, so zero
	// candidates qualify. The result must still be a non-nil, zero-length
	// slice: nil is reserved exclusively for abstention below 100 eligible
	// transactions.
	aggregate := twoComponentAggregate(100, 100, "a", "b", 20, 100, 2, 90, 10)

	got, err := DeriveCandidates(aggregate, looseConfiguration())
	if err != nil {
		t.Fatalf("DeriveCandidates() error = %v", err)
	}
	if got == nil {
		t.Fatalf("candidates = nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("len(candidates) = %d, want 0", len(got))
	}
}

func TestDeriveCandidatesRequiresMinimumRawSupport(t *testing.T) {
	tests := []struct {
		name           string
		rawSupport     uint64
		wantCandidates int
	}{
		{name: "2 is below the floor", rawSupport: 2, wantCandidates: 0},
		{name: "3 meets the floor", rawSupport: 3, wantCandidates: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregate := twoComponentAggregate(
				100, 100, "a", "b", 20, 100, tt.rawSupport, 90, 10,
			)

			got, err := DeriveCandidates(aggregate, looseConfiguration())
			if err != nil {
				t.Fatalf("DeriveCandidates() error = %v", err)
			}
			if len(got) != tt.wantCandidates {
				t.Errorf("len(candidates) = %d, want %d", len(got), tt.wantCandidates)
			}
		})
	}
}

func TestDeriveCandidatesAppliesOpportunityThreshold(t *testing.T) {
	tests := []struct {
		name           string
		rawOpportunity uint64
		wantCandidates int
	}{
		{name: "9 is below minimum of 10", rawOpportunity: 9, wantCandidates: 0},
		{name: "10 meets minimum of 10", rawOpportunity: 10, wantCandidates: 1},
	}

	configuration := Configuration{
		SizeWeight: WeightUnit, MinimumOpportunities: 10,
		MinimumConfidence: 0.70, MinimumLift: 1.25,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregate := twoComponentAggregate(
				100, 100, "a", "b", tt.rawOpportunity, 100, 10, 95, 10,
			)

			got, err := DeriveCandidates(aggregate, configuration)
			if err != nil {
				t.Fatalf("DeriveCandidates() error = %v", err)
			}
			if len(got) != tt.wantCandidates {
				t.Errorf("len(candidates) = %d, want %d", len(got), tt.wantCandidates)
			}
		})
	}
}

func TestDeriveCandidatesAppliesConfidenceThreshold(t *testing.T) {
	tests := []struct {
		name           string
		support        float64
		wantCandidates int
	}{
		{name: "0.79 is below minimum of 0.80", support: 79, wantCandidates: 0},
		{name: "0.80 meets minimum of 0.80", support: 80, wantCandidates: 1},
	}

	configuration := Configuration{
		SizeWeight: WeightUnit, MinimumOpportunities: 5,
		MinimumConfidence: 0.80, MinimumLift: 1.25,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// exposure = 100, so support directly encodes confidence * 100.
			aggregate := twoComponentAggregate(
				100, 100, "a", "b", 20, 100, 10, tt.support, 50,
			)

			got, err := DeriveCandidates(aggregate, configuration)
			if err != nil {
				t.Fatalf("DeriveCandidates() error = %v", err)
			}
			if len(got) != tt.wantCandidates {
				t.Errorf("len(candidates) = %d, want %d", len(got), tt.wantCandidates)
			}
		})
	}
}

func TestDeriveCandidatesAppliesLiftThreshold(t *testing.T) {
	// exposure=64, support=45 gives confidence=45/64=0.703125, exactly
	// representable in binary floating point and comfortably above the
	// loosest 0.70 grid confidence floor.
	const exposure = 64
	const support = 45

	tests := []struct {
		name                 string
		consequentOccurrence float64
		eligibleWeight       float64
		wantCandidates       int
	}{
		// occurrence=10, eligibleWeight=16 => prevalence=0.625,
		// lift=0.703125/0.625=1.125, exactly representable and below 1.25.
		{name: "1.125 is below minimum of 1.25", consequentOccurrence: 10, eligibleWeight: 16, wantCandidates: 0},
		// occurrence=9, eligibleWeight=16 => prevalence=0.5625,
		// lift=0.703125/0.5625=1.25 exactly, at the minimum.
		{name: "1.25 meets minimum of 1.25", consequentOccurrence: 9, eligibleWeight: 16, wantCandidates: 1},
	}

	configuration := Configuration{
		SizeWeight: WeightUnit, MinimumOpportunities: 5,
		MinimumConfidence: 0.70, MinimumLift: 1.25,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregate := twoComponentAggregate(
				100, tt.eligibleWeight, "a", "b", 20, exposure, 10, support,
				tt.consequentOccurrence,
			)

			got, err := DeriveCandidates(aggregate, configuration)
			if err != nil {
				t.Fatalf("DeriveCandidates() error = %v", err)
			}
			if len(got) != tt.wantCandidates {
				t.Errorf("len(candidates) = %d, want %d", len(got), tt.wantCandidates)
			}
		})
	}
}

func TestDeriveCandidatesDerivesFromFourIndependentWeightModeSources(t *testing.T) {
	// Every one of the four size-weighted state sources — Pair.Support,
	// antecedent Occurrence (exposure), consequent Occurrence (prevalence
	// numerator), and Aggregate.EligibleWeight — is given a distinct value per
	// weight mode, with no value repeated across any source or mode. That
	// makes every arithmetic result unique per mode: selecting the wrong
	// mode's value for even one source would produce a WeightedConfidence,
	// WeightedPrevalence, or WeightedLift that fails the exact-value
	// assertions below, so passing proves forMode is applied consistently to
	// all four sources and no other.
	aggregate := Aggregate{
		EligibleTransactions: 100,
		EligibleWeight: Weights{
			Unit:             48,
			InverseComponent: 40,
			PairNormalized:   12,
		},
		Components: []Component{
			{
				ID: 0, Name: "antecedent", RawOpportunity: 42,
				Occurrence: Weights{Unit: 64, InverseComponent: 128, PairNormalized: 32},
			},
			{
				// RawOpportunity is deliberately different from the
				// antecedent's and must never surface in a derived Candidate.
				ID: 1, Name: "consequent", RawOpportunity: 999,
				Occurrence: Weights{Unit: 6, InverseComponent: 20, PairNormalized: 3},
			},
		},
		Pairs: []Pair{
			{
				Antecedent: 0, Consequent: 1, RawSupport: 77,
				Support: Weights{Unit: 56, InverseComponent: 96, PairNormalized: 30},
			},
		},
	}

	tests := []struct {
		name           string
		mode           WeightMode
		wantSupport    float64
		wantExposure   float64
		wantConfidence float64
		wantPrevalence float64
		wantLift       float64
	}{
		// unit: confidence=56/64=0.875, prevalence=6/48=0.125, lift=7.0.
		{
			name: "unit", mode: WeightUnit,
			wantSupport: 56, wantExposure: 64,
			wantConfidence: 0.875, wantPrevalence: 0.125, wantLift: 7.0,
		},
		// inverse-component: confidence=96/128=0.75, prevalence=20/40=0.5,
		// lift=1.5.
		{
			name: "inverse-component", mode: WeightInverseComponent,
			wantSupport: 96, wantExposure: 128,
			wantConfidence: 0.75, wantPrevalence: 0.5, wantLift: 1.5,
		},
		// pair-normalized: confidence=30/32=0.9375, prevalence=3/12=0.25,
		// lift=3.75.
		{
			name: "pair-normalized", mode: WeightPairNormalized,
			wantSupport: 30, wantExposure: 32,
			wantConfidence: 0.9375, wantPrevalence: 0.25, wantLift: 3.75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				Antecedent:         0,
				Consequent:         1,
				RawSupport:         77,
				RawOpportunity:     42, // from the antecedent, never the consequent's 999
				WeightedSupport:    tt.wantSupport,
				WeightedExposure:   tt.wantExposure,
				WeightedConfidence: tt.wantConfidence,
				WeightedPrevalence: tt.wantPrevalence,
				WeightedLift:       tt.wantLift,
				Rank:               1,
			}
			if got[0] != want {
				t.Errorf("candidate = %+v, want %+v", got[0], want)
			}
		})
	}
}

func TestDeriveCandidatesExcludesMalformedAggregateValues(t *testing.T) {
	// baseline is comfortably eligible: exposure=100/support=90 confidence=0.9,
	// consequentOccurrence=10/eligibleWeight=100 prevalence=0.1, lift=9.0.
	type overrides struct {
		support              *float64
		exposure             *float64
		consequentOccurrence *float64
		eligibleWeight       *float64
	}
	f := func(v float64) *float64 { return &v }

	tests := []struct {
		name string
		overrides
	}{
		{name: "zero support", overrides: overrides{support: f(0)}},
		{name: "negative support", overrides: overrides{support: f(-90)}},
		{name: "NaN support", overrides: overrides{support: f(math.NaN())}},
		{name: "Inf support", overrides: overrides{support: f(math.Inf(1))}},
		{name: "zero exposure", overrides: overrides{exposure: f(0)}},
		{name: "negative exposure", overrides: overrides{exposure: f(-100)}},
		{name: "NaN exposure", overrides: overrides{exposure: f(math.NaN())}},
		{name: "Inf exposure", overrides: overrides{exposure: f(math.Inf(1))}},
		{
			name:      "negative support and negative exposure yield positive ratio",
			overrides: overrides{support: f(-90), exposure: f(-100)},
		},
		{name: "zero consequent occurrence", overrides: overrides{consequentOccurrence: f(0)}},
		{name: "negative consequent occurrence", overrides: overrides{consequentOccurrence: f(-10)}},
		{name: "NaN consequent occurrence", overrides: overrides{consequentOccurrence: f(math.NaN())}},
		{name: "Inf consequent occurrence", overrides: overrides{consequentOccurrence: f(math.Inf(1))}},
		{name: "zero eligible weight", overrides: overrides{eligibleWeight: f(0)}},
		{name: "negative eligible weight", overrides: overrides{eligibleWeight: f(-100)}},
		{name: "NaN eligible weight", overrides: overrides{eligibleWeight: f(math.NaN())}},
		{name: "Inf eligible weight", overrides: overrides{eligibleWeight: f(math.Inf(1))}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			support, exposure := 90.0, 100.0
			consequentOccurrence, eligibleWeight := 10.0, 100.0
			if tt.support != nil {
				support = *tt.support
			}
			if tt.exposure != nil {
				exposure = *tt.exposure
			}
			if tt.consequentOccurrence != nil {
				consequentOccurrence = *tt.consequentOccurrence
			}
			if tt.eligibleWeight != nil {
				eligibleWeight = *tt.eligibleWeight
			}

			aggregate := twoComponentAggregate(
				100, eligibleWeight, "a", "b", 20, exposure, 10, support,
				consequentOccurrence,
			)

			got, err := DeriveCandidates(aggregate, looseConfiguration())
			if err != nil {
				t.Fatalf("DeriveCandidates() error = %v, want nil (ineligible, not an error)", err)
			}
			if len(got) != 0 {
				t.Errorf("len(candidates) = %d, want 0", len(got))
			}
		})
	}
}

func TestDeriveCandidatesRejectsUnknownComponentReference(t *testing.T) {
	aggregate := Aggregate{
		EligibleTransactions: 100,
		EligibleWeight:       Weights{Unit: 100, InverseComponent: 100, PairNormalized: 100},
		Components: []Component{
			{ID: 0, Name: "a", RawOpportunity: 20, Occurrence: Weights{Unit: 100}},
		},
		Pairs: []Pair{
			// Consequent 7 is never declared in Components.
			{Antecedent: 0, Consequent: 7, RawSupport: 10, Support: Weights{Unit: 90}},
		},
	}

	got, err := DeriveCandidates(aggregate, looseConfiguration())

	if got != nil {
		t.Errorf("candidates = %v, want nil", got)
	}
	if !errors.Is(err, ErrUnknownComponent) {
		t.Fatalf("error = %v, want ErrUnknownComponent", err)
	}
	var unknownErr *UnknownComponentError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("error type = %T, want *UnknownComponentError", err)
	}
	if unknownErr.ComponentID != 7 {
		t.Errorf("UnknownComponentError.ComponentID = %d, want 7", unknownErr.ComponentID)
	}
}

func TestDeriveCandidatesRejectsUnknownComponentReferenceBelowRawSupportFloor(t *testing.T) {
	// The malformed pair's RawSupport (1) is below minRawSupport (3), the
	// filter that would otherwise skip this pair before it is ever resolved.
	// Reference resolution must happen unconditionally for every pair, so
	// this unknown reference is caught exactly like any other, not silently
	// dropped by the raw-support filter.
	aggregate := Aggregate{
		EligibleTransactions: 100,
		EligibleWeight:       Weights{Unit: 100, InverseComponent: 100, PairNormalized: 100},
		Components: []Component{
			{ID: 0, Name: "a", RawOpportunity: 20, Occurrence: Weights{Unit: 100}},
		},
		Pairs: []Pair{
			// Consequent 7 is never declared in Components, and RawSupport is
			// below the floor.
			{Antecedent: 0, Consequent: 7, RawSupport: 1, Support: Weights{Unit: 90}},
		},
	}

	got, err := DeriveCandidates(aggregate, looseConfiguration())

	if got != nil {
		t.Errorf("candidates = %v, want nil", got)
	}
	if !errors.Is(err, ErrUnknownComponent) {
		t.Fatalf("error = %v, want ErrUnknownComponent", err)
	}
	var unknownErr *UnknownComponentError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("error type = %T, want *UnknownComponentError", err)
	}
	if unknownErr.ComponentID != 7 {
		t.Errorf("UnknownComponentError.ComponentID = %d, want 7", unknownErr.ComponentID)
	}
}

func TestDeriveCandidatesRejectsDuplicateComponentIdentity(t *testing.T) {
	aggregate := Aggregate{
		EligibleTransactions: 100,
		EligibleWeight:       Weights{Unit: 100, InverseComponent: 100, PairNormalized: 100},
		Components: []Component{
			{ID: 0, Name: "a", RawOpportunity: 20, Occurrence: Weights{Unit: 100}},
			{ID: 0, Name: "duplicate", RawOpportunity: 20, Occurrence: Weights{Unit: 100}},
		},
		Pairs: []Pair{
			{Antecedent: 0, Consequent: 0, RawSupport: 10, Support: Weights{Unit: 90}},
		},
	}

	got, err := DeriveCandidates(aggregate, looseConfiguration())

	if got != nil {
		t.Errorf("candidates = %v, want nil", got)
	}
	if !errors.Is(err, ErrDuplicateComponentIdentity) {
		t.Fatalf("error = %v, want ErrDuplicateComponentIdentity", err)
	}
	var duplicateErr *DuplicateComponentIdentityError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("error type = %T, want *DuplicateComponentIdentityError", err)
	}
	if duplicateErr.ComponentID != 0 {
		t.Errorf("DuplicateComponentIdentityError.ComponentID = %d, want 0", duplicateErr.ComponentID)
	}
}

func TestDeriveCandidatesRejectsDuplicateComponentName(t *testing.T) {
	// Two distinct ComponentID values repeat the byte-identical Name "a".
	// Production Snapshot state interns components by name and can never
	// produce this, so a hand-built Aggregate that does is rejected as
	// malformed: the frozen byte-level name tie-breakers assume distinct
	// component names, and silently accepting a duplicate would make ranking
	// among candidates that reach the name tie-breaker order-dependent on
	// map iteration rather than deterministic.
	aggregate := Aggregate{
		EligibleTransactions: 100,
		EligibleWeight:       Weights{Unit: 100, InverseComponent: 100, PairNormalized: 100},
		Components: []Component{
			{ID: 0, Name: "a", RawOpportunity: 20, Occurrence: Weights{Unit: 100}},
			{ID: 1, Name: "a", RawOpportunity: 20, Occurrence: Weights{Unit: 100}},
		},
		Pairs: []Pair{
			{Antecedent: 0, Consequent: 1, RawSupport: 10, Support: Weights{Unit: 90}},
		},
	}

	got, err := DeriveCandidates(aggregate, looseConfiguration())

	if got != nil {
		t.Errorf("candidates = %v, want nil", got)
	}
	if !errors.Is(err, ErrDuplicateComponentName) {
		t.Fatalf("error = %v, want ErrDuplicateComponentName", err)
	}
	var duplicateErr *DuplicateComponentNameError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("error type = %T, want *DuplicateComponentNameError", err)
	}
	if duplicateErr.Name != "a" {
		t.Errorf("DuplicateComponentNameError.Name = %q, want %q", duplicateErr.Name, "a")
	}
}

// rankingAggregate builds an Aggregate with the two given directional pairs,
// each over its own antecedent/consequent component pair, so ranking tests
// can isolate exactly one tie-break dimension at a time.
func rankingAggregate(first, second candidateSpec) Aggregate {
	return Aggregate{
		EligibleTransactions: 100,
		EligibleWeight:       Weights{Unit: 100, InverseComponent: 100, PairNormalized: 100},
		Components: []Component{
			{
				ID: 0, Name: first.antecedentName, RawOpportunity: first.rawOpportunity,
				Occurrence: Weights{Unit: first.exposure},
			},
			{
				ID: 1, Name: first.consequentName,
				Occurrence: Weights{Unit: first.consequentOccurrence},
			},
			{
				ID: 2, Name: second.antecedentName, RawOpportunity: second.rawOpportunity,
				Occurrence: Weights{Unit: second.exposure},
			},
			{
				ID: 3, Name: second.consequentName,
				Occurrence: Weights{Unit: second.consequentOccurrence},
			},
		},
		Pairs: []Pair{
			{
				Antecedent: 0, Consequent: 1, RawSupport: first.rawSupport,
				Support: Weights{Unit: first.support},
			},
			{
				Antecedent: 2, Consequent: 3, RawSupport: second.rawSupport,
				Support: Weights{Unit: second.support},
			},
		},
	}
}

type candidateSpec struct {
	antecedentName, consequentName string
	rawOpportunity                 uint64
	exposure                       float64
	rawSupport                     uint64
	support                        float64
	consequentOccurrence           float64
}

func TestDeriveCandidatesRanksByFrozenTieBreakers(t *testing.T) {
	configuration := looseConfiguration()

	t.Run("weighted confidence descending", func(t *testing.T) {
		// higher: confidence 0.95; lower: confidence 0.80. Everything else
		// held constant so confidence alone decides order.
		higher := candidateSpec{"a1", "b1", 20, 100, 10, 95, 20}
		lower := candidateSpec{"a2", "b2", 20, 100, 10, 80, 20}
		aggregate := rankingAggregate(higher, lower)

		got, err := DeriveCandidates(aggregate, configuration)
		if err != nil {
			t.Fatalf("DeriveCandidates() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(candidates) = %d, want 2", len(got))
		}
		if got[0].Antecedent != 0 || got[1].Antecedent != 2 {
			t.Errorf("order = %+v, want higher-confidence antecedent (0) first", got)
		}
	})

	t.Run("weighted lift descending when confidence ties", func(t *testing.T) {
		// Both pairs: exposure=10, support=8 => confidence=0.8 (tied).
		// higherLift: consequentOccurrence=10 => prevalence=0.1, lift=8.0.
		// lowerLift: consequentOccurrence=20 => prevalence=0.2, lift=4.0.
		higherLift := candidateSpec{"a1", "b1", 20, 10, 5, 8, 10}
		lowerLift := candidateSpec{"a2", "b2", 20, 10, 5, 8, 20}
		aggregate := rankingAggregate(higherLift, lowerLift)

		got, err := DeriveCandidates(aggregate, configuration)
		if err != nil {
			t.Fatalf("DeriveCandidates() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(candidates) = %d, want 2", len(got))
		}
		if got[0].WeightedConfidence != got[1].WeightedConfidence {
			t.Fatalf("test setup error: confidence not tied: %+v", got)
		}
		if got[0].Antecedent != 0 || got[1].Antecedent != 2 {
			t.Errorf("order = %+v, want higher-lift antecedent (0) first", got)
		}
	})

	t.Run("raw support descending when confidence and lift tie", func(t *testing.T) {
		// Both pairs share identical weighted metrics; only RawSupport differs.
		higherSupport := candidateSpec{"a1", "b1", 20, 10, 9, 8, 20}
		lowerSupport := candidateSpec{"a2", "b2", 20, 10, 5, 8, 20}
		aggregate := rankingAggregate(higherSupport, lowerSupport)

		got, err := DeriveCandidates(aggregate, configuration)
		if err != nil {
			t.Fatalf("DeriveCandidates() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(candidates) = %d, want 2", len(got))
		}
		if got[0].WeightedConfidence != got[1].WeightedConfidence ||
			got[0].WeightedLift != got[1].WeightedLift {
			t.Fatalf("test setup error: confidence/lift not tied: %+v", got)
		}
		if got[0].Antecedent != 0 || got[1].Antecedent != 2 {
			t.Errorf("order = %+v, want higher-raw-support antecedent (0) first", got)
		}
	})

	t.Run("raw opportunity descending when confidence, lift, and support tie", func(t *testing.T) {
		// RawOpportunity is independent of Occurrence, so it can differ while
		// every weighted metric and RawSupport stay tied.
		higherOpportunity := candidateSpec{"a1", "b1", 15, 10, 5, 8, 20}
		lowerOpportunity := candidateSpec{"a2", "b2", 5, 10, 5, 8, 20}
		aggregate := rankingAggregate(higherOpportunity, lowerOpportunity)

		got, err := DeriveCandidates(aggregate, configuration)
		if err != nil {
			t.Fatalf("DeriveCandidates() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(candidates) = %d, want 2", len(got))
		}
		if got[0].WeightedConfidence != got[1].WeightedConfidence ||
			got[0].WeightedLift != got[1].WeightedLift ||
			got[0].RawSupport != got[1].RawSupport {
			t.Fatalf("test setup error: earlier keys not tied: %+v", got)
		}
		if got[0].Antecedent != 0 || got[1].Antecedent != 2 {
			t.Errorf("order = %+v, want higher-raw-opportunity antecedent (0) first", got)
		}
	})

	t.Run("antecedent bytes ascending, non-UTF8, when earlier keys tie", func(t *testing.T) {
		// Identical numeric metrics and a shared consequent; only antecedent
		// name bytes differ, using invalid UTF-8 byte sequences to prove
		// plain byte comparison is used.
		aggregate := Aggregate{
			EligibleTransactions: 100,
			EligibleWeight:       Weights{Unit: 100},
			Components: []Component{
				{ID: 0, Name: "\x01\x02", RawOpportunity: 20, Occurrence: Weights{Unit: 10}},
				{ID: 1, Name: "\xff\xfe", RawOpportunity: 20, Occurrence: Weights{Unit: 10}},
				{ID: 2, Name: "shared-consequent", Occurrence: Weights{Unit: 20}},
			},
			Pairs: []Pair{
				{Antecedent: 0, Consequent: 2, RawSupport: 5, Support: Weights{Unit: 8}},
				{Antecedent: 1, Consequent: 2, RawSupport: 5, Support: Weights{Unit: 8}},
			},
		}

		got, err := DeriveCandidates(aggregate, configuration)
		if err != nil {
			t.Fatalf("DeriveCandidates() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(candidates) = %d, want 2", len(got))
		}
		// antecedent 0 is "\x01\x02", antecedent 1 is "\xff\xfe"; ascending
		// byte order ranks antecedent 0 first.
		if got[0].Antecedent != 0 || got[1].Antecedent != 1 {
			t.Errorf("order = %+v, want lower-byte antecedent (0) first", got)
		}
	})

	t.Run("consequent bytes ascending when all other keys tie including antecedent", func(t *testing.T) {
		aggregate := Aggregate{
			EligibleTransactions: 100,
			EligibleWeight:       Weights{Unit: 100},
			Components: []Component{
				{ID: 0, Name: "shared-antecedent", RawOpportunity: 20, Occurrence: Weights{Unit: 10}},
				{ID: 1, Name: "\xff\xfe", Occurrence: Weights{Unit: 20}},
				{ID: 2, Name: "\x01\x02", Occurrence: Weights{Unit: 20}},
			},
			Pairs: []Pair{
				{Antecedent: 0, Consequent: 1, RawSupport: 5, Support: Weights{Unit: 8}},
				{Antecedent: 0, Consequent: 2, RawSupport: 5, Support: Weights{Unit: 8}},
			},
		}

		got, err := DeriveCandidates(aggregate, configuration)
		if err != nil {
			t.Fatalf("DeriveCandidates() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(candidates) = %d, want 2", len(got))
		}
		if got[0].Consequent != 2 || got[1].Consequent != 1 {
			t.Errorf("order = %+v, want lower-byte consequent (2) first", got)
		}
	})
}

// TestDeriveCandidatesRanksByPrecedenceNotJustPresence proves the frozen
// ranking key order is strict precedence, not merely that each key can break
// a tie in isolation. Each case pits an earlier key against a later one in
// opposite directions on the same two candidates, so a candidate that wins on
// the earlier key but loses on the later one must still rank first.
func TestDeriveCandidatesRanksByPrecedenceNotJustPresence(t *testing.T) {
	configuration := looseConfiguration()

	t.Run("confidence beats lift even when lift strongly favors the loser", func(t *testing.T) {
		// higherConfidence: confidence=0.80, lift=1.6 (both comfortably
		// eligible, but lift is the weaker of the two).
		// higherLift: confidence=0.75, lift=12.5 (much higher lift, but lower
		// confidence). Confidence decides first, so higherConfidence must win
		// despite its far smaller lift.
		higherConfidence := candidateSpec{"a1", "b1", 20, 100, 10, 80, 50}
		higherLift := candidateSpec{"a2", "b2", 20, 80, 10, 60, 6}
		aggregate := rankingAggregate(higherConfidence, higherLift)

		got, err := DeriveCandidates(aggregate, configuration)
		if err != nil {
			t.Fatalf("DeriveCandidates() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(candidates) = %d, want 2", len(got))
		}
		if got[0].WeightedConfidence <= got[1].WeightedConfidence {
			t.Fatalf("test setup error: candidate 0 does not have strictly higher confidence: %+v", got)
		}
		if got[0].WeightedLift >= got[1].WeightedLift {
			t.Fatalf("test setup error: candidate 0 does not have strictly lower lift: %+v", got)
		}
		if got[0].Antecedent != 0 || got[1].Antecedent != 2 {
			t.Errorf("order = %+v, want higher-confidence antecedent (0) first despite its lower lift", got)
		}
	})

	t.Run("lift beats raw support even when raw support strongly favors the loser", func(t *testing.T) {
		// Both pairs share identical weighted confidence (tied), so lift is
		// the deciding key. higherLift has the smaller RawSupport; if
		// RawSupport were consulted first or given equal weight,
		// higherSupport (with the larger RawSupport) would win instead.
		higherLift := candidateSpec{"a1", "b1", 20, 10, 5, 8, 10}
		higherSupport := candidateSpec{"a2", "b2", 20, 10, 20, 8, 20}
		aggregate := rankingAggregate(higherLift, higherSupport)

		got, err := DeriveCandidates(aggregate, configuration)
		if err != nil {
			t.Fatalf("DeriveCandidates() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(candidates) = %d, want 2", len(got))
		}
		if got[0].WeightedConfidence != got[1].WeightedConfidence {
			t.Fatalf("test setup error: confidence not tied: %+v", got)
		}
		if got[0].WeightedLift <= got[1].WeightedLift {
			t.Fatalf("test setup error: candidate 0 does not have strictly higher lift: %+v", got)
		}
		if got[0].RawSupport >= got[1].RawSupport {
			t.Fatalf("test setup error: candidate 0 does not have strictly lower raw support: %+v", got)
		}
		if got[0].Antecedent != 0 || got[1].Antecedent != 2 {
			t.Errorf("order = %+v, want higher-lift antecedent (0) first despite its lower raw support", got)
		}
	})

	t.Run("raw support beats raw opportunity even when opportunity strongly favors the loser", func(t *testing.T) {
		// Both pairs share identical weighted confidence and lift (tied), so
		// RawSupport is the deciding key. higherSupport has the smaller
		// RawOpportunity; if RawOpportunity were consulted first or given
		// equal weight, lowerSupport (with the larger RawOpportunity) would
		// win instead.
		higherSupport := candidateSpec{"a1", "b1", 10, 10, 9, 8, 20}
		lowerSupport := candidateSpec{"a2", "b2", 20, 10, 5, 8, 20}
		aggregate := rankingAggregate(higherSupport, lowerSupport)

		got, err := DeriveCandidates(aggregate, configuration)
		if err != nil {
			t.Fatalf("DeriveCandidates() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(candidates) = %d, want 2", len(got))
		}
		if got[0].WeightedConfidence != got[1].WeightedConfidence ||
			got[0].WeightedLift != got[1].WeightedLift {
			t.Fatalf("test setup error: confidence/lift not tied: %+v", got)
		}
		if got[0].RawSupport <= got[1].RawSupport {
			t.Fatalf("test setup error: candidate 0 does not have strictly higher raw support: %+v", got)
		}
		if got[0].RawOpportunity >= got[1].RawOpportunity {
			t.Fatalf("test setup error: candidate 0 does not have strictly lower raw opportunity: %+v", got)
		}
		if got[0].Antecedent != 0 || got[1].Antecedent != 2 {
			t.Errorf("order = %+v, want higher-raw-support antecedent (0) first despite its lower raw opportunity", got)
		}
	})

	t.Run("raw opportunity beats antecedent byte order even when byte order favors the loser", func(t *testing.T) {
		// Every earlier key ties (confidence, lift, raw support), so
		// RawOpportunity is the deciding key. higherOpportunity's antecedent
		// name ("\xff") sorts after lowerOpportunity's ("\x01"); if
		// antecedent bytes were consulted first or given equal weight,
		// lowerOpportunity would win instead.
		higherOpportunity := candidateSpec{"\xff", "c1", 20, 10, 10, 8, 20}
		lowerOpportunity := candidateSpec{"\x01", "c2", 5, 10, 10, 8, 20}
		aggregate := rankingAggregate(higherOpportunity, lowerOpportunity)

		got, err := DeriveCandidates(aggregate, configuration)
		if err != nil {
			t.Fatalf("DeriveCandidates() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(candidates) = %d, want 2", len(got))
		}
		if got[0].WeightedConfidence != got[1].WeightedConfidence ||
			got[0].WeightedLift != got[1].WeightedLift ||
			got[0].RawSupport != got[1].RawSupport {
			t.Fatalf("test setup error: earlier keys not tied: %+v", got)
		}
		if got[0].RawOpportunity <= got[1].RawOpportunity {
			t.Fatalf("test setup error: candidate 0 does not have strictly higher raw opportunity: %+v", got)
		}
		if got[0].Antecedent != 0 || got[1].Antecedent != 2 {
			t.Errorf(
				"order = %+v, want higher-raw-opportunity antecedent (0) first"+
					" despite its higher-byte antecedent name",
				got,
			)
		}
	})

	t.Run("antecedent bytes beat consequent bytes even when consequent order favors the loser", func(t *testing.T) {
		// Every numeric key ties, so the candidates reach the two name
		// tie-breakers. Antecedent name order favors component 0
		// ("\x01" < "\x02"), but consequent name order favors component 3
		// ("\x00" < "\xff"), the opposite candidate. Antecedent bytes decide
		// first, so component 0's candidate must still rank first.
		aggregate := Aggregate{
			EligibleTransactions: 100,
			EligibleWeight:       Weights{Unit: 100},
			Components: []Component{
				{ID: 0, Name: "\x01", RawOpportunity: 20, Occurrence: Weights{Unit: 10}},
				{ID: 1, Name: "\xff", Occurrence: Weights{Unit: 20}},
				{ID: 2, Name: "\x02", RawOpportunity: 20, Occurrence: Weights{Unit: 10}},
				{ID: 3, Name: "\x00", Occurrence: Weights{Unit: 20}},
			},
			Pairs: []Pair{
				{Antecedent: 0, Consequent: 1, RawSupport: 5, Support: Weights{Unit: 8}},
				{Antecedent: 2, Consequent: 3, RawSupport: 5, Support: Weights{Unit: 8}},
			},
		}

		got, err := DeriveCandidates(aggregate, configuration)
		if err != nil {
			t.Fatalf("DeriveCandidates() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(candidates) = %d, want 2", len(got))
		}
		if got[0].WeightedConfidence != got[1].WeightedConfidence ||
			got[0].WeightedLift != got[1].WeightedLift ||
			got[0].RawSupport != got[1].RawSupport ||
			got[0].RawOpportunity != got[1].RawOpportunity {
			t.Fatalf("test setup error: earlier keys not tied: %+v", got)
		}
		if got[0].Antecedent != 0 || got[1].Antecedent != 2 {
			t.Errorf(
				"order = %+v, want lower-antecedent-byte candidate (antecedent 0) first"+
					" despite its higher-byte consequent",
				got,
			)
		}
	})
}

func TestDeriveCandidatesAssignsSequentialOneBasedRank(t *testing.T) {
	higher := candidateSpec{"a1", "b1", 20, 100, 10, 95, 20}
	lower := candidateSpec{"a2", "b2", 20, 100, 10, 80, 20}
	aggregate := rankingAggregate(lower, higher)

	got, err := DeriveCandidates(aggregate, looseConfiguration())
	if err != nil {
		t.Fatalf("DeriveCandidates() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(candidates) = %d, want 2", len(got))
	}
	if got[0].Rank != 1 || got[1].Rank != 2 {
		t.Errorf("ranks = [%d %d], want [1 2]", got[0].Rank, got[1].Rank)
	}
}

func TestDeriveCandidatesReturnsAllEligibleCandidatesWithoutTruncation(t *testing.T) {
	const pairCount = 15

	components := make([]Component, 0, pairCount*2)
	pairs := make([]Pair, 0, pairCount)
	for index := 0; index < pairCount; index++ {
		antecedentID := ComponentID(index * 2)
		consequentID := ComponentID(index*2 + 1)
		// Names must be byte-unique per component: production Snapshot state
		// interns by name, and DeriveCandidates rejects a hand-built aggregate
		// that repeats one, so each pair gets its own antecedent/consequent
		// name pair.
		components = append(components,
			Component{
				ID: antecedentID, Name: fmt.Sprintf("a%d", index), RawOpportunity: 20,
				Occurrence: Weights{Unit: 100},
			},
			Component{
				ID: consequentID, Name: fmt.Sprintf("b%d", index), RawOpportunity: 20,
				Occurrence: Weights{Unit: 20},
			},
		)
		pairs = append(pairs, Pair{
			Antecedent: antecedentID, Consequent: consequentID,
			RawSupport: 10, Support: Weights{Unit: 90},
		})
	}

	aggregate := Aggregate{
		EligibleTransactions: 100,
		EligibleWeight:       Weights{Unit: 100},
		Components:           components,
		Pairs:                pairs,
	}

	got, err := DeriveCandidates(aggregate, looseConfiguration())
	if err != nil {
		t.Fatalf("DeriveCandidates() error = %v", err)
	}
	if len(got) != pairCount {
		t.Errorf("len(candidates) = %d, want %d (no display truncation at this layer)",
			len(got), pairCount)
	}
}

func TestDeriveCandidatesIsDeterministicAndDoesNotMutateAggregate(t *testing.T) {
	aggregate := twoComponentAggregate(100, 100, "a", "b", 20, 100, 10, 90, 10)
	pairsBefore := slices.Clone(aggregate.Pairs)
	componentsBefore := slices.Clone(aggregate.Components)

	first, err := DeriveCandidates(aggregate, looseConfiguration())
	if err != nil {
		t.Fatalf("first DeriveCandidates() error = %v", err)
	}
	second, err := DeriveCandidates(aggregate, looseConfiguration())
	if err != nil {
		t.Fatalf("second DeriveCandidates() error = %v", err)
	}

	if !slices.Equal(first, second) {
		t.Errorf("DeriveCandidates() is not deterministic: %+v != %+v", first, second)
	}
	if !slices.Equal(aggregate.Pairs, pairsBefore) {
		t.Errorf("DeriveCandidates() mutated aggregate.Pairs")
	}
	if !slices.Equal(aggregate.Components, componentsBefore) {
		t.Errorf("DeriveCandidates() mutated aggregate.Components")
	}

	// The returned slice must be detached from the aggregate's backing state.
	first[0].RawSupport = 12345
	third, err := DeriveCandidates(aggregate, looseConfiguration())
	if err != nil {
		t.Fatalf("third DeriveCandidates() error = %v", err)
	}
	if third[0].RawSupport == 12345 {
		t.Errorf("DeriveCandidates() result is not detached from later calls")
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
	configuration := looseConfiguration()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := DeriveCandidates(aggregate, configuration); err != nil {
			b.Fatal(err)
		}
	}
}
