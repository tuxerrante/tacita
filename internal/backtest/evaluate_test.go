package backtest

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
)

func TestWindowsIndexesEachEvaluationWindow(t *testing.T) {
	partitions := []Partition{
		{Percentile: 60, TrainingEnd: 2, EvaluationEnd: 4},
		{Percentile: 70, TrainingEnd: 4, EvaluationEnd: 6},
	}
	transactions := []Transaction{
		{Position: 1, Components: []string{"training"}},
		{Position: 2, Components: []string{"training"}},
		{Position: 3, Components: []string{"a", "b"}},
		{Position: 4, Components: []string{"a"}},
		{Position: 6, Components: []string{"b"}},
	}

	windows, err := Windows(transactions, partitions)
	if err != nil {
		t.Fatalf("Windows() error = %v", err)
	}

	want := []Window{
		{
			Partition:    partitions[0],
			Transactions: 2,
			postings:     map[string][]int{"a": {0, 1}, "b": {0}},
		},
		{
			Partition:    partitions[1],
			Transactions: 1,
			postings:     map[string][]int{"b": {0}},
		},
	}
	if !reflect.DeepEqual(windows, want) {
		t.Errorf("Windows() = %+v, want %+v", windows, want)
	}
}

func TestWindowsRejectsInvalidInput(t *testing.T) {
	partitions := []Partition{{Percentile: 90, TrainingEnd: 2, EvaluationEnd: 4}}

	if _, err := Windows(nil, nil); !errors.Is(err, ErrInvalidPartitions) {
		t.Errorf("Windows(nil partitions) error = %v, want ErrInvalidPartitions", err)
	}
	unordered := []Transaction{{Position: 3}, {Position: 3}}
	if _, err := Windows(unordered, partitions); !errors.Is(err, ErrTransactionOrder) {
		t.Errorf("Windows(repeated position) error = %v, want ErrTransactionOrder", err)
	}
	beyond := []Transaction{{Position: 5}}
	if _, err := Windows(beyond, partitions); !errors.Is(err, ErrTransactionOrder) {
		t.Errorf("Windows(position beyond ordering) error = %v, want ErrTransactionOrder", err)
	}
}

func TestIntersectionSize(t *testing.T) {
	tests := []struct {
		left, right []int
		want        int
	}{
		{left: nil, right: []int{1}, want: 0},
		{left: []int{0, 2, 4}, right: []int{1, 3}, want: 0},
		{left: []int{0, 2, 4, 6}, right: []int{2, 3, 6}, want: 2},
		{left: []int{5}, right: []int{5}, want: 1},
	}

	for _, tt := range tests {
		if got := intersectionSize(tt.left, tt.right); got != tt.want {
			t.Errorf("intersectionSize(%v, %v) = %d, want %d", tt.left, tt.right, got, tt.want)
		}
	}
}

// twoWindows holds two evaluation windows whose expected metrics are worked
// out by hand in TestEvaluateComputesPooledMetrics.
func twoWindows(t *testing.T) []Window {
	t.Helper()

	partitions := []Partition{
		{Percentile: 60, TrainingEnd: 1, EvaluationEnd: 5},
		{Percentile: 70, TrainingEnd: 5, EvaluationEnd: 8},
	}
	transactions := []Transaction{
		{Position: 2, Components: []string{"a", "b"}},
		{Position: 3, Components: []string{"a"}},
		{Position: 4, Components: []string{"c", "d"}},
		{Position: 5, Components: []string{"e"}},
		{Position: 6, Components: []string{"a", "b"}},
		{Position: 7, Components: []string{"c"}},
		{Position: 8, Components: []string{"f"}},
	}
	windows, err := Windows(transactions, partitions)
	if err != nil {
		t.Fatalf("Windows() error = %v", err)
	}
	return windows
}

func TestEvaluateComputesPooledMetrics(t *testing.T) {
	cutoffs := []CutoffRules{
		{Rules: []Rule{{Antecedent: "a", Consequent: "b"}, {Antecedent: "c", Consequent: "d"}}},
		{Rules: []Rule{{Antecedent: "c", Consequent: "d"}, {Antecedent: "a", Consequent: "b"}}},
	}

	got, err := Evaluate(twoWindows(t), cutoffs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	// Window 1: a->b 1/2, c->d 1/1, covered 3 of 4.
	// Window 2: c->d 0/1, a->b 1/1, covered 2 of 3.
	// Micro 3/5. Macro (0.5 + 1 + 0 + 1) / 4. Coverage 5/7.
	// Same identity set at both cutoffs in reversed order: Jaccard 1, tau -1.
	want := Evaluation{
		Micro:            defined(3.0 / 5.0),
		Macro:            defined(2.5 / 4.0),
		Coverage:         defined(5.0 / 7.0),
		CutoffAbstention: defined(0),
		MinimumJaccard:   defined(1),
		Jaccard:          []Metric{defined(1)},
		RankCorrelation:  []Metric{defined(-1)},
		Covered:          []int{3, 2},
		Transactions:     []int{4, 3},
		Results: [][]RuleResult{
			{
				{Rule: cutoffs[0].Rules[0], Opportunities: 2, Adherences: 1},
				{Rule: cutoffs[0].Rules[1], Opportunities: 1, Adherences: 1},
			},
			{
				{Rule: cutoffs[1].Rules[0], Opportunities: 1, Adherences: 0},
				{Rule: cutoffs[1].Rules[1], Opportunities: 1, Adherences: 1},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Evaluate() = %+v, want %+v", got, want)
	}
}

func TestEvaluateReportsUndefinedAdherenceReasons(t *testing.T) {
	tests := []struct {
		name    string
		cutoffs []CutoffRules
		reason  Reason
	}{
		{
			name:    "every cutoff below the inference floor",
			cutoffs: []CutoffRules{{Abstained: true}, {Abstained: true}},
			reason:  ReasonBelowInferenceFloor,
		},
		{
			name:    "no displayed candidate above the floor",
			cutoffs: []CutoffRules{{Abstained: true}, {Rules: []Rule{}}},
			reason:  ReasonNoCandidates,
		},
		{
			name: "no future opportunity",
			cutoffs: []CutoffRules{
				{Rules: []Rule{{Antecedent: "z", Consequent: "a"}}},
				{Rules: []Rule{{Antecedent: "y", Consequent: "a"}}},
			},
			reason: ReasonNoOpportunities,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(twoWindows(t), tt.cutoffs)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if got.Micro != undefined(tt.reason) || got.Macro != undefined(tt.reason) {
				t.Errorf("Micro, Macro = %+v, %+v, want both undefined with %s", got.Micro, got.Macro, tt.reason)
			}
		})
	}
}

func TestEvaluateCountsCutoffAbstentionAndEmptyCoverage(t *testing.T) {
	windows := twoWindows(t)
	cutoffs := []CutoffRules{{Abstained: true}, {Rules: []Rule{{Antecedent: "f", Consequent: "a"}}}}

	got, err := Evaluate(windows, cutoffs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if got.CutoffAbstention != defined(0.5) {
		t.Errorf("CutoffAbstention = %+v, want 0.5", got.CutoffAbstention)
	}
	if got.Coverage != defined(1.0/7.0) || !slices.Equal(got.Covered, []int{0, 1}) {
		t.Errorf("Coverage = %+v covered %v, want 1/7 covered [0 1]", got.Coverage, got.Covered)
	}
	if got.Jaccard[0] != defined(0) {
		t.Errorf("Jaccard against an empty cutoff = %+v, want 0", got.Jaccard[0])
	}

	empty := []Window{{Partition: windows[0].Partition}, {Partition: windows[1].Partition}}
	got, err = Evaluate(empty, cutoffs)
	if err != nil {
		t.Fatalf("Evaluate(empty windows) error = %v", err)
	}
	if got.Coverage != undefined(ReasonNoEligibleEvaluationEvents) {
		t.Errorf("Coverage without evaluation events = %+v, want no_eligible_evaluation_events", got.Coverage)
	}
}

func TestEvaluateScoresEverySlotButComparesIdentities(t *testing.T) {
	duplicate := Rule{Antecedent: "a", Consequent: "b"}
	cutoffs := []CutoffRules{
		{Rules: []Rule{duplicate, duplicate}},
		{Rules: []Rule{duplicate}},
	}

	got, err := Evaluate(twoWindows(t), cutoffs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	// Window 1 scores a->b twice (1/2 each); window 2 once (1/1).
	if got.Micro != defined(3.0/5.0) || got.Macro != defined(2.0/3.0) {
		t.Errorf("Micro, Macro = %+v, %+v, want 3/5 and 2/3", got.Micro, got.Macro)
	}
	if got.Jaccard[0] != defined(1) {
		t.Errorf("Jaccard = %+v, want 1 over identity sets", got.Jaccard[0])
	}
	if got.Covered[0] != 2 {
		t.Errorf("Covered[0] = %d, want each transaction counted once", got.Covered[0])
	}
}

func TestJaccardAndMinimum(t *testing.T) {
	a := Rule{Antecedent: "a", Consequent: "b"}
	b := Rule{Antecedent: "b", Consequent: "a"}
	c := Rule{Antecedent: "c", Consequent: "a"}

	tests := []struct {
		name        string
		left, right []Rule
		want        Metric
	}{
		{name: "both empty", want: undefined(ReasonEmptyCandidateSets)},
		{name: "one empty", left: []Rule{a}, want: defined(0)},
		{name: "partial overlap", left: []Rule{a, b}, right: []Rule{b, c}, want: defined(1.0 / 3.0)},
		{name: "direction matters", left: []Rule{a}, right: []Rule{b}, want: defined(0)},
	}
	for _, tt := range tests {
		if got := jaccard(tt.left, tt.right); got != tt.want {
			t.Errorf("%s: jaccard() = %+v, want %+v", tt.name, got, tt.want)
		}
	}

	if got := minimum([]Metric{defined(0.75), defined(0.5), defined(1)}); got != defined(0.5) {
		t.Errorf("minimum() = %+v, want 0.5", got)
	}
	undefinedPair := undefined(ReasonEmptyCandidateSets)
	if got := minimum([]Metric{defined(0.75), undefinedPair}); got != undefinedPair {
		t.Errorf("minimum() with an undefined value = %+v, want it undefined", got)
	}
	if got := minimum(nil); got.Defined() {
		t.Errorf("minimum(nil) = %+v, want undefined", got)
	}
}

func TestKendallTauB(t *testing.T) {
	rules := make([]Rule, 4)
	for index := range rules {
		rules[index] = Rule{Antecedent: fmt.Sprintf("a%d", index), Consequent: "z"}
	}
	reversed := slices.Clone(rules)
	slices.Reverse(reversed)

	tests := []struct {
		name        string
		left, right []Rule
		want        Metric
	}{
		{name: "same order", left: rules, right: rules, want: defined(1)},
		{name: "reversed", left: rules, right: reversed, want: defined(-1)},
		{
			name:  "one swap among survivors",
			left:  rules,
			right: []Rule{rules[1], rules[0], rules[2], {Antecedent: "new", Consequent: "z"}},
			want:  defined(1.0 / 3.0),
		},
		{name: "one survivor", left: rules[:1], right: rules[:1], want: undefined(ReasonInsufficientRankOverlap)},
		{name: "no survivor", left: rules[:2], right: rules[2:], want: undefined(ReasonInsufficientRankOverlap)},
		{
			name:  "a repeated rule keeps its first rank",
			left:  []Rule{rules[0], rules[1], rules[0]},
			right: []Rule{rules[0], rules[1]},
			want:  defined(1),
		},
	}

	for _, tt := range tests {
		got := kendallTauB(tt.left, tt.right)
		if got.Defined() != tt.want.Defined() || got.Reason != tt.want.Reason ||
			math.Abs(got.Value-tt.want.Value) > 1e-12 {
			t.Errorf("%s: kendallTauB() = %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestEvaluateRejectsMismatchedCutoffs(t *testing.T) {
	windows := twoWindows(t)

	for _, cutoffs := range [][]CutoffRules{nil, {{}}, {{}, {}, {}}} {
		_, err := Evaluate(windows, cutoffs)
		var countErr *CutoffCountError
		if !errors.As(err, &countErr) || countErr.Cutoffs != len(cutoffs) || countErr.Windows != 2 {
			t.Errorf("Evaluate(%d cutoffs) error = %v, want CutoffCountError", len(cutoffs), err)
		}
		if !errors.Is(err, ErrCutoffCount) {
			t.Errorf("Evaluate(%d cutoffs) error = %v, want ErrCutoffCount", len(cutoffs), err)
		}
	}
	if _, err := Evaluate(nil, nil); !errors.Is(err, ErrCutoffCount) {
		t.Errorf("Evaluate(no windows) error = %v, want ErrCutoffCount", err)
	}
	if got := (&CutoffCountError{Windows: 4, Cutoffs: 3}).Error(); got != "3 cutoffs for 4 evaluation windows" {
		t.Errorf("Error() = %q", got)
	}
}

// TestEvaluateMatchesABruteForceScan compares the indexed evaluation with a
// direct scan of every evaluation transaction.
func TestEvaluateMatchesABruteForceScan(t *testing.T) {
	const eventCount = 500
	transactions := syntheticTransactions(eventCount, 3)
	partitions, err := Partitions(eventCount)
	if err != nil {
		t.Fatalf("Partitions() error = %v", err)
	}
	windows, err := Windows(transactions, partitions)
	if err != nil {
		t.Fatalf("Windows() error = %v", err)
	}

	random := rand.New(rand.NewPCG(5, 5))
	cutoffs := make([]CutoffRules, len(partitions))
	for index := range cutoffs {
		for range 1 + random.IntN(10) {
			cutoffs[index].Rules = append(cutoffs[index].Rules, Rule{
				Antecedent: fmt.Sprintf("dir%02d", random.IntN(14)),
				Consequent: fmt.Sprintf("dir%02d", random.IntN(14)),
			})
		}
	}

	got, err := Evaluate(windows, cutoffs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	var opportunities, adherences, covered, total int
	for index, partition := range partitions {
		for _, transaction := range transactions {
			if transaction.Position <= partition.TrainingEnd || transaction.Position > partition.EvaluationEnd {
				continue
			}
			total++
			isCovered := false
			for slot, rule := range cutoffs[index].Rules {
				hasAntecedent := slices.Contains(transaction.Components, rule.Antecedent)
				hasBoth := hasAntecedent && slices.Contains(transaction.Components, rule.Consequent)
				if hasAntecedent {
					opportunities++
					isCovered = true
				}
				if hasBoth {
					adherences++
				}
				result := got.Results[index][slot]
				if result.Rule != rule {
					t.Fatalf("result rule %+v, want %+v", result.Rule, rule)
				}
			}
			if isCovered {
				covered++
			}
		}
	}

	if want := defined(float64(adherences) / float64(opportunities)); got.Micro != want {
		t.Errorf("Micro = %+v, want %+v", got.Micro, want)
	}
	if want := defined(float64(covered) / float64(total)); got.Coverage != want {
		t.Errorf("Coverage = %+v, want %+v", got.Coverage, want)
	}
}

func TestDisplayRules(t *testing.T) {
	display := Display{
		Abstained: false,
		Candidates: []DisplayedCandidate{
			{Antecedent: "a", Consequent: "b"},
			{Antecedent: "b", Consequent: "a"},
		},
	}

	got := display.Rules()

	want := CutoffRules{Rules: []Rule{{Antecedent: "a", Consequent: "b"}, {Antecedent: "b", Consequent: "a"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Rules() = %+v, want %+v", got, want)
	}
	if abstained := (Display{Abstained: true}).Rules(); !abstained.Abstained || len(abstained.Rules) != 0 {
		t.Errorf("Rules() of an abstaining display = %+v", abstained)
	}
}
