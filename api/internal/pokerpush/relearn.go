package pokerpush

import (
	"context"
	"log/slog"
	"time"

	"gopkg.aoctech.app/api-commons/pushsvc"
)

// SlotMover is what the Relearner needs of pushsvc.ReminderStore.
type SlotMover interface {
	MoveSlot(ctx context.Context, sub, kind string, from, to int) (bool, error)
}

// StartReader reads the start times of a player's newest sessions (SessionLog).
type StartReader interface {
	RecentStarts(ctx context.Context, playerID string, n int) ([]time.Time, error)
}

// Relearner moves a "come play" reminder to the hour the player usually starts a session, after each push it sent.
type Relearner struct {
	movers SlotMover
	starts StartReader
}

// NewRelearner returns a Relearner.
func NewRelearner(m SlotMover, s StartReader) *Relearner { return &Relearner{movers: m, starts: s} }

// OnSent is pushsvc.Tick's OnSent hook. Best effort: a failure keeps the old hour. The move is conditional on the reminder
// still being on and still where the tick found it (MoveSlot), so a player who switched it off, or moved it, meanwhile wins
// and a background move can never resurrect an opt-out.
func (r *Relearner) OnSent(ctx context.Context, rem pushsvc.Reminder) {
	if rem.Kind != KindPlay {
		return
	}
	starts, err := r.starts.RecentStarts(ctx, rem.Sub, sessionsToLearnFrom)
	if err != nil {
		slog.WarnContext(ctx, "push tick: sessions not read", "error", err)
		return
	}
	slot := TypicalSlot(starts)
	if slot == rem.Slot {
		return
	}
	if _, err := r.movers.MoveSlot(ctx, rem.Sub, rem.Kind, rem.Slot, slot); err != nil {
		slog.WarnContext(ctx, "push tick: slot not moved", "error", err)
	}
}
