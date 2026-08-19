package gitlog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

const (
	maxScalarOutput = 4 << 10
	maxStreamOutput = 1 << 30
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

// boundedReader caps how much of a streamed Git output the run will read. The
// parser above it bounds memory, not total bytes, so without this a repository
// could keep a run busy for as long as it can keep producing valid records.
//
// Reading one byte past the limit is what distinguishes a stream that ends
// exactly at the limit from one that exceeds it.
type boundedReader struct {
	source    io.Reader
	remaining int
	exceeded  bool
	stop      func()
}

func (b *boundedReader) Read(p []byte) (int, error) {
	// A negative remainder would make the short count below negative, which no
	// reader may return. Only a caller defect can produce one, and the stream
	// still fails as a limit rather than becoming a panic further up.
	if b.exceeded || b.remaining < 0 {
		b.exceeded = true
		return 0, errOutputLimit
	}
	if len(p) > b.remaining+1 {
		p = p[:b.remaining+1]
	}

	n, err := b.source.Read(p)
	b.remaining -= n
	if b.remaining < 0 {
		b.exceeded = true
		b.stop()
		return n - 1, errOutputLimit
	}

	return n, err
}

// runStreaming runs a Git command whose output is too large to buffer and hands
// its stdout to parse.
//
// Tacita owns no goroutine: it reads stdout on the calling goroutine, and only
// os/exec's own stdin and stderr copiers run alongside. That makes the teardown
// order load-bearing. Git can block writing stdout while the stdin copier
// blocks writing stdin, and cancelling closes the child's ends of both, which
// releases the copier and lets the drain reach EOF. Waiting on Git before its stdout reaches EOF can block it
// forever on a full pipe, and WaitDelay does not start until the wait begins,
// so a parse that stops early must cancel, drain to EOF, and only then wait.
//
// The drain is deliberately unbounded. Stopping it short would recreate the
// full-pipe deadlock, and the frozen commands disable every repository
// controlled subprocess, so nothing else can hold the pipe open.
func runStreaming(
	ctx context.Context,
	operation string,
	stdin io.Reader,
	limit int,
	parse func(io.Reader) error,
	args ...string,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	runCtx, stop := context.WithCancel(ctx)
	defer stop()

	stderr := newLimitedBuffer(maxGitStderr, stop)

	cmd := exec.CommandContext(runCtx, "git", args...)
	cmd.Env = gitEnvironment()
	cmd.Stdin = stdin
	cmd.Stderr = stderr
	cmd.WaitDelay = gitWaitDelay

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return &GitError{Operation: operation, Err: err}
	}
	if err := cmd.Start(); err != nil {
		return &GitError{Operation: operation, Err: err, stderr: stderr.bytes()}
	}

	stdout := &boundedReader{source: pipe, remaining: limit, stop: stop}
	parseErr := parse(stdout)
	if parseErr != nil {
		stop()
	}
	// The drain is unconditional because waiting behind unread stdout blocks
	// Git on a full pipe whether the parse failed or merely stopped early. The
	// raw pipe is drained rather than the bounded reader, which refuses to read
	// once its limit is gone.
	_, _ = io.Copy(io.Discard, pipe)
	waitErr := cmd.Wait()

	return classifyStream(operation, ctx, limit, stdout, stderr, parseErr, waitErr)
}

// classifyStream picks the one failure worth reporting.
//
// The caller's deadline outranks everything, because a run it gave up on
// explains every other symptom. A limit comes next: exceeding one cancels Git,
// which would otherwise surface as a killed process or a truncated stream. A
// genuine grammar violation outranks the exit status it caused, while a merely
// truncated stream does not, because Git's own diagnosis says more.
func classifyStream(
	operation string,
	ctx context.Context,
	limit int,
	stdout *boundedReader,
	stderr *limitedBuffer,
	parseErr error,
	waitErr error,
) error {
	switch {
	case ctx.Err() != nil:
		return fmt.Errorf("%s: %w", operation, ctx.Err())
	case stdout.exceeded:
		return &OutputLimitError{Operation: operation, Stream: "stdout", Limit: limit}
	case stderr.exceeded:
		return &OutputLimitError{Operation: operation, Stream: "stderr", Limit: maxGitStderr}
	case parseErr != nil && !errors.Is(parseErr, errTruncatedStream):
		return parseErr
	case waitErr != nil:
		return &GitError{Operation: operation, Err: waitErr, stderr: stderr.bytes()}
	default:
		return parseErr
	}
}
