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
	// ErrPairObservationLimit identifies too many directional pair observations
	// in one repository run.
	ErrPairObservationLimit = errors.New("directional pair observation limit exceeded")
	// ErrPairIdentityLimit identifies too many distinct directional pairs in one
	// repository run.
	ErrPairIdentityLimit = errors.New("distinct directional pair limit exceeded")
	// ErrInvalidConfiguration identifies a Configuration outside the frozen
	// 81-point candidate configuration grid.
	ErrInvalidConfiguration = errors.New("invalid candidate configuration")
	// ErrDuplicateComponentIdentity identifies an Aggregate whose Components
	// slice repeats one ComponentID.
	ErrDuplicateComponentIdentity = errors.New("duplicate component identity in aggregate")
	// ErrDuplicateComponentName identifies an Aggregate whose Components slice
	// repeats one byte-identical Name across distinct ComponentID values. The
	// production Snapshot boundary interns components by name and can never
	// produce this; it is rejected as malformed rather than given an
	// unspecified extra ranking key, because the frozen ranking order already
	// assumes distinct component names.
	ErrDuplicateComponentName = errors.New("duplicate component name in aggregate")
	// ErrUnknownComponent identifies an Aggregate Pair that references a
	// ComponentID absent from its Components slice.
	ErrUnknownComponent = errors.New("unknown component identity in aggregate")
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

// ConfigurationError reports a Configuration field whose value is outside the
// frozen 81-point candidate configuration grid.
type ConfigurationError struct {
	Field string
	Value string
}

func (e *ConfigurationError) Error() string {
	return fmt.Sprintf("invalid configuration field %s value %s", e.Field, e.Value)
}

func (e *ConfigurationError) Is(target error) bool {
	return target == ErrInvalidConfiguration
}

// DuplicateComponentIdentityError reports an Aggregate whose Components slice
// repeats one ComponentID.
type DuplicateComponentIdentityError struct {
	ComponentID ComponentID
}

func (e *DuplicateComponentIdentityError) Error() string {
	return fmt.Sprintf("duplicate component identity %d in aggregate", e.ComponentID)
}

func (e *DuplicateComponentIdentityError) Is(target error) bool {
	return target == ErrDuplicateComponentIdentity
}

// DuplicateComponentNameError reports an Aggregate whose Components slice
// repeats one byte-identical Name across distinct ComponentID values.
type DuplicateComponentNameError struct {
	Name string
}

func (e *DuplicateComponentNameError) Error() string {
	return fmt.Sprintf("duplicate component name %q in aggregate", e.Name)
}

func (e *DuplicateComponentNameError) Is(target error) bool {
	return target == ErrDuplicateComponentName
}

// UnknownComponentError reports an Aggregate Pair that references a
// ComponentID absent from its Components slice.
type UnknownComponentError struct {
	ComponentID ComponentID
}

func (e *UnknownComponentError) Error() string {
	return fmt.Sprintf("unknown component identity %d in aggregate", e.ComponentID)
}

func (e *UnknownComponentError) Is(target error) bool {
	return target == ErrUnknownComponent
}
