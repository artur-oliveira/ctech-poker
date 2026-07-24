package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func TestViewForLeavesEquityNil(t *testing.T) {
	table := NewTable([]*Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	for _, seat := range table.ViewFor("p1").Seats {
		if seat.Equity != nil {
			t.Fatal("engine snapshot populated orchestration-owned equity")
		}
	}
}

func TestHandOutcomeUsesNetOfRakeWinnerAndExcludesRefunds(t *testing.T) {
	p1 := &Player{ID: "winner", State: Active, Contributed: 100, HoleCards: [2]deck.Card{{Rank: deck.Ace, Suit: deck.Clubs}, {Rank: deck.Ace, Suit: deck.Diamonds}}}
	p2 := &Player{ID: "folded", State: Folded, Contributed: 100}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	table.handOrder = []*Player{p1, p2}
	table.board = []deck.Card{{Rank: deck.Two, Suit: deck.Clubs}, {Rank: deck.Three, Suit: deck.Diamonds}, {Rank: deck.Four, Suit: deck.Hearts}}
	table.ConfigureRake("real")
	table.runShowdown()

	outcome := table.LastOutcomeForActor()
	if outcome == nil || len(outcome.Winners) != 1 || outcome.Winners[0] != "winner" || !outcome.WonWithoutShowdown {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	if table.RakeCollected() != 5 || table.Payouts()["winner"] != 195 {
		t.Fatalf("outcome was not based on net pot: rake=%d payouts=%+v", table.RakeCollected(), table.Payouts())
	}
}

func TestHandOutcomeBookkeepingSurvivesRecovery(t *testing.T) {
	table := NewTable(nil, 10, 20)
	table.lastOutcome = &HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1", "p2"}}
	table.wasEverAllIn = map[string]bool{"p1": true}
	rebuilt := NewTableFromState(table.ExportState())
	if rebuilt.lastOutcome == nil || rebuilt.lastOutcome.Winners[0] != "p1" || !rebuilt.wasEverAllIn["p1"] {
		t.Fatalf("outcome state not restored: %+v / %+v", rebuilt.lastOutcome, rebuilt.wasEverAllIn)
	}
}
