package report

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"testing"
)

func minimalReport() Report {
	return Report{
		Schema: ReportSchema,
		Kind:   "calibration",
		Status: "complete",
		Tool: Tool{
			TacitaVersion:      "dev",
			TacitaBinarySHA256: strings.Repeat("a", 64),
			GitVersion:         "2.55.0",
		},
		Repository: Repository{
			ObjectFormat:       "sha1",
			RootCommit:         strings.Repeat("1", 40),
			ResolvedCommit:     strings.Repeat("2", 40),
			ReachableCommits:   4,
			FirstParentEvents:  3,
			SingleParentEvents: 1,
			MergeResultEvents:  1,
		},
	}
}

func TestEncodeMinimalReport(t *testing.T) {
	encoded, err := Encode(minimalReport())
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	want := `{"schema":"tacita.report/v1","kind":"calibration","status":"complete",` +
		`"tool":{"tacita_version":"dev","tacita_binary_sha256":"` + strings.Repeat("a", 64) +
		`","git_version":"2.55.0","go_helper_version":null},` +
		`"repository":{"object_format":"sha1","root_commit":"` + strings.Repeat("1", 40) +
		`","resolved_commit":"` + strings.Repeat("2", 40) + `","reachable_commits":4,` +
		`"first_parent_events":3,"single_parent_events":1,"merge_result_events":1},` +
		`"configurations":[],"budgets":[],"partitions":[],"exclusions":[],"candidates":[],` +
		`"baselines":[],"controls":[],"profiles":[],"preregistered_checks":[],"metrics":[],` +
		`"gates":[],"limitations":[],"failure":null}` + "\n"
	if string(encoded) != want {
		t.Errorf("Encode() =\n%s\nwant\n%s", encoded, want)
	}
}

func TestDecodeRoundTripsCanonicalBytes(t *testing.T) {
	report := minimalReport()
	report.Limitations = []string{"self_attested_freeze", "sha1_only"}
	observed := int64(10)
	pass := "pass"
	report.Budgets = []Budget{
		{ID: "elapsed_time", Unit: "nanoseconds", Limit: 1_200_000_000_000},
		{ID: "first_parent_events", Unit: "events", Limit: 200_000, Observed: &observed, Outcome: &pass},
	}
	encoded, err := Encode(report)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	again, err := Encode(decoded)
	if err != nil {
		t.Fatalf("Encode(decoded) error = %v", err)
	}
	if !bytes.Equal(again, encoded) {
		t.Errorf("round trip changed bytes:\n%s\n%s", encoded, again)
	}
}

func TestDecodeRejectsNonCanonicalBytes(t *testing.T) {
	encoded, err := Encode(minimalReport())
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	text := string(encoded)

	tests := map[string]string{
		"unknown field":      strings.Replace(text, `"kind"`, `"extra":1,"kind"`, 1),
		"unknown nested":     strings.Replace(text, `"git_version"`, `"os":"linux","git_version"`, 1),
		"whitespace":         strings.Replace(text, `,"kind"`, `, "kind"`, 1),
		"missing final LF":   strings.TrimSuffix(text, "\n"),
		"second LF":          text + "\n",
		"field order":        strings.Replace(text, `"schema":"tacita.report/v1","kind":"calibration"`, `"kind":"calibration","schema":"tacita.report/v1"`, 1),
		"unknown enum value": strings.Replace(text, `"calibration"`, `"holdout"`, 1),
		"trailing value":     text + "{}",
		"profile record":     strings.Replace(text, `"profiles":[]`, `"profiles":[{"x":1}]`, 1),
		"duplicate key":      strings.Replace(text, `"status":"complete"`, `"status":"failed","status":"complete"`, 1),
	}
	for name, input := range tests {
		_, err := Decode([]byte(input))
		if !errors.Is(err, ErrDecode) && !errors.Is(err, ErrEncode) {
			t.Errorf("%s: Decode() error = %v, want a decoding or contract error", name, err)
		}
	}
}

func TestEncodeRejectsContractViolations(t *testing.T) {
	text := func(s string) *string { return &s }
	value := Decimal(0.5)
	tests := map[string]func(*Report){
		"schema":                 func(r *Report) { r.Schema = "tacita.report/v2" },
		"status":                 func(r *Report) { r.Status = "done" },
		"failed without failure": func(r *Report) { r.Status = "failed" },
		"failure on complete": func(r *Report) {
			r.Failure = &Failure{Code: "cancelled", Operation: "calibration"}
		},
		"unknown failure code": func(r *Report) {
			r.Status = "failed"
			r.Failure = &Failure{Code: "oops", Operation: "calibration"}
		},
		"binary digest":         func(r *Report) { r.Tool.TacitaBinarySHA256 = "ABC" },
		"object format":         func(r *Report) { r.Repository.ObjectFormat = "sha256" },
		"short commit":          func(r *Report) { r.Repository.RootCommit = "abc" },
		"event arithmetic":      func(r *Report) { r.Repository.MergeResultEvents = 5 },
		"reachable below chain": func(r *Report) { r.Repository.ReachableCommits = 2 },
		"configuration order": func(r *Report) {
			r.Configurations = []Configuration{
				{ConfigurationID: "b", SizeWeight: "unit"},
				{ConfigurationID: "a", SizeWeight: "unit"},
			}
		},
		"size weight": func(r *Report) {
			r.Configurations = []Configuration{{ConfigurationID: "a", SizeWeight: "heavy"}}
		},
		"budget limit": func(r *Report) {
			r.Budgets = []Budget{{ID: "report_rows", Unit: "rows", Limit: 5}}
		},
		"external budget observed": func(r *Report) {
			observed := int64(1)
			r.Budgets = []Budget{{ID: "elapsed_time", Unit: "nanoseconds", Limit: 1_200_000_000_000, Observed: &observed, Outcome: text("pass")}}
		},
		"partition percentile": func(r *Report) {
			r.Partitions = []Partition{{Percentile: 50, CutoffCommit: strings.Repeat("3", 40)}}
		},
		"exclusion order": func(r *Report) {
			r.Exclusions = []Exclusion{{Scope: "path", Reason: "vendor_path"}, {Scope: "event", Reason: "root_event"}}
		},
		"candidate identity": func(r *Report) {
			r.Candidates = []Candidate{{ID: strings.Repeat("0", 64), Origin: "inferred", Rank: 1, Antecedent: "a", Consequent: "b"}}
		},
		"metric with value and reason": func(r *Report) {
			r.Metrics = []MetricRecord{{
				ConfigurationID: "a",
				MicroAdherence:  Metric{Value: &value, Reason: text("no_candidates")},
			}}
		},
		"gate reason": func(r *Report) {
			r.Gates = []Gate{{ID: "coverage", Outcome: "inconclusive", Operator: ">=", Metric: Metric{Reason: text("sideways")}}}
		},
		"limitation order":   func(r *Report) { r.Limitations = []string{"sha1_only", "linux_amd64_only"} },
		"unknown limitation": func(r *Report) { r.Limitations = []string{"tired"} },
		"control id":         func(r *Report) { r.Controls = []Control{{ID: "fresh", Outcome: "pass"}} },
		"profile record":     func(r *Report) { r.Profiles = []Profile{{}} },
		"empty tool version": func(r *Report) { r.Tool.GitVersion = "" },
		"diagnostic digest": func(r *Report) {
			r.Status = "failed"
			r.Failure = &Failure{Code: "cancelled", Operation: "calibration", DiagnosticDigest: text("zz")}
		},
		"lock digest": func(r *Report) {
			r.Configurations = []Configuration{{ConfigurationID: "a", SizeWeight: "unit", DevelopmentLockDigest: text("x")}}
		},
		"budget order": func(r *Report) {
			r.Budgets = []Budget{
				{ID: "report_bytes", Unit: "bytes", Limit: maxReportBytes},
				{ID: "elapsed_time", Unit: "nanoseconds", Limit: 1_200_000_000_000},
			}
		},
		"budget outcome": func(r *Report) {
			observed := int64(1)
			r.Budgets = []Budget{{ID: "report_rows", Unit: "rows", Limit: maxReportRows, Observed: &observed, Outcome: text("maybe")}}
		},
		"cutoff commit": func(r *Report) { r.Partitions = []Partition{{Percentile: 60, CutoffCommit: "x"}} },
		"partition order": func(r *Report) {
			r.Partitions = []Partition{
				{Percentile: 70, CutoffCommit: strings.Repeat("3", 40)},
				{Percentile: 60, CutoffCommit: strings.Repeat("3", 40)},
			}
		},
		"exclusion reason": func(r *Report) { r.Exclusions = []Exclusion{{Scope: "event", Reason: "boredom"}} },
		"candidate origin": func(r *Report) { r.Candidates = []Candidate{{Origin: "profile"}} },
		"candidate rank": func(r *Report) {
			r.Candidates = []Candidate{{ID: CandidateID("inferred", "a", "b"), Origin: "inferred", Antecedent: "a", Consequent: "b"}}
		},
		"candidate support above opportunity": func(r *Report) {
			r.Candidates = []Candidate{{
				ID: CandidateID("inferred", "a", "b"), Origin: "inferred", Rank: 1,
				Antecedent: "a", Consequent: "b", RawSupport: 2, RawOpportunity: 1,
			}}
		},
		"candidate evidence": func(r *Report) {
			r.Candidates = []Candidate{{
				ID: CandidateID("inferred", "a", "b"), Origin: "inferred", Rank: 1, Antecedent: "a", Consequent: "b",
				Supporting: Evidence{Events: []EvidenceEvent{{}}},
			}}
		},
		"candidate order": func(r *Report) {
			r.Candidates = []Candidate{
				{ID: CandidateID("inferred", "a", "b"), Origin: "inferred", Rank: 2, Antecedent: "a", Consequent: "b"},
				{ID: CandidateID("inferred", "a", "b"), Origin: "inferred", Rank: 1, Antecedent: "a", Consequent: "b"},
			}
		},
		"baseline order": func(r *Report) {
			r.Baselines = []Baseline{
				{ID: "unweighted", ConfigurationID: text("a"), MicroAdherence: Metric{Value: &value}, MacroAdherence: Metric{Value: &value}, Coverage: Metric{Value: &value}, CutoffAbstention: Metric{Value: &value}, MinimumJaccard: Metric{Value: &value}},
				{ID: "frequent_consequent", ConfigurationID: text("a"), MicroAdherence: Metric{Value: &value}, MacroAdherence: Metric{Value: &value}, Coverage: Metric{Value: &value}, CutoffAbstention: Metric{Value: &value}, MinimumJaccard: Metric{Value: &value}},
			}
		},
		"baseline outcome": func(r *Report) {
			r.Baselines = []Baseline{{ID: "no_history", Outcome: text("great")}}
		},
		"control order": func(r *Report) {
			r.Controls = []Control{{ID: "young_repository", Outcome: "pass"}, {ID: "template_origin", Outcome: "pass"}}
		},
		"control configuration": func(r *Report) {
			r.Controls = []Control{{ID: "young_repository", Outcome: "pass", ConfigurationID: text("a")}}
		},
		"metric order": func(r *Report) {
			defined := Metric{Value: &value}
			r.Metrics = []MetricRecord{
				{ConfigurationID: "b", MicroAdherence: defined, MacroAdherence: defined, Coverage: defined, CutoffAbstention: defined, MinimumJaccard: defined},
				{ConfigurationID: "a", MicroAdherence: defined, MacroAdherence: defined, Coverage: defined, CutoffAbstention: defined, MinimumJaccard: defined},
			}
		},
		"non-finite metric": func(r *Report) {
			infinite := Decimal(math.Inf(1))
			r.Gates = []Gate{{ID: "coverage", Outcome: "pass", Operator: ">=", Metric: Metric{Value: &infinite}}}
		},
		"gate order": func(r *Report) {
			defined := Metric{Value: &value}
			r.Gates = []Gate{
				{ID: "coverage", Outcome: "pass", Operator: ">=", Metric: defined},
				{ID: "adjacent_jaccard", Outcome: "pass", Operator: ">=", Metric: defined},
			}
		},
		"gate reason value": func(r *Report) {
			r.Gates = []Gate{{ID: "coverage", Outcome: "pass", Operator: ">=", Metric: Metric{Value: &value}, Reason: text("tired")}}
		},
		"baseline id": func(r *Report) {
			r.Baselines = []Baseline{{ID: "random", MicroAdherence: Metric{Value: &value}}}
		},
	}

	for name, mutate := range tests {
		report := minimalReport()
		mutate(&report)
		if _, err := Encode(report); !errors.Is(err, ErrEncode) {
			t.Errorf("%s: Encode() error = %v, want ErrEncode", name, err)
		}
	}
}

func TestEncodeEnforcesTheRowAndByteBudgets(t *testing.T) {
	report := minimalReport()
	report.Limitations = []string{"self_attested_freeze", "sha1_only"}

	_, err := encode(report, 1, maxReportBytes)
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Code != "budget_report_rows" || limit.Observed != 2 || limit.Limit != 1 {
		t.Errorf("encode(row limit 1) error = %v, want budget_report_rows 2 of 1", err)
	}

	_, err = encode(report, maxReportRows, 100)
	if !errors.As(err, &limit) || limit.Code != "budget_report_bytes" || limit.Limit != 100 || !errors.Is(err, ErrLimit) {
		t.Errorf("encode(byte limit 100) error = %v, want budget_report_bytes", err)
	}
}

func TestCountRows(t *testing.T) {
	report := minimalReport()
	report.Partitions = make([]Partition, 4)
	report.Gates = make([]Gate, 3)
	report.Candidates = []Candidate{{
		Supporting: Evidence{Events: make([]EvidenceEvent, 2)},
		Exceptions: Evidence{Events: make([]EvidenceEvent, 1)},
	}}
	report.Failure = &Failure{}

	if got, want := CountRows(report), 4+3+1+2+1+1; got != want {
		t.Errorf("CountRows() = %d, want %d", got, want)
	}
}

func TestEvidenceValidation(t *testing.T) {
	event := func(index int, id string) EvidenceEvent {
		return EvidenceEvent{
			ObjectID: id, Kind: "single-parent", Index: index, TotalPaths: 2,
			Paths: []EvidencePath{{Path: "a/x", Status: "M"}, {Path: "b/y", Status: "A"}},
		}
	}
	first, second := strings.Repeat("a", 40), strings.Repeat("b", 40)

	valid := Evidence{TotalEvents: 30, Events: []EvidenceEvent{event(1, first), event(2, second)}}
	if err := validateEvidence(valid); err != nil {
		t.Fatalf("validateEvidence(valid) error = %v", err)
	}

	tests := map[string]Evidence{
		"more than twenty":    {TotalEvents: 30, Events: make([]EvidenceEvent, 21)},
		"more than the total": {TotalEvents: 1, Events: []EvidenceEvent{event(1, first), event(2, second)}},
		"out of index order":  {TotalEvents: 5, Events: []EvidenceEvent{event(2, first), event(1, second)}},
		"unknown status": {TotalEvents: 1, Events: []EvidenceEvent{{
			ObjectID: first, Kind: "root", Index: 1, TotalPaths: 1, Paths: []EvidencePath{{Path: "a", Status: "R"}},
		}}},
		"paths out of order": {TotalEvents: 1, Events: []EvidenceEvent{{
			ObjectID: first, Kind: "root", Index: 1, TotalPaths: 2,
			Paths: []EvidencePath{{Path: "b", Status: "A"}, {Path: "a", Status: "A"}},
		}}},
	}
	for name, evidence := range tests {
		if err := validateEvidence(evidence); !errors.Is(err, ErrEncode) {
			t.Errorf("%s: validateEvidence() error = %v, want ErrEncode", name, err)
		}
	}
}

func TestCompareOptional(t *testing.T) {
	a, b := "a", "b"
	tests := []struct {
		left, right *string
		want        int
	}{
		{nil, nil, 0}, {nil, &a, -1}, {&a, nil, 1}, {&a, &b, -1}, {&b, &a, 1}, {&a, &a, 0},
	}
	for _, tt := range tests {
		if got := compareOptional(tt.left, tt.right); got != tt.want {
			t.Errorf("compareOptional(%v, %v) = %d, want %d", tt.left, tt.right, got, tt.want)
		}
	}
}
