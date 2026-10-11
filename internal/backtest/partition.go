// Package backtest composes the frozen temporal evaluation of one repository's
// integration history over the pure descriptive miner.
package backtest

// maxEvents is the frozen cap on first-parent integration events scanned in
// one run. Ingestion enforces it; here it also bounds p * N below overflow.
const maxEvents = 200_000

// cutoffPercentiles are the frozen expanding-history cutoffs in report order.
var cutoffPercentiles = [...]int{60, 70, 80, 90}

// Partition is one frozen cutoff over a one-based event ordering.
//
// Training covers events 1 through TrainingEnd, and evaluation covers
// TrainingEnd+1 through EvaluationEnd. Indices count every event, root and
// excluded events included, so no exclusion or weight mode can move a
// boundary.
type Partition struct {
	Percentile    int
	TrainingEnd   int
	EvaluationEnd int
}

// Partitions returns the four frozen partitions of an ordering of eventCount
// events, b(p) = floor(p * N / 100), with the last evaluation window ending at
// event N.
func Partitions(eventCount int) ([]Partition, error) {
	if eventCount < 1 || eventCount > maxEvents {
		return nil, &EventCountError{Count: eventCount}
	}

	partitions := make([]Partition, len(cutoffPercentiles))
	for index, percentile := range cutoffPercentiles {
		evaluationEnd := eventCount
		if index+1 < len(cutoffPercentiles) {
			evaluationEnd = boundary(cutoffPercentiles[index+1], eventCount)
		}
		partitions[index] = Partition{
			Percentile:    percentile,
			TrainingEnd:   boundary(percentile, eventCount),
			EvaluationEnd: evaluationEnd,
		}
	}

	return partitions, nil
}

func boundary(percentile int, eventCount int) int {
	return percentile * eventCount / 100
}

// validatePartitions requires training ends that never move backwards and
// evaluation windows that start where their training ends and do not overlap.
func validatePartitions(partitions []Partition) error {
	if len(partitions) == 0 {
		return &PartitionError{Index: 0}
	}

	for index, partition := range partitions {
		if partition.TrainingEnd < 0 || partition.EvaluationEnd < partition.TrainingEnd {
			return &PartitionError{Index: index}
		}
		if index > 0 && partition.TrainingEnd != partitions[index-1].EvaluationEnd {
			return &PartitionError{Index: index}
		}
	}

	return nil
}
