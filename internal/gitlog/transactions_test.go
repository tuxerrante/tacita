package gitlog

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestProjectTransaction(t *testing.T) {
	tests := []struct {
		name       string
		paths      []string
		components []string
	}{
		{
			name:       "root file",
			paths:      []string{"README.md"},
			components: []string{"."},
		},
		{
			name:       "shared parent",
			paths:      []string{"a/one.go", "a/two.go", "b/three.go"},
			components: []string{"a", "b"},
		},
		{
			name: "first occurrence order",
			paths: []string{
				"z/first.go",
				"a/first.go",
				"z/second.go",
				"m/first.go",
			},
			components: []string{"z", "a", "m"},
		},
		{
			name: "raw bytes and lexical segments",
			paths: []string{
				"a/../odd\xff.go",
				"A/file.go",
				"a/file.go",
				"./root.go",
				"root.go",
			},
			components: []string{"a/..", "A", "a", "."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := EventPaths{Paths: tt.paths}

			transaction, ok := projectTransaction(paths, maxEventComponents)

			if !ok {
				t.Fatal("projectTransaction() excluded an event within the component limit")
			}
			if !slices.Equal(transaction.Paths, tt.paths) {
				t.Errorf("Paths = %q, want %q", transaction.Paths, tt.paths)
			}
			if !slices.Equal(transaction.Components, tt.components) {
				t.Errorf("Components = %q, want %q", transaction.Components, tt.components)
			}
			if len(tt.paths) > 0 && &transaction.Paths[0] != &tt.paths[0] {
				t.Error("Paths were copied instead of transferring the visitor-owned slice")
			}
		})
	}
}

func TestProjectTransactionAppliesComponentLimitToDistinctComponents(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		ok    bool
	}{
		{
			name:  "exactly at limit",
			paths: []string{"a/one", "a/two", "b/one"},
			ok:    true,
		},
		{
			name:  "one over limit",
			paths: []string{"a/one", "b/one", "c/one"},
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transaction, ok := projectTransaction(EventPaths{Paths: tt.paths}, 2)

			if ok != tt.ok {
				t.Fatalf("projectTransaction() included = %t, want %t", ok, tt.ok)
			}
			if !ok &&
				(transaction.Event != (Event{}) ||
					transaction.Paths != nil ||
					transaction.Components != nil) {
				t.Errorf("excluded transaction = %v, want zero value", transaction)
			}
		})
	}
}

func TestProjectTransactionAppliesFrozenComponentLimit(t *testing.T) {
	paths := make([]string, maxEventComponents)
	for i := range maxEventComponents {
		paths[i] = fmt.Sprintf("%03d/file.go", i)
	}

	transaction, ok := projectTransaction(EventPaths{Paths: paths}, maxEventComponents)
	if !ok {
		t.Fatalf("projectTransaction() excluded %d components at the limit", maxEventComponents)
	}
	if len(transaction.Components) != maxEventComponents {
		t.Errorf("Components = %d, want %d", len(transaction.Components), maxEventComponents)
	}

	paths = append(paths, "overflow/file.go")
	if transaction, ok := projectTransaction(
		EventPaths{Paths: paths},
		maxEventComponents,
	); ok {
		t.Errorf("projectTransaction() included %d components one over the limit: %v",
			len(paths), transaction)
	}
}

func TestEachTransactionMergesDiagnosticsAndContinuesAfterComponentLimit(t *testing.T) {
	repository := newChangeTestRepository(t)
	commitTree(t, repository, map[string]string{
		"a/one.go":        "1",
		"b/two.go":        "1",
		"vendor/lib/x.go": "1",
	})
	commitTree(t, repository, map[string]string{
		"c/one.go":   "1",
		"d/two.go":   "1",
		"e/three.go": "1",
	})
	commitTree(t, repository, map[string]string{"after/file.go": "1"})

	opened := openTestRepository(t, repository)
	events, err := opened.FirstParentEvents(t.Context(), headCommit(t, repository))
	if err != nil {
		t.Fatalf("FirstParentEvents() error = %v", err)
	}

	var visited []Transaction
	diagnostics, err := opened.eachTransaction(
		t.Context(),
		events,
		func(transaction Transaction) error {
			visited = append(visited, transaction)
			return nil
		},
		maxStreamOutput,
		maxEventPaths,
		2,
	)
	if err != nil {
		t.Fatalf("eachTransaction() error = %v", err)
	}

	if len(visited) != 2 {
		t.Fatalf("visited %d transactions, want 2: %v", len(visited), visited)
	}
	if got := visited[0].Components; !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("first Components = %q, want [a b]", got)
	}
	if got := visited[1].Components; !slices.Equal(got, []string{"after"}) {
		t.Errorf("second Components = %q, want [after]", got)
	}
	if got := visited[0].Paths; !slices.Equal(got, []string{"a/one.go", "b/two.go"}) {
		t.Errorf("retained first Paths = %q, want [a/one.go b/two.go]", got)
	}

	assertDiagnostics(t, diagnostics, map[Exclusion]uint64{
		{Scope: EventScope, Reason: RootEventReason}:           1,
		{Scope: EventScope, Reason: EventComponentLimitReason}: 1,
		{Scope: PathScope, Reason: VendorPathReason}:           1,
	})
}

func TestEachTransactionReportsVisitorFailureWithoutPartialDiagnostics(t *testing.T) {
	repository := newChangeTestRepository(t)
	commitTree(t, repository, map[string]string{"a/file.go": "1"})

	opened := openTestRepository(t, repository)
	events, err := opened.FirstParentEvents(t.Context(), headCommit(t, repository))
	if err != nil {
		t.Fatalf("FirstParentEvents() error = %v", err)
	}

	refused := errors.New("refused")
	diagnostics, err := opened.EachTransaction(
		t.Context(),
		events,
		func(Transaction) error { return refused },
	)

	if !errors.Is(err, refused) {
		t.Fatalf("EachTransaction() error = %v, want %v", err, refused)
	}
	if diagnostics != nil {
		t.Errorf("EachTransaction() diagnostics = %v, want nil after an error", diagnostics)
	}
}

func FuzzComponentForPath(f *testing.F) {
	for _, path := range []string{
		"README.md",
		"dir/file.go",
		"a/b/c.go",
		"a/../odd.go",
		"raw/\xff\n.go",
	} {
		f.Add(path)
	}

	f.Fuzz(func(t *testing.T, path string) {
		if path == "" || strings.IndexByte(path, 0) >= 0 {
			t.Skip()
		}

		separator := strings.LastIndexByte(path, pathSeparator)
		want := rootComponent
		if separator >= 0 {
			want = path[:separator]
		}

		if got := componentForPath(path); got != want {
			t.Fatalf("componentForPath(%q) = %q, want %q", path, got, want)
		}

		transaction, ok := projectTransaction(
			EventPaths{Paths: []string{path, path}},
			1,
		)
		if !ok {
			t.Fatal("duplicate path exceeded a one-component limit")
		}
		if !slices.Equal(transaction.Components, []string{want}) {
			t.Fatalf("Components = %q, want [%q]", transaction.Components, want)
		}
	})
}
