package backtest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/tuxerrante/tacita/internal/gitlog"
	"github.com/tuxerrante/tacita/internal/mining"
)

func TestShuffleKeyMatchesTheFrozenEncoding(t *testing.T) {
	var repository gitlog.ObjectID
	copy(repository[:], "a231bf3f37761e8955eefba61855da1a526a3eb9")

	key := shuffleKey(repository, testObjectID(1))

	// Computed independently: sha256(b"tacita-shuffle-v1\0" + repository +
	// b"\0" + event), with lowercase hexadecimal object IDs and no trailing NUL.
	const want = "0a910dfb657006ffdc8992c5ec432da4e072eab8808da10e43984d75e50a38b1"
	if got := hex.EncodeToString(key[:]); got != want {
		t.Errorf("shuffleKey() = %s, want %s", got, want)
	}
}

func TestShuffledTransactionsRepositionsByHashOrder(t *testing.T) {
	events := testChain(40)
	var repository gitlog.ObjectID
	copy(repository[:], "d68c5293798322055c6b3ec11cf34d1e8705438f")
	var transactions []Transaction
	for index := 1; index < len(events); index += 3 {
		transactions = append(transactions, Transaction{
			Position:   index + 1,
			Event:      events[index],
			Components: []string{fmt.Sprintf("c%d", index)},
		})
	}

	shuffled, err := ShuffledTransactions(repository, events, transactions)
	if err != nil {
		t.Fatalf("ShuffledTransactions() error = %v", err)
	}

	order := slices.Clone(events)
	slices.SortFunc(order, func(left, right gitlog.Event) int {
		leftKey := sha256.Sum256([]byte("tacita-shuffle-v1\x00" + repository.String() + "\x00" + left.ID.String()))
		rightKey := sha256.Sum256([]byte("tacita-shuffle-v1\x00" + repository.String() + "\x00" + right.ID.String()))
		return bytes.Compare(leftKey[:], rightKey[:])
	})
	want := make(map[gitlog.ObjectID]int, len(order))
	for index, event := range order {
		want[event.ID] = index + 1
	}

	if len(shuffled) != len(transactions) {
		t.Fatalf("returned %d transactions, want %d", len(shuffled), len(transactions))
	}
	for index, transaction := range shuffled {
		if transaction.Position != want[transaction.Event.ID] {
			t.Errorf("event %s at position %d, want %d",
				transaction.Event.ID, transaction.Position, want[transaction.Event.ID])
		}
		if index > 0 && transaction.Position <= shuffled[index-1].Position {
			t.Errorf("positions not increasing at %d", index)
		}
	}
	if transactions[0].Position != 2 {
		t.Error("ShuffledTransactions modified its input")
	}
	if slices.EqualFunc(shuffled, transactions, func(a, b Transaction) bool { return a.Event == b.Event }) {
		t.Error("the hash order kept the chronological order; the test proves nothing")
	}
}

func TestShuffledTransactionsRejectsUnknownEvents(t *testing.T) {
	events := testChain(3)

	_, err := ShuffledTransactions(testObjectID(9), events, []Transaction{
		{Position: 2, Event: gitlog.Event{ID: testObjectID(77)}},
	})

	if !errors.Is(err, ErrUnmatchedTransaction) {
		t.Fatalf("ShuffledTransactions() error = %v, want ErrUnmatchedTransaction", err)
	}
}

func TestMostPrevalentBreaksTiesByComponentBytes(t *testing.T) {
	aggregate := mining.Aggregate{Components: []mining.Component{
		{ID: 0, Name: "b", RawOpportunity: 5},
		{ID: 1, Name: "z", RawOpportunity: 9},
		{ID: 2, Name: "a", RawOpportunity: 5},
		{ID: 3, Name: "c", RawOpportunity: 2},
	}}

	got := mostPrevalent(aggregate)

	if got != (prevalence{first: "z", second: "a"}) {
		t.Errorf("mostPrevalent() = %+v, want z then a", got)
	}
	if empty := mostPrevalent(mining.Aggregate{}); empty != (prevalence{}) {
		t.Errorf("mostPrevalent(empty) = %+v", empty)
	}
}

func TestReplaceConsequentsSkipsTheAntecedentItself(t *testing.T) {
	top := prevalence{first: "z", second: "a"}
	cutoff := CutoffRules{Rules: []Rule{
		{Antecedent: "b", Consequent: "c"},
		{Antecedent: "z", Consequent: "c"},
	}}

	got := top.replaceConsequents(cutoff)

	want := CutoffRules{Rules: []Rule{
		{Antecedent: "b", Consequent: "z"},
		{Antecedent: "z", Consequent: "a"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("replaceConsequents() = %+v, want %+v", got, want)
	}
	if abstained := top.replaceConsequents(CutoffRules{Abstained: true}); !abstained.Abstained {
		t.Error("replaceConsequents() dropped abstention")
	}
}

// correlatedTransactions pairs a and b components so every configuration
// displays candidates.
func correlatedTransactions(eventCount int) []Transaction {
	var transactions []Transaction
	for position := 2; position <= eventCount; position++ {
		group := position % 7
		components := []string{fmt.Sprintf("a%d", group), fmt.Sprintf("b%d", group), "hub"}
		if position%5 == 0 {
			components = []string{fmt.Sprintf("a%d", group), "hub"}
		}
		transactions = append(transactions, Transaction{
			Position:   position,
			Event:      gitlog.Event{ID: testObjectID(position)},
			Components: components,
		})
	}
	return transactions
}

func TestEvaluateOrderingComposesFoldDisplayAndEvaluate(t *testing.T) {
	const eventCount = 600
	transactions := correlatedTransactions(eventCount)
	partitions, err := Partitions(eventCount)
	if err != nil {
		t.Fatalf("Partitions() error = %v", err)
	}
	configurations := mining.AllConfigurations()

	outcomes, err := EvaluateOrdering(t.Context(), transactions, partitions, configurations)
	if err != nil {
		t.Fatalf("EvaluateOrdering() error = %v", err)
	}

	windows, err := Windows(transactions, partitions)
	if err != nil {
		t.Fatalf("Windows() error = %v", err)
	}
	if len(outcomes) != len(configurations) {
		t.Fatalf("returned %d outcomes, want %d", len(outcomes), len(configurations))
	}
	displayed := 0
	for index, outcome := range outcomes {
		if outcome.Configuration != configurations[index] || len(outcome.Displays) != len(partitions) {
			t.Fatalf("outcome %d = %s with %d displays", index, outcome.Configuration.ID(), len(outcome.Displays))
		}
		var rules []CutoffRules
		for _, display := range outcome.Displays {
			rules = append(rules, display.Rules())
			displayed += len(display.Candidates)
		}
		want, err := Evaluate(windows, rules)
		if err != nil {
			t.Fatalf("Evaluate() error = %v", err)
		}
		if !reflect.DeepEqual(outcome.Model, want) {
			t.Errorf("%s model differs from evaluating its displays", outcome.Configuration.ID())
		}
		for cutoff, results := range outcome.Model.Results {
			for slot, result := range results {
				baseline := outcome.FrequentConsequent.Results[cutoff][slot]
				if baseline.Rule.Antecedent != result.Rule.Antecedent ||
					baseline.Opportunities != result.Opportunities {
					t.Errorf("%s cutoff %d slot %d baseline changed the antecedent's opportunities",
						outcome.Configuration.ID(), cutoff, slot)
				}
			}
		}
	}
	if displayed == 0 {
		t.Fatal("no configuration displayed a candidate; the comparison proves nothing")
	}
}

func TestEvaluateOrderingPropagatesFailures(t *testing.T) {
	partitions, err := Partitions(600)
	if err != nil {
		t.Fatalf("Partitions() error = %v", err)
	}
	invalid := []mining.Configuration{{SizeWeight: "none"}}
	if _, err := EvaluateOrdering(t.Context(), correlatedTransactions(600), partitions, invalid); !errors.Is(err, mining.ErrInvalidConfiguration) {
		t.Errorf("EvaluateOrdering(invalid configuration) error = %v", err)
	}
	unordered := []Transaction{{Position: 3, Components: []string{"a"}}, {Position: 2, Components: []string{"b"}}}
	if _, err := EvaluateOrdering(t.Context(), unordered, partitions, mining.AllConfigurations()); !errors.Is(err, ErrTransactionOrder) {
		t.Errorf("EvaluateOrdering(unordered) error = %v", err)
	}
}

func TestUnweightedCounterpart(t *testing.T) {
	weighted := mining.Configuration{
		SizeWeight: mining.WeightPairNormalized, MinimumOpportunities: 10,
		MinimumConfidence: 0.80, MinimumLift: 1.50,
	}

	got, ok := UnweightedCounterpart(weighted)

	want := weighted
	want.SizeWeight = mining.WeightUnit
	if !ok || got != want {
		t.Errorf("UnweightedCounterpart(%s) = %s, %t, want %s, true", weighted.ID(), got.ID(), ok, want.ID())
	}
	if _, ok := UnweightedCounterpart(want); ok {
		t.Errorf("UnweightedCounterpart(%s) reported a separate baseline", want.ID())
	}
}

func TestNoHistoryControlEmitsNoCandidate(t *testing.T) {
	count, err := NoHistoryCandidates(mining.AllConfigurations())
	if err != nil {
		t.Fatalf("NoHistoryCandidates() error = %v", err)
	}
	if count != 0 {
		t.Errorf("NoHistoryCandidates() = %d, want 0", count)
	}

	if _, err := NoHistoryCandidates([]mining.Configuration{{SizeWeight: "none"}}); !errors.Is(err, mining.ErrInvalidConfiguration) {
		t.Errorf("NoHistoryCandidates(invalid) error = %v", err)
	}
}

func TestEvaluateOrderingStopsWhenCancelled(t *testing.T) {
	partitions, err := Partitions(600)
	if err != nil {
		t.Fatalf("Partitions() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = EvaluateOrdering(ctx, correlatedTransactions(600), partitions, mining.AllConfigurations())

	if !errors.Is(err, context.Canceled) {
		t.Errorf("EvaluateOrdering(cancelled) error = %v, want context.Canceled", err)
	}
}
