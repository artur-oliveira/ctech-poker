package hand

import (
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

	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if table.Stage() != PreFlop {
		t.Fatalf("expected PreFlop after StartHand, got %v", table.Stage())
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
	// t.nextCard is 6 at this point (3 players x 2 hole cards already
	// dealt); indices 6..10 are the flop/turn/river in dealing order.
	table.shuffle.Cards[6] = deck.Card{Rank: deck.Ace, Suit: deck.Clubs}
	table.shuffle.Cards[7] = deck.Card{Rank: deck.Ace, Suit: deck.Diamonds}
	table.shuffle.Cards[8] = deck.Card{Rank: deck.Two, Suit: deck.Spades}
	table.shuffle.Cards[9] = deck.Card{Rank: deck.Three, Suit: deck.Spades}
	table.shuffle.Cards[10] = deck.Card{Rank: deck.Four, Suit: deck.Hearts}

	// Pre-flop: Dealer raises to 220 (their whole intent), SB shoves all-in
	// for 200 total (a short all-in — SB already posted 10 as small blind,
	// so calling Dealer's raise plus going all-in uses the remaining 190 of
	// their 200 stack; Table.Act redirects this ActionRaise to a Call since
	// 200 can't reach the 220 current bet), BB calls.
	if err := table.Act("Dealer", betting.ActionRaise, 220); err != nil {
		t.Fatalf("Dealer raises to 220: %v", err)
	}
	if err := table.Act("SB", betting.ActionRaise, 200); err != nil {
		t.Fatalf("SB shoves all-in for 200 total: %v", err)
	}
	if err := table.Act("BB", betting.ActionCall, 220); err != nil {
		t.Fatalf("BB calls 220: %v", err)
	}
	if err := table.Act("Dealer", betting.ActionCall, 220); err != nil {
		t.Fatalf("Dealer calls the short all-in (owes nothing more, already at 220): %v", err)
	}

	// SB is all-in with 200 total in the pot; Dealer and BB each have 220 in.
	// Main pot: 200*3=600, eligible all three. Side pot: 20*2=40, eligible
	// Dealer+BB only. Play remaining streets with both non-all-in players
	// checking through (SB has no more decisions — they're all-in).
	for table.Stage() != Showdown && table.Stage() != Complete {
		for _, id := range []string{"Dealer", "BB"} {
			if table.currentPlayerCanAct(id) {
				if err := table.Act(id, betting.ActionCheck, 0); err != nil {
					t.Fatalf("check on %v for %s: %v", table.Stage(), id, err)
				}
			}
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
	// players can still act. Before the fix this would start a Flop betting
	// round with nobody able to complete it, hanging the hand forever; no
	// further call to Act should ever be needed after this line.
	if err := table.Act("BB", betting.ActionFold, 0); err != nil {
		t.Fatalf("BB folds: %v", err)
	}

	if table.Stage() != Complete {
		t.Fatalf("expected the all-in runout to reach Complete immediately, got stage %v", table.Stage())
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
	players[0].HoleCards = [2]deck.Card{{Rank: deck.Ace, Suit: deck.Spades}, {Rank: deck.Ace, Suit: deck.Hearts}}    // Dealer: As Ah
	players[1].HoleCards = [2]deck.Card{{Rank: deck.Five, Suit: deck.Clubs}, {Rank: deck.Six, Suit: deck.Clubs}}     // SB: 5c 6c
	players[2].HoleCards = [2]deck.Card{{Rank: deck.Seven, Suit: deck.Hearts}, {Rank: deck.Eight, Suit: deck.Hearts}} // BB: 7h 8h (folds, never shown)
	table.shuffle.Cards[6] = deck.Card{Rank: deck.Ace, Suit: deck.Clubs}
	table.shuffle.Cards[7] = deck.Card{Rank: deck.Ace, Suit: deck.Diamonds}
	table.shuffle.Cards[8] = deck.Card{Rank: deck.Two, Suit: deck.Spades}
	table.shuffle.Cards[9] = deck.Card{Rank: deck.Three, Suit: deck.Spades}
	table.shuffle.Cards[10] = deck.Card{Rank: deck.Four, Suit: deck.Hearts}

	if err := table.Act("Dealer", betting.ActionRaise, 500); err != nil {
		t.Fatalf("Dealer shoves all-in for 500: %v", err)
	}
	if err := table.Act("SB", betting.ActionRaise, 50); err != nil {
		t.Fatalf("SB shoves all-in for 50 (short all-in, redirected to a call): %v", err)
	}
	if err := table.Act("BB", betting.ActionFold, 0); err != nil {
		t.Fatalf("BB folds: %v", err)
	}

	if table.Stage() != Complete {
		t.Fatalf("expected the all-in runout to reach Complete immediately, got stage %v", table.Stage())
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
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if table.Stage() != PreFlop {
		t.Fatalf("expected PreFlop after StartHand, got %v", table.Stage())
	}

	// Pre-flop: A and C shove all-in for 100 each (their whole stack); D1
	// (posted BB) and D2 call the 100, both staying active with room behind.
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
	playToCompletion(t, table, []string{"P1", "P2", "P3"})
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

func playToCompletion(t *testing.T, table *Table, playerIDs []string) {
	t.Helper()
	for i := 0; table.Stage() != Complete; i++ {
		if i > 1000 {
			t.Fatalf("hand did not reach Complete after 1000 action rounds — possible stall (Finding 1 regression?)")
		}
		acted := false
		for _, id := range playerIDs {
			if table.currentPlayerCanAct(id) {
				if err := table.Act(id, betting.ActionCall, 0); err != nil {
					t.Fatalf("Act(%s, Call): %v", id, err)
				}
				acted = true
			}
		}
		if !acted && table.Stage() != Complete {
			t.Fatalf("no player could act but hand did not reach Complete (stage %v) — possible stall", table.Stage())
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
	table.shuffle.Cards[4] = deck.Card{Rank: deck.Ace, Suit: deck.Clubs}
	table.shuffle.Cards[5] = deck.Card{Rank: deck.Ace, Suit: deck.Diamonds}
	table.shuffle.Cards[6] = deck.Card{Rank: deck.Two, Suit: deck.Spades}
	table.shuffle.Cards[7] = deck.Card{Rank: deck.Three, Suit: deck.Spades}
	table.shuffle.Cards[8] = deck.Card{Rank: deck.Four, Suit: deck.Hearts}

	if err := table.Act("A", betting.ActionRaise, 500); err != nil {
		t.Fatalf("A shoves all-in for 500: %v", err)
	}
	if err := table.Act("B", betting.ActionCall, 0); err != nil {
		t.Fatalf("B calls all-in: %v", err)
	}

	if table.Stage() != Complete {
		t.Fatalf("expected the all-in runout to reach Complete, got stage %v", table.Stage())
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
