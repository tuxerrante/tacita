package gitlog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestResolveCommit(t *testing.T) {
	repository, want := newTestRepository(t)
	runTestGit(t, "-C", repository, "update-ref", "refs/heads/--help", "HEAD")

	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "hostile-git-dir"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "hostile-config"))

	tests := []struct {
		name     string
		revision string
		wantErr  error
	}{
		{name: "symbolic revision", revision: "HEAD"},
		{name: "full object ID", revision: want},
		{name: "option-like revision", revision: "--help"},
		{name: "missing revision", revision: "does-not-exist", wantErr: ErrGitFailure},
		{name: "non-commit object", revision: "HEAD^{tree}", wantErr: ErrGitFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveCommit(t.Context(), repository, tt.revision)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ResolveCommit() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveCommit() error = %v", err)
			}
			if got != want {
				t.Errorf("ResolveCommit() = %q, want %q", got, want)
			}
		})
	}
}

func TestResolveCommitBareRepository(t *testing.T) {
	repository, want := newTestRepository(t)
	bare := filepath.Join(t.TempDir(), "bare repository.git")
	runTestGit(t, "clone", "--quiet", "--bare", repository, bare)

	got, err := ResolveCommit(t.Context(), bare, "HEAD")
	if err != nil {
		t.Fatalf("ResolveCommit() error = %v", err)
	}
	if got != want {
		t.Errorf("ResolveCommit() = %q, want %q", got, want)
	}
}

func TestResolveCommitRejectsSHA256(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "sha256")
	runTestGit(t, "init", "--quiet", "--object-format=sha256", repository)

	_, err := ResolveCommit(t.Context(), repository, "HEAD")
	if !errors.Is(err, ErrUnsupportedObjectFormat) {
		t.Fatalf("ResolveCommit() error = %v, want ErrUnsupportedObjectFormat", err)
	}
}

func TestResolveCommitRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name       string
		repository string
		revision   string
	}{
		{name: "empty repository", revision: "HEAD"},
		{name: "empty revision", repository: "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveCommit(t.Context(), tt.repository, tt.revision)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("ResolveCommit() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestResolveCommitCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := ResolveCommit(ctx, ".", "HEAD")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveCommit() error = %v, want context.Canceled", err)
	}
}

func TestResolveCommitCancellationWaitsForGit(t *testing.T) {
	repository, _ := newTestRepository(t)
	head := filepath.Join(repository, ".git", "HEAD")
	if err := os.Remove(head); err != nil {
		t.Fatalf("removing HEAD: %v", err)
	}
	if err := syscall.Mkfifo(head, 0o600); err != nil {
		t.Fatalf("creating HEAD FIFO: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := ResolveCommit(ctx, repository, "HEAD")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ResolveCommit() error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("ResolveCommit() returned after %s, want <= 5s", elapsed)
	}
}

func TestGitErrorStderrIsBoundedAndCopied(t *testing.T) {
	repository, _ := newTestRepository(t)

	_, err := ResolveCommit(t.Context(), repository, "missing")
	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("ResolveCommit() error = %v, want *GitError", err)
	}

	diagnostic := gitErr.Stderr()
	if len(diagnostic) == 0 {
		t.Fatal("GitError.Stderr() is empty")
	}
	if len(diagnostic) > maxGitStderr {
		t.Fatalf("GitError.Stderr() length = %d, want <= %d", len(diagnostic), maxGitStderr)
	}

	diagnostic[0] ^= 0xff
	if string(diagnostic) == string(gitErr.Stderr()) {
		t.Fatal("GitError.Stderr() returned shared storage")
	}
}

func TestValidatePlatform(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		goarch  string
		wantErr error
	}{
		{name: "supported", goos: "linux", goarch: "amd64"},
		{name: "operating system", goos: "darwin", goarch: "amd64", wantErr: ErrUnsupportedOS},
		{name: "architecture", goos: "linux", goarch: "arm64", wantErr: ErrUnsupportedArchitecture},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePlatform(tt.goos, tt.goarch)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("validatePlatform() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseGitVersion(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantMajor int
		wantMinor int
		wantErr   bool
	}{
		{name: "release", output: "git version 2.55.0\n", wantMajor: 2, wantMinor: 55},
		{name: "suffix", output: "git version 2.43.1.fc44\n", wantMajor: 2, wantMinor: 43},
		{name: "future major", output: "git version 3.0.0\n", wantMajor: 3},
		{name: "missing minor", output: "git version 2\n", wantErr: true},
		{name: "unexpected prefix", output: "version 2.55.0\n", wantErr: true},
		{name: "missing newline", output: "git version 2.55.0", wantErr: true},
		{name: "multiple lines", output: "git version 2.55.0\nextra\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor, err := parseGitVersion([]byte(tt.output))
			if tt.wantErr {
				if !errors.Is(err, ErrMalformedGitOutput) {
					t.Fatalf("parseGitVersion() error = %v, want ErrMalformedGitOutput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGitVersion() error = %v", err)
			}
			if major != tt.wantMajor || minor != tt.wantMinor {
				t.Errorf(
					"parseGitVersion() = %d.%d, want %d.%d",
					major,
					minor,
					tt.wantMajor,
					tt.wantMinor,
				)
			}
		})
	}
}

func TestSupportedGitVersion(t *testing.T) {
	tests := []struct {
		major int
		minor int
		want  bool
	}{
		{major: 2, minor: 42},
		{major: 2, minor: 43, want: true},
		{major: 2, minor: 55, want: true},
		{major: 3, minor: 0, want: true},
		{major: 1, minor: 99},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d.%d", tt.major, tt.minor), func(t *testing.T) {
			if got := supportedGitVersion(tt.major, tt.minor); got != tt.want {
				t.Errorf(
					"supportedGitVersion(%d, %d) = %t, want %t",
					tt.major,
					tt.minor,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestLimitedBuffer(t *testing.T) {
	buffer := newLimitedBuffer(4)

	for _, input := range []string{"abc", "def"} {
		n, err := buffer.Write([]byte(input))
		if err != nil {
			t.Fatalf("Write(%q) error = %v", input, err)
		}
		if n != len(input) {
			t.Fatalf("Write(%q) = %d, want %d", input, n, len(input))
		}
	}

	if got := string(buffer.bytes()); got != "abcd" {
		t.Errorf("buffer = %q, want %q", got, "abcd")
	}
	if !buffer.exceeded {
		t.Fatal("buffer.exceeded = false, want true")
	}
}

func newTestRepository(t *testing.T) (string, string) {
	t.Helper()

	repository := filepath.Join(t.TempDir(), "-repo with space")
	runTestGit(t, "init", "--quiet", "--object-format=sha1", repository)

	if err := os.WriteFile(filepath.Join(repository, "tracked.txt"), []byte("content\n"), 0o600); err != nil {
		t.Fatalf("writing tracked file: %v", err)
	}
	runTestGit(t, "-C", repository, "add", "--", "tracked.txt")
	runTestGit(
		t,
		"-C", repository,
		"-c", "user.name=Tacita Test",
		"-c", "user.email=tacita@example.invalid",
		"commit", "--quiet", "-m", "initial",
	)

	return repository, runTestGit(t, "-C", repository, "rev-parse", "HEAD")
}

func runTestGit(t *testing.T, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Env = gitEnvironment()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
