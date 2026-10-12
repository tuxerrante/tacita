package report

import "slices"

// Frozen enumerations. A value outside them is an encoding and decoding error.
var (
	kinds          = []string{"backtest", "calibration", "profile"}
	statuses       = []string{"complete", "failed", "inconclusive"}
	origins        = []string{"inferred"}
	sizeWeights    = []string{"inverse-component", "pair-normalized", "unit"}
	gateOutcomes   = []string{"fail", "inconclusive", "pass"}
	operators      = []string{"<=", ">="}
	exclusionScope = []string{"event", "path"}
	eventKinds     = []string{"merge-result", "root", "single-parent"}
	pathStatuses   = []string{"A", "D", "M", "T"}
	baselineIDs    = []string{"frequent_consequent", "hash_shuffled", "no_history", "unweighted"}
	controlIDs     = []string{"template_origin", "young_repository"}

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

	exclusionReasons = []string{
		"event_component_limit",
		"event_path_limit",
		"gitlink_path",
		"no_eligible_paths",
		"root_event",
		"vendor_path",
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

// Budgets lists the frozen per-run resource budgets in budget-ID order.
var Budgets = []BudgetSpec{
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
