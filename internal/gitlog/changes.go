package gitlog

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// maxEventPaths is the frozen cap on changed paths in one non-root event.
	maxEventPaths = 2_000

	recordPrefix    = ':'
	tokenTerminator = 0
	gitlinkMode     = "160000"
	vendorSegment   = "vendor"
	pathSeparator   = '/'

	// recordHeaderLength is the fixed part of a raw record after its `:`, up to
	// and including the status byte:
	// `<mode> <mode> <oid> <oid> <status>`.
	recordHeaderLength = 6 + 1 + 6 + 1 + sha1HexLength + 1 + sha1HexLength + 1 + 1
	modeLength         = 6
)

// ExclusionScope says whether an exclusion removed a whole event or one path.
type ExclusionScope string

const (
	EventScope ExclusionScope = "event"
	PathScope  ExclusionScope = "path"
)

// ExclusionReason is a frozen reason code from the report vocabulary.
type ExclusionReason string

const (
	RootEventReason       ExclusionReason = "root_event"
	NoEligiblePathsReason ExclusionReason = "no_eligible_paths"
	EventPathLimitReason  ExclusionReason = "event_path_limit"
	VendorPathReason      ExclusionReason = "vendor_path"
	GitlinkPathReason     ExclusionReason = "gitlink_path"
)

// Exclusion identifies one counted exclusion.
type Exclusion struct {
	Scope  ExclusionScope
	Reason ExclusionReason
}

// Diagnostics counts every exclusion. Nothing is dropped silently: an event or
// path that does not reach the visitor is counted here under exactly one
// reason.
type Diagnostics map[Exclusion]uint64

func (d Diagnostics) record(scope ExclusionScope, reason ExclusionReason, count uint64) {
	d[Exclusion{Scope: scope, Reason: reason}] += count
}

// EventPaths is one eligible integration event and the distinct paths it
// changed, in the order Git reported them.
//
// Paths are raw repository bytes. They are not required to be valid UTF-8 and
// may contain spaces, tabs, and newlines. Each value is owned by the receiver:
// the decoder never reuses or mutates a slice it has handed over.
type EventPaths struct {
	Event Event
	Paths []string
}

// EachEventChange streams the changes of every event and calls visit once for
// each eligible one, in chain order.
//
// events must be the sequence [Repository.FirstParentEvents] returned, because
// every `diff-tree` boundary is matched against it. A stream that skipped,
// repeated, or reordered a boundary fails rather than attributing paths to the
// wrong event.
//
// The root event is never visited: the frozen flow omits `--root`, so its diff
// against the empty tree is recorded but not mined. Excluded events and paths
// are counted in the returned diagnostics, which are complete only when the
// error is nil.
func (r *Repository) EachEventChange(
	ctx context.Context,
	events []Event,
	visit func(EventPaths) error,
) (Diagnostics, error) {
	return r.eachEventChange(ctx, events, visit, maxStreamOutput, maxEventPaths)
}

// eachEventChange carries the limits explicitly so a test can reach them without
// building a repository the size of the frozen caps.
func (r *Repository) eachEventChange(
	ctx context.Context,
	events []Event,
	visit func(EventPaths) error,
	outputLimit int,
	pathLimit int,
) (Diagnostics, error) {
	bound, err := r.bind(
		"diff-tree",
		"--stdin",
		"--always",
		"-r",
		"--raw",
		"-z",
		"--abbrev=40",
		"--no-renames",
		"--no-textconv",
		"--no-ext-diff",
		"--diff-merges=first-parent",
	)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("%w: no integration event was supplied", ErrInvalidInput)
	}

	decoder := &changeDecoder{
		events:      events,
		visit:       visit,
		pathLimit:   pathLimit,
		diagnostics: Diagnostics{},
		seen:        make(map[string]struct{}),
	}
	parse := func(source io.Reader) error {
		return decoder.decode(source)
	}

	err = runStreaming(
		ctx,
		"reading integration event changes",
		eventIDReader(events),
		outputLimit,
		parse,
		bound...,
	)
	if err != nil {
		return nil, err
	}

	return decoder.diagnostics, nil
}

// eventIDReader builds the stdin `diff-tree` consumes: one object ID per line,
// in chain order. os/exec owns the goroutine that writes it, which is finite
// because the reader is.
func eventIDReader(events []Event) io.Reader {
	stdin := make([]byte, 0, len(events)*(sha1HexLength+1))
	for _, event := range events {
		stdin = append(stdin, event.ID[:]...)
		stdin = append(stdin, '\n')
	}
	return bytes.NewReader(stdin)
}

// changeDecoder turns the NUL-framed `diff-tree` stream into eligible events.
//
// It holds one event's paths at a time. A record token is recognized by its
// leading `:`, which no object ID can start with, so boundaries and records
// never need lookahead beyond one byte.
type changeDecoder struct {
	events    []Event
	next      int
	pathLimit int
	visit     func(EventPaths) error

	diagnostics Diagnostics

	open      bool
	current   Event
	seen      map[string]struct{}
	paths     []string
	distinct  int
	overLimit bool
	vendor    uint64
	gitlink   uint64
}

func (d *changeDecoder) decode(source io.Reader) error {
	reader := bufio.NewReader(source)

	for {
		first, err := reader.ReadByte()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: reading a change record: %w", errTruncatedStream, err)
		}

		if first == recordPrefix {
			if err := d.readRecord(reader); err != nil {
				return err
			}
			continue
		}

		if err := reader.UnreadByte(); err != nil {
			return fmt.Errorf("%w: reading a change boundary: %w", errTruncatedStream, err)
		}
		if err := d.readBoundary(reader); err != nil {
			return err
		}
	}

	if err := d.closeEvent(); err != nil {
		return err
	}
	if d.next != len(d.events) {
		return fmt.Errorf(
			"%w: changes ended after %d of %d integration events",
			errTruncatedStream,
			d.next,
			len(d.events),
		)
	}

	return nil
}

// readBoundary starts the next event and checks it against the chain.
func (d *changeDecoder) readBoundary(reader *bufio.Reader) error {
	var id ObjectID
	if _, err := io.ReadFull(reader, id[:]); err != nil {
		return fmt.Errorf("%w: change boundary ended mid-identifier: %w", errTruncatedStream, err)
	}
	if !isObjectID(id[:]) {
		return fmt.Errorf("%w: change boundary is not an object ID", ErrMalformedGitOutput)
	}
	if err := expectTerminator(reader, "change boundary"); err != nil {
		return err
	}

	if err := d.closeEvent(); err != nil {
		return err
	}
	if d.next == len(d.events) {
		return fmt.Errorf(
			"%w: changes list %s after the last integration event",
			ErrMalformedGitOutput,
			id,
		)
	}
	if expected := d.events[d.next].ID; id != expected {
		return fmt.Errorf(
			"%w: changes list %s where integration event %s was expected",
			ErrMalformedGitOutput,
			id,
			expected,
		)
	}

	d.current = d.events[d.next]
	d.next++
	d.open = true

	return nil
}

// readRecord decodes one raw record and folds its path into the open event.
func (d *changeDecoder) readRecord(reader *bufio.Reader) error {
	header := make([]byte, recordHeaderLength)
	if _, err := io.ReadFull(reader, header); err != nil {
		return fmt.Errorf("%w: change record ended mid-header: %w", errTruncatedStream, err)
	}

	gitlink, err := parseRecordHeader(header)
	if err != nil {
		return err
	}
	if err := expectTerminator(reader, "change record"); err != nil {
		return err
	}

	path, err := readPath(reader)
	if err != nil {
		return err
	}
	if !d.open {
		return fmt.Errorf("%w: change record precedes every boundary", ErrMalformedGitOutput)
	}
	if d.current.Kind == RootEvent {
		return fmt.Errorf(
			"%w: root integration event %s reported a change",
			ErrMalformedGitOutput,
			d.current.ID,
		)
	}

	d.addPath(path, gitlink)

	return nil
}

// addPath applies the frozen normalization order: deduplicate the raw path,
// count it against the event's path budget, and only then decide whether it is
// eligible. Filtering first would let a repository put unlimited excluded paths
// in one event without ever reaching the budget.
func (d *changeDecoder) addPath(path string, gitlink bool) {
	if d.overLimit {
		// The event is already excluded whole, so nothing more about it is
		// retained, not even what would be needed to deduplicate it. Its
		// records are still decoded, because the stream must stay synchronized
		// with the chain.
		return
	}
	if _, duplicate := d.seen[path]; duplicate {
		return
	}
	d.seen[path] = struct{}{}

	d.distinct++
	if d.distinct > d.pathLimit {
		d.overLimit = true
		d.paths = nil
		clear(d.seen)
		return
	}

	switch {
	case isVendorPath(path):
		d.vendor++
	case gitlink:
		d.gitlink++
	default:
		d.paths = append(d.paths, path)
	}
}

// closeEvent decides the open event and resets the per-event state.
//
// Exclusion reasons are disjoint and ordered: the root event is excluded as
// such, an oversized event is excluded whole and contributes no path counts,
// and only a surviving event can be excluded for having no eligible path.
func (d *changeDecoder) closeEvent() error {
	if !d.open {
		return nil
	}
	defer d.reset()

	switch {
	case d.current.Kind == RootEvent:
		d.diagnostics.record(EventScope, RootEventReason, 1)
		return nil
	case d.overLimit:
		d.diagnostics.record(EventScope, EventPathLimitReason, 1)
		return nil
	}

	d.diagnostics.record(PathScope, VendorPathReason, d.vendor)
	d.diagnostics.record(PathScope, GitlinkPathReason, d.gitlink)

	if len(d.paths) == 0 {
		d.diagnostics.record(EventScope, NoEligiblePathsReason, 1)
		return nil
	}

	// The slice is handed over and never touched again, so a consumer may
	// retain it.
	visited := EventPaths{Event: d.current, Paths: d.paths}
	d.paths = nil

	return d.visit(visited)
}

func (d *changeDecoder) reset() {
	d.open = false
	d.paths = nil
	d.distinct = 0
	d.overLimit = false
	d.vendor = 0
	d.gitlink = 0
	clear(d.seen)
}

// parseRecordHeader validates the fixed record grammar and reports whether
// either side of the change is a gitlink.
func parseRecordHeader(header []byte) (bool, error) {
	source := header[0:modeLength]
	destination := header[modeLength+1 : modeLength+1+modeLength]
	sourceID := header[14 : 14+sha1HexLength]
	destinationID := header[55 : 55+sha1HexLength]
	status := header[recordHeaderLength-1]

	if header[modeLength] != ' ' || header[13] != ' ' || header[54] != ' ' || header[95] != ' ' {
		return false, fmt.Errorf("%w: change record fields are not space separated", ErrMalformedGitOutput)
	}
	if !isOctalMode(source) || !isOctalMode(destination) {
		return false, fmt.Errorf("%w: change record mode is not octal", ErrMalformedGitOutput)
	}
	if !isObjectID(sourceID) || !isObjectID(destinationID) {
		return false, fmt.Errorf("%w: change record object ID is malformed", ErrMalformedGitOutput)
	}

	switch status {
	case 'A', 'D', 'M', 'T':
	case 'R', 'C':
		return false, fmt.Errorf(
			"%w: change record reports status %q with rename detection disabled",
			ErrMalformedGitOutput,
			status,
		)
	default:
		return false, fmt.Errorf(
			"%w: change record status is %#x",
			ErrMalformedGitOutput,
			status,
		)
	}

	isGitlink := string(source) == gitlinkMode || string(destination) == gitlinkMode

	return isGitlink, nil
}

// readPath reads one NUL-terminated path. Path bytes receive no interpretation:
// no cleaning, case folding, Unicode normalization, or filesystem access.
func readPath(reader *bufio.Reader) (string, error) {
	var path strings.Builder

	for {
		fragment, err := reader.ReadSlice(tokenTerminator)
		path.Write(fragment)
		if err == nil {
			break
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return "", fmt.Errorf("%w: change record ended mid-path: %w", errTruncatedStream, err)
	}

	// Every fragment loop above ends on the terminator, which is not part of
	// the path.
	value := path.String()
	value = value[:len(value)-1]
	if value == "" {
		return "", fmt.Errorf("%w: change record path is empty", ErrMalformedGitOutput)
	}

	return value, nil
}

// isVendorPath reports whether any `/` separated segment is exactly `vendor`.
// The comparison is byte exact: `Vendor` and `vendored` are ordinary paths.
func isVendorPath(path string) bool {
	for segment := range strings.SplitSeq(path, string(pathSeparator)) {
		if segment == vendorSegment {
			return true
		}
	}
	return false
}

func isOctalMode(mode []byte) bool {
	for _, b := range mode {
		if b < '0' || b > '7' {
			return false
		}
	}
	return true
}

func expectTerminator(reader *bufio.Reader, token string) error {
	terminator, err := reader.ReadByte()
	if err != nil {
		return fmt.Errorf("%w: %s is unterminated: %w", errTruncatedStream, token, err)
	}
	if terminator != tokenTerminator {
		return fmt.Errorf("%w: %s ends with %#x", ErrMalformedGitOutput, token, terminator)
	}
	return nil
}
