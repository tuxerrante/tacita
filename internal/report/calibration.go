package report

import (
	"cmp"
	"slices"

	"github.com/tuxerrante/tacita/internal/backtest"
	"github.com/tuxerrante/tacita/internal/gitlog"
	"github.com/tuxerrante/tacita/internal/mining"
)

// inferredOrigin is the origin of every repository-inferred candidate.
const inferredOrigin = "inferred"

// CalibrationInput is one repository's complete calibration: the chain, the
// retained transactions, both orderings' outcomes, and what ingestion and the
// harness observed.
type CalibrationInput struct {
	Tool       Tool
	Repository Repository
	// Events is the complete first-parent chain; partition cutoffs name its
	// events.
	Events       []gitlog.Event
	Transactions []backtest.Transaction
	Partitions   []backtest.Partition
	// Chronological and Shuffled are EvaluateOrdering over the chronological
	// and hash-shuffled orderings of the same configurations.
	Chronological []backtest.Outcome
	Shuffled      []backtest.Outcome
	// NoHistoryCandidates is what the no-history control displayed.
	NoHistoryCandidates int
	Exclusions          gitlog.Diagnostics
	Budgets             []Budget
	Controls            []Control
	Limitations         []string
}

// eventExclusions and pathExclusions are every frozen exclusion reason by
// scope. A calibration report lists each one, zero counts included, so its
// shape does not depend on which exclusions occurred.
var (
	eventExclusions = []gitlog.ExclusionReason{
		gitlog.EventComponentLimitReason,
		gitlog.EventPathLimitReason,
		gitlog.NoEligiblePathsReason,
		gitlog.RootEventReason,
	}
	pathExclusions = []gitlog.ExclusionReason{
		gitlog.GitlinkPathReason,
		gitlog.VendorPathReason,
	}
)

// Calibration builds the complete calibration report of one repository. It
// decides the development gates from the two orderings, so the report cannot
// disagree with the outcomes it records.
func Calibration(input CalibrationInput) (Report, error) {
	decided, err := backtest.DevelopmentGates(input.Chronological, input.Shuffled)
	if err != nil {
		return Report{}, err
	}

	partitions, err := calibrationPartitions(input)
	if err != nil {
		return Report{}, err
	}

	report := Report{
		Schema:         ReportSchema,
		Kind:           "calibration",
		Status:         "complete",
		Tool:           input.Tool,
		Repository:     input.Repository,
		Configurations: configurations(input.Chronological),
		Budgets:        input.Budgets,
		Partitions:     partitions,
		Exclusions:     exclusions(input.Exclusions),
		Candidates:     candidates(input.Chronological, input.Partitions),
		Baselines:      baselines(input.Chronological, input.Shuffled, input.NoHistoryCandidates),
		Controls:       input.Controls,
		Metrics:        metricRecords(input.Chronological),
		Gates:          gates(decided, input.NoHistoryCandidates),
		Limitations:    slices.Sorted(slices.Values(input.Limitations)),
	}

	return report, nil
}

func configurations(outcomes []backtest.Outcome) []Configuration {
	records := make([]Configuration, len(outcomes))
	for index, outcome := range outcomes {
		configuration := outcome.Configuration
		records[index] = Configuration{
			ConfigurationID:      configuration.ID(),
			SizeWeight:           string(configuration.SizeWeight),
			MinimumOpportunities: configuration.MinimumOpportunities,
			MinimumConfidence:    Decimal(configuration.MinimumConfidence),
			MinimumLift:          Decimal(configuration.MinimumLift),
		}
	}

	return records
}

func calibrationPartitions(input CalibrationInput) ([]Partition, error) {
	records := make([]Partition, len(input.Partitions))
	for index, partition := range input.Partitions {
		if partition.TrainingEnd < 1 || partition.TrainingEnd > len(input.Events) {
			return nil, invalid("partition %d cutoff %d outside the chain", partition.Percentile, partition.TrainingEnd)
		}
		training, evaluation := 0, 0
		for _, transaction := range input.Transactions {
			switch {
			case transaction.Position <= partition.TrainingEnd:
				training++
			case transaction.Position <= partition.EvaluationEnd:
				evaluation++
			}
		}
		displayed := 0
		for _, outcome := range input.Chronological {
			displayed += len(outcome.Displays[index].Candidates)
		}
		records[index] = Partition{
			Percentile:                     partition.Percentile,
			CutoffCommit:                   input.Events[partition.TrainingEnd-1].ID.String(),
			TrainingFirstEvent:             1,
			TrainingLastEvent:              partition.TrainingEnd,
			EvaluationFirstEvent:           partition.TrainingEnd + 1,
			EvaluationLastEvent:            partition.EvaluationEnd,
			EligibleTrainingTransactions:   training,
			EligibleEvaluationTransactions: evaluation,
			DisplayedCandidates:            displayed,
		}
	}

	return records, nil
}

func exclusions(diagnostics gitlog.Diagnostics) []Exclusion {
	records := make([]Exclusion, 0, len(eventExclusions)+len(pathExclusions))
	for _, scope := range []struct {
		scope   gitlog.ExclusionScope
		reasons []gitlog.ExclusionReason
	}{
		{scope: gitlog.EventScope, reasons: eventExclusions},
		{scope: gitlog.PathScope, reasons: pathExclusions},
	} {
		for _, reason := range scope.reasons {
			records = append(records, Exclusion{
				Scope:  string(scope.scope),
				Reason: string(reason),
				Count:  diagnostics[gitlog.Exclusion{Scope: scope.scope, Reason: reason}],
			})
		}
	}

	return records
}

func candidates(outcomes []backtest.Outcome, partitions []backtest.Partition) []Candidate {
	var records []Candidate
	for _, outcome := range outcomes {
		for index, display := range outcome.Displays {
			for _, shown := range display.Candidates {
				candidate := shown.Candidate
				records = append(records, Candidate{
					ID:                 CandidateID(inferredOrigin, shown.Antecedent, shown.Consequent),
					Origin:             inferredOrigin,
					ConfigurationID:    outcome.Configuration.ID(),
					Partition:          partitions[index].Percentile,
					Rank:               candidate.Rank,
					Antecedent:         Bytes(shown.Antecedent),
					Consequent:         Bytes(shown.Consequent),
					RawOpportunity:     candidate.RawOpportunity,
					RawSupport:         candidate.RawSupport,
					WeightedExposure:   Decimal(candidate.WeightedExposure),
					WeightedSupport:    Decimal(candidate.WeightedSupport),
					WeightedConfidence: Decimal(candidate.WeightedConfidence),
					WeightedPrevalence: Decimal(candidate.WeightedPrevalence),
					WeightedLift:       Decimal(candidate.WeightedLift),
					Supporting:         Evidence{TotalEvents: candidate.RawSupport},
					Exceptions:         Evidence{TotalEvents: candidate.RawOpportunity - candidate.RawSupport},
				})
			}
		}
	}
	slices.SortFunc(records, compareCandidates)

	return records
}

func baselines(chronological, shuffled []backtest.Outcome, noHistory int) []Baseline {
	byConfiguration := make(map[mining.Configuration]backtest.Evaluation, len(chronological))
	for _, outcome := range chronological {
		byConfiguration[outcome.Configuration] = outcome.Model
	}

	noHistoryOutcome := string(backtest.NoHistoryGate(noHistory).Outcome)
	floor := metric(backtest.Metric{Reason: backtest.ReasonBelowInferenceFloor})
	records := []Baseline{{
		ID:                         "no_history",
		MicroAdherence:             floor,
		MacroAdherence:             floor,
		Coverage:                   floor,
		CutoffAbstention:           floor,
		MinimumJaccard:             floor,
		RankCorrelationDiagnostics: []Metric{},
		Outcome:                    &noHistoryOutcome,
	}}
	for index, outcome := range chronological {
		id := outcome.Configuration.ID()
		records = append(records,
			evaluationBaseline(id, "frequent_consequent", outcome.FrequentConsequent),
			evaluationBaseline(id, "hash_shuffled", shuffled[index].Model),
		)
		if counterpart, ok := backtest.UnweightedCounterpart(outcome.Configuration); ok {
			records = append(records, evaluationBaseline(id, "unweighted", byConfiguration[counterpart]))
			continue
		}
		identical := metric(backtest.Metric{Reason: backtest.ReasonIdenticalBaseline})
		records = append(records, Baseline{
			ConfigurationID:            &id,
			ID:                         "unweighted",
			MicroAdherence:             identical,
			MacroAdherence:             identical,
			Coverage:                   identical,
			CutoffAbstention:           identical,
			MinimumJaccard:             identical,
			RankCorrelationDiagnostics: []Metric{},
		})
	}

	return records
}

func evaluationBaseline(configurationID string, id string, evaluation backtest.Evaluation) Baseline {
	return Baseline{
		ConfigurationID:            &configurationID,
		ID:                         id,
		MicroAdherence:             metric(evaluation.Micro),
		MacroAdherence:             metric(evaluation.Macro),
		Coverage:                   metric(evaluation.Coverage),
		CutoffAbstention:           metric(evaluation.CutoffAbstention),
		MinimumJaccard:             metric(evaluation.MinimumJaccard),
		RankCorrelationDiagnostics: metrics(evaluation.RankCorrelation),
	}
}

func metricRecords(outcomes []backtest.Outcome) []MetricRecord {
	records := make([]MetricRecord, len(outcomes))
	for index, outcome := range outcomes {
		model := outcome.Model
		records[index] = MetricRecord{
			ConfigurationID:            outcome.Configuration.ID(),
			MicroAdherence:             metric(model.Micro),
			MacroAdherence:             metric(model.Macro),
			Coverage:                   metric(model.Coverage),
			CutoffAbstention:           metric(model.CutoffAbstention),
			MinimumJaccard:             metric(model.MinimumJaccard),
			RankCorrelationDiagnostics: metrics(model.RankCorrelation),
		}
	}

	return records
}

func gates(decided []backtest.ConfigurationGates, noHistory int) []Gate {
	records := []Gate{gate(nil, backtest.NoHistoryGate(noHistory))}
	for _, configuration := range decided {
		id := configuration.Configuration.ID()
		for _, decision := range configuration.Gates {
			records = append(records, gate(&id, decision))
		}
	}
	slices.SortFunc(records, func(left, right Gate) int {
		return cmp.Or(cmp.Compare(left.ID, right.ID), compareOptional(left.ConfigurationID, right.ConfigurationID))
	})

	return records
}

func gate(configurationID *string, decision backtest.Gate) Gate {
	record := Gate{
		ConfigurationID: configurationID,
		ID:              decision.ID,
		Outcome:         string(decision.Outcome),
		Metric:          metric(decision.Metric),
		Operator:        decision.Operator,
		Threshold:       Decimal(decision.Threshold),
	}
	if decision.Reason != "" {
		reason := string(decision.Reason)
		record.Reason = &reason
	}

	return record
}

func metric(value backtest.Metric) Metric {
	if !value.Defined() {
		reason := string(value.Reason)
		return Metric{Reason: &reason}
	}
	decimal := Decimal(value.Value)

	return Metric{Value: &decimal}
}

func metrics(values []backtest.Metric) []Metric {
	records := make([]Metric, len(values))
	for index, value := range values {
		records[index] = metric(value)
	}

	return records
}
