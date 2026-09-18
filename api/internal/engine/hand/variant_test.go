package hand

import (
	"fmt"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/sidepots"
)

func fullPotLayer(playerIDs ...string) sidepots.PotLayer {
	return sidepots.PotLayer{Amount: 100, Eligible: playerIDs}
}

// #296 acceptance criterion: a ShortDeck table plays a real hand end to end
// through the exact same StartHand/Act pipeline standard tables use, and its
// dealt cards come from the 36-card short deck (no rank Two-Five).
func TestShortDeckTableDealsFromTheReducedDeck(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTableWithVariant([]*Player{p1, p2}, 10, 20, deck.ShortDeck)
	if table.Variant() != deck.ShortDeck {
		t.Fatalf("Variant()=%v, want ShortDeck", table.Variant())
	}
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if table.shortShuffle == nil {
		t.Fatal("expected shortShuffle to be populated for a ShortDeck table")
	}
	if table.shuffle != nil {
		t.Fatal("a ShortDeck table must never populate the standard 52-card shuffle field")
	}
	for _, c := range table.shortShuffle.Cards {
		if c.Rank < deck.Six {
			t.Fatalf("short deck must never contain rank %v (below Six)", c.Rank)
		}
	}

	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}
	outcome := table.LastOutcomeForActor()
	if outcome == nil {
		t.Fatal("expected a hand outcome from a completed ShortDeck hand")
	}
	// Every dealt card in the outcome (board + hole cards) must come from the
	// short deck's rank range.
	for _, code := range outcome.Board {
		if r := rankFromCode(t, code); r < deck.Six {
			t.Fatalf("board card %q has a rank below Six, impossible in short-deck", code)
		}
	}
	for id, info := range outcome.PlayerHands {
		for _, code := range info.HoleCards {
			if r := rankFromCode(t, code); r < deck.Six {
				t.Fatalf("player %s hole card %q has a rank below Six, impossible in short-deck", id, code)
			}
		}
	}
}

// #296 acceptance criterion: NewTable (used by every standard/real-money
// path) is unaffected — it still defaults to deck.Standard and behaves
// exactly as before variant existed.
func TestNewTableDefaultsToStandardVariant(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if table.Variant() != deck.Standard {
		t.Fatalf("NewTable's default Variant()=%v, want Standard", table.Variant())
	}
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if table.shuffle == nil {
		t.Fatal("a Standard table must populate the standard 52-card shuffle field")
	}
	if table.shortShuffle != nil {
		t.Fatal("a Standard table must never populate shortShuffle")
	}
	if len(table.shuffle.Cards) != 52 {
		t.Fatalf("standard shuffle has %d cards, want 52", len(table.shuffle.Cards))
	}
}

// #296 acceptance criterion: short-deck hand-strength ordering (flush beats
// full house) actually drives the winner at showdown, through the table's
// own best7/categoryOf dispatch — not just the shortdeck package in
// isolation.
func TestShortDeckTableRanksFlushAboveFullHouseAtShowdown(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, State: Active}
	p2 := &Player{ID: "p2", Stack: 1000, State: Active}
	table := NewTableWithVariant([]*Player{p1, p2}, 10, 20, deck.ShortDeck)
	table.handOrder = []*Player{p1, p2}

	// p1: flush. p2: full house. Ranks are all Six+ (valid short-deck ranks).
	p1.HoleCards = [2]deck.Card{{Rank: deck.Six, Suit: deck.Clubs}, {Rank: deck.Eight, Suit: deck.Clubs}}
	p2.HoleCards = [2]deck.Card{{Rank: deck.King, Suit: deck.Diamonds}, {Rank: deck.King, Suit: deck.Hearts}}
	table.board = []deck.Card{
		{Rank: deck.Ten, Suit: deck.Clubs}, {Rank: deck.Queen, Suit: deck.Clubs}, {Rank: deck.Ace, Suit: deck.Clubs},
		{Rank: deck.Queen, Suit: deck.Diamonds}, {Rank: deck.Queen, Suit: deck.Hearts},
	}

	winners, _, _ := table.evaluateLayer(fullPotLayer(p1.ID, p2.ID), table.board)
	if len(winners) != 1 || winners[0] != p1.ID {
		t.Fatalf("expected p1's flush to win over p2's full house in short-deck, winners=%v", winners)
	}
}

// The same board/holes on a Standard table must flip: full house wins.
func TestStandardTableRanksFullHouseAboveFlushAtShowdown(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, State: Active}
	p2 := &Player{ID: "p2", Stack: 1000, State: Active}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	table.handOrder = []*Player{p1, p2}

	p1.HoleCards = [2]deck.Card{{Rank: deck.Six, Suit: deck.Clubs}, {Rank: deck.Eight, Suit: deck.Clubs}}
	p2.HoleCards = [2]deck.Card{{Rank: deck.King, Suit: deck.Diamonds}, {Rank: deck.King, Suit: deck.Hearts}}
	table.board = []deck.Card{
		{Rank: deck.Ten, Suit: deck.Clubs}, {Rank: deck.Queen, Suit: deck.Clubs}, {Rank: deck.Ace, Suit: deck.Clubs},
		{Rank: deck.Queen, Suit: deck.Diamonds}, {Rank: deck.Queen, Suit: deck.Hearts},
	}

	winners, _, _ := table.evaluateLayer(fullPotLayer(p1.ID, p2.ID), table.board)
	if len(winners) != 1 || winners[0] != p2.ID {
		t.Fatalf("expected p2's full house to win over p1's flush on a Standard table, winners=%v", winners)
	}
}

// #296: a full 9-handed short-deck table (9x2 hole cards + 3 burns + 5 board
// = 26 of the 36-card deck) must play a complete hand — through the same
// StartHand/Act pipeline as the 2-player test above — without exhausting the
// reduced deck, and every dealt card (board + every hand's hole cards) must
// have rank >= Six.
func TestShortDeckNineHandedTablePlaysAFullHandWithoutExhaustingTheDeck(t *testing.T) {
	players := make([]*Player, 9)
	for i := range players {
		players[i] = &Player{ID: fmt.Sprintf("p%d", i+1), Stack: 1000, Ready: true}
	}
	table := NewTableWithVariant(players, 10, 20, deck.ShortDeck)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}
	if table.nextCard > 36 {
		t.Fatalf("nine-handed short-deck hand consumed %d deck positions, want <= 36", table.nextCard)
	}
	outcome := table.LastOutcomeForActor()
	if outcome == nil {
		t.Fatal("expected a hand outcome from a completed 9-handed ShortDeck hand")
	}
	for _, code := range outcome.Board {
		if r := rankFromCode(t, code); r < deck.Six {
			t.Fatalf("board card %q has a rank below Six, impossible in short-deck", code)
		}
	}
	if len(outcome.PlayerHands) == 0 {
		t.Fatal("expected at least one participant's hole cards in the outcome")
	}
	for id, info := range outcome.PlayerHands {
		for _, code := range info.HoleCards {
			if r := rankFromCode(t, code); r < deck.Six {
				t.Fatalf("player %s hole card %q has a rank below Six, impossible in short-deck", id, code)
			}
		}
	}
}

// #296: a 9-way all-in run-it-twice short-deck hand (9x2 hole cards + 1 burn +
// 5 board + 1 burn + 5 boardTwo = 34 of 36 cards, manually verified against
// the standard-deck TestRunItTwiceNineWayPreflopFitsInTheCommittedDeck's
// shape) still fits the reduced deck and every dealt card obeys the
// short-deck rank floor. RunItTwiceEnabled is a per-room setting, so this is
// tested separately from the plain 9-handed case above.
func TestShortDeckNineWayRunItTwiceFitsInTheReducedDeck(t *testing.T) {
	players := make([]*Player, 9)
	for i := range players {
		players[i] = &Player{ID: fmt.Sprintf("p%d", i+1), Stack: 1000, Ready: true, RunItTwice: true}
	}
	table := NewTableWithVariant(players, 10, 20, deck.ShortDeck)
	table.ConfigureRunItTwice(true)
	table.dealerSeat, table.dealerDrawn = 0, true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	if table.shortShuffle == nil || table.shuffle != nil {
		t.Fatal("expected only the short-deck shuffle to be populated")
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
	if table.nextCard != 34 || table.nextCard > len(table.shortShuffle.Cards) {
		t.Fatalf("nine-way short-deck double runout consumed %d deck positions, want 34 of %d",
			table.nextCard, len(table.shortShuffle.Cards))
	}
	outcome := table.LastOutcomeForActor()
	if outcome == nil {
		t.Fatal("expected a hand outcome from the completed run-it-twice hand")
	}
	for _, code := range outcome.Board {
		if r := rankFromCode(t, code); r < deck.Six {
			t.Fatalf("board card %q has a rank below Six, impossible in short-deck", code)
		}
	}
	for _, code := range outcome.BoardTwo {
		if r := rankFromCode(t, code); r < deck.Six {
			t.Fatalf("boardTwo card %q has a rank below Six, impossible in short-deck", code)
		}
	}
	for _, c := range table.shortShuffle.Cards {
		if c.Rank < deck.Six {
			t.Fatalf("short deck must never contain rank %v (below Six)", c.Rank)
		}
	}
}

// A short-deck table reloaded from persisted State (a normal occurrence on any
// cross-instance handoff) must keep dealing from the same 36-card shuffle.
func TestShortDeckVariantSurvivesStateRoundTrip(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTableWithVariant([]*Player{p1, p2}, 10, 20, deck.ShortDeck)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	restored := NewTableFromState(table.ExportState())
	if restored.Variant() != deck.ShortDeck || restored.shortShuffle == nil || restored.shuffle != nil {
		t.Fatalf("variant=%v shortShuffle=%v shuffle=%v", restored.Variant(), restored.shortShuffle, restored.shuffle)
	}
	if got, want := restored.dealCard(), table.shortShuffle.Cards[table.nextCard]; got != want {
		t.Fatalf("restored table dealt %v, want %v", got, want)
	}
}

// #296: HandOutcome.Variant (surfaced to the frontend via sessionlog.HandItem
// for hand history) must reflect the table's actual variant, so a client
// re-deriving hand strength from a past hand's raw cards uses the right
// ranking rules.
func TestHandOutcomeCapturesVariant(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}

	standard := NewTable([]*Player{p1, p2}, 10, 20)
	if err := standard.StartHand(); err != nil {
		t.Fatal(err)
	}
	for standard.Stage() != Complete {
		toAct := standard.playerToActForTest()
		if err := standard.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = standard.Act(toAct, betting.ActionCheck, 0)
		}
	}
	if got := standard.LastOutcomeForActor().Variant; got != "" {
		t.Fatalf("standard table outcome Variant = %q, want empty", got)
	}

	p3 := &Player{ID: "p3", Stack: 1000, Ready: true}
	p4 := &Player{ID: "p4", Stack: 1000, Ready: true}
	shortDeck := NewTableWithVariant([]*Player{p3, p4}, 10, 20, deck.ShortDeck)
	if err := shortDeck.StartHand(); err != nil {
		t.Fatal(err)
	}
	for shortDeck.Stage() != Complete {
		toAct := shortDeck.playerToActForTest()
		if err := shortDeck.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = shortDeck.Act(toAct, betting.ActionCheck, 0)
		}
	}
	if got := shortDeck.LastOutcomeForActor().Variant; got != "short_deck" {
		t.Fatalf("short-deck table outcome Variant = %q, want short_deck", got)
	}
}

func rankFromCode(t *testing.T, code string) deck.Rank {
	t.Helper()
	if len(code) < 2 {
		t.Fatalf("malformed card code %q", code)
	}
	switch code[0] {
	case 'T':
		return deck.Ten
	case 'J':
		return deck.Jack
	case 'Q':
		return deck.Queen
	case 'K':
		return deck.King
	case 'A':
		return deck.Ace
	default:
		r := deck.Rank(code[0] - '0')
		if r < deck.Two || r > deck.Nine {
			t.Fatalf("malformed card code %q", code)
		}
		return r
	}
}
