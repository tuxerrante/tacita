package backtest

import "math"

// Rule is one displayed directional rule. Rules are compared by component
// bytes because component IDs are local to one accumulation.
type Rule struct {
	Antecedent string
	Consequent string
}

// CutoffRules is what one cutoff displayed, in rank order. Abstained reports a
// training partition below the inference floor.
type CutoffRules struct {
	Abstained bool
	Rules     []Rule
}

// RuleResult is one displayed rule's outcome in its evaluation window. A rule
// with zero opportunities has an undefined adherence.
type RuleResult struct {
	Rule          Rule
	Opportunities int
	Adherences    int
}

// Evaluation holds one model's repository metrics over the fixed partitions.
//
// Micro, Macro, and Coverage pool the evaluation windows. Jaccard and
// RankCorrelation hold one value per adjacent cutoff pair, in cutoff order.
type Evaluation struct {
	Micro            Metric
	Macro            Metric
	Coverage         Metric
	CutoffAbstention Metric
	MinimumJaccard   Metric
	Jaccard          []Metric
	RankCorrelation  []Metric
	// Covered counts each window's transactions containing a displayed
	// antecedent; Transactions counts each window's eligible transactions.
	Covered      []int
	Transactions []int
	// Results holds one entry per displayed slot, per cutoff.
	Results [][]RuleResult
}

// Evaluate scores the rules each cutoff displayed against that cutoff's
// evaluation window. cutoffs and windows are in the same partition order.
//
// A holdout opportunity for A -> B is an evaluation transaction containing A;
// it adheres when the same transaction contains B. Every displayed slot counts,
// so a rule displayed twice is scored twice, while Jaccard and rank correlation
// compare identity sets.
func Evaluate(windows []Window, cutoffs []CutoffRules) (Evaluation, error) {
	if len(windows) == 0 || len(windows) != len(cutoffs) {
		return Evaluation{}, &CutoffCountError{Windows: len(windows), Cutoffs: len(cutoffs)}
	}

	evaluation := Evaluation{
		Covered:      make([]int, len(windows)),
		Transactions: make([]int, len(windows)),
		Results:      make([][]RuleResult, len(windows)),
	}

	var opportunities, adherences, scored, displayed, abstained, empty int
	var adherenceSum float64
	for index, window := range windows {
		cutoff := cutoffs[index]
		if cutoff.Abstained {
			abstained++
		}
		if len(cutoff.Rules) == 0 {
			empty++
		}
		displayed += len(cutoff.Rules)

		results := make([]RuleResult, len(cutoff.Rules))
		for slot, rule := range cutoff.Rules {
			antecedent := window.opportunities(rule.Antecedent)
			result := RuleResult{
				Rule:          rule,
				Opportunities: len(antecedent),
				Adherences:    intersectionSize(antecedent, window.opportunities(rule.Consequent)),
			}
			results[slot] = result
			if result.Opportunities == 0 {
				continue
			}
			opportunities += result.Opportunities
			adherences += result.Adherences
			adherenceSum += float64(result.Adherences) / float64(result.Opportunities)
			scored++
		}
		evaluation.Results[index] = results
		evaluation.Covered[index] = covered(window, cutoff.Rules)
		evaluation.Transactions[index] = window.Transactions
	}

	switch {
	case abstained == len(cutoffs):
		evaluation.Micro = undefined(ReasonBelowInferenceFloor)
		evaluation.Macro = undefined(ReasonBelowInferenceFloor)
	case displayed == 0:
		evaluation.Micro = undefined(ReasonNoCandidates)
		evaluation.Macro = undefined(ReasonNoCandidates)
	case scored == 0:
		evaluation.Micro = undefined(ReasonNoOpportunities)
		evaluation.Macro = undefined(ReasonNoOpportunities)
	default:
		evaluation.Micro = defined(float64(adherences) / float64(opportunities))
		evaluation.Macro = defined(adherenceSum / float64(scored))
	}

	evaluation.Coverage = ratio(sum(evaluation.Covered), sum(evaluation.Transactions))
	evaluation.CutoffAbstention = defined(float64(empty) / float64(len(cutoffs)))

	for index := 1; index < len(cutoffs); index++ {
		evaluation.Jaccard = append(evaluation.Jaccard,
			jaccard(cutoffs[index-1].Rules, cutoffs[index].Rules))
		evaluation.RankCorrelation = append(evaluation.RankCorrelation,
			kendallTauB(cutoffs[index-1].Rules, cutoffs[index].Rules))
	}
	evaluation.MinimumJaccard = minimum(evaluation.Jaccard)

	return evaluation, nil
}

// covered counts window transactions containing the antecedent of at least one
// displayed rule.
func covered(window Window, rules []Rule) int {
	if len(rules) == 0 || window.Transactions == 0 {
		return 0
	}

	marked := make([]bool, window.Transactions)
	count := 0
	for _, rule := range rules {
		for _, transaction := range window.opportunities(rule.Antecedent) {
			if !marked[transaction] {
				marked[transaction] = true
				count++
			}
		}
	}

	return count
}

func ratio(numerator, denominator int) Metric {
	if denominator == 0 {
		return undefined(ReasonNoEligibleEvaluationEvents)
	}

	return defined(float64(numerator) / float64(denominator))
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}

	return total
}

// jaccard compares two displayed identity sets. Two empty sets are undefined;
// one empty set against a non-empty one is the defined value 0.
func jaccard(left, right []Rule) Metric {
	leftSet := identitySet(left)
	rightSet := identitySet(right)
	union := len(leftSet)
	shared := 0
	for rule := range rightSet {
		if leftSet[rule] {
			shared++
		} else {
			union++
		}
	}
	if union == 0 {
		return undefined(ReasonEmptyCandidateSets)
	}

	return defined(float64(shared) / float64(union))
}

func identitySet(rules []Rule) map[Rule]bool {
	set := make(map[Rule]bool, len(rules))
	for _, rule := range rules {
		set[rule] = true
	}

	return set
}

// minimum is the smallest adjacent value, or the first undefined value's
// reason when any is undefined, because a missing comparison cannot be
// treated as stable.
func minimum(values []Metric) Metric {
	if len(values) == 0 {
		return undefined(ReasonEmptyCandidateSets)
	}

	lowest := math.Inf(1)
	for _, value := range values {
		if !value.Defined() {
			return value
		}
		lowest = min(lowest, value.Value)
	}

	return defined(lowest)
}

// kendallTauB correlates the ranks of rules displayed at both adjacent
// cutoffs. A rule's rank is its first displayed slot, so neither ranking has
// ties and tau-b reduces to (concordant - discordant) / (n * (n - 1) / 2).
func kendallTauB(left, right []Rule) Metric {
	leftRanks := firstRanks(left)
	rightRanks := firstRanks(right)

	type rankPair struct{ left, right int }
	var shared []rankPair
	for rule, rank := range leftRanks {
		if other, ok := rightRanks[rule]; ok {
			shared = append(shared, rankPair{left: rank, right: other})
		}
	}
	if len(shared) < 2 {
		return undefined(ReasonInsufficientRankOverlap)
	}

	agreement := 0
	for i := range shared {
		for j := i + 1; j < len(shared); j++ {
			if (shared[i].left < shared[j].left) == (shared[i].right < shared[j].right) {
				agreement++
			} else {
				agreement--
			}
		}
	}
	pairs := len(shared) * (len(shared) - 1) / 2

	return defined(float64(agreement) / float64(pairs))
}

func firstRanks(rules []Rule) map[Rule]int {
	ranks := make(map[Rule]int, len(rules))
	for slot, rule := range rules {
		if _, seen := ranks[rule]; !seen {
			ranks[rule] = slot + 1
		}
	}

	return ranks
}
