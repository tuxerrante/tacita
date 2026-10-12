package report

import (
	"bytes"
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

	for exit, accepted := range map[int]bool{maxExitStatus: true, maxExitStatus + 1: false} {
		_, err := NewOperation(OperationFields{Schema: OperationSchema, ProcessExitStatus: exit, ReferenceRunner: "r"})
		if (err == nil) != accepted {
			t.Errorf("NewOperation(exit %d) error = %v", exit, err)
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
	if gate := index.AnalyticalGates[0]; gate.Outcome != outcomeFail || gate.PassingConfigurations == nil || *gate.PassingConfigurations != 0 {
		t.Errorf("failed report gate = %+v", gate)
	}

	runs[0].Report = []byte("{}\n")
	runs[0].Operation = operationFor(t, runs[0].Report, 1, 1, 0)
	if _, err := BuildEvidence(EvidenceInput{Runs: runs}); !errors.Is(err, ErrDecode) {
		t.Errorf("BuildEvidence(malformed report) error = %v, want ErrDecode", err)
	}
}

func TestBuildEvidenceTiesSidecarsAndReportsToTheirRuns(t *testing.T) {
	passing := map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}}
	input := selectionInput(t, passing, passing)
	tests := map[string]func([]RunRecord){
		"a sidecar for another report": func(runs []RunRecord) {
			runs[0].Operation = operationFor(t, runs[2].Report, 1, 1, 0)
		},
		"a report without a sidecar digest": func(runs []RunRecord) {
			runs[0].Operation = operationFor(t, nil, 1, 1, 0)
		},
		"a sidecar digest without a report": func(runs []RunRecord) { runs[0].Report = nil },
		"a report of another repository": func(runs []RunRecord) {
			runs[0].Report = runs[2].Report
			runs[0].Operation = operationFor(t, runs[2].Report, 1, 1, 0)
		},
	}
	for name, mutate := range tests {
		runs := recordedRuns(t, input)
		mutate(runs)
		if _, err := BuildEvidence(EvidenceInput{Runs: runs}); err == nil {
			t.Errorf("%s: BuildEvidence() succeeded", name)
		}
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
		"schema":                       func(f *EvidenceFields) { f.Schema = "tacita.evidence/v2" },
		"experiment":                   func(f *EvidenceFields) { f.ExperimentID = "other" },
		"artifact kind":                func(f *EvidenceFields) { f.Artifacts[0].Kind = "photo" },
		"artifact digest":              func(f *EvidenceFields) { f.Artifacts[0].Digest = "x" },
		"artifact repository":          func(f *EvidenceFields) { f.Artifacts[0].RepositoryObjectID = &unknown },
		"artifact run":                 func(f *EvidenceFields) { f.Artifacts[0].Run = &run },
		"lock with a run":              func(f *EvidenceFields) { f.Artifacts[0].Kind = ArtifactDevelopmentLock },
		"artifact order":               func(f *EvidenceFields) { f.Artifacts[0], f.Artifacts[1] = f.Artifacts[1], f.Artifacts[0] },
		"analytical outcome":           func(f *EvidenceFields) { f.AnalyticalGates[0].Outcome = "great" },
		"analytical repository":        func(f *EvidenceFields) { f.AnalyticalGates[0].RepositoryObjectID = unknown },
		"operational outcome":          func(f *EvidenceFields) { f.OperationalGates[0].MemoryOutcome = "inconclusive" },
		"operational run":              func(f *EvidenceFields) { f.OperationalGates[0].Run = 0 },
		"product review":               func(f *EvidenceFields) { f.ProductReviewGates = []ProductReviewGate{{}} },
		"missing lane":                 func(f *EvidenceFields) { f.Lanes = f.Lanes[:1] },
		"lane reason":                  func(f *EvidenceFields) { f.Lanes[0].Reason = text("tired") },
		"passing lane reason":          func(f *EvidenceFields) { f.Lanes[0].Outcome = outcomePass },
		"experiment outcome":           func(f *EvidenceFields) { f.ExperimentOutcome = outcomePass },
		"a missing analytical gate":    func(f *EvidenceFields) { f.AnalyticalGates = f.AnalyticalGates[1:] },
		"a duplicated analytical gate": func(f *EvidenceFields) { f.AnalyticalGates[1] = f.AnalyticalGates[0] },
		"a control with a count": func(f *EvidenceFields) {
			count := 0
			f.AnalyticalGates[4].PassingConfigurations = &count
		},
		"a development report without a count": func(f *EvidenceFields) { f.AnalyticalGates[0].PassingConfigurations = nil },
		"a missing operational gate":           func(f *EvidenceFields) { f.OperationalGates = f.OperationalGates[1:] },
		"an operational gate for a third run":  func(f *EvidenceFields) { f.OperationalGates[1].Run = 3 },
		"a third run everywhere": func(f *EvidenceFields) {
			for index := range f.Artifacts {
				if f.Artifacts[index].Run != nil && *f.Artifacts[index].Run == 2 && *f.Artifacts[index].RepositoryObjectID == Proficiency().Commit {
					third := 3
					f.Artifacts[index].Run = &third
				}
			}
			f.AnalyticalGates[7].Run, f.OperationalGates[7].Run = 3, 3
		},
		"an inconclusive analytical gate": func(f *EvidenceFields) { f.AnalyticalGates[4].Outcome = outcomeInconclusive },
		"a passing count of zero": func(f *EvidenceFields) {
			zero := 0
			f.AnalyticalGates[2].PassingConfigurations = &zero
			f.AnalyticalGates[2].Outcome = outcomePass
		},
		"a failing positive count": func(f *EvidenceFields) { f.AnalyticalGates[0].Outcome = outcomeFail },
		"a report without a sidecar": func(f *EvidenceFields) {
			f.Artifacts = slices.DeleteFunc(f.Artifacts, func(a Artifact) bool {
				return a.Kind == ArtifactOperationalSidecar && *a.Run == 1 && *a.RepositoryObjectID == GolangTools().Commit
			})
			f.OperationalGates = f.OperationalGates[1:]
		},
		"a repository without runs": func(f *EvidenceFields) {
			f.Artifacts = slices.DeleteFunc(f.Artifacts, func(a Artifact) bool {
				return a.RepositoryObjectID != nil && *a.RepositoryObjectID == Proficiency().Commit
			})
			f.AnalyticalGates, f.OperationalGates = f.AnalyticalGates[:6], f.OperationalGates[:6]
		},
		"a lock with a failed inferred lane": func(f *EvidenceFields) {
			f.Artifacts = append([]Artifact{{Kind: ArtifactDevelopmentLock, Digest: strings.Repeat("a", 64)}}, f.Artifacts...)
		},
		"an inconclusive inferred lane without a lock": func(f *EvidenceFields) {
			f.Lanes[0] = LaneOutcome{Lane: LaneInferred, Outcome: outcomeInconclusive, Reason: text(ReasonHoldoutNotRun)}
			f.ExperimentOutcome = outcomeInconclusive
		},
		"an inferred lane failing as not run": func(f *EvidenceFields) { f.Lanes[0].Reason = text(ReasonLaneNotRun) },
		"a failed profile lane":               func(f *EvidenceFields) { f.Lanes[1].Outcome = outcomeFail },
		"a profile lane awaiting the holdout": func(f *EvidenceFields) {
			f.Lanes[1].Reason = text(ReasonHoldoutNotRun)
		},
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

func TestDecodeEvidenceIndexRejectsOversizedInput(t *testing.T) {
	if _, err := DecodeEvidenceIndex(make([]byte, maxEvidenceBytes+1)); !errors.Is(err, ErrDecode) {
		t.Errorf("DecodeEvidenceIndex(oversized) error = %v, want ErrDecode", err)
	}
}

// FuzzDecodeEvidenceIndex asserts that every accepted evidence index
// re-encodes to exactly the bytes that were decoded.
func FuzzDecodeEvidenceIndex(f *testing.F) {
	report := failedReport()
	report.Repository.ResolvedCommit = GolangTools().Commit
	encodedReport, err := Encode(report)
	if err != nil {
		f.Fatal(err)
	}
	// golang/tools wrote a failed report on both runs and every other
	// repository exited without one, so the seed holds the full run plan.
	var runs []RunRecord
	for _, repository := range CalibrationCorpus() {
		for number := 1; number <= runsPerRepository; number++ {
			fields := OperationFields{Schema: OperationSchema, ElapsedDurationNS: 1, PeakResidentBytes: 1, ProcessExitStatus: 1, ReferenceRunner: "fuzz"}
			record := RunRecord{Repository: repository, Run: number}
			if repository.Commit == GolangTools().Commit {
				digest := Digest(encodedReport)
				fields.ReportDigest, fields.ProcessExitStatus, record.Report = &digest, 0, encodedReport
			}
			operation, err := NewOperation(fields)
			if err != nil {
				f.Fatal(err)
			}
			record.Operation = operation
			runs = append(runs, record)
		}
	}
	index, err := BuildEvidence(EvidenceInput{Runs: runs})
	if err != nil {
		f.Fatal(err)
	}
	encoded, err := EncodeEvidenceIndex(index)
	if err != nil {
		f.Fatal(err)
	}
	text := string(encoded)
	for _, seed := range []string{
		text,
		strings.Replace(text, `"run":1`, `"run":3`, 1),
		strings.Replace(text, `"outcome":"fail"`, `"outcome":"pass"`, 1),
		strings.Replace(text, `"passing_configurations":0`, `"passing_configurations":null`, 1),
		strings.Replace(text, `"elapsed_outcome":"pass"`, `"elapsed_outcome":"inconclusive"`, 1),
		strings.Replace(text, `"experiment_outcome":"fail"`, `"experiment_outcome":"pass"`, 1),
		strings.Replace(text, `"reason":"report_not_complete"`, `"reason":"holdout_not_run"`, 1),
		strings.Replace(text, `"reason":"not_run"`, `"reason":"holdout_not_run"`, 1),
		strings.Replace(text, `"product_review_gates":[]`, `"product_review_gates":[{}]`, 1),
		strings.Replace(text, `,"artifacts"`, `, "artifacts"`, 1),
		strings.Replace(text, `"schema"`, `"x":1,"schema"`, 1),
		strings.TrimSuffix(text, "\n"),
		"",
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		decoded, err := DecodeEvidenceIndex(data)
		if err != nil {
			return
		}
		again, err := EncodeEvidenceIndex(decoded)
		if err != nil {
			t.Fatalf("EncodeEvidenceIndex(decoded) error = %v", err)
		}
		if !bytes.Equal(again, data) {
			t.Fatalf("accepted index is not canonical:\n%q\n%q", data, again)
		}
	})
}

// FuzzDecodeOperation asserts that every accepted sidecar re-encodes to
// exactly the bytes that were decoded.
func FuzzDecodeOperation(f *testing.F) {
	digest := strings.Repeat("a", 64)
	paths := []string{"golang-tools.1.stderr.txt"}
	for _, fields := range []OperationFields{
		{Schema: OperationSchema, ReportDigest: &digest, ElapsedDurationNS: 1, PeakResidentBytes: 2, ReferenceRunner: "fuzz"},
		{Schema: OperationSchema, ProcessExitStatus: 137, ReferenceRunner: "fuzz", DiagnosticPaths: &paths},
	} {
		operation, err := NewOperation(fields)
		if err != nil {
			f.Fatal(err)
		}
		encoded, err := EncodeOperation(operation)
		if err != nil {
			f.Fatal(err)
		}
		text := string(encoded)
		for _, seed := range []string{
			text,
			strings.Replace(text, `"elapsed_duration_ns":1`, `"elapsed_duration_ns":-1`, 1),
			strings.Replace(text, `"reference_runner":"fuzz"`, `"reference_runner":""`, 1),
			strings.Replace(text, `"sidecar_digest":"`, `"sidecar_digest":"0`, 1),
			strings.Replace(text, `"schema"`, `"x":1,"schema"`, 1),
			strings.TrimSuffix(text, "\n"),
		} {
			f.Add([]byte(seed))
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		decoded, err := DecodeOperation(data)
		if err != nil {
			return
		}
		again, err := EncodeOperation(decoded)
		if err != nil {
			t.Fatalf("EncodeOperation(decoded) error = %v", err)
		}
		if !bytes.Equal(again, data) {
			t.Fatalf("accepted sidecar is not canonical:\n%q\n%q", data, again)
		}
	})
}

func TestEncodeEvidenceIndexEnforcesItsByteBudget(t *testing.T) {
	input := selectionInput(t,
		map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
		map[string]keys{unitLoose: {fail: true}},
	)
	selection, err := Select(input)
	if err != nil {
		t.Fatal(err)
	}
	index, err := BuildEvidence(EvidenceInput{Runs: recordedRuns(t, input), Selection: &selection})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeEvidenceIndex(index)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeEvidenceIndex(encoded); err != nil {
		t.Errorf("an encoded index does not round-trip: %v", err)
	}
	if len(encoded) > maxEvidenceBytes {
		t.Errorf("a recorded run's index is %d bytes, over the budget", len(encoded))
	}
}
