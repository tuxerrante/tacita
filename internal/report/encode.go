package report

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/tuxerrante/tacita/internal/mining"
)

const (
	maxReportRows  = 100_000
	maxReportBytes = 64 << 20
	maxEvidence    = 20
)

// Encode validates report and renders its canonical bytes: one minified object
// followed by one LF byte. A report over the frozen row or byte budget fails
// with a LimitError naming that budget's failure code.
func Encode(report Report) ([]byte, error) {
	return encode(report, maxReportRows, maxReportBytes)
}

// encode carries the budgets explicitly so a test can reach them without a
// report of the frozen size.
func encode(report Report, rowLimit int, byteLimit int) ([]byte, error) {
	normalize(&report)
	if err := validate(report); err != nil {
		return nil, err
	}
	if rows := countRows(report); rows > rowLimit {
		return nil, &LimitError{Code: "budget_report_rows", Observed: int64(rows), Limit: int64(rowLimit)}
	}

	encoded, err := canonicalJSON(report)
	if err != nil {
		return nil, &EncodeError{Detail: err.Error()}
	}
	encoded = append(encoded, '\n')
	if len(encoded) > byteLimit {
		return nil, &LimitError{Code: "budget_report_bytes", Observed: int64(len(encoded)), Limit: int64(byteLimit)}
	}

	return encoded, nil
}

// Decode parses canonical report bytes. Unknown fields, unknown enum values,
// non-canonical ordering or formatting, and trailing data are errors, so a
// decoded report re-encodes to exactly the input.
func Decode(data []byte) (Report, error) {
	var report Report
	if err := strictUnmarshal(data, &report); err != nil {
		return Report{}, err
	}
	canonical, err := Encode(report)
	if err != nil {
		return Report{}, asDecodeError(err)
	}
	if !bytes.Equal(canonical, data) {
		return Report{}, &DecodeError{Detail: "report bytes are not canonical"}
	}

	return report, nil
}

// asDecodeError reports a contract violation found in decoded bytes as a
// decoding error. A budget breach keeps its own category and failure code.
func asDecodeError(err error) error {
	var encodeErr *EncodeError
	if errors.As(err, &encodeErr) {
		return &DecodeError{Detail: encodeErr.Detail}
	}

	return err
}

// CountRows returns the report's rows as the frozen row budget counts them.
func CountRows(report Report) int {
	return countRows(report)
}

func countRows(report Report) int {
	rows := len(report.Partitions) + len(report.Exclusions) + len(report.Candidates) +
		len(report.Baselines) + len(report.Controls) + len(report.Profiles) +
		len(report.PreregisteredChecks) + len(report.Metrics) + len(report.Gates) +
		len(report.Limitations)
	for _, candidate := range report.Candidates {
		rows += len(candidate.Supporting.Events) + len(candidate.Exceptions.Events)
	}
	if report.Failure != nil {
		rows++
	}

	return rows
}

// normalize replaces nil arrays with empty ones, so every array is present.
func normalize(report *Report) {
	report.Configurations = nonNil(report.Configurations)
	report.Budgets = nonNil(report.Budgets)
	report.Partitions = nonNil(report.Partitions)
	report.Exclusions = nonNil(report.Exclusions)
	report.Candidates = nonNil(report.Candidates)
	report.Baselines = nonNil(report.Baselines)
	report.Controls = nonNil(report.Controls)
	report.Profiles = nonNil(report.Profiles)
	report.PreregisteredChecks = nonNil(report.PreregisteredChecks)
	report.Metrics = nonNil(report.Metrics)
	report.Gates = nonNil(report.Gates)
	report.Limitations = nonNil(report.Limitations)
	for index := range report.Candidates {
		candidate := &report.Candidates[index]
		candidate.Supporting.Events = nonNil(candidate.Supporting.Events)
		candidate.Exceptions.Events = nonNil(candidate.Exceptions.Events)
		for _, evidence := range []*Evidence{&candidate.Supporting, &candidate.Exceptions} {
			for event := range evidence.Events {
				evidence.Events[event].Paths = nonNil(evidence.Events[event].Paths)
			}
		}
	}
	for index := range report.Baselines {
		report.Baselines[index].RankCorrelationDiagnostics = nonNil(report.Baselines[index].RankCorrelationDiagnostics)
	}
	for index := range report.Metrics {
		report.Metrics[index].RankCorrelationDiagnostics = nonNil(report.Metrics[index].RankCorrelationDiagnostics)
	}
	for index := range report.PreregisteredChecks {
		report.PreregisteredChecks[index].ExcludedBy = nonNil(report.PreregisteredChecks[index].ExcludedBy)
	}
}

func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}

	return values
}

func validate(report Report) error {
	checks := []func(Report) error{
		validateHeader,
		validateShape,
		validateConfigurations,
		validateBudgets,
		validatePartitions,
		validateExclusions,
		validateCandidates,
		validateBaselines,
		validateControls,
		validateMetricRecords,
		validateGates,
		validateLimitations,
	}
	for _, check := range checks {
		if err := check(report); err != nil {
			return err
		}
	}

	return nil
}

func invalid(format string, args ...any) error {
	return &EncodeError{Detail: fmt.Sprintf(format, args...)}
}

func validateHeader(report Report) error {
	switch {
	case report.Schema != ReportSchema:
		return invalid("schema %q", report.Schema)
	case !oneOf(kinds, report.Kind):
		return invalid("kind %q", report.Kind)
	case !oneOf(statuses, report.Status):
		return invalid("status %q", report.Status)
	case (report.Status == statusFailed) != (report.Failure != nil):
		return invalid("status %q with failure present %t", report.Status, report.Failure != nil)
	case !isDigest(report.Tool.TacitaBinarySHA256):
		return invalid("tacita_binary_sha256 %q", report.Tool.TacitaBinarySHA256)
	case report.Tool.TacitaVersion == "" || report.Tool.GitVersion == "":
		return invalid("empty tool version")
	case report.Repository.ObjectFormat != "sha1":
		return invalid("object_format %q", report.Repository.ObjectFormat)
	case !isObjectID(report.Repository.RootCommit) || !isObjectID(report.Repository.ResolvedCommit):
		return invalid("repository commit is not a full object ID")
	case report.Repository.FirstParentEvents !=
		report.Repository.SingleParentEvents+report.Repository.MergeResultEvents+1:
		return invalid("first_parent_events is not the root plus single-parent and merge-result events")
	case report.Repository.ReachableCommits < report.Repository.FirstParentEvents:
		return invalid("reachable_commits below first_parent_events")
	case len(report.Profiles) > 0:
		return invalid("profile records are not implemented in report v1 yet")
	}
	if failure := report.Failure; failure != nil {
		if !oneOf(failureCodes, failure.Code) || failure.Operation == "" {
			return invalid("failure code %q operation %q", failure.Code, failure.Operation)
		}
		if failure.DiagnosticDigest != nil && !isDigest(*failure.DiagnosticDigest) {
			return invalid("failure diagnostic_digest %q", *failure.DiagnosticDigest)
		}
	}

	return nil
}

// cutoffPercentiles are the frozen calibration partitions, in report order.
var cutoffPercentiles = []int{60, 70, 80, 90}

// validateShape requires a complete calibration report to hold the whole
// frozen grid, the four frozen partitions, and one metric record per
// configuration in the same order.
func validateShape(report Report) error {
	if report.Kind != kindCalibration || report.Status != statusComplete {
		return nil
	}
	if len(report.Configurations) != len(mining.AllConfigurations()) {
		return invalid("complete calibration holds %d configurations, want %d",
			len(report.Configurations), len(mining.AllConfigurations()))
	}
	percentiles := make([]int, len(report.Partitions))
	for index, partition := range report.Partitions {
		percentiles[index] = partition.Percentile
	}
	if !slices.Equal(percentiles, cutoffPercentiles) {
		return invalid("complete calibration partitions %v, want %v", percentiles, cutoffPercentiles)
	}
	if len(report.Metrics) != len(report.Configurations) {
		return invalid("complete calibration holds %d metric records for %d configurations",
			len(report.Metrics), len(report.Configurations))
	}
	for index, record := range report.Metrics {
		if record.ConfigurationID != report.Configurations[index].ConfigurationID {
			return invalid("metric record %d is for %s, want %s",
				index, record.ConfigurationID, report.Configurations[index].ConfigurationID)
		}
	}

	return nil
}

// validateConfigurations resolves every record against the frozen grid, so an
// ID cannot carry thresholds other than its own.
func validateConfigurations(report Report) error {
	frozen := make(map[string]mining.Configuration)
	for _, configuration := range mining.AllConfigurations() {
		frozen[configuration.ID()] = configuration
	}
	for index, record := range report.Configurations {
		configuration, ok := frozen[record.ConfigurationID]
		if !ok || string(configuration.SizeWeight) != record.SizeWeight ||
			configuration.MinimumOpportunities != record.MinimumOpportunities ||
			configuration.MinimumConfidence != float64(record.MinimumConfidence) ||
			configuration.MinimumLift != float64(record.MinimumLift) {
			return invalid("configuration %q is not a frozen grid point", record.ConfigurationID)
		}
		if lock := record.DevelopmentLockDigest; lock != nil && !isDigest(*lock) {
			return invalid("development_lock_digest %q", *lock)
		}
		if index > 0 && report.Configurations[index-1].ConfigurationID >= record.ConfigurationID {
			return invalid("configurations not in strict configuration-ID order at %d", index)
		}
	}

	return nil
}

// validateBudgets requires the frozen budget table and derives each outcome
// from its observation, so a record cannot pass a limit it exceeds.
func validateBudgets(report Report) error {
	for index, budget := range report.Budgets {
		spec := slices.IndexFunc(budgetSpecs, func(spec BudgetSpec) bool { return spec.ID == budget.ID })
		if spec < 0 || budgetSpecs[spec].Unit != budget.Unit || budgetSpecs[spec].Limit != budget.Limit {
			return invalid("budget %q unit %q limit %d", budget.ID, budget.Unit, budget.Limit)
		}
		if budgetSpecs[spec].Unobserved != (budget.Observed == nil) || (budget.Observed == nil) != (budget.Outcome == nil) {
			return invalid("budget %q observation presence", budget.ID)
		}
		if budget.Observed != nil {
			want := outcomePass
			if *budget.Observed > budget.Limit {
				want = outcomeFail
			}
			if *budget.Observed < 0 || *budget.Outcome != want {
				return invalid("budget %q observed %d outcome %q", budget.ID, *budget.Observed, *budget.Outcome)
			}
		}
		if index > 0 && report.Budgets[index-1].ID >= budget.ID {
			return invalid("budgets not in strict ID order at %d", index)
		}
	}

	return nil
}

// validatePartitions requires one-based, contiguous, non-overlapping event
// ranges inside the chain, eligible counts that fit their ranges, and
// displayed counts that match the candidate records.
func validatePartitions(report Report) error {
	events := report.Repository.FirstParentEvents
	displayed := make(map[int]int)
	for _, candidate := range report.Candidates {
		displayed[candidate.Partition]++
	}
	for index, partition := range report.Partitions {
		if !slices.Contains([]int{60, 70, 80, 90, 100}, partition.Percentile) ||
			(index > 0 && report.Partitions[index-1].Percentile >= partition.Percentile) {
			return invalid("partition percentile %d at %d", partition.Percentile, index)
		}
		if !isObjectID(partition.CutoffCommit) {
			return invalid("partition %d cutoff_commit %q", partition.Percentile, partition.CutoffCommit)
		}
		training := partition.TrainingLastEvent
		evaluation := partition.EvaluationLastEvent - partition.TrainingLastEvent
		if partition.TrainingFirstEvent != 1 || training < 1 ||
			partition.EvaluationFirstEvent != training+1 || evaluation < 0 ||
			partition.EvaluationLastEvent > events {
			return invalid("partition %d event ranges", partition.Percentile)
		}
		if index > 0 && partition.Percentile != 100 &&
			training != report.Partitions[index-1].EvaluationLastEvent {
			return invalid("partition %d does not start where %d ends",
				partition.Percentile, report.Partitions[index-1].Percentile)
		}
		if partition.EligibleTrainingTransactions < 0 || partition.EligibleTrainingTransactions > training ||
			partition.EligibleEvaluationTransactions < 0 || partition.EligibleEvaluationTransactions > evaluation {
			return invalid("partition %d eligible counts", partition.Percentile)
		}
		if partition.DisplayedCandidates != displayed[partition.Percentile] {
			return invalid("partition %d displays %d candidates, report holds %d",
				partition.Percentile, partition.DisplayedCandidates, displayed[partition.Percentile])
		}
		delete(displayed, partition.Percentile)
	}
	if len(report.Partitions) > 0 && report.Partitions[len(report.Partitions)-1].Percentile == 90 &&
		report.Partitions[len(report.Partitions)-1].EvaluationLastEvent != events {
		return invalid("the last evaluation window does not end at the chain's last event")
	}
	if len(displayed) > 0 {
		return invalid("candidates name a partition the report does not hold")
	}

	return nil
}

func validateExclusions(report Report) error {
	for index, exclusion := range report.Exclusions {
		if !oneOf(exclusionScope, exclusion.Scope) || !oneOf(exclusionReasons, exclusion.Reason) {
			return invalid("exclusion %s/%s", exclusion.Scope, exclusion.Reason)
		}
		if index > 0 {
			previous := report.Exclusions[index-1]
			if cmp.Or(cmp.Compare(previous.Scope, exclusion.Scope), cmp.Compare(previous.Reason, exclusion.Reason)) >= 0 {
				return invalid("exclusions not in strict scope and reason order at %d", index)
			}
		}
	}

	return nil
}

func validateCandidates(report Report) error {
	for index, candidate := range report.Candidates {
		if !oneOf(origins, candidate.Origin) {
			return invalid("candidate origin %q", candidate.Origin)
		}
		if candidate.ID != CandidateID(candidate.Origin, string(candidate.Antecedent), string(candidate.Consequent)) {
			return invalid("candidate %d id does not match its identity", index)
		}
		if candidate.Rank < 1 || candidate.RawSupport > candidate.RawOpportunity {
			return invalid("candidate %s rank %d support %d opportunity %d",
				candidate.ID, candidate.Rank, candidate.RawSupport, candidate.RawOpportunity)
		}
		if candidate.Supporting.TotalEvents != candidate.RawSupport ||
			candidate.Exceptions.TotalEvents != candidate.RawOpportunity-candidate.RawSupport {
			return invalid("candidate %s evidence totals disagree with its raw counts", candidate.ID)
		}
		if err := validateEvidence(candidate.Supporting); err != nil {
			return err
		}
		if err := validateEvidence(candidate.Exceptions); err != nil {
			return err
		}
		if index > 0 && compareCandidates(report.Candidates[index-1], candidate) >= 0 {
			return invalid("candidates not in strict configuration, partition, rank, and ID order at %d", index)
		}
	}

	return nil
}

func compareCandidates(left, right Candidate) int {
	return cmp.Or(
		cmp.Compare(left.ConfigurationID, right.ConfigurationID),
		cmp.Compare(left.Partition, right.Partition),
		cmp.Compare(left.Rank, right.Rank),
		cmp.Compare(left.ID, right.ID),
	)
}

func validateEvidence(evidence Evidence) error {
	if len(evidence.Events) > maxEvidence || uint64(len(evidence.Events)) > evidence.TotalEvents {
		return invalid("evidence holds %d of %d events", len(evidence.Events), evidence.TotalEvents)
	}
	for index, event := range evidence.Events {
		if !isObjectID(event.ObjectID) || !oneOf(eventKinds, event.Kind) || event.Index < 1 ||
			uint64(len(event.Paths)) > event.TotalPaths {
			return invalid("evidence event %d", index)
		}
		if index > 0 {
			previous := evidence.Events[index-1]
			if cmp.Or(cmp.Compare(previous.Index, event.Index), cmp.Compare(previous.ObjectID, event.ObjectID)) >= 0 {
				return invalid("evidence events not in index and object-ID order at %d", index)
			}
		}
		for path, record := range event.Paths {
			if !oneOf(pathStatuses, record.Status) {
				return invalid("evidence path status %q", record.Status)
			}
			if path > 0 && event.Paths[path-1].Path >= record.Path {
				return invalid("evidence paths not in strict byte order at %d", path)
			}
		}
	}

	return nil
}

func validateBaselines(report Report) error {
	for index, baseline := range report.Baselines {
		if !oneOf(baselineIDs, baseline.ID) {
			return invalid("baseline id %q", baseline.ID)
		}
		if baseline.Outcome != nil && !oneOf(gateOutcomes, *baseline.Outcome) {
			return invalid("baseline outcome %q", *baseline.Outcome)
		}
		metrics := append([]Metric{
			baseline.MicroAdherence, baseline.MacroAdherence, baseline.Coverage,
			baseline.CutoffAbstention, baseline.MinimumJaccard,
		}, baseline.RankCorrelationDiagnostics...)
		if err := validateMetrics(metrics...); err != nil {
			return err
		}
		if index > 0 {
			previous := report.Baselines[index-1]
			if cmp.Or(compareOptional(previous.ConfigurationID, baseline.ConfigurationID),
				cmp.Compare(previous.ID, baseline.ID)) >= 0 {
				return invalid("baselines not in strict configuration-ID and ID order at %d", index)
			}
		}
	}

	return nil
}

func validateControls(report Report) error {
	for index, control := range report.Controls {
		if !oneOf(controlIDs, control.ID) || !oneOf(gateOutcomes, control.Outcome) || control.ConfigurationID != nil {
			return invalid("control %q outcome %q", control.ID, control.Outcome)
		}
		if index > 0 && report.Controls[index-1].ID >= control.ID {
			return invalid("controls not in strict ID order at %d", index)
		}
	}

	return nil
}

func validateMetrics(metrics ...Metric) error {
	for _, metric := range metrics {
		if (metric.Value == nil) == (metric.Reason == nil) {
			return invalid("metric must hold exactly one of value and reason")
		}
		if metric.Reason != nil && !oneOf(metricReasons, *metric.Reason) {
			return invalid("metric reason %q", *metric.Reason)
		}
		if metric.Value != nil {
			if _, err := formatDecimal(float64(*metric.Value)); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateMetricRecords(report Report) error {
	for index, record := range report.Metrics {
		metrics := append([]Metric{
			record.MicroAdherence, record.MacroAdherence, record.Coverage,
			record.CutoffAbstention, record.MinimumJaccard,
		}, record.RankCorrelationDiagnostics...)
		if err := validateMetrics(metrics...); err != nil {
			return err
		}
		if index > 0 && report.Metrics[index-1].ConfigurationID >= record.ConfigurationID {
			return invalid("metrics not in strict configuration-ID order at %d", index)
		}
	}

	return nil
}

func validateGates(report Report) error {
	for index, gate := range report.Gates {
		if !oneOf(gateIDs, gate.ID) || !oneOf(gateOutcomes, gate.Outcome) || !oneOf(operators, gate.Operator) {
			return invalid("gate %q outcome %q operator %q", gate.ID, gate.Outcome, gate.Operator)
		}
		if gate.Reason != nil && !oneOf(metricReasons, *gate.Reason) {
			return invalid("gate reason %q", *gate.Reason)
		}
		if err := validateMetrics(gate.Metric); err != nil {
			return err
		}
		if index > 0 {
			previous := report.Gates[index-1]
			if cmp.Or(cmp.Compare(previous.ID, gate.ID),
				compareOptional(previous.ConfigurationID, gate.ConfigurationID)) >= 0 {
				return invalid("gates not in strict ID and configuration-ID order at %d", index)
			}
		}
	}

	return nil
}

func validateLimitations(report Report) error {
	for index, limitation := range report.Limitations {
		if !oneOf(limitationCodes, limitation) {
			return invalid("limitation %q", limitation)
		}
		if index > 0 && report.Limitations[index-1] >= limitation {
			return invalid("limitations not in strict order at %d", index)
		}
	}

	return nil
}

// compareOptional orders a null value before any string.
func compareOptional(left, right *string) int {
	switch {
	case left == nil && right == nil:
		return 0
	case left == nil:
		return -1
	case right == nil:
		return 1
	default:
		return cmp.Compare(*left, *right)
	}
}

func isObjectID(value string) bool {
	return len(value) == 40 && isLowerHex(value)
}

func isDigest(value string) bool {
	return len(value) == 64 && isLowerHex(value)
}

func isLowerHex(value string) bool {
	for index := range len(value) {
		character := value[index]
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}

	return true
}
