package gitlog

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
)

// Repository is a validated, run-scoped binding to one analyzed repository. It
// exists to make the required ordering unrepresentable rather than to cache
// work: every object access is a method, so no command can run before the
// environment is validated and the repository target is classified.
//
// A Repository is obtained only from [Open] and is valid for a single analysis
// run. It is not a reusable long-lived handle and holds no operating system
// resource, so there is nothing to close.
//
// Open validates containment, platform, Git version, object format, and the
// repository-level completeness mechanisms. It cannot prove that every required
// object is present, so a missing object is still classified during traversal.
type Repository struct {
	path      string
	target    []string
	preflight preflight
	ready     bool
}

// Open validates the execution environment, classifies path as a worktree or a
// bare repository, rejects the known incompleteness mechanisms, and returns a
// Repository bound to it. The caller owns the elapsed-time deadline.
//
// path is resolved to an absolute path first, before any subprocess runs, so a
// later working-directory change cannot redirect the run to a repository that
// was never validated.
func Open(ctx context.Context, path string) (*Repository, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: repository path is empty", ErrInvalidInput)
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving repository path: %w", err)
	}

	if err := validateEnvironment(ctx); err != nil {
		return nil, err
	}

	target, err := repositoryTarget(absolute)
	if err != nil {
		return nil, err
	}

	if err := validateObjectFormat(ctx, target); err != nil {
		return nil, err
	}

	result, err := inspectRepository(ctx, target)
	if err != nil {
		return nil, err
	}

	return &Repository{
		path:      absolute,
		target:    target,
		preflight: result,
		ready:     true,
	}, nil
}

// bind prepends the classified target and the deterministic configuration
// overrides to args.
//
// It is the single choke point every Git invocation passes through, and it
// fails closed on a repository that [Open] did not produce. That guard is not
// redundant with the type's documentation: an unvalidated Repository carries no
// target, and Git without an explicit target discovers a repository from the
// process working directory, which is exactly the containment failure the
// classified target removes.
func (r *Repository) bind(args ...string) ([]string, error) {
	if r == nil || !r.ready {
		return nil, errUninitializedRepository
	}
	return gitArgs(r.target, args...), nil
}

// runScalar runs a Git command whose output has a fixed, small grammar and is
// safe to buffer whole.
//
// It is deliberately not a general runner. History commands must be streamed,
// so they build their own bounded pipeline instead of calling this.
func (r *Repository) runScalar(
	ctx context.Context,
	operation string,
	limit int,
	args ...string,
) ([]byte, error) {
	bound, err := r.bind(args...)
	if err != nil {
		return nil, err
	}
	return runGit(ctx, operation, limit, bound...)
}

// runTargeted runs a bounded Git command against an already classified target.
// It exists for the checks that run while the Repository is still being
// constructed and therefore cannot use a method on it.
func runTargeted(
	ctx context.Context,
	target []string,
	operation string,
	limit int,
	args ...string,
) ([]byte, error) {
	return runGit(ctx, operation, limit, gitArgs(target, args...)...)
}

// validateEnvironment checks the process-wide properties that no repository can
// change. It runs before classification so an unsupported host fails before any
// repository-controlled path is inspected.
func validateEnvironment(ctx context.Context) error {
	if err := validatePlatform(runtime.GOOS, runtime.GOARCH); err != nil {
		return err
	}
	return validateGitVersion(ctx)
}
