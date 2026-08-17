package gitlog

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

const minimumGitMinor = 43

// ResolveCommit validates the platform, Git version, and object format, then
// resolves revision to a complete lowercase SHA-1 commit object ID. The caller
// owns the elapsed-time deadline. Success does not validate repository
// completeness; history preflight must run before traversal.
func ResolveCommit(
	ctx context.Context,
	repository string,
	revision string,
) (string, error) {
	if repository == "" {
		return "", fmt.Errorf("%w: repository path is empty", ErrInvalidInput)
	}
	if revision == "" {
		return "", fmt.Errorf("%w: revision is empty", ErrInvalidInput)
	}

	if err := validatePlatform(runtime.GOOS, runtime.GOARCH); err != nil {
		return "", err
	}
	if err := validateGitVersion(ctx); err != nil {
		return "", err
	}
	if err := validateObjectFormat(ctx, repository); err != nil {
		return "", err
	}

	output, err := runGit(
		ctx,
		"resolving commit",
		maxScalarOutput,
		repositoryGitArgs(
			repository,
			"rev-parse",
			"--verify",
			"--end-of-options",
			revision+"^{commit}",
		)...,
	)
	if err != nil {
		return "", err
	}
	if !isFullSHA1(output) {
		return "", fmt.Errorf(
			"%w: resolving commit returned %d bytes",
			ErrMalformedGitOutput,
			len(output),
		)
	}

	return string(output[:40]), nil
}

func validatePlatform(goos string, goarch string) error {
	if goos != "linux" {
		return fmt.Errorf("%w: got %q, require %q", ErrUnsupportedOS, goos, "linux")
	}
	if goarch != "amd64" {
		return fmt.Errorf(
			"%w: got %q, require %q",
			ErrUnsupportedArchitecture,
			goarch,
			"amd64",
		)
	}
	return nil
}

func validateGitVersion(ctx context.Context) error {
	output, err := runGit(ctx, "checking Git version", maxScalarOutput, "version")
	if err != nil {
		return err
	}

	major, minor, err := parseGitVersion(output)
	if err != nil {
		return err
	}
	if !supportedGitVersion(major, minor) {
		return fmt.Errorf(
			"%w: got %d.%d, require 2.%d or newer",
			ErrUnsupportedGitVersion,
			major,
			minor,
			minimumGitMinor,
		)
	}
	return nil
}

func parseGitVersion(output []byte) (int, int, error) {
	if len(output) == 0 || output[len(output)-1] != '\n' ||
		strings.Count(string(output), "\n") != 1 {
		return 0, 0, fmt.Errorf("%w: unexpected Git version framing", ErrMalformedGitOutput)
	}

	line := strings.TrimSuffix(string(output), "\n")
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[0] != "git" || fields[1] != "version" {
		return 0, 0, fmt.Errorf("%w: unexpected Git version", ErrMalformedGitOutput)
	}

	parts := strings.Split(fields[2], ".")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("%w: unexpected Git version", ErrMalformedGitOutput)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("%w: unexpected Git major version", ErrMalformedGitOutput)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("%w: unexpected Git minor version", ErrMalformedGitOutput)
	}

	return major, minor, nil
}

func supportedGitVersion(major int, minor int) bool {
	return major > 2 || (major == 2 && minor >= minimumGitMinor)
}

func validateObjectFormat(ctx context.Context, repository string) error {
	output, err := runGit(
		ctx,
		"checking Git object format",
		maxScalarOutput,
		repositoryGitArgs(repository, "rev-parse", "--show-object-format")...,
	)
	if err != nil {
		return err
	}

	switch string(output) {
	case "sha1\n":
		return nil
	case "sha256\n":
		return fmt.Errorf("%w: got %q, require %q", ErrUnsupportedObjectFormat, "sha256", "sha1")
	default:
		return fmt.Errorf("%w: unexpected object format", ErrMalformedGitOutput)
	}
}

func isFullSHA1(output []byte) bool {
	if len(output) != 41 || output[40] != '\n' {
		return false
	}
	for _, b := range output[:40] {
		if (b < '0' || b > '9') && (b < 'a' || b > 'f') {
			return false
		}
	}
	return true
}
