package gitlog

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const (
	supportedGOOS         = "linux"
	supportedGOARCH       = "amd64"
	minimumGitMajor       = 2
	minimumGitMinor       = 43
	supportedObjectFormat = "sha1"
	sha256ObjectFormat    = "sha256"
	sha1HexLength         = 40
)

// ResolveCommit resolves revision to a complete lowercase SHA-1 commit object
// ID. The caller owns the elapsed-time deadline.
//
// Open already rejected the repository-level mechanisms that make local history
// incomplete. Success still does not prove that every object traversal needs is
// present, which is classified while traversing.
func (r *Repository) ResolveCommit(ctx context.Context, revision string) (string, error) {
	if revision == "" {
		return "", fmt.Errorf("%w: revision is empty", ErrInvalidInput)
	}

	output, err := r.runScalar(
		ctx,
		"resolving commit",
		maxScalarOutput,
		"rev-parse",
		"--verify",
		"--end-of-options",
		revision+"^{commit}",
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

	return string(output[:sha1HexLength]), nil
}

func validatePlatform(goos string, goarch string) error {
	if goos != supportedGOOS {
		return fmt.Errorf("%w: got %q, require %q", ErrUnsupportedOS, goos, supportedGOOS)
	}
	if goarch != supportedGOARCH {
		return fmt.Errorf(
			"%w: got %q, require %q",
			ErrUnsupportedArchitecture,
			goarch,
			supportedGOARCH,
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
			"%w: got %d.%d, require %d.%d or newer",
			ErrUnsupportedGitVersion,
			major,
			minor,
			minimumGitMajor,
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
	return major > minimumGitMajor ||
		(major == minimumGitMajor && minor >= minimumGitMinor)
}

// validateObjectFormat runs against the already classified target because the
// Repository is not constructed until every check has passed.
func validateObjectFormat(ctx context.Context, target []string) error {
	output, err := runTargeted(
		ctx,
		target,
		"checking Git object format",
		maxScalarOutput,
		"rev-parse", "--show-object-format",
	)
	if err != nil {
		return err
	}

	switch string(output) {
	case supportedObjectFormat + "\n":
		return nil
	case sha256ObjectFormat + "\n":
		return fmt.Errorf(
			"%w: got %q, require %q",
			ErrUnsupportedObjectFormat,
			sha256ObjectFormat,
			supportedObjectFormat,
		)
	default:
		return fmt.Errorf("%w: unexpected object format", ErrMalformedGitOutput)
	}
}

func isFullSHA1(output []byte) bool {
	return len(output) == sha1HexLength+1 &&
		output[sha1HexLength] == '\n' &&
		isObjectID(output[:sha1HexLength])
}

// isObjectID reports whether value is exactly one complete lowercase
// hexadecimal SHA-1 object ID, with nothing around it.
func isObjectID(value []byte) bool {
	if len(value) != sha1HexLength {
		return false
	}
	for _, b := range value {
		if (b < '0' || b > '9') && (b < 'a' || b > 'f') {
			return false
		}
	}
	return true
}
