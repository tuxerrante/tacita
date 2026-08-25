package mining

import (
	"errors"
	"fmt"
)

var (
	// ErrEmptyTransaction identifies an input that cannot be an eligible
	// component transaction.
	ErrEmptyTransaction = errors.New("empty component transaction")
	// ErrDuplicateComponent identifies a transaction whose component set was not
	// deduplicated by the ingestion boundary.
	ErrDuplicateComponent = errors.New("duplicate component in transaction")
	// ErrComponentIdentityLimit identifies too many distinct components in the
	// accumulated state.
	ErrComponentIdentityLimit = errors.New("distinct component identity limit exceeded")
	// ErrPairObservationLimit identifies too many directional pair observations
	// in one repository run.
	ErrPairObservationLimit = errors.New("directional pair observation limit exceeded")
	// ErrPairIdentityLimit identifies too many distinct directional pairs in one
	// repository run.
	ErrPairIdentityLimit = errors.New("distinct directional pair limit exceeded")
)

// DuplicateComponentError reports a transaction that repeats one component.
type DuplicateComponentError struct {
	First     int
	Duplicate int
}

func (e *DuplicateComponentError) Error() string {
	return fmt.Sprintf(
		"component at index %d duplicates index %d",
		e.Duplicate,
		e.First,
	)
}

func (e *DuplicateComponentError) Is(target error) bool {
	return target == ErrDuplicateComponent
}

// ComponentIdentityLimitError reports accumulated state that would exceed the
// frozen distinct component budget.
type ComponentIdentityLimitError struct {
	Observed int
	Limit    int
}

func (e *ComponentIdentityLimitError) Error() string {
	return fmt.Sprintf(
		"distinct component identities reached %d, limit %d",
		e.Observed,
		e.Limit,
	)
}

func (e *ComponentIdentityLimitError) Is(target error) bool {
	return target == ErrComponentIdentityLimit
}

// PairObservationLimitError reports a transaction that would exceed the frozen
// directional pair observation budget.
type PairObservationLimitError struct {
	Observed int
	Limit    int
}

func (e *PairObservationLimitError) Error() string {
	return fmt.Sprintf(
		"directional pair observations reached %d, limit %d",
		e.Observed,
		e.Limit,
	)
}

func (e *PairObservationLimitError) Is(target error) bool {
	return target == ErrPairObservationLimit
}

// PairIdentityLimitError reports a transaction that would exceed the frozen
// distinct directional pair budget.
type PairIdentityLimitError struct {
	Observed int
	Limit    int
}

func (e *PairIdentityLimitError) Error() string {
	return fmt.Sprintf(
		"distinct directional pairs reached %d, limit %d",
		e.Observed,
		e.Limit,
	)
}

func (e *PairIdentityLimitError) Is(target error) bool {
	return target == ErrPairIdentityLimit
}
