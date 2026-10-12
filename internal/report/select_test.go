package report

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
)

// keys are one configuration's development outcomes on one repository.
type keys struct {
	gain, coverage, jaccard float64
	fail                    bool
}

func decimalMetric(value float64) Metric {
	decimal := Decimal(value)
	return Metric{Value: &decimal}
}

// selectionReport builds a calibration report for commit whose
// configurations have the given selector keys; configurations not listed fail.
func selectionReport(t *testing.T, commit string, outcomes map[string]keys) []byte {
	t.Helper()

	report := completeReport()
	report.Repository.ResolvedCommit = commit
	pass := outcomePass
	report.Gates = []Gate{{
		ID: "no_history_control", Outcome: pass, Metric: decimalMetric(0), Operator: "<=", Threshold: 0,
	}}
	report.Metrics = nil
	for _, record := range report.Configurations {
		outcome, listed := outcomes[record.ConfigurationID]
		gateOutcome := outcomePass
		if !listed || outcome.fail {
			gateOutcome = outcomeFail
		}
		id := record.ConfigurationID
		report.Gates = append(report.Gates,
			Gate{ConfigurationID: &id, ID: "coverage", Outcome: gateOutcome, Metric: decimalMetric(outcome.coverage), Operator: ">=", Threshold: 0.05},
			Gate{ConfigurationID: &id, ID: "frequent_consequent_gain", Outcome: outcomePass, Metric: decimalMetric(outcome.gain), Operator: ">=", Threshold: 0.10},
		)
		report.Metrics = append(report.Metrics, MetricRecord{
			ConfigurationID:  id,
			MicroAdherence:   decimalMetric(0.9),
			MacroAdherence:   decimalMetric(0.9),
			Coverage:         decimalMetric(outcome.coverage),
			CutoffAbstention: decimalMetric(0),
			MinimumJaccard:   decimalMetric(outcome.jaccard),
		})
	}
	slices.SortFunc(report.Gates, func(left, right Gate) int {
		if c := strings.Compare(left.ID, right.ID); c != 0 {
			return c
		}
		return compareOptional(left.ConfigurationID, right.ConfigurationID)
	})
	encoded, err := Encode(report)
	if err != nil {
		t.Fatalf("Encode(selection report) error = %v", err)
	}
	return encoded
}

func controlReport(t *testing.T, repository CorpusRepository, outcome string) []byte {
	t.Helper()

	report := completeReport()
	report.Repository.ResolvedCommit = repository.Commit
	id := "young_repository"
	if repository.Role == RoleTemplateControl {
		id = "template_origin"
	}
	report.Controls = []Control{{ID: id, RootEventExcluded: true, Outcome: outcome}}
	encoded, err := Encode(report)
	if err != nil {
		t.Fatalf("Encode(control report) error = %v", err)
	}
	return encoded
}

func selectionInput(t *testing.T, tools, kubernetes map[string]keys) SelectionInput {
	t.Helper()

	toolsReport := selectionReport(t, GolangTools.Commit, tools)
	kubernetesReport := selectionReport(t, Kubernetes.Commit, kubernetes)
	return SelectionInput{
		Development: [2]DevelopmentRun{
			{Report: toolsReport, Repeat: toolsReport, OperationalPass: true},
			{Report: kubernetesReport, Repeat: kubernetesReport, OperationalPass: true},
		},
		Controls: [2][]byte{
			controlReport(t, GoStarter, outcomePass),
			controlReport(t, Proficiency, outcomePass),
		},
	}
}

const (
	unitLoose    = "weight=unit;opportunities=5;confidence=0.70;lift=1.25"
	unitStrict   = "weight=unit;opportunities=20;confidence=0.90;lift=2.00"
	inverseLoose = "weight=inverse-component;opportunities=5;confidence=0.70;lift=1.25"
	pairLoose    = "weight=pair-normalized;opportunities=5;confidence=0.70;lift=1.25"
	unitTenLoose = "weight=unit;opportunities=10;confidence=0.70;lift=1.25"
)

func TestSelectOrdersTheFrozenKeys(t *testing.T) {
	tests := []struct {
		name       string
		tools      map[string]keys
		kubernetes map[string]keys
		want       string
	}{
		{
			name:       "worst-case gain decides",
			tools:      map[string]keys{unitLoose: {gain: 0.5, coverage: 0.1, jaccard: 0.6}, inverseLoose: {gain: 0.3, coverage: 0.9, jaccard: 0.9}},
			kubernetes: map[string]keys{unitLoose: {gain: 0.2, coverage: 0.1, jaccard: 0.6}, inverseLoose: {gain: 0.3, coverage: 0.9, jaccard: 0.9}},
			want:       inverseLoose,
		},
		{
			name:       "then worst-case coverage",
			tools:      map[string]keys{unitLoose: {gain: 0.3, coverage: 0.08, jaccard: 0.9}, pairLoose: {gain: 0.3, coverage: 0.2, jaccard: 0.5}},
			kubernetes: map[string]keys{unitLoose: {gain: 0.3, coverage: 0.3, jaccard: 0.9}, pairLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.5}},
			want:       pairLoose,
		},
		{
			name:       "then minimum Jaccard",
			tools:      map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}, inverseLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.7}},
			kubernetes: map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}, inverseLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.7}},
			want:       inverseLoose,
		},
		{
			name:       "then the simplest weight",
			tools:      map[string]keys{pairLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}, unitStrict: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
			kubernetes: map[string]keys{pairLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}, unitStrict: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
			want:       unitStrict,
		},
		{
			name:       "then the lowest thresholds",
			tools:      map[string]keys{unitTenLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}, unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
			kubernetes: map[string]keys{unitTenLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}, unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
			want:       unitLoose,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection, err := Select(selectionInput(t, tt.tools, tt.kubernetes))
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if selection.Lock == nil {
				t.Fatalf("Select() produced no lock: %+v", selection.LaneFailures)
			}
			if selection.Lock.ConfigurationID != tt.want {
				t.Errorf("selected %s, want %s", selection.Lock.ConfigurationID, tt.want)
			}
		})
	}
}

func TestSelectWritesTheLock(t *testing.T) {
	input := selectionInput(t,
		map[string]keys{inverseLoose: {gain: 0.25, coverage: 0.12, jaccard: 0.75}},
		map[string]keys{inverseLoose: {gain: 0.2, coverage: 0.07, jaccard: 0.5}},
	)

	selection, err := Select(input)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	lock := selection.Lock
	if lock == nil {
		t.Fatal("Select() produced no lock")
	}

	grid, err := FrozenGrid()
	if err != nil {
		t.Fatalf("FrozenGrid() error = %v", err)
	}
	if !slices.Equal(lock.Corpus, []string{GolangTools.Commit, Kubernetes.Commit, GoStarter.Commit, Proficiency.Commit}) {
		t.Errorf("corpus = %v", lock.Corpus)
	}
	if lock.GridDigest != grid.GridDigest || lock.Selector != SelectorVersion || lock.Environment.Architecture != "linux/amd64" {
		t.Errorf("lock header = %+v", lock.LockFields)
	}
	if lock.Parameters != (Parameters{SizeWeight: "inverse-component", MinimumOpportunities: 5, MinimumConfidence: 0.70, MinimumLift: 1.25}) {
		t.Errorf("parameters = %+v", lock.Parameters)
	}
	wantDigests := []ReportDigest{
		{RepositoryObjectID: GolangTools.Commit, ReportDigest: Digest(input.Development[0].Report)},
		{RepositoryObjectID: Kubernetes.Commit, ReportDigest: Digest(input.Development[1].Report)},
	}
	if !slices.Equal(lock.DevelopmentReports, wantDigests) {
		t.Errorf("development reports = %+v, want %+v", lock.DevelopmentReports, wantDigests)
	}
	wantMetrics := []SelectorMetric{
		{Metric: MetricWorstCaseFrequentConsequentGain, Value: 0.2},
		{Metric: MetricWorstCaseCoverage, Value: 0.07},
		{Metric: MetricMinimumAdjacentJaccard, Value: 0.5},
	}
	if !slices.Equal(lock.SelectorMetrics, wantMetrics) {
		t.Errorf("selector metrics = %+v, want %+v", lock.SelectorMetrics, wantMetrics)
	}

	encoded, err := EncodeLock(*lock)
	if err != nil {
		t.Fatalf("EncodeLock() error = %v", err)
	}
	decoded, err := DecodeLock(encoded)
	if err != nil {
		t.Fatalf("DecodeLock() error = %v", err)
	}
	if decoded.LockDigest != lock.LockDigest {
		t.Errorf("round trip changed the digest")
	}
	if !strings.HasPrefix(string(encoded), `{"schema":"tacita.dev-lock/v1","corpus":[`) ||
		!strings.HasSuffix(string(encoded), `,"lock_digest":"`+lock.LockDigest+"\"}\n") {
		t.Errorf("lock bytes = %s", encoded)
	}
}

func TestSelectRecordsRejections(t *testing.T) {
	input := selectionInput(t,
		map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
		map[string]keys{unitLoose: {fail: true}},
	)

	selection, err := Select(input)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}

	if selection.Lock != nil {
		t.Fatalf("Select() selected %s although it failed on Kubernetes", selection.Lock.ConfigurationID)
	}
	if len(selection.LaneFailures) != 0 {
		t.Errorf("lane failures = %+v, want none", selection.LaneFailures)
	}
	want := Rejection{ConfigurationID: unitLoose, Repository: Kubernetes.Commit, Gate: "coverage", Outcome: outcomeFail}
	if !slices.Contains(selection.Rejections, want) {
		t.Errorf("rejections do not contain %+v", want)
	}
	if got := len(selection.Rejections); got != 81+80 {
		t.Errorf("rejections = %d, want every failing coverage gate (161)", got)
	}
}

func TestSelectFailsTheLane(t *testing.T) {
	passing := map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}}
	tests := []struct {
		name   string
		mutate func(*testing.T, *SelectionInput)
		want   LaneFailure
	}{
		{
			name: "a repeated run differs",
			mutate: func(t *testing.T, input *SelectionInput) {
				input.Development[1].Repeat = selectionReport(t, Kubernetes.Commit, map[string]keys{})
			},
			want: LaneFailure{Repository: Kubernetes.Commit, Reason: LaneNondeterministicReport},
		},
		{
			name:   "an operational budget",
			mutate: func(_ *testing.T, input *SelectionInput) { input.Development[0].OperationalPass = false },
			want:   LaneFailure{Repository: GolangTools.Commit, Reason: LaneOperationalLimit},
		},
		{
			name: "a failing control",
			mutate: func(t *testing.T, input *SelectionInput) {
				input.Controls[1] = controlReport(t, Proficiency, outcomeFail)
			},
			want: LaneFailure{Repository: Proficiency.Commit, Reason: LaneControlFailed},
		},
		{
			name: "a control report without a control record",
			mutate: func(t *testing.T, input *SelectionInput) {
				report := completeReport()
				report.Repository.ResolvedCommit = GoStarter.Commit
				encoded, err := Encode(report)
				if err != nil {
					t.Fatalf("Encode() error = %v", err)
				}
				input.Controls[0] = encoded
			},
			want: LaneFailure{Repository: GoStarter.Commit, Reason: LaneControlFailed},
		},
		{
			name: "a failed development report",
			mutate: func(t *testing.T, input *SelectionInput) {
				report := failedReport()
				report.Repository.ResolvedCommit = GolangTools.Commit
				report.Failure = &Failure{Code: "budget_unique_pairs", Operation: "calibration"}
				encoded, err := Encode(report)
				if err != nil {
					t.Fatalf("Encode() error = %v", err)
				}
				input.Development[0] = DevelopmentRun{Report: encoded, Repeat: encoded, OperationalPass: true}
			},
			want: LaneFailure{Repository: GolangTools.Commit, Reason: LaneReportNotComplete},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := selectionInput(t, passing, passing)
			tt.mutate(t, &input)

			selection, err := Select(input)
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if selection.Lock != nil {
				t.Errorf("Select() wrote a lock despite %s", tt.name)
			}
			if !slices.Contains(selection.LaneFailures, tt.want) {
				t.Errorf("lane failures = %+v, want %+v", selection.LaneFailures, tt.want)
			}
		})
	}
}

func TestSelectRequiresRepositoryGatesToPass(t *testing.T) {
	input := selectionInput(t,
		map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
		map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
	)
	report, err := Decode(input.Development[1].Report)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	report.Gates[slices.IndexFunc(report.Gates, func(g Gate) bool { return g.ID == "no_history_control" })].Outcome = outcomeFail
	encoded, err := Encode(report)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	input.Development[1] = DevelopmentRun{Report: encoded, Repeat: encoded, OperationalPass: true}

	selection, err := Select(input)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if selection.Lock != nil {
		t.Error("Select() wrote a lock although the no-history control failed")
	}
	want := Rejection{Repository: Kubernetes.Commit, Gate: "no_history_control", Outcome: outcomeFail}
	if !slices.Contains(selection.Rejections, want) {
		t.Errorf("rejections do not contain %+v", want)
	}
}

func TestSelectRejectsInvalidEvidence(t *testing.T) {
	passing := map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}}
	tests := map[string]func(*testing.T, *SelectionInput){
		"swapped development reports": func(_ *testing.T, input *SelectionInput) {
			input.Development[0], input.Development[1] = input.Development[1], input.Development[0]
		},
		"a holdout or unknown commit": func(t *testing.T, input *SelectionInput) {
			input.Controls[1] = controlReport(t, CorpusRepository{Name: "other", Commit: strings.Repeat("9", 40)}, outcomePass)
		},
		"malformed bytes": func(_ *testing.T, input *SelectionInput) {
			input.Development[0].Report = []byte("{}\n")
		},
		"a different binary": func(t *testing.T, input *SelectionInput) {
			report, err := Decode(input.Controls[0])
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			report.Tool.TacitaBinarySHA256 = strings.Repeat("b", 64)
			input.Controls[0], err = Encode(report)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
		},
		"a backtest report": func(t *testing.T, input *SelectionInput) {
			report, err := Decode(input.Controls[0])
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			report.Kind = "backtest"
			input.Controls[0], err = Encode(report)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
		},
	}

	for name, mutate := range tests {
		input := selectionInput(t, passing, passing)
		mutate(t, &input)
		if _, err := Select(input); err == nil {
			t.Errorf("%s: Select() succeeded, want an error", name)
		}
	}
}

func TestDecodeLockRejectsTampering(t *testing.T) {
	selection, err := Select(selectionInput(t,
		map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
		map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
	))
	if err != nil || selection.Lock == nil {
		t.Fatalf("Select() = %+v, %v", selection, err)
	}
	encoded, err := EncodeLock(*selection.Lock)
	if err != nil {
		t.Fatalf("EncodeLock() error = %v", err)
	}

	tests := map[string]string{
		"changed parameter": strings.Replace(string(encoded), `"minimum_opportunities":5`, `"minimum_opportunities":10`, 1),
		"whitespace":        strings.Replace(string(encoded), `,"corpus"`, `, "corpus"`, 1),
		"unknown field":     strings.Replace(string(encoded), `"corpus"`, `"holdout":1,"corpus"`, 1),
		"other schema":      strings.Replace(string(encoded), LockSchema, "tacita.dev-lock/v2", 1),
	}
	for name, input := range tests {
		if _, err := DecodeLock([]byte(input)); !errors.Is(err, ErrDecode) && !errors.Is(err, ErrEncode) {
			t.Errorf("%s: DecodeLock() error = %v, want a decoding or contract error", name, err)
		}
	}
}

func TestCorpusRepositoryFor(t *testing.T) {
	if got, ok := CorpusRepositoryFor(Kubernetes.Commit); !ok || got != Kubernetes {
		t.Errorf("CorpusRepositoryFor(kubernetes) = %+v, %t", got, ok)
	}
	if _, ok := CorpusRepositoryFor("5f9694c42c06e63bb825536486bea93b36418c5d"); ok {
		t.Error("the configuration holdout is part of the calibration corpus")
	}
}

func TestEncodeLockRejectsUnfrozenContents(t *testing.T) {
	selection, err := Select(selectionInput(t,
		map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
		map[string]keys{unitLoose: {gain: 0.3, coverage: 0.1, jaccard: 0.6}},
	))
	if err != nil || selection.Lock == nil {
		t.Fatalf("Select() = %+v, %v", selection, err)
	}

	tests := map[string]func(*LockFields){
		"selector":          func(f *LockFields) { f.Selector = "development-selector-v2" },
		"holdout in corpus": func(f *LockFields) { f.Corpus = append(f.Corpus, "5f9694c42c06e63bb825536486bea93b36418c5d") },
		"architecture":      func(f *LockFields) { f.Environment.Architecture = "darwin/arm64" },
		"binary digest":     func(f *LockFields) { f.Environment.TacitaBinarySHA256 = "x" },
		"empty git version": func(f *LockFields) { f.Environment.GitVersion = "" },
		"grid digest":       func(f *LockFields) { f.GridDigest = strings.Repeat("0", 64) },
		"unknown identity":  func(f *LockFields) { f.ConfigurationID = "weight=unit" },
		"parameters":        func(f *LockFields) { f.Parameters.MinimumOpportunities = 0 },
		"one report":        func(f *LockFields) { f.DevelopmentReports = f.DevelopmentReports[:1] },
		"report repository": func(f *LockFields) { f.DevelopmentReports[0].RepositoryObjectID = Kubernetes.Commit },
		"report digest":     func(f *LockFields) { f.DevelopmentReports[1].ReportDigest = "x" },
		"metric order": func(f *LockFields) {
			f.SelectorMetrics[0], f.SelectorMetrics[1] = f.SelectorMetrics[1], f.SelectorMetrics[0]
		},
		"non-finite metric": func(f *LockFields) { f.SelectorMetrics[0].Value = Decimal(math.NaN()) },
	}
	for name, mutate := range tests {
		fields := selection.Lock.LockFields
		fields.Corpus = slices.Clone(fields.Corpus)
		fields.DevelopmentReports = slices.Clone(fields.DevelopmentReports)
		fields.SelectorMetrics = slices.Clone(fields.SelectorMetrics)
		mutate(&fields)
		lock, err := NewLock(fields)
		if err != nil {
			if !errors.Is(err, ErrEncode) {
				t.Errorf("%s: NewLock() error = %v, want ErrEncode", name, err)
			}
			continue
		}
		if _, err := EncodeLock(lock); !errors.Is(err, ErrEncode) {
			t.Errorf("%s: EncodeLock() error = %v, want ErrEncode", name, err)
		}
	}

	forged := *selection.Lock
	forged.LockDigest = strings.Repeat("0", 64)
	if _, err := EncodeLock(forged); !errors.Is(err, ErrEncode) {
		t.Errorf("EncodeLock(forged digest) error = %v, want ErrEncode", err)
	}
}
