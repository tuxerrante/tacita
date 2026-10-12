package report

import (
	"bytes"
	"cmp"
	"math"
	"slices"
	"strings"
)

// sizeWeightSimplicity is the frozen simplicity order of size weights.
var sizeWeightSimplicity = []string{weightUnit, weightInverseComponent, weightPairNormalized}

// Lane failure reasons: conditions that make every configuration unselectable.
const (
	LaneNondeterministicReport = "nondeterministic_report"
	LaneOperationalLimit       = "operational_limit"
	LaneReportNotComplete      = "report_not_complete"
	LaneControlFailed          = "control_failed"
)

// RepositoryRun is one corpus repository's calibration evidence: two
// independent runs' canonical reports and the harness's operational verdict.
type RepositoryRun struct {
	Report []byte
	Repeat []byte
	// OperationalPass reports that both runs exited cleanly and stayed within
	// the elapsed-time and peak-memory budgets the harness enforces.
	OperationalPass bool
}

// SelectionInput is the complete development evidence, in corpus order.
type SelectionInput struct {
	// Development holds golang/tools, then Kubernetes.
	Development [2]RepositoryRun
	// Controls holds go-starter, then Proficiency.
	Controls [2]RepositoryRun
	// Platform is the trusted GOOS/GOARCH of the process that produced and
	// selects from the reports; only the frozen linux/amd64 can write a lock.
	Platform string
}

// LaneFailure is a condition that fails the whole development lane.
type LaneFailure struct {
	Repository string
	Reason     string
}

// Rejection is one non-passing gate that makes a configuration unselectable.
type Rejection struct {
	ConfigurationID string
	Repository      string
	Gate            string
	Outcome         string
}

// Selection is the development selector's result: a lock when a
// configuration is selectable, otherwise the conditions that failed the lane.
type Selection struct {
	Lock         *Lock
	LaneFailures []LaneFailure
	Rejections   []Rejection
}

// selectable is a selectable configuration and its selector keys. A
// configuration that passes every gate has every key defined.
type selectable struct {
	configuration Configuration
	gain          float64
	coverage      float64
	jaccard       float64
}

// Select applies development-selector-v1. A configuration is selectable only
// if every development gate and every required control passes on both
// development repositories, both runs of every corpus repository are
// byte-identical, and the harness reports every run within its operational
// budgets. Among selectable
// configurations it compares, in order: the greatest worst-case gain over
// frequent consequent, the greatest worst-case coverage, the greatest minimum
// adjacent Jaccard, the simplest size weight, the lowest threshold tuple, and
// the ascending configuration ID.
//
// Malformed reports, reports of the wrong corpus commits, reports from
// different environments, gate metrics that disagree with the recorded
// metrics, and selection outside linux/amd64 are errors rather than lane
// failures: they are not evidence about the experiment.
func Select(input SelectionInput) (Selection, error) {
	if input.Platform != lockArchitecture {
		return Selection{}, invalid("selection runs only on the frozen %s, not %q", lockArchitecture, input.Platform)
	}

	var selection Selection
	checkRuns := func(run RepositoryRun, repository CorpusRepository) (Report, error) {
		report, err := decodeRole(run.Report, repository)
		if err != nil {
			return Report{}, err
		}
		if !bytes.Equal(run.Report, run.Repeat) {
			selection.LaneFailures = append(selection.LaneFailures,
				LaneFailure{Repository: repository.Commit, Reason: LaneNondeterministicReport})
		}
		if !run.OperationalPass {
			selection.LaneFailures = append(selection.LaneFailures,
				LaneFailure{Repository: repository.Commit, Reason: LaneOperationalLimit})
		}
		return report, nil
	}

	development := make([]Report, len(input.Development))
	digests := make([]ReportDigest, len(input.Development))
	for index, run := range input.Development {
		repository := []CorpusRepository{golangTools, kubernetes}[index]
		report, err := checkRuns(run, repository)
		if err != nil {
			return Selection{}, err
		}
		development[index] = report
		digests[index] = ReportDigest{RepositoryObjectID: repository.Commit, ReportDigest: Digest(run.Report)}
		if report.Status != statusComplete {
			selection.LaneFailures = append(selection.LaneFailures,
				LaneFailure{Repository: repository.Commit, Reason: LaneReportNotComplete})
		}
	}

	controls := make([]Report, len(input.Controls))
	for index, run := range input.Controls {
		repository := []CorpusRepository{goStarter, proficiency}[index]
		report, err := checkRuns(run, repository)
		if err != nil {
			return Selection{}, err
		}
		controls[index] = report
		if !controlPasses(report, controlIDFor(repository)) {
			selection.LaneFailures = append(selection.LaneFailures,
				LaneFailure{Repository: repository.Commit, Reason: LaneControlFailed})
		}
	}

	environment, err := sharedEnvironment(append(slices.Clone(development), controls...))
	if err != nil {
		return Selection{}, err
	}
	for _, report := range development {
		if report.Status != statusComplete {
			// A failed or inconclusive run evaluated no complete grid, so there
			// is nothing to compare; the lane failure is the result.
			return selection, nil
		}
	}
	// Decoding guarantees that both complete reports hold the identical frozen
	// grid in configuration order, so their configurations correspond.

	for _, report := range development {
		if err := checkGains(report); err != nil {
			return Selection{}, err
		}
		if err := checkNoHistory(report); err != nil {
			return Selection{}, err
		}
	}
	candidates, rejections, err := selectableConfigurations(development)
	if err != nil {
		return Selection{}, err
	}
	selection.Rejections = rejections
	if len(selection.LaneFailures) > 0 || len(candidates) == 0 {
		return selection, nil
	}

	slices.SortFunc(candidates, compareSelectable)
	winner := candidates[0]
	grid, err := FrozenGrid()
	if err != nil {
		return Selection{}, err
	}
	corpus := make([]string, 0, len(CalibrationCorpus()))
	for _, repository := range CalibrationCorpus() {
		corpus = append(corpus, repository.Commit)
	}
	lock, err := NewLock(LockFields{
		Schema:          LockSchema,
		Corpus:          corpus,
		Environment:     environment,
		GridDigest:      grid.GridDigest,
		Selector:        SelectorVersion,
		ConfigurationID: winner.configuration.ConfigurationID,
		Parameters: Parameters{
			SizeWeight:           winner.configuration.SizeWeight,
			MinimumOpportunities: winner.configuration.MinimumOpportunities,
			MinimumConfidence:    winner.configuration.MinimumConfidence,
			MinimumLift:          winner.configuration.MinimumLift,
		},
		DevelopmentReports: digests,
		SelectorMetrics: []SelectorMetric{
			{Metric: MetricWorstCaseFrequentConsequentGain, Value: Decimal(winner.gain)},
			{Metric: MetricWorstCaseCoverage, Value: Decimal(winner.coverage)},
			{Metric: MetricMinimumAdjacentJaccard, Value: Decimal(winner.jaccard)},
		},
	})
	if err != nil {
		return Selection{}, err
	}
	selection.Lock = &lock

	return selection, nil
}

// decodeRole decodes a calibration report and requires it to be the frozen
// commit of repository.
func decodeRole(data []byte, repository CorpusRepository) (Report, error) {
	report, err := Decode(data)
	if err != nil {
		return Report{}, err
	}
	if report.Kind != kindCalibration {
		return Report{}, invalid("%s report kind %q, want calibration", repository.Name, report.Kind)
	}
	if report.Repository.ResolvedCommit != repository.Commit {
		return Report{}, invalid("%s report resolved %s, want the frozen commit %s",
			repository.Name, report.Repository.ResolvedCommit, repository.Commit)
	}

	return report, nil
}

// controlIDFor names the control a control repository's role requires.
func controlIDFor(role CorpusRepository) string {
	if role.Role == RoleTemplateControl {
		return controlTemplateOrigin
	}
	return controlYoungRepository
}

// controlPasses requires a complete report holding exactly the control its
// role requires, passing, with the measurement the frozen control needs: zero
// inferred candidates for the young repository and an excluded root event for
// the template.
func controlPasses(report Report, id string) bool {
	if report.Status != statusComplete || len(report.Controls) != 1 {
		return false
	}
	control := report.Controls[0]
	measured := control.RootEventExcluded
	if id == controlYoungRepository {
		measured = control.InferredCandidates == 0
	}
	return control.ID == id && control.Outcome == outcomePass && measured
}

// sharedEnvironment requires every report to come from the same Tacita
// binary and helper version and from the frozen Git version.
func sharedEnvironment(reports []Report) (Environment, error) {
	first := reports[0].Tool
	for _, report := range reports {
		tool := report.Tool
		if tool.TacitaBinarySHA256 != first.TacitaBinarySHA256 || tool.GitVersion != FrozenGitVersion ||
			compareOptional(tool.GoHelperVersion, first.GoHelperVersion) != 0 {
			return Environment{}, invalid(
				"reports must come from one Tacita binary and Git %s; got Git %s", FrozenGitVersion, tool.GitVersion)
		}
	}

	return Environment{
		Architecture:       lockArchitecture,
		GitVersion:         first.GitVersion,
		TacitaBinarySHA256: first.TacitaBinarySHA256,
		GoHelperVersion:    first.GoHelperVersion,
	}, nil
}

// configurationGates indexes one report's gates by configuration. Decoding
// already guarantees that a complete calibration holds exactly the frozen gate
// set, so no gate can be missing here and a missing gate cannot count as a
// pass.
func configurationGates(report Report) map[string]map[string]Gate {
	byConfiguration := make(map[string]map[string]Gate, len(report.Configurations))
	for _, gate := range report.Gates {
		if gate.ConfigurationID == nil {
			continue
		}
		if byConfiguration[*gate.ConfigurationID] == nil {
			byConfiguration[*gate.ConfigurationID] = make(map[string]Gate)
		}
		byConfiguration[*gate.ConfigurationID][gate.ID] = gate
	}

	return byConfiguration
}

// selectableConfigurations returns every configuration whose gates pass on
// both development reports, with its selector keys, and every non-passing
// gate. The keys are the passing gates' own metrics, which must agree with the
// reports' metric records; a disagreement is malformed evidence.
func selectableConfigurations(reports []Report) ([]selectable, []Rejection, error) {
	indexed := make([]map[string]map[string]Gate, len(reports))
	var rejections []Rejection
	repositoryGatesPass := true
	for index, report := range reports {
		indexed[index] = configurationGates(report)
		for _, gate := range report.Gates {
			if gate.ConfigurationID == nil && gate.Outcome != outcomePass {
				repositoryGatesPass = false
				rejections = append(rejections, Rejection{
					Repository: report.Repository.ResolvedCommit, Gate: gate.ID, Outcome: gate.Outcome,
				})
			}
		}
	}

	var candidates []selectable
	for position, configuration := range reports[0].Configurations {
		passes := repositoryGatesPass
		keys := selectable{
			configuration: configuration,
			gain:          math.Inf(1),
			coverage:      math.Inf(1),
			jaccard:       math.Inf(1),
		}
		for index, report := range reports {
			gates := indexed[index][configuration.ConfigurationID]
			record := report.Metrics[position]
			if !sameMetric(gates["coverage"].Metric, record.Coverage) ||
				!sameMetric(gates["adjacent_jaccard"].Metric, record.MinimumJaccard) {
				return nil, nil, invalid("report %s configuration %s gate metrics disagree with its metric record",
					report.Repository.ResolvedCommit, configuration.ConfigurationID)
			}
			for _, id := range slices.Sorted(func(yield func(string) bool) {
				for id := range gates {
					if !yield(id) {
						return
					}
				}
			}) {
				gate := gates[id]
				if gate.Outcome != outcomePass {
					passes = false
					rejections = append(rejections, Rejection{
						ConfigurationID: configuration.ConfigurationID,
						Repository:      report.Repository.ResolvedCommit,
						Gate:            gate.ID,
						Outcome:         gate.Outcome,
					})
				}
			}
			if passes {
				keys.gain = min(keys.gain, float64(*gates[gateFrequentGain].Metric.Value))
				keys.coverage = min(keys.coverage, float64(*gates["coverage"].Metric.Value))
				keys.jaccard = min(keys.jaccard, float64(*gates["adjacent_jaccard"].Metric.Value))
			}
		}
		if passes {
			candidates = append(candidates, keys)
		}
	}

	return candidates, rejections, nil
}

// sameMetric reports whether two metrics hold the same value or the same
// undefined reason.
func sameMetric(left, right Metric) bool {
	if (left.Value == nil) != (right.Value == nil) {
		return false
	}
	if left.Value != nil {
		return *left.Value == *right.Value
	}
	return *left.Reason == *right.Reason
}

// compareSelectable orders the preferred configuration first.
func compareSelectable(left, right selectable) int {
	return cmp.Or(
		cmp.Compare(right.gain, left.gain),
		cmp.Compare(right.coverage, left.coverage),
		cmp.Compare(right.jaccard, left.jaccard),
		cmp.Compare(
			slices.Index(sizeWeightSimplicity, left.configuration.SizeWeight),
			slices.Index(sizeWeightSimplicity, right.configuration.SizeWeight),
		),
		cmp.Compare(left.configuration.MinimumOpportunities, right.configuration.MinimumOpportunities),
		cmp.Compare(left.configuration.MinimumConfidence, right.configuration.MinimumConfidence),
		cmp.Compare(left.configuration.MinimumLift, right.configuration.MinimumLift),
		cmp.Compare(left.configuration.ConfigurationID, right.configuration.ConfigurationID),
	)
}

// gainTolerance bounds the disagreement between a gain gate's metric, decided
// on unrounded values, and the same gain recomputed from six-digit report
// values: each of the three roundings contributes at most half a unit in the
// sixth place.
const gainTolerance = 2e-6

// checkGains recomputes every configuration's three gain gates from the
// report's own model and baseline records and rejects any disagreement, so a
// gate cannot claim a gain its recorded metrics do not support.
func checkGains(report Report) error {
	models := make(map[string]MetricRecord, len(report.Metrics))
	for _, record := range report.Metrics {
		models[record.ConfigurationID] = record
	}
	baselines := make(map[string]Baseline, len(report.Baselines))
	for _, baseline := range report.Baselines {
		if baseline.ConfigurationID != nil {
			baselines[recordKey(baseline.ConfigurationID, baseline.ID)] = baseline
		}
	}

	for _, configuration := range report.Configurations {
		id := configuration.ConfigurationID
		model := models[id]
		comparisons := map[string][2]Metric{
			"frequent_consequent_gain": {
				baselines[recordKey(&id, "frequent_consequent")].MicroAdherence,
				baselines[recordKey(&id, "frequent_consequent")].MacroAdherence,
			},
			"shuffled_control_gain": {
				baselines[recordKey(&id, "hash_shuffled")].MicroAdherence,
				baselines[recordKey(&id, "hash_shuffled")].MacroAdherence,
			},
		}
		if configuration.SizeWeight != weightUnit {
			counterpart := models[strings.Replace(id, "weight="+configuration.SizeWeight, "weight="+weightUnit, 1)]
			comparisons[gateUnweighted] = [2]Metric{counterpart.MicroAdherence, counterpart.MacroAdherence}
		}
		for _, gate := range report.Gates {
			baseline, ok := comparisons[gate.ID]
			if !ok || gate.ConfigurationID == nil || *gate.ConfigurationID != id {
				continue
			}
			want, defined := recomputedGain(model, baseline)
			if defined != (gate.Metric.Value != nil) ||
				(defined && math.Abs(float64(*gate.Metric.Value)-want) > gainTolerance) {
				return invalid("report %s configuration %s gate %s disagrees with its recorded metrics",
					report.Repository.ResolvedCommit, id, gate.ID)
			}
		}
	}

	return nil
}

// recomputedGain is the smaller of the micro and macro differences between a
// model record and a baseline, or undefined when any value is.
func recomputedGain(model MetricRecord, baseline [2]Metric) (float64, bool) {
	values := []*Decimal{model.MicroAdherence.Value, model.MacroAdherence.Value, baseline[0].Value, baseline[1].Value}
	for _, value := range values {
		if value == nil {
			return 0, false
		}
	}

	return math.Min(float64(*values[0]-*values[2]), float64(*values[1]-*values[3])), true
}

// checkNoHistory requires the no_history baseline and the no_history_control
// gate to record the same outcome, so the gate cannot pass a control its
// baseline records as failing.
func checkNoHistory(report Report) error {
	var baseline, gate string
	for _, record := range report.Baselines {
		if record.ID == baselineNoHistory && record.Outcome != nil {
			baseline = *record.Outcome
		}
	}
	for _, record := range report.Gates {
		if record.ID == gateNoHistory {
			gate = record.Outcome
		}
	}
	if baseline != gate {
		return invalid("report %s no_history baseline %q disagrees with its gate %q",
			report.Repository.ResolvedCommit, baseline, gate)
	}

	return nil
}
