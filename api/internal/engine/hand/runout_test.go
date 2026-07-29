package hand

import (
	"encoding/hex"
	"fmt"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

// TestAllInPreflopDealsFlopImmediatelyThenAwaitsPacedRunout covers the
// 3-missing-streets case: an all-in accepted at PreFlop deals the flop
// synchronously (same as advanceStage always has), then stops — pacing the
// turn and river is table.Actor's job (see IsAwaitingRunoutForActor), one
// street at a time via AdvanceRunoutStreetForActor.
func TestAllInPreflopDealsFlopImmediatelyThenAwaitsPacedRunout(t *testing.T) {
	table := NewTable([]*Player{{ID: "p1", Stack: 30, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	table.dealerSeat = 0
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}

	// Heads-up: p1 (dealer) posts the small blind and acts first preflop.
	// Shove the rest of their stack; p2 calls and remains Active with plenty
	// of chips left -- nobody else can bet against them, so this triggers
	// the runout path.
	first := table.currentPlayerToAct()
	if err := table.Act(first, betting.ActionRaise, 30); err != nil {
		t.Fatalf("p1 shove: %v", err)
	}
	second := table.currentPlayerToAct()
	if err := table.Act(second, betting.ActionCall, 0); err != nil {
		t.Fatalf("p2 call: %v", err)
	}

	if table.Stage() != Flop {
		t.Fatalf("expected the flop dealt immediately, got stage %v", table.Stage())
	}
	if len(table.board) != 3 {
		t.Fatalf("expected 3 board cards after the immediate flop deal, got %d", len(table.board))
	}
	if !table.IsAwaitingRunoutForActor() {
		t.Fatal("expected a paced runout to still be pending (turn + river missing)")
	}

	table.AdvanceRunoutStreetForActor()
	if table.Stage() != Turn || len(table.board) != 4 {
		t.Fatalf("expected the turn dealt next, got stage %v with %d board cards", table.Stage(), len(table.board))
	}
	if !table.IsAwaitingRunoutForActor() {
		t.Fatal("expected the river still pending")
	}

	table.AdvanceRunoutStreetForActor()
	if table.Stage() != Complete || len(table.board) != 5 {
		t.Fatalf("expected the river dealt and showdown run, got stage %v with %d board cards", table.Stage(), len(table.board))
	}
	if table.IsAwaitingRunoutForActor() {
		t.Fatal("expected no runout pending once showdown has run")
	}
	if table.LastOutcomeForActor() == nil {
		t.Fatal("expected showdown to have recorded an outcome")
	}
}

// TestAllInWithOnlyRiverMissingSkipsPacing covers the single-missing-street
// case: nothing to pace, the last street reveals and showdown runs in the
// same call, same as normal (non-all-in) play.
func TestAllInWithOnlyRiverMissingSkipsPacing(t *testing.T) {
	table := NewTable([]*Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	table.dealerSeat = 0
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}

	// Preflop: dealer (SB) calls up to the big blind, BB checks behind.
	preflopFirst := table.currentPlayerToAct()
	if err := table.Act(preflopFirst, betting.ActionCall, 0); err != nil {
		t.Fatalf("preflop call: %v", err)
	}
	preflopSecond := table.currentPlayerToAct()
	if err := table.Act(preflopSecond, betting.ActionCheck, 0); err != nil {
		t.Fatalf("preflop check: %v", err)
	}
	if table.Stage() != Flop {
		t.Fatalf("expected Flop after preflop, got %v", table.Stage())
	}

	// Flop: both check through, no chips committed.
	flopFirst := table.currentPlayerToAct()
	if err := table.Act(flopFirst, betting.ActionCheck, 0); err != nil {
		t.Fatalf("flop first check: %v", err)
	}
	flopSecond := table.currentPlayerToAct()
	if err := table.Act(flopSecond, betting.ActionCheck, 0); err != nil {
		t.Fatalf("flop second check: %v", err)
	}
	if table.Stage() != Turn {
		t.Fatalf("expected Turn after flop, got %v", table.Stage())
	}

	// Turn: first-to-act shoves their entire remaining stack, the other
	// calls -- only the river is left to deal.
	turnFirst := table.currentPlayerToAct()
	shoveAmount := table.playerByID(turnFirst).Stack
	if err := table.Act(turnFirst, betting.ActionBet, shoveAmount); err != nil {
		t.Fatalf("turn shove: %v", err)
	}
	turnSecond := table.currentPlayerToAct()
	if err := table.Act(turnSecond, betting.ActionCall, 0); err != nil {
		t.Fatalf("turn call: %v", err)
	}

	if table.Stage() != Complete {
		t.Fatalf("expected the river to reveal and showdown to run immediately (only one street was missing), got stage %v", table.Stage())
	}
	if len(table.board) != 5 {
		t.Fatalf("expected all 5 board cards dealt, got %d", len(table.board))
	}
	if table.LastOutcomeForActor() == nil {
		t.Fatal("expected showdown to have recorded an outcome")
	}
}

func TestRunItTwiceRequiresRoomGateAndUnanimousPlayers(t *testing.T) {
	for _, tc := range []struct {
		name        string
		roomEnabled bool
		secondOptIn bool
		wantTwice   bool
	}{
		{name: "unanimous", roomEnabled: true, secondOptIn: true, wantTwice: true},
		{name: "one player declines", roomEnabled: true, secondOptIn: false},
		{name: "room disabled", roomEnabled: false, secondOptIn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			table := NewTable([]*Player{
				{ID: "p1", Stack: 30, Ready: true, RunItTwice: true},
				{ID: "p2", Stack: 1000, Ready: true, RunItTwice: tc.secondOptIn},
			}, 10, 20)
			table.ConfigureRunItTwice(tc.roomEnabled)
			table.ConfigureRake("sandbox")
			table.dealerSeat, table.dealerDrawn = 0, true
			if err := table.StartHand(); err != nil {
				t.Fatal(err)
			}
			first := table.currentPlayerToAct()
			if err := table.Act(first, betting.ActionRaise, 30); err != nil {
				t.Fatal(err)
			}
			if err := table.Act(table.currentPlayerToAct(), betting.ActionCall, 0); err != nil {
				t.Fatal(err)
			}
			recoveredInPhaseTwo := false
			observedPhaseTwoFlop := false
			for table.IsAwaitingRunoutForActor() {
				if tc.wantTwice && table.runoutPhase == 2 && !recoveredInPhaseTwo {
					table = NewTableFromState(table.ExportState())
					recoveredInPhaseTwo = true
				}
				if tc.wantTwice && table.runoutPhase == 2 && len(table.boardTwo) == 3 {
					_, equityBoard, ok := table.HoleAndBoardForActor("p1")
					if !ok || len(equityBoard) != 3 {
						t.Fatalf("phase-two equity must use its current flop, got %v (ok=%v)", boardCodes(equityBoard), ok)
					}
					observedPhaseTwoFlop = true
				}
				table.AdvanceRunoutStreetForActor()
			}
			outcome := table.LastOutcomeForActor()
			if outcome == nil {
				t.Fatal("expected completed outcome")
			}
			if got := len(outcome.BoardTwo) == 5; got != tc.wantTwice {
				t.Fatalf("second board presence = %v, want %v (board=%v boardTwo=%v)", got, tc.wantTwice, outcome.Board, outcome.BoardTwo)
			}
			if tc.wantTwice {
				if !recoveredInPhaseTwo {
					t.Fatal("expected to exercise recovery during runout phase two")
				}
				if !observedPhaseTwoFlop {
					t.Fatal("expected to observe the second runout's flop")
				}
				if len(outcome.Board) != 5 || len(table.boardTwo) != 5 || table.nextCard > 52 {
					t.Fatalf("invalid double runout: board=%v suffix=%v nextCard=%d", outcome.Board, table.boardTwo, table.nextCard)
				}
				paid := int64(0)
				for _, amount := range outcome.Payouts {
					paid += amount
				}
				contributed := int64(0)
				for _, amount := range outcome.Contributions {
					contributed += amount
				}
				if paid+table.RakeCollected() != contributed {
					t.Fatalf("payout conservation failed: paid=%d rake=%d contributed=%d", paid, table.RakeCollected(), contributed)
				}
				if table.RakeCollected() != 1 {
					t.Fatalf("RIT must charge the 60-chip layer's rake once, got %d", table.RakeCollected())
				}
				var firstRunout, secondRunout int64
				for _, result := range outcome.PotResults {
					if !result.Refund && result.Runout != 1 && result.Runout != 2 {
						t.Fatalf("contested RIT result missing runout: %+v", result)
					}
					if result.Runout == 1 {
						firstRunout += result.PayoutAmount
					}
					if result.Runout == 2 {
						secondRunout += result.PayoutAmount
					}
				}
				if firstRunout != 30 || secondRunout != 29 {
					t.Fatalf("odd half-chip must go to runout one: first=%d second=%d", firstRunout, secondRunout)
				}
			}
		})
	}
}

func TestRunItTwiceKeepsTheFlopAsASingleCommonPrefix(t *testing.T) {
	table := NewTable([]*Player{
		{ID: "p1", Stack: 1000, Ready: true, RunItTwice: true},
		{ID: "p2", Stack: 1000, Ready: true, RunItTwice: true},
	}, 10, 20)
	table.ConfigureRunItTwice(true)
	table.dealerSeat, table.dealerDrawn = 0, true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	if err := table.Act(table.currentPlayerToAct(), betting.ActionCall, 0); err != nil {
		t.Fatal(err)
	}
	if err := table.Act(table.currentPlayerToAct(), betting.ActionCheck, 0); err != nil {
		t.Fatal(err)
	}
	common := append([]deck.Card(nil), table.board...)
	first := table.currentPlayerToAct()
	if err := table.Act(first, betting.ActionBet, table.playerByID(first).Stack); err != nil {
		t.Fatal(err)
	}
	if err := table.Act(table.currentPlayerToAct(), betting.ActionCall, 0); err != nil {
		t.Fatal(err)
	}
	for table.IsAwaitingRunoutForActor() {
		table.AdvanceRunoutStreetForActor()
	}
	outcome := table.LastOutcomeForActor()
	if outcome == nil || len(outcome.BoardTwo) != 5 || table.boardSplitAt != 3 || len(table.boardTwo) != 2 {
		t.Fatalf("unexpected split runout: outcome=%+v split=%d suffix=%v", outcome, table.boardSplitAt, table.boardTwo)
	}
	for i := range common {
		if outcome.Board[i] != cardCode(common[i]) || outcome.BoardTwo[i] != cardCode(common[i]) {
			t.Fatalf("common flop was redealt: common=%v board1=%v board2=%v", boardCodes(common), outcome.Board, outcome.BoardTwo)
		}
	}
	if outcome.Board[3] == outcome.BoardTwo[3] && outcome.Board[4] == outcome.BoardTwo[4] {
		t.Fatalf("both divergent suffixes unexpectedly match: %v / %v", outcome.Board, outcome.BoardTwo)
	}
}

func TestRunItTwiceNineWayPreflopFitsInTheCommittedDeck(t *testing.T) {
	players := make([]*Player, 9)
	for i := range players {
		players[i] = &Player{ID: fmt.Sprintf("p%d", i+1), Stack: 1000, Ready: true, RunItTwice: true}
	}
	table := NewTable(players, 10, 20)
	table.ConfigureRunItTwice(true)
	table.dealerSeat, table.dealerDrawn = 0, true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	for _, player := range table.handOrder {
		player.State = AllIn
		table.wasEverAllIn[player.ID] = true
	}
	table.round = nil
	table.advanceStage()
	for table.IsAwaitingRunoutForActor() {
		table.AdvanceRunoutStreetForActor()
	}
	if table.nextCard != 34 || table.nextCard > len(table.shuffle.Cards) {
		t.Fatalf("nine-way double runout consumed %d deck positions, want 34 of %d", table.nextCard, len(table.shuffle.Cards))
	}
}

func TestRunItTwicePartialFairnessProofCoversBothBoardsWithoutLeakingSeed(t *testing.T) {
	table := NewTable([]*Player{
		{ID: "p1", Stack: 1000, Ready: true, RunItTwice: true},
		{ID: "p2", Stack: 1000, Ready: true, RunItTwice: true},
		{ID: "p3", Stack: 1000, Ready: true},
	}, 10, 20)
	table.ConfigureRunItTwice(true)
	table.dealerSeat, table.dealerDrawn = 0, true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	table.handOrder[0].State = AllIn
	table.handOrder[1].State = AllIn
	table.handOrder[2].State = Folded
	table.round = nil
	table.advanceStage()
	for table.IsAwaitingRunoutForActor() {
		table.AdvanceRunoutStreetForActor()
	}
	view := table.ViewFor("p1")
	if view.ShuffleServerSeedHex != "" {
		t.Fatal("a folded hidden hand must keep the shuffle seed private during RIT")
	}
	if len(view.RevealedCardSalts)+len(view.UnrevealedCardHashes) != 52 {
		t.Fatalf("partial proof covers %d positions, want 52", len(view.RevealedCardSalts)+len(view.UnrevealedCardHashes))
	}
	for index := len(table.handOrder) * 2; index < table.nextCard; index++ {
		if _, ok := view.RevealedCardSalts[index]; !ok {
			t.Fatalf("dealt runout position %d is absent from the revealed proof", index)
		}
	}
	revealed := make(map[int]struct {
		Card deck.Card
		Salt [32]byte
	}, len(view.RevealedCardSalts))
	for index, reveal := range view.RevealedCardSalts {
		var salt [32]byte
		decoded, err := hex.DecodeString(reveal.SaltHex)
		if err != nil {
			t.Fatal(err)
		}
		copy(salt[:], decoded)
		revealed[index] = struct {
			Card deck.Card
			Salt [32]byte
		}{Card: table.shuffle.Cards[index], Salt: salt}
	}
	unrevealed := make(map[int][32]byte, len(view.UnrevealedCardHashes))
	for index, hashHex := range view.UnrevealedCardHashes {
		decoded, err := hex.DecodeString(hashHex)
		if err != nil {
			t.Fatal(err)
		}
		var hash [32]byte
		copy(hash[:], decoded)
		unrevealed[index] = hash
	}
	root := deck.RootCommitHash(table.shuffle.ServerSeed, table.shuffle.Cards)
	if !deck.VerifyPartial(root, revealed, unrevealed) {
		t.Fatal("RIT partial proof did not verify against the original root commit")
	}
}
