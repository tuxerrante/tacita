package backtest

import (
	"errors"
	"fmt"

	"github.com/tuxerrante/tacita/internal/gitlog"
	"github.com/tuxerrante/tacita/internal/mining"
)

var (
	// ErrInvalidEventCount identifies an event ordering that cannot carry the
	// frozen partitions.
	ErrInvalidEventCount = errors.New("invalid integration event count")
	// ErrInvalidPartitions identifies partitions that are not nested expanding
	// cutoffs over one ordering.
	ErrInvalidPartitions = errors.New("invalid temporal partitions")
	// ErrTransactionOrder identifies a transaction whose position is not after
	// its predecessor's or lies outside the ordering.
	ErrTransactionOrder = errors.New("transaction out of position order")
	// ErrUnmatchedTransaction identifies a transaction whose event is not the
	// next unused event of the first-parent chain.
	ErrUnmatchedTransaction = errors.New("transaction does not match the event chain")
	// ErrCutoffCount identifies evaluation input whose cutoffs and windows do
	// not pair one to one.
	ErrCutoffCount = errors.New("cutoffs do not match evaluation windows")
	// ErrOutcomeMismatch identifies chronological and shuffled outcomes that do
	// not cover the same configurations in the same order.
	ErrOutcomeMismatch = errors.New("chronological and shuffled outcomes do not match")
	// ErrMissingCounterpart identifies a weighted configuration whose unit
	// counterpart was not evaluated.
	ErrMissingCounterpart = errors.New("unweighted counterpart not evaluated")
)

// EventCountError reports an event count outside two to the frozen event cap.
type EventCountError struct {
	Count int
}

func (e *EventCountError) Error() string {
	return fmt.Sprintf("integration event count %d, require %d to %d", e.Count, minEvents, maxEvents)
}

func (e *EventCountError) Is(target error) bool {
	return target == ErrInvalidEventCount
}

// PartitionError reports the first partition that breaks the nesting.
type PartitionError struct {
	Index int
}

func (e *PartitionError) Error() string {
	return fmt.Sprintf("partition %d is not a nested expanding cutoff", e.Index)
}

func (e *PartitionError) Is(target error) bool {
	return target == ErrInvalidPartitions
}

// TransactionOrderError reports the first transaction whose position is not
// after its predecessor's or exceeds the ordering's last event.
type TransactionOrderError struct {
	Index    int
	Position int
	Previous int
	Limit    int
}

func (e *TransactionOrderError) Error() string {
	return fmt.Sprintf(
		"transaction %d at position %d after position %d, limit %d",
		e.Index,
		e.Position,
		e.Previous,
		e.Limit,
	)
}

func (e *TransactionOrderError) Is(target error) bool {
	return target == ErrTransactionOrder
}

// UnmatchedTransactionError reports a transaction whose event was absent from
// the remainder of the chain.
type UnmatchedTransactionError struct {
	Event gitlog.ObjectID
}

func (e *UnmatchedTransactionError) Error() string {
	return fmt.Sprintf("transaction event %s is not in the remaining chain", e.Event)
}

func (e *UnmatchedTransactionError) Is(target error) bool {
	return target == ErrUnmatchedTransaction
}

// CutoffCountError reports evaluation input with no windows or with a
// different number of cutoffs and windows.
type CutoffCountError struct {
	Windows int
	Cutoffs int
}

func (e *CutoffCountError) Error() string {
	return fmt.Sprintf("%d cutoffs for %d evaluation windows", e.Cutoffs, e.Windows)
}

func (e *CutoffCountError) Is(target error) bool {
	return target == ErrCutoffCount
}

// OutcomeMismatchError reports chronological and shuffled outcomes of
// different lengths or configuration order.
type OutcomeMismatchError struct {
	Chronological int
	Shuffled      int
}

func (e *OutcomeMismatchError) Error() string {
	return fmt.Sprintf(
		"%d chronological and %d shuffled outcomes do not cover the same configurations",
		e.Chronological,
		e.Shuffled,
	)
}

func (e *OutcomeMismatchError) Is(target error) bool {
	return target == ErrOutcomeMismatch
}

// MissingCounterpartError reports a unit configuration that a weighted
// configuration needs as its unweighted baseline but that was not evaluated.
type MissingCounterpartError struct {
	Configuration mining.Configuration
}

func (e *MissingCounterpartError) Error() string {
	return fmt.Sprintf("unweighted counterpart %s was not evaluated", e.Configuration.ID())
}

func (e *MissingCounterpartError) Is(target error) bool {
	return target == ErrMissingCounterpart
}
