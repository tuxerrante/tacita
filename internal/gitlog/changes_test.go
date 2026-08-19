package gitlog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEachEventChangeNormalizesEvidence covers one chain holding every
// normalization rule at once: an excluded root, an eligible event with hostile
// path bytes, a vendor segment, a gitlink, an event whose only changes are
// excluded, and an empty event.
func TestEachEventChangeNormalizesEvidence(t *testing.T) {
	repository := newChangeTestRepository(t)
	hostile := "dir/od\ndd \tname\xff.txt"

	commits := []map[string]string{
		{"kept.txt": "1"},
		{hostile: "1", "vendor/x/lib.go": "1", "src/main.go": "1"},
		{"vendor/x/lib.go": "2"},
		{},
	}
	for _, files := range commits {
		commitTree(t, repository, files)
	}
	addGitlink(t, repository, "modules/sub")

	events, err := openTestRepository(t, repository).
		FirstParentEvents(t.Context(), headCommit(t, repository))
	if err != nil {
		t.Fatalf("FirstParentEvents() error = %v", err)
	}

	var visited []EventPaths
	diagnostics, err := openTestRepository(t, repository).
		EachEventChange(t.Context(), events, func(paths EventPaths) error {
			visited = append(visited, paths)
			return nil
		})
	if err != nil {
		t.Fatalf("EachEventChange() error = %v", err)
	}

	want := [][]string{
		{"kept.txt"},
		{hostile, "src/main.go"},
	}
	if len(visited) != len(want) {
		t.Fatalf("visited %d events, want %d: %v", len(visited), len(want), visited)
	}
	for i, paths := range visited {
		if strings.Join(paths.Paths, "\x00") != strings.Join(want[i], "\x00") {
			t.Errorf("event %d paths = %q, want %q", i, paths.Paths, want[i])
		}
	}

	wantCounts := map[Exclusion]uint64{
		{Scope: EventScope, Reason: RootEventReason}:       1,
		{Scope: EventScope, Reason: NoEligiblePathsReason}: 3,
		{Scope: PathScope, Reason: VendorPathReason}:       2,
		{Scope: PathScope, Reason: GitlinkPathReason}:      1,
	}
	assertDiagnostics(t, diagnostics, wantCounts)
}

// TestEachEventChangeExcludesOversizedEvents proves the per-event path budget
// excludes the whole event rather than retaining a prefix, and that the stream
// stays synchronized afterwards.
func TestEachEventChangeExcludesOversizedEvents(t *testing.T) {
	repository := newChangeTestRepository(t)
	commitTree(t, repository, map[string]string{"a.txt": "1", "b.txt": "1", "c.txt": "1"})
	commitTree(t, repository, map[string]string{"after.txt": "1"})

	events, err := openTestRepository(t, repository).
		FirstParentEvents(t.Context(), headCommit(t, repository))
	if err != nil {
		t.Fatalf("FirstParentEvents() error = %v", err)
	}

	var visited []EventPaths
	diagnostics, err := openTestRepository(t, repository).
		eachEventChange(t.Context(), events, func(paths EventPaths) error {
			visited = append(visited, paths)
			return nil
		}, maxStreamOutput, 2)
	if err != nil {
		t.Fatalf("eachEventChange() error = %v", err)
	}

	if len(visited) != 1 || len(visited[0].Paths) != 1 || visited[0].Paths[0] != "after.txt" {
		t.Fatalf("visited = %v, want only the following event", visited)
	}
	assertDiagnostics(t, diagnostics, map[Exclusion]uint64{
		{Scope: EventScope, Reason: RootEventReason}:      1,
		{Scope: EventScope, Reason: EventPathLimitReason}: 1,
	})
}

// TestEachEventChangeReportsVisitorFailure covers the early termination path:
// the visitor's failure survives the cancel, drain, and wait that stopping the
// child requires.
func TestEachEventChangeReportsVisitorFailure(t *testing.T) {
	repository := newChangeTestRepository(t)
	commitTree(t, repository, map[string]string{"a.txt": "1"})
	commitTree(t, repository, map[string]string{"b.txt": "1"})

	events, err := openTestRepository(t, repository).
		FirstParentEvents(t.Context(), headCommit(t, repository))
	if err != nil {
		t.Fatalf("FirstParentEvents() error = %v", err)
	}

	refused := errors.New("refused")
	_, err = openTestRepository(t, repository).
		EachEventChange(t.Context(), events, func(EventPaths) error {
			return refused
		})

	if !errors.Is(err, refused) {
		t.Fatalf("EachEventChange() error = %v, want %v", err, refused)
	}
}

func TestEachEventChangeRejectsAnEmptyChain(t *testing.T) {
	repository, _ := newTestRepository(t)

	_, err := openTestRepository(t, repository).
		EachEventChange(t.Context(), nil, func(EventPaths) error { return nil })

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("EachEventChange() error = %v, want ErrInvalidInput", err)
	}
}

// TestDecodeRejectsDesynchronizedStreams drives the decoder with byte fixtures.
// The frozen command echoes boundaries in the order it was fed, so a real Git
// process cannot produce a desynchronized stream; the guard exists because a
// misattributed path is invisible in the report, and only a fixture can prove it
// holds.
func TestDecodeRejectsDesynchronizedStreams(t *testing.T) {
	first := strings.Repeat("a", sha1HexLength)
	second := strings.Repeat("b", sha1HexLength)
	other := strings.Repeat("c", sha1HexLength)
	blob := strings.Repeat("d", sha1HexLength)
	record := fmt.Sprintf(":100644 100644 %s %s M\x00file.txt\x00", blob, blob)

	chain := []Event{
		{ID: mustObjectID(t, first), Kind: RootEvent},
		{ID: mustObjectID(t, second), Kind: SingleParentEvent},
	}

	// RootEvent is EventKind's zero value, so the guard that rejects a record
	// before every boundary and the guard that rejects a change on the root
	// event both fire on the same zero-value event. The message is asserted so
	// removing either one is still visible.
	tests := []struct {
		name    string
		input   string
		err     error
		message string
	}{
		{
			name:  "unexpected boundary",
			input: first + "\x00" + other + "\x00",
			err:   ErrMalformedGitOutput,
		},
		{
			name:  "boundary after the last event",
			input: first + "\x00" + second + "\x00" + other + "\x00",
			err:   ErrMalformedGitOutput,
		},
		{
			name:  "missing boundary",
			input: first + "\x00",
			err:   errTruncatedStream,
		},
		{
			name:    "record before every boundary",
			input:   record + first + "\x00" + second + "\x00",
			err:     ErrMalformedGitOutput,
			message: "precedes every boundary",
		},
		{
			name:    "change on the root event",
			input:   first + "\x00" + record + second + "\x00",
			err:     ErrMalformedGitOutput,
			message: "root integration event",
		},
		{
			name:  "unterminated boundary",
			input: first,
			err:   errTruncatedStream,
		},
		{
			name:  "unterminated path",
			input: first + "\x00" + second + "\x00" + strings.TrimSuffix(record, "\x00"),
			err:   errTruncatedStream,
		},
		{
			name:  "empty path",
			input: first + "\x00" + second + "\x00" + fmt.Sprintf(":100644 100644 %s %s M\x00\x00", blob, blob),
			err:   ErrMalformedGitOutput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := &changeDecoder{
				events:      chain,
				visit:       func(EventPaths) error { return nil },
				pathLimit:   maxEventPaths,
				diagnostics: Diagnostics{},
				seen:        make(map[string]struct{}),
			}

			err := decoder.decode(strings.NewReader(tt.input))

			if !errors.Is(err, tt.err) {
				t.Fatalf("decode() error = %v, want %v", err, tt.err)
			}
			if tt.message != "" && !strings.Contains(err.Error(), tt.message) {
				t.Errorf("decode() error = %v, want it to mention %q", err, tt.message)
			}
		})
	}
}

// TestDecodeDeduplicatesPaths pins the rule that a path is counted once per
// event. The frozen command reports each path once per event, so only a fixture
// can reach the guard, which exists because a repeated path would otherwise
// inflate both the evidence and the event's path budget.
func TestDecodeDeduplicatesPaths(t *testing.T) {
	first := strings.Repeat("a", sha1HexLength)
	second := strings.Repeat("b", sha1HexLength)
	blob := strings.Repeat("d", sha1HexLength)
	record := func(path string) string {
		return fmt.Sprintf(":100644 100644 %s %s M\x00%s\x00", blob, blob, path)
	}

	var visited []EventPaths
	decoder := &changeDecoder{
		events: []Event{
			{ID: mustObjectID(t, first), Kind: RootEvent},
			{ID: mustObjectID(t, second), Kind: SingleParentEvent},
		},
		visit:       func(paths EventPaths) error { visited = append(visited, paths); return nil },
		pathLimit:   2,
		diagnostics: Diagnostics{},
		seen:        make(map[string]struct{}),
	}

	input := first + "\x00" + second + "\x00" +
		record("a.txt") + record("a.txt") + record("b.txt")
	if err := decoder.decode(strings.NewReader(input)); err != nil {
		t.Fatalf("decode() error = %v", err)
	}

	if len(visited) != 1 {
		t.Fatalf("visited %d events, want 1", len(visited))
	}
	if got := visited[0].Paths; strings.Join(got, ",") != "a.txt,b.txt" {
		t.Errorf("paths = %q, want [a.txt b.txt]", got)
	}
	assertDiagnostics(t, decoder.diagnostics, map[Exclusion]uint64{
		{Scope: EventScope, Reason: RootEventReason}: 1,
	})
}

func TestParseRecordHeader(t *testing.T) {
	blob := strings.Repeat("a", sha1HexLength)
	header := func(source string, destination string, status string) string {
		return fmt.Sprintf("%s %s %s %s %s", source, destination, blob, blob, status)
	}

	tests := []struct {
		name    string
		header  string
		gitlink bool
		err     error
	}{
		{name: "modification", header: header("100644", "100644", "M")},
		{name: "addition", header: header("000000", "100644", "A")},
		{name: "deletion", header: header("100644", "000000", "D")},
		{name: "type change", header: header("100644", "120000", "T")},
		{name: "gitlink source", header: header("160000", "000000", "D"), gitlink: true},
		{name: "gitlink destination", header: header("000000", "160000", "A"), gitlink: true},
		{name: "rename", header: header("100644", "100644", "R"), err: ErrMalformedGitOutput},
		{name: "copy", header: header("100644", "100644", "C"), err: ErrMalformedGitOutput},
		{name: "unknown status", header: header("100644", "100644", "X"), err: ErrMalformedGitOutput},
		{name: "non-octal mode", header: header("1006m4", "100644", "M"), err: ErrMalformedGitOutput},
		{
			name:   "abbreviated object ID",
			header: fmt.Sprintf("100644 100644 %s %s M", strings.Repeat("a", 39)+"z", blob),
			err:    ErrMalformedGitOutput,
		},
		{
			name:   "wrong separator",
			header: fmt.Sprintf("100644\t100644 %s %s M", blob, blob),
			err:    ErrMalformedGitOutput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.header) != recordHeaderLength {
				t.Fatalf("test header is %d bytes, want %d", len(tt.header), recordHeaderLength)
			}

			gitlink, err := parseRecordHeader([]byte(tt.header))

			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("parseRecordHeader() error = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRecordHeader() error = %v", err)
			}
			if gitlink != tt.gitlink {
				t.Errorf("parseRecordHeader() gitlink = %t, want %t", gitlink, tt.gitlink)
			}
		})
	}
}

func TestIsVendorPath(t *testing.T) {
	tests := map[string]bool{
		"vendor":            true,
		"vendor/x/lib.go":   true,
		"a/vendor/lib.go":   true,
		"a/b/vendor":        true,
		"vendored/lib.go":   false,
		"Vendor/lib.go":     false,
		"a/myvendor/lib.go": false,
		"a/vendor.go":       false,
	}

	for path, want := range tests {
		if got := isVendorPath(path); got != want {
			t.Errorf("isVendorPath(%q) = %t, want %t", path, got, want)
		}
	}
}

func assertDiagnostics(t *testing.T, got Diagnostics, want map[Exclusion]uint64) {
	t.Helper()

	for exclusion, count := range want {
		if got[exclusion] != count {
			t.Errorf("%s/%s count = %d, want %d",
				exclusion.Scope, exclusion.Reason, got[exclusion], count)
		}
	}
	for exclusion, count := range got {
		if count != 0 && want[exclusion] == 0 {
			t.Errorf("unexpected %s/%s count = %d", exclusion.Scope, exclusion.Reason, count)
		}
	}
}

// newChangeTestRepository creates an empty repository holding only its root
// commit, so a test can append exactly the events it needs.
func newChangeTestRepository(t *testing.T) string {
	t.Helper()

	repository := filepath.Join(t.TempDir(), "changes")
	runTestGit(t, "init", "--quiet", "--object-format=sha1", "--initial-branch=main", repository)
	runTestGit(t, "-C", repository, "-c", "user.name=Tacita Test",
		"-c", "user.email=tacita@example.invalid",
		"commit", "--quiet", "--allow-empty", "-m", "root")

	return repository
}

// commitTree writes files and commits them, replacing nothing, so each call adds
// exactly one integration event. Paths are written through the index rather than
// the worktree, which keeps bytes Git accepts but a filesystem may not.
func commitTree(t *testing.T, repository string, files map[string]string) {
	t.Helper()

	for path, content := range files {
		id := hashObject(t, repository, content)
		runTestGit(t, "-C", repository, "update-index", "--add", "--cacheinfo",
			fmt.Sprintf("100644,%s,%s", id, path))
	}
	runTestGit(t, "-C", repository, "-c", "user.name=Tacita Test",
		"-c", "user.email=tacita@example.invalid",
		"commit", "--quiet", "--allow-empty", "-m", "event")
}

// addGitlink commits a submodule entry without a submodule: the gitlink is a
// tree entry, and mode 160000 is all the normalization looks at.
func addGitlink(t *testing.T, repository string, path string) {
	t.Helper()

	runTestGit(t, "-C", repository, "update-index", "--add", "--cacheinfo",
		fmt.Sprintf("160000,%s,%s", headCommit(t, repository), path))
	runTestGit(t, "-C", repository, "-c", "user.name=Tacita Test",
		"-c", "user.email=tacita@example.invalid",
		"commit", "--quiet", "-m", "gitlink")
}

func hashObject(t *testing.T, repository string, content string) string {
	t.Helper()

	blob := filepath.Join(t.TempDir(), "blob")
	if err := os.WriteFile(blob, []byte(content), 0o600); err != nil {
		t.Fatalf("writing blob: %v", err)
	}
	return runTestGit(t, "-C", repository, "hash-object", "-w", "--", blob)
}

func headCommit(t *testing.T, repository string) string {
	t.Helper()

	return runTestGit(t, "-C", repository, "rev-parse", "HEAD")
}

// FuzzChangeDecoder asserts that no byte sequence can make the decoder panic,
// exceed the path budget, or attribute a path to the root event.
func FuzzChangeDecoder(f *testing.F) {
	first := strings.Repeat("a", sha1HexLength)
	second := strings.Repeat("b", sha1HexLength)
	blob := strings.Repeat("d", sha1HexLength)
	record := fmt.Sprintf(":100644 100644 %s %s M\x00file.txt\x00", blob, blob)

	f.Add(first + "\x00" + second + "\x00")
	f.Add(first + "\x00" + second + "\x00" + record)
	f.Add(first + "\x00" + record)

	events := []Event{
		{ID: mustObjectID(f, first), Kind: RootEvent},
		{ID: mustObjectID(f, second), Kind: SingleParentEvent},
	}

	f.Fuzz(func(t *testing.T, input string) {
		const pathLimit = 4

		var visited []EventPaths
		decoder := &changeDecoder{
			events:      events,
			visit:       func(paths EventPaths) error { visited = append(visited, paths); return nil },
			pathLimit:   pathLimit,
			diagnostics: Diagnostics{},
			seen:        make(map[string]struct{}),
		}

		if err := decoder.decode(strings.NewReader(input)); err != nil {
			// Every rejection speaks the boundary's vocabulary. An error that
			// escapes untyped would reach the CLI as an unexplained failure.
			if !errors.Is(err, ErrMalformedGitOutput) {
				t.Fatalf("rejected input with an unclassified error: %v", err)
			}
			return
		}
		// Boundaries are matched against the chain, so an accepted stream
		// visits events in chain order and never revisits one.
		next := 0
		for _, paths := range visited {
			for next < len(events) && events[next].ID != paths.Event.ID {
				next++
			}
			if next == len(events) {
				t.Fatalf("visited %s out of chain order", paths.Event.ID)
			}
			next++
		}
		for _, paths := range visited {
			if paths.Event.Kind == RootEvent {
				t.Fatalf("attributed %d paths to the root event", len(paths.Paths))
			}
			if len(paths.Paths) > pathLimit {
				t.Fatalf("emitted %d paths, above the %d limit", len(paths.Paths), pathLimit)
			}
		}
	})
}
