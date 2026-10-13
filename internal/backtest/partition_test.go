package backtest

import (
	"errors"
	"slices"
	"testing"
)

func TestPartitions(t *testing.T) {
	tests := []struct {
		name       string
		eventCount int
		want       []Partition
	}{
		{
			name:       "golang/tools frozen chain",
			eventCount: 10_693,
			want: []Partition{
				{Percentile: 60, TrainingEnd: 6_415, EvaluationEnd: 7_485},
				{Percentile: 70, TrainingEnd: 7_485, EvaluationEnd: 8_554},
				{Percentile: 80, TrainingEnd: 8_554, EvaluationEnd: 9_623},
				{Percentile: 90, TrainingEnd: 9_623, EvaluationEnd: 10_693},
			},
		},
		{
			name:       "kubernetes frozen chain",
			eventCount: 58_699,
			want: []Partition{
				{Percentile: 60, TrainingEnd: 35_219, EvaluationEnd: 41_089},
				{Percentile: 70, TrainingEnd: 41_089, EvaluationEnd: 46_959},
				{Percentile: 80, TrainingEnd: 46_959, EvaluationEnd: 52_829},
				{Percentile: 90, TrainingEnd: 52_829, EvaluationEnd: 58_699},
			},
		},
		{
			name:       "floor division",
			eventCount: 46,
			want: []Partition{
				{Percentile: 60, TrainingEnd: 27, EvaluationEnd: 32},
				{Percentile: 70, TrainingEnd: 32, EvaluationEnd: 36},
				{Percentile: 80, TrainingEnd: 36, EvaluationEnd: 41},
				{Percentile: 90, TrainingEnd: 41, EvaluationEnd: 46},
			},
		},
		{
			name:       "shortest chain with a named cutoff",
			eventCount: 2,
			want: []Partition{
				{Percentile: 60, TrainingEnd: 1, EvaluationEnd: 1},
				{Percentile: 70, TrainingEnd: 1, EvaluationEnd: 1},
				{Percentile: 80, TrainingEnd: 1, EvaluationEnd: 1},
				{Percentile: 90, TrainingEnd: 1, EvaluationEnd: 2},
			},
		},
		{
			name:       "event cap",
			eventCount: 200_000,
			want: []Partition{
				{Percentile: 60, TrainingEnd: 120_000, EvaluationEnd: 140_000},
				{Percentile: 70, TrainingEnd: 140_000, EvaluationEnd: 160_000},
				{Percentile: 80, TrainingEnd: 160_000, EvaluationEnd: 180_000},
				{Percentile: 90, TrainingEnd: 180_000, EvaluationEnd: 200_000},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Partitions(tt.eventCount)
			if err != nil {
				t.Fatalf("Partitions(%d) error = %v", tt.eventCount, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Partitions(%d) = %+v, want %+v", tt.eventCount, got, tt.want)
			}
			if err := validatePartitions(got); err != nil {
				t.Errorf("validatePartitions(Partitions(%d)) error = %v", tt.eventCount, err)
			}
		})
	}
}

func TestPartitionsRejectsEventCountsOutsideTheCap(t *testing.T) {
	for _, count := range []int{-1, 0, 1, 200_001} {
		got, err := Partitions(count)
		if !errors.Is(err, ErrInvalidEventCount) {
			t.Errorf("Partitions(%d) error = %v, want ErrInvalidEventCount", count, err)
		}
		if got != nil {
			t.Errorf("Partitions(%d) = %+v, want nil", count, got)
		}
		var countErr *EventCountError
		if !errors.As(err, &countErr) || countErr.Count != count {
			t.Errorf("Partitions(%d) error = %#v, want EventCountError{Count: %d}", count, err, count)
		}
	}
}

func TestValidatePartitionsRejectsBrokenNesting(t *testing.T) {
	tests := []struct {
		name       string
		partitions []Partition
		index      int
	}{
		{name: "empty", partitions: nil, index: 0},
		{
			name:       "negative training end",
			partitions: []Partition{{TrainingEnd: -1, EvaluationEnd: 2}},
			index:      0,
		},
		{
			name:       "window ends before training",
			partitions: []Partition{{TrainingEnd: 3, EvaluationEnd: 2}},
			index:      0,
		},
		{
			name: "gap between windows",
			partitions: []Partition{
				{TrainingEnd: 2, EvaluationEnd: 4},
				{TrainingEnd: 5, EvaluationEnd: 6},
			},
			index: 1,
		},
		{
			name: "overlapping windows",
			partitions: []Partition{
				{TrainingEnd: 2, EvaluationEnd: 4},
				{TrainingEnd: 3, EvaluationEnd: 6},
			},
			index: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePartitions(tt.partitions)

			var partitionErr *PartitionError
			if !errors.As(err, &partitionErr) || partitionErr.Index != tt.index {
				t.Fatalf("validatePartitions() error = %v, want PartitionError{Index: %d}", err, tt.index)
			}
			if !errors.Is(err, ErrInvalidPartitions) {
				t.Errorf("validatePartitions() error = %v, want ErrInvalidPartitions", err)
			}
		})
	}
}

func TestErrorMessagesNameTheOffendingValues(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{err: &EventCountError{Count: 1}, want: "integration event count 1, require 2 to 200000"},
		{err: &PartitionError{Index: 2}, want: "partition 2 is not a nested expanding cutoff"},
		{
			err:  &TransactionOrderError{Index: 3, Position: 4, Previous: 5, Limit: 9},
			want: "transaction 3 at position 4 after position 5, limit 9",
		},
		{
			err:  &UnmatchedTransactionError{Event: testObjectID(1)},
			want: "transaction event 0000000000000000000000000000000000000001 is not in the remaining chain",
		},
	}

	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("Error() = %q, want %q", got, tt.want)
		}
	}
}
