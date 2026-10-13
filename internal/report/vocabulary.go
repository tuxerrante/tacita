package report

import "slices"

// Enumeration values the package itself produces or branches on.
const (
	kindCalibration        = "calibration"
	statusComplete         = "complete"
	statusFailed           = "failed"
	outcomePass            = "pass"
	outcomeFail            = "fail"
	outcomeInconclusive    = "inconclusive"
	weightUnit             = "unit"
	weightInverseComponent = "inverse-component"
	weightPairNormalized   = "pair-normalized"
	scopeEvent             = "event"
	scopePath              = "path"
	gateNoHistory          = "no_history_control"
	gateUnweighted         = "unweighted_gain"
	gateFrequentGain       = "frequent_consequent_gain"
	gateShuffledGain       = "shuffled_control_gain"
	baselineNoHistory      = "no_history"
	baselineUnweighted     = "unweighted"
	codeReportBytes        = "budget_report_bytes"
	controlYoungRepository = "young_repository"
	controlTemplateOrigin  = "template_origin"
)

// Frozen enumerations. A value outside them is an encoding and decoding error.
var (
	kinds        = []string{"backtest", kindCalibration, "profile"}
	statuses     = []string{statusComplete, statusFailed, outcomeInconclusive}
	origins      = []string{"inferred"}
	gateOutcomes = []string{outcomeFail, outcomeInconclusive, outcomePass}
	eventKinds   = []string{"merge-result", "root", "single-parent"}
	pathStatuses = []string{"A", "D", "M", "T"}
	baselineIDs  = []string{"frequent_consequent", "hash_shuffled", "no_history", "unweighted"}
	controlIDs   = []string{controlTemplateOrigin, controlYoungRepository}

	gateIDs = []string{
		"adjacent_jaccard",
		"coverage",
		"cutoff_display",
		"evaluation_windows_covered",
		"frequent_consequent_gain",
		"no_history_control",
		"shuffled_control_gain",
		"unweighted_gain",
	}

	metricReasons = []string{
		"below_inference_floor",
		"empty_candidate_sets",
		"identical_baseline",
		"insufficient_rank_overlap",
		"no_candidates",
		"no_eligible_evaluation_events",
		"no_opportunities",
		"profile_not_applicable",
	}

	failureCodes = []string{
		"budget_elapsed",
		"budget_events",
		"budget_git_stderr",
		"budget_git_stdout",
		"budget_memory",
		"budget_pair_observations",
		"budget_report_bytes",
		"budget_report_rows",
		"budget_unique_components",
		"budget_unique_pairs",
		"budget_unique_paths",
		"cancelled",
		"incomplete_repository",
		"internal_invariant",
		"malformed_git_output",
		"malformed_profile_input",
		"profile_helper_failure",
		"unsupported_architecture",
		"unsupported_git_version",
		"unsupported_object_format",
		"unsupported_platform",
	}

	limitationCodes = []string{
		"evidence_metadata_confidential",
		"generated_status_not_inferred",
		"human_inspected_holdout",
		"linux_amd64_only",
		"merge_policy_sensitive",
		"profile_policy_opinion",
		"pull_request_boundaries_unavailable",
		"rename_identity_unavailable",
		"self_attested_freeze",
		"sha1_only",
		"single_configuration_holdout",
		"single_reviewer",
		"small_review_sample",
		"timing_label_coupling",
	}
)

// exclusionPairs are the frozen exclusion scope and reason pairs, in record
// order.
var exclusionPairs = [][2]string{
	{scopeEvent, "event_component_limit"},
	{scopeEvent, "event_path_limit"},
	{scopeEvent, "no_eligible_paths"},
	{scopeEvent, "root_event"},
	{scopePath, "gitlink_path"},
	{scopePath, "vendor_path"},
}

// gateRequirement is a development gate's frozen operator and threshold.
type gateRequirement struct {
	operator  string
	threshold float64
}

// gateRequirements are the frozen development requirements by gate ID.
var gateRequirements = map[string]gateRequirement{
	"adjacent_jaccard":           {operator: ">=", threshold: 0.50},
	"coverage":                   {operator: ">=", threshold: 0.05},
	"cutoff_display":             {operator: ">=", threshold: 1},
	"evaluation_windows_covered": {operator: ">=", threshold: 1},
	gateFrequentGain:             {operator: ">=", threshold: 0.10},
	gateNoHistory:                {operator: "<=", threshold: 0},
	gateShuffledGain:             {operator: ">=", threshold: 0.05},
	gateUnweighted:               {operator: ">=", threshold: 0},
}

// BudgetSpec is one frozen resource budget and the unit it is counted in.
type BudgetSpec struct {
	ID    string
	Unit  string
	Limit int64
	// Unobserved budgets carry a null canonical observation and outcome:
	// runtime, memory, and sidecar size are observed by the experiment
	// harness, and a report cannot contain its own byte size. The encoder still
	// enforces the report byte budget.
	Unobserved bool
}

// Budget units.
const (
	unitBytes      = "bytes"
	unitComponents = "components"
	unitPairs      = "pairs"
	unitPaths      = "paths"
)

// FrozenBudgets returns a detached copy of the frozen per-run resource budgets
// in budget-ID order.
func FrozenBudgets() []BudgetSpec {
	return slices.Clone(budgetSpecs)
}

// budgetSpecs is the canonical budget table that validation trusts. It is
// never handed out, so no caller can change a frozen limit.
var budgetSpecs = []BudgetSpec{
	{ID: "distinct_components", Unit: unitComponents, Limit: 50_000},
	{ID: "distinct_pairs", Unit: unitPairs, Limit: 2_000_000},
	{ID: "distinct_paths", Unit: unitPaths, Limit: 2_000_000},
	{ID: "elapsed_time", Unit: "nanoseconds", Limit: 20 * 60 * 1_000_000_000, Unobserved: true},
	{ID: "event_components", Unit: unitComponents, Limit: 100},
	{ID: "event_paths", Unit: unitPaths, Limit: 2_000},
	{ID: "first_parent_events", Unit: "events", Limit: 200_000},
	{ID: "git_stderr", Unit: unitBytes, Limit: 1 << 20},
	{ID: "git_stdout", Unit: unitBytes, Limit: 1 << 30},
	{ID: "operational_sidecar_bytes", Unit: unitBytes, Limit: 1 << 20, Unobserved: true},
	{ID: "pair_observations", Unit: unitPairs, Limit: 20_000_000},
	{ID: "peak_resident_memory", Unit: unitBytes, Limit: 2 << 30, Unobserved: true},
	{ID: "report_bytes", Unit: unitBytes, Limit: maxReportBytes, Unobserved: true},
	{ID: "report_rows", Unit: "rows", Limit: maxReportRows},
}

func oneOf(values []string, value string) bool {
	return slices.Contains(values, value)
}
