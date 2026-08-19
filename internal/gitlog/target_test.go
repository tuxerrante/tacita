package gitlog

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestOpenRejectsAncestorRepository is the regression test for Git's discovery
// walk: without explicit targeting, a directory that is not a repository
// resolves its ancestor's commit instead of failing.
func TestOpenRejectsAncestorRepository(t *testing.T) {
	repository, _ := newTestRepository(t)
	nested := filepath.Join(repository, "sub", "deep")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("creating nested directory: %v", err)
	}

	if _, err := Open(t.Context(), nested); !errors.Is(err, ErrNotARepository) {
		t.Fatalf("Open() error = %v, want ErrNotARepository", err)
	}
}

func TestOpenAcceptsRepositoryShapes(t *testing.T) {
	repository, want := newTestRepository(t)

	linked := filepath.Join(t.TempDir(), "linked worktree")
	runTestGit(t, "-C", repository, "worktree", "add", "--quiet", "--detach", linked)

	symlinked := filepath.Join(t.TempDir(), "symlinked")
	if err := os.Symlink(repository, symlinked); err != nil {
		t.Fatalf("creating repository symlink: %v", err)
	}

	tests := []struct {
		name       string
		repository string
	}{
		{name: "worktree", repository: repository},
		{name: "linked worktree", repository: linked},
		{name: "symlinked path", repository: symlinked},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := openTestRepository(t, tt.repository).ResolveCommit(t.Context(), "HEAD")
			if err != nil {
				t.Fatalf("ResolveCommit() error = %v", err)
			}
			if got != want {
				t.Errorf("ResolveCommit() = %q, want %q", got, want)
			}
		})
	}
}

func TestOpenAcceptsSubmoduleWorktree(t *testing.T) {
	child, want := newTestRepository(t)
	parent, _ := newTestRepository(t)

	runTestGit(
		t,
		"-C", parent,
		"-c", "protocol.file.allow=always",
		"-c", "user.name=Tacita Test",
		"-c", "user.email=tacita@example.invalid",
		"submodule", "add", "--quiet", "--", child, "child",
	)

	submodule := openTestRepository(t, filepath.Join(parent, "child"))
	got, err := submodule.ResolveCommit(t.Context(), "HEAD")
	if err != nil {
		t.Fatalf("ResolveCommit() error = %v", err)
	}
	if got != want {
		t.Errorf("ResolveCommit() = %q, want %q", got, want)
	}
}

func TestRepositoryTargetReportsUnreadablePath(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	unreadable := filepath.Join(t.TempDir(), "unreadable")
	if err := os.Mkdir(unreadable, 0o000); err != nil {
		t.Fatalf("creating unreadable directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o700) })

	_, err := repositoryTarget(filepath.Join(unreadable, "repository"))
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("repositoryTarget() error = %v, want fs.ErrPermission", err)
	}
	if errors.Is(err, ErrNotARepository) {
		t.Fatal("repositoryTarget() reported an unreadable path as not a repository")
	}
}

func TestRepositoryTarget(t *testing.T) {
	root := t.TempDir()

	worktree := filepath.Join(root, "worktree")
	if err := os.MkdirAll(filepath.Join(worktree, gitDirEntry), 0o750); err != nil {
		t.Fatalf("creating worktree: %v", err)
	}

	gitFileWorktree := filepath.Join(root, "gitfile")
	if err := os.MkdirAll(gitFileWorktree, 0o750); err != nil {
		t.Fatalf("creating gitfile worktree: %v", err)
	}
	gitFile := filepath.Join(gitFileWorktree, gitDirEntry)
	if err := os.WriteFile(gitFile, []byte("gitdir: elsewhere\n"), 0o600); err != nil {
		t.Fatalf("creating gitfile: %v", err)
	}

	bare := filepath.Join(root, "bare.git")
	if err := os.MkdirAll(bare, 0o750); err != nil {
		t.Fatalf("creating bare repository: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bare, headEntry), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatalf("creating bare HEAD: %v", err)
	}

	plain := filepath.Join(root, "plain")
	if err := os.MkdirAll(plain, 0o750); err != nil {
		t.Fatalf("creating plain directory: %v", err)
	}

	tests := []struct {
		name       string
		repository string
		want       []string
		wantErr    error
	}{
		{
			name:       "worktree",
			repository: worktree,
			want: []string{
				"--git-dir=" + filepath.Join(worktree, gitDirEntry),
				"--work-tree=" + worktree,
			},
		},
		{
			name:       "gitfile worktree",
			repository: gitFileWorktree,
			want: []string{
				"--git-dir=" + gitFile,
				"--work-tree=" + gitFileWorktree,
			},
		},
		{name: "bare repository", repository: bare, want: []string{"--git-dir=" + bare}},
		{name: "plain directory", repository: plain, wantErr: ErrNotARepository},
		{
			name:       "missing path",
			repository: filepath.Join(root, "absent"),
			wantErr:    ErrNotARepository,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repositoryTarget(tt.repository)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("repositoryTarget() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("repositoryTarget() error = %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("repositoryTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGitArgsTargetsBeforeSubcommand(t *testing.T) {
	repository, _ := newTestRepository(t)

	got, err := openTestRepository(t, repository).bind("rev-parse", "--show-object-format")
	if err != nil {
		t.Fatalf("bind() error = %v", err)
	}
	if got[0] != "--git-dir="+filepath.Join(repository, gitDirEntry) {
		t.Errorf("bind() first argument = %q, want the Git directory", got[0])
	}
	if got[len(got)-2] != "rev-parse" {
		t.Errorf("bind() = %q, want the subcommand last", got)
	}
	for _, argument := range got {
		if argument == "-C" {
			t.Fatal("bind() still relies on Git repository discovery")
		}
	}
}
