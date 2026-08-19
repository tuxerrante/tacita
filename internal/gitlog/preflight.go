package gitlog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	// maxConfigOutput bounds the effective repository configuration. It is far
	// above any plausible legitimate configuration and still refuses to buffer
	// an unbounded repository-controlled stream.
	maxConfigOutput = 1 << 20
	// maxOverrideFile bounds a graft or alternates file. Both are rejected when
	// non-empty, so the limit only has to be large enough to distinguish empty
	// from non-empty. A file that exceeds it is rejected rather than truncated,
	// because a truncated prefix cannot prove the rest of the file is empty.
	maxOverrideFile = 64 << 10

	// asciiWhitespace is what Git skips when it reads a graft or alternates
	// file. Go's Unicode notion of space is wider, and every byte outside this
	// set is a path byte Git will use, so trimming by it would read a file
	// declaring a no-break space as if it declared nothing.
	asciiWhitespace = " \t\r\n\v\f"

	graftsFile     = "info/grafts"
	alternatesFile = "objects/info/alternates"

	partialCloneExtension = "extensions.partialclone"
	remoteKeyPrefix       = "remote."
	promisorKeySuffix     = ".promisor"
	filterKeySuffix       = ".partialclonefilter"
)

// preflight records what the completeness checks established about a
// repository. It is only ever produced by inspectRepository, so holding one
// means every rejection below already ran.
type preflight struct {
	// commonDir is the absolute Git common directory. A linked worktree shares
	// it with the main worktree, which is where grafts and alternates live.
	commonDir string
}

// inspectRepository rejects the repository-level mechanisms that make local
// history incomplete or make traversal reach the network.
//
// It cannot prove that every required object is present: the frozen data flow
// omits `--root` and never traverses secondary parents, so a missing object is
// classified during traversal instead. What it does guarantee is that the known
// incompleteness mechanisms are gone before any history command runs.
//
// It takes the classified target rather than a Repository because it runs while
// the Repository is still being constructed; a value that could reach Git
// before this returned would defeat the ordering the type exists to enforce.
func inspectRepository(ctx context.Context, target []string) (preflight, error) {
	if err := rejectShallow(ctx, target); err != nil {
		return preflight{}, err
	}

	commonDir, err := commonDirectory(ctx, target)
	if err != nil {
		return preflight{}, err
	}

	for _, override := range []struct {
		mechanism string
		path      string
	}{
		{mechanism: "graft", path: filepath.Join(commonDir, graftsFile)},
		{mechanism: "object alternates", path: filepath.Join(commonDir, alternatesFile)},
	} {
		if err := rejectHistoryOverride(override.mechanism, override.path); err != nil {
			return preflight{}, err
		}
	}

	if err := rejectPartialClone(ctx, target); err != nil {
		return preflight{}, err
	}

	return preflight{commonDir: commonDir}, nil
}

func rejectShallow(ctx context.Context, target []string) error {
	output, err := runTargeted(
		ctx,
		target,
		"checking shallow repository",
		maxScalarOutput,
		"rev-parse", "--is-shallow-repository",
	)
	if err != nil {
		return err
	}

	switch string(output) {
	case "false\n":
		return nil
	case "true\n":
		return fmt.Errorf("%w: repository is shallow", ErrIncompleteRepository)
	default:
		return fmt.Errorf("%w: unexpected shallow report", ErrMalformedGitOutput)
	}
}

// commonDirectory returns the directory shared by every worktree. Grafts and
// alternates must be inspected there: a linked worktree's private Git directory
// does not hold them, and Git does not read them from it either.
func commonDirectory(ctx context.Context, target []string) (string, error) {
	output, err := runTargeted(
		ctx,
		target,
		"locating Git common directory",
		maxScalarOutput,
		"rev-parse", "--path-format=absolute", "--git-common-dir",
	)
	if err != nil {
		return "", err
	}

	line, found := strings.CutSuffix(string(output), "\n")
	if !found || strings.ContainsAny(line, "\n\x00") || !filepath.IsAbs(line) {
		return "", fmt.Errorf("%w: unexpected Git common directory", ErrMalformedGitOutput)
	}

	return line, nil
}

// rejectHistoryOverride rejects a non-empty graft or alternates file.
//
// The path is inspected before it is read, so a symlink, FIFO, or device cannot
// redirect the run or block it. The file is then opened without following a
// final symlink and without blocking, and re-checked through the descriptor, so
// a path swapped between the two steps is still rejected rather than read.
func rejectHistoryOverride(mechanism string, path string) error {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
		return nil
	case err != nil:
		return fmt.Errorf("inspecting %s file: %w", mechanism, err)
	}
	if !info.Mode().IsRegular() {
		return notRegularFile(mechanism, path)
	}

	// The path is built from the repository's own common directory, and the
	// open refuses a final symlink, so the variable path is intended here.
	file, err := os.OpenFile( //nolint:gosec // repository-derived path, opened without following symlinks
		path,
		os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK,
		0,
	)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return notRegularFile(mechanism, path)
		}
		return fmt.Errorf("reading %s file: %w", mechanism, err)
	}
	defer func() { _ = file.Close() }()

	opened, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspecting %s file: %w", mechanism, err)
	}
	if !opened.Mode().IsRegular() {
		return notRegularFile(mechanism, path)
	}

	return rejectOverrideContent(mechanism, path, file, maxOverrideFile)
}

// rejectOverrideContent decides whether an opened override file declares
// anything. It is separated from opening the file because the decision is
// algorithmic rather than filesystem specific, which makes it reachable from a
// fuzz target without a filesystem.
func rejectOverrideContent(mechanism string, path string, source io.Reader, limit int) error {
	// One byte past the limit distinguishes a fully read file from a truncated
	// one. Without it a file of leading whitespace longer than the limit would
	// trim to empty and be accepted while Git still reads the entry that
	// follows.
	content, err := io.ReadAll(io.LimitReader(source, int64(limit)+1))
	if err != nil {
		return fmt.Errorf("reading %s file: %w", mechanism, err)
	}
	if len(content) > limit {
		return fmt.Errorf(
			"%w: %s file %q exceeds %d bytes",
			ErrIncompleteRepository,
			mechanism,
			path,
			limit,
		)
	}
	if len(bytes.Trim(content, asciiWhitespace)) > 0 {
		return fmt.Errorf(
			"%w: repository declares %s in %q",
			ErrIncompleteRepository,
			mechanism,
			path,
		)
	}

	return nil
}

func notRegularFile(mechanism string, path string) error {
	return fmt.Errorf(
		"%w: %s path %q is not a regular file",
		ErrIncompleteRepository,
		mechanism,
		path,
	)
}

// rejectPartialClone refuses a repository whose objects are fetched on demand.
// This is an isolation requirement as much as a completeness one: traversal over
// a partial clone reaches the network and writes new packs, breaking both the
// offline and the read-only guarantee.
//
// A remote is a promisor when it declares `promisor` as a true boolean or when
// it declares a `partialCloneFilter`, which Git registers regardless of the
// filter's value, and the repository is also a partial clone when it declares
// the `partialClone` extension.
//
// The effective configuration is listed rather than the local scope alone.
// `--local` does not expand a conditional include, and `--worktree` fails
// outright once a linked worktree exists without the worktree-config extension,
// so either would let a repository hide the very keys being looked for. System
// and global configuration are already disabled through the child environment,
// which leaves the plain listing equal to the effective repository scopes.
func rejectPartialClone(ctx context.Context, target []string) error {
	output, err := runTargeted(
		ctx,
		target,
		"reading repository configuration",
		maxConfigOutput,
		"config", "--list", "-z",
	)
	if err != nil {
		return err
	}

	return rejectPartialCloneConfig(output)
}

// rejectPartialCloneConfig decides whether a repository's own configuration
// registers a promisor remote. It is separated from running Git because the
// framing is repository controlled and worth fuzzing on its own.
//
// `config --list -z` frames every entry as `key\nvalue\0`. A key listed with no
// newline has no value, which Git reads as boolean true.
func rejectPartialCloneConfig(output []byte) error {
	for entry := range bytes.SplitSeq(output, []byte{0}) {
		if len(entry) == 0 {
			continue
		}

		rawKey, value, hasValue := bytes.Cut(entry, []byte{'\n'})
		// Git lowercases the section and the key but preserves a subsection, so
		// a remote name keeps its case and only the surrounding key can be
		// compared literally.
		key := strings.ToLower(string(rawKey))

		switch {
		case key == partialCloneExtension:
			return fmt.Errorf(
				"%w: repository is a partial clone (%s)",
				ErrIncompleteRepository,
				partialCloneExtension,
			)
		case strings.HasPrefix(key, remoteKeyPrefix) &&
			strings.HasSuffix(key, promisorKeySuffix) &&
			!isGitFalse(hasValue, string(value)):
			return fmt.Errorf(
				"%w: repository has a promisor remote (%s)",
				ErrIncompleteRepository,
				key,
			)
		// Git registers a promisor remote for a partial-clone filter whatever
		// the filter says, so the key is rejected on presence alone.
		case strings.HasPrefix(key, remoteKeyPrefix) &&
			strings.HasSuffix(key, filterKeySuffix):
			return fmt.Errorf(
				"%w: repository has a promisor remote (%s)",
				ErrIncompleteRepository,
				key,
			)
		}
	}

	return nil
}

// isGitFalse reports whether a configuration value is boolean false. A key
// listed without a value is true, which is how `[remote "x"] promisor` appears.
func isGitFalse(hasValue bool, value string) bool {
	if !hasValue {
		return false
	}
	switch strings.ToLower(value) {
	case "", "false", "no", "off", "0":
		return true
	default:
		return false
	}
}
