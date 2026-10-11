package backtest

import (
	"bytes"
	"crypto/sha256"
	"slices"

	"github.com/tuxerrante/tacita/internal/gitlog"
)

// shuffleDomain separates the hash-shuffled control's keys from every other
// hash Tacita computes.
const shuffleDomain = "tacita-shuffle-v1"

// shuffleKey is SHA-256 over the domain, the frozen repository object ID, and
// the event object ID, joined by single NUL bytes with no trailing NUL.
func shuffleKey(repository gitlog.ObjectID, event gitlog.ObjectID) [sha256.Size]byte {
	hash := sha256.New()
	hash.Write([]byte(shuffleDomain))
	hash.Write([]byte{0})
	hash.Write(repository[:])
	hash.Write([]byte{0})
	hash.Write(event[:])

	var key [sha256.Size]byte
	hash.Sum(key[:0])

	return key
}

// ShuffledTransactions repositions transactions at their events' places in
// the frozen hash-shuffled event order, sorted by their new positions.
//
// Every event of the chain is ordered, root and excluded events included, so
// the partitions keep the boundaries they have over the chronological order.
// repository is the frozen repository commit. Each transaction must name an
// event of events.
func ShuffledTransactions(
	repository gitlog.ObjectID,
	events []gitlog.Event,
	transactions []Transaction,
) ([]Transaction, error) {
	type keyed struct {
		key   [sha256.Size]byte
		event gitlog.ObjectID
	}
	order := make([]keyed, len(events))
	for index, event := range events {
		order[index] = keyed{key: shuffleKey(repository, event.ID), event: event.ID}
	}
	slices.SortFunc(order, func(left, right keyed) int {
		if c := bytes.Compare(left.key[:], right.key[:]); c != 0 {
			return c
		}
		return bytes.Compare(left.event[:], right.event[:])
	})

	positions := make(map[gitlog.ObjectID]int, len(order))
	for index, entry := range order {
		positions[entry.event] = index + 1
	}

	shuffled := make([]Transaction, len(transactions))
	for index, transaction := range transactions {
		position, ok := positions[transaction.Event.ID]
		if !ok {
			return nil, &UnmatchedTransactionError{Event: transaction.Event.ID}
		}
		transaction.Position = position
		shuffled[index] = transaction
	}
	slices.SortFunc(shuffled, func(left, right Transaction) int {
		return left.Position - right.Position
	})

	return shuffled, nil
}
