package backtest

import (
	"errors"
	"slices"
	"testing"

	"github.com/tuxerrante/tacita/internal/mining"
)

func TestAtLeast(t *testing.T) {
	tests := []struct {
		name    string
		metric  Metric
		outcome GateOutcome
		reason  Reason
	}{
		{name: "exactly the threshold", metric: defined(0.05), outcome: GatePass},
		{name: "above", metric: defined(0.5), outcome: GatePass},
		{name: "below", metric: defined(0.049999), outcome: GateFail},
		{name: "undefined", metric: undefined(ReasonNoOpportunities), outcome: GateInconclusive, reason: ReasonNoOpportunities},
	}

	for _, tt := range tests {
		got := atLeast(GateCoverage, tt.metric, minimumCoverage)
		if got.Outcome != tt.outcome || got.Reason != tt.reason || got.Metric != tt.metric ||
			got.Operator != ">=" || got.Threshold != 0.05 || got.ID != GateCoverage {
			t.Errorf("%s: atLeast() = %+v, want outcome %s reason %q", tt.name, got, tt.outcome, tt.reason)
		}
	}
}

func TestGainIsTheSmallerDifference(t *testing.T) {
	model := Evaluation{Micro: defined(0.8), Macro: defined(0.6)}
	baseline := Evaluation{Micro: defined(0.5), Macro: defined(0.55)}

	got := gain(model, baseline)

	if !got.Defined() || got.Value < 0.0499999 || got.Value > 0.0500001 {
		t.Errorf("gain() = %+v, want the macro difference 0.05", got)
	}
	undefinedBaseline := Evaluation{Micro: defined(0.5), Macro: undefined(ReasonNoCandidates)}
	if got := gain(model, undefinedBaseline); got != undefined(ReasonNoCandidates) {
		t.Errorf("gain() with an undefined baseline = %+v, want no_candidates", got)
	}
}

func displaysShowing(counts ...int) []Display {
	displays := make([]Display, len(counts))
	for index, count := range counts {
		if count < 0 {
			displays[index] = Display{Abstained: true}
			continue
		}
		displays[index] = Display{Candidates: make([]DisplayedCandidate, count)}
	}
	return displays
}

func TestCutoffDisplayGate(t *testing.T) {
	tests := []struct {
		name    string
		counts  []int
		outcome GateOutcome
		metric  Metric
	}{
		{name: "every cutoff displays", counts: []int{10, 3, 10, 1}, outcome: GatePass, metric: defined(1)},
		{name: "a cutoff above the floor displays nothing", counts: []int{10, 0, 10, 10}, outcome: GateFail, metric: defined(0)},
		{name: "a cutoff below the floor", counts: []int{-1, 10, 10, 10}, outcome: GateInconclusive, metric: undefined(ReasonBelowInferenceFloor)},
		{name: "failure outranks abstention", counts: []int{-1, 0, 10, 10}, outcome: GateFail, metric: defined(0)},
		{name: "every cutoff below the floor", counts: []int{-1, -1, -1, -1}, outcome: GateInconclusive, metric: undefined(ReasonBelowInferenceFloor)},
	}

	for _, tt := range tests {
		got := cutoffDisplayGate(displaysShowing(tt.counts...))
		if got.Outcome != tt.outcome || got.Metric != tt.metric {
			t.Errorf("%s: cutoffDisplayGate() = %+v, want %s %+v", tt.name, got, tt.outcome, tt.metric)
		}
	}
}

func TestWindowsCoveredGate(t *testing.T) {
	tests := []struct {
		name         string
		counts       []int
		covered      []int
		transactions []int
		outcome      GateOutcome
		metric       Metric
	}{
		{
			name: "every window covered", counts: []int{1, 1}, covered: []int{3, 1}, transactions: []int{9, 9},
			outcome: GatePass, metric: defined(1),
		},
		{
			name: "a window uncovered", counts: []int{1, 1}, covered: []int{3, 0}, transactions: []int{9, 9},
			outcome: GateFail, metric: defined(0),
		},
		{
			name: "after an abstaining cutoff", counts: []int{-1, 1}, covered: []int{0, 2}, transactions: []int{9, 9},
			outcome: GateInconclusive, metric: undefined(ReasonBelowInferenceFloor),
		},
		{
			name: "an empty window", counts: []int{1, 1}, covered: []int{2, 0}, transactions: []int{9, 0},
			outcome: GateInconclusive, metric: undefined(ReasonNoEligibleEvaluationEvents),
		},
		{
			name: "failure outranks an empty window", counts: []int{1, 1, 1}, covered: []int{0, 0, 1}, transactions: []int{9, 0, 9},
			outcome: GateFail, metric: defined(0),
		},
	}

	for _, tt := range tests {
		model := Evaluation{Covered: tt.covered, Transactions: tt.transactions}
		got := windowsCoveredGate(displaysShowing(tt.counts...), model)
		if got.Outcome != tt.outcome || got.Metric != tt.metric {
			t.Errorf("%s: windowsCoveredGate() = %+v, want %s %+v", tt.name, got, tt.outcome, tt.metric)
		}
	}
}

func TestConjunction(t *testing.T) {
	pass := Gate{Outcome: GatePass}
	fail := Gate{Outcome: GateFail}
	inconclusive := Gate{Outcome: GateInconclusive}

	tests := []struct {
		gates []Gate
		want  GateOutcome
	}{
		{gates: []Gate{pass, pass}, want: GatePass},
		{gates: []Gate{pass, inconclusive}, want: GateInconclusive},
		{gates: []Gate{inconclusive, fail, pass}, want: GateFail},
	}
	for _, tt := range tests {
		if got := conjunction(tt.gates); got != tt.want {
			t.Errorf("conjunction(%+v) = %s, want %s", tt.gates, got, tt.want)
		}
	}
}

func TestNoHistoryGate(t *testing.T) {
	if got := NoHistoryGate(0); got.Outcome != GatePass || got.Metric != defined(0) || got.Operator != "<=" {
		t.Errorf("NoHistoryGate(0) = %+v, want pass", got)
	}
	if got := NoHistoryGate(3); got.Outcome != GateFail || got.Metric != defined(3) {
		t.Errorf("NoHistoryGate(3) = %+v, want fail", got)
	}
}

func evaluatedOrderings(t *testing.T, configurations []mining.Configuration) ([]Outcome, []Outcome) {
	t.Helper()

	const eventCount = 600
	transactions := correlatedTransactions(eventCount)
	partitions, err := Partitions(eventCount)
	if err != nil {
		t.Fatalf("Partitions() error = %v", err)
	}
	chronological, err := EvaluateOrdering(transactions, partitions, configurations)
	if err != nil {
		t.Fatalf("EvaluateOrdering(chronological) error = %v", err)
	}
	reversed := make([]Transaction, len(transactions))
	for index, transaction := range transactions {
		transaction.Position = eventCount + 2 - transaction.Position
		reversed[len(transactions)-1-index] = transaction
	}
	shuffled, err := EvaluateOrdering(reversed, partitions, configurations)
	if err != nil {
		t.Fatalf("EvaluateOrdering(reordered) error = %v", err)
	}
	return chronological, shuffled
}

func TestDevelopmentGatesDecidesEveryDimension(t *testing.T) {
	configurations := mining.AllConfigurations()
	chronological, shuffled := evaluatedOrderings(t, configurations)

	decided, err := DevelopmentGates(chronological, shuffled)
	if err != nil {
		t.Fatalf("DevelopmentGates() error = %v", err)
	}

	if len(decided) != len(configurations) {
		t.Fatalf("decided %d configurations, want %d", len(decided), len(configurations))
	}
	common := []string{
		GateAdjacentJaccard, GateCoverage, GateCutoffDisplay, GateEvaluationWindowsCovered,
		GateFrequentConsequentGain, GateShuffledControlGain,
	}
	for index, configuration := range decided {
		if configuration.Configuration != configurations[index] {
			t.Fatalf("configuration %d = %s, want %s", index, configuration.Configuration.ID(), configurations[index].ID())
		}
		var ids []string
		for _, gate := range configuration.Gates {
			ids = append(ids, gate.ID)
		}
		want := slices.Clone(common)
		if configuration.Configuration.SizeWeight != mining.WeightUnit {
			want = append(want, GateUnweightedGain)
		}
		if !slices.Equal(ids, want) {
			t.Errorf("%s gates = %v, want %v", configuration.Configuration.ID(), ids, want)
		}
		if configuration.Outcome != conjunction(configuration.Gates) {
			t.Errorf("%s outcome %s is not the conjunction of its gates",
				configuration.Configuration.ID(), configuration.Outcome)
		}
	}
}

func TestDevelopmentGatesComparesTheUnitCounterpart(t *testing.T) {
	configurations := mining.AllConfigurations()
	chronological, shuffled := evaluatedOrderings(t, configurations)
	decided, err := DevelopmentGates(chronological, shuffled)
	if err != nil {
		t.Fatalf("DevelopmentGates() error = %v", err)
	}
	byConfiguration := make(map[mining.Configuration]Outcome)
	for _, outcome := range chronological {
		byConfiguration[outcome.Configuration] = outcome
	}

	for _, configuration := range decided {
		counterpart, ok := UnweightedCounterpart(configuration.Configuration)
		if !ok {
			continue
		}
		index := slices.IndexFunc(configuration.Gates, func(gate Gate) bool { return gate.ID == GateUnweightedGain })
		want := gain(byConfiguration[configuration.Configuration].Model, byConfiguration[counterpart].Model)
		if got := configuration.Gates[index].Metric; got != want {
			t.Errorf("%s unweighted gain = %+v, want %+v", configuration.Configuration.ID(), got, want)
		}
	}
}

func TestDevelopmentGatesRejectsMismatchedInput(t *testing.T) {
	configurations := mining.AllConfigurations()
	chronological, shuffled := evaluatedOrderings(t, configurations)

	if _, err := DevelopmentGates(chronological, shuffled[1:]); !errors.Is(err, ErrOutcomeMismatch) {
		t.Errorf("DevelopmentGates(short shuffled) error = %v, want ErrOutcomeMismatch", err)
	}
	swapped := slices.Clone(shuffled)
	swapped[0], swapped[1] = swapped[1], swapped[0]
	if _, err := DevelopmentGates(chronological, swapped); !errors.Is(err, ErrOutcomeMismatch) {
		t.Errorf("DevelopmentGates(reordered shuffled) error = %v, want ErrOutcomeMismatch", err)
	}

	weightedOnly := slices.DeleteFunc(slices.Clone(configurations), func(c mining.Configuration) bool {
		return c.SizeWeight == mining.WeightUnit
	})
	chronological, shuffled = evaluatedOrderings(t, weightedOnly)
	_, err := DevelopmentGates(chronological, shuffled)
	var missing *MissingCounterpartError
	if !errors.As(err, &missing) || !errors.Is(err, ErrMissingCounterpart) {
		t.Fatalf("DevelopmentGates(no unit configurations) error = %v, want MissingCounterpartError", err)
	}
	if missing.Configuration.SizeWeight != mining.WeightUnit {
		t.Errorf("missing counterpart = %s, want a unit configuration", missing.Configuration.ID())
	}
}

func TestGateErrorMessages(t *testing.T) {
	counterpart := mining.Configuration{
		SizeWeight: mining.WeightUnit, MinimumOpportunities: 5, MinimumConfidence: 0.70, MinimumLift: 1.25,
	}
	tests := []struct {
		err  error
		want string
	}{
		{
			err:  &OutcomeMismatchError{Chronological: 81, Shuffled: 80},
			want: "81 chronological and 80 shuffled outcomes do not cover the same configurations",
		},
		{
			err:  &MissingCounterpartError{Configuration: counterpart},
			want: "unweighted counterpart weight=unit;opportunities=5;confidence=0.70;lift=1.25 was not evaluated",
		},
	}
	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("Error() = %q, want %q", got, tt.want)
		}
	}
}

func TestDevelopmentGatesRejectsMalformedCutoffShapes(t *testing.T) {
	configurations := mining.AllConfigurations()[:1]
	chronological, shuffled := evaluatedOrderings(t, configurations)

	tests := map[string]func(*Outcome){
		"no cutoff": func(o *Outcome) {
			o.Displays = nil
			o.Model.Covered = nil
			o.Model.Transactions = nil
		},
		"extra covered count":       func(o *Outcome) { o.Model.Covered = append(o.Model.Covered, 1) },
		"missing transaction count": func(o *Outcome) { o.Model.Transactions = o.Model.Transactions[1:] },
	}
	for name, mutate := range tests {
		for _, side := range []string{"chronological", "shuffled"} {
			left := slices.Clone(chronological)
			right := slices.Clone(shuffled)
			target := &left[0]
			if side == "shuffled" {
				target = &right[0]
			}
			mutate(target)

			_, err := DevelopmentGates(left, right)

			var shapeErr *OutcomeShapeError
			if !errors.As(err, &shapeErr) || !errors.Is(err, ErrOutcomeShape) {
				t.Errorf("%s %s: DevelopmentGates() error = %v, want OutcomeShapeError", name, side, err)
			}
		}
	}
	got := (&OutcomeShapeError{Configuration: configurations[0], Displays: 4, Covered: 5, Transactions: 4}).Error()
	if want := "outcome " + configurations[0].ID() + " has 4 displays, 5 covered counts, and 4 transaction counts"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
