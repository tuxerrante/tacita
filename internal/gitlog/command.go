package gitlog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

const (
	maxScalarOutput = 4 << 10
	maxGitStderr    = 1 << 20
	gitWaitDelay    = 2 * time.Second
)

// errOutputLimit stops the copy that feeds a limitedBuffer. It never reaches a
// caller: runGit classifies an overflow from the buffer itself, which carries
// the stream and the limit that were exceeded.
var errOutputLimit = errors.New("git output limit exceeded")

// limitedBuffer captures at most limit bytes of a stream and then fails the
// write. Failing rather than discarding is what bounds the cost of a hostile
// repository: os/exec closes its end of the pipe when the copy fails, and stop
// tears the child down, so the run pays for the copy buffer that crossed the
// limit rather than for the whole overflow.
//
// The bytes captured before the limit are kept, so an over-long stderr still
// diagnoses the failure.
type limitedBuffer struct {
	data     []byte
	limit    int
	exceeded bool
	stop     func()
}

func newLimitedBuffer(limit int, stop func()) *limitedBuffer {
	return &limitedBuffer{
		data:  make([]byte, 0, min(limit, 1024)),
		limit: limit,
		stop:  stop,
	}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - len(b.data)
	if len(p) <= remaining {
		b.data = append(b.data, p...)
		return len(p), nil
	}

	if remaining > 0 {
		b.data = append(b.data, p[:remaining]...)
	}
	b.exceeded = true
	b.stop()

	return remaining, errOutputLimit
}

func (b *limitedBuffer) bytes() []byte {
	return bytes.Clone(b.data)
}

func runGit(
	ctx context.Context,
	operation string,
	stdoutLimit int,
	args ...string,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}

	// The child is killed through a context Tacita owns, so an overflow can
	// tear it down without the caller's context being cancelled, which keeps a
	// self-inflicted stop distinguishable from the caller's deadline below.
	runCtx, stop := context.WithCancel(ctx)
	defer stop()

	stdout := newLimitedBuffer(stdoutLimit, stop)
	stderr := newLimitedBuffer(maxGitStderr, stop)

	cmd := exec.CommandContext(runCtx, "git", args...)
	cmd.Env = gitEnvironment()
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = gitWaitDelay

	err := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("%s: %w", operation, ctxErr)
	}
	if stdout.exceeded {
		return nil, &OutputLimitError{
			Operation: operation,
			Stream:    "stdout",
			Limit:     stdoutLimit,
		}
	}
	if stderr.exceeded {
		return nil, &OutputLimitError{
			Operation: operation,
			Stream:    "stderr",
			Limit:     maxGitStderr,
		}
	}
	if err != nil {
		return nil, &GitError{
			Operation: operation,
			Err:       err,
			stderr:    stderr.bytes(),
		}
	}

	return stdout.bytes(), nil
}

func gitEnvironment() []string {
	return []string{
		"HOME=/nonexistent",
		"LANG=C",
		"LC_ALL=C",
		"TZ=UTC",
		"TERM=dumb",
		"GIT_ASKPASS=/bin/false",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PAGER=cat",
		"GIT_TERMINAL_PROMPT=0",
		"PAGER=cat",
		"SSH_ASKPASS=/bin/false",
	}
}
