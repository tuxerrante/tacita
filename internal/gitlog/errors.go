package gitlog

import (
	"bytes"
	"errors"
	"fmt"
)

var (
	// ErrInvalidInput identifies an empty repository path or revision.
	ErrInvalidInput = errors.New("invalid input")
	// ErrNotARepository identifies a path that is neither a worktree nor a bare
	// repository, which Git would otherwise resolve against an ancestor.
	ErrNotARepository = errors.New("not a Git repository")
	// ErrUnsupportedOS identifies an operating system outside the frozen boundary.
	ErrUnsupportedOS = errors.New("unsupported operating system")
	// ErrUnsupportedArchitecture identifies a CPU architecture outside the frozen boundary.
	ErrUnsupportedArchitecture = errors.New("unsupported architecture")
	// ErrUnsupportedGitVersion identifies an installed Git older than the frozen minimum.
	ErrUnsupportedGitVersion = errors.New("unsupported Git version")
	// ErrUnsupportedObjectFormat identifies a repository that does not use SHA-1.
	ErrUnsupportedObjectFormat = errors.New("unsupported object format")
	// ErrMalformedGitOutput identifies output that violates a command's fixed grammar.
	ErrMalformedGitOutput = errors.New("malformed Git output")
	// ErrGitFailure identifies a Git process that could not start or exited unsuccessfully.
	ErrGitFailure = errors.New("git command failed")
	// ErrGitOutputLimit identifies bounded Git output that exceeded its operation limit.
	ErrGitOutputLimit = errors.New("git output limit exceeded")
)

// GitError reports a failed Git operation without rendering repository-controlled
// stderr. Stderr returns the bounded diagnostic for an operational consumer.
type GitError struct {
	Operation string
	Err       error
	stderr    []byte
}

func (e *GitError) Error() string {
	return fmt.Sprintf("%s: %v", e.Operation, e.Err)
}

func (e *GitError) Unwrap() error {
	return e.Err
}

func (e *GitError) Is(target error) bool {
	return target == ErrGitFailure
}

// Stderr returns a copy of the bounded Git diagnostic.
func (e *GitError) Stderr() []byte {
	return bytes.Clone(e.stderr)
}

// OutputLimitError reports which bounded stream exceeded its limit.
type OutputLimitError struct {
	Operation string
	Stream    string
	Limit     int
}

func (e *OutputLimitError) Error() string {
	return fmt.Sprintf("%s: %s exceeded %d bytes", e.Operation, e.Stream, e.Limit)
}

func (e *OutputLimitError) Is(target error) bool {
	return target == ErrGitOutputLimit
}
