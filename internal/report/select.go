package report

import (
	"bytes"
	"cmp"
	"math"
	"slices"
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

// DevelopmentRun is one development repository's calibration evidence: two
// independent runs' canonical reports and the harness's operational verdict.
type DevelopmentRun struct {
	Report []byte
	Repeat []byte
	// OperationalPass reports that both runs stayed within the elapsed-time
	// and peak-memory budgets the harness enforces.
	OperationalPass bool
}

// SelectionInput is the complete development evidence, in corpus order.
type SelectionInput struct {
	// Development holds golang/tools, then Kubernetes.
	Development [2]DevelopmentRun
	// Controls holds the go-starter, then the Proficiency calibration report.
	Controls [2][]byte
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
// development repositories, both runs of each are byte-identical, and the
// harness reports both within their operational budgets. Among selectable
// configurations it compares, in order: the greatest worst-case gain over
// frequent consequent, the greatest worst-case coverage, the greatest minimum
// adjacent Jaccard, the simplest size weight, the lowest threshold tuple, and
// the ascending configuration ID.
//
// Malformed reports, reports of the wrong corpus commits, and reports from
// different environments are errors rather than lane failures: they are not
// evidence about the experiment.
func Select(input SelectionInput) (Selection, error) {
	development := make([]Report, len(input.Development))
	digests := make([]ReportDigest, len(input.Development))
	var selection Selection
	for index, run := range input.Development {
		report, err := decodeRole(run.Report, []CorpusRepository{golangTools, kubernetes}[index])
		if err != nil {
			return Selection{}, err
		}
		development[index] = report
		repository := report.Repository.ResolvedCommit
		digests[index] = ReportDigest{RepositoryObjectID: repository, ReportDigest: Digest(run.Report)}
		if !bytes.Equal(run.Report, run.Repeat) {
			selection.LaneFailures = append(selection.LaneFailures, LaneFailure{Repository: repository, Reason: LaneNondeterministicReport})
		}
		if !run.OperationalPass {
			selection.LaneFailures = append(selection.LaneFailures, LaneFailure{Repository: repository, Reason: LaneOperationalLimit})
		}
		if report.Status != statusComplete {
			selection.LaneFailures = append(selection.LaneFailures, LaneFailure{Repository: repository, Reason: LaneReportNotComplete})
		}
	}

	controls := make([]Report, len(input.Controls))
	for index, data := range input.Controls {
		role := []CorpusRepository{goStarter, proficiency}[index]
		report, err := decodeRole(data, role)
		if err != nil {
			return Selection{}, err
		}
		controls[index] = report
		if !controlPasses(report, controlIDFor(role)) {
			selection.LaneFailures = append(selection.LaneFailures,
				LaneFailure{Repository: report.Repository.ResolvedCommit, Reason: LaneControlFailed})
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
		return "template_origin"
	}
	return "young_repository"
}

// controlPasses requires a complete report holding exactly the control its
// role requires, and that control passing.
func controlPasses(report Report, id string) bool {
	return report.Status == statusComplete && len(report.Controls) == 1 &&
		report.Controls[0].ID == id && report.Controls[0].Outcome == outcomePass
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
				keys.gain = min(keys.gain, float64(*gates["frequent_consequent_gain"].Metric.Value))
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
