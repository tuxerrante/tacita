package report

// GridSchema identifies the frozen candidate configuration grid artifact.
const GridSchema = "tacita.grid/v1"

// Grid is the canonical tacita.grid/v1 artifact. Values are exactly those of
// the frozen candidate configuration grid, in their frozen orders.
type Grid struct {
	Schema                      string    `json:"schema"`
	MinimumTrainingTransactions int       `json:"minimum_training_transactions"`
	MinimumRawSupport           int       `json:"minimum_raw_support"`
	SizeWeights                 []string  `json:"size_weights"`
	MinimumOpportunities        []uint64  `json:"minimum_opportunities"`
	MinimumConfidence           []Decimal `json:"minimum_confidence"`
	MinimumLift                 []Decimal `json:"minimum_lift"`
	DisplayLimit                int       `json:"display_limit"`
	RankingKeys                 []string  `json:"ranking_keys"`
	GridDigest                  string    `json:"grid_digest"`
}

// gridFields is Grid without its digest: the canonical fields the digest
// covers.
type gridFields struct {
	Schema                      string    `json:"schema"`
	MinimumTrainingTransactions int       `json:"minimum_training_transactions"`
	MinimumRawSupport           int       `json:"minimum_raw_support"`
	SizeWeights                 []string  `json:"size_weights"`
	MinimumOpportunities        []uint64  `json:"minimum_opportunities"`
	MinimumConfidence           []Decimal `json:"minimum_confidence"`
	MinimumLift                 []Decimal `json:"minimum_lift"`
	DisplayLimit                int       `json:"display_limit"`
	RankingKeys                 []string  `json:"ranking_keys"`
}

// FrozenGrid returns the frozen grid with its digest.
func FrozenGrid() (Grid, error) {
	fields := gridFields{
		Schema:                      GridSchema,
		MinimumTrainingTransactions: 100,
		MinimumRawSupport:           3,
		SizeWeights:                 []string{weightUnit, weightInverseComponent, weightPairNormalized},
		MinimumOpportunities:        []uint64{5, 10, 20},
		MinimumConfidence:           []Decimal{0.70, 0.80, 0.90},
		MinimumLift:                 []Decimal{1.25, 1.50, 2.00},
		DisplayLimit:                10,
		RankingKeys: []string{
			"weighted_confidence_descending",
			"weighted_lift_descending",
			"raw_support_descending",
			"raw_opportunity_descending",
			"antecedent_bytes_ascending",
			"consequent_bytes_ascending",
		},
	}
	digest, err := fieldsDigest(fields)
	if err != nil {
		return Grid{}, err
	}

	return Grid{
		Schema:                      fields.Schema,
		MinimumTrainingTransactions: fields.MinimumTrainingTransactions,
		MinimumRawSupport:           fields.MinimumRawSupport,
		SizeWeights:                 fields.SizeWeights,
		MinimumOpportunities:        fields.MinimumOpportunities,
		MinimumConfidence:           fields.MinimumConfidence,
		MinimumLift:                 fields.MinimumLift,
		DisplayLimit:                fields.DisplayLimit,
		RankingKeys:                 fields.RankingKeys,
		GridDigest:                  digest,
	}, nil
}

// EncodeFrozenGrid renders the canonical frozen grid artifact. It takes no
// argument, so no caller can encode a grid the experiment did not freeze.
func EncodeFrozenGrid() ([]byte, error) {
	grid, err := FrozenGrid()
	if err != nil {
		return nil, err
	}
	encoded, err := canonicalJSON(grid)
	if err != nil {
		return nil, &EncodeError{Detail: err.Error()}
	}

	return append(encoded, '\n'), nil
}

// fieldsDigest is the SHA-256 of the canonical artifact bytes of the fields an
// artifact digest covers: the preceding fields as one minified object
// followed by one LF byte.
func fieldsDigest(fields any) (string, error) {
	encoded, err := canonicalJSON(fields)
	if err != nil {
		return "", &EncodeError{Detail: err.Error()}
	}

	return Digest(append(encoded, '\n')), nil
}
