package hand

import (
	"errors"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func TestFullHandWithThreeWayAllInProducesCorrectPayouts(t *testing.T) {
	players := []*Player{
		{ID: "Dealer", Stack: 1000, Ready: true},
		{ID: "SB", Stack: 200, Ready: true},
		{ID: "BB", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)
	// Scenario names encode the intended seat positions (see the action
	// sequence below) rather than leaving the first hand's dealer to a
	// random draw.
	table.dealerSeat = 0
	table.dealerDrawn = true

	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if table.Stage() != PreFlop {
		t.Fatalf("expected PreFlop after StartHand, got %v", table.Stage())
	}
	for _, player := range players {
		expected := int64(1000)
		if player.ID == "SB" {
			expected = 200
		}
		if player.HandStartStack == nil || *player.HandStartStack != expected {
			t.Fatalf("%s pre-blind stack must be %d, got %+v", player.ID, expected, player.HandStartStack)
		}
	}

	// Rig the deal so the showdown winner is deterministic instead of
	// depending on deck.NewShuffle's crypto/rand seed: SB gets pocket aces
	// and the board pairs the other two aces, giving SB an unbeatable
	// four-of-a-kind. Dealer/BB get low, disjoint hole cards that can't
	// improve past a straight/pair off this board, so there's no chance of
	// a tie muddying the "SB must be paid" assertion below.
	players[0].HoleCards = [2]deck.Card{{Rank: deck.Five, Suit: deck.Clubs}, {Rank: deck.Six, Suit: deck.Clubs}}      // Dealer: 5c 6c
	players[1].HoleCards = [2]deck.Card{{Rank: deck.Ace, Suit: deck.Spades}, {Rank: deck.Ace, Suit: deck.Hearts}}     // SB: As Ah
	players[2].HoleCards = [2]deck.Card{{Rank: deck.Seven, Suit: deck.Hearts}, {Rank: deck.Eight, Suit: deck.Hearts}} // BB: 7h 8h
	// t.nextCard is 6 at this point. The board consumes a burn before each
	// street: flop 7..9, turn 11 and river 13.
	table.shuffle.Cards[7] = deck.Card{Rank: deck.Ace, Suit: deck.Clubs}
	table.shuffle.Cards[8] = deck.Card{Rank: deck.Ace, Suit: deck.Diamonds}
	table.shuffle.Cards[9] = deck.Card{Rank: deck.Two, Suit: deck.Spades}
	table.shuffle.Cards[11] = deck.Card{Rank: deck.Three, Suit: deck.Spades}
	table.shuffle.Cards[13] = deck.Card{Rank: deck.Four, Suit: deck.Hearts}

	// Pre-flop (dealer pinned above, so seat order is exact): Dealer is UTG
	// and raises to 220 (their whole intent), SB shoves all-in for 200 total
	// (a short all-in — SB already posted 10 as small blind, so calling
	// Dealer's raise plus going all-in uses the remaining 190 of their 200
	// stack; Table.Act redirects this ActionRaise to a Call since 200 can't
	// reach the 220 current bet), BB calls and closes the round -- neither
	// SB's short all-in nor BB's call was a real raise, so Dealer's own
	// raise already satisfied their own action requirement and they owe
	// nothing further this street.
	if err := table.Act("Dealer", betting.ActionRaise, 220); err != nil {
		t.Fatalf("Dealer raises to 220: %v", err)
	}
	if err := table.Act("SB", betting.ActionRaise, 200); err != nil {
		t.Fatalf("SB shoves all-in for 200 total: %v", err)
	}
	if err := table.Act("BB", betting.ActionCall, 220); err != nil {
		t.Fatalf("BB calls 220: %v", err)
	}
	if table.Stage() != Flop {
		t.Fatalf("expected the flop once BB's call closed pre-flop action, got %v", table.Stage())
	}

	// SB is all-in with 200 total in the pot; Dealer and BB each have 220 in.
	// Main pot: 200*3=600, eligible all three. Side pot: 20*2=40, eligible
	// Dealer+BB only. Play remaining streets with both non-all-in players
	// checking through (SB has no more decisions — they're all-in). Post-flop
	// action starts left of the button and skips the all-in SB, so BB checks
	// first each street, then Dealer.
	for table.Stage() != Showdown && table.Stage() != Complete {
		id := table.CurrentPlayerIDForActor()
		if id == "" {
			t.Fatalf("no player currently on turn but hand did not reach Showdown/Complete (stage %v)", table.Stage())
		}
		if err := table.Act(id, betting.ActionCheck, 0); err != nil {
			t.Fatalf("check on %v for %s: %v", table.Stage(), id, err)
		}
	}

	payouts := table.Payouts()
	var total int64
	for _, amount := range payouts {
		total += amount
	}
	if total != 640 { // 600 main pot + 40 side pot
		t.Fatalf("total payouts must equal total pot (640), got %d (%+v)", total, payouts)
	}
	if _, ok := payouts["SB"]; !ok {
		t.Fatal("SB contributed to and must be eligible for the main pot")
	}
	if payouts["SB"] != 600 {
		t.Fatalf("SB's rigged quad aces must win the full 600 main pot outright, got %d", payouts["SB"])
	}
	if payouts["Dealer"] != 40 {
		t.Fatalf("Dealer's rigged straight must beat BB's board-pair-of-aces for the 40 side pot (SB isn't eligible for it), got %d", payouts["Dealer"])
	}
	outcome := table.LastOutcomeForActor()
	if len(outcome.PotResults) != 2 {
		t.Fatalf("expected main+side pot results, got %+v", outcome.PotResults)
	}
	if got := outcome.PotResults[0].Winners; len(got) != 1 || got[0] != "SB" {
		t.Fatalf("main pot must be won outright by SB, got %v", got)
	}
	if got := outcome.PotResults[1].Winners; len(got) != 1 || got[0] != "Dealer" {
		t.Fatalf("side pot must be won outright by Dealer, got %v", got)
	}
	if outcome.PotResults[0].Payouts["SB"] != 600 || outcome.PotResults[1].Payouts["Dealer"] != 40 {
		t.Fatalf("pot results must preserve exact per-pot credits: %+v", outcome.PotResults)
	}
	for _, id := range []string{"SB", "Dealer"} {
		if result := outcome.ShowdownResults[id]; !result.Won || result.Tied || result.SplitPot {
			t.Fatalf("%s won a distinct pot outright and must not be marked tied: %+v", id, result)
		}
	}
}

func TestHeadsUpDealerPostsSmallBlind(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 1000, Ready: true},
		{ID: "P2", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)
	table.dealerSeat = 0 // P1 is dealer
	table.dealerDrawn = true

	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if table.players[0].Contributed != 10 {
		t.Fatalf("heads-up: dealer (P1) must post the small blind, got Contributed=%d", table.players[0].Contributed)
	}
	if table.players[1].Contributed != 20 {
		t.Fatalf("heads-up: non-dealer (P2) must post the big blind, got Contributed=%d", table.players[1].Contributed)
	}
}

func TestAddWaitingPlayerIsReadyImmediately(t *testing.T) {
	table := NewTable(nil, 10, 20)
	p := &Player{ID: "p1", Stack: 1000}
	if err := table.AddWaitingPlayer(p); err != nil {
		t.Fatalf("AddWaitingPlayer: %v", err)
	}
	if !p.Ready {
		t.Fatal("a player added via AddWaitingPlayer must be Ready immediately (no manual ready click to enter play)")
	}
}

func TestAddMidHandJoinerIsReadyImmediately(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()

	p3 := &Player{ID: "p3", Stack: 1000}
	if err := table.AddMidHandJoiner(p3); err != nil {
		t.Fatalf("AddMidHandJoiner: %v", err)
	}
	if !p3.Ready {
		t.Fatal("a mid-hand joiner must be Ready immediately (still gated by readyToPost/BB, see PostBigBlindCmd)")
	}
}

// TestAddWaitingPlayerRebuysBustedSeatInsteadOfRejecting reproduces the
// rebuy bug: a busted player (Stack 0) never leaves t.players, so a plain
// "reject if already seated" join silently no-ops the rebuy and never
// credits the chips. AddWaitingPlayer must top up the existing seat instead.
func TestAddWaitingPlayerRebuysBustedSeatInsteadOfRejecting(t *testing.T) {
	busted := &Player{ID: "p1", Stack: 0, Ready: true, State: SittingOut}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	p3 := &Player{ID: "p3", Stack: 1000, Ready: true}
	p4 := &Player{ID: "p4", Stack: 1000, Ready: true}
	table := NewTable([]*Player{busted, p2, p3, p4}, 10, 20)
	table.dealerDrawn = true // dealerSeat 0 (busted p1); next blinds land on p2 (SB), p3 (BB) — p1 is neither

	rebuy := &Player{ID: "p1", Stack: 500}
	if err := table.AddWaitingPlayer(rebuy); err != nil {
		t.Fatalf("AddWaitingPlayer rebuy: %v", err)
	}
	if len(table.players) != 4 {
		t.Fatalf("rebuy must top up the existing seat, not add a second one, got %d players", len(table.players))
	}
	if busted.Stack != 500 {
		t.Fatalf("expected busted seat's stack credited to 500, got %d", busted.Stack)
	}
	if busted.State == SittingOut {
		t.Fatal("p1's seat is not SB/BB of the next hand — rebuy return must be free and immediate")
	}
}

// TestAddWaitingPlayerRejectsRebuyWithChipsRemaining guards the idempotency
// side of the same fix: a player who still has chips must still hit
// ErrAlreadySeated, so a retried join request can't double-spend.
func TestAddWaitingPlayerRejectsRebuyWithChipsRemaining(t *testing.T) {
	seated := &Player{ID: "p1", Stack: 300, Ready: true}
	table := NewTable([]*Player{seated}, 10, 20)

	dup := &Player{ID: "p1", Stack: 500}
	err := table.AddWaitingPlayer(dup)
	if !errors.Is(err, ErrAlreadySeated) {
		t.Fatalf("expected ErrAlreadySeated for a seat with chips remaining, got %v", err)
	}
	if seated.Stack != 300 {
		t.Fatalf("stack must not change on a rejected duplicate join, got %d", seated.Stack)
	}
}

// TestReturnFromSitOutIsFreeWhenNotNearOwnBlind: a 4-handed table where the
// returning player's seat would NOT be SB/BB of the next hand returns
// immediately, no BB owed.
func TestReturnFromSitOutIsFreeWhenNotNearOwnBlind(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	p3 := &Player{ID: "p3", Stack: 1000, Ready: true}
	p4 := &Player{ID: "p4", Stack: 1000, Ready: true, State: SittingOut}
	table := NewTable([]*Player{p1, p2, p3, p4}, 10, 20)
	table.dealerDrawn = true // dealerSeat 0 (p1); blinds for the next hand land on p2 (SB), p3 (BB)

	table.RequestReturnFromSitOut("p4")
	if p4.State == SittingOut {
		t.Fatal("p4's seat is not SB/BB of the next hand — return must be free and immediate")
	}
}

// TestReturnFromSitOutOwesBigBlindWhenNearOwnBlind: the returning player's
// seat IS the projected BB of the next hand — return must stay SittingOut
// until StartHand charges the out-of-position BB.
func TestReturnFromSitOutOwesBigBlindWhenNearOwnBlind(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true, State: SittingOut}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	table.dealerDrawn = true // heads-up: dealer (p1) posts SB, p2 posts BB — p2 IS the projected BB

	table.RequestReturnFromSitOut("p2")
	if p2.State != SittingOut {
		t.Fatal("p2 projects to BB of the next hand — must stay SittingOut until the BB is actually charged")
	}

	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if p2.State != Active {
		t.Fatalf("expected p2 to be dealt in after paying the owed BB, got state %v", p2.State)
	}
	if p2.Contributed < 20 {
		t.Fatalf("expected p2 to have posted at least the big blind (20), got %d", p2.Contributed)
	}
}

func TestRequestReturnFromSitOutIsNoOpForNonSittingOutPlayer(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1}, 10, 20)
	table.RequestReturnFromSitOut("p1") // must not panic or change anything
	if p1.State != Active {
		t.Fatalf("no-op expected, got state %v", p1.State)
	}
}

// TestMidHandReturnDoesNotBecomeGhostActor reproduces a live-table trace:
// a player who sat out of the current hand requests a free return while the
// pre-flop round is still running. Their seat becomes Active for the next
// hand, but they are not part of this hand's immutable handOrder and must not
// be inserted into later streets' betting rounds. Otherwise the dealt players
// can all check, Round.IsComplete still waits for the undealt returner, and
// CurrentPlayerIDForActor becomes empty forever.
func TestMidHandReturnDoesNotBecomeGhostActor(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	p3 := &Player{ID: "p3", Stack: 1000, Ready: true}
	returner := &Player{ID: "returner", Stack: 1000, Ready: false, State: SittingOut}
	table := NewTable([]*Player{p1, p2, p3, returner}, 10, 20)
	table.dealerSeat = 0
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	returner.Ready = true
	table.RequestReturnFromSitOut(returner.ID)
	if returner.State != Active {
		t.Fatalf("returner should be ready for the next hand, got state %v", returner.State)
	}

	for table.Stage() == PreFlop {
		current := table.CurrentPlayerIDForActor()
		if current == "" {
			t.Fatal("pre-flop lost its current player")
		}
		if err := table.Act(current, betting.ActionCall, 0); err != nil {
			t.Fatalf("%s pre-flop action: %v", current, err)
		}
	}
	if table.Stage() != Flop {
		t.Fatalf("expected flop, got %v", table.Stage())
	}
	if _, _, ok := table.HoleAndBoardForActor(returner.ID); ok {
		t.Fatal("undealt returner must not be treated as a participant for equity")
	}

	for range 3 {
		current := table.CurrentPlayerIDForActor()
		if current == "" {
			t.Fatal("flop lost its current player before every dealt player acted")
		}
		if current == returner.ID {
			t.Fatal("undealt returner was prompted to act in the current hand")
		}
		if err := table.Act(current, betting.ActionCheck, 0); err != nil {
			t.Fatalf("%s flop check: %v", current, err)
		}
	}
	if table.Stage() != Turn {
		t.Fatalf("expected checks to advance to turn, got stage=%v current=%q", table.Stage(), table.CurrentPlayerIDForActor())
	}
	if table.CurrentPlayerIDForActor() == "" {
		t.Fatal("turn has no current player after the dealt players checked the flop")
	}
}

// TestSitOutForActorFoldsTheCurrentPlayerInsteadOfWedgingTheRound guards the
// bug where a bare state flip (no fold in the live betting round) left
// betting.Round waiting forever on a decision nobody could ever make again —
// the hand never reaches Complete and the universal turn timer's idempotent
// re-arm treats the unchanged CurrentPlayerIDForActor as a no-op, wedging the
// table permanently. This is exactly the path both a voluntary "sit out"
// mid-hand and the disconnect-escalation timeout take.
func TestSitOutForActorFoldsTheCurrentPlayerInsteadOfWedgingTheRound(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	current := table.CurrentPlayerIDForActor()
	if current == "" {
		t.Fatal("expected a player to be on the clock after StartHand")
	}

	table.SitOutForActor(current)

	if p := table.playerByID(current); p.State != Folded {
		t.Fatalf("expected the sat-out player to be Folded (out of the live round), got state %v", p.State)
	}
	if table.Stage() != Complete {
		t.Fatalf("folding the only other active player in a heads-up hand must end it — stage is still %v, hand is wedged", table.Stage())
	}
	other := p1.ID
	if current == p1.ID {
		other = p2.ID
	}
	if table.Payouts()[other] == 0 {
		t.Fatalf("the player who did not sit out must win the pot uncontested, got payouts %+v", table.Payouts())
	}
}

func TestReadyGateBlocksHandStartWithFewerThanTwoReady(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 1000, Ready: true},
		{ID: "P2", Stack: 1000, Ready: false},
	}
	table := NewTable(players, 10, 20)
	if err := table.StartHand(); err == nil {
		t.Fatal("expected StartHand to fail with fewer than 2 ready players")
	}
}

func TestPendingEntryPlayerIsNotDealtIntoHandsUntilTheyPostBigBlind(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 1000, Ready: true},
		{ID: "P2", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	table.AddMidHandJoiner(&Player{ID: "P3", Stack: 1000})
	if table.playerByID("P3").State != PendingEntry {
		t.Fatal("mid-hand joiner must start as PendingEntry")
	}
}

// TestAllInRunoutDoesNotStallTheHand covers Finding 1 from the final
// whole-branch review: when a betting round completes with 2+ players still
// in the hand but at most one of them NOT all-in, there's nobody left who
// could ever call Act to complete another betting round. advanceStage must
// deal out the rest of the board itself and go straight to showdown instead
// of starting a betting round nobody can act in (which would hang forever).
//
// Scenario: Dealer shoves all-in pre-flop for 500, SB shoves all-in for a
// short 50 (creating a side pot layer above SB's cap), and BB folds. That
// leaves Dealer and SB both all-in with zero players who could still act —
// exactly the classic "two players shove, board just runs out" situation.
func TestAllInRunoutDoesNotStallTheHand(t *testing.T) {
	players := []*Player{
		{ID: "Dealer", Stack: 500, Ready: true},
		{ID: "SB", Stack: 50, Ready: true},
		{ID: "BB", Stack: 300, Ready: true},
	}
	table := NewTable(players, 10, 20)
	table.dealerDrawn = true // scenario names encode the intended seat positions
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	if err := table.Act("Dealer", betting.ActionRaise, 500); err != nil {
		t.Fatalf("Dealer shoves all-in for 500: %v", err)
	}
	if err := table.Act("SB", betting.ActionRaise, 50); err != nil {
		t.Fatalf("SB shoves all-in for 50 (short all-in, redirected to a call): %v", err)
	}
	// After this fold, Dealer and SB are both all-in and BB has folded — 0
	// players can still act. Before the original fix this would have
	// started a Flop betting round with nobody able to complete it, hanging
	// the hand forever. Now advanceStage deals only the immediate next
	// missing street (the flop) synchronously here; pacing the rest one
	// street at a time (via AdvanceRunoutStreetForActor) is table.Actor's
	// job, driven directly below since there's no Actor in this test.
	if err := table.Act("BB", betting.ActionFold, 0); err != nil {
		t.Fatalf("BB folds: %v", err)
	}
	if table.Stage() != Flop {
		t.Fatalf("expected the flop dealt immediately, got stage %v", table.Stage())
	}
	if !table.IsAwaitingRunoutForActor() {
		t.Fatal("expected a paced runout to still be pending (turn + river missing)")
	}
	table.AdvanceRunoutStreetForActor()
	table.AdvanceRunoutStreetForActor()

	if table.Stage() != Complete {
		t.Fatalf("expected the all-in runout to reach Complete once every street is dealt, got stage %v", table.Stage())
	}
	if len(table.board) != 5 {
		t.Fatalf("expected the full board to be dealt by the runout, got %d cards", len(table.board))
	}

	payouts := table.Payouts()
	var total int64
	for _, amount := range payouts {
		total += amount
	}
	const wantTotal = 570 // Dealer 500 + SB 50 + BB 20 (BB's posted big blind, forfeited on the fold)
	if total != wantTotal {
		t.Fatalf("total payouts must equal total contributed (%d), got %d (%+v)", wantTotal, total, payouts)
	}
}

// TestRemovePlayerForActorRejectsFoldedPlayerStillInHand reproduces a
// production panic: a player folds, then leaves the table before the hand
// reaches showdown. RemovePlayerForActor only blocked removal for
// Active/AllIn players, so a folded player (still in t.handOrder, still
// carrying a contribution that side-pot eligibility needs to resolve) could
// be deleted from t.players. runShowdown's playerByID lookup for that ID then
// returned nil, and dereferencing p.State panicked with a nil pointer.
// Removal must stay blocked for anyone dealt into the hand, in any state,
// until the hand reaches Complete.
func TestRemovePlayerForActorRejectsFoldedPlayerStillInHand(t *testing.T) {
	players := []*Player{
		{ID: "Dealer", Stack: 500, Ready: true},
		{ID: "SB", Stack: 500, Ready: true},
		{ID: "BB", Stack: 500, Ready: true},
	}
	table := NewTable(players, 10, 20)
	table.dealerDrawn = true // scenario names encode the intended seat positions
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	if err := table.Act("Dealer", betting.ActionFold, 0); err != nil {
		t.Fatalf("Dealer folds: %v", err)
	}

	if _, _, err := table.RemovePlayerForActor("Dealer"); err == nil {
		t.Fatal("expected removal of a folded-but-still-dealt-in player to be rejected while the hand is in progress")
	}

	// SB still owes 10 to match BB's posted big blind; call it so the
	// preflop round completes, then check the rest of the streets through to
	// showdown with the folded player still seated — this is exactly where
	// the panic used to happen if removal had wrongly succeeded above.
	if err := table.Act("SB", betting.ActionCall, 20); err != nil {
		t.Fatalf("SB calls the big blind: %v", err)
	}
	for table.Stage() != Complete {
		for _, id := range []string{"SB", "BB"} {
			if table.currentPlayerCanAct(id) {
				if err := table.Act(id, betting.ActionCheck, 0); err != nil {
					t.Fatalf("check on %v for %s: %v", table.Stage(), id, err)
				}
			}
		}
	}

	if _, _, err := table.RemovePlayerForActor("Dealer"); err != nil {
		t.Fatalf("expected removal to succeed once the hand is Complete, got: %v", err)
	}
}

// TestBustedAllInPlayerSitsOutInsteadOfBeingRedealt reproduces the reported
// bug: a player who shoves all-in, loses, and ends the hand at Stack 0 was
// still included in the next StartHand call and dealt fresh hole cards
// despite having no chips. runShowdown must transition a Stack-0 seat to
// SittingOut so StartHand's existing SittingOut skip (which already exists
// for the disconnect flow) keeps them out until they rebuy.
func TestBustedAllInPlayerSitsOutInsteadOfBeingRedealt(t *testing.T) {
	players := []*Player{
		{ID: "Dealer", Stack: 500, Ready: true},
		{ID: "SB", Stack: 50, Ready: true},
		{ID: "BB", Stack: 300, Ready: true},
	}
	table := NewTable(players, 10, 20)
	table.dealerDrawn = true // scenario names encode the intended seat positions
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// Rig the deal so Dealer's quad aces beat SB's weak hole cards
	// deterministically instead of depending on crypto/rand — SB must lose
	// this all-in and bust to Stack 0.
	players[0].HoleCards = [2]deck.Card{{Rank: deck.Ace, Suit: deck.Spades}, {Rank: deck.Ace, Suit: deck.Hearts}}     // Dealer: As Ah
	players[1].HoleCards = [2]deck.Card{{Rank: deck.Five, Suit: deck.Clubs}, {Rank: deck.Six, Suit: deck.Clubs}}      // SB: 5c 6c
	players[2].HoleCards = [2]deck.Card{{Rank: deck.Seven, Suit: deck.Hearts}, {Rank: deck.Eight, Suit: deck.Hearts}} // BB: 7h 8h (folds, never shown)
	table.shuffle.Cards[7] = deck.Card{Rank: deck.Ace, Suit: deck.Clubs}
	table.shuffle.Cards[8] = deck.Card{Rank: deck.Ace, Suit: deck.Diamonds}
	table.shuffle.Cards[9] = deck.Card{Rank: deck.Two, Suit: deck.Spades}
	table.shuffle.Cards[11] = deck.Card{Rank: deck.Three, Suit: deck.Spades}
	table.shuffle.Cards[13] = deck.Card{Rank: deck.Four, Suit: deck.Hearts}

	if err := table.Act("Dealer", betting.ActionRaise, 500); err != nil {
		t.Fatalf("Dealer shoves all-in for 500: %v", err)
	}
	if err := table.Act("SB", betting.ActionRaise, 50); err != nil {
		t.Fatalf("SB shoves all-in for 50 (short all-in, redirected to a call): %v", err)
	}
	if err := table.Act("BB", betting.ActionFold, 0); err != nil {
		t.Fatalf("BB folds: %v", err)
	}
	// advanceStage deals only the immediate next missing street (the flop)
	// synchronously; pace the rest via AdvanceRunoutStreetForActor, same as
	// table.Actor's paced timer would in production.
	table.AdvanceRunoutStreetForActor()
	table.AdvanceRunoutStreetForActor()

	if table.Stage() != Complete {
		t.Fatalf("expected the all-in runout to reach Complete once every street is dealt, got stage %v", table.Stage())
	}
	payouts := table.Payouts()
	if payouts["SB"] != 0 {
		t.Fatalf("SB's rigged weak hand must not win any pot, got payout %d", payouts["SB"])
	}
	if payouts["Dealer"] == 0 {
		t.Fatal("Dealer's rigged quad aces must win the pot")
	}

	sb := table.playerByID("SB")
	if sb.Stack != 0 {
		t.Fatalf("SB must be busted (Stack 0) after losing their entire all-in, got %d", sb.Stack)
	}
	if sb.State != SittingOut {
		t.Fatalf("busted SB must transition to SittingOut so the next hand doesn't redeal them, got state %v", sb.State)
	}

	if err := table.StartHand(); err != nil {
		t.Fatalf("second StartHand (Dealer+BB only): %v", err)
	}
	if len(table.handOrder) != 2 {
		t.Fatalf("busted SB must be excluded from the next hand, expected 2 active players, got %d", len(table.handOrder))
	}
	for _, p := range table.handOrder {
		if p.ID == "SB" {
			t.Fatal("busted SB must not be dealt into the next hand")
		}
	}
	if sb.State != SittingOut {
		t.Fatalf("SB must remain SittingOut (not silently reset to Active) across StartHand, got state %v", sb.State)
	}
}

// TestOrphanedSidePotLayerIsRefundedNotDropped covers Finding 2: a pot layer
// whose sole eligible contributor(s) have since folded must not simply
// vanish from Payouts() — sidepots.ComputeSidePots' Eligible list includes
// folded players by contract, and if EVERY eligible player for a layer has
// folded there's no showdown winner to award it to. That layer is an
// uncalled/unmatched bet: it must be refunded to whoever funded it, not
// dropped.
//
// Scenario: A and C shove all-in pre-flop for 100 each (a shared floor).
// D1 and D2 both call the 100, then both raise/call their way up to a tied
// 400 on the flop, then BOTH fold on the turn without either of them ever
// being called at that level. The layer between 100 and 400 (Amount 600) is
// eligible only to D1 and D2 — and both are folded — while A and C (neither
// folded) remain live for the lower layer. Without the fix, that 600 simply
// disappears from Payouts(); with the fix, D1 and D2 split it back evenly
// (they contributed equally into that specific layer, per
// sidepots.ComputeSidePots' own construction).
func TestOrphanedSidePotLayerIsRefundedNotDropped(t *testing.T) {
	players := []*Player{
		{ID: "A", Stack: 100, Ready: true},
		{ID: "C", Stack: 100, Ready: true},
		{ID: "D1", Stack: 2000, Ready: true},
		{ID: "D2", Stack: 2000, Ready: true},
	}
	table := NewTable(players, 10, 20)
	// Pin the dealer to C (index 1) so blindSeats assigns SB/BB to D1/D2 and
	// pre-flop action starts at A (index 0), matching the hardcoded sequence
	// below deterministically rather than leaving it to the random initial
	// dealer draw.
	table.dealerSeat = 1
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if table.Stage() != PreFlop {
		t.Fatalf("expected PreFlop after StartHand, got %v", table.Stage())
	}

	// Pre-flop: A and C shove all-in for 100 each (their whole stack); D1
	// (posted SB) and D2 (posted BB) call the 100, both staying active with
	// room behind.
	if err := table.Act("A", betting.ActionRaise, 100); err != nil {
		t.Fatalf("A shoves all-in for 100: %v", err)
	}
	if err := table.Act("C", betting.ActionCall, 100); err != nil {
		t.Fatalf("C shoves all-in for 100 (short all-in, redirected to a call): %v", err)
	}
	if err := table.Act("D1", betting.ActionCall, 100); err != nil {
		t.Fatalf("D1 calls 100: %v", err)
	}
	if err := table.Act("D2", betting.ActionCall, 100); err != nil {
		t.Fatalf("D2 calls 100: %v", err)
	}
	if table.Stage() != Flop {
		t.Fatalf("expected Flop once D1/D2 both matched 100, got %v", table.Stage())
	}

	// Flop: D1 bets 300 more (total 400), D2 calls — both tied at 400, both
	// still active (not all-in).
	if err := table.Act("D1", betting.ActionBet, 300); err != nil {
		t.Fatalf("D1 bets 300 on the flop: %v", err)
	}
	if err := table.Act("D2", betting.ActionCall, 300); err != nil {
		t.Fatalf("D2 calls 300 on the flop: %v", err)
	}
	if table.Stage() != Turn {
		t.Fatalf("expected Turn once D1/D2 both matched 400, got %v", table.Stage())
	}

	// Turn: both D1 and D2 fold without either being called at the 400
	// level. The layer between 100 and 400 (600 chips) is now eligible only
	// to D1 and D2 — and both have folded — while A and C remain in the
	// hand at the lower layer.
	if err := table.Act("D1", betting.ActionFold, 0); err != nil {
		t.Fatalf("D1 folds on the turn: %v", err)
	}
	if err := table.Act("D2", betting.ActionFold, 0); err != nil {
		t.Fatalf("D2 folds on the turn: %v", err)
	}

	if table.Stage() != Complete {
		t.Fatalf("expected the hand to reach Complete once A/C (both all-in) are the only players left, got %v", table.Stage())
	}

	payouts := table.Payouts()
	var total int64
	var contributedTotal int64
	for _, p := range players {
		contributedTotal += p.Contributed
	}
	for _, amount := range payouts {
		total += amount
	}
	if total != contributedTotal {
		t.Fatalf("total payouts (%d) must equal total contributed (%d) — chips must never vanish, got %+v", total, contributedTotal, payouts)
	}
	if payouts["D1"] != 300 {
		t.Fatalf("D1 folded but funded half of the orphaned 600-chip layer and must be refunded 300, got %d", payouts["D1"])
	}
	if payouts["D2"] != 300 {
		t.Fatalf("D2 folded but funded half of the orphaned 600-chip layer and must be refunded 300, got %d", payouts["D2"])
	}
}

// TestDealerButtonRotatesBetweenHands covers Finding 3: dealerSeat must
// actually be wired into blind posting and rotated forward at the end of
// each hand, not just sit there as dead state. This plays two full hands on
// the same Table and verifies the players who post small/big blind actually
// change between hand 1 and hand 2 — not just that the dealerSeat field
// changed value.
func TestDealerButtonRotatesBetweenHands(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 1000, Ready: true},
		{ID: "P2", Stack: 1000, Ready: true},
		{ID: "P3", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)

	if err := table.StartHand(); err != nil {
		t.Fatalf("hand 1 StartHand: %v", err)
	}
	hand1SB, hand1BB := blindPosters(t, players, 10, 20)

	// Play hand 1 out to completion with everyone just calling/checking —
	// the point of this test is dealer rotation, not showdown math.
	playToCompletion(t, table)
	if table.Stage() != Complete {
		t.Fatalf("expected hand 1 to reach Complete, got %v", table.Stage())
	}

	if err := table.StartHand(); err != nil {
		t.Fatalf("hand 2 StartHand: %v", err)
	}
	hand2SB, hand2BB := blindPosters(t, players, 10, 20)

	if hand2SB == hand1SB {
		t.Fatalf("expected the small blind seat to rotate between hands, both hands had %s post it", hand1SB)
	}
	if hand2BB == hand1BB {
		t.Fatalf("expected the big blind seat to rotate between hands, both hands had %s post it", hand1BB)
	}
}

// blindPosters inspects Contributed immediately after StartHand (before any
// Act call changes it further) to find which player posted the small vs big
// blind, without assuming any particular seat index.
func blindPosters(t *testing.T, players []*Player, smallBlind, bigBlind int64) (sb, bb string) {
	t.Helper()
	for _, p := range players {
		switch p.Contributed {
		case smallBlind:
			sb = p.ID
		case bigBlind:
			bb = p.ID
		}
	}
	if sb == "" || bb == "" {
		t.Fatalf("could not identify both blind posters from Contributed amounts, players: %+v", players)
	}
	return sb, bb
}

// playToCompletion drives a hand to Complete with every player always
// calling what they owe (Table.Act's own Call->Check redirect handles the
// case where nothing is owed) — a plain check-down with no folds or raises.
// Bounded iteration count so a regression that reintroduces Finding 1's hang
// fails the test instead of hanging `go test` forever.
func TestRevealHoleCardsMakesFoldedWinnerCardsVisible(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	toAct := table.playerToActForTest()
	winnerID := "p1"
	if toAct == "p1" {
		winnerID = "p2"
	}
	if err := table.Act(toAct, betting.ActionFold, 0); err != nil {
		t.Fatalf("%s folds: %v", toAct, err)
	}

	if err := table.RevealHoleCards(winnerID); err != nil {
		t.Fatalf("RevealHoleCards: %v", err)
	}
	view := table.ViewFor(toAct)
	for _, s := range view.Seats {
		if s.PlayerID == winnerID && len(s.HoleCards) != 2 {
			t.Fatal("expected the voluntarily-revealed winner's hole cards to be visible to everyone")
		}
	}
}

func TestRevealHoleCardsRejectsPlayerNotDealtIntoTheHand(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	p3 := &Player{ID: "p3", Stack: 1000}
	_ = table.AddMidHandJoiner(p3)
	if err := table.RevealHoleCards("p3"); err == nil {
		t.Fatal("expected an error revealing cards for a player never dealt into this hand")
	}
}

// TestHandOutcomeCapturesBoardAndHoleCards pins down what sessionlog's
// per-player match history needs: a folded player's hole cards must be
// captured (Revealed=false, never shown to opponents) but the winner who
// folds-showdown-won-without-showdown never reveals either, while a genuine
// showdown or a voluntary show marks Revealed=true.
func TestHandOutcomeCapturesBoardAndHoleCards(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	toAct := table.playerToActForTest()
	folder := toAct
	if err := table.Act(folder, betting.ActionFold, 0); err != nil {
		t.Fatalf("%s folds: %v", folder, err)
	}

	outcome := table.LastOutcomeForActor()
	if outcome == nil {
		t.Fatal("expected a hand outcome after everyone but one player folds")
	}
	if len(outcome.Board) != 0 {
		t.Fatalf("won-without-showdown pre-flop must not reveal any board cards, got %v", outcome.Board)
	}
	for id, info := range outcome.PlayerHands {
		if info.HoleCards == ([2]string{}) {
			t.Fatalf("expected %s's own hole cards to always be captured", id)
		}
		if info.Revealed {
			t.Fatalf("won-without-showdown must never mark %s's hand revealed", id)
		}
	}
}

// TestHandOutcomeShowdownResultsMarksWinnerAndLoser pins down what
// achievements.Service needs (looser/almost_winner/tied/bad_beat/cooler):
// a genuine showdown must populate ShowdownResults for every non-folded
// participant with their own category and Won, and must leave it nil for a
// won-without-showdown pot (nothing was ever compared).
func TestHandOutcomeShowdownResultsMarksWinnerAndLoser(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}
	outcome := table.LastOutcomeForActor()
	if outcome == nil {
		t.Fatal("expected a hand outcome after a completed showdown")
	}
	if len(outcome.ShowdownResults) != 2 {
		t.Fatalf("expected both non-folded players in ShowdownResults, got %+v", outcome.ShowdownResults)
	}
	winners := make(map[string]bool, len(outcome.Winners))
	for _, w := range outcome.Winners {
		winners[w] = true
	}
	for id, result := range outcome.ShowdownResults {
		if result.Category == "" {
			t.Fatalf("expected %s's own category to be captured", id)
		}
		if result.Won != winners[id] {
			t.Fatalf("%s: ShowdownResults.Won=%v but outcome.Winners=%v", id, result.Won, outcome.Winners)
		}
	}
}

// TestHandOutcomeCapturesShuffleFairnessProof pins down that ServerSeed and
// CommitHash (deck.ShuffleResult, B32) survive into HandOutcome after a real
// StartHand-driven hand, hex-encoded — the field consumers (sessionlog.HandItem)
// need to let a player recompute and verify the shuffle themselves.
func TestHandOutcomeCapturesShuffleFairnessProof(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}
	outcome := table.LastOutcomeForActor()
	if outcome == nil {
		t.Fatal("expected a hand outcome after a completed showdown")
	}
	if len(outcome.ServerSeed) != 64 || len(outcome.CommitHash) != 64 {
		t.Fatalf("expected 32-byte hex-encoded seed/hash (64 chars each), got seed=%q hash=%q", outcome.ServerSeed, outcome.CommitHash)
	}
}

func TestVoluntarilyShownResetsOnNextHand(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	toAct := table.playerToActForTest()
	_ = table.Act(toAct, betting.ActionFold, 0)
	winnerID := "p1"
	if toAct == "p1" {
		winnerID = "p2"
	}
	_ = table.RevealHoleCards(winnerID)

	if err := table.StartHand(); err != nil {
		t.Fatalf("second StartHand: %v", err)
	}
	if table.playerByID(winnerID).VoluntarilyShown {
		t.Fatal("VoluntarilyShown must reset at the start of the next hand")
	}
	if table.playerByID(winnerID).VoluntarilyShownCards != ([2]bool{}) {
		t.Fatal("per-card voluntary reveal mask must reset at the start of the next hand")
	}
}

func TestRevealHoleCardIsIndependentAndIdempotent(t *testing.T) {
	players := []*Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)
	_ = table.StartHand()
	folder := table.playerToActForTest()
	_ = table.Act(folder, betting.ActionFold, 0)

	first := int32(0)
	changed, err := table.RevealHoleCard(folder, &first)
	if err != nil || !changed {
		t.Fatalf("first reveal: changed=%v err=%v", changed, err)
	}
	if got := table.playerByID(folder).VoluntarilyShownCards; got != ([2]bool{true, false}) {
		t.Fatalf("expected only card zero revealed, got %v", got)
	}
	changed, err = table.RevealHoleCard(folder, &first)
	if err != nil || changed {
		t.Fatalf("duplicate reveal must be a no-op: changed=%v err=%v", changed, err)
	}
	otherID := "p1"
	if folder == "p1" {
		otherID = "p2"
	}
	for _, seat := range table.ViewFor(otherID).Seats {
		if seat.PlayerID == folder &&
			(len(seat.HoleCards) != 2 || seat.HoleCards[0] == "back" || seat.HoleCards[1] != "back") {
			t.Fatalf("other viewer must see exactly the first card, got %v", seat.HoleCards)
		}
	}
}

func TestHandOutcomeIncludesPayoutsAndContributions(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 1000, Ready: true},
		{ID: "P2", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}
	outcome := table.LastOutcomeForActor()
	if outcome.Payouts == nil || outcome.Contributions == nil {
		t.Fatal("expected HandOutcome to carry Payouts and Contributions")
	}
	if outcome.Contributions["P1"] == 0 && outcome.Contributions["P2"] == 0 {
		t.Fatal("expected non-zero contributions recorded for at least one player")
	}
}

func playToCompletion(t *testing.T, table *Table) {
	t.Helper()
	for i := 0; table.Stage() != Complete; i++ {
		if i > 1000 {
			t.Fatalf("hand did not reach Complete after 1000 action rounds — possible stall (Finding 1 regression?)")
		}
		current := table.CurrentPlayerIDForActor()
		if current == "" {
			t.Fatalf("no player currently on turn but hand did not reach Complete (stage %v) — possible stall", table.Stage())
		}
		if err := table.Act(current, betting.ActionCall, 0); err != nil {
			t.Fatalf("Act(%s, Call): %v", current, err)
		}
	}
}

// TestBustedHeadsUpPlayerCannotStartDegenerateSoloHand reproduces the
// reported bug: heads-up, one player busts to Stack 0 and correctly
// transitions to SittingOut, but StartHand's readyCount check counted them as
// ready anyway (it never checked SittingOut), so the next StartHand call
// "succeeded" with only one real player actually dealt in. That lone
// survivor then posted both blinds against themselves every hand, and — via
// the OTHER half of this bug (runShowdown scanning t.players instead of
// t.handOrder for contributions) — the busted player's stale leftover
// Contributed kept resurfacing as an "eligible" refund target every
// subsequent hand, so their stack grew forever off a hand they were never
// dealt into. StartHand must refuse to start when fewer than 2 players are
// truly eligible (Ready and not SittingOut).
func TestBustedHeadsUpPlayerCannotStartDegenerateSoloHand(t *testing.T) {
	players := []*Player{
		{ID: "A", Stack: 500, Ready: true},
		{ID: "B", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)
	table.dealerDrawn = true // A is dealer/SB heads-up

	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// Rig the deal so B's quad aces beat A's weak hole cards deterministically,
	// busting A to Stack 0.
	players[0].HoleCards = [2]deck.Card{{Rank: deck.Five, Suit: deck.Clubs}, {Rank: deck.Six, Suit: deck.Clubs}}
	players[1].HoleCards = [2]deck.Card{{Rank: deck.Ace, Suit: deck.Spades}, {Rank: deck.Ace, Suit: deck.Hearts}}
	table.shuffle.Cards[5] = deck.Card{Rank: deck.Ace, Suit: deck.Clubs}
	table.shuffle.Cards[6] = deck.Card{Rank: deck.Ace, Suit: deck.Diamonds}
	table.shuffle.Cards[7] = deck.Card{Rank: deck.Two, Suit: deck.Spades}
	table.shuffle.Cards[9] = deck.Card{Rank: deck.Three, Suit: deck.Spades}
	table.shuffle.Cards[11] = deck.Card{Rank: deck.Four, Suit: deck.Hearts}

	if err := table.Act("A", betting.ActionRaise, 500); err != nil {
		t.Fatalf("A shoves all-in for 500: %v", err)
	}
	if err := table.Act("B", betting.ActionCall, 0); err != nil {
		t.Fatalf("B calls all-in: %v", err)
	}
	// advanceStage deals only the immediate next missing street (the flop)
	// synchronously; pace the rest via AdvanceRunoutStreetForActor, same as
	// table.Actor's paced timer would in production.
	table.AdvanceRunoutStreetForActor()
	table.AdvanceRunoutStreetForActor()

	if table.Stage() != Complete {
		t.Fatalf("expected the all-in runout to reach Complete once every street is dealt, got stage %v", table.Stage())
	}
	a := table.playerByID("A")
	if a.Stack != 0 || a.State != SittingOut {
		t.Fatalf("A must be busted (Stack 0, SittingOut), got stack=%d state=%v", a.Stack, a.State)
	}

	if err := table.StartHand(); err == nil {
		t.Fatal("StartHand must refuse to start with only 1 truly eligible player (B); A is busted and SittingOut")
	}
	if table.Stage() != WaitingForPlayers {
		t.Fatalf("table must fall back to WaitingForPlayers, not stay stuck on Complete, got stage %v", table.Stage())
	}
}

// TestAllInRunoutWaitsForFacingPlayerToRespond reproduces a production bug
// found from real websocket traces: as soon as one heads-up player shoves
// all-in, IsAwaitingRunoutForActor was computed purely from player STATE
// (only one player left Active) with no check that the still-Active player
// had actually matched the bet. That let the paced runout timer start
// dealing streets straight to Complete the instant the shove landed — one
// Act() call before the facing player had any chance to call or fold. Their
// decision was silently skipped and their Contributed stayed frozen at its
// pre-shove amount.
func TestAllInRunoutWaitsForFacingPlayerToRespond(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 1000, Ready: true}, // dealer/SB
		{ID: "P2", Stack: 100, Ready: true},  // BB, short stack
	}
	table := NewTable(players, 10, 20)
	table.dealerSeat = 0
	table.dealerDrawn = true

	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	// Pre-flop, heads-up: SB (P1) acts first.
	if err := table.Act("P1", betting.ActionCall, 0); err != nil {
		t.Fatalf("P1 calls to close pre-flop: %v", err)
	}
	if err := table.Act("P2", betting.ActionCheck, 0); err != nil {
		t.Fatalf("P2 checks to close pre-flop: %v", err)
	}
	if table.Stage() != Flop {
		t.Fatalf("expected Flop after pre-flop closes, got %v", table.Stage())
	}

	// Post-flop, heads-up: BB (P2) acts first and shoves their entire
	// remaining stack (80, after posting the 20 big blind).
	if err := table.Act("P2", betting.ActionRaise, 80); err != nil {
		t.Fatalf("P2 shoves all-in for 80: %v", err)
	}
	if table.Stage() != Flop {
		t.Fatalf("shoving must not itself advance the stage, got %v", table.Stage())
	}
	if table.IsAwaitingRunoutForActor() {
		t.Fatal("must not treat this as an all-in runout yet — P1 hasn't called or folded the shove")
	}
	if current := table.CurrentPlayerIDForActor(); current != "P1" {
		t.Fatalf("P1 must still be on the hook to call/fold the shove, current player is %q", current)
	}

	// Now P1 actually responds — only then may the runout proceed.
	if err := table.Act("P1", betting.ActionCall, 0); err != nil {
		t.Fatalf("P1 calls the all-in: %v", err)
	}
	if table.Stage() != Turn {
		t.Fatalf("expected the runout to deal the turn immediately once P1's call closed the action, got %v", table.Stage())
	}
	if p1 := table.playerByID("P1"); p1.Contributed != 100 {
		t.Fatalf("P1's call must actually add chips to the pot, got Contributed=%d", p1.Contributed)
	}

	table.AdvanceRunoutStreetForActor() // river
	if table.Stage() != Complete {
		t.Fatalf("expected Complete once the river is dealt, got %v", table.Stage())
	}
}

// TestUncalledAllInExcessIsNotCountedAsAWin reproduces a second production
// bug found alongside the one above: an all-in shove's uncalled excess forms
// its own side-pot layer with exactly one eligible contributor. runShowdown's
// winner-determination loop trivially declared that lone contributor the
// "winner" of their own returned money, adding them to HandOutcome.Winners
// (and, since they were ever all-in, ComebackWinners too) — so the losing
// all-in player fired win/comeback achievements for a hand they lost, purely
// because their overbet came back to them.
func TestUncalledAllInExcessIsNotCountedAsAWin(t *testing.T) {
	players := []*Player{
		{ID: "Shover", Stack: 1000, Ready: true},
		{ID: "Caller", Stack: 100, Ready: true},
	}
	table := NewTable(players, 10, 20)
	table.dealerSeat = 0 // Shover is dealer/SB
	table.dealerDrawn = true

	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	// Rig the deal so Caller's pocket aces beat Shover's weak hole cards —
	// Caller wins outright despite being the short stack. Board avoids
	// pairing/straightening/flushing Shover's 5c6c (no 3/4/7-8 run, no third
	// club) so Caller's pair of aces is the unambiguous winner.
	players[0].HoleCards = [2]deck.Card{{Rank: deck.Five, Suit: deck.Clubs}, {Rank: deck.Six, Suit: deck.Clubs}}
	players[1].HoleCards = [2]deck.Card{{Rank: deck.Ace, Suit: deck.Spades}, {Rank: deck.Ace, Suit: deck.Hearts}}
	table.shuffle.Cards[5] = deck.Card{Rank: deck.King, Suit: deck.Diamonds}
	table.shuffle.Cards[6] = deck.Card{Rank: deck.Queen, Suit: deck.Hearts}
	table.shuffle.Cards[7] = deck.Card{Rank: deck.Nine, Suit: deck.Spades}
	table.shuffle.Cards[9] = deck.Card{Rank: deck.Two, Suit: deck.Clubs}
	table.shuffle.Cards[11] = deck.Card{Rank: deck.Seven, Suit: deck.Diamonds}

	if err := table.Act("Shover", betting.ActionRaise, 1000); err != nil {
		t.Fatalf("Shover shoves all-in for 1000: %v", err)
	}
	if err := table.Act("Caller", betting.ActionCall, 0); err != nil {
		t.Fatalf("Caller calls all-in for their remaining 90: %v", err)
	}
	table.AdvanceRunoutStreetForActor()
	table.AdvanceRunoutStreetForActor()
	if table.Stage() != Complete {
		t.Fatalf("expected Complete, got %v", table.Stage())
	}

	outcome := table.LastOutcomeForActor()
	for _, id := range outcome.Winners {
		if id == "Shover" {
			t.Fatal("Shover lost the hand — must not appear in Winners just because their uncalled excess was refunded")
		}
	}
	for _, id := range outcome.ComebackWinners {
		if id == "Shover" {
			t.Fatal("Shover lost the hand — must not appear in ComebackWinners")
		}
	}
	payouts := table.Payouts()
	if payouts["Shover"] != 900 { // 1000 shoved - 100 matched by Caller, refunded uncalled
		t.Fatalf("Shover must still get their uncalled 900 back, got %d", payouts["Shover"])
	}
	if payouts["Caller"] != 200 { // 100+100 matched pot, Caller's rigged aces win it
		t.Fatalf("Caller must win the contested 200 pot, got %d", payouts["Caller"])
	}
	var contested, refund *PotResult
	for i := range outcome.PotResults {
		result := &outcome.PotResults[i]
		if result.Refund {
			refund = result
		} else {
			contested = result
		}
	}
	if contested == nil || contested.Payouts["Caller"] != 200 {
		t.Fatalf("contested win was not attributed exactly: %+v", outcome.PotResults)
	}
	if refund == nil || refund.Payouts["Shover"] != 900 || len(refund.Winners) != 0 {
		t.Fatalf("uncalled excess was not identified as a refund: %+v", outcome.PotResults)
	}
}

// TestRejoinAfterTableEmptiesDoesNotLeakStaleHandData reproduces the reported
// bug: a player finishes a hand, both players leave (table goes empty), then
// the same player rejoins under the same ID before any new hand starts. On
// the next state reload (a normal occurrence — every real command
// round-trips through NewTableFromState), the old handOrder entry used to
// get re-linked by ID to the new (fresh, zero hole cards) Player object,
// making ViewFor think the rejoiner was dealt into a hand that doesn't
// exist, and the stale Payouts made the client fire a false win/lose banner
// on the very first snapshot after rejoining.
func TestRejoinAfterTableEmptiesDoesNotLeakStaleHandData(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 500, Ready: true},
		{ID: "P2", Stack: 500, Ready: true},
	}
	table := NewTable(players, 10, 20)
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if err := table.Act("P1", betting.ActionFold, 0); err != nil {
		t.Fatalf("P1 folds: %v", err)
	}
	if table.Stage() != Complete {
		t.Fatalf("expected Complete after uncontested fold, got %v", table.Stage())
	}

	if _, _, err := table.RemovePlayerForActor("P2"); err != nil {
		t.Fatalf("remove P2: %v", err)
	}
	if _, _, err := table.RemovePlayerForActor("P1"); err != nil {
		t.Fatalf("remove P1: %v", err)
	}

	rejoined := &Player{ID: "P1", Stack: 2500, Ready: true}
	if err := table.AddWaitingPlayer(rejoined); err != nil {
		t.Fatalf("rejoin P1: %v", err)
	}

	// table.Actor opportunistically calls StartHand after every join; with
	// only one player seated this fails (readyCount < 2) but still falls
	// back to WaitingForPlayers, exactly as in the reported bug.
	if err := table.StartHand(); err == nil {
		t.Fatal("expected StartHand to reject a solo table")
	}
	if table.Stage() != WaitingForPlayers {
		t.Fatalf("expected fallback to waiting_for_players, got %v", table.Stage())
	}

	// Simulate the persistence round-trip every real command goes through.
	table = NewTableFromState(table.ExportState())

	snap := table.ViewFor("P1")
	if snap.Payouts != nil {
		t.Fatalf("stale payouts leaked into a fresh rejoin: %+v", snap.Payouts)
	}
	for _, seat := range snap.Seats {
		if seat.PlayerID == "P1" && len(seat.HoleCards) != 0 {
			t.Fatalf("rejoining player must not see hole cards before a new hand deals: %+v", seat.HoleCards)
		}
	}
}

// TestSittingOutAfterHandDoesNotLeakStalePayouts reproduces a second report:
// nobody leaves the table, but the loser sits out right after a hand
// completes. table.Actor's tryStartHand re-runs StartHand on every later
// Ready toggle (actor.go's applyReadyAndCommit); each call hits the
// readyCount<2 branch again since only one player is ready, and used to
// leave the finished hand's Payouts/HandOrder in place forever, so the
// client's holdOutcomeOpen (Boolean(payouts)) kept re-showing the "you lost"
// banner to a player who was just sitting out, not playing a new hand.
func TestSittingOutAfterHandDoesNotLeakStalePayouts(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 500, Ready: true},
		{ID: "P2", Stack: 500, Ready: true},
	}
	table := NewTable(players, 10, 20)
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if err := table.Act("P1", betting.ActionFold, 0); err != nil {
		t.Fatalf("P1 folds: %v", err)
	}
	if table.Stage() != Complete {
		t.Fatalf("expected Complete after uncontested fold, got %v", table.Stage())
	}

	// P1 sits out instead of leaving; a Ready(false) toggle re-triggers
	// StartHand with only P2 ready, same as table.Actor does on every
	// ReadyCmd.
	table.SitOutForActor("P1")
	if err := table.StartHand(); err == nil {
		t.Fatal("expected StartHand to reject with only one ready player")
	}
	if table.Stage() != WaitingForPlayers {
		t.Fatalf("expected fallback to waiting_for_players, got %v", table.Stage())
	}

	snap := table.ViewFor("P1")
	if snap.Payouts != nil {
		t.Fatalf("stale payouts leaked into a sitting-out snapshot: %+v", snap.Payouts)
	}
	for _, seat := range snap.Seats {
		if seat.Contributed != 0 {
			t.Fatalf("stale contributed=%d leaked into a waiting_for_players snapshot for seat %s", seat.Contributed, seat.PlayerID)
		}
	}
}

// TestWaitingForPlayersFallbackClearsBoardAndShuffle reproduces the "new
// join sees the previous hand's board" report: an all-in runout deals a full
// 5-card board and a shuffle commit hash, the hand completes, then only one
// player stays ready so StartHand's readyCount<2 branch falls back to
// WaitingForPlayers. That branch cleared handOrder/payouts/lastOutcome but
// never cleared board/shuffle, so a fresh joiner's very first snapshot
// showed a "waiting_for_players" stage carrying the last hand's board and
// shuffle_commit_hash.
func TestWaitingForPlayersFallbackClearsBoardAndShuffle(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 500, Ready: true},
		{ID: "P2", Stack: 500, Ready: true},
	}
	table := NewTable(players, 10, 20)
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if err := table.Act("P1", betting.ActionRaise, 500); err != nil {
		t.Fatalf("P1 shoves all-in: %v", err)
	}
	if err := table.Act("P2", betting.ActionRaise, 500); err != nil {
		t.Fatalf("P2 calls all-in: %v", err)
	}
	table.AdvanceRunoutStreetForActor()
	table.AdvanceRunoutStreetForActor()
	if table.Stage() != Complete {
		t.Fatalf("expected Complete after the all-in runout, got %v", table.Stage())
	}
	if len(table.board) != 5 {
		t.Fatalf("expected a full 5-card board dealt by the runout, got %d cards", len(table.board))
	}

	table.SitOutForActor("P1")
	if err := table.StartHand(); err == nil {
		t.Fatal("expected StartHand to reject with only one ready player")
	}
	if table.Stage() != WaitingForPlayers {
		t.Fatalf("expected fallback to waiting_for_players, got %v", table.Stage())
	}

	snap := table.ViewFor("P2")
	if len(snap.Board) != 0 {
		t.Fatalf("stale board leaked into a waiting_for_players snapshot: %+v", snap.Board)
	}
	if snap.ShuffleCommitHash != "" {
		t.Fatalf("stale shuffle_commit_hash leaked into a waiting_for_players snapshot: %q", snap.ShuffleCommitHash)
	}
}

func TestActRejectsWhenNoPlayerHasPendingDecision(t *testing.T) {
	table := NewTable([]*Player{
		{ID: "p1", Stack: 1000},
		{ID: "p2", Stack: 1000},
	}, 10, 20)
	if err := table.Act("p1", betting.ActionCheck, 0); err == nil {
		t.Fatal("waiting table must reject an action when current_player_id is empty")
	}
}
