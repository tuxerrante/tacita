package report

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// RunRecord is one calibration run's artifacts: the report bytes, nil when the
// run wrote none, and its operational sidecar.
type RunRecord struct {
	Repository CorpusRepository
	Run        int
	Report     []byte
	Operation  Operation
}

// EvidenceInput is a recorded calibration: every run, the selection when all
// reports existed, and the encoded lock when one was written.
type EvidenceInput struct {
	Runs      []RunRecord
	Selection *Selection
	Lock      []byte
}

// BuildEvidence assembles the evidence index of a recorded calibration. The
// inferred lane fails without a lock, as the frozen protocol requires, and is
// inconclusive with one until the holdout runs; the profile lane has not run.
func BuildEvidence(input EvidenceInput) (EvidenceIndex, error) {
	fields := EvidenceFields{
		Schema:             EvidenceSchema,
		ExperimentID:       ExperimentID,
		Artifacts:          []Artifact{},
		AnalyticalGates:    []AnalyticalGate{},
		OperationalGates:   []OperationalGate{},
		ProductReviewGates: []ProductReviewGate{},
	}
	if input.Lock != nil {
		fields.Artifacts = append(fields.Artifacts, Artifact{Kind: ArtifactDevelopmentLock, Digest: Digest(input.Lock)})
	}

	for _, run := range input.Runs {
		commit, number := run.Repository.Commit, run.Run
		sidecar, err := EncodeOperation(run.Operation)
		if err != nil {
			return EvidenceIndex{}, fmt.Errorf("encoding %s run %d sidecar: %w", run.Repository.Name, number, err)
		}
		fields.Artifacts = append(fields.Artifacts, Artifact{
			Kind: ArtifactOperationalSidecar, RepositoryObjectID: &commit, Run: &number, Digest: Digest(sidecar),
		})
		fields.OperationalGates = append(fields.OperationalGates, OperationalGate{
			RepositoryObjectID: commit,
			Run:                number,
			ElapsedOutcome:     passOrFail(run.Operation.WithinElapsedLimit()),
			MemoryOutcome:      passOrFail(run.Operation.WithinMemoryLimit()),
		})
		if run.Report == nil {
			continue
		}
		fields.Artifacts = append(fields.Artifacts, Artifact{
			Kind: ArtifactAnalyticalReport, RepositoryObjectID: &commit, Run: &number, Digest: Digest(run.Report),
		})
		gate, err := analyticalGate(run)
		if err != nil {
			return EvidenceIndex{}, err
		}
		fields.AnalyticalGates = append(fields.AnalyticalGates, gate)
	}
	slices.SortFunc(fields.Artifacts, compareArtifacts)

	fields.Lanes = []LaneOutcome{inferredLane(input), {Lane: LaneProfile, Outcome: outcomeInconclusive, Reason: text(ReasonLaneNotRun)}}
	fields.ExperimentOutcome = outcomeFail
	if fields.Lanes[0].Outcome != outcomeFail {
		fields.ExperimentOutcome = outcomeInconclusive
	}

	return NewEvidenceIndex(fields)
}

// analyticalGate summarizes one report: how many configurations pass every
// gate of a development report, or a control report's control outcome.
func analyticalGate(run RunRecord) (AnalyticalGate, error) {
	report, err := Decode(run.Report)
	if err != nil {
		return AnalyticalGate{}, fmt.Errorf("decoding %s run %d report: %w", run.Repository.Name, run.Run, err)
	}
	gate := AnalyticalGate{RepositoryObjectID: run.Repository.Commit, Run: run.Run, Outcome: outcomeFail}
	if report.Status != statusComplete {
		return gate, nil
	}

	if run.Repository.Role != RoleDevelopment {
		if controlPasses(report, controlIDFor(run.Repository)) {
			gate.Outcome = outcomePass
		}
		return gate, nil
	}

	failing := make(map[string]bool)
	repositoryPasses := true
	for _, record := range report.Gates {
		if record.Outcome == outcomePass {
			continue
		}
		if record.ConfigurationID == nil {
			repositoryPasses = false
			continue
		}
		failing[*record.ConfigurationID] = true
	}
	passing := 0
	if repositoryPasses {
		passing = len(report.Configurations) - len(failing)
	}
	gate.PassingConfigurations = &passing
	if passing > 0 {
		gate.Outcome = outcomePass
	}

	return gate, nil
}

// inferredLane fails without a lock, giving the first lane failure or that no
// configuration was selectable, and is inconclusive with one until the
// configuration holdout runs.
func inferredLane(input EvidenceInput) LaneOutcome {
	lane := LaneOutcome{Lane: LaneInferred, Outcome: outcomeFail}
	switch {
	case input.Lock != nil:
		lane.Outcome, lane.Reason = outcomeInconclusive, text(ReasonHoldoutNotRun)
	case input.Selection == nil:
		lane.Reason = text(LaneReportNotComplete)
	case len(input.Selection.LaneFailures) > 0:
		failures := slices.Clone(input.Selection.LaneFailures)
		slices.SortFunc(failures, func(left, right LaneFailure) int {
			return cmp.Or(cmp.Compare(left.Reason, right.Reason), cmp.Compare(left.Repository, right.Repository))
		})
		lane.Reason = text(failures[0].Reason)
	default:
		lane.Reason = text(ReasonNoSelectableConfiguration)
	}

	return lane
}

func passOrFail(passed bool) string {
	if passed {
		return outcomePass
	}
	return outcomeFail
}

func text(value string) *string {
	return &value
}

// Summary renders why a selection wrote no lock: every lane failure, the
// number of rejected configurations, and the rejections counted by gate and
// outcome, in byte order.
func (s Selection) Summary() string {
	var builder strings.Builder
	builder.WriteString("no selectable configuration\n")

	failures := slices.Clone(s.LaneFailures)
	slices.SortFunc(failures, func(left, right LaneFailure) int {
		return cmp.Or(cmp.Compare(left.Repository, right.Repository), cmp.Compare(left.Reason, right.Reason))
	})
	for _, failure := range failures {
		fmt.Fprintf(&builder, "lane_failure %s %s\n", failure.Repository, failure.Reason)
	}

	configurations := make(map[string]bool)
	counts := make(map[string]int)
	for _, rejection := range s.Rejections {
		if rejection.ConfigurationID != "" {
			configurations[rejection.ConfigurationID] = true
		}
		counts[rejection.Gate+" "+rejection.Outcome]++
	}
	fmt.Fprintf(&builder, "rejected_configurations %d\n", len(configurations))
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		fmt.Fprintf(&builder, "rejections %s %d\n", key, counts[key])
	}

	return builder.String()
}
