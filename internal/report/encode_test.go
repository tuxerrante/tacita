package report

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/tuxerrante/tacita/internal/mining"
)

// failedReport is a calibration that stopped before evaluating anything.
func failedReport() Report {
	return Report{
		Schema: ReportSchema,
		Kind:   kindCalibration,
		Status: statusFailed,
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
		Failure: &Failure{Code: "cancelled", Operation: "calibration"},
	}
}

func configurationRecord(configuration mining.Configuration) Configuration {
	return Configuration{
		ConfigurationID:      configuration.ID(),
		SizeWeight:           string(configuration.SizeWeight),
		MinimumOpportunities: configuration.MinimumOpportunities,
		MinimumConfidence:    Decimal(configuration.MinimumConfidence),
		MinimumLift:          Decimal(configuration.MinimumLift),
	}
}

func reasonMetric(reason string) Metric {
	return Metric{Reason: &reason}
}

// completeReport is a complete calibration of a ten-event chain, too short to
// infer from, so every configuration abstains at every cutoff.
func completeReport() Report {
	report := failedReport()
	report.Status = statusComplete
	report.Failure = nil
	report.Repository.ReachableCommits = 10
	report.Repository.FirstParentEvents = 10
	report.Repository.SingleParentEvents = 9
	report.Repository.MergeResultEvents = 0
	for index, bounds := range [][2]int{{6, 7}, {7, 8}, {8, 9}, {9, 10}} {
		report.Partitions = append(report.Partitions, Partition{
			Percentile:           cutoffPercentiles[index],
			CutoffCommit:         strings.Repeat("3", 40),
			TrainingFirstEvent:   1,
			TrainingLastEvent:    bounds[0],
			EvaluationFirstEvent: bounds[0] + 1,
			EvaluationLastEvent:  bounds[1],
		})
	}
	floor := reasonMetric("below_inference_floor")
	for _, configuration := range mining.AllConfigurations() {
		report.Configurations = append(report.Configurations, configurationRecord(configuration))
		report.Metrics = append(report.Metrics, MetricRecord{
			ConfigurationID:  configuration.ID(),
			MicroAdherence:   floor,
			MacroAdherence:   floor,
			Coverage:         floor,
			CutoffAbstention: floor,
			MinimumJaccard:   floor,
		})
	}
	return report
}

// withCandidate adds one consistent candidate at partition 60.
func withCandidate(report *Report, candidate Candidate) {
	candidate.Partition = 60
	report.Candidates = append(report.Candidates, candidate)
	report.Partitions[0].DisplayedCandidates++
}

func validCandidate() Candidate {
	return Candidate{
		ID:              CandidateID("inferred", "a", "b"),
		Origin:          "inferred",
		ConfigurationID: mining.AllConfigurations()[0].ID(),
		Rank:            1,
		Antecedent:      "a",
		Consequent:      "b",
		RawOpportunity:  5,
		RawSupport:      4,
		Supporting:      Evidence{TotalEvents: 4},
		Exceptions:      Evidence{TotalEvents: 1},
	}
}

func TestEncodeFailedReport(t *testing.T) {
	encoded, err := Encode(failedReport())
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	want := `{"schema":"tacita.report/v1","kind":"calibration","status":"failed",` +
		`"tool":{"tacita_version":"dev","tacita_binary_sha256":"` + strings.Repeat("a", 64) +
		`","git_version":"2.55.0","go_helper_version":null},` +
		`"repository":{"object_format":"sha1","root_commit":"` + strings.Repeat("1", 40) +
		`","resolved_commit":"` + strings.Repeat("2", 40) + `","reachable_commits":4,` +
		`"first_parent_events":3,"single_parent_events":1,"merge_result_events":1},` +
		`"configurations":[],"budgets":[],"partitions":[],"exclusions":[],"candidates":[],` +
		`"baselines":[],"controls":[],"profiles":[],"preregistered_checks":[],"metrics":[],` +
		`"gates":[],"limitations":[],"failure":{"code":"cancelled","operation":"calibration",` +
		`"observed":null,"limit":null,"diagnostic_digest":null}}` + "\n"
	if string(encoded) != want {
		t.Errorf("Encode() =\n%s\nwant\n%s", encoded, want)
	}
}

func TestDecodeRoundTripsCanonicalBytes(t *testing.T) {
	report := completeReport()
	report.Limitations = []string{"self_attested_freeze", "sha1_only"}
	observed := int64(10)
	pass := outcomePass
	report.Budgets = []Budget{
		{ID: "elapsed_time", Unit: "nanoseconds", Limit: 1_200_000_000_000},
		{ID: "first_parent_events", Unit: "events", Limit: 200_000, Observed: &observed, Outcome: &pass},
	}
	withCandidate(&report, validCandidate())
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
	encoded, err := Encode(failedReport())
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
		"empty profile":      strings.Replace(text, `"profiles":[]`, `"profiles":[{}]`, 1),
		"duplicate key":      strings.Replace(text, `"status":"failed"`, `"status":"complete","status":"failed"`, 1),
	}
	for name, input := range tests {
		_, err := Decode([]byte(input))
		if !errors.Is(err, ErrDecode) {
			t.Errorf("%s: Decode() error = %v, want ErrDecode", name, err)
		}
	}
}

func TestDecodeKeepsTheBudgetCategory(t *testing.T) {
	report := failedReport()
	report.Limitations = []string{"self_attested_freeze", "sha1_only"}

	err := asDecodeError(&LimitError{Code: "budget_report_rows", Observed: 2, Limit: 1})

	var limit *LimitError
	if !errors.As(err, &limit) || errors.Is(err, ErrDecode) {
		t.Errorf("asDecodeError(limit) = %v, want the LimitError unchanged", err)
	}
}

func TestEncodeRejectsContractViolations(t *testing.T) {
	text := func(s string) *string { return &s }
	value := Decimal(0.5)
	defined := Metric{Value: &value}
	tests := map[string]func(*Report){
		"schema":                 func(r *Report) { r.Schema = "tacita.report/v2" },
		"status":                 func(r *Report) { r.Status = "done" },
		"failed without failure": func(r *Report) { r.Status = statusFailed },
		"failure on complete": func(r *Report) {
			r.Failure = &Failure{Code: "cancelled", Operation: "calibration"}
		},
		"unknown failure code": func(r *Report) {
			r.Status = statusFailed
			r.Failure = &Failure{Code: "oops", Operation: "calibration"}
		},
		"diagnostic digest": func(r *Report) {
			r.Status = statusFailed
			r.Failure = &Failure{Code: "cancelled", Operation: "calibration", DiagnosticDigest: text("zz")}
		},
		"empty tool version":    func(r *Report) { r.Tool.GitVersion = "" },
		"binary digest":         func(r *Report) { r.Tool.TacitaBinarySHA256 = "ABC" },
		"object format":         func(r *Report) { r.Repository.ObjectFormat = "sha256" },
		"short commit":          func(r *Report) { r.Repository.RootCommit = "abc" },
		"event arithmetic":      func(r *Report) { r.Repository.MergeResultEvents = 5 },
		"reachable below chain": func(r *Report) { r.Repository.ReachableCommits = 2 },
		"profile record":        func(r *Report) { r.Profiles = []Profile{{}} },
		"missing configuration": func(r *Report) {
			r.Configurations = r.Configurations[1:]
			r.Metrics = r.Metrics[1:]
		},
		"missing partition": func(r *Report) { r.Partitions = r.Partitions[:3] },
		"missing metric":    func(r *Report) { r.Metrics = r.Metrics[1:] },
		"misaligned metric": func(r *Report) {
			r.Metrics[0], r.Metrics[1] = r.Metrics[1], r.Metrics[0]
		},
		"configuration order": func(r *Report) {
			r.Configurations[0], r.Configurations[1] = r.Configurations[1], r.Configurations[0]
		},
		"threshold not frozen": func(r *Report) { r.Configurations[0].MinimumOpportunities = 0 },
		"identity not frozen":  func(r *Report) { r.Configurations[0].ConfigurationID = "weight=unit" },
		"lock digest": func(r *Report) {
			r.Configurations[0].DevelopmentLockDigest = text("x")
		},
		"budget limit": func(r *Report) {
			r.Budgets = []Budget{{ID: "report_rows", Unit: "rows", Limit: 5}}
		},
		"external budget observed": func(r *Report) {
			observed := int64(1)
			r.Budgets = []Budget{{ID: "elapsed_time", Unit: "nanoseconds", Limit: 1_200_000_000_000, Observed: &observed, Outcome: text(outcomePass)}}
		},
		"budget passing over its limit": func(r *Report) {
			observed := int64(300_000)
			r.Budgets = []Budget{{ID: "first_parent_events", Unit: "events", Limit: 200_000, Observed: &observed, Outcome: text(outcomePass)}}
		},
		"budget failing under its limit": func(r *Report) {
			observed := int64(3)
			r.Budgets = []Budget{{ID: "first_parent_events", Unit: "events", Limit: 200_000, Observed: &observed, Outcome: text(outcomeFail)}}
		},
		"negative observation": func(r *Report) {
			observed := int64(-1)
			r.Budgets = []Budget{{ID: "first_parent_events", Unit: "events", Limit: 200_000, Observed: &observed, Outcome: text(outcomePass)}}
		},
		"budget order": func(r *Report) {
			r.Budgets = []Budget{
				{ID: "report_bytes", Unit: "bytes", Limit: maxReportBytes},
				{ID: "elapsed_time", Unit: "nanoseconds", Limit: 1_200_000_000_000},
			}
		},
		"partition percentile": func(r *Report) { r.Partitions[0].Percentile = 50 },
		"partition order": func(r *Report) {
			r.Partitions[0], r.Partitions[1] = r.Partitions[1], r.Partitions[0]
		},
		"cutoff commit":          func(r *Report) { r.Partitions[0].CutoffCommit = "x" },
		"training not from one":  func(r *Report) { r.Partitions[0].TrainingFirstEvent = 0 },
		"empty training":         func(r *Report) { r.Partitions[0].TrainingLastEvent = 0 },
		"evaluation gap":         func(r *Report) { r.Partitions[1].EvaluationFirstEvent = 9 },
		"reversed evaluation":    func(r *Report) { r.Partitions[3].EvaluationLastEvent = 8 },
		"window past the chain":  func(r *Report) { r.Partitions[3].EvaluationLastEvent = 11 },
		"windows not contiguous": func(r *Report) { r.Partitions[0].EvaluationLastEvent = 6 },
		"last window short": func(r *Report) {
			r.Repository.FirstParentEvents, r.Repository.SingleParentEvents, r.Repository.ReachableCommits = 11, 10, 11
		},
		"negative eligible count":  func(r *Report) { r.Partitions[0].EligibleTrainingTransactions = -1 },
		"eligible above the range": func(r *Report) { r.Partitions[0].EligibleEvaluationTransactions = 2 },
		"displayed count":          func(r *Report) { r.Partitions[0].DisplayedCandidates = 1 },
		"candidate at no partition": func(r *Report) {
			withCandidate(r, validCandidate())
			r.Candidates[0].Partition = 100
		},
		"exclusion order": func(r *Report) {
			r.Exclusions = []Exclusion{{Scope: "path", Reason: "vendor_path"}, {Scope: "event", Reason: "root_event"}}
		},
		"exclusion reason": func(r *Report) { r.Exclusions = []Exclusion{{Scope: "event", Reason: "boredom"}} },
		"candidate origin": func(r *Report) {
			candidate := validCandidate()
			candidate.Origin = "profile"
			withCandidate(r, candidate)
		},
		"candidate identity": func(r *Report) {
			candidate := validCandidate()
			candidate.ID = strings.Repeat("0", 64)
			withCandidate(r, candidate)
		},
		"candidate rank": func(r *Report) {
			candidate := validCandidate()
			candidate.Rank = 0
			withCandidate(r, candidate)
		},
		"candidate support above opportunity": func(r *Report) {
			candidate := validCandidate()
			candidate.RawSupport, candidate.RawOpportunity = 2, 1
			withCandidate(r, candidate)
		},
		"supporting total": func(r *Report) {
			candidate := validCandidate()
			candidate.Supporting.TotalEvents = 3
			withCandidate(r, candidate)
		},
		"exception total": func(r *Report) {
			candidate := validCandidate()
			candidate.Exceptions.TotalEvents = 5
			withCandidate(r, candidate)
		},
		"candidate evidence": func(r *Report) {
			candidate := validCandidate()
			candidate.Supporting.Events = []EvidenceEvent{{}}
			withCandidate(r, candidate)
		},
		"candidate order": func(r *Report) {
			second := validCandidate()
			second.Rank = 2
			withCandidate(r, second)
			withCandidate(r, validCandidate())
		},
		"metric with value and reason": func(r *Report) {
			r.Metrics[0].MicroAdherence = Metric{Value: &value, Reason: text("no_candidates")}
		},
		"metric order": func(r *Report) {
			r.Kind = "backtest"
			r.Metrics[0], r.Metrics[1] = r.Metrics[1], r.Metrics[0]
		},
		"non-finite metric": func(r *Report) {
			infinite := Decimal(math.Inf(1))
			r.Gates = []Gate{{ID: "coverage", Outcome: outcomePass, Operator: ">=", Metric: Metric{Value: &infinite}}}
		},
		"gate reason": func(r *Report) {
			r.Gates = []Gate{{ID: "coverage", Outcome: "inconclusive", Operator: ">=", Metric: Metric{Reason: text("sideways")}}}
		},
		"gate reason value": func(r *Report) {
			r.Gates = []Gate{{ID: "coverage", Outcome: outcomePass, Operator: ">=", Metric: defined, Reason: text("tired")}}
		},
		"gate order": func(r *Report) {
			r.Gates = []Gate{
				{ID: "coverage", Outcome: outcomePass, Operator: ">=", Metric: defined},
				{ID: "adjacent_jaccard", Outcome: outcomePass, Operator: ">=", Metric: defined},
			}
		},
		"limitation order":   func(r *Report) { r.Limitations = []string{"sha1_only", "linux_amd64_only"} },
		"unknown limitation": func(r *Report) { r.Limitations = []string{"tired"} },
		"control id":         func(r *Report) { r.Controls = []Control{{ID: "fresh", Outcome: outcomePass}} },
		"control order": func(r *Report) {
			r.Controls = []Control{{ID: "young_repository", Outcome: outcomePass}, {ID: "template_origin", Outcome: outcomePass}}
		},
		"control configuration": func(r *Report) {
			r.Controls = []Control{{ID: "young_repository", Outcome: outcomePass, ConfigurationID: text("a")}}
		},
		"baseline id": func(r *Report) {
			r.Baselines = []Baseline{{ID: "random", MicroAdherence: defined}}
		},
		"baseline outcome": func(r *Report) {
			r.Baselines = []Baseline{{ID: "no_history", Outcome: text("great")}}
		},
		"baseline order": func(r *Report) {
			r.Baselines = []Baseline{
				{ID: "unweighted", ConfigurationID: text("a"), MicroAdherence: defined, MacroAdherence: defined, Coverage: defined, CutoffAbstention: defined, MinimumJaccard: defined},
				{ID: "frequent_consequent", ConfigurationID: text("a"), MicroAdherence: defined, MacroAdherence: defined, Coverage: defined, CutoffAbstention: defined, MinimumJaccard: defined},
			}
		},
	}

	for name, mutate := range tests {
		report := completeReport()
		mutate(&report)
		if _, err := Encode(report); !errors.Is(err, ErrEncode) {
			t.Errorf("%s: Encode() error = %v, want ErrEncode", name, err)
		}
	}
}

func TestEncodeAcceptsTheCompleteFixture(t *testing.T) {
	if _, err := Encode(completeReport()); err != nil {
		t.Fatalf("Encode(completeReport()) error = %v", err)
	}
}

func TestEncodeEnforcesTheRowAndByteBudgets(t *testing.T) {
	report := failedReport()
	report.Limitations = []string{"self_attested_freeze", "sha1_only"}

	_, err := encode(report, 1, maxReportBytes)
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Code != "budget_report_rows" || limit.Observed != 3 || limit.Limit != 1 {
		t.Errorf("encode(row limit 1) error = %v, want budget_report_rows 3 of 1", err)
	}

	_, err = encode(report, maxReportRows, 100)
	if !errors.As(err, &limit) || limit.Code != "budget_report_bytes" || limit.Limit != 100 || !errors.Is(err, ErrLimit) {
		t.Errorf("encode(byte limit 100) error = %v, want budget_report_bytes", err)
	}
}

func TestCountRows(t *testing.T) {
	report := failedReport()
	report.Partitions = make([]Partition, 4)
	report.Gates = make([]Gate, 3)
	report.Candidates = []Candidate{{
		Supporting: Evidence{Events: make([]EvidenceEvent, 2)},
		Exceptions: Evidence{Events: make([]EvidenceEvent, 1)},
	}}

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

func TestFrozenBudgetsIsDetached(t *testing.T) {
	budgets := FrozenBudgets()
	budgets[0].Limit = 1

	if FrozenBudgets()[0].Limit == 1 {
		t.Error("changing the returned budgets changed the frozen table")
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
