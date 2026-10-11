package backtest

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/tuxerrante/tacita/internal/mining"
)

// syntheticTransactions positions eligible transactions on a chain of
// eventCount events, skipping some events as exclusions do.
func syntheticTransactions(eventCount int, seed uint64) []Transaction {
	random := rand.New(rand.NewPCG(seed, seed))
	var transactions []Transaction
	for position := 2; position <= eventCount; position++ {
		if random.IntN(5) == 0 {
			continue
		}
		size := 1 + random.IntN(4)
		seen := make(map[string]bool, size)
		var components []string
		for range size {
			component := fmt.Sprintf("dir%02d", random.IntN(12))
			if seen[component] {
				continue
			}
			seen[component] = true
			components = append(components, component)
		}
		transactions = append(transactions, Transaction{Position: position, Components: components})
	}
	return transactions
}

func TestFoldSnapshotsEachTrainingPrefix(t *testing.T) {
	const eventCount = 400
	transactions := syntheticTransactions(eventCount, 7)
	partitions, err := Partitions(eventCount)
	if err != nil {
		t.Fatalf("Partitions() error = %v", err)
	}

	var visited []Partition
	err = Fold(transactions, partitions, func(partition Partition, got mining.Aggregate) error {
		visited = append(visited, partition)

		oracle := mining.NewAccumulator()
		for _, transaction := range transactions {
			if transaction.Position > partition.TrainingEnd {
				break
			}
			if err := oracle.Observe(transaction.Components); err != nil {
				t.Fatalf("oracle Observe() error = %v", err)
			}
		}
		if want := oracle.Snapshot(); !reflect.DeepEqual(got, want) {
			t.Errorf("partition %d aggregate differs from a fresh fold of its training prefix",
				partition.Percentile)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Fold() error = %v", err)
	}
	if !reflect.DeepEqual(visited, partitions) {
		t.Errorf("visited %+v, want %+v", visited, partitions)
	}
}

func TestFoldIncludesTheTransactionAtTheTrainingEnd(t *testing.T) {
	partitions := []Partition{
		{Percentile: 60, TrainingEnd: 2, EvaluationEnd: 3},
		{Percentile: 70, TrainingEnd: 3, EvaluationEnd: 4},
	}
	transactions := []Transaction{
		{Position: 2, Components: []string{"a", "b"}},
		{Position: 3, Components: []string{"a"}},
		{Position: 4, Components: []string{"b"}},
	}

	var training []uint64
	err := Fold(transactions, partitions, func(_ Partition, aggregate mining.Aggregate) error {
		training = append(training, aggregate.EligibleTransactions)
		return nil
	})
	if err != nil {
		t.Fatalf("Fold() error = %v", err)
	}
	if want := []uint64{1, 2}; !reflect.DeepEqual(training, want) {
		t.Errorf("training transactions per cutoff = %v, want %v", training, want)
	}
}

func TestFoldVisitsEveryPartitionWithoutTransactions(t *testing.T) {
	partitions, err := Partitions(46)
	if err != nil {
		t.Fatalf("Partitions() error = %v", err)
	}

	visits := 0
	err = Fold(nil, partitions, func(_ Partition, aggregate mining.Aggregate) error {
		visits++
		if aggregate.EligibleTransactions != 0 {
			t.Errorf("empty fold reported %d transactions", aggregate.EligibleTransactions)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Fold() error = %v", err)
	}
	if visits != len(partitions) {
		t.Errorf("visited %d partitions, want %d", visits, len(partitions))
	}
}

// TestFoldObservesTransactionsAfterTheLastCutoff proves the fold reaches the
// final evaluation window: an invalid transaction there still fails it.
func TestFoldObservesTransactionsAfterTheLastCutoff(t *testing.T) {
	partitions := []Partition{{Percentile: 90, TrainingEnd: 2, EvaluationEnd: 5}}
	transactions := []Transaction{
		{Position: 2, Components: []string{"a"}},
		{Position: 5, Components: nil},
	}

	visits := 0
	err := Fold(transactions, partitions, func(Partition, mining.Aggregate) error {
		visits++
		return nil
	})

	if !errors.Is(err, mining.ErrEmptyTransaction) {
		t.Fatalf("Fold() error = %v, want mining.ErrEmptyTransaction", err)
	}
	if visits != 1 {
		t.Errorf("visited %d partitions before the failure, want 1", visits)
	}
}

func TestFoldRejectsTransactionsOutOfPositionOrder(t *testing.T) {
	partitions := []Partition{{Percentile: 90, TrainingEnd: 2, EvaluationEnd: 5}}
	tests := []struct {
		name         string
		transactions []Transaction
		index        int
	}{
		{
			name:         "position zero",
			transactions: []Transaction{{Position: 0, Components: []string{"a"}}},
			index:        0,
		},
		{
			name: "repeated position",
			transactions: []Transaction{
				{Position: 2, Components: []string{"a"}},
				{Position: 2, Components: []string{"b"}},
			},
			index: 1,
		},
		{
			name: "decreasing position",
			transactions: []Transaction{
				{Position: 4, Components: []string{"a"}},
				{Position: 3, Components: []string{"b"}},
			},
			index: 1,
		},
		{
			name:         "beyond the ordering",
			transactions: []Transaction{{Position: 6, Components: []string{"a"}}},
			index:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Fold(tt.transactions, partitions, func(Partition, mining.Aggregate) error {
				return nil
			})

			var orderErr *TransactionOrderError
			if !errors.As(err, &orderErr) || orderErr.Index != tt.index {
				t.Fatalf("Fold() error = %v, want TransactionOrderError{Index: %d}", err, tt.index)
			}
			if !errors.Is(err, ErrTransactionOrder) {
				t.Errorf("Fold() error = %v, want ErrTransactionOrder", err)
			}
		})
	}
}

func TestFoldRejectsInvalidPartitionsBeforeVisiting(t *testing.T) {
	err := Fold(nil, nil, func(Partition, mining.Aggregate) error {
		t.Fatal("visit called for invalid partitions")
		return nil
	})

	if !errors.Is(err, ErrInvalidPartitions) {
		t.Fatalf("Fold() error = %v, want ErrInvalidPartitions", err)
	}
}

func TestFoldReturnsTheVisitorError(t *testing.T) {
	stop := errors.New("stop")
	partitions, err := Partitions(10)
	if err != nil {
		t.Fatalf("Partitions() error = %v", err)
	}

	visits := 0
	err = Fold(syntheticTransactions(10, 1), partitions, func(Partition, mining.Aggregate) error {
		visits++
		return stop
	})

	if !errors.Is(err, stop) {
		t.Fatalf("Fold() error = %v, want the visitor error", err)
	}
	if visits != 1 {
		t.Errorf("visited %d partitions after the visitor failed, want 1", visits)
	}
}

func TestFoldReturnsTheVisitorErrorAfterTheLastTransaction(t *testing.T) {
	stop := errors.New("stop")
	partitions := []Partition{{Percentile: 90, TrainingEnd: 4, EvaluationEnd: 5}}

	err := Fold([]Transaction{{Position: 2, Components: []string{"a"}}}, partitions,
		func(Partition, mining.Aggregate) error { return stop })

	if !errors.Is(err, stop) {
		t.Fatalf("Fold() error = %v, want the visitor error", err)
	}
}
