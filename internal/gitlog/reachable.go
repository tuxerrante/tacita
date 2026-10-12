package gitlog

import (
	"context"
	"fmt"
	"strconv"
)

// ReachableCommits counts every commit reachable from commit, merged branches
// included, for the report's reproducibility record. It reads commit objects
// only and contributes no event or path. A successful first-parent walk does
// not prove that commits reachable only through secondary parents exist, so a
// failed count is classified like a failed first-parent walk: incomplete only
// when every object a bounded diagnostic reports is confirmed missing.
//
// commit must be a complete lowercase object ID, normally one [ResolveCommit]
// returned: like the first-parent walk, the command carries no
// `--end-of-options`.
func (r *Repository) ReachableCommits(ctx context.Context, commit string) (int, error) {
	id, err := parseObjectID(commit)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	output, err := r.runScalar(ctx, "counting reachable commits", maxScalarOutput, "rev-list", "--count", id.String())
	if err != nil {
		return 0, r.classifyTraversalFailure(ctx, id, err, "reachable history")
	}

	return parseCount(output)
}

// parseCount accepts exactly one positive decimal count without leading zeros
// followed by one LF.
func parseCount(output []byte) (int, error) {
	text := string(output)
	if len(text) < 2 || text[len(text)-1] != '\n' {
		return 0, fmt.Errorf("%w: unexpected commit count framing", ErrMalformedGitOutput)
	}
	digits := text[:len(text)-1]
	for index := range len(digits) {
		if digits[index] < '0' || digits[index] > '9' {
			return 0, fmt.Errorf("%w: commit count is not decimal", ErrMalformedGitOutput)
		}
	}
	count, err := strconv.Atoi(digits)
	if err != nil || count < 1 || digits[0] == '0' {
		return 0, fmt.Errorf("%w: commit count %q", ErrMalformedGitOutput, digits)
	}

	return count, nil
}
