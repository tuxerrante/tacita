package report

import (
	"bytes"
	"slices"

	"github.com/tuxerrante/tacita/internal/mining"
)

// Lock identifiers.
const (
	LockSchema       = "tacita.dev-lock/v1"
	SelectorVersion  = "development-selector-v1"
	lockArchitecture = "linux/amd64"
)

// Selector metric names, in the frozen selector key order.
const (
	MetricWorstCaseFrequentConsequentGain = "worst_case_frequent_consequent_gain"
	MetricWorstCaseCoverage               = "worst_case_coverage"
	MetricMinimumAdjacentJaccard          = "minimum_adjacent_jaccard"
)

// Lock is the canonical tacita.dev-lock/v1 artifact. Its fields are the
// embedded LockFields followed by the digest over them.
type Lock struct {
	LockFields
	LockDigest string `json:"lock_digest"`
}

// LockFields are the canonical lock fields the lock digest covers, in order.
type LockFields struct {
	Schema             string           `json:"schema"`
	Corpus             []string         `json:"corpus"`
	Environment        Environment      `json:"environment"`
	GridDigest         string           `json:"grid_digest"`
	Selector           string           `json:"selector"`
	ConfigurationID    string           `json:"configuration_id"`
	Parameters         Parameters       `json:"parameters"`
	DevelopmentReports []ReportDigest   `json:"development_reports"`
	SelectorMetrics    []SelectorMetric `json:"selector_metrics"`
}

// Environment pins what every calibration and holdout run must reuse.
type Environment struct {
	Architecture       string  `json:"architecture"`
	GitVersion         string  `json:"git_version"`
	TacitaBinarySHA256 string  `json:"tacita_binary_sha256"`
	GoHelperVersion    *string `json:"go_helper_version"`
}

// Parameters are the selected configuration's thresholds.
type Parameters struct {
	SizeWeight           string  `json:"size_weight"`
	MinimumOpportunities uint64  `json:"minimum_opportunities"`
	MinimumConfidence    Decimal `json:"minimum_confidence"`
	MinimumLift          Decimal `json:"minimum_lift"`
}

// ReportDigest pairs a development repository with its report's digest.
type ReportDigest struct {
	RepositoryObjectID string `json:"repository_object_id"`
	ReportDigest       string `json:"report_digest"`
}

// SelectorMetric is one selector key's value for the selected configuration.
type SelectorMetric struct {
	Metric string  `json:"metric"`
	Value  Decimal `json:"value"`
}

// NewLock seals fields with their digest.
func NewLock(fields LockFields) (Lock, error) {
	digest, err := fieldsDigest(fields)
	if err != nil {
		return Lock{}, err
	}

	return Lock{LockFields: fields, LockDigest: digest}, nil
}

// EncodeLock renders the canonical lock bytes after validating its contents
// against the frozen corpus, grid, and selector and checking its digest.
func EncodeLock(lock Lock) ([]byte, error) {
	if err := validateLock(lock.LockFields); err != nil {
		return nil, err
	}
	digest, err := fieldsDigest(lock.LockFields)
	if err != nil {
		return nil, err
	}
	if digest != lock.LockDigest {
		return nil, invalid("lock_digest does not cover the lock fields")
	}
	encoded, err := canonicalJSON(lock)
	if err != nil {
		return nil, &EncodeError{Detail: err.Error()}
	}

	return append(encoded, '\n'), nil
}

// DecodeLock parses canonical lock bytes and verifies the embedded digest.
func DecodeLock(data []byte) (Lock, error) {
	var lock Lock
	if err := strictUnmarshal(data, &lock); err != nil {
		return Lock{}, err
	}
	canonical, err := EncodeLock(lock)
	if err != nil {
		return Lock{}, err
	}
	if !bytes.Equal(canonical, data) {
		return Lock{}, &DecodeError{Detail: "lock bytes are not canonical"}
	}

	return lock, nil
}

// selectorMetricOrder is the frozen selector key order.
var selectorMetricOrder = []string{
	MetricWorstCaseFrequentConsequentGain,
	MetricWorstCaseCoverage,
	MetricMinimumAdjacentJaccard,
}

// validateLock requires a lock that only the frozen experiment could have
// produced: its corpus, environment shape, grid, selector, parameters, and
// development reports.
func validateLock(fields LockFields) error {
	corpus := make([]string, 0, len(CalibrationCorpus()))
	for _, repository := range CalibrationCorpus() {
		corpus = append(corpus, repository.Commit)
	}
	grid, err := FrozenGrid()
	if err != nil {
		return err
	}
	switch {
	case fields.Schema != LockSchema || fields.Selector != SelectorVersion:
		return invalid("lock schema %q selector %q", fields.Schema, fields.Selector)
	case !slices.Equal(fields.Corpus, corpus):
		return invalid("lock corpus is not the frozen development and control corpus")
	case fields.Environment.Architecture != lockArchitecture || fields.Environment.GitVersion == "" ||
		!isDigest(fields.Environment.TacitaBinarySHA256):
		return invalid("lock environment %+v", fields.Environment)
	case fields.GridDigest != grid.GridDigest:
		return invalid("lock grid_digest is not the frozen grid's")
	}

	var configuration *mining.Configuration
	for _, candidate := range mining.AllConfigurations() {
		if candidate.ID() == fields.ConfigurationID {
			configuration = &candidate
		}
	}
	if configuration == nil || fields.Parameters != (Parameters{
		SizeWeight:           string(configuration.SizeWeight),
		MinimumOpportunities: configuration.MinimumOpportunities,
		MinimumConfidence:    Decimal(configuration.MinimumConfidence),
		MinimumLift:          Decimal(configuration.MinimumLift),
	}) {
		return invalid("lock configuration %q and parameters disagree with the frozen grid", fields.ConfigurationID)
	}

	if len(fields.DevelopmentReports) != 2 {
		return invalid("lock holds %d development reports, want 2", len(fields.DevelopmentReports))
	}
	for index, report := range fields.DevelopmentReports {
		if report.RepositoryObjectID != corpus[index] || !isDigest(report.ReportDigest) {
			return invalid("lock development report %d", index)
		}
	}
	metrics := make([]string, len(fields.SelectorMetrics))
	for index, metric := range fields.SelectorMetrics {
		metrics[index] = metric.Metric
		if _, err := formatDecimal(float64(metric.Value)); err != nil {
			return err
		}
	}
	if !slices.Equal(metrics, selectorMetricOrder) {
		return invalid("lock selector metrics %v, want %v", metrics, selectorMetricOrder)
	}

	return nil
}
