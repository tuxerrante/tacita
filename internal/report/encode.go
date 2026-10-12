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
	maxDisplayed   = 10
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
		return nil, &LimitError{Code: codeReportBytes, Observed: int64(len(encoded)), Limit: int64(byteLimit)}
	}

	return encoded, nil
}

// Decode parses canonical report bytes. Unknown fields, unknown enum values,
// non-canonical ordering or formatting, and trailing data are errors, so a
// decoded report re-encodes to exactly the input.
func Decode(data []byte) (Report, error) {
	if len(data) > maxReportBytes {
		return Report{}, &LimitError{Code: codeReportBytes, Observed: int64(len(data)), Limit: maxReportBytes}
	}
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
		validateShape,
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

	budgets := make([]string, len(report.Budgets))
	for index, budget := range report.Budgets {
		budgets[index] = budget.ID
	}
	if !slices.Equal(budgets, budgetIDs()) {
		return invalid("complete calibration budgets %v, want the frozen table", budgets)
	}
	if len(report.Exclusions) != len(exclusionPairs) {
		return invalid("complete calibration holds %d exclusion records, want %d",
			len(report.Exclusions), len(exclusionPairs))
	}

	var wantBaselines, wantGates []string
	wantBaselines = append(wantBaselines, recordKey(nil, baselineNoHistory))
	wantGates = append(wantGates, recordKey(nil, gateNoHistory))
	for _, configuration := range report.Configurations {
		id := configuration.ConfigurationID
		for _, baseline := range []string{"frequent_consequent", "hash_shuffled", baselineUnweighted} {
			wantBaselines = append(wantBaselines, recordKey(&id, baseline))
		}
		for _, gate := range gateIDs {
			if gate == gateNoHistory || (gate == gateUnweighted && configuration.SizeWeight == weightUnit) {
				continue
			}
			wantGates = append(wantGates, recordKey(&id, gate))
		}
	}
	gotBaselines := make([]string, len(report.Baselines))
	for index, baseline := range report.Baselines {
		gotBaselines[index] = recordKey(baseline.ConfigurationID, baseline.ID)
	}
	gotGates := make([]string, len(report.Gates))
	for index, gate := range report.Gates {
		gotGates[index] = recordKey(gate.ConfigurationID, gate.ID)
	}
	slices.Sort(wantBaselines)
	slices.Sort(gotBaselines)
	slices.Sort(wantGates)
	slices.Sort(gotGates)
	if !slices.Equal(gotBaselines, wantBaselines) {
		return invalid("complete calibration baselines are not one per frozen baseline and configuration")
	}
	if !slices.Equal(gotGates, wantGates) {
		return invalid("complete calibration gates are not the frozen development gate set")
	}

	return nil
}

// recordKey identifies a baseline or gate record by configuration and ID.
func recordKey(configurationID *string, id string) string {
	if configurationID == nil {
		return "\x00" + id
	}
	return *configurationID + "\x00" + id
}

// budgetIDs lists the frozen budget IDs in order.
func budgetIDs() []string {
	ids := make([]string, len(budgetSpecs))
	for index, spec := range budgetSpecs {
		ids[index] = spec.ID
	}
	return ids
}

// configurationIDs returns the report's configuration IDs.
func configurationIDs(report Report) map[string]bool {
	ids := make(map[string]bool, len(report.Configurations))
	for _, configuration := range report.Configurations {
		ids[configuration.ConfigurationID] = true
	}
	return ids
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
		if !slices.Contains(exclusionPairs, [2]string{exclusion.Scope, exclusion.Reason}) {
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
	configurations := configurationIDs(report)
	for index, candidate := range report.Candidates {
		if !oneOf(origins, candidate.Origin) {
			return invalid("candidate origin %q", candidate.Origin)
		}
		if !configurations[candidate.ConfigurationID] {
			return invalid("candidate %d names configuration %q the report does not hold", index, candidate.ConfigurationID)
		}
		if err := checkRank(report.Candidates, index); err != nil {
			return err
		}
		if candidate.ID != CandidateID(candidate.Origin, string(candidate.Antecedent), string(candidate.Consequent)) {
			return invalid("candidate %d id does not match its identity", index)
		}
		if candidate.RawSupport > candidate.RawOpportunity {
			return invalid("candidate %s support %d above opportunity %d",
				candidate.ID, candidate.RawSupport, candidate.RawOpportunity)
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

// checkRank requires ranks 1 through at most 10, consecutive within each
// configuration and partition, as the display limit produces them.
func checkRank(candidates []Candidate, index int) error {
	candidate := candidates[index]
	want := 1
	if index > 0 {
		previous := candidates[index-1]
		if previous.ConfigurationID == candidate.ConfigurationID && previous.Partition == candidate.Partition {
			want = previous.Rank + 1
		}
	}
	if candidate.Rank != want || candidate.Rank > maxDisplayed {
		return invalid("candidate %d rank %d, want %d within the first %d", index, candidate.Rank, want, maxDisplayed)
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
	configurations := configurationIDs(report)
	for index, baseline := range report.Baselines {
		if !oneOf(baselineIDs, baseline.ID) {
			return invalid("baseline id %q", baseline.ID)
		}
		if (baseline.ID == baselineNoHistory) != (baseline.ConfigurationID == nil) ||
			(baseline.ConfigurationID != nil && !configurations[*baseline.ConfigurationID]) {
			return invalid("baseline %q configuration does not match the report", baseline.ID)
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
	configurations := configurationIDs(report)
	for index, gate := range report.Gates {
		requirement, ok := gateRequirements[gate.ID]
		if !ok || !oneOf(gateOutcomes, gate.Outcome) ||
			gate.Operator != requirement.operator || float64(gate.Threshold) != requirement.threshold {
			return invalid("gate %q outcome %q is not the frozen requirement %s %v",
				gate.ID, gate.Outcome, requirement.operator, requirement.threshold)
		}
		if (gate.ID == gateNoHistory) != (gate.ConfigurationID == nil) ||
			(gate.ConfigurationID != nil && !configurations[*gate.ConfigurationID]) {
			return invalid("gate %q configuration does not match the report", gate.ID)
		}
		if gate.Reason != nil && !oneOf(metricReasons, *gate.Reason) {
			return invalid("gate reason %q", *gate.Reason)
		}
		if err := validateMetrics(gate.Metric); err != nil {
			return err
		}
		if err := validateGateOutcome(gate); err != nil {
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

// validateGateOutcome requires an outcome its metric supports: a passing or
// failing gate holds a defined metric on the matching side of its threshold
// and no reason, and an inconclusive gate holds an undefined metric whose
// reason it repeats.
func validateGateOutcome(gate Gate) error {
	if gate.Outcome == outcomeInconclusive {
		if gate.Metric.Value != nil || gate.Reason == nil || *gate.Reason != *gate.Metric.Reason {
			return invalid("inconclusive gate %q must repeat its undefined metric's reason", gate.ID)
		}
		return nil
	}
	if gate.Metric.Value == nil || gate.Reason != nil {
		return invalid("%s gate %q needs a defined metric and no reason", gate.Outcome, gate.ID)
	}
	value, threshold := float64(*gate.Metric.Value), float64(gate.Threshold)
	satisfied := value >= threshold
	if gate.Operator == "<=" {
		satisfied = value <= threshold
	}
	if satisfied != (gate.Outcome == outcomePass) {
		return invalid("gate %q outcome %q contradicts its metric and threshold", gate.ID, gate.Outcome)
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
