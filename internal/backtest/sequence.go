package backtest

import (
	"strings"

	"github.com/tuxerrante/tacita/internal/gitlog"
)

// Transaction is one eligible transaction at its one-based position in an
// event ordering. It retains components but not paths, so a full history fits
// in memory for replay over a second ordering. Component strings never share
// memory with a path.
type Transaction struct {
	Position   int
	Event      gitlog.Event
	Components []string
}

// Positioner assigns first-parent event indices to eligible transactions as
// [gitlog.Repository.EachTransaction] visits them.
//
// Ingestion visits eligible events in chain order and skips excluded ones, so
// a transaction's index is found by advancing over the chain rather than by a
// lookup table. A transaction whose event is missing from the remaining chain,
// repeated, or out of order fails instead of receiving another event's index.
//
// Ingestion slices each component from one of its paths, so keeping that
// string would keep the whole path allocation alive. The Positioner therefore
// copies each distinct component once and hands every transaction the shared
// copy, which bounds retained component bytes by the distinct components
// rather than by the paths they came from.
type Positioner struct {
	events     []gitlog.Event
	next       int
	components map[string]string
}

// NewPositioner returns a Positioner over the complete chain that
// [gitlog.Repository.FirstParentEvents] returned.
func NewPositioner(events []gitlog.Event) *Positioner {
	return &Positioner{events: events, components: make(map[string]string)}
}

// Position returns transaction at its one-based chain index with detached,
// shared component strings. Ingestion transfers ownership of the component
// slice, which is reused for the detached strings.
func (p *Positioner) Position(transaction gitlog.Transaction) (Transaction, error) {
	for p.next < len(p.events) && p.events[p.next] != transaction.Event {
		p.next++
	}
	if p.next == len(p.events) {
		return Transaction{}, &UnmatchedTransactionError{Event: transaction.Event.ID}
	}

	p.next++
	for index, component := range transaction.Components {
		detached, ok := p.components[component]
		if !ok {
			detached = strings.Clone(component)
			p.components[detached] = detached
		}
		transaction.Components[index] = detached
	}

	return Transaction{
		Position:   p.next,
		Event:      transaction.Event,
		Components: transaction.Components,
	}, nil
}
