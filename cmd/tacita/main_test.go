package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"strings"
	"testing"
)

var errWrite = errors.New("write failed")

func TestRun(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      string
		wantUsage bool
	}{
		{
			name: "default revision",
			args: []string{"backtest", "."},
			want: "repository=. revision=HEAD\n",
		},
		{
			name: "explicit revision",
			args: []string{"backtest", "--revision", "main", "."},
			want: "repository=. revision=main\n",
		},
		{
			name: "root help",
			args: []string{"--help"},
			want: rootUsage,
		},
		{
			name: "help",
			args: []string{"backtest", "--help"},
			want: backtestUsage,
		},
		{
			name:      "missing command",
			wantUsage: true,
		},
		{
			name:      "unknown command",
			args:      []string{"unknown"},
			wantUsage: true,
		},
		{
			name:      "missing repository",
			args:      []string{"backtest"},
			wantUsage: true,
		},
		{
			name:      "unknown flag",
			args:      []string{"backtest", "--unknown"},
			wantUsage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer

			err := run(t.Context(), tt.args, &stdout)
			if tt.wantUsage {
				if _, ok := errors.AsType[*usageError](err); !ok {
					t.Fatalf("run() error = %v, want *usageError", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("run() error = %v", err)
			}
			if got := stdout.String(); !strings.Contains(got, tt.want) {
				t.Errorf("stdout = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestRunWriteErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "root help", args: []string{"--help"}},
		{name: "command help", args: []string{"backtest", "--help"}},
		{name: "command output", args: []string{"backtest", "."}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(t.Context(), tt.args, errorWriter{})
			if !errors.Is(err, errWrite) {
				t.Fatalf("run() error = %v, want errWrite", err)
			}
		})
	}
}

func TestRunCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := run(ctx, []string{"backtest", "."}, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run() error = %v, want context.Canceled", err)
	}
}

func TestRealMainSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer

	exitCode := realMain([]string{"backtest", "."}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("realMain() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRealMainPrintsFlagErrorOnce(t *testing.T) {
	var stdout, stderr bytes.Buffer

	exitCode := realMain(
		[]string{"backtest", "--unknown"},
		&stdout,
		&stderr,
	)

	if exitCode != 2 {
		t.Fatalf("realMain() exit code = %d, want 2", exitCode)
	}
	if got := strings.Count(stderr.String(), "flag provided but not defined"); got != 1 {
		t.Errorf("flag error count = %d, want 1; stderr = %q", got, stderr.String())
	}
}

func TestRealMainReturnsOperationalFailureWhenStderrFails(t *testing.T) {
	exitCode := realMain(
		[]string{"backtest", "--unknown"},
		io.Discard,
		errorWriter{},
	)

	if exitCode != 1 {
		t.Fatalf("realMain() exit code = %d, want 1", exitCode)
	}
}

func TestUsageErrorUnwrapsCause(t *testing.T) {
	err := &usageError{detail: backtestUsage, cause: flag.ErrHelp}
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("errors.Is(%v, flag.ErrHelp) = false", err)
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errWrite
}
