package gitlog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

// TestFirstParentEventsWalksTheChain uses a repository holding each event kind:
// a root, ordinary commits, and an octopus merge whose line carries four
// parents.
func TestFirstParentEventsWalksTheChain(t *testing.T) {
	repository, ids := newBranchedTestRepository(t)

	events, err := openTestRepository(t, repository).
		FirstParentEvents(t.Context(), ids["merge"])
	if err != nil {
		t.Fatalf("FirstParentEvents() error = %v", err)
	}

	want := []Event{
		{ID: mustObjectID(t, ids["root"]), Kind: RootEvent},
		{ID: mustObjectID(t, ids["main"]), Kind: SingleParentEvent},
		{ID: mustObjectID(t, ids["merge"]), Kind: MergeEvent},
	}
	if len(events) != len(want) {
		t.Fatalf("FirstParentEvents() returned %d events, want %d", len(events), len(want))
	}
	for i, event := range events {
		if event != want[i] {
			t.Errorf("event %d = {%s %s}, want {%s %s}",
				i, event.ID, event.Kind, want[i].ID, want[i].Kind)
		}
	}
}

// TestFirstParentEventsRejectsUnvalidatedRevision covers the missing
// `--end-of-options` in the frozen command: the argument is an object ID or the
// command does not run at all.
func TestFirstParentEventsRejectsUnvalidatedRevision(t *testing.T) {
	repository, _ := newTestRepository(t)
	opened := openTestRepository(t, repository)

	for _, revision := range []string{"", "HEAD", "--help", strings.Repeat("A", 40)} {
		if _, err := opened.FirstParentEvents(t.Context(), revision); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("FirstParentEvents(%q) error = %v, want ErrInvalidInput", revision, err)
		}
	}
}

// TestFirstParentEventsEnforcesTheEventLimit reaches the frozen cap through the
// injected limit, so the budget is proven without building a repository the size
// of the budget.
func TestFirstParentEventsEnforcesTheEventLimit(t *testing.T) {
	repository, ids := newBranchedTestRepository(t)
	opened := openTestRepository(t, repository)

	if _, err := opened.firstParentEvents(t.Context(), ids["merge"], maxStreamOutput, 3); err != nil {
		t.Fatalf("firstParentEvents() with a limit of 3 error = %v", err)
	}

	_, err := opened.firstParentEvents(t.Context(), ids["merge"], maxStreamOutput, 2)
	if !errors.Is(err, ErrEventLimit) {
		t.Fatalf("firstParentEvents() with a limit of 2 error = %v, want ErrEventLimit", err)
	}
}

// TestFirstParentEventsEnforcesTheOutputLimit pins the byte budget the parser
// itself does not bound. A root event is exactly 41 bytes on the wire.
func TestFirstParentEventsEnforcesTheOutputLimit(t *testing.T) {
	repository, ids := newBranchedTestRepository(t)
	opened := openTestRepository(t, repository)

	if _, err := opened.firstParentEvents(t.Context(), ids["root"], 41, maxIntegrationEvents); err != nil {
		t.Fatalf("firstParentEvents() with a limit of 41 bytes error = %v", err)
	}

	_, err := opened.firstParentEvents(t.Context(), ids["root"], 40, maxIntegrationEvents)
	if !errors.Is(err, ErrGitOutputLimit) {
		t.Fatalf("firstParentEvents() with a limit of 40 bytes error = %v, want ErrGitOutputLimit", err)
	}
}

// TestFirstParentEventsReportsIncompleteRepository proves a missing commit
// discovered after Open is classified from machine-readable Git output rather
// than stderr text.
func TestFirstParentEventsReportsIncompleteRepository(t *testing.T) {
	repository, ids := newBranchedTestRepository(t)
	opened := openTestRepository(t, repository)
	removeObject(t, repository, ids["main"])

	_, err := opened.FirstParentEvents(t.Context(), ids["merge"])

	if !errors.Is(err, ErrIncompleteRepository) {
		t.Fatalf("FirstParentEvents() error = %v, want ErrIncompleteRepository", err)
	}
	if errors.Is(err, ErrGitFailure) {
		t.Fatalf("FirstParentEvents() error = %v, also matches ErrGitFailure", err)
	}
}

func TestFirstParentEventsPreservesUnclassifiableGitFailure(t *testing.T) {
	repository, ids := newBranchedTestRepository(t)
	opened := openTestRepository(t, repository)

	objects := filepath.Join(repository, gitDirEntry, "objects")
	if err := os.Rename(objects, objects+".unavailable"); err != nil {
		t.Fatalf("making object store unavailable: %v", err)
	}

	_, err := opened.FirstParentEvents(t.Context(), ids["merge"])

	if !errors.Is(err, ErrGitFailure) {
		t.Fatalf("FirstParentEvents() error = %v, want ErrGitFailure", err)
	}
	if errors.Is(err, ErrIncompleteRepository) {
		t.Fatalf("FirstParentEvents() error = %v, unexpectedly matches ErrIncompleteRepository", err)
	}
}

func TestReportsMissingObjects(t *testing.T) {
	id := strings.Repeat("a", sha1HexLength)
	other := strings.Repeat("b", sha1HexLength)

	tests := []struct {
		name    string
		output  string
		missing bool
		wantErr bool
	}{
		{name: "empty"},
		{name: "one", output: "?" + id + "\n", missing: true},
		{name: "multiple", output: "?" + id + "\n?" + other + "\n", missing: true},
		{name: "plain object ID", output: id + "\n", wantErr: true},
		{name: "invalid object ID", output: "?" + strings.Repeat("g", sha1HexLength) + "\n", wantErr: true},
		{name: "missing newline", output: "?" + id, wantErr: true},
		{name: "trailing data", output: "?" + id + "\nextra", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing, err := reportsMissingObjects([]byte(tt.output))
			if (err != nil) != tt.wantErr {
				t.Fatalf("reportsMissingObjects() error = %v, wantErr %t", err, tt.wantErr)
			}
			if missing != tt.missing {
				t.Errorf("reportsMissingObjects() = %t, want %t", missing, tt.missing)
			}
		})
	}
}

func TestParseFirstParentEvents(t *testing.T) {
	root := strings.Repeat("a", 40)
	second := strings.Repeat("b", 40)
	other := strings.Repeat("c", 40)

	tests := []struct {
		name  string
		input string
		want  []EventKind
		err   error
	}{
		{
			name:  "root only",
			input: root + "\n",
			want:  []EventKind{RootEvent},
		},
		{
			name:  "single parent",
			input: root + "\n" + second + " " + root + "\n",
			want:  []EventKind{RootEvent, SingleParentEvent},
		},
		{
			name:  "octopus merge",
			input: root + "\n" + second + " " + root + " " + other + " " + other + "\n",
			want:  []EventKind{RootEvent, MergeEvent},
		},
		{name: "empty stream", err: errTruncatedStream},
		{name: "missing final newline", input: root, err: errTruncatedStream},
		{name: "identifier cut short", input: root[:39] + "\n", err: ErrMalformedGitOutput},
		{name: "uppercase identifier", input: strings.ToUpper(root) + "\n", err: ErrMalformedGitOutput},
		{name: "carriage return", input: root + "\r\n", err: ErrMalformedGitOutput},
		{name: "double separator", input: root + "  " + second + "\n", err: ErrMalformedGitOutput},
		{name: "trailing separator", input: root + " \n", err: ErrMalformedGitOutput},
		{
			name:  "second root",
			input: root + "\n" + second + "\n",
			err:   ErrMalformedGitOutput,
		},
		{
			name:  "chain starts below the root",
			input: second + " " + root + "\n",
			err:   ErrMalformedGitOutput,
		},
		{
			name:  "broken first-parent link",
			input: root + "\n" + second + " " + other + "\n",
			err:   ErrMalformedGitOutput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// One byte at a time proves the parser never relies on a read
			// returning a whole field.
			events, err := parseFirstParentEvents(
				iotest.OneByteReader(strings.NewReader(tt.input)),
				maxIntegrationEvents,
			)

			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("parseFirstParentEvents() error = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFirstParentEvents() error = %v", err)
			}
			if len(events) != len(tt.want) {
				t.Fatalf("parsed %d events, want %d", len(events), len(tt.want))
			}
			for i, event := range events {
				if event.Kind != tt.want[i] {
					t.Errorf("event %d kind = %s, want %s", i, event.Kind, tt.want[i])
				}
			}
		})
	}
}

func FuzzParseFirstParentEvents(f *testing.F) {
	root := strings.Repeat("a", 40)
	f.Add(root + "\n")
	f.Add(root + "\n" + strings.Repeat("b", 40) + " " + root + "\n")
	f.Add(root)

	f.Fuzz(func(t *testing.T, input string) {
		events, err := parseFirstParentEvents(strings.NewReader(input), maxIntegrationEvents)
		if err != nil {
			// Every rejection speaks the boundary's vocabulary. An error that
			// escapes untyped would reach the CLI as an unexplained failure.
			if !errors.Is(err, ErrMalformedGitOutput) && !errors.Is(err, ErrEventLimit) {
				t.Fatalf("rejected input with an unclassified error: %v", err)
			}
			return
		}
		if len(events) == 0 {
			t.Fatal("accepted an input that produced no event")
		}
		if events[0].Kind != RootEvent {
			t.Errorf("accepted a chain starting with %s", events[0].Kind)
		}
		// Linkage is validated while parsing and not retained, so what stays
		// observable is that the chain holds exactly one root.
		for i, event := range events[1:] {
			if event.Kind == RootEvent {
				t.Fatalf("event %d is a second root", i+1)
			}
		}
	})
}

func mustObjectID(t testing.TB, value string) ObjectID {
	t.Helper()

	id, err := parseObjectID(value)
	if err != nil {
		t.Fatalf("parseObjectID(%q) error = %v", value, err)
	}
	return id
}

// newBranchedTestRepository builds a chain holding every event kind: a root, an
// ordinary commit, and an octopus merge of three side commits. Commits are
// written directly with commit-tree, so the shape is exact and no worktree
// operation is involved.
func newBranchedTestRepository(t *testing.T) (string, map[string]string) {
	t.Helper()

	repository := filepath.Join(t.TempDir(), "branched")
	runTestGit(t, "init", "--quiet", "--object-format=sha1", repository)
	tree := runTestGit(t, "-C", repository, "hash-object", "-t", "tree", "-w", "--stdin", "--path", "x")

	ids := map[string]string{}
	commit := func(name string, parents ...string) {
		args := []string{
			"-C", repository,
			"-c", "user.name=Tacita Test",
			"-c", "user.email=tacita@example.invalid",
			"commit-tree", tree, "-m", name,
		}
		for _, parent := range parents {
			args = append(args, "-p", parent)
		}
		ids[name] = runTestGit(t, args...)
	}

	commit("root")
	commit("main", ids["root"])
	for _, side := range []string{"side1", "side2", "side3"} {
		commit(side, ids["root"])
	}
	commit("merge", ids["main"], ids["side1"], ids["side2"], ids["side3"])
	runTestGit(t, "-C", repository, "update-ref", "refs/heads/main", ids["merge"])

	return repository, ids
}

func removeObject(t *testing.T, repository string, id string) {
	t.Helper()

	path := filepath.Join(repository, gitDirEntry, "objects", id[:2], id[2:])
	if err := os.Remove(path); err != nil {
		t.Fatalf("removing object: %v", err)
	}
}

// TestFirstParentEventsReportsCallerCancellation pins the highest precedence in
// the streamed classification: a caller that gave up is reported as such, not as
// the killed process or truncated stream its cancellation produces.
func TestFirstParentEventsReportsCallerCancellation(t *testing.T) {
	repository, ids := newBranchedTestRepository(t)
	opened := openTestRepository(t, repository)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := opened.FirstParentEvents(ctx, ids["merge"])

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("FirstParentEvents() error = %v, want context.Canceled", err)
	}
}

// TestFirstParentEventsRejectsAZeroEventLimit covers the degenerate budget: no
// event is allowed, so the first one already exceeds it.
func TestFirstParentEventsRejectsAZeroEventLimit(t *testing.T) {
	repository, ids := newBranchedTestRepository(t)

	_, err := openTestRepository(t, repository).
		firstParentEvents(t.Context(), ids["root"], maxStreamOutput, 0)

	if !errors.Is(err, ErrEventLimit) {
		t.Fatalf("firstParentEvents() error = %v, want ErrEventLimit", err)
	}
}

func TestEventKindString(t *testing.T) {
	tests := map[EventKind]string{
		RootEvent:         "root",
		SingleParentEvent: "single-parent",
		MergeEvent:        "merge-result",
		EventKind(9):      "unknown",
	}

	for kind, want := range tests {
		if got := kind.String(); got != want {
			t.Errorf("EventKind(%d).String() = %q, want %q", kind, got, want)
		}
	}
}
