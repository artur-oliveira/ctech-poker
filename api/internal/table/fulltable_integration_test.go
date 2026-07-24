//go:build integration

// Full-lifecycle integration coverage for a growing/shrinking table, driven
// through the same Actor commands the WS gateway dispatches (tablews.go) --
// join, ready, sit-out, act, leave -- and committed against real DynamoDB
// Local, not mocks. Two scenarios:
//
//   - TestNineHandedTableGrowsPlaysPausesAndLeaves: 2 players start heads-up,
//     7 more join one at a time while hands are already in progress (mid-hand
//     PendingEntry -> PostBigBlind -> dealt in next hand), a player pauses
//     (sits out while staying seated) and later returns, another cashes out
//     and leaves for good, and an out-of-turn action is rejected. Chip
//     conservation (sum of seated stacks == total buy-ins minus cash-outs) is
//     checked after every hand.
//   - TestSidePotsSplitCorrectlyAcrossUnequalAllInStacks: three unequal stacks
//     shove all-in preflop, forcing a main pot plus two side-pot layers.
//     Because the deck is shuffled from crypto/rand there is no seam to fix
//     the winner, so the assertions are winner-independent invariants (each
//     stack's payout is capped to the layers it could possibly be eligible
//     for, and the short/mid/big split conserves every chip) -- the exact
//     per-layer math is already unit-tested in
//     internal/engine/hand/sidepots_test.go and internal/engine/sidepots.
package table

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func loadState(t *testing.T, store *tablestore.Store, tableID string) *tablestore.StoredTable {
	t.Helper()
	st, err := store.LoadTable(context.Background(), tableID)
	if err != nil || st == nil {
		t.Fatalf("load table %s: %v", tableID, err)
	}
	return st
}

// waitForStage polls the store (never the Actor's in-memory a.cached, which
// has no lock and is only safe to read from the Actor's own goroutine or
// synchronously right after a Dispatch) until the committed stage matches
// want -- needed after any transition paced by a real timer (the runout
// timer here) rather than a synchronous Dispatch reply.
func waitForStage(t *testing.T, store *tablestore.Store, tableID string, want hand.Stage, timeout time.Duration) *tablestore.StoredTable {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		st := loadState(t, store, tableID)
		if st.State.Stage == want {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for stage %v, last stage %v", want, st.State.Stage)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func joinPlayer(t *testing.T, a *Actor, id string, stack int64, maxSeats int) {
	t.Helper()
	if err := a.Dispatch(JoinCmd{PlayerID: id, Stack: stack, MaxSeats: maxSeats, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("join %s: %v", id, err)
	}
}

func setReady(t *testing.T, a *Actor, id string, ready bool) error {
	t.Helper()
	return a.Dispatch(ReadyCmd{PlayerID: id, Ready: ready, Reply: make(chan error, 1)})
}

func postBigBlind(t *testing.T, a *Actor, id string) {
	t.Helper()
	if err := a.Dispatch(PostBigBlindCmd{PlayerID: id, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("post big blind for %s: %v", id, err)
	}
}

func leavePlayer(t *testing.T, a *Actor, id string) int64 {
	t.Helper()
	stackCh := make(chan int64, 1)
	holdCh := make(chan string, 1)
	if err := a.Dispatch(LeaveCmd{PlayerID: id, Stack: stackCh, HoldID: holdCh, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("leave %s: %v", id, err)
	}
	return <-stackCh
}

func hasLegalAction(actions []string, want string) bool {
	for _, a := range actions {
		if a == want {
			return true
		}
	}
	return false
}

func findPlayer(tbl *hand.Table, id string) *hand.Player {
	for _, p := range tbl.PlayersForActor() {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func sumStacks(tbl *hand.Table) int64 {
	var total int64
	for _, p := range tbl.PlayersForActor() {
		total += p.Stack
	}
	return total
}

func verifyChipConservation(t *testing.T, store *tablestore.Store, tableID string, wantTotal int64) {
	t.Helper()
	tbl := hand.NewTableFromState(loadState(t, store, tableID).State)
	if got := sumStacks(tbl); got != wantTotal {
		t.Fatalf("chip conservation broken: seated stacks sum to %d, want exactly %d (sandbox mode never rakes)", got, wantTotal)
	}
}

func removeID(list []string, id string) []string {
	out := make([]string, 0, len(list))
	for _, x := range list {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

func actionIDGen(prefix string) func() string {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("%s-%d", prefix, n)
	}
}

// playHandCallingDown drives whichever hand is currently in progress to
// Complete using only check/call, never raise, so equal-stacked players
// never go all-in by accident (their stakes are set by blinds/join-BB
// alone). Any seat lacking a check or call option at its own turn is a
// driver bug worth failing on immediately rather than silently skipping.
func playHandCallingDown(t *testing.T, a *Actor, store *tablestore.Store, tableID string, nextActionID func() string) {
	t.Helper()
	for i := 0; i < 300; i++ {
		st := loadState(t, store, tableID)
		tbl := hand.NewTableFromState(st.State)
		if tbl.Stage() == hand.Complete {
			return
		}
		current := tbl.CurrentPlayerIDForActor()
		if current == "" {
			// Between an Act and the next player becoming current there is
			// no async gap on this path (no all-ins here), but leave a
			// short retry margin rather than assuming it's always instant.
			time.Sleep(5 * time.Millisecond)
			continue
		}
		legal := tbl.ViewFor(current).LegalActions
		action := betting.ActionCall
		switch {
		case hasLegalAction(legal.Actions, "check"):
			action = betting.ActionCheck
		case hasLegalAction(legal.Actions, "call"):
			action = betting.ActionCall
		default:
			t.Fatalf("player %s has neither check nor call available: %+v", current, legal)
		}
		if err := a.Dispatch(ActCmd{PlayerID: current, ActionID: nextActionID(), Action: action, Reply: make(chan error, 1)}); err != nil {
			t.Fatalf("act %s %s: %v", current, action, err)
		}
	}
	st := loadState(t, store, tableID)
	tbl := hand.NewTableFromState(st.State)
	var seats []string
	for _, p := range tbl.PlayersForActor() {
		seats = append(seats, fmt.Sprintf("%s(stack=%d,state=%v,contributed=%d)", p.ID, p.Stack, p.State, p.Contributed))
	}
	t.Fatalf("playHandCallingDown: did not reach Complete within the iteration budget, stage=%v board=%d current=%q players=%v",
		tbl.Stage(), len(st.State.Board), tbl.CurrentPlayerIDForActor(), seats)
}

// shoveAllIn pushes whoever is currently on turn all-in: it raises to an
// amount no stack at this table can ever reach, which betting.Round.Act's
// goingAllIn branch clamps down to exactly their remaining stack. If a raise
// isn't legal (their total can't even match the current bet) it calls
// instead, which the engine caps to an all-in call the same way.
func shoveAllIn(t *testing.T, a *Actor, store *tablestore.Store, tableID string, nextActionID func() string) {
	t.Helper()
	tbl := hand.NewTableFromState(loadState(t, store, tableID).State)
	current := tbl.CurrentPlayerIDForActor()
	if current == "" {
		t.Fatal("shoveAllIn: no player currently on turn")
	}
	legal := tbl.ViewFor(current).LegalActions
	action := betting.ActionCall
	if hasLegalAction(legal.Actions, "raise") {
		action = betting.ActionRaise
	}
	if err := a.Dispatch(ActCmd{PlayerID: current, ActionID: nextActionID(), Action: action, Amount: 1_000_000, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("shove %s: %v", current, err)
	}
}

// TestNineHandedTableGrowsPlaysPausesAndLeaves simulates a real table filling
// up: two players start heads-up, seven more join one at a time while hands
// are already running, a seated player pauses and returns, another cashes
// out, and one out-of-turn action attempt is rejected. Buy-in accounting is
// checked to be exact (chip conservation) after every single hand.
func TestNineHandedTableGrowsPlaysPausesAndLeaves(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "fullscale_test")
	mustCreateTestTables(t, db, "fullscale_test")
	tableID := uniqueTableID(t)
	const buyIn = int64(1000)
	const smallBlind, bigBlind = int64(10), int64(20)
	const maxSeats = 9

	ctx := context.Background()
	seed := hand.NewTable(nil, smallBlind, bigBlind)
	if err := store.SeedTable(ctx, tableID, seed.ExportState()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	a.SetEquityEnabledForActor(false) // equity's Monte Carlo simulation is irrelevant here and would otherwise fire on every broadcast across 15+ hands
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go a.Run(runCtx)

	nextActionID := actionIDGen("nine")
	var totalBuyIn int64

	// --- Phase 1: two players join and immediately start playing heads-up.
	// AddWaitingPlayer marks a fresh joiner Ready=true and applyJoinAndCommit
	// calls tryStartHand itself, so no separate ReadyCmd is needed to kick
	// off the very first hand.
	joinPlayer(t, a, "p1", buyIn, maxSeats)
	totalBuyIn += buyIn
	joinPlayer(t, a, "p2", buyIn, maxSeats)
	totalBuyIn += buyIn

	st := loadState(t, store, tableID)
	if st.State.Stage != hand.PreFlop {
		t.Fatalf("expected the hand to auto-start once both p1 and p2 joined, got stage %v", st.State.Stage)
	}

	// --- Invalid state transition: whoever is NOT the current player to act
	// must be rejected, and rejection must not mutate anything.
	current := hand.NewTableFromState(st.State).CurrentPlayerIDForActor()
	notCurrent := "p2"
	if current == "p2" {
		notCurrent = "p1"
	}
	if err := a.Dispatch(ActCmd{PlayerID: notCurrent, ActionID: nextActionID(), Action: betting.ActionCall, Reply: make(chan error, 1)}); err == nil || !strings.Contains(err.Error(), "not player") {
		t.Fatalf("expected an out-of-turn Act from %s (current is %s) to be rejected, got %v", notCurrent, current, err)
	}
	if after := loadState(t, store, tableID); after.Version != st.Version {
		t.Fatalf("expected the rejected out-of-turn action to leave the table version unchanged, got %d -> %d", st.Version, after.Version)
	}

	playHandCallingDown(t, a, store, tableID, nextActionID)
	verifyChipConservation(t, store, tableID, totalBuyIn)

	// --- Phase 2: players 3..9 join one at a time while the current roster
	// keeps playing. Each joiner arrives mid-hand (seated as PendingEntry),
	// posts a big blind to opt into the very next hand, and is only then
	// actually dealt in -- exercising the real "table already running" join
	// path, not the empty-table AddWaitingPlayer path from Phase 1.
	players := []string{"p1", "p2"}
	for n := 3; n <= maxSeats; n++ {
		id := fmt.Sprintf("p%d", n)

		if err := setReady(t, a, players[0], true); err != nil {
			t.Fatalf("re-ready %s: %v", players[0], err)
		}
		if st := loadState(t, store, tableID); st.State.Stage != hand.PreFlop {
			t.Fatalf("expected a fresh hand among %d players before %s joins, got stage %v", len(players), id, st.State.Stage)
		}

		joinPlayer(t, a, id, buyIn, maxSeats)
		postBigBlind(t, a, id)
		totalBuyIn += buyIn

		midHand := hand.NewTableFromState(loadState(t, store, tableID).State)
		if p := findPlayer(midHand, id); p == nil || p.State != hand.PendingEntry {
			t.Fatalf("expected %s to be seated as pending-entry mid-hand, got %+v", id, p)
		}

		playHandCallingDown(t, a, store, tableID, nextActionID)
		verifyChipConservation(t, store, tableID, totalBuyIn)

		if err := setReady(t, a, players[0], true); err != nil {
			t.Fatalf("re-ready %s to start %s's first hand: %v", players[0], id, err)
		}
		next := hand.NewTableFromState(loadState(t, store, tableID).State)
		if next.Stage() != hand.PreFlop {
			t.Fatalf("expected the %d-player hand to start, got stage %v", n, next.Stage())
		}
		if got := len(next.PlayersForActor()); got != n {
			t.Fatalf("expected %d seated players once %s is dealt in, got %d", n, id, got)
		}
		if p := findPlayer(next, id); p == nil || p.State == hand.PendingEntry {
			t.Fatalf("expected %s to be dealt into the %d-player hand, got %+v", id, n, p)
		}

		players = append(players, id)
		playHandCallingDown(t, a, store, tableID, nextActionID)
		verifyChipConservation(t, store, tableID, totalBuyIn)
	}

	// The room's 9-seat cap must actually be enforced.
	if err := a.Dispatch(JoinCmd{PlayerID: "p10", Stack: buyIn, MaxSeats: maxSeats, Reply: make(chan error, 1)}); err == nil {
		t.Fatal("expected a 10th join to be rejected once the 9-seat table is full")
	}

	// --- Phase 3: "pause" -- a player sits out but stays seated, excluded
	// from being dealt into hands until they opt back in.
	pausedID := "p4"
	if err := setReady(t, a, pausedID, false); err != nil {
		t.Fatalf("pause %s: %v", pausedID, err)
	}
	if err := setReady(t, a, players[0], true); err != nil {
		t.Fatalf("re-ready %s to start the post-pause hand: %v", players[0], err)
	}
	paused := hand.NewTableFromState(loadState(t, store, tableID).State)
	if paused.Stage() != hand.PreFlop {
		t.Fatalf("expected a hand to start with %s sitting out, got stage %v", pausedID, paused.Stage())
	}
	if got := len(paused.PlayersForActor()); got != len(players) {
		t.Fatalf("expected %s's seat to stay occupied while paused, got %d players (want %d)", pausedID, got, len(players))
	}
	p := findPlayer(paused, pausedID)
	if p == nil {
		t.Fatalf("expected %s to still occupy a seat while paused", pausedID)
	}
	if p.State != hand.SittingOut {
		t.Fatalf("expected %s to be sitting_out while paused, got state %v", pausedID, p.State)
	}

	playHandCallingDown(t, a, store, tableID, nextActionID)
	verifyChipConservation(t, store, tableID, totalBuyIn)

	// Bring them back. RequestReturnFromSitOut may defer the actual return
	// by one hand if they'd otherwise post a blind out of turn, so give it
	// up to two hands to land.
	if err := setReady(t, a, pausedID, true); err != nil {
		t.Fatalf("un-pause %s: %v", pausedID, err)
	}
	returned := false
	for i := 0; i < 2; i++ {
		if err := setReady(t, a, players[0], true); err != nil {
			t.Fatalf("re-ready %s: %v", players[0], err)
		}
		tbl := hand.NewTableFromState(loadState(t, store, tableID).State)
		if pl := findPlayer(tbl, pausedID); pl != nil && pl.State != hand.PendingEntry && pl.State != hand.SittingOut {
			returned = true
			break
		}
		playHandCallingDown(t, a, store, tableID, nextActionID)
	}
	if !returned {
		t.Fatalf("expected %s to return from sitting out within two hands of un-pausing", pausedID)
	}
	playHandCallingDown(t, a, store, tableID, nextActionID)
	verifyChipConservation(t, store, tableID, totalBuyIn)

	// --- Phase 4: "saída" -- a player cashes out and leaves their seat for
	// good, for exactly their current stack.
	leavingID := "p5"
	stackBeforeLeave := findPlayer(hand.NewTableFromState(loadState(t, store, tableID).State), leavingID).Stack
	got := leavePlayer(t, a, leavingID)
	if got != stackBeforeLeave {
		t.Fatalf("expected %s to cash out their exact stack %d, got %d", leavingID, stackBeforeLeave, got)
	}
	totalBuyIn -= got
	players = removeID(players, leavingID)

	afterLeave := hand.NewTableFromState(loadState(t, store, tableID).State)
	if findPlayer(afterLeave, leavingID) != nil {
		t.Fatalf("expected %s's seat to be freed after leaving", leavingID)
	}
	if got := len(afterLeave.PlayersForActor()); got != len(players) {
		t.Fatalf("expected %d seats remaining after %s left, got %d", len(players), leavingID, got)
	}

	if err := setReady(t, a, players[0], true); err != nil {
		t.Fatalf("re-ready %s: %v", players[0], err)
	}
	playHandCallingDown(t, a, store, tableID, nextActionID)
	verifyChipConservation(t, store, tableID, totalBuyIn)
}

// TestSidePotsSplitCorrectlyAcrossUnequalAllInStacks seats three players with
// unequal stacks, forces all three all-in preflop, and lets the real
// Actor/DynamoDB path pace the runout to Complete. The shuffle comes from
// crypto/rand (no seam to fix the winner), so this asserts winner-independent
// invariants: total chip conservation across every layer, and that no stack
// is ever paid beyond the layers it could possibly be eligible for (a short
// stack can never win more than the main pot, a mid stack never more than
// main+first side pot, and the top stack always recoups its uncalled excess
// since nobody else covered it). The exact per-layer split math is already
// unit-tested in internal/engine/hand/sidepots_test.go.
func TestSidePotsSplitCorrectlyAcrossUnequalAllInStacks(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "fullscale_test")
	mustCreateTestTables(t, db, "fullscale_test")
	tableID := uniqueTableID(t)
	const smallBlind, bigBlind = int64(10), int64(20)
	const (
		shortStack = "short"
		midStack   = "mid"
		bigStack   = "big"
	)
	stacks := map[string]int64{shortStack: 100, midStack: 300, bigStack: 1000}

	ctx := context.Background()
	seed := hand.NewTable([]*hand.Player{
		{ID: shortStack, Stack: stacks[shortStack], Ready: true},
		{ID: midStack, Stack: stacks[midStack], Ready: true},
		{ID: bigStack, Stack: stacks[bigStack], Ready: true},
	}, smallBlind, bigBlind)
	if err := store.SeedTable(ctx, tableID, seed.ExportState()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	a.SetEquityEnabledForActor(false)
	a.runoutStreetDelay = 15 * time.Millisecond
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go a.Run(runCtx)

	nextActionID := actionIDGen("sidepot")
	if err := setReady(t, a, shortStack, true); err != nil {
		t.Fatalf("start hand: %v", err)
	}
	if st := loadState(t, store, tableID); st.State.Stage != hand.PreFlop {
		t.Fatalf("expected the hand to start, got stage %v", st.State.Stage)
	}

	for i := 0; i < 3; i++ {
		shoveAllIn(t, a, store, tableID, nextActionID)
	}

	final := waitForStage(t, store, tableID, hand.Complete, 3*time.Second)
	tbl := hand.NewTableFromState(final.State)
	payouts := tbl.Payouts()

	contributed := map[string]int64{}
	for _, p := range tbl.PlayersForActor() {
		contributed[p.ID] = p.Contributed
	}
	for id, want := range stacks {
		if contributed[id] != want {
			t.Fatalf("expected %s to have shoved their entire %d-chip stack, got contributed=%d", id, want, contributed[id])
		}
	}

	var totalPayout int64
	for id, amount := range payouts {
		if _, known := stacks[id]; !known {
			t.Fatalf("unexpected payout recipient %q: %+v", id, payouts)
		}
		totalPayout += amount
	}
	totalPot := stacks[shortStack] + stacks[midStack] + stacks[bigStack]
	if totalPayout != totalPot {
		t.Fatalf("chip conservation broken across side pots: payouts sum to %d, want %d (main pot + side pots, no rake in sandbox)", totalPayout, totalPot)
	}

	mainPot := stacks[shortStack] * 3
	sidePot1 := (stacks[midStack] - stacks[shortStack]) * 2
	excessLayer := stacks[bigStack] - stacks[midStack]

	if got := payouts[shortStack]; got > mainPot {
		t.Fatalf("short stack is only eligible for the main pot (%d, it never covered any side pot) but was paid %d", mainPot, got)
	}
	if got := payouts[midStack]; got > mainPot+sidePot1 {
		t.Fatalf("mid stack is only eligible for the main pot + first side pot (%d) but was paid %d", mainPot+sidePot1, got)
	}
	if got := payouts[bigStack]; got < excessLayer {
		t.Fatalf("big stack must always recoup its uncalled excess of %d (nobody else covered it) but was paid only %d", excessLayer, got)
	}

	for _, p := range tbl.PlayersForActor() {
		if p.Stack != payouts[p.ID] {
			t.Fatalf("expected %s's final stack to equal their payout %d exactly, got %d", p.ID, payouts[p.ID], p.Stack)
		}
	}
}
