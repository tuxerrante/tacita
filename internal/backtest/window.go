package backtest

// Window is one partition's eligible evaluation transactions, indexed by
// component so a rule's opportunities and adherences are list lengths and
// intersections rather than scans.
type Window struct {
	Partition    Partition
	Transactions int
	// postings maps each component to the ascending window-local indices of the
	// transactions containing it.
	postings map[string][]int
}

// Windows indexes the eligible transactions of each partition's evaluation
// window, TrainingEnd+1 through EvaluationEnd, in partition order.
//
// transactions must satisfy the same ordering contract as [Fold].
func Windows(transactions []Transaction, partitions []Partition) ([]Window, error) {
	if err := validatePartitions(partitions); err != nil {
		return nil, err
	}

	windows := make([]Window, len(partitions))
	for index, partition := range partitions {
		windows[index] = Window{Partition: partition, postings: make(map[string][]int)}
	}

	limit := partitions[len(partitions)-1].EvaluationEnd
	current := 0
	previous := 0
	for index, transaction := range transactions {
		if err := checkPosition(index, transaction.Position, previous, limit); err != nil {
			return nil, err
		}
		previous = transaction.Position

		for current < len(windows) && transaction.Position > windows[current].Partition.EvaluationEnd {
			current++
		}
		window := &windows[current]
		if transaction.Position <= window.Partition.TrainingEnd {
			continue
		}

		local := window.Transactions
		for _, component := range transaction.Components {
			window.postings[component] = append(window.postings[component], local)
		}
		window.Transactions++
	}

	return windows, nil
}

// opportunities returns the window-local transactions containing component.
func (w Window) opportunities(component string) []int {
	return w.postings[component]
}

// intersectionSize counts the indices two ascending lists share.
func intersectionSize(left, right []int) int {
	count := 0
	for len(left) > 0 && len(right) > 0 {
		switch {
		case left[0] < right[0]:
			left = left[1:]
		case left[0] > right[0]:
			right = right[1:]
		default:
			count++
			left = left[1:]
			right = right[1:]
		}
	}

	return count
}
