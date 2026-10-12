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
		report, err := decodeRole(run.Report, []CorpusRepository{GolangTools, Kubernetes}[index])
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
		report, err := decodeRole(data, []CorpusRepository{GoStarter, Proficiency}[index])
		if err != nil {
			return Selection{}, err
		}
		controls[index] = report
		if !controlsPass(report) {
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

	candidates, rejections := selectableConfigurations(development)
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

// controlsPass requires a complete report whose every control record passes,
// and at least one control record.
func controlsPass(report Report) bool {
	if report.Status != statusComplete || len(report.Controls) == 0 {
		return false
	}
	for _, control := range report.Controls {
		if control.Outcome != outcomePass {
			return false
		}
	}

	return true
}

// sharedEnvironment requires every report to come from the same Tacita
// binary, Git version, and helper version.
func sharedEnvironment(reports []Report) (Environment, error) {
	first := reports[0].Tool
	for _, report := range reports[1:] {
		tool := report.Tool
		if tool.TacitaBinarySHA256 != first.TacitaBinarySHA256 || tool.GitVersion != first.GitVersion ||
			compareOptional(tool.GoHelperVersion, first.GoHelperVersion) != 0 {
			return Environment{}, invalid("reports come from different binaries or Git versions")
		}
	}

	return Environment{
		Architecture:       lockArchitecture,
		GitVersion:         first.GitVersion,
		TacitaBinarySHA256: first.TacitaBinarySHA256,
		GoHelperVersion:    first.GoHelperVersion,
	}, nil
}

// selectableConfigurations returns every configuration whose gates pass on both
// development reports, with its selector keys, and every non-passing gate.
func selectableConfigurations(reports []Report) ([]selectable, []Rejection) {
	var rejections []Rejection
	repositoryGatesPass := true
	for _, report := range reports {
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
	for _, configuration := range reports[0].Configurations {
		passes := repositoryGatesPass
		keys := selectable{
			configuration: configuration,
			gain:          math.Inf(1),
			coverage:      math.Inf(1),
			jaccard:       math.Inf(1),
		}
		for _, report := range reports {
			for _, gate := range report.Gates {
				if gate.ConfigurationID == nil || *gate.ConfigurationID != configuration.ConfigurationID {
					continue
				}
				if gate.Outcome != outcomePass {
					passes = false
					rejections = append(rejections, Rejection{
						ConfigurationID: configuration.ConfigurationID,
						Repository:      report.Repository.ResolvedCommit,
						Gate:            gate.ID,
						Outcome:         gate.Outcome,
					})
					continue
				}
				if gate.ID == "frequent_consequent_gain" {
					keys.gain = min(keys.gain, float64(*gate.Metric.Value))
				}
			}
			for _, record := range report.Metrics {
				if record.ConfigurationID != configuration.ConfigurationID {
					continue
				}
				if record.Coverage.Value != nil {
					keys.coverage = min(keys.coverage, float64(*record.Coverage.Value))
				}
				if record.MinimumJaccard.Value != nil {
					keys.jaccard = min(keys.jaccard, float64(*record.MinimumJaccard.Value))
				}
			}
		}
		if passes {
			candidates = append(candidates, keys)
		}
	}

	return candidates, rejections
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
