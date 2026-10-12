package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tuxerrante/tacita/internal/gitlog"
	"github.com/tuxerrante/tacita/internal/mining"
	"github.com/tuxerrante/tacita/internal/report"
)

// newHistory builds a repository of commits paired across directories with
// one fast-import, so the history crosses the inference floor quickly.
func newHistory(t *testing.T, commits int) (string, string) {
	t.Helper()

	repository := filepath.Join(t.TempDir(), "history")
	runGit(t, "", "init", "--quiet", "--bare", "--object-format=sha1", repository)

	var stream strings.Builder
	for index := range commits {
		fmt.Fprintf(&stream, "commit refs/heads/main\nmark :%d\n", index+1)
		fmt.Fprintf(&stream, "committer Tacita Test <tacita@example.invalid> %d +0000\n", 1_700_000_000+index)
		stream.WriteString("data 5\nevent\n")
		if index > 0 {
			fmt.Fprintf(&stream, "from :%d\n", index)
		}
		group := index % 6
		content := fmt.Sprintf("%d\n", index)
		fmt.Fprintf(&stream, "M 100644 inline a%d/file.go\ndata %d\n%s", group, len(content), content)
		if index%5 != 0 {
			fmt.Fprintf(&stream, "M 100644 inline b%d/file.go\ndata %d\n%s", group, len(content), content)
		}
		stream.WriteString("\n")
	}
	runGit(t, stream.String(), "-C", repository, "fast-import", "--quiet")

	return repository, runGit(t, "", "-C", repository, "rev-parse", "refs/heads/main")
}

func runGit(t *testing.T, stdin string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Env = []string{
		"HOME=" + t.TempDir(), "PATH=" + os.Getenv("PATH"), "LC_ALL=C", "TZ=UTC",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
	}
	cmd.Stdin = strings.NewReader(stdin)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func lookupAs(commit string, role string) corpusLookup {
	return func(candidate string) (report.CorpusRepository, bool) {
		if candidate != commit {
			return report.CorpusRepository{}, false
		}
		return report.CorpusRepository{Name: "test", Commit: commit, Role: role}, true
	}
}

func TestCalibrateWritesACompleteReport(t *testing.T) {
	repository, tip := newHistory(t, 160)

	encoded, err := calibrate(t.Context(), repository, "refs/heads/main", lookupAs(tip, report.RoleDevelopment))
	if err != nil {
		t.Fatalf("calibrate() error = %v", err)
	}
	decoded, err := report.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if decoded.Status != "complete" || decoded.Repository.ResolvedCommit != tip ||
		decoded.Repository.FirstParentEvents != 160 || decoded.Repository.ReachableCommits != 160 {
		t.Errorf("report header = %s %+v", decoded.Status, decoded.Repository)
	}
	if len(decoded.Configurations) != len(mining.AllConfigurations()) || len(decoded.Candidates) == 0 {
		t.Errorf("configurations %d candidates %d", len(decoded.Configurations), len(decoded.Candidates))
	}
	if len(decoded.Controls) != 0 {
		t.Errorf("a development report holds controls: %+v", decoded.Controls)
	}
	if len(decoded.Budgets) != len(report.FrozenBudgets()) {
		t.Errorf("budgets = %d, want every frozen budget", len(decoded.Budgets))
	}
	for _, budget := range decoded.Budgets {
		if budget.ID == "first_parent_events" && *budget.Observed != 160 {
			t.Errorf("first_parent_events observed %d, want 160", *budget.Observed)
		}
		if budget.ID == "distinct_components" && *budget.Observed != 12 {
			t.Errorf("distinct_components observed %d, want 12", *budget.Observed)
		}
	}
	if decoded.Tool.GitVersion == "" || len(decoded.Tool.TacitaBinarySHA256) != 64 {
		t.Errorf("tool = %+v", decoded.Tool)
	}

	again, err := calibrate(t.Context(), repository, "refs/heads/main", lookupAs(tip, report.RoleDevelopment))
	if err != nil {
		t.Fatalf("calibrate() again error = %v", err)
	}
	if string(again) != string(encoded) {
		t.Error("two calibrations of one commit produced different bytes")
	}
}

func TestCalibrateRecordsControls(t *testing.T) {
	young, youngTip := newHistory(t, 40)
	encoded, err := calibrate(t.Context(), young, "refs/heads/main", lookupAs(youngTip, report.RoleYoungControl))
	if err != nil {
		t.Fatalf("calibrate(young) error = %v", err)
	}
	decoded, err := report.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	want := report.Control{ID: "young_repository", InferredCandidates: 0, RootEventExcluded: true, Outcome: "pass"}
	if len(decoded.Controls) != 1 || decoded.Controls[0] != want {
		t.Errorf("young controls = %+v, want %+v", decoded.Controls, want)
	}

	template, templateTip := newHistory(t, 160)
	encoded, err = calibrate(t.Context(), template, "refs/heads/main", lookupAs(templateTip, report.RoleTemplateControl))
	if err != nil {
		t.Fatalf("calibrate(template) error = %v", err)
	}
	decoded, err = report.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(decoded.Controls) != 1 || decoded.Controls[0].ID != "template_origin" ||
		decoded.Controls[0].Outcome != "pass" || decoded.Controls[0].InferredCandidates == 0 {
		t.Errorf("template controls = %+v", decoded.Controls)
	}
}

func TestCalibrateRefusesCommitsOutsideTheCorpus(t *testing.T) {
	repository, _ := newHistory(t, 3)

	_, err := calibrate(t.Context(), repository, "refs/heads/main", report.CorpusRepositoryFor)

	if !errors.Is(err, errNotCalibrationCorpus) {
		t.Errorf("calibrate() error = %v, want errNotCalibrationCorpus", err)
	}
}

func TestCalibrateFailsBeforeTheChainWithoutAReport(t *testing.T) {
	repository, tip := newHistory(t, 3)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	encoded, err := calibrate(ctx, repository, "refs/heads/main", lookupAs(tip, report.RoleDevelopment))
	if err == nil || encoded != nil {
		t.Errorf("calibrate(cancelled) = %d bytes, %v; want an error and no report", len(encoded), err)
	}
	if _, err := calibrate(t.Context(), filepath.Join(t.TempDir(), "missing"), "HEAD", report.CorpusRepositoryFor); err == nil {
		t.Error("calibrate(missing repository) succeeded")
	}
	if _, err := calibrate(t.Context(), repository, "refs/heads/absent", report.CorpusRepositoryFor); err == nil {
		t.Error("calibrate(absent revision) succeeded")
	}
}

func TestFailureFor(t *testing.T) {
	tests := []struct {
		err      error
		code     string
		observed int64
		limit    int64
	}{
		{err: &gitlog.PathIdentityLimitError{Observed: 3, Limit: 2}, code: "budget_unique_paths", observed: 3, limit: 2},
		{err: &gitlog.ComponentIdentityLimitError{Observed: 3, Limit: 2}, code: "budget_unique_components", observed: 3, limit: 2},
		{err: fmt.Errorf("wrapped: %w", &mining.PairObservationLimitError{Observed: 5, Limit: 4}), code: "budget_pair_observations", observed: 5, limit: 4},
		{err: &mining.PairIdentityLimitError{Observed: 5, Limit: 4}, code: "budget_unique_pairs", observed: 5, limit: 4},
		{err: &report.LimitError{Code: "budget_report_rows", Observed: 9, Limit: 8}, code: "budget_report_rows", observed: 9, limit: 8},
		{err: &gitlog.OutputLimitError{Stream: "stdout", Limit: 7}, code: "budget_git_stdout", limit: 7},
		{err: &gitlog.OutputLimitError{Stream: "stderr", Limit: 7}, code: "budget_git_stderr", limit: 7},
		{err: context.DeadlineExceeded, code: "budget_elapsed"},
		{err: context.Canceled, code: "cancelled"},
		{err: fmt.Errorf("traversal: %w", gitlog.ErrIncompleteRepository), code: "incomplete_repository"},
		{err: fmt.Errorf("decode: %w", gitlog.ErrMalformedGitOutput), code: "malformed_git_output"},
	}
	for _, tt := range tests {
		failure, ok := failureFor(tt.err)
		if !ok || failure.Code != tt.code || failure.Operation != "calibration" {
			t.Errorf("failureFor(%v) = %+v, %t, want %s", tt.err, failure, ok, tt.code)
			continue
		}
		if tt.observed != 0 && (failure.Observed == nil || *failure.Observed != tt.observed) {
			t.Errorf("failureFor(%v) observed = %v, want %d", tt.err, failure.Observed, tt.observed)
		}
		if tt.limit != 0 && (failure.Limit == nil || *failure.Limit != tt.limit) {
			t.Errorf("failureFor(%v) limit = %v, want %d", tt.err, failure.Limit, tt.limit)
		}
	}

	if _, ok := failureFor(errors.New("unexpected")); ok {
		t.Error("failureFor() named an unclassified error")
	}
}
