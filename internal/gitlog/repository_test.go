package gitlog

import (
	"errors"
	"path/filepath"
	"testing"
)

// TestUnvalidatedRepositoryRunsNoGit covers the reason bind guards its
// receiver. A Repository that Open did not produce carries no target, and Git
// without an explicit target discovers a repository from the process working
// directory. The working directory here is a real repository, so a missing
// guard would return a commit instead of an error.
func TestUnvalidatedRepositoryRunsNoGit(t *testing.T) {
	repository, head := newTestRepository(t)
	t.Chdir(repository)

	tests := []struct {
		name       string
		repository *Repository
	}{
		{name: "zero value", repository: &Repository{}},
		{name: "nil receiver", repository: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.repository.ResolveCommit(t.Context(), "HEAD")
			if !errors.Is(err, errUninitializedRepository) {
				t.Fatalf("ResolveCommit() error = %v, want errUninitializedRepository", err)
			}
			if got == head {
				t.Fatal("ResolveCommit() resolved the working directory's repository")
			}
		})
	}
}

// TestOpenBindsPathBeforeWorkingDirectoryChanges covers the reason Open stores
// an absolute path. A relative path names a different repository once the
// process changes directory, so a run could otherwise continue against a
// repository that never passed classification or validation.
func TestOpenBindsPathBeforeWorkingDirectoryChanges(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	want := newTestRepositoryAt(t, filepath.Join(first, "repository"))
	decoy := newTestRepositoryAt(t, filepath.Join(second, "repository"))
	if want == decoy {
		t.Fatal("test repositories share a commit ID")
	}

	t.Chdir(first)
	repository := openTestRepository(t, "repository")
	t.Chdir(second)

	got, err := repository.ResolveCommit(t.Context(), "HEAD")
	if err != nil {
		t.Fatalf("ResolveCommit() error = %v", err)
	}
	if got != want {
		t.Errorf("ResolveCommit() = %q, want the repository Open validated (%q)", got, want)
	}
}
