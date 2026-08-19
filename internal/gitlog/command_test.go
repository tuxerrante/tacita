package gitlog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLimitedBufferKeepsPrefixAndFailsOnOverflow pins the write contract the
// teardown depends on: the bytes that fit are kept, the write reports only
// those bytes, and the overflow both fails the copy and stops the child.
func TestLimitedBufferKeepsPrefixAndFailsOnOverflow(t *testing.T) {
	stopped := 0
	buffer := newLimitedBuffer(4, func() { stopped++ })

	if n, err := buffer.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("Write(%q) = (%d, %v), want (3, nil)", "abc", n, err)
	}
	if stopped != 0 {
		t.Fatalf("stop called %d times before the limit, want 0", stopped)
	}

	n, err := buffer.Write([]byte("def"))
	if !errors.Is(err, errOutputLimit) {
		t.Fatalf("Write(%q) error = %v, want errOutputLimit", "def", err)
	}
	if n != 1 {
		t.Errorf("Write(%q) = %d, want 1", "def", n)
	}
	if stopped != 1 {
		t.Errorf("stop called %d times, want 1", stopped)
	}
	if got := string(buffer.bytes()); got != "abcd" {
		t.Errorf("buffer = %q, want %q", got, "abcd")
	}
	if !buffer.exceeded {
		t.Error("buffer.exceeded = false, want true")
	}
}

// TestRunGitClassifiesStdoutOverflow guards the classification the teardown
// could break. Cancelling the child makes Git die of a signal, and reporting
// that instead of the limit would hide why the run failed.
func TestRunGitClassifiesStdoutOverflow(t *testing.T) {
	repository, _ := newTestRepository(t)
	blob := filepath.Join(repository, "large")
	if err := os.WriteFile(blob, []byte(strings.Repeat("a", 8<<20)), 0o600); err != nil {
		t.Fatalf("writing blob: %v", err)
	}
	id := strings.TrimSpace(runTestGit(t, "-C", repository, "hash-object", "-w", "large"))

	_, err := runGit(
		t.Context(),
		"reading a blob",
		maxScalarOutput,
		"-C", repository, "cat-file", "blob", id,
	)

	if !errors.Is(err, ErrGitOutputLimit) {
		t.Fatalf("runGit() error = %v, want ErrGitOutputLimit", err)
	}
	var limit *OutputLimitError
	if !errors.As(err, &limit) || limit.Stream != "stdout" {
		t.Fatalf("runGit() error = %v, want a stdout OutputLimitError", err)
	}
}

// TestRunGitReportsCallerCancellation keeps the caller's context distinguishable
// from the cancellation Tacita performs itself on overflow.
func TestRunGitReportsCallerCancellation(t *testing.T) {
	repository, _ := newTestRepository(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := runGit(ctx, "resolving a revision", maxScalarOutput,
		"-C", repository, "rev-parse", "--verify", "HEAD")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runGit() error = %v, want context.Canceled", err)
	}
	if errors.Is(err, ErrGitOutputLimit) {
		t.Error("runGit() reported an output limit for a cancelled caller")
	}
}
