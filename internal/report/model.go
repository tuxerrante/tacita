package report

// ReportSchema identifies the frozen analytical report contract.
const ReportSchema = "tacita.report/v1"

// Report is the typed tacita.report/v1 model. Field order is the canonical
// field order. Every array is present, possibly empty; failure is null when
// the report was produced completely.
type Report struct {
	Schema              string               `json:"schema"`
	Kind                string               `json:"kind"`
	Status              string               `json:"status"`
	Tool                Tool                 `json:"tool"`
	Repository          Repository           `json:"repository"`
	Configurations      []Configuration      `json:"configurations"`
	Budgets             []Budget             `json:"budgets"`
	Partitions          []Partition          `json:"partitions"`
	Exclusions          []Exclusion          `json:"exclusions"`
	Candidates          []Candidate          `json:"candidates"`
	Baselines           []Baseline           `json:"baselines"`
	Controls            []Control            `json:"controls"`
	Profiles            []Profile            `json:"profiles"`
	PreregisteredChecks []PreregisteredCheck `json:"preregistered_checks"`
	Metrics             []MetricRecord       `json:"metrics"`
	Gates               []Gate               `json:"gates"`
	Limitations         []string             `json:"limitations"`
	Failure             *Failure             `json:"failure"`
}

// Tool identifies the binaries that produced a report.
type Tool struct {
	TacitaVersion      string  `json:"tacita_version"`
	TacitaBinarySHA256 string  `json:"tacita_binary_sha256"`
	GitVersion         string  `json:"git_version"`
	GoHelperVersion    *string `json:"go_helper_version"`
}

// Repository identifies the analyzed history.
type Repository struct {
	ObjectFormat       string `json:"object_format"`
	RootCommit         string `json:"root_commit"`
	ResolvedCommit     string `json:"resolved_commit"`
	ReachableCommits   int    `json:"reachable_commits"`
	FirstParentEvents  int    `json:"first_parent_events"`
	SingleParentEvents int    `json:"single_parent_events"`
	MergeResultEvents  int    `json:"merge_result_events"`
}

// Configuration is one evaluated grid configuration.
type Configuration struct {
	ConfigurationID       string  `json:"configuration_id"`
	DevelopmentLockDigest *string `json:"development_lock_digest"`
	SizeWeight            string  `json:"size_weight"`
	MinimumOpportunities  uint64  `json:"minimum_opportunities"`
	MinimumConfidence     Decimal `json:"minimum_confidence"`
	MinimumLift           Decimal `json:"minimum_lift"`
}

// Budget is one deterministic resource limit and its observed work. Runtime
// and memory budgets leave observed and outcome null.
type Budget struct {
	ID       string  `json:"id"`
	Unit     string  `json:"unit"`
	Limit    int64   `json:"limit"`
	Observed *int64  `json:"observed"`
	Outcome  *string `json:"outcome"`
}

// Partition is one frozen cutoff and its event ranges, with one-based
// inclusive indices.
type Partition struct {
	Percentile                     int    `json:"percentile"`
	CutoffCommit                   string `json:"cutoff_commit"`
	TrainingFirstEvent             int    `json:"training_first_event"`
	TrainingLastEvent              int    `json:"training_last_event"`
	EvaluationFirstEvent           int    `json:"evaluation_first_event"`
	EvaluationLastEvent            int    `json:"evaluation_last_event"`
	EligibleTrainingTransactions   int    `json:"eligible_training_transactions"`
	EligibleEvaluationTransactions int    `json:"eligible_evaluation_transactions"`
	DisplayedCandidates            int    `json:"displayed_candidates"`
}

// Exclusion counts one exclusion reason.
type Exclusion struct {
	Scope  string `json:"scope"`
	Reason string `json:"reason"`
	Count  uint64 `json:"count"`
}

// Candidate is one displayed inferred candidate at one partition.
type Candidate struct {
	ID                 string   `json:"id"`
	Origin             string   `json:"origin"`
	ConfigurationID    string   `json:"configuration_id"`
	Partition          int      `json:"partition"`
	Rank               int      `json:"rank"`
	Antecedent         Bytes    `json:"antecedent"`
	Consequent         Bytes    `json:"consequent"`
	RawOpportunity     uint64   `json:"raw_opportunity"`
	RawSupport         uint64   `json:"raw_support"`
	WeightedExposure   Decimal  `json:"weighted_exposure"`
	WeightedSupport    Decimal  `json:"weighted_support"`
	WeightedConfidence Decimal  `json:"weighted_confidence"`
	WeightedPrevalence Decimal  `json:"weighted_prevalence"`
	WeightedLift       Decimal  `json:"weighted_lift"`
	Supporting         Evidence `json:"supporting_evidence"`
	Exceptions         Evidence `json:"exception_evidence"`
}

// Evidence is a bounded evidence sample and its pre-sampling event count.
// Calibration reports keep the count and leave the sample empty.
type Evidence struct {
	TotalEvents uint64          `json:"total_events"`
	Events      []EvidenceEvent `json:"events"`
}

// EvidenceEvent is one sampled integration event.
type EvidenceEvent struct {
	ObjectID   string         `json:"object_id"`
	Kind       string         `json:"kind"`
	Index      int            `json:"index"`
	TotalPaths uint64         `json:"total_paths"`
	Paths      []EvidencePath `json:"paths"`
}

// EvidencePath is one sampled changed path and its Git status letter.
type EvidencePath struct {
	Path   Bytes  `json:"path"`
	Status string `json:"status"`
}

// Baseline holds one baseline's or the no-history control's metrics for one
// configuration, or for the repository when ConfigurationID is null.
type Baseline struct {
	ConfigurationID            *string  `json:"configuration_id"`
	ID                         string   `json:"id"`
	MicroAdherence             Metric   `json:"micro_adherence"`
	MacroAdherence             Metric   `json:"macro_adherence"`
	Coverage                   Metric   `json:"coverage"`
	CutoffAbstention           Metric   `json:"cutoff_abstention"`
	MinimumJaccard             Metric   `json:"minimum_jaccard"`
	RankCorrelationDiagnostics []Metric `json:"rank_correlation_diagnostics"`
	Outcome                    *string  `json:"outcome"`
}

// Control records a young-repository or template-origin control outcome.
type Control struct {
	ConfigurationID    *string `json:"configuration_id"`
	ID                 string  `json:"id"`
	InferredCandidates int     `json:"inferred_candidates"`
	RootEventExcluded  bool    `json:"root_event_excluded"`
	Outcome            string  `json:"outcome"`
}

// Profile is a profile-lane record. The profile lane is not implemented, so
// any profile record is a decoding error.
type Profile struct{}

// PreregisteredCheck records a preregistered pair's diagnostics.
type PreregisteredCheck struct {
	ID                 string   `json:"id"`
	Antecedent         Bytes    `json:"antecedent"`
	Consequent         Bytes    `json:"consequent"`
	RawOpportunity     uint64   `json:"raw_opportunity"`
	RawSupport         uint64   `json:"raw_support"`
	WeightedConfidence Decimal  `json:"weighted_confidence"`
	WeightedLift       Decimal  `json:"weighted_lift"`
	Eligible           bool     `json:"eligible"`
	ExcludedBy         []string `json:"excluded_by"`
}

// MetricRecord holds one configuration's repository metrics.
type MetricRecord struct {
	ConfigurationID            string   `json:"configuration_id"`
	MicroAdherence             Metric   `json:"micro_adherence"`
	MacroAdherence             Metric   `json:"macro_adherence"`
	Coverage                   Metric   `json:"coverage"`
	CutoffAbstention           Metric   `json:"cutoff_abstention"`
	MinimumJaccard             Metric   `json:"minimum_jaccard"`
	RankCorrelationDiagnostics []Metric `json:"rank_correlation_diagnostics"`
}

// Metric is a decimal value or null with a stable reason.
type Metric struct {
	Value  *Decimal `json:"value"`
	Reason *string  `json:"reason"`
}

// Gate is one decided engineering dimension.
type Gate struct {
	ConfigurationID *string `json:"configuration_id"`
	ID              string  `json:"id"`
	Outcome         string  `json:"outcome"`
	Metric          Metric  `json:"metric"`
	Operator        string  `json:"operator"`
	Threshold       Decimal `json:"threshold"`
	Reason          *string `json:"reason"`
}

// Failure is a typed report-production failure.
type Failure struct {
	Code             string  `json:"code"`
	Operation        string  `json:"operation"`
	Observed         *int64  `json:"observed"`
	Limit            *int64  `json:"limit"`
	DiagnosticDigest *string `json:"diagnostic_digest"`
}
