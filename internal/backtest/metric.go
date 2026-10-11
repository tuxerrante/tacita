package backtest

// Reason is a frozen undefined-metric reason code from the report vocabulary.
type Reason string

// The undefined-metric reasons temporal evaluation can produce.
const (
	ReasonNoEligibleEvaluationEvents Reason = "no_eligible_evaluation_events"
	ReasonNoOpportunities            Reason = "no_opportunities"
	ReasonNoCandidates               Reason = "no_candidates"
	ReasonBelowInferenceFloor        Reason = "below_inference_floor"
	ReasonEmptyCandidateSets         Reason = "empty_candidate_sets"
	ReasonInsufficientRankOverlap    Reason = "insufficient_rank_overlap"
)

// Metric is a finite value, or no value with the reason it is undefined. An
// undefined metric is never replaced by a success-shaped number.
type Metric struct {
	Value  float64
	Reason Reason
}

// Defined reports whether the metric carries a value.
func (m Metric) Defined() bool {
	return m.Reason == ""
}

func defined(value float64) Metric {
	return Metric{Value: value}
}

func undefined(reason Reason) Metric {
	return Metric{Reason: reason}
}
