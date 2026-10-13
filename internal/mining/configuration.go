package mining

import (
	"cmp"
	"fmt"
	"slices"
)

// WeightMode names one of the three frozen descriptive size-weight modes.
type WeightMode string

// The frozen size-weight modes, in their canonical simplicity order.
const (
	WeightUnit             WeightMode = "unit"
	WeightInverseComponent WeightMode = "inverse-component"
	WeightPairNormalized   WeightMode = "pair-normalized"
)

var (
	weightModes              = []WeightMode{WeightUnit, WeightInverseComponent, WeightPairNormalized}
	minimumOpportunityValues = []uint64{5, 10, 20}
	minimumConfidenceValues  = []float64{0.70, 0.80, 0.90}
	minimumLiftValues        = []float64{1.25, 1.50, 2.00}
)

// Configuration is one point in the frozen 81-point candidate configuration
// grid: a size-weight mode and the three post-aggregation filter thresholds.
type Configuration struct {
	SizeWeight           WeightMode
	MinimumOpportunities uint64
	MinimumConfidence    float64
	MinimumLift          float64
}

// ID returns the canonical configuration ID:
//
//	weight=<name>;opportunities=<integer>;confidence=<two decimals>;lift=<two decimals>
func (c Configuration) ID() string {
	return fmt.Sprintf(
		"weight=%s;opportunities=%d;confidence=%.2f;lift=%.2f",
		c.SizeWeight, c.MinimumOpportunities, c.MinimumConfidence, c.MinimumLift,
	)
}

func (c Configuration) validate() error {
	if !slices.Contains(weightModes, c.SizeWeight) {
		return &ConfigurationError{Field: "SizeWeight", Value: string(c.SizeWeight)}
	}
	if !slices.Contains(minimumOpportunityValues, c.MinimumOpportunities) {
		return &ConfigurationError{
			Field: "MinimumOpportunities",
			Value: fmt.Sprintf("%d", c.MinimumOpportunities),
		}
	}
	if !slices.Contains(minimumConfidenceValues, c.MinimumConfidence) {
		return &ConfigurationError{
			Field: "MinimumConfidence",
			Value: fmt.Sprintf("%v", c.MinimumConfidence),
		}
	}
	if !slices.Contains(minimumLiftValues, c.MinimumLift) {
		return &ConfigurationError{
			Field: "MinimumLift",
			Value: fmt.Sprintf("%v", c.MinimumLift),
		}
	}

	return nil
}

// AllConfigurations returns all 81 frozen candidate configuration grid points,
// in ascending canonical ID order. Each call returns a freshly built,
// detached slice.
func AllConfigurations() []Configuration {
	configurations := make(
		[]Configuration,
		0,
		len(weightModes)*len(minimumOpportunityValues)*len(minimumConfidenceValues)*len(minimumLiftValues),
	)

	for _, weight := range weightModes {
		for _, opportunities := range minimumOpportunityValues {
			for _, confidence := range minimumConfidenceValues {
				for _, lift := range minimumLiftValues {
					configurations = append(configurations, Configuration{
						SizeWeight:           weight,
						MinimumOpportunities: opportunities,
						MinimumConfidence:    confidence,
						MinimumLift:          lift,
					})
				}
			}
		}
	}

	slices.SortFunc(configurations, func(left, right Configuration) int {
		return cmp.Compare(left.ID(), right.ID())
	})

	return configurations
}
