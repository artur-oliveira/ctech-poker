package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
)

func TestViewForHidesOtherHoleCards(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	view := table.ViewFor("p1")
	var seatP1, seatP2 SeatView
	for _, s := range view.Seats {
		if s.PlayerID == "p1" {
			seatP1 = s
		}
		if s.PlayerID == "p2" {
			seatP2 = s
		}
	}
	if len(seatP1.HoleCards) != 2 {
		t.Fatalf("expected viewer to see their own 2 hole cards, got %d", len(seatP1.HoleCards))
	}
	if len(seatP2.HoleCards) != 0 {
		t.Fatalf("expected viewer NOT to see opponent hole cards, got %v", seatP2.HoleCards)
	}
}

func TestViewForRevealsAllHandsAtShowdownForNonFolded(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	// Heads-up preflop: dealer(p1) posts SB and acts first. Call then check
	// through every street to reach Complete without any fold.
	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}
	view := table.ViewFor("p1")
	for _, s := range view.Seats {
		if len(s.HoleCards) != 2 {
			t.Fatalf("expected every non-folded player's hand revealed at Complete, seat %s had %d cards", s.PlayerID, len(s.HoleCards))
		}
	}
}
