package report

import (
	"errors"
	"fmt"
)

var (
	// ErrEncode identifies a model that violates the canonical contract.
	ErrEncode = errors.New("invalid canonical artifact")
	// ErrDecode identifies bytes that are not a canonical artifact.
	ErrDecode = errors.New("malformed canonical artifact")
	// ErrLimit identifies an artifact over a frozen row or byte budget.
	ErrLimit = errors.New("canonical artifact budget exceeded")
)

// EncodeError reports the first contract violation found while encoding.
type EncodeError struct {
	Detail string
}

func (e *EncodeError) Error() string {
	return "encoding canonical artifact: " + e.Detail
}

func (e *EncodeError) Is(target error) bool {
	return target == ErrEncode
}

// DecodeError reports why bytes are not a canonical artifact.
type DecodeError struct {
	Detail string
}

func (e *DecodeError) Error() string {
	return "decoding canonical artifact: " + e.Detail
}

func (e *DecodeError) Is(target error) bool {
	return target == ErrDecode
}

// LimitError reports a report row or byte budget breach with its frozen
// failure code.
type LimitError struct {
	Code     string
	Observed int64
	Limit    int64
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("%s: observed %d, limit %d", e.Code, e.Observed, e.Limit)
}

func (e *LimitError) Is(target error) bool {
	return target == ErrLimit
}
