package report

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/tuxerrante/tacita/internal/backtest"
	"github.com/tuxerrante/tacita/internal/gitlog"
	"github.com/tuxerrante/tacita/internal/mining"
)

func testEvents(count int) []gitlog.Event {
	events := make([]gitlog.Event, count)
	for index := range events {
		copy(events[index].ID[:], fmt.Sprintf("%040x", index+1))
		events[index].Kind = gitlog.SingleParentEvent
	}
	events[0].Kind = gitlog.RootEvent
	return events
}

// calibrationInput runs the backtest over a correlated synthetic history so
// every configuration displays candidates.
func calibrationInput(t *testing.T) CalibrationInput {
	t.Helper()

	const eventCount = 600
	events := testEvents(eventCount)
	var transactions []backtest.Transaction
	for position := 2; position <= eventCount; position++ {
		group := position % 7
		components := []string{fmt.Sprintf("a%d", group), fmt.Sprintf("b%d", group), "hub\xff"}
		if position%5 == 0 {
			components = []string{fmt.Sprintf("a%d", group), "hub\xff"}
		}
		transactions = append(transactions, backtest.Transaction{
			Position: position, Event: events[position-1], Components: components,
		})
	}
	partitions, err := backtest.Partitions(eventCount)
	if err != nil {
		t.Fatalf("Partitions() error = %v", err)
	}
	configurations := mining.AllConfigurations()
	chronological, err := backtest.EvaluateOrdering(transactions, partitions, configurations)
	if err != nil {
		t.Fatalf("EvaluateOrdering() error = %v", err)
	}
	var repository gitlog.ObjectID
	copy(repository[:], strings.Repeat("f", 40))
	shuffledTransactions, err := backtest.ShuffledTransactions(repository, events, transactions)
	if err != nil {
		t.Fatalf("ShuffledTransactions() error = %v", err)
	}
	shuffled, err := backtest.EvaluateOrdering(shuffledTransactions, partitions, configurations)
	if err != nil {
		t.Fatalf("EvaluateOrdering(shuffled) error = %v", err)
	}

	report := failedReport()
	return CalibrationInput{
		Tool: report.Tool,
		Repository: Repository{
			ObjectFormat:       "sha1",
			RootCommit:         events[0].ID.String(),
			ResolvedCommit:     events[eventCount-1].ID.String(),
			ReachableCommits:   eventCount,
			FirstParentEvents:  eventCount,
			SingleParentEvents: eventCount - 1,
		},
		Events:        events,
		Transactions:  transactions,
		Partitions:    partitions,
		Chronological: chronological,
		Shuffled:      shuffled,
		Exclusions: gitlog.Diagnostics{
			{Scope: gitlog.EventScope, Reason: gitlog.RootEventReason}: 1,
		},
		Limitations: []string{"sha1_only", "linux_amd64_only"},
	}
}

func TestCalibrationReportEncodesAndDecodes(t *testing.T) {
	input := calibrationInput(t)

	report, err := Calibration(input)
	if err != nil {
		t.Fatalf("Calibration() error = %v", err)
	}
	encoded, err := Encode(report)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if len(decoded.Configurations) != 81 || len(decoded.Metrics) != 81 || len(decoded.Partitions) != 4 {
		t.Errorf("configurations %d metrics %d partitions %d, want 81 81 4",
			len(decoded.Configurations), len(decoded.Metrics), len(decoded.Partitions))
	}
	if len(decoded.Baselines) != 1+81*3 {
		t.Errorf("baselines = %d, want %d", len(decoded.Baselines), 1+81*3)
	}
	if want := 1 + 81*6 + 54; len(decoded.Gates) != want {
		t.Errorf("gates = %d, want %d", len(decoded.Gates), want)
	}
	if len(decoded.Exclusions) != 6 || decoded.Exclusions[3] != (Exclusion{Scope: "event", Reason: "root_event", Count: 1}) {
		t.Errorf("exclusions = %+v, want all six with root_event 1", decoded.Exclusions)
	}

	displayed := 0
	for _, partition := range decoded.Partitions {
		displayed += partition.DisplayedCandidates
	}
	if displayed == 0 || displayed != len(decoded.Candidates) {
		t.Errorf("partitions report %d displayed, report holds %d candidates", displayed, len(decoded.Candidates))
	}
	for _, candidate := range decoded.Candidates {
		if candidate.Supporting.TotalEvents != candidate.RawSupport ||
			candidate.Exceptions.TotalEvents != candidate.RawOpportunity-candidate.RawSupport ||
			len(candidate.Supporting.Events) != 0 || len(candidate.Exceptions.Events) != 0 {
			t.Fatalf("candidate %s evidence = %+v / %+v", candidate.ID, candidate.Supporting, candidate.Exceptions)
		}
	}
}

func TestCalibrationReportIsByteIdentical(t *testing.T) {
	first, err := Calibration(calibrationInput(t))
	if err != nil {
		t.Fatalf("Calibration() error = %v", err)
	}
	second, err := Calibration(calibrationInput(t))
	if err != nil {
		t.Fatalf("Calibration() error = %v", err)
	}
	left, err := Encode(first)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	right, err := Encode(second)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if !bytes.Equal(left, right) {
		t.Error("two calibrations of identical input produced different bytes")
	}
}

func TestCalibrationRecordsPartitionsAndBaselines(t *testing.T) {
	input := calibrationInput(t)
	report, err := Calibration(input)
	if err != nil {
		t.Fatalf("Calibration() error = %v", err)
	}

	first := report.Partitions[0]
	want := Partition{
		Percentile:                     60,
		CutoffCommit:                   input.Events[359].ID.String(),
		TrainingFirstEvent:             1,
		TrainingLastEvent:              360,
		EvaluationFirstEvent:           361,
		EvaluationLastEvent:            420,
		EligibleTrainingTransactions:   359,
		EligibleEvaluationTransactions: 60,
		DisplayedCandidates:            first.DisplayedCandidates,
	}
	if first != want {
		t.Errorf("partition 60 = %+v, want %+v", first, want)
	}

	noHistory := report.Baselines[0]
	if noHistory.ID != "no_history" || noHistory.ConfigurationID != nil || *noHistory.Outcome != "pass" {
		t.Errorf("first baseline = %+v, want the passing no-history control", noHistory)
	}
	for _, baseline := range report.Baselines[1:] {
		if baseline.ID != "unweighted" || !strings.HasPrefix(*baseline.ConfigurationID, "weight=unit;") {
			continue
		}
		if baseline.MicroAdherence.Reason == nil || *baseline.MicroAdherence.Reason != "identical_baseline" {
			t.Errorf("unit unweighted baseline = %+v, want identical_baseline", baseline.MicroAdherence)
		}
	}
	if report.Gates[0].ID != "adjacent_jaccard" || report.Limitations[0] != "linux_amd64_only" {
		t.Errorf("gates and limitations are not sorted: %s, %v", report.Gates[0].ID, report.Limitations)
	}
}

func TestCalibrationRejectsInconsistentInput(t *testing.T) {
	input := calibrationInput(t)
	input.Shuffled = input.Shuffled[1:]
	if _, err := Calibration(input); err == nil {
		t.Error("Calibration() accepted mismatched orderings")
	}

	tests := map[string]func(*CalibrationInput){
		"another root":         func(in *CalibrationInput) { in.Repository.RootCommit = strings.Repeat("9", 40) },
		"another tip":          func(in *CalibrationInput) { in.Repository.ResolvedCommit = strings.Repeat("9", 40) },
		"another event count":  func(in *CalibrationInput) { in.Repository.FirstParentEvents++ },
		"another kind count":   func(in *CalibrationInput) { in.Repository.MergeResultEvents = 1 },
		"a second root":        func(in *CalibrationInput) { in.Events[3].Kind = gitlog.RootEvent },
		"no root":              func(in *CalibrationInput) { in.Events[0].Kind = gitlog.SingleParentEvent },
		"no events":            func(in *CalibrationInput) { in.Events = nil },
		"a truncated chain":    func(in *CalibrationInput) { in.Events = in.Events[:10] },
		"a missing display":    func(in *CalibrationInput) { in.Chronological[3].Displays = in.Chronological[3].Displays[1:] },
		"an extra partition":   func(in *CalibrationInput) { in.Partitions = append(in.Partitions, in.Partitions[3]) },
		"a short shuffled run": func(in *CalibrationInput) { in.Shuffled[0].Displays = nil },
	}
	for name, mutate := range tests {
		input := calibrationInput(t)
		mutate(&input)
		if _, err := Calibration(input); err == nil {
			t.Errorf("%s: Calibration() succeeded, want an error", name)
		}
	}
}

func TestGateRecordCarriesTheReason(t *testing.T) {
	id := "weight=unit;opportunities=5;confidence=0.70;lift=1.25"
	decision := backtest.Gate{
		ID:        backtest.GateCoverage,
		Outcome:   backtest.GateInconclusive,
		Metric:    backtest.Metric{Reason: backtest.ReasonNoOpportunities},
		Operator:  ">=",
		Threshold: 0.05,
		Reason:    backtest.ReasonNoOpportunities,
	}

	got := gate(&id, decision)

	if got.Outcome != "inconclusive" || got.Reason == nil || *got.Reason != "no_opportunities" ||
		got.Metric.Value != nil || *got.Metric.Reason != "no_opportunities" || got.Threshold != 0.05 {
		t.Errorf("gate() = %+v", got)
	}
}
