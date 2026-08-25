package gitlog

import (
	"context"
	"strings"
)

const (
	// maxEventComponents is the frozen cap on distinct components in one event.
	maxEventComponents = 100
	// maxPathIdentities is the frozen cap on distinct eligible paths in one run.
	maxPathIdentities = 2_000_000
	// maxComponentIdentities is the frozen cap on distinct eligible components
	// in one run.
	maxComponentIdentities = 50_000

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
// events must be the exact sequence returned by [Repository.FirstParentEvents].
// Every diff-tree boundary is matched against it, so a skipped, repeated, or
// reordered event fails instead of receiving another event's paths.
//
// Component projection is lexical and never accesses the filesystem. An event
// exceeding the frozen component limit is excluded whole and counted in the
// returned diagnostics. Distinct path and component identities are bounded
// across eligible transactions before the visitor receives them. Diagnostics
// are complete only when the error is nil.
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
		maxPathIdentities,
		maxComponentIdentities,
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
	pathIdentityLimit int,
	componentIdentityLimit int,
) (Diagnostics, error) {
	var componentLimitExclusions uint64
	budget := newIdentityBudget(pathIdentityLimit, componentIdentityLimit)

	diagnostics, err := r.eachEventChange(
		ctx,
		events,
		func(paths EventPaths) error {
			transaction, ok := projectTransaction(paths, componentLimit)
			if !ok {
				componentLimitExclusions++
				return nil
			}

			if err := budget.observe(transaction); err != nil {
				return err
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

type identityBudget struct {
	paths          map[string]struct{}
	components     map[string]struct{}
	pathLimit      int
	componentLimit int
}

func newIdentityBudget(pathLimit int, componentLimit int) *identityBudget {
	return &identityBudget{
		paths:          make(map[string]struct{}),
		components:     make(map[string]struct{}),
		pathLimit:      pathLimit,
		componentLimit: componentLimit,
	}
}

func (b *identityBudget) observe(transaction Transaction) error {
	for _, path := range transaction.Paths {
		if _, exists := b.paths[path]; exists {
			continue
		}

		observed := len(b.paths) + 1
		if observed > b.pathLimit {
			return &PathIdentityLimitError{Observed: observed, Limit: b.pathLimit}
		}
		b.paths[path] = struct{}{}
	}

	for _, component := range transaction.Components {
		if _, exists := b.components[component]; exists {
			continue
		}

		observed := len(b.components) + 1
		if observed > b.componentLimit {
			return &ComponentIdentityLimitError{
				Observed: observed,
				Limit:    b.componentLimit,
			}
		}
		b.components[component] = struct{}{}
	}

	return nil
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
