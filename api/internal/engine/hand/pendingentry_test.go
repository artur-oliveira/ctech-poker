package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func pendingEntryTable(t *testing.T) (*Table, *Player) {
	t.Helper()
	table := NewTable([]*Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	joiner := &Player{ID: "p3", Stack: 1000, Ready: true}
	table.AddMidHandJoiner(joiner)
	return table, joiner
}

func TestPendingEntryStaysUndealtWithoutPosting(t *testing.T) {
	table, joiner := pendingEntryTable(t)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	if joiner.State != PendingEntry || joiner.HoleCards != [2]deck.Card{} {
		t.Fatalf("pending entrant was dealt without opting in: %+v", joiner)
	}
}

func TestPendingEntryPostsBigBlindWhenDealtIn(t *testing.T) {
	table, joiner := pendingEntryTable(t)
	table.MarkReadyToPost(joiner.ID)
	// Pin the dealer to the joiner itself (table.players is [p1, p2, joiner]
	// post-AddMidHandJoiner) so blindSeats lands SB/BB on p1/p2 and the
	// joiner sits in the non-blind button seat -- the case this test means
	// to cover, deterministically rather than leaving it to the random
	// initial dealer draw (see TestPendingEntryInBlindSeatOwesOnlyThatBlind
	// for the SB/BB-seat case, which owes a different amount).
	table.dealerSeat = 2
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	if joiner.State == PendingEntry {
		t.Fatal("opted-in entrant was not dealt in")
	}
	if joiner.Contributed < 20 {
		t.Fatalf("entrant must post at least one big blind, contributed %d", joiner.Contributed)
	}
}

// TestPendingEntryInBlindSeatOwesOnlyThatBlind covers a new entrant who lands
// in the SB seat on the very hand they're first dealt into. They already post
// a blind via the normal seat-based post (see StartHand); charging them an
// additional "pay a big blind to enter" fee on top -- as an earlier revision
// did -- left them contributed above t.round.CurrentBet, which
// currentPlayerCanAct/betting.Round.IsComplete can never resolve via a check
// (both require Contributed to exactly equal CurrentBet), hanging the hand
// forever with that seat perpetually "current". This asserts both the
// correct (lower) contribution and that the round actually completes.
func TestPendingEntryInBlindSeatOwesOnlyThatBlind(t *testing.T) {
	table, joiner := pendingEntryTable(t)
	table.MarkReadyToPost(joiner.ID)
	// Dealer at index 1 (p2) makes blindSeats assign SB to index (1+1)%3=2,
	// which is the joiner (table.players is [p1, p2, joiner]).
	table.dealerSeat = 1
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	if joiner.State == PendingEntry {
		t.Fatal("opted-in entrant was not dealt in")
	}
	if joiner.Contributed != 10 {
		t.Fatalf("entrant landing in the small blind seat should owe only that blind (10), got %d", joiner.Contributed)
	}
	if table.round.CurrentBet != 20 {
		t.Fatalf("current bet should stay at one big blind, got %d", table.round.CurrentBet)
	}

	for i := 0; i < 3; i++ {
		current := table.CurrentPlayerIDForActor()
		if current == "" {
			t.Fatalf("round ended early after %d action(s)", i)
		}
		if err := table.Act(current, betting.ActionCall, 0); err != nil {
			t.Fatalf("%s call: %v", current, err)
		}
	}
	if table.stage == PreFlop {
		t.Fatalf("expected the round to close after every seat acted once, hand is still stuck pre-flop (current=%q)", table.CurrentPlayerIDForActor())
	}
}

func TestMarkReadyToPostIsNoOpForNonPendingPlayer(t *testing.T) {
	table := NewTable([]*Player{{ID: "p1", Stack: 1000, Ready: true}}, 10, 20)
	table.MarkReadyToPost("p1")
	if table.readyToPost["p1"] {
		t.Fatal("non-pending player was marked to post")
	}
}

func TestJoiningSamePlayerTwiceIsRejected(t *testing.T) {
	table := NewTable([]*Player{{ID: "p1", Stack: 1000}}, 10, 20)
	if err := table.AddWaitingPlayer(&Player{ID: "p1", Stack: 1000}); err == nil {
		t.Fatal("duplicate waiting player was accepted")
	}
	if err := table.AddMidHandJoiner(&Player{ID: "p1", Stack: 1000}); err == nil {
		t.Fatal("duplicate pending player was accepted")
	}
}
