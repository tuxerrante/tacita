package backtest

import "github.com/tuxerrante/tacita/internal/gitlog"

// Transaction is one eligible transaction at its one-based position in an
// event ordering. It retains components but not paths, so a full history fits
// in memory for replay over a second ordering.
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
type Positioner struct {
	events []gitlog.Event
	next   int
}

// NewPositioner returns a Positioner over the complete chain that
// [gitlog.Repository.FirstParentEvents] returned.
func NewPositioner(events []gitlog.Event) *Positioner {
	return &Positioner{events: events}
}

// Position returns transaction at its one-based chain index. The component
// slice is retained, not copied; ingestion transfers its ownership.
func (p *Positioner) Position(transaction gitlog.Transaction) (Transaction, error) {
	for p.next < len(p.events) && p.events[p.next] != transaction.Event {
		p.next++
	}
	if p.next == len(p.events) {
		return Transaction{}, &UnmatchedTransactionError{Event: transaction.Event.ID}
	}

	p.next++

	return Transaction{
		Position:   p.next,
		Event:      transaction.Event,
		Components: transaction.Components,
	}, nil
}
