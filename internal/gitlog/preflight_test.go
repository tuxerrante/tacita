package gitlog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestOpenRejectsShallowRepository(t *testing.T) {
	source, _ := newTestRepository(t)
	runTestGit(t, "-C", source,
		"-c", "user.name=Tacita Test", "-c", "user.email=tacita@example.invalid",
		"commit", "--quiet", "--allow-empty", "-m", "second")

	shallow := filepath.Join(t.TempDir(), "shallow")
	runTestGit(t, "clone", "--quiet", "--depth", "1", "file://"+source, shallow)

	if _, err := Open(t.Context(), shallow); !errors.Is(err, ErrIncompleteRepository) {
		t.Fatalf("Open() error = %v, want ErrIncompleteRepository", err)
	}
}

func TestOpenRejectsPartialClone(t *testing.T) {
	repository, _ := newTestRepository(t)

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "promisor remote", key: "remote.origin.promisor", value: "true"},
		{name: "mixed case remote", key: "remote.Upstream.promisor", value: "true"},
		{name: "partial clone extension", key: "extensions.partialClone", value: "origin"},
		// Git registers a promisor remote for a filter on its own, without any
		// promisor key, so the filter has to be rejected on presence alone.
		{name: "partial clone filter", key: "remote.origin.partialCloneFilter", value: "blob:none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clone := filepath.Join(t.TempDir(), "clone")
			runTestGit(t, "clone", "--quiet", repository, clone)
			runTestGit(t, "-C", clone, "config", tt.key, tt.value)

			if _, err := Open(t.Context(), clone); !errors.Is(err, ErrIncompleteRepository) {
				t.Fatalf("Open() error = %v, want ErrIncompleteRepository", err)
			}
		})
	}
}

// TestOpenRejectsPartialCloneHiddenByConditionalInclude covers why the effective
// configuration is listed instead of the local scope: `config --list --local`
// does not expand a conditional include, so the local scope alone would report
// a partial clone as an ordinary repository.
func TestOpenRejectsPartialCloneHiddenByConditionalInclude(t *testing.T) {
	repository, _ := newTestRepository(t)

	included := filepath.Join(t.TempDir(), "included")
	if err := os.WriteFile(included, []byte("[remote \"origin\"]\n\tpromisor = true\n"), 0o600); err != nil {
		t.Fatalf("writing included configuration: %v", err)
	}
	runTestGit(t, "-C", repository, "config",
		"includeIf.gitdir:"+repository+"/.path", included)

	if _, err := Open(t.Context(), repository); !errors.Is(err, ErrIncompleteRepository) {
		t.Fatalf("Open() error = %v, want ErrIncompleteRepository", err)
	}
}

// TestOpenAcceptsFalsePromisor keeps the promisor match from rejecting a
// repository that only records the key as disabled.
func TestOpenAcceptsFalsePromisor(t *testing.T) {
	repository, want := newTestRepository(t)
	runTestGit(t, "-C", repository, "config", "remote.origin.promisor", "false")

	got, err := openTestRepository(t, repository).ResolveCommit(t.Context(), "HEAD")
	if err != nil {
		t.Fatalf("ResolveCommit() error = %v", err)
	}
	if got != want {
		t.Errorf("ResolveCommit() = %q, want %q", got, want)
	}
}

func TestOpenRejectsHistoryOverrideFiles(t *testing.T) {
	tests := []struct {
		name     string
		relative string
	}{
		{name: "grafts", relative: graftsFile},
		{name: "alternates", relative: alternatesFile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository, _ := newTestRepository(t)
			writeOverrideFile(t, repository, tt.relative, "aabbccdd\n")

			if _, err := Open(t.Context(), repository); !errors.Is(err, ErrIncompleteRepository) {
				t.Fatalf("Open() error = %v, want ErrIncompleteRepository", err)
			}
		})
	}
}

// TestOpenAcceptsEmptyHistoryOverrideFiles keeps an existing but empty file from
// rejecting a complete repository. Git creates neither file, but tooling does.
func TestOpenAcceptsEmptyHistoryOverrideFiles(t *testing.T) {
	repository, want := newTestRepository(t)
	writeOverrideFile(t, repository, graftsFile, "\n")
	writeOverrideFile(t, repository, alternatesFile, "")

	got, err := openTestRepository(t, repository).ResolveCommit(t.Context(), "HEAD")
	if err != nil {
		t.Fatalf("ResolveCommit() error = %v", err)
	}
	if got != want {
		t.Errorf("ResolveCommit() = %q, want %q", got, want)
	}
}

// TestOpenRejectsOversizedHistoryOverrideFiles covers a file that cannot be read
// within the bound. Reading only a prefix would accept leading whitespace longer
// than the limit while Git still reads the alternate that follows it.
func TestOpenRejectsOversizedHistoryOverrideFiles(t *testing.T) {
	repository, _ := newTestRepository(t)
	padding := strings.Repeat("\n", maxOverrideFile)
	writeOverrideFile(t, repository, alternatesFile, padding+"/elsewhere/objects\n")

	if _, err := Open(t.Context(), repository); !errors.Is(err, ErrIncompleteRepository) {
		t.Fatalf("Open() error = %v, want ErrIncompleteRepository", err)
	}
}

// TestOpenRejectsIrregularHistoryOverrideFiles covers the rule that an override
// path is classified before it is read. A FIFO would otherwise block the run and
// a symlink would redirect the read outside the repository.
func TestOpenRejectsIrregularHistoryOverrideFiles(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("aabbccdd\n"), 0o600); err != nil {
		t.Fatalf("writing outside file: %v", err)
	}

	tests := []struct {
		name   string
		create func(t *testing.T, path string)
	}{
		{
			name: "symlink",
			create: func(t *testing.T, path string) {
				if err := os.Symlink(outside, path); err != nil {
					t.Fatalf("creating symlink: %v", err)
				}
			},
		},
		{
			name: "fifo",
			create: func(t *testing.T, path string) {
				if err := syscall.Mkfifo(path, 0o600); err != nil {
					t.Fatalf("creating FIFO: %v", err)
				}
			},
		},
		{
			name: "directory",
			create: func(t *testing.T, path string) {
				if err := os.Mkdir(path, 0o750); err != nil {
					t.Fatalf("creating directory: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository, _ := newTestRepository(t)
			path := filepath.Join(repository, gitDirEntry, graftsFile)
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				t.Fatalf("creating override directory: %v", err)
			}
			tt.create(t, path)

			if _, err := Open(t.Context(), repository); !errors.Is(err, ErrIncompleteRepository) {
				t.Fatalf("Open() error = %v, want ErrIncompleteRepository", err)
			}
		})
	}
}

// TestOpenInspectsCommonDirectoryOfLinkedWorktree covers why the common
// directory is resolved instead of the Git directory: a linked worktree has a
// private Git directory, but Git reads grafts and alternates from the directory
// shared with the main worktree.
func TestOpenInspectsCommonDirectoryOfLinkedWorktree(t *testing.T) {
	repository, _ := newTestRepository(t)
	linked := filepath.Join(t.TempDir(), "linked")
	runTestGit(t, "-C", repository, "worktree", "add", "--quiet", "--detach", linked)

	if _, err := Open(t.Context(), linked); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	writeOverrideFile(t, repository, alternatesFile, "/elsewhere/objects\n")

	if _, err := Open(t.Context(), linked); !errors.Is(err, ErrIncompleteRepository) {
		t.Fatalf("Open() error = %v, want ErrIncompleteRepository", err)
	}
}

// TestOpenRejectsValuelessPromisorKey covers Git's implicit boolean: a key
// written without a value is true. Splitting configuration output on the value
// separator alone would drop the key and let a promisor remote through.
func TestOpenRejectsValuelessPromisorKey(t *testing.T) {
	repository, _ := newTestRepository(t)
	config := filepath.Join(repository, gitDirEntry, "config")
	existing, err := os.ReadFile(config)
	if err != nil {
		t.Fatalf("reading configuration: %v", err)
	}
	valueless := string(existing) + "[remote \"origin\"]\n\tpromisor\n"
	if err := os.WriteFile(config, []byte(valueless), 0o600); err != nil {
		t.Fatalf("writing configuration: %v", err)
	}

	if _, err := Open(t.Context(), repository); !errors.Is(err, ErrIncompleteRepository) {
		t.Fatalf("Open() error = %v, want ErrIncompleteRepository", err)
	}
}

func TestIsGitFalse(t *testing.T) {
	tests := []struct {
		name     string
		hasValue bool
		value    string
		want     bool
	}{
		{name: "valueless key is true", hasValue: false, want: false},
		{name: "empty value", hasValue: true, value: "", want: true},
		{name: "false", hasValue: true, value: "false", want: true},
		{name: "uppercase false", hasValue: true, value: "FALSE", want: true},
		{name: "off", hasValue: true, value: "off", want: true},
		{name: "zero", hasValue: true, value: "0", want: true},
		{name: "true", hasValue: true, value: "true", want: false},
		{name: "one", hasValue: true, value: "1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGitFalse(tt.hasValue, tt.value); got != tt.want {
				t.Errorf("isGitFalse(%v, %q) = %v, want %v", tt.hasValue, tt.value, got, tt.want)
			}
		})
	}
}

func writeOverrideFile(t *testing.T, repository string, relative string, content string) {
	t.Helper()

	path := filepath.Join(repository, gitDirEntry, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("creating override directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing override file: %v", err)
	}
}
