package table

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// processPendingExitAutoFolds has to fold out *everyone* the action reaches
// who cannot be waited on, not just the first of them: the sweep only runs on
// a commit, so a player it leaves on the clock is left there until the turn
// timer eventually fires — which is the whole 45s (15s + a full 30s time bank)
// the player never asked for and cannot spend, because a pending exit takes
// their action buttons away. Reported live on 2026-09-21 ("Luvas" asked to
// leave and the table waited out his time bank instead of folding him).
func TestPendingExitSweepFoldsEveryUnwaitableSeatInOneCommit(t *testing.T) {
	game := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
		{ID: "p3", Stack: 1000, Ready: true},
		{ID: "p4", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}
	current := game.CurrentPlayerIDForActor()
	leaving := make([]string, 0, 3)
	for _, playerID := range []string{"p1", "p2", "p3", "p4"} {
		if playerID == current {
			continue
		}
		if err := game.RequestExit(playerID); err != nil {
			t.Fatalf("request exit %s: %v", playerID, err)
		}
		leaving = append(leaving, playerID)
	}

	a := New("pending-exit-sweep", nil, true, func(string, hand.Snapshot) {})
	a.SetCachedForTest(game)
	a.handID = "hand-1"
	a.version = 1
	a.gameplayVersion = 1

	// Any commit is what drives the sweep; the player on the clock calling is
	// the ordinary one. Everyone behind them has asked to leave, so the hand
	// must resolve to an uncontested win right here.
	if err := a.handleAct(context.Background(), ActCmd{
		PlayerID: current, ActionID: "call-1", Action: betting.ActionCall,
		Amount: game.ProspectiveCallAmountForActor(current),
	}); err != nil {
		t.Fatalf("act: %v", err)
	}

	if stage := a.cached.Stage(); stage != hand.Complete {
		t.Fatalf("hand stage=%v, want complete: the sweep stopped after the first auto-fold and left %q on the clock",
			stage, a.cached.CurrentPlayerIDForActor())
	}
	view := a.cached.ViewFor("")
	for _, playerID := range leaving {
		for _, seat := range view.Seats {
			if seat.PlayerID == playerID && seat.State != "folded" {
				t.Fatalf("seat %s state=%q, want folded", playerID, seat.State)
			}
		}
	}
}
