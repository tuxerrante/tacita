package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tuxerrante/tacita/internal/report"
)

func TestRunRequiresEveryFlag(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--unknown"},
		{"--tacita", "t", "--runner", "r", "--out", "o"},
		{"--tacita", "t", "--runner", "r", "--out", "o", "--golang-tools", "a", "--kubernetes", "b", "--go-starter", "c"},
		{"--tacita", "t", "--runner", "r", "--out", "o", "extra"},
	} {
		if _, err := run(t.Context(), args); err == nil || !strings.Contains(err.Error(), "usage: tacita-harness") {
			t.Errorf("run(%q) error = %v, want usage", args, err)
		}
	}
}

func TestLimitedBufferStopsAtItsLimit(t *testing.T) {
	buffer := &limitedBuffer{limit: 4}

	n, err := buffer.Write([]byte("abc"))
	if n != 3 || err != nil || buffer.overflowed {
		t.Fatalf("Write(abc) = %d, %v, overflowed %t", n, err, buffer.overflowed)
	}
	n, err = buffer.Write([]byte("def"))
	if n != 1 || err != nil || !buffer.overflowed || buffer.String() != "abcd" {
		t.Errorf("Write(def) = %d, %v, overflowed %t, buffer %q", n, err, buffer.overflowed, buffer.String())
	}
}

func TestFlagNames(t *testing.T) {
	want := []string{"golang-tools", "kubernetes", "go-starter", "proficiency"}
	for index, repository := range report.CalibrationCorpus() {
		if got := flagName(repository); got != want[index] {
			t.Errorf("flagName(%s) = %q, want %q", repository.Name, got, want[index])
		}
	}
}

func TestExitStatusRecordsSignals(t *testing.T) {
	ok := exec.CommandContext(t.Context(), "sh", "-c", "exit 3")
	_ = ok.Run()
	if got := exitStatus(ok.ProcessState); got != 3 {
		t.Errorf("exitStatus(exit 3) = %d", got)
	}
	killed := exec.CommandContext(t.Context(), "sh", "-c", "kill -TERM $$")
	_ = killed.Run()
	if got := exitStatus(killed.ProcessState); got != 128+15 {
		t.Errorf("exitStatus(SIGTERM) = %d, want 143", got)
	}
}

// failedReports writes a failed calibration report for every corpus commit,
// so a fake tacita can print the one its --revision names.
func failedReports(t *testing.T, dir string) {
	t.Helper()

	for _, repository := range report.CalibrationCorpus() {
		encoded, err := report.Encode(report.Report{
			Schema: report.ReportSchema,
			Kind:   "calibration",
			Status: "failed",
			Tool: report.Tool{
				TacitaVersion: "test", TacitaBinarySHA256: strings.Repeat("a", 64), GitVersion: report.FrozenGitVersion,
			},
			Repository: report.Repository{
				ObjectFormat: "sha1", RootCommit: strings.Repeat("1", 40), ResolvedCommit: repository.Commit,
				ReachableCommits: 2, FirstParentEvents: 2, SingleParentEvents: 1,
			},
			Failure: &report.Failure{Code: "budget_unique_pairs", Operation: "calibration"},
		})
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, repository.Commit+".json"), encoded, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// fakeTacita writes a script that answers `calibrate --revision <commit>
// <path>` with the prepared report for that commit, or exits 1 without one.
func fakeTacita(t *testing.T, dir string, body string) string {
	t.Helper()

	path := filepath.Join(dir, "tacita")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func harnessArgs(tacita string, out string) []string {
	return []string{
		"--tacita", tacita, "--runner", "test runner", "--out", out,
		"--golang-tools", "/corpus/tools.git", "--kubernetes", "/corpus/kubernetes.git",
		"--go-starter", "/corpus/go-starter.git", "--proficiency", "/corpus/proficiency.git",
	}
}

func TestHarnessRecordsFailedRuns(t *testing.T) {
	dir := t.TempDir()
	failedReports(t, dir)
	tacita := fakeTacita(t, dir, `cat "`+dir+`/$3.json"; echo diagnostic >&2`)
	out := filepath.Join(dir, "out")

	var stdout, stderr bytes.Buffer
	code := realMain(harnessArgs(tacita, out), &stdout, &stderr)

	if code != 0 {
		t.Fatalf("realMain() = %d, stderr %q", code, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(out, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := report.DecodeEvidenceIndex(data)
	if err != nil {
		t.Fatalf("DecodeEvidenceIndex() error = %v", err)
	}
	if len(index.Artifacts) != 16 || index.Lanes[0].Outcome != "fail" || index.ExperimentOutcome != "fail" {
		t.Errorf("index = %d artifacts, lanes %+v, experiment %s", len(index.Artifacts), index.Lanes, index.ExperimentOutcome)
	}
	sidecar, err := os.ReadFile(filepath.Join(out, "tools.git.2.operation.json"))
	if err != nil {
		t.Fatal(err)
	}
	operation, err := report.DecodeOperation(sidecar)
	if err != nil || operation.ProcessExitStatus != 0 || operation.ReportDigest == nil || operation.PeakResidentBytes <= 0 {
		t.Errorf("sidecar = %+v, %v", operation, err)
	}
	if diagnostic, err := os.ReadFile(filepath.Join(out, "kubernetes.git.1.stderr.txt")); err != nil || string(diagnostic) != "diagnostic\n" {
		t.Errorf("stderr file = %q, %v", diagnostic, err)
	}
	if _, err := os.Stat(filepath.Join(out, "selection.txt")); err != nil {
		t.Errorf("selection summary not written: %v", err)
	}
	if !strings.Contains(stdout.String(), "experiment fail") {
		t.Errorf("summary = %q", stdout.String())
	}
}

func TestHarnessRecordsRunsWithoutReports(t *testing.T) {
	dir := t.TempDir()
	tacita := fakeTacita(t, dir, "echo refused >&2; exit 1")
	out := filepath.Join(dir, "out")

	var stdout, stderr bytes.Buffer
	if code := realMain(harnessArgs(tacita, out), &stdout, &stderr); code != 0 {
		t.Fatalf("realMain() = %d, stderr %q", code, stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(out, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := report.DecodeEvidenceIndex(data)
	if err != nil {
		t.Fatalf("DecodeEvidenceIndex() error = %v", err)
	}
	if len(index.Artifacts) != 8 || *index.Lanes[0].Reason != report.LaneReportNotComplete {
		t.Errorf("index = %d artifacts, lane %+v", len(index.Artifacts), index.Lanes[0])
	}
	if _, err := os.Stat(filepath.Join(out, "tools.git.1.report.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a report was written for a run that produced none: %v", err)
	}
}

func TestHarnessFailsWhenTacitaCannotStart(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")

	var stdout, stderr bytes.Buffer
	if code := realMain(harnessArgs(filepath.Join(t.TempDir(), "missing"), out), &stdout, &stderr); code != 1 {
		t.Errorf("realMain(missing binary) = %d, want 1", code)
	}
}
