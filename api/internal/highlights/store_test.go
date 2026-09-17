package highlights

import (
	"reflect"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestRevealedHandsOf_OnlyCopiesRevealedHands(t *testing.T) {
	outcome := hand.HandOutcome{
		Participants: []string{"p1", "p2"},
		PlayerHands: map[string]hand.PlayerHandInfo{
			"p1": {HoleCards: [2]string{"Ah", "Kd"}, Revealed: true},
			"p2": {HoleCards: [2]string{"2c", "7s"}, Revealed: false},
		},
	}
	names := map[string]string{"p1": "Alice", "p2": "Bob"}

	got := revealedHandsOf(outcome, names)

	if len(got) != 1 {
		t.Fatalf("expected 1 revealed hand, got %d: %+v", len(got), got)
	}
	if got[0].PlayerID != "p1" || got[0].Name != "Alice" {
		t.Fatalf("unexpected revealed hand: %+v", got[0])
	}
	if !reflect.DeepEqual(got[0].HoleCards, []string{"Ah", "Kd"}) {
		t.Fatalf("unexpected hole cards: %+v", got[0].HoleCards)
	}
	for _, entry := range got {
		if entry.PlayerID == "p2" {
			t.Fatalf("folded participant p2's hole cards leaked into highlight: %+v", got)
		}
	}
}

func TestRevealedHandsOf_NoneRevealed(t *testing.T) {
	outcome := hand.HandOutcome{
		Participants: []string{"p1"},
		PlayerHands: map[string]hand.PlayerHandInfo{
			"p1": {HoleCards: [2]string{"Ah", "Kd"}, Revealed: false},
		},
	}
	if got := revealedHandsOf(outcome, nil); len(got) != 0 {
		t.Fatalf("expected no revealed hands, got %+v", got)
	}
}

// The reported bug: an all-in everybody folds to has no showdown, so
// revealedHandsOf returns nothing and the card rendered with no name at all
// — even though the outcome names the winner outright.
func TestWinnersOf_NamesTheWinnerWithoutAShowdown(t *testing.T) {
	outcome := hand.HandOutcome{
		Winners:            []string{"p1"},
		WonWithoutShowdown: true,
		Participants:       []string{"p1", "p2"},
		Payouts:            map[string]int64{"p1": 150875},
		PlayerHands: map[string]hand.PlayerHandInfo{
			"p1": {HoleCards: [2]string{"Ah", "Qd"}, Revealed: false},
			"p2": {HoleCards: [2]string{"2c", "7s"}, Revealed: false},
		},
	}
	names := map[string]string{"p1": "Artur 1234", "p2": "Dexther"}

	got := winnersOf(outcome, names)

	if len(got) != 1 {
		t.Fatalf("expected 1 winner, got %d: %+v", len(got), got)
	}
	if got[0].PlayerID != "p1" || got[0].Name != "Artur 1234" || got[0].Payout != 150875 {
		t.Fatalf("unexpected winner: %+v", got[0])
	}
	if revealed := revealedHandsOf(outcome, names); len(revealed) != 0 {
		t.Fatalf("no showdown happened, so nothing may be revealed: %+v", revealed)
	}
}

// A split pot names everyone who was paid, ordered by payout then id so the
// caption is stable between reads.
func TestWinnersOf_SplitPotIsOrderedAndComplete(t *testing.T) {
	outcome := hand.HandOutcome{
		Winners:      []string{"p2", "p1"},
		Participants: []string{"p1", "p2"},
		Payouts:      map[string]int64{"p1": 500, "p2": 1500},
	}

	got := winnersOf(outcome, map[string]string{"p1": "Alice", "p2": "Bob"})

	if len(got) != 2 {
		t.Fatalf("expected 2 winners, got %+v", got)
	}
	if got[0].PlayerID != "p2" || got[0].Payout != 1500 {
		t.Fatalf("biggest payout must come first, got %+v", got)
	}
	if got[1].PlayerID != "p1" || got[1].Payout != 500 {
		t.Fatalf("unexpected runner-up: %+v", got[1])
	}
}

// A winner with no payout entry is still named — the pot layer bookkeeping is
// PotResults' job, and dropping the name would reproduce the original bug.
func TestWinnersOf_WinnerWithoutAPayoutEntryIsStillNamed(t *testing.T) {
	outcome := hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1"}}

	got := winnersOf(outcome, nil)

	if len(got) != 1 || got[0].PlayerID != "p1" || got[0].Payout != 0 {
		t.Fatalf("unexpected winners: %+v", got)
	}
}
