package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/tuxerrante/tacita/internal/backtest"
	"github.com/tuxerrante/tacita/internal/gitlog"
	"github.com/tuxerrante/tacita/internal/mining"
	"github.com/tuxerrante/tacita/internal/report"
)

// elapsedBudget is the frozen analysis elapsed-time budget.
const elapsedBudget = 20 * time.Minute

// calibrationLimitations are the stable limitation codes every calibration
// report carries: the frozen boundary's technical limits and the three that
// the freeze record requires on every report.
var calibrationLimitations = []string{
	"evidence_metadata_confidential",
	"generated_status_not_inferred",
	"human_inspected_holdout",
	"linux_amd64_only",
	"merge_policy_sensitive",
	"pull_request_boundaries_unavailable",
	"rename_identity_unavailable",
	"self_attested_freeze",
	"sha1_only",
}

// corpusLookup resolves a frozen commit to its calibration corpus entry.
type corpusLookup func(commit string) (report.CorpusRepository, bool)

// errNotCalibrationCorpus identifies a commit outside the frozen development
// and control corpus, which includes the configuration holdout.
var errNotCalibrationCorpus = errors.New("commit is not in the frozen calibration corpus")

// calibrate produces the canonical calibration report of one repository
// revision. A run that fails after the chain is known produces a failed
// report carrying the frozen failure code; a run that fails earlier returns
// only the error, because a report must name the analyzed commits.
func calibrate(ctx context.Context, path string, revision string, lookup corpusLookup) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, elapsedBudget)
	defer cancel()

	tool, err := toolRecord()
	if err != nil {
		return nil, err
	}
	repository, err := gitlog.Open(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("opening repository: %w", err)
	}
	commit, err := repository.ResolveCommit(ctx, revision)
	if err != nil {
		return nil, fmt.Errorf("resolving revision: %w", err)
	}
	role, ok := lookup(commit)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errNotCalibrationCorpus, commit)
	}
	tool.GitVersion = repository.GitVersion()

	events, err := repository.FirstParentEvents(ctx, commit)
	if err != nil {
		return nil, fmt.Errorf("listing integration events: %w", err)
	}
	record := repositoryRecord(events)
	record.ReachableCommits, err = repository.ReachableCommits(ctx, commit)
	if err != nil {
		return nil, fmt.Errorf("counting reachable commits: %w", err)
	}

	built, err := buildCalibration(ctx, repository, events, commit, role, tool, record)
	if err != nil {
		failure, ok := failureFor(err)
		if !ok {
			return nil, err
		}
		failed := report.Report{
			Schema:     report.ReportSchema,
			Kind:       "calibration",
			Status:     "failed",
			Tool:       tool,
			Repository: record,
			Failure:    &failure,
		}
		return report.Encode(failed)
	}

	return report.Encode(built)
}

// buildCalibration runs ingestion, both orderings, and the controls, and
// assembles the complete report.
func buildCalibration(
	ctx context.Context,
	repository *gitlog.Repository,
	events []gitlog.Event,
	commit string,
	role report.CorpusRepository,
	tool report.Tool,
	record report.Repository,
) (report.Report, error) {
	partitions, err := backtest.Partitions(len(events))
	if err != nil {
		return report.Report{}, err
	}

	observed := newObservations()
	positioner := backtest.NewPositioner(events)
	var transactions []backtest.Transaction
	diagnostics, err := repository.EachTransaction(ctx, events, func(transaction gitlog.Transaction) error {
		observed.transaction(transaction)
		positioned, err := positioner.Position(transaction)
		if err != nil {
			return err
		}
		transactions = append(transactions, positioned)
		return ctx.Err()
	})
	if err != nil {
		return report.Report{}, fmt.Errorf("reading transactions: %w", err)
	}

	configurations := mining.AllConfigurations()
	chronological, err := backtest.EvaluateOrdering(transactions, partitions, configurations)
	if err != nil {
		return report.Report{}, fmt.Errorf("evaluating chronological order: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return report.Report{}, err
	}
	var frozen gitlog.ObjectID
	copy(frozen[:], commit)
	shuffledTransactions, err := backtest.ShuffledTransactions(frozen, events, transactions)
	if err != nil {
		return report.Report{}, err
	}
	shuffled, err := backtest.EvaluateOrdering(shuffledTransactions, partitions, configurations)
	if err != nil {
		return report.Report{}, fmt.Errorf("evaluating hash-shuffled order: %w", err)
	}
	noHistory, err := backtest.NoHistoryCandidates(configurations)
	if err != nil {
		return report.Report{}, err
	}
	if err := ctx.Err(); err != nil {
		return report.Report{}, err
	}

	built, err := report.Calibration(report.CalibrationInput{
		Tool:                tool,
		Repository:          record,
		Events:              events,
		Transactions:        transactions,
		Partitions:          partitions,
		Chronological:       chronological,
		Shuffled:            shuffled,
		NoHistoryCandidates: noHistory,
		Exclusions:          diagnostics,
		Controls:            controls(role, chronological, diagnostics),
		Limitations:         calibrationLimitations,
	})
	if err != nil {
		return report.Report{}, err
	}
	built.Budgets = observed.budgets(len(events), report.CountRows(built))

	return built, nil
}

// repositoryRecord describes the chain; the reachable count is filled in once
// it is known.
func repositoryRecord(events []gitlog.Event) report.Repository {
	record := report.Repository{
		ObjectFormat:      "sha1",
		FirstParentEvents: len(events),
	}
	if len(events) > 0 {
		record.RootCommit = events[0].ID.String()
		record.ResolvedCommit = events[len(events)-1].ID.String()
	}
	for _, event := range events {
		switch event.Kind {
		case gitlog.SingleParentEvent:
			record.SingleParentEvents++
		case gitlog.MergeEvent:
			record.MergeResultEvents++
		}
	}

	return record
}

// controls records the young-repository or template-origin control for a
// control repository, and nothing for a development repository.
func controls(role report.CorpusRepository, outcomes []backtest.Outcome, diagnostics gitlog.Diagnostics) []report.Control {
	displayed := 0
	for _, outcome := range outcomes {
		for _, display := range outcome.Displays {
			displayed += len(display.Candidates)
		}
	}
	rootExcluded := diagnostics[gitlog.Exclusion{Scope: gitlog.EventScope, Reason: gitlog.RootEventReason}] == 1

	switch role.Role {
	case report.RoleYoungControl:
		outcome := outcomeFail
		if displayed == 0 {
			outcome = outcomePass
		}
		return []report.Control{{
			ID: "young_repository", InferredCandidates: displayed, RootEventExcluded: rootExcluded, Outcome: outcome,
		}}
	case report.RoleTemplateControl:
		outcome := outcomeFail
		if rootExcluded {
			outcome = outcomePass
		}
		return []report.Control{{
			ID: "template_origin", InferredCandidates: displayed, RootEventExcluded: rootExcluded, Outcome: outcome,
		}}
	default:
		return nil
	}
}

// toolRecord identifies the running binary by version and content digest.
func toolRecord() (report.Tool, error) {
	executable, err := os.Executable()
	if err != nil {
		return report.Tool{}, fmt.Errorf("locating the Tacita binary: %w", err)
	}
	file, err := os.Open(executable) //nolint:gosec // the running binary's own path
	if err != nil {
		return report.Tool{}, fmt.Errorf("reading the Tacita binary: %w", err)
	}
	defer file.Close() //nolint:errcheck // read-only file
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return report.Tool{}, fmt.Errorf("hashing the Tacita binary: %w", err)
	}

	version := "(unknown)"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		version = info.Main.Version
	}

	return report.Tool{
		TacitaVersion:      version,
		TacitaBinarySHA256: hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

// failureFor maps a typed run failure to its frozen failure record. It reports
// false for a failure the frozen vocabulary does not name.
func failureFor(err error) (report.Failure, bool) {
	failure := report.Failure{Operation: "calibration"}
	set := func(code string, observed, limit int) {
		failure.Code = code
		o, l := int64(observed), int64(limit)
		failure.Observed, failure.Limit = &o, &l
	}

	var pathLimit *gitlog.PathIdentityLimitError
	var componentLimit *gitlog.ComponentIdentityLimitError
	var observationLimit *mining.PairObservationLimitError
	var pairLimit *mining.PairIdentityLimitError
	var outputLimit *gitlog.OutputLimitError
	var rowLimit *report.LimitError
	switch {
	case errors.As(err, &pathLimit):
		set("budget_unique_paths", pathLimit.Observed, pathLimit.Limit)
	case errors.As(err, &componentLimit):
		set("budget_unique_components", componentLimit.Observed, componentLimit.Limit)
	case errors.As(err, &observationLimit):
		set("budget_pair_observations", observationLimit.Observed, observationLimit.Limit)
	case errors.As(err, &pairLimit):
		set("budget_unique_pairs", pairLimit.Observed, pairLimit.Limit)
	case errors.As(err, &outputLimit):
		failure.Code = "budget_git_stdout"
		if outputLimit.Stream == "stderr" {
			failure.Code = "budget_git_stderr"
		}
		limit := int64(outputLimit.Limit)
		failure.Limit = &limit
	case errors.As(err, &rowLimit):
		failure.Code = rowLimit.Code
		failure.Observed, failure.Limit = &rowLimit.Observed, &rowLimit.Limit
	case errors.Is(err, context.DeadlineExceeded):
		failure.Code = "budget_elapsed"
	case errors.Is(err, context.Canceled):
		failure.Code = "cancelled"
	case errors.Is(err, gitlog.ErrIncompleteRepository):
		failure.Code = "incomplete_repository"
	case errors.Is(err, gitlog.ErrMalformedGitOutput):
		failure.Code = "malformed_git_output"
	default:
		return report.Failure{}, false
	}

	return failure, true
}
