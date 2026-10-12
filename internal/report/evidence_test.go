package report

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func operationFor(t *testing.T, report []byte, elapsed int64, peak int64, exit int) Operation {
	t.Helper()

	fields := OperationFields{
		Schema: OperationSchema, ElapsedDurationNS: elapsed, PeakResidentBytes: peak,
		ProcessExitStatus: exit, ReferenceRunner: "test runner",
	}
	if report != nil {
		digest := Digest(report)
		fields.ReportDigest = &digest
	}
	operation, err := NewOperation(fields)
	if err != nil {
		t.Fatalf("NewOperation() error = %v", err)
	}
	return operation
}

func TestOperationRoundTripsAndChecksBudgets(t *testing.T) {
	operation := operationFor(t, []byte("report\n"), int64(ElapsedLimit), PeakMemoryLimit, 0)

	encoded, err := EncodeOperation(operation)
	if err != nil {
		t.Fatalf("EncodeOperation() error = %v", err)
	}
	decoded, err := DecodeOperation(encoded)
	if err != nil || decoded.SidecarDigest != operation.SidecarDigest {
		t.Fatalf("DecodeOperation() = %+v, %v", decoded, err)
	}
	if !operation.WithinElapsedLimit() || !operation.WithinMemoryLimit() {
		t.Error("a run exactly at both limits is reported over them")
	}
	over := operationFor(t, nil, int64(ElapsedLimit)+1, PeakMemoryLimit+1, 1)
	if over.WithinElapsedLimit() || over.WithinMemoryLimit() {
		t.Error("a run over both limits is reported within them")
	}
	if !strings.Contains(string(encoded), `"diagnostic_paths":null`) {
		t.Errorf("sidecar = %s", encoded)
	}
}

func TestOperationRejectsInvalidContents(t *testing.T) {
	bad := "x"
	tests := map[string]OperationFields{
		"schema":           {Schema: "tacita.operation/v2", ReferenceRunner: "r"},
		"report digest":    {Schema: OperationSchema, ReportDigest: &bad, ReferenceRunner: "r"},
		"negative elapsed": {Schema: OperationSchema, ElapsedDurationNS: -1, ReferenceRunner: "r"},
		"negative exit":    {Schema: OperationSchema, ProcessExitStatus: -1, ReferenceRunner: "r"},
		"empty runner":     {Schema: OperationSchema},
		"oversized runner": {Schema: OperationSchema, ReferenceRunner: strings.Repeat("r", maxSidecarBytes)},
	}
	for name, fields := range tests {
		operation := Operation{OperationFields: fields}
		if _, err := EncodeOperation(operation); !errors.Is(err, ErrEncode) {
			t.Errorf("%s: EncodeOperation() error = %v, want ErrEncode", name, err)
		}
	}

	valid := operationFor(t, nil, 1, 1, 0)
	forged := valid
	forged.SidecarDigest = strings.Repeat("0", 64)
	if _, err := EncodeOperation(forged); !errors.Is(err, ErrEncode) {
		t.Errorf("EncodeOperation(forged digest) error = %v", err)
	}
	encoded, err := EncodeOperation(valid)
	if err != nil {
		t.Fatal(err)
	}
	for name, input := range map[string]string{
		"whitespace":    strings.Replace(string(encoded), `,"elapsed_duration_ns"`, `, "elapsed_duration_ns"`, 1),
		"changed value": strings.Replace(string(encoded), `"elapsed_duration_ns":1`, `"elapsed_duration_ns":2`, 1),
		"unknown field": strings.Replace(string(encoded), `"schema"`, `"x":1,"schema"`, 1),
		"oversized":     strings.Repeat(" ", maxSidecarBytes+1),
	} {
		if _, err := DecodeOperation([]byte(input)); !errors.Is(err, ErrDecode) {
			t.Errorf("%s: DecodeOperation() error = %v, want ErrDecode", name, err)
		}
	}
}

// recordedRuns builds every calibration run from a selection input, all
// within their operational budgets.
func recordedRuns(t *testing.T, input SelectionInput) []RunRecord {
	t.Helper()

	var runs []RunRecord
	for index, run := range input.Development {
		repository := []CorpusRepository{GolangTools(), Kubernetes()}[index]
		runs = append(runs,
			RunRecord{Repository: repository, Run: 1, Report: run.Report, Operation: operationFor(t, run.Report, 1, 1, 0)},
			RunRecord{Repository: repository, Run: 2, Report: run.Repeat, Operation: operationFor(t, run.Repeat, 1, 1, 0)},
		)
	}
	for index, run := range input.Controls {
		repository := []CorpusRepository{GoStarter(), Proficiency()}[index]
		runs = append(runs,
			RunRecord{Repository: repository, Run: 1, Report: run.Report, Operation: operationFor(t, run.Report, 1, 1, 0)},
			RunRecord{Repository: repository, Run: 2, Report: run.Repeat, Operation: operationFor(t, run.Repeat, 1, 1, 0)},
		)
	}
	return runs
}

func TestBuildEvidenceWithoutALock(t *testing.T) {
	input := selectionInput(t,
		map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
		map[string]keys{unitLoose: {fail: true}},
	)
	selection, err := Select(input)
	if err != nil || selection.Lock != nil {
		t.Fatalf("Select() = %+v, %v", selection, err)
	}

	index, err := BuildEvidence(EvidenceInput{Runs: recordedRuns(t, input), Selection: &selection})
	if err != nil {
		t.Fatalf("BuildEvidence() error = %v", err)
	}

	if len(index.Artifacts) != 16 || index.Artifacts[0].Kind != ArtifactAnalyticalReport {
		t.Errorf("artifacts = %d, first %s", len(index.Artifacts), index.Artifacts[0].Kind)
	}
	if got := *index.AnalyticalGates[0].PassingConfigurations; got != 1 {
		t.Errorf("golang/tools passing configurations = %d, want 1", got)
	}
	if index.AnalyticalGates[2].Outcome != outcomeFail || index.AnalyticalGates[4].Outcome != outcomePass || index.AnalyticalGates[7].Outcome != outcomePass {
		t.Errorf("analytical gates = %+v", index.AnalyticalGates)
	}
	lane := index.Lanes[0]
	if lane.Outcome != outcomeFail || *lane.Reason != ReasonNoSelectableConfiguration || index.ExperimentOutcome != outcomeFail {
		t.Errorf("lanes = %+v experiment %s", index.Lanes, index.ExperimentOutcome)
	}

	encoded, err := EncodeEvidenceIndex(index)
	if err != nil {
		t.Fatalf("EncodeEvidenceIndex() error = %v", err)
	}
	decoded, err := DecodeEvidenceIndex(encoded)
	if err != nil || decoded.EvidenceDigest != index.EvidenceDigest {
		t.Fatalf("DecodeEvidenceIndex() = %v", err)
	}
}

func TestBuildEvidenceWithALock(t *testing.T) {
	passing := map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}}
	input := selectionInput(t, passing, passing)
	selection, err := Select(input)
	if err != nil || selection.Lock == nil {
		t.Fatalf("Select() = %+v, %v", selection, err)
	}
	lock, err := EncodeLock(*selection.Lock)
	if err != nil {
		t.Fatal(err)
	}

	index, err := BuildEvidence(EvidenceInput{Runs: recordedRuns(t, input), Selection: &selection, Lock: lock})
	if err != nil {
		t.Fatalf("BuildEvidence() error = %v", err)
	}

	if index.Artifacts[0].Kind != ArtifactDevelopmentLock || index.Artifacts[0].Digest != Digest(lock) {
		t.Errorf("first artifact = %+v, want the lock", index.Artifacts[0])
	}
	if lane := index.Lanes[0]; lane.Outcome != outcomeInconclusive || *lane.Reason != ReasonHoldoutNotRun ||
		index.ExperimentOutcome != outcomeInconclusive {
		t.Errorf("lanes = %+v experiment %s", index.Lanes, index.ExperimentOutcome)
	}
}

func TestBuildEvidenceRecordsLaneFailuresAndMissingReports(t *testing.T) {
	passing := map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}}
	input := selectionInput(t, passing, passing)
	input.Development[0].OperationalPass = false
	selection, err := Select(input)
	if err != nil {
		t.Fatal(err)
	}
	runs := recordedRuns(t, input)
	runs[0].Operation = operationFor(t, runs[0].Report, int64(ElapsedLimit)+1, 1, 0)

	index, err := BuildEvidence(EvidenceInput{Runs: runs, Selection: &selection})
	if err != nil {
		t.Fatal(err)
	}
	if *index.Lanes[0].Reason != LaneOperationalLimit || index.OperationalGates[0].ElapsedOutcome != outcomeFail {
		t.Errorf("lane %+v operational %+v", index.Lanes[0], index.OperationalGates[0])
	}

	runs[2].Report = nil
	runs[2].Operation = operationFor(t, nil, 1, 1, 1)
	index, err = BuildEvidence(EvidenceInput{Runs: runs})
	if err != nil {
		t.Fatal(err)
	}
	if *index.Lanes[0].Reason != LaneReportNotComplete || len(index.Artifacts) != 15 || len(index.AnalyticalGates) != 7 {
		t.Errorf("missing report: lane %+v artifacts %d gates %d", index.Lanes[0], len(index.Artifacts), len(index.AnalyticalGates))
	}

	failed := failedReport()
	failed.Repository.ResolvedCommit = GolangTools().Commit
	encoded, err := Encode(failed)
	if err != nil {
		t.Fatal(err)
	}
	runs[0].Report = encoded
	runs[0].Operation = operationFor(t, encoded, 1, 1, 0)
	index, err = BuildEvidence(EvidenceInput{Runs: runs})
	if err != nil {
		t.Fatal(err)
	}
	if gate := index.AnalyticalGates[0]; gate.Outcome != outcomeFail || gate.PassingConfigurations != nil {
		t.Errorf("failed report gate = %+v", gate)
	}

	runs[0].Report = []byte("{}\n")
	if _, err := BuildEvidence(EvidenceInput{Runs: runs}); !errors.Is(err, ErrDecode) {
		t.Errorf("BuildEvidence(malformed report) error = %v, want ErrDecode", err)
	}
}

func TestEvidenceIndexRejectsInvalidContents(t *testing.T) {
	input := selectionInput(t,
		map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
		map[string]keys{unitLoose: {fail: true}},
	)
	selection, err := Select(input)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := BuildEvidence(EvidenceInput{Runs: recordedRuns(t, input), Selection: &selection})
	if err != nil {
		t.Fatal(err)
	}
	clone := func() EvidenceFields {
		fields := valid.EvidenceFields
		fields.Artifacts = slices.Clone(fields.Artifacts)
		fields.AnalyticalGates = slices.Clone(fields.AnalyticalGates)
		fields.OperationalGates = slices.Clone(fields.OperationalGates)
		fields.Lanes = slices.Clone(fields.Lanes)
		return fields
	}
	unknown := strings.Repeat("9", 40)
	run := 0
	tests := map[string]func(*EvidenceFields){
		"schema":                func(f *EvidenceFields) { f.Schema = "tacita.evidence/v2" },
		"experiment":            func(f *EvidenceFields) { f.ExperimentID = "other" },
		"artifact kind":         func(f *EvidenceFields) { f.Artifacts[0].Kind = "photo" },
		"artifact digest":       func(f *EvidenceFields) { f.Artifacts[0].Digest = "x" },
		"artifact repository":   func(f *EvidenceFields) { f.Artifacts[0].RepositoryObjectID = &unknown },
		"artifact run":          func(f *EvidenceFields) { f.Artifacts[0].Run = &run },
		"lock with a run":       func(f *EvidenceFields) { f.Artifacts[0].Kind = ArtifactDevelopmentLock },
		"artifact order":        func(f *EvidenceFields) { f.Artifacts[0], f.Artifacts[1] = f.Artifacts[1], f.Artifacts[0] },
		"analytical outcome":    func(f *EvidenceFields) { f.AnalyticalGates[0].Outcome = "great" },
		"analytical repository": func(f *EvidenceFields) { f.AnalyticalGates[0].RepositoryObjectID = unknown },
		"operational outcome":   func(f *EvidenceFields) { f.OperationalGates[0].MemoryOutcome = "inconclusive" },
		"operational run":       func(f *EvidenceFields) { f.OperationalGates[0].Run = 0 },
		"product review":        func(f *EvidenceFields) { f.ProductReviewGates = []ProductReviewGate{{}} },
		"missing lane":          func(f *EvidenceFields) { f.Lanes = f.Lanes[:1] },
		"lane reason":           func(f *EvidenceFields) { f.Lanes[0].Reason = text("tired") },
		"passing lane reason":   func(f *EvidenceFields) { f.Lanes[0].Outcome = outcomePass },
		"experiment outcome":    func(f *EvidenceFields) { f.ExperimentOutcome = outcomePass },
	}
	for name, mutate := range tests {
		fields := clone()
		mutate(&fields)
		if _, err := NewEvidenceIndex(fields); !errors.Is(err, ErrEncode) {
			t.Errorf("%s: NewEvidenceIndex() error = %v, want ErrEncode", name, err)
		}
	}

	forged := valid
	forged.EvidenceDigest = strings.Repeat("0", 64)
	if _, err := EncodeEvidenceIndex(forged); !errors.Is(err, ErrEncode) {
		t.Errorf("EncodeEvidenceIndex(forged) error = %v", err)
	}
	encoded, err := EncodeEvidenceIndex(valid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeEvidenceIndex([]byte(strings.Replace(string(encoded), `"schema"`, `"x":1,"schema"`, 1))); !errors.Is(err, ErrDecode) {
		t.Errorf("DecodeEvidenceIndex(unknown field) error = %v", err)
	}
	if _, err := DecodeEvidenceIndex([]byte(strings.Replace(string(encoded), `,"experiment_id"`, `, "experiment_id"`, 1))); !errors.Is(err, ErrDecode) {
		t.Errorf("DecodeEvidenceIndex(whitespace) error = %v", err)
	}
}

func TestSelectionSummary(t *testing.T) {
	selection := Selection{
		LaneFailures: []LaneFailure{
			{Repository: "b", Reason: LaneOperationalLimit},
			{Repository: "a", Reason: LaneNondeterministicReport},
		},
		Rejections: []Rejection{
			{ConfigurationID: "x", Repository: "a", Gate: "coverage", Outcome: "fail"},
			{ConfigurationID: "x", Repository: "b", Gate: "coverage", Outcome: "fail"},
			{ConfigurationID: "y", Repository: "a", Gate: "adjacent_jaccard", Outcome: "inconclusive"},
			{Repository: "b", Gate: "no_history_control", Outcome: "fail"},
		},
	}

	want := "no selectable configuration\n" +
		"lane_failure a nondeterministic_report\n" +
		"lane_failure b operational_limit\n" +
		"rejected_configurations 2\n" +
		"rejections adjacent_jaccard inconclusive 1\n" +
		"rejections coverage fail 2\n" +
		"rejections no_history_control fail 1\n"
	if got := selection.Summary(); got != want {
		t.Errorf("Summary() =\n%s\nwant\n%s", got, want)
	}
}
