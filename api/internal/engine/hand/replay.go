package hand

import (
	"fmt"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
)

// ReplayedAction is the minimal shape Replay needs from a durable log entry —
// deliberately not tablestore.ActionLogEntry itself, so this package never
// imports a persistence package (OVERVIEW.md's "pure logic" boundary for
// internal/engine/*).
type ReplayedAction struct {
	PlayerID string
	Action   betting.Action
	Amount   int64
}

// Replay re-applies entries (assumed already in seq order) starting from a
// hand this table hasn't started yet. handID is accepted for the caller's
// bookkeeping symmetry with StartHand's own hand-id generation but is not
// itself validated here — tablemanager is the layer that knows whether
// entries actually belong to the hand currently in progress.
func (t *Table) Replay(handID string, entries []ReplayedAction) error {
	if err := t.StartHand(); err != nil {
		return fmt.Errorf("hand: replay: %w", err)
	}
	for _, e := range entries {
		if err := t.Act(e.PlayerID, e.Action, e.Amount); err != nil {
			return fmt.Errorf("hand: replay action %+v: %w", e, err)
		}
	}
	return nil
}
