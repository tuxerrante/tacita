package gitlog

import (
	"context"
	"strings"
)

const (
	// maxEventComponents is the frozen cap on distinct components in one event.
	maxEventComponents = 100

	rootComponent = "."
)

// Transaction is one eligible integration event with its normalized path
// evidence and distinct projected components.
//
// Paths remain in Git's order. Components are ordered by their first path
// occurrence. Both slices are owned by the receiver and may be retained; they
// are never reused or mutated after the visitor returns.
type Transaction struct {
	Event      Event
	Paths      []string
	Components []string
}

// EachTransaction streams normalized component transactions in chain order.
//
// Component projection is lexical and never accesses the filesystem. An event
// exceeding the frozen component limit is excluded whole and counted in the
// returned diagnostics. Diagnostics are complete only when the error is nil.
func (r *Repository) EachTransaction(
	ctx context.Context,
	events []Event,
	visit func(Transaction) error,
) (Diagnostics, error) {
	return r.eachTransaction(
		ctx,
		events,
		visit,
		maxStreamOutput,
		maxEventPaths,
		maxEventComponents,
	)
}

// eachTransaction carries the limits explicitly so tests can reach the frozen
// exclusion boundaries with small repositories.
func (r *Repository) eachTransaction(
	ctx context.Context,
	events []Event,
	visit func(Transaction) error,
	outputLimit int,
	pathLimit int,
	componentLimit int,
) (Diagnostics, error) {
	var componentLimitExclusions uint64

	diagnostics, err := r.eachEventChange(
		ctx,
		events,
		func(paths EventPaths) error {
			transaction, ok := projectTransaction(paths, componentLimit)
			if !ok {
				componentLimitExclusions++
				return nil
			}

			return visit(transaction)
		},
		outputLimit,
		pathLimit,
	)
	if err != nil {
		return nil, err
	}

	diagnostics.record(EventScope, EventComponentLimitReason, componentLimitExclusions)

	return diagnostics, nil
}

func projectTransaction(paths EventPaths, componentLimit int) (Transaction, bool) {
	components := make([]string, 0, min(len(paths.Paths), componentLimit))
	seen := make(map[string]struct{}, min(len(paths.Paths), componentLimit))

	for _, path := range paths.Paths {
		component := componentForPath(path)
		if _, duplicate := seen[component]; duplicate {
			continue
		}
		if len(components) == componentLimit {
			return Transaction{}, false
		}

		seen[component] = struct{}{}
		components = append(components, component)
	}

	return Transaction{
		Event:      paths.Event,
		Paths:      paths.Paths,
		Components: components,
	}, true
}

func componentForPath(path string) string {
	separator := strings.LastIndexByte(path, pathSeparator)
	if separator < 0 {
		return rootComponent
	}

	return path[:separator]
}
