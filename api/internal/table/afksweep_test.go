package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func lastActionAtFor(table *hand.Table, id string) int64 {
	for _, p := range table.PlayersForActor() {
		if p.ID == id {
			return p.LastActionAt
		}
	}
	return -1
}

// TestGenuineActionRefreshesLastActionButTurnTimeoutFoldDoesNot guards the
// one non-obvious invariant the AFK sweep depends on: a real inbound Act
// bumps LastActionAt, but the server's own turn-timeout auto-fold (driven by
// the same applyActAndCommit path) must not — otherwise a truly-silent
// player's own auto-folds would keep resetting their staleness clock forever.
func TestGenuineActionRefreshesLastActionButTurnTimeoutFoldDoesNot(t *testing.T) {
	table := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	defer actor.afkSweepTimer.Stop()
	actor.cached = table
	actor.SetEquityEnabledForActor(false)

	current := table.CurrentPlayerIDForActor()
	before := timeNowFunc().UnixMilli()
	if err := actor.handleAct(context.Background(), ActCmd{PlayerID: current, ActionID: "a1", Action: betting.ActionCall, Amount: 0, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("handleAct: %v", err)
	}
	if got := lastActionAtFor(table, current); got < before {
		t.Fatalf("expected genuine action to refresh LastActionAt to >= %d, got %d", before, got)
	}

	other := table.CurrentPlayerIDForActor()
	staleBefore := lastActionAtFor(table, other)
	if err := actor.handleTurnTimeout(context.Background(), turnTimeoutCmd{PlayerID: other, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("handleTurnTimeout: %v", err)
	}
	if staleAfter := lastActionAtFor(table, other); staleAfter != staleBefore {
		t.Fatalf("turn-timeout auto-fold must not refresh LastActionAt: before=%d after=%d", staleBefore, staleAfter)
	}
}

// TestAFKSweepKicksOnlyStalePlayers checks the sweep only removes players
// whose LastActionAt is older than kickGrace, leaving recently-active seats
// alone — and that it runs independent of whose turn it currently is (there
// is no live hand here at all).
func TestAFKSweepKicksOnlyStalePlayers(t *testing.T) {
	now := time.Now()
	table := hand.NewTable([]*hand.Player{
		{ID: "fresh", Stack: 1000, LastActionAt: now.UnixMilli()},
		{ID: "stale", Stack: 1000, LastActionAt: now.Add(-10 * time.Minute).UnixMilli()},
	}, 10, 20)

	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	defer actor.afkSweepTimer.Stop()
	actor.cached = table
	actor.kickGrace = 5 * time.Minute

	if err := actor.handleAFKSweep(context.Background(), afkSweepCmd{Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("handleAFKSweep: %v", err)
	}
	defer actor.afkSweepTimer.Stop() // handleAFKSweep re-armed a new one

	seated := map[string]bool{}
	for _, p := range table.PlayersForActor() {
		seated[p.ID] = true
	}
	if seated["stale"] {
		t.Fatal("expected the stale player (10 min silent, 5 min grace) to be removed")
	}
	if !seated["fresh"] {
		t.Fatal("fresh player should not have been removed")
	}
}
