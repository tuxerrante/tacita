package gitlog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"
)

const (
	// gitDirEntry names the worktree entry pointing at the repository. It is a
	// directory in an ordinary worktree and a file in a linked worktree or a
	// submodule.
	gitDirEntry = ".git"
	// headEntry names the file present in every Git directory, which
	// distinguishes a bare repository from an ordinary directory.
	headEntry = "HEAD"
)

// repositoryTarget returns the global Git arguments binding a command to
// repository.
//
// Passing -C would not bind it: Git walks upward until it finds a repository,
// so a path that is not a repository resolves against an enclosing one and
// silently analyzes a target the operator never named. Naming the Git
// directory explicitly removes that discovery walk instead of detecting it
// afterwards.
func repositoryTarget(repository string) ([]string, error) {
	gitDir := filepath.Join(repository, gitDirEntry)
	worktree, err := pathExists(gitDir)
	if err != nil {
		return nil, err
	}
	if worktree {
		return []string{"--git-dir=" + gitDir, "--work-tree=" + repository}, nil
	}

	bare, err := pathExists(filepath.Join(repository, headEntry))
	if err != nil {
		return nil, err
	}
	if bare {
		return []string{"--git-dir=" + repository}, nil
	}

	return nil, fmt.Errorf(
		"%w: %q is neither a worktree nor a bare repository",
		ErrNotARepository,
		repository,
	)
}

// pathExists reports whether path exists. A missing entry, or a parent that is
// not a directory, means absent. Any other failure is returned, so an
// unreadable directory is not silently reported as "not a repository".
func pathExists(path string) (bool, error) {
	switch _, err := os.Stat(path); {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
		return false, nil
	default:
		return false, fmt.Errorf("inspecting repository path: %w", err)
	}
}

func repositoryGitArgs(repository string, args ...string) ([]string, error) {
	target, err := repositoryTarget(repository)
	if err != nil {
		return nil, err
	}

	overrides := []string{
		"-c", "core.abbrev=40",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=/dev/null",
		"-c", "diff.external=",
	}

	return slices.Concat(target, overrides, args), nil
}
