package backtest

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tuxerrante/tacita/internal/gitlog"
)

func testObjectID(n int) gitlog.ObjectID {
	var id gitlog.ObjectID
	copy(id[:], fmt.Sprintf("%040x", n))
	return id
}

func testChain(length int) []gitlog.Event {
	events := make([]gitlog.Event, length)
	for index := range events {
		events[index] = gitlog.Event{ID: testObjectID(index + 1), Kind: gitlog.SingleParentEvent}
	}
	events[0].Kind = gitlog.RootEvent
	return events
}

func TestPositionerSkipsExcludedEvents(t *testing.T) {
	events := testChain(6)
	positioner := NewPositioner(events)

	for _, index := range []int{1, 3, 4} {
		components := []string{fmt.Sprintf("c%d", index)}
		got, err := positioner.Position(gitlog.Transaction{
			Event:      events[index],
			Paths:      []string{"ignored"},
			Components: components,
		})
		if err != nil {
			t.Fatalf("Position(event %d) error = %v", index, err)
		}
		if got.Position != index+1 || got.Event != events[index] {
			t.Errorf("Position(event %d) = position %d event %s, want position %d event %s",
				index, got.Position, got.Event.ID, index+1, events[index].ID)
		}
		if &got.Components[0] != &components[0] {
			t.Errorf("Position(event %d) copied the components instead of retaining them", index)
		}
	}
}

func TestPositionerRejectsTransactionsOutsideTheRemainingChain(t *testing.T) {
	events := testChain(4)
	tests := []struct {
		name  string
		first gitlog.Event
		next  gitlog.Event
	}{
		{name: "repeated", first: events[2], next: events[2]},
		{name: "out of order", first: events[3], next: events[1]},
		{name: "unknown", first: events[1], next: gitlog.Event{ID: testObjectID(99)}},
		{
			name:  "same object, different kind",
			first: events[1],
			next:  gitlog.Event{ID: events[2].ID, Kind: gitlog.MergeEvent},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			positioner := NewPositioner(events)
			if _, err := positioner.Position(gitlog.Transaction{Event: tt.first}); err != nil {
				t.Fatalf("Position(first) error = %v", err)
			}

			_, err := positioner.Position(gitlog.Transaction{Event: tt.next})

			var unmatched *UnmatchedTransactionError
			if !errors.As(err, &unmatched) || unmatched.Event != tt.next.ID {
				t.Fatalf("Position(next) error = %v, want UnmatchedTransactionError for %s", err, tt.next.ID)
			}
			if !errors.Is(err, ErrUnmatchedTransaction) {
				t.Errorf("Position(next) error = %v, want ErrUnmatchedTransaction", err)
			}
		})
	}
}

// TestPositionerMatchesIngestion positions the transactions ingestion visits in
// a real repository whose chain holds a root event and an event with no
// eligible paths, both of which ingestion skips.
func TestPositionerMatchesIngestion(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "positions")
	runGit(t, "init", "--quiet", "--object-format=sha1", "--initial-branch=main", repository)
	commitFiles(t, repository, map[string]string{"root.txt": "root"})
	commitFiles(t, repository, map[string]string{"a/one.go": "1", "b/one.go": "1"})
	commitFiles(t, repository, map[string]string{"vendor/x/one.go": "1"})
	commitFiles(t, repository, map[string]string{"a/two.go": "2"})
	tip := runGit(t, "-C", repository, "rev-parse", "HEAD")

	repo, err := gitlog.Open(t.Context(), repository)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	events, err := repo.FirstParentEvents(t.Context(), tip)
	if err != nil {
		t.Fatalf("FirstParentEvents() error = %v", err)
	}

	positioner := NewPositioner(events)
	var got []Transaction
	if _, err := repo.EachTransaction(t.Context(), events, func(transaction gitlog.Transaction) error {
		positioned, err := positioner.Position(transaction)
		if err != nil {
			return err
		}
		got = append(got, positioned)
		return nil
	}); err != nil {
		t.Fatalf("EachTransaction() error = %v", err)
	}

	want := []struct {
		position   int
		components []string
	}{
		{position: 2, components: []string{"a", "b"}},
		{position: 4, components: []string{"a"}},
	}
	if len(got) != len(want) {
		t.Fatalf("positioned %d transactions, want %d: %+v", len(got), len(want), got)
	}
	for index, transaction := range got {
		if transaction.Position != want[index].position ||
			!slices.Equal(transaction.Components, want[index].components) ||
			transaction.Event != events[want[index].position-1] {
			t.Errorf("transaction %d = position %d components %q, want position %d components %q",
				index, transaction.Position, transaction.Components,
				want[index].position, want[index].components)
		}
	}
}

func commitFiles(t *testing.T, repository string, files map[string]string) {
	t.Helper()

	for path, content := range files {
		blob := filepath.Join(t.TempDir(), "blob")
		if err := os.WriteFile(blob, []byte(content), 0o600); err != nil {
			t.Fatalf("writing blob: %v", err)
		}
		id := runGit(t, "-C", repository, "hash-object", "-w", "--", blob)
		runGit(t, "-C", repository, "update-index", "--add", "--cacheinfo",
			fmt.Sprintf("100644,%s,%s", id, path))
	}
	runGit(t, "-C", repository, "-c", "user.name=Tacita Test",
		"-c", "user.email=tacita@example.invalid",
		"commit", "--quiet", "-m", "event")
}

func runGit(t *testing.T, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Env = []string{
		"HOME=" + t.TempDir(),
		"PATH=" + os.Getenv("PATH"),
		"LC_ALL=C",
		"TZ=UTC",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
