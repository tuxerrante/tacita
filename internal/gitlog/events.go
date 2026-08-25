package gitlog

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
)

const (
	// maxIntegrationEvents is the frozen cap on first-parent integration events
	// scanned in one run.
	maxIntegrationEvents = 200_000

	missingObjectPrefix = '?'
	fieldSeparator      = ' '
	recordSeparator     = '\n'
)

// ObjectID is a complete lowercase hexadecimal SHA-1 object ID.
//
// It is an array rather than a string so that the retained chain stays close to
// its budget: at the frozen event cap, an Event costs 41 bytes inline instead of
// a 16-byte string header plus 40 separately allocated bytes. It is also the
// form `diff-tree` boundaries are compared against, byte for byte.
type ObjectID [sha1HexLength]byte

func (o ObjectID) String() string {
	return string(o[:])
}

// EventKind records what produced an integration event, which decides how its
// evidence is later attributed.
type EventKind uint8

const (
	// RootEvent is the first-parent chain's parentless commit. Its diff against
	// the empty tree is recorded but never mined.
	RootEvent EventKind = iota
	// SingleParentEvent is an ordinary commit on the integration line.
	SingleParentEvent
	// MergeEvent is a merge result, diffed against its first parent so the event
	// represents the net change introduced onto the integration line.
	MergeEvent
)

func (k EventKind) String() string {
	switch k {
	case RootEvent:
		return "root"
	case SingleParentEvent:
		return "single-parent"
	case MergeEvent:
		return "merge-result"
	default:
		return "unknown"
	}
}

// Event is one integration event on the first-parent chain.
type Event struct {
	ID   ObjectID
	Kind EventKind
}

// FirstParentEvents streams the first-parent chain from its root to commit and
// returns one event per chain commit, in root-first order.
//
// commit must be a complete lowercase object ID, normally one [ResolveCommit]
// returned. The frozen command carries no `--end-of-options`, so an unvalidated
// argument could otherwise be read as an option.
//
// The command's output is never buffered whole. An octopus merge prints every
// parent on one line, so a line has no upper length, and only each event's own
// ID and kind are retained.
func (r *Repository) FirstParentEvents(ctx context.Context, commit string) ([]Event, error) {
	return r.firstParentEvents(ctx, commit, maxStreamOutput, maxIntegrationEvents)
}

// firstParentEvents carries the limits explicitly so a test can reach them
// without building a repository the size of the frozen caps.
func (r *Repository) firstParentEvents(
	ctx context.Context,
	commit string,
	outputLimit int,
	eventLimit int,
) ([]Event, error) {
	id, err := parseObjectID(commit)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	bound, err := r.bind(
		"rev-list",
		"--first-parent",
		"--reverse",
		"--parents",
		id.String(),
	)
	if err != nil {
		return nil, err
	}

	var events []Event
	parse := func(source io.Reader) error {
		var parseErr error
		events, parseErr = parseFirstParentEvents(source, eventLimit)
		return parseErr
	}
	if err := runStreaming(ctx, "listing integration events", nil, outputLimit, parse, bound...); err != nil {
		return nil, r.classifyFirstParentFailure(ctx, id, err)
	}

	return events, nil
}

// classifyFirstParentFailure checks only a failed first-parent traversal. Git's
// --missing=print output is machine-readable for this exact walk; any failed,
// bounded, or malformed diagnostic preserves the original Git failure instead
// of guessing from stderr.
func (r *Repository) classifyFirstParentFailure(
	ctx context.Context,
	commit ObjectID,
	traversalErr error,
) error {
	var gitErr *GitError
	if !errors.As(traversalErr, &gitErr) {
		return traversalErr
	}

	output, err := r.runScalar(
		ctx,
		"checking first-parent completeness",
		maxScalarOutput,
		"rev-list",
		"--quiet",
		"--first-parent",
		"--missing=print",
		commit.String(),
	)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		return traversalErr
	}

	missing, err := parseMissingObjectReport(output)
	if err != nil || len(missing) == 0 {
		return traversalErr
	}

	confirmed, err := r.confirmMissingObjects(ctx, missing)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		return traversalErr
	}
	if !confirmed {
		return traversalErr
	}

	return fmt.Errorf("%w: first-parent history references an unavailable object", ErrIncompleteRepository)
}

func parseMissingObjectReport(output []byte) ([]ObjectID, error) {
	if len(output) == 0 {
		return nil, nil
	}

	var missing []ObjectID
	for len(output) > 0 {
		if len(output) < 1+sha1HexLength+1 {
			return nil, errors.New("missing-object report ended mid-record")
		}
		if output[0] != missingObjectPrefix ||
			!isObjectID(output[1:1+sha1HexLength]) ||
			output[1+sha1HexLength] != recordSeparator {
			return nil, errors.New("missing-object report has invalid framing")
		}

		var id ObjectID
		copy(id[:], output[1:1+sha1HexLength])
		missing = append(missing, id)
		output = output[1+sha1HexLength+1:]
	}

	return missing, nil
}

func (r *Repository) confirmMissingObjects(ctx context.Context, candidates []ObjectID) (bool, error) {
	stdin := make([]byte, 0, len(candidates)*(sha1HexLength+1))
	for _, id := range candidates {
		stdin = append(stdin, id[:]...)
		stdin = append(stdin, recordSeparator)
	}

	output, err := r.runScalarInput(
		ctx,
		"checking reported missing objects",
		bytes.NewReader(stdin),
		maxScalarOutput,
		"cat-file",
		"--batch-check=%(objectname) %(objecttype)",
	)
	if err != nil {
		return false, err
	}

	return allReportedObjectsMissing(candidates, output)
}

func allReportedObjectsMissing(candidates []ObjectID, output []byte) (bool, error) {
	for _, want := range candidates {
		lineEnd := bytes.IndexByte(output, recordSeparator)
		if lineEnd < 0 {
			return false, errors.New("object check ended mid-record")
		}

		line := output[:lineEnd]
		if len(line) < sha1HexLength+1 ||
			!bytes.Equal(line[:sha1HexLength], want[:]) ||
			line[sha1HexLength] != fieldSeparator {
			return false, errors.New("object check has invalid framing")
		}

		switch string(line[sha1HexLength+1:]) {
		case "missing":
		case "blob", "commit", "tag", "tree":
			return false, nil
		default:
			return false, errors.New("object check has an unknown status")
		}
		output = output[lineEnd+1:]
	}

	if len(output) != 0 {
		return false, errors.New("object check has trailing records")
	}
	return true, nil
}

// parseFirstParentEvents decodes `rev-list --parents` output field by field.
//
// Nothing longer than one object ID is ever held, so an octopus merge line of
// any length costs the same as a root line. The chain is validated as it is
// read: the first event is the chain's root, and every later event names its
// predecessor as its first parent, so a stream that skips or reorders an event
// fails instead of producing a chain that never existed.
func parseFirstParentEvents(source io.Reader, eventLimit int) ([]Event, error) {
	reader := bufio.NewReader(source)
	var events []Event

	for {
		id, separator, err := readField(reader, true)
		if errors.Is(err, errEndOfStream) {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(events) == eventLimit {
			return nil, &EventLimitError{Limit: eventLimit}
		}

		parents := 0
		var firstParent ObjectID
		for separator == fieldSeparator {
			var parent ObjectID
			parent, separator, err = readField(reader, false)
			if err != nil {
				return nil, err
			}
			if parents == 0 {
				firstParent = parent
			}
			parents++
		}

		if err := checkChain(events, id, firstParent, parents); err != nil {
			return nil, err
		}
		events = append(events, Event{ID: id, Kind: kindOf(parents)})
	}

	if len(events) == 0 {
		return nil, fmt.Errorf("%w: no integration event was listed", errTruncatedStream)
	}

	return events, nil
}

// readField reads one object ID and the byte that ends it. atStart marks the
// only position where the stream may legitimately end.
func readField(reader *bufio.Reader, atStart bool) (ObjectID, byte, error) {
	var id ObjectID

	if _, err := io.ReadFull(reader, id[:]); err != nil {
		if atStart && errors.Is(err, io.EOF) {
			return id, 0, errEndOfStream
		}
		return id, 0, fmt.Errorf("%w: integration event ended mid-identifier: %w", errTruncatedStream, err)
	}
	if !isObjectID(id[:]) {
		return id, 0, fmt.Errorf("%w: integration event field is not an object ID", ErrMalformedGitOutput)
	}

	separator, err := reader.ReadByte()
	if err != nil {
		return id, 0, fmt.Errorf("%w: integration event ended after an identifier: %w", errTruncatedStream, err)
	}
	if separator != fieldSeparator && separator != recordSeparator {
		return id, 0, fmt.Errorf(
			"%w: integration event separator is %#x",
			ErrMalformedGitOutput,
			separator,
		)
	}

	return id, separator, nil
}

// checkChain enforces what `--first-parent --reverse` promises.
func checkChain(events []Event, id ObjectID, firstParent ObjectID, parents int) error {
	if len(events) == 0 {
		if parents != 0 {
			return fmt.Errorf(
				"%w: first-parent chain starts at %s, which is not a root commit",
				ErrMalformedGitOutput,
				id,
			)
		}
		return nil
	}

	previous := events[len(events)-1].ID
	if parents == 0 {
		return fmt.Errorf("%w: %s follows %s but has no parent", ErrMalformedGitOutput, id, previous)
	}
	if firstParent != previous {
		return fmt.Errorf(
			"%w: %s names %s as its first parent, but follows %s",
			ErrMalformedGitOutput,
			id,
			firstParent,
			previous,
		)
	}

	return nil
}

func kindOf(parents int) EventKind {
	switch parents {
	case 0:
		return RootEvent
	case 1:
		return SingleParentEvent
	default:
		return MergeEvent
	}
}

// parseObjectID validates before it copies, and reports the length rather than
// the value, so an oversized argument costs neither a copy nor a message.
func parseObjectID(value string) (ObjectID, error) {
	var id ObjectID
	if len(value) != sha1HexLength {
		return id, fmt.Errorf(
			"object ID is %d bytes, want %d",
			len(value),
			sha1HexLength,
		)
	}

	copy(id[:], value)
	if !isObjectID(id[:]) {
		return id, errors.New("object ID is not complete lowercase hexadecimal")
	}

	return id, nil
}
