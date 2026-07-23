package table

import (
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// TestBroadcastAddsRevealGraceOnlyOnFirstArmAfterStageTransition drives a
// heads-up hand from PreFlop into Flop and asserts the first action deadline
// on the new street includes the +1.5s grace, while a same-street follow-up
// action (still on Flop) does not.
func TestBroadcastAddsRevealGraceOnlyOnFirstArmAfterStageTransition(t *testing.T) {
	table := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}

	seen := map[string]hand.Snapshot{}
	actor := New("table-1", nil, true, func(id string, snapshot hand.Snapshot) {
		seen[id] = snapshot
	})
	actor.cached = table
	actor.broadcastAll() // settles lastBroadcastStage at PreFlop, no grace expected there

	// Drive PreFlop to completion. dealerSeat is randomized (not pinned, since
	// this test lives in package table and can't reach hand.Table's
	// unexported dealer fields), so whichever of SB/BB acts first isn't known
	// ahead of time here — Act silently downgrades an ActionCall to a Check
	// when nothing is owed, so passing ActionCall for both preflop actions
	// works regardless of which one the current player actually is.
	sb := table.CurrentPlayerIDForActor()
	if err := table.Act(sb, betting.ActionCall, 0); err != nil {
		t.Fatalf("first preflop action: %v", err)
	}
	bb := table.CurrentPlayerIDForActor()
	if err := table.Act(bb, betting.ActionCall, 0); err != nil {
		t.Fatalf("second preflop action: %v", err)
	}
	if table.Stage() != hand.Flop {
		t.Fatalf("expected Flop after preflop completes, got %v", table.Stage())
	}

	before := time.Now()
	actor.broadcastAll()
	firstToAct := table.CurrentPlayerIDForActor()
	deadline := time.UnixMilli(seen[firstToAct].ActionDeadlineUnixMs)
	wantMin := before.Add(actor.turnTimeout + RevealGrace - 300*time.Millisecond)
	wantMax := before.Add(actor.turnTimeout + RevealGrace + 300*time.Millisecond)
	if deadline.Before(wantMin) || deadline.After(wantMax) {
		t.Fatalf("expected deadline ~turnTimeout+%v after the flop reveal, got %v (turnTimeout=%v)", RevealGrace, deadline, actor.turnTimeout)
	}

	// Same-street follow-up: firstToAct checks, the second Flop actor's
	// deadline must NOT carry the grace again.
	if err := table.Act(firstToAct, betting.ActionCheck, 0); err != nil {
		t.Fatalf("first flop action: %v", err)
	}
	if table.Stage() != hand.Flop {
		t.Fatalf("expected to still be on Flop awaiting the second action, got %v", table.Stage())
	}
	before2 := time.Now()
	actor.broadcastAll()
	secondToAct := table.CurrentPlayerIDForActor()
	deadline2 := time.UnixMilli(seen[secondToAct].ActionDeadlineUnixMs)
	wantMin2 := before2.Add(actor.turnTimeout - 300*time.Millisecond)
	wantMax2 := before2.Add(actor.turnTimeout + 300*time.Millisecond)
	if deadline2.Before(wantMin2) || deadline2.After(wantMax2) {
		t.Fatalf("expected no grace on the same-street follow-up, deadline %v not within the turnTimeout-only window", deadline2)
	}
}
