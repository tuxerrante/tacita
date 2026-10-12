package report

import (
	"bytes"
	"cmp"
	"slices"
)

// Evidence index identifiers.
const (
	EvidenceSchema = "tacita.evidence/v1"
	// ExperimentID names the first preregistered experiment.
	ExperimentID = "tacita-experiment-1"
	// maxEvidenceBytes bounds an evidence index, which lists a few dozen
	// digests and outcomes, on encoding and before it is parsed.
	maxEvidenceBytes = 1 << 20
	// runsPerRepository is how many times the harness runs each repository.
	runsPerRepository = 2
)

// Artifact kinds, in index order.
const (
	ArtifactDevelopmentLock    = "development_lock"
	ArtifactAnalyticalReport   = "analytical_report"
	ArtifactReviewManifest     = "review_manifest"
	ArtifactReviewResult       = "review_result"
	ArtifactOperationalSidecar = "operational_sidecar"
)

// Lanes and their outcome reasons.
const (
	LaneInferred = "inferred"
	LaneProfile  = "profile"

	ReasonNoSelectableConfiguration = "no_selectable_configuration"
	ReasonHoldoutNotRun             = "holdout_not_run"
	ReasonLaneNotRun                = "not_run"
)

var (
	artifactKinds = []string{
		ArtifactDevelopmentLock, ArtifactAnalyticalReport, ArtifactReviewManifest,
		ArtifactReviewResult, ArtifactOperationalSidecar,
	}
	laneReasons = []string{
		ReasonHoldoutNotRun, LaneControlFailed, LaneNondeterministicReport,
		ReasonNoSelectableConfiguration, ReasonLaneNotRun, LaneOperationalLimit, LaneReportNotComplete,
	}
)

// EvidenceIndex is the canonical tacita.evidence/v1 envelope of one recorded
// experiment run: the embedded fields followed by the digest over them.
type EvidenceIndex struct {
	EvidenceFields
	EvidenceDigest string `json:"evidence_digest"`
}

// EvidenceFields are the evidence index fields the digest covers, in order.
type EvidenceFields struct {
	Schema             string              `json:"schema"`
	ExperimentID       string              `json:"experiment_id"`
	Artifacts          []Artifact          `json:"artifacts"`
	AnalyticalGates    []AnalyticalGate    `json:"analytical_gates"`
	OperationalGates   []OperationalGate   `json:"operational_gates"`
	ProductReviewGates []ProductReviewGate `json:"product_review_gates"`
	Lanes              []LaneOutcome       `json:"lanes"`
	ExperimentOutcome  string              `json:"experiment_outcome"`
}

// Artifact references one artifact by digest. The lock has no repository or
// run; reports and sidecars name both.
type Artifact struct {
	Kind               string  `json:"kind"`
	RepositoryObjectID *string `json:"repository_object_id"`
	Run                *int    `json:"run"`
	Digest             string  `json:"digest"`
}

// AnalyticalGate is one analytical report's deterministic outcome: for a
// development repository, whether any configuration passes every gate there;
// for a control repository, its control's outcome.
type AnalyticalGate struct {
	RepositoryObjectID    string `json:"repository_object_id"`
	Run                   int    `json:"run"`
	Outcome               string `json:"outcome"`
	PassingConfigurations *int   `json:"passing_configurations"`
}

// OperationalGate is one run's elapsed-time and peak-memory outcomes.
type OperationalGate struct {
	RepositoryObjectID string `json:"repository_object_id"`
	Run                int    `json:"run"`
	ElapsedOutcome     string `json:"elapsed_outcome"`
	MemoryOutcome      string `json:"memory_outcome"`
}

// ProductReviewGate is a blinded product-review gate outcome. The review runs
// only after a lock exists, so the first index records none.
type ProductReviewGate struct{}

// LaneOutcome is one lane's outcome and, unless it passed, why.
type LaneOutcome struct {
	Lane    string  `json:"lane"`
	Outcome string  `json:"outcome"`
	Reason  *string `json:"reason"`
}

// NewEvidenceIndex seals fields with their digest.
func NewEvidenceIndex(fields EvidenceFields) (EvidenceIndex, error) {
	if err := validateEvidenceIndex(fields); err != nil {
		return EvidenceIndex{}, err
	}
	digest, err := fieldsDigest(fields)
	if err != nil {
		return EvidenceIndex{}, err
	}

	return EvidenceIndex{EvidenceFields: fields, EvidenceDigest: digest}, nil
}

// EncodeEvidenceIndex renders the canonical index bytes after checking its
// contents and digest.
func EncodeEvidenceIndex(index EvidenceIndex) ([]byte, error) {
	if err := validateEvidenceIndex(index.EvidenceFields); err != nil {
		return nil, err
	}
	digest, err := fieldsDigest(index.EvidenceFields)
	if err != nil {
		return nil, err
	}
	if digest != index.EvidenceDigest {
		return nil, invalid("evidence_digest does not cover the index fields")
	}
	encoded, err := canonicalJSON(index)
	if err != nil {
		return nil, &EncodeError{Detail: err.Error()}
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maxEvidenceBytes {
		return nil, invalid("evidence index holds %d bytes, budget %d", len(encoded), maxEvidenceBytes)
	}

	return encoded, nil
}

// DecodeEvidenceIndex parses canonical index bytes and verifies the digest.
func DecodeEvidenceIndex(data []byte) (EvidenceIndex, error) {
	if len(data) > maxEvidenceBytes {
		return EvidenceIndex{}, &DecodeError{Detail: "evidence index exceeds its byte budget"}
	}
	var index EvidenceIndex
	if err := strictUnmarshal(data, &index); err != nil {
		return EvidenceIndex{}, err
	}
	canonical, err := EncodeEvidenceIndex(index)
	if err != nil {
		return EvidenceIndex{}, asDecodeError(err)
	}
	if !bytes.Equal(canonical, data) {
		return EvidenceIndex{}, &DecodeError{Detail: "evidence index bytes are not canonical"}
	}

	return index, nil
}

// corpusPosition orders repositories by the frozen corpus table; an unknown
// commit sorts last.
func corpusPosition(commit *string) int {
	if commit == nil {
		return -1
	}
	for index, repository := range CalibrationCorpus() {
		if repository.Commit == *commit {
			return index
		}
	}

	return len(CalibrationCorpus())
}

func compareArtifacts(left, right Artifact) int {
	return cmp.Or(
		cmp.Compare(slices.Index(artifactKinds, left.Kind), slices.Index(artifactKinds, right.Kind)),
		cmp.Compare(corpusPosition(left.RepositoryObjectID), corpusPosition(right.RepositoryObjectID)),
		cmp.Compare(optionalRun(left.Run), optionalRun(right.Run)),
	)
}

func optionalRun(run *int) int {
	if run == nil {
		return 0
	}
	return *run
}

// validateEvidenceIndex requires the frozen schema and experiment, known
// artifact kinds in index order with valid digests and calibration-corpus
// repositories, known outcomes, both lanes, and an experiment outcome that is
// the lanes' conjunction.
func validateEvidenceIndex(fields EvidenceFields) error {
	if fields.Schema != EvidenceSchema || fields.ExperimentID != ExperimentID {
		return invalid("evidence schema %q experiment %q", fields.Schema, fields.ExperimentID)
	}
	for index, artifact := range fields.Artifacts {
		lock := artifact.Kind == ArtifactDevelopmentLock
		if !oneOf(artifactKinds, artifact.Kind) || !isDigest(artifact.Digest) ||
			lock != (artifact.RepositoryObjectID == nil) || lock != (artifact.Run == nil) ||
			(!lock && (corpusPosition(artifact.RepositoryObjectID) == len(CalibrationCorpus()) ||
				*artifact.Run < 1 || *artifact.Run > runsPerRepository)) {
			return invalid("evidence artifact %d", index)
		}
		if index > 0 && compareArtifacts(fields.Artifacts[index-1], artifact) >= 0 {
			return invalid("evidence artifacts not in strict kind, repository, and run order at %d", index)
		}
	}
	reports := artifactKeys(fields.Artifacts, ArtifactAnalyticalReport)
	sidecars := artifactKeys(fields.Artifacts, ArtifactOperationalSidecar)
	for _, key := range reports {
		if !slices.Contains(sidecars, key) {
			return invalid("evidence report %s run %d has no operational sidecar", key.commit, key.run)
		}
	}
	if len(fields.AnalyticalGates) != len(reports) {
		return invalid("evidence holds %d analytical gates for %d reports", len(fields.AnalyticalGates), len(reports))
	}
	for index, gate := range fields.AnalyticalGates {
		development := corpusPosition(&gate.RepositoryObjectID) < 2
		if (artifactKey{commit: gate.RepositoryObjectID, run: gate.Run}) != reports[index] ||
			!oneOf([]string{outcomePass, outcomeFail}, gate.Outcome) ||
			development != (gate.PassingConfigurations != nil) ||
			(development && (*gate.PassingConfigurations < 0 ||
				(*gate.PassingConfigurations > 0) != (gate.Outcome == outcomePass))) {
			return invalid("evidence analytical gate %d does not match its report", index)
		}
	}
	if len(fields.OperationalGates) != len(sidecars) {
		return invalid("evidence holds %d operational gates for %d sidecars", len(fields.OperationalGates), len(sidecars))
	}
	for index, gate := range fields.OperationalGates {
		if (artifactKey{commit: gate.RepositoryObjectID, run: gate.Run}) != sidecars[index] ||
			!oneOf([]string{outcomePass, outcomeFail}, gate.ElapsedOutcome) ||
			!oneOf([]string{outcomePass, outcomeFail}, gate.MemoryOutcome) {
			return invalid("evidence operational gate %d does not match its sidecar", index)
		}
	}
	if len(fields.ProductReviewGates) > 0 {
		return invalid("product review gates are recorded only after a review exists")
	}

	lanes := make([]string, len(fields.Lanes))
	outcome := outcomePass
	for index, lane := range fields.Lanes {
		lanes[index] = lane.Lane
		if !oneOf(gateOutcomes, lane.Outcome) || (lane.Outcome == outcomePass) != (lane.Reason == nil) ||
			(lane.Reason != nil && !oneOf(laneReasons, *lane.Reason)) {
			return invalid("evidence lane %q outcome %q", lane.Lane, lane.Outcome)
		}
		switch {
		case lane.Outcome == outcomeFail:
			outcome = outcomeFail
		case lane.Outcome == outcomeInconclusive && outcome == outcomePass:
			outcome = outcomeInconclusive
		}
	}
	if !slices.Equal(lanes, []string{LaneInferred, LaneProfile}) {
		return invalid("evidence lanes %v, want [inferred profile]", lanes)
	}
	if fields.ExperimentOutcome != outcome {
		return invalid("experiment outcome %q is not the lanes' conjunction %q", fields.ExperimentOutcome, outcome)
	}

	return nil
}

// artifactKey identifies a report or sidecar artifact by repository and run.
type artifactKey struct {
	commit string
	run    int
}

// artifactKeys lists the repository and run of every artifact of kind, in
// index order.
func artifactKeys(artifacts []Artifact, kind string) []artifactKey {
	var keys []artifactKey
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			keys = append(keys, artifactKey{commit: *artifact.RepositoryObjectID, run: *artifact.Run})
		}
	}
	return keys
}
