package mining

import (
	"errors"
	"slices"
	"testing"
)

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
					wantIDs[Configuration{
						SizeWeight:           weight,
						MinimumOpportunities: opportunity,
						MinimumConfidence:    confidence,
						MinimumLift:          lift,
					}.ID()] = true
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
		return compareStrings(left.ID(), right.ID())
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
			err := tt.configuration.validate()
			if !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf("validate() error = %v, want ErrInvalidConfiguration", err)
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

func compareStrings(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
