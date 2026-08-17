package gitlog

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

const (
	maxScalarOutput = 4 << 10
	maxGitStderr    = 1 << 20
	gitWaitDelay    = 2 * time.Second
)

type limitedBuffer struct {
	data     []byte
	limit    int
	exceeded bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{
		data:  make([]byte, 0, min(limit, 1024)),
		limit: limit,
	}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - len(b.data)
	if remaining > 0 {
		b.data = append(b.data, p[:min(len(p), remaining)]...)
	}
	if len(p) > remaining {
		b.exceeded = true
	}
	return len(p), nil
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

	stdout := newLimitedBuffer(stdoutLimit)
	stderr := newLimitedBuffer(maxGitStderr)

	cmd := exec.CommandContext(ctx, "git", args...)
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

func repositoryGitArgs(repository string, args ...string) []string {
	base := []string{
		"-C", repository,
		"-c", "core.abbrev=40",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=/dev/null",
		"-c", "diff.external=",
	}
	return append(base, args...)
}
