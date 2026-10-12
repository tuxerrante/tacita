package gitlog

import (
	"errors"
	"strings"
	"testing"
)

func TestReachableCommitsCountsMergedBranches(t *testing.T) {
	repository, ids := newBranchedTestRepository(t)
	repo := openTestRepository(t, repository)

	got, err := repo.ReachableCommits(t.Context(), ids["merge"])
	if err != nil {
		t.Fatalf("ReachableCommits() error = %v", err)
	}

	// root, main, three side branches, and the octopus merge; the first-parent
	// chain holds only three of them.
	if got != 6 {
		t.Errorf("ReachableCommits() = %d, want 6", got)
	}
}

func TestReachableCommitsRejectsUnvalidatedRevision(t *testing.T) {
	repository, _ := newBranchedTestRepository(t)
	repo := openTestRepository(t, repository)

	for _, revision := range []string{"HEAD", "--all", strings.Repeat("A", 40)} {
		if _, err := repo.ReachableCommits(t.Context(), revision); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("ReachableCommits(%q) error = %v, want ErrInvalidInput", revision, err)
		}
	}
}

func TestReachableCommitsRefusesAnUnopenedRepository(t *testing.T) {
	var repo Repository
	if _, err := repo.ReachableCommits(t.Context(), strings.Repeat("a", 40)); !errors.Is(err, errUninitializedRepository) {
		t.Errorf("ReachableCommits() error = %v, want errUninitializedRepository", err)
	}
}

func TestOpenRecordsTheGitVersion(t *testing.T) {
	repository, _ := newBranchedTestRepository(t)
	repo := openTestRepository(t, repository)

	output := runTestGit(t, "version")
	want := strings.Fields(output)[2]
	if got := repo.GitVersion(); got != want {
		t.Errorf("GitVersion() = %q, want %q from %q", got, want, output)
	}
}

func TestParseCount(t *testing.T) {
	tests := []struct {
		output string
		want   int
	}{
		{output: "1\n", want: 1},
		{output: "140375\n", want: 140375},
	}
	for _, tt := range tests {
		got, err := parseCount([]byte(tt.output))
		if err != nil || got != tt.want {
			t.Errorf("parseCount(%q) = %d, %v, want %d", tt.output, got, err, tt.want)
		}
	}

	for _, output := range []string{"", "\n", "0\n", "007\n", "12", "1 2\n", "-1\n", "+3\n", "3\n\n", "99999999999999999999999\n"} {
		if _, err := parseCount([]byte(output)); !errors.Is(err, ErrMalformedGitOutput) {
			t.Errorf("parseCount(%q) error = %v, want ErrMalformedGitOutput", output, err)
		}
	}
}

// TestReachableCommitsClassifiesAMissingSideParent removes a commit reachable
// only through a merge's secondary parent: the first-parent walk still
// succeeds, and the full count must report an incomplete repository rather
// than a generic Git failure.
func TestReachableCommitsClassifiesAMissingSideParent(t *testing.T) {
	repository, ids := newBranchedTestRepository(t)
	removeObject(t, repository, ids["side2"])
	repo := openTestRepository(t, repository)

	if _, err := repo.FirstParentEvents(t.Context(), ids["merge"]); err != nil {
		t.Fatalf("FirstParentEvents() error = %v, want the first-parent walk to succeed", err)
	}
	_, err := repo.ReachableCommits(t.Context(), ids["merge"])
	if !errors.Is(err, ErrIncompleteRepository) {
		t.Fatalf("ReachableCommits() error = %v, want ErrIncompleteRepository", err)
	}
}
