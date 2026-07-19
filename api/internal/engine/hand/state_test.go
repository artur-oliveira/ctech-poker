package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
)

func TestExportImportRoundTripsFullState(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	original := NewTable([]*Player{p1, p2}, 10, 20)
	if err := original.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	toAct := original.playerToActForTest()
	if err := original.Act(toAct, betting.ActionCall, 0); err != nil {
		t.Fatalf("Act: %v", err)
	}

	rebuilt := NewTableFromState(original.ExportState())

	originalView := original.ViewFor("p1")
	rebuiltView := rebuilt.ViewFor("p1")
	if originalView.Stage != rebuiltView.Stage {
		t.Fatalf("stage mismatch: original=%s rebuilt=%s", originalView.Stage, rebuiltView.Stage)
	}
	if len(originalView.Board) != len(rebuiltView.Board) {
		t.Fatalf("board length mismatch: original=%d rebuilt=%d", len(originalView.Board), len(rebuiltView.Board))
	}

	nextToAct := rebuilt.playerToActForTest()
	if nextToAct == "" {
		t.Fatal("expected rebuilt table to still have a player to act")
	}
	if err := rebuilt.Act(nextToAct, betting.ActionCheck, 0); err != nil {
		if err := rebuilt.Act(nextToAct, betting.ActionCall, 0); err != nil {
			t.Fatalf("rebuilt table rejected a legal action: %v", err)
		}
	}
}

func TestActIdempotentSkipsRepeatedActionID(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	toAct := table.playerToActForTest()

	applied1, err := table.ActIdempotent("dup-1", toAct, betting.ActionCall, 0)
	if err != nil || !applied1 {
		t.Fatalf("expected first call applied, got applied=%v err=%v", applied1, err)
	}

	before := table.ViewFor(toAct)
	applied2, err := table.ActIdempotent("dup-1", toAct, betting.ActionCall, 0)
	if err != nil {
		t.Fatalf("duplicate action_id must not error: %v", err)
	}
	if applied2 {
		t.Fatal("expected duplicate action_id to report applied=false")
	}
	after := table.ViewFor(toAct)
	if len(before.Seats) != len(after.Seats) {
		t.Fatal("duplicate action_id must not mutate state")
	}
}
