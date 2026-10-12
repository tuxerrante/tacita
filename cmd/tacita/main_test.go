package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tuxerrante/tacita/internal/report"
)

var errWrite = errors.New("write failed")

func TestRunUsage(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      string
		wantUsage bool
	}{
		{name: "root help", args: []string{"--help"}, want: rootUsage},
		{name: "calibrate help", args: []string{"calibrate", "--help"}, want: calibrateUsage},
		{name: "select help", args: []string{"select", "--help"}, want: "usage: tacita select"},
		{name: "missing command", wantUsage: true},
		{name: "unknown command", args: []string{"backtest", "."}, wantUsage: true},
		{name: "calibrate without revision", args: []string{"calibrate", "."}, wantUsage: true},
		{name: "calibrate without repository", args: []string{"calibrate", "--revision", "HEAD"}, wantUsage: true},
		{name: "calibrate with two repositories", args: []string{"calibrate", "--revision", "HEAD", ".", "."}, wantUsage: true},
		{name: "calibrate unknown flag", args: []string{"calibrate", "--unknown"}, wantUsage: true},
		{name: "select with seven reports", args: []string{"select", "a", "b", "c", "d", "e", "f", "g"}, wantUsage: true},
		{name: "select unknown repository", args: []string{"select", "--operational-failure", "tuxerrante/kapparmor"}, wantUsage: true},
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

func TestRunHelpWriteErrors(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"calibrate", "--help"}, {"select", "--help"}} {
		if err := run(t.Context(), args, errorWriter{}); !errors.Is(err, errWrite) {
			t.Errorf("run(%q) error = %v, want errWrite", args, err)
		}
	}
}

func TestRunCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	for _, args := range [][]string{
		{"calibrate", "--revision", "HEAD", "."},
		{"select", "a", "b", "c", "d", "e", "f", "g", "h"},
	} {
		if err := run(ctx, args, io.Discard); !errors.Is(err, context.Canceled) {
			t.Errorf("run(%q) error = %v, want context.Canceled", args, err)
		}
	}
}

func TestRealMainExitCodes(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := realMain([]string{"--help"}, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
		t.Errorf("realMain(--help) = %d, stderr %q", code, stderr.String())
	}

	stderr.Reset()
	if code := realMain([]string{"calibrate", "--unknown"}, &stdout, &stderr); code != 2 {
		t.Errorf("realMain(unknown flag) = %d, want 2", code)
	}
	if got := strings.Count(stderr.String(), "flag provided but not defined"); got != 1 {
		t.Errorf("flag error printed %d times; stderr = %q", got, stderr.String())
	}

	missing := filepath.Join(t.TempDir(), "missing.json")
	args := []string{"select", missing, missing, missing, missing, missing, missing, missing, missing}
	if code := realMain(args, &stdout, io.Discard); code != 1 {
		t.Errorf("realMain(missing reports) = %d, want 1", code)
	}

	if code := realMain([]string{"calibrate", "--unknown"}, io.Discard, errorWriter{}); code != 1 {
		t.Errorf("realMain() with failing stderr = %d, want 1", code)
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{err: &usageError{detail: rootUsage}, want: 2},
		{err: errNoSelection, want: exitNoSelection},
		{err: errors.New("git failed"), want: 1},
	}
	for _, tt := range tests {
		if got := exitCode(tt.err); got != tt.want {
			t.Errorf("exitCode(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}

func TestUsageErrorUnwrapsCause(t *testing.T) {
	err := &usageError{detail: calibrateUsage, cause: flag.ErrHelp}
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("errors.Is(%v, flag.ErrHelp) = false", err)
	}
}

func TestReadReportEnforcesTheByteBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, make([]byte, maxReportFile+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readReport(path); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("readReport(oversized) error = %v", err)
	}
}

func TestSelectRejectsMalformedReports(t *testing.T) {
	dir := t.TempDir()
	var args []string
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, path)
	}

	err := run(t.Context(), append([]string{"select"}, args...), io.Discard)
	if !errors.Is(err, report.ErrDecode) {
		t.Errorf("run(select malformed) error = %v, want report.ErrDecode", err)
	}
}

func TestSelectionSummary(t *testing.T) {
	selection := report.Selection{
		LaneFailures: []report.LaneFailure{
			{Repository: "b", Reason: report.LaneOperationalLimit},
			{Repository: "a", Reason: report.LaneNondeterministicReport},
		},
		Rejections: []report.Rejection{
			{ConfigurationID: "x", Repository: "a", Gate: "coverage", Outcome: "fail"},
			{ConfigurationID: "x", Repository: "b", Gate: "coverage", Outcome: "fail"},
			{ConfigurationID: "y", Repository: "a", Gate: "adjacent_jaccard", Outcome: "inconclusive"},
			{Repository: "b", Gate: "no_history_control", Outcome: "fail"},
		},
	}

	want := "no selectable configuration\n" +
		"lane_failure a nondeterministic_report\n" +
		"lane_failure b operational_limit\n" +
		"rejected_configurations 2\n" +
		"rejections adjacent_jaccard inconclusive 1\n" +
		"rejections coverage fail 2\n" +
		"rejections no_history_control fail 1\n"
	if got := selectionSummary(selection); got != want {
		t.Errorf("selectionSummary() =\n%s\nwant\n%s", got, want)
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errWrite
}
