package handhook

import (
	"context"
	"time"
)

// EventSchemaVersion is the version of the Event contract below. Bump it when
// a field is removed or its meaning changes; adding a field is backwards
// compatible and does not. A consumer that cares reads Event.SchemaVersion —
// which is why the field is carried on the event itself and not implied by
// the Go type.
const EventSchemaVersion = 1

// Event is the versioned post-hand contract handed to every registered
// Consumer (issue #315).
//
// It is deliberately NOT hand.HandOutcome: that is the engine's internal
// representation of a finished hand (hole cards, fairness proofs, per-seat
// engine state) and coupling a derived consumer to it means every engine
// refactor is a breaking change for bookkeeping that only ever needed "who
// played, who won, at which table". Anything a consumer needs that isn't here
// is a deliberate decision to add — not an accident of what the engine happens
// to expose.
type Event struct {
	SchemaVersion int
	TableID       string
	HandID        string
	// CurrencyMode is the room's mode ("sandbox"/"real"), resolved once by
	// the pipeline — consumers must never re-read the room to get it.
	CurrencyMode string
	Participants []string
	Winners      []string
	// Names is the persisted seat-name map (player_id -> display name), so a
	// consumer can denormalize a name without a profile read.
	Names       map[string]string
	CompletedAt time.Time
}

// Consumer is one derived writer reacting to a completed hand. Name is the
// step name used for logging and metrics dimensions, so it must come from a
// small fixed set (a hand or table id here would be one custom metric per
// table — see internal/metrics).
type Consumer struct {
	Name string
	Run  func(ctx context.Context, ev Event) error
}

// Dispatch runs every consumer in order and reports each failure to onErr,
// never aborting the rest: by the time an Event exists the hand is already
// Complete and broadcast, and the fleet-wide Claim for it has been taken, so
// stopping at the first failure would silently drop every later consumer's
// bookkeeping for a hand nobody will retry. onErr is also given a nil error
// after a successful run so the caller can time the step; it may be nil.
//
// This adds no exclusivity of its own — Claim is still the only thing that
// decides which instance runs a hand's hooks at all, and it stays a
// synchronous, in-process call list, not an event bus.
func Dispatch(ctx context.Context, ev Event, consumers []Consumer, observe func(name string, started time.Time, err error)) {
	for _, consumer := range consumers {
		if consumer.Run == nil {
			continue
		}
		started := time.Now()
		err := consumer.Run(ctx, ev)
		if observe != nil {
			observe(consumer.Name, started, err)
		}
	}
}
