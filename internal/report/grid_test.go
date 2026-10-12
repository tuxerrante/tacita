package report

import (
	"testing"

	"github.com/tuxerrante/tacita/internal/mining"
)

func TestFrozenGridEncodesCanonically(t *testing.T) {
	grid, err := FrozenGrid()
	if err != nil {
		t.Fatalf("FrozenGrid() error = %v", err)
	}
	encoded, err := EncodeGrid(grid)
	if err != nil {
		t.Fatalf("EncodeGrid() error = %v", err)
	}

	const fields = `{"schema":"tacita.grid/v1","minimum_training_transactions":100,"minimum_raw_support":3,` +
		`"size_weights":["unit","inverse-component","pair-normalized"],"minimum_opportunities":[5,10,20],` +
		`"minimum_confidence":["0.700000","0.800000","0.900000"],"minimum_lift":["1.250000","1.500000","2.000000"],` +
		`"display_limit":10,"ranking_keys":["weighted_confidence_descending","weighted_lift_descending",` +
		`"raw_support_descending","raw_opportunity_descending","antecedent_bytes_ascending","consequent_bytes_ascending"]`
	digest := Digest([]byte(fields + "}\n"))
	want := fields + `,"grid_digest":"` + digest + `"}` + "\n"
	if string(encoded) != want {
		t.Errorf("EncodeGrid() =\n%s\nwant\n%s", encoded, want)
	}
	if grid.GridDigest != digest {
		t.Errorf("GridDigest = %s, want %s", grid.GridDigest, digest)
	}
}

// TestFrozenGridMatchesTheMiner proves the artifact describes the grid the
// miner enumerates: its Cartesian product is exactly the 81 configurations.
func TestFrozenGridMatchesTheMiner(t *testing.T) {
	grid, err := FrozenGrid()
	if err != nil {
		t.Fatalf("FrozenGrid() error = %v", err)
	}

	product := make(map[mining.Configuration]bool)
	for _, weight := range grid.SizeWeights {
		for _, opportunities := range grid.MinimumOpportunities {
			for _, confidence := range grid.MinimumConfidence {
				for _, lift := range grid.MinimumLift {
					product[mining.Configuration{
						SizeWeight:           mining.WeightMode(weight),
						MinimumOpportunities: opportunities,
						MinimumConfidence:    float64(confidence),
						MinimumLift:          float64(lift),
					}] = true
				}
			}
		}
	}

	configurations := mining.AllConfigurations()
	if len(product) != len(configurations) {
		t.Fatalf("grid product has %d configurations, miner has %d", len(product), len(configurations))
	}
	for _, configuration := range configurations {
		if !product[configuration] {
			t.Errorf("miner configuration %s is not in the grid artifact", configuration.ID())
		}
	}
}
