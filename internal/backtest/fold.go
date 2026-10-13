package backtest

import (
	"fmt"

	"github.com/tuxerrante/tacita/internal/mining"
)

// Fold observes transactions in position order with one accumulator and calls
// visit with a detached training aggregate at each partition boundary, in
// partition order.
//
// The aggregate passed for a partition holds exactly the transactions
// positioned at or before its TrainingEnd. One accumulator serves every cutoff
// because training membership is nested and the first experiment applies no
// decay. Fold continues through every transaction after the last boundary, so
// the pair budgets observe the full history in every ordering.
//
// transactions must be strictly increasing by position and lie within the last
// partition's evaluation window. An accumulator failure stops the fold and is
// returned wrapped with the failing position.
func Fold(
	transactions []Transaction,
	partitions []Partition,
	visit func(Partition, mining.Aggregate) error,
) error {
	if err := validatePartitions(partitions); err != nil {
		return err
	}

	limit := partitions[len(partitions)-1].EvaluationEnd
	accumulator := mining.NewAccumulator()
	next := 0
	previous := 0

	for index, transaction := range transactions {
		if transaction.Position <= previous || transaction.Position > limit {
			return &TransactionOrderError{
				Index:    index,
				Position: transaction.Position,
				Previous: previous,
				Limit:    limit,
			}
		}
		previous = transaction.Position

		for next < len(partitions) && transaction.Position > partitions[next].TrainingEnd {
			if err := visit(partitions[next], accumulator.Snapshot()); err != nil {
				return err
			}
			next++
		}

		if err := accumulator.Observe(transaction.Components); err != nil {
			return fmt.Errorf("observing transaction at position %d: %w", transaction.Position, err)
		}
	}

	for ; next < len(partitions); next++ {
		if err := visit(partitions[next], accumulator.Snapshot()); err != nil {
			return err
		}
	}

	return nil
}
