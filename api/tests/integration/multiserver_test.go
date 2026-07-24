//go:build integration

// Simulates the real production fleet (ARCHITECTURE.md §2): multiple server
// processes, each running its own tablemanager.Manager with its own
// in-memory actor registry, sharing only the DynamoDB store and the lease
// cache backend (Redis in prod). Different players connect through
// different servers -- there is no single "owner" instance for a table, so
// correctness must come entirely from DynamoDB's conditional writes, never
// from in-memory state shared between the two Actor objects (they share
// none).
package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// snapshotSink stands in for the production ws.Registry: reg.Broadcast is
// Redis pub/sub, reaching whichever physical server holds a viewer's live WS
// connection regardless of which server's Actor computed the snapshot (see
// internal/app/app.go's newTableManager). A single sink fed by BOTH
// managers' broadcast callbacks reproduces that fan-out here.
type snapshotSink struct {
	mu   sync.Mutex
	seen map[string]int
}

func newSnapshotSink() *snapshotSink { return &snapshotSink{seen: map[string]int{}} }

func findPlayer(tbl *hand.Table, id string) *hand.Player {
	for _, p := range tbl.PlayersForActor() {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (s *snapshotSink) record(_ string, viewerID string, _ hand.Snapshot) {
	s.mu.Lock()
	s.seen[viewerID]++
	s.mu.Unlock()
}

func (s *snapshotSink) count(viewerID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen[viewerID]
}

// TestTwoServersDifferentPlayersConvergeOnSharedTable: p1's every command
// goes through "server A" (actorA), p2's through "server B" (actorB) --
// exactly how two real backend instances would each own one player's live WS
// connection to the same table. Neither Actor object shares any in-memory
// state with the other; they only share the DynamoDB row and the lease
// cache backend (standing in for shared Redis).
func TestTwoServersDifferentPlayersConvergeOnSharedTable(t *testing.T) {
	db := testDynamoClient(t)
	store := tablestore.NewStore(db, "flow_test")
	mustCreatePokerTables(t, db, "flow_test")
	tableID := uniqueTableID(t)

	sharedLeaseBackend := cache.NewMemoryBackend(16)
	sink := newSnapshotSink()

	mgrA := tablemanager.NewManager(tablelease.NewService(sharedLeaseBackend), store, sink.record, nil)
	mgrB := tablemanager.NewManager(tablelease.NewService(sharedLeaseBackend), store, sink.record, nil)

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	actorA, err := mgrA.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("server A: acquire actor: %v", err)
	}
	actorB, err := mgrB.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("server B: acquire actor: %v", err)
	}
	serverFor := map[string]*table.Actor{"p1": actorA, "p2": actorB}

	nextActionID := actionIDSeq()
	if err := serverFor["p1"].Dispatch(table.JoinCmd{PlayerID: "p1", Stack: 1000, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("join p1 via server A: %v", err)
	}
	if err := serverFor["p2"].Dispatch(table.JoinCmd{PlayerID: "p2", Stack: 1000, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("join p2 via server B: %v", err)
	}

	stored, err := store.LoadTable(context.Background(), tableID)
	if err != nil || stored == nil || stored.State.Stage != hand.PreFlop {
		t.Fatalf("expected the hand to auto-start once both p1 (via A) and p2 (via B) joined, got %+v err=%v", stored, err)
	}
	if sink.count("p1") == 0 || sink.count("p2") == 0 {
		t.Fatalf("expected both players to receive at least one snapshot from the joins, got p1=%d p2=%d", sink.count("p1"), sink.count("p2"))
	}

	// Play the hand to completion, dispatching each action through whichever
	// "server" owns that player -- exercising cross-instance
	// read-before-write: whichever server processes an action must see the
	// OTHER server's most recent commit even though the two Actor objects
	// share no in-memory state at all.
	for i := 0; i < 50; i++ {
		st, err := store.LoadTable(context.Background(), tableID)
		if err != nil {
			t.Fatalf("load table: %v", err)
		}
		tbl := hand.NewTableFromState(st.State)
		if tbl.Stage() == hand.Complete {
			break
		}
		current := tbl.CurrentPlayerIDForActor()
		if current == "" {
			t.Fatalf("no current player but hand not complete, stage=%v", tbl.Stage())
		}
		legal := tbl.ViewFor(current).LegalActions
		action := betting.ActionCall
		if containsAction(legal.Actions, "check") {
			action = betting.ActionCheck
		}
		id := nextActionID()
		if err := serverFor[current].Dispatch(table.ActCmd{PlayerID: current, ActionID: id, Action: action, Reply: make(chan error, 1)}); err != nil {
			t.Fatalf("act %s %s via its own server: %v", current, action, err)
		}
	}

	final, err := store.LoadTable(context.Background(), tableID)
	if err != nil || final.State.Stage != hand.Complete {
		t.Fatalf("expected the hand to reach Complete, got %+v err=%v", final, err)
	}
	tbl := hand.NewTableFromState(final.State)
	var total int64
	for _, p := range tbl.PlayersForActor() {
		total += p.Stack
	}
	if total != 2000 {
		t.Fatalf("chip conservation broken across the two servers: stacks sum to %d, want 2000", total)
	}
	if sink.count("p1") == 0 || sink.count("p2") == 0 {
		t.Fatalf("expected both players to have received snapshots throughout the hand, got p1=%d p2=%d", sink.count("p1"), sink.count("p2"))
	}
}

// TestConcurrentActionsFromBothServersNeverDoubleApply races both servers to
// commit the SAME idempotent action (identical action_id) for the current
// player at once -- simulating a client's retry landing on a different
// server than its original request (e.g. after a load balancer failover)
// while the first attempt is still in flight. Exactly one may actually
// mutate state; the other must observe tablestore's per-action idempotency
// guard as a no-op, never double-apply the action. Run with -race: the two
// Actor goroutines share no state but the DynamoDB row, so this also
// exercises the conditional-write path under genuine concurrent access
// rather than sequential simulation.
func TestConcurrentActionsFromBothServersNeverDoubleApply(t *testing.T) {
	db := testDynamoClient(t)
	store := tablestore.NewStore(db, "flow_test")
	mustCreatePokerTables(t, db, "flow_test")
	tableID := uniqueTableID(t)
	backend := cache.NewMemoryBackend(16)

	mgrA := tablemanager.NewManager(tablelease.NewService(backend), store, nil, nil)
	mgrB := tablemanager.NewManager(tablelease.NewService(backend), store, nil, nil)
	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)
	}
	actorA, err := mgrA.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("server A: acquire actor: %v", err)
	}
	actorB, err := mgrB.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("server B: acquire actor: %v", err)
	}

	if err := actorA.Dispatch(table.ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("ready p1 via A: %v", err)
	}
	if err := actorB.Dispatch(table.ReadyCmd{PlayerID: "p2", Ready: true, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("ready p2 via B: %v", err)
	}

	nextActionID := actionIDSeq()
	for i := 0; i < 20; i++ {
		st, err := store.LoadTable(context.Background(), tableID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		tbl := hand.NewTableFromState(st.State)
		if tbl.Stage() == hand.Complete {
			break
		}
		current := tbl.CurrentPlayerIDForActor()
		if current == "" {
			t.Fatalf("no current player, stage=%v", tbl.Stage())
		}
		legal := tbl.ViewFor(current).LegalActions
		action := betting.ActionCall
		if containsAction(legal.Actions, "check") {
			action = betting.ActionCheck
		}
		id := nextActionID()

		var wg sync.WaitGroup
		errs := make([]error, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			errs[0] = actorA.Dispatch(table.ActCmd{PlayerID: current, ActionID: id, Action: action, Reply: make(chan error, 1)})
		}()
		go func() {
			defer wg.Done()
			errs[1] = actorB.Dispatch(table.ActCmd{PlayerID: current, ActionID: id, Action: action, Reply: make(chan error, 1)})
		}()
		wg.Wait()
		if errs[0] != nil && errs[1] != nil {
			t.Fatalf("both racing servers failed to apply the same action %s for %s: A=%v B=%v", id, current, errs[0], errs[1])
		}
	}

	final, err := store.LoadTable(context.Background(), tableID)
	if err != nil || final.State.Stage != hand.Complete {
		t.Fatalf("expected the hand to reach Complete despite the race, got %+v err=%v", final, err)
	}
	tbl := hand.NewTableFromState(final.State)
	var total int64
	for _, p := range tbl.PlayersForActor() {
		total += p.Stack
	}
	if total != 2000 {
		t.Fatalf("chip conservation broken by the race: stacks sum to %d, want 2000 -- suggests the same action_id was double-applied by both servers", total)
	}
}

// waitForHandComplete polls the shared store (never an Actor's in-memory
// cache) until the hand reaches Complete -- an all-in runout is paced by
// table.RunoutStreetDelay's real timer, not a synchronous Dispatch reply, and
// either server's Actor may be the one whose timer fires the final street.
func waitForHandComplete(t *testing.T, store *tablestore.Store, tableID string, timeout time.Duration) *tablestore.StoredTable {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		st, err := store.LoadTable(context.Background(), tableID)
		if err != nil {
			t.Fatalf("load table: %v", err)
		}
		if st.State.Stage == hand.Complete {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for hand to complete, last stage %v", st.State.Stage)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// shoveAllInVia pushes whoever is currently on turn all-in, dispatching
// through THAT player's own server -- exactly like production, where a
// client's action always goes through the one instance holding their live WS
// connection, never through whichever server happens to be looping the test.
func shoveAllInVia(t *testing.T, serverFor map[string]*table.Actor, store *tablestore.Store, tableID string, nextActionID func() string) {
	t.Helper()
	st, err := store.LoadTable(context.Background(), tableID)
	if err != nil {
		t.Fatalf("load table: %v", err)
	}
	tbl := hand.NewTableFromState(st.State)
	current := tbl.CurrentPlayerIDForActor()
	if current == "" {
		t.Fatal("shoveAllInVia: no player currently on turn")
	}
	legal := tbl.ViewFor(current).LegalActions
	action := betting.ActionCall
	if containsAction(legal.Actions, "raise") {
		action = betting.ActionRaise
	}
	server, ok := serverFor[current]
	if !ok {
		t.Fatalf("shoveAllInVia: no server registered for player %s", current)
	}
	if err := server.Dispatch(table.ActCmd{PlayerID: current, ActionID: nextActionID(), Action: action, Amount: 1_000_000, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("shove %s via its own server: %v", current, err)
	}
}

// TestHeadsUpUnequalAllInAcrossServers pins down the exact heads-up all-in
// settlement production must guarantee regardless of which server processed
// which player's shove: a 2000-stack shoving into a 1000-stack can only ever
// win or lose the 1000 that was actually contested -- their uncalled excess
// of 1000 is never at risk and is refunded untouched if they lose. The
// shuffle is real crypto/rand (no fixed winner), so both outcomes are
// asserted as a single winner-independent invariant covering both branches
// the user called out: big stack wins -> takes all 3000; big stack loses ->
// keeps exactly its uncontested 1000.
func TestHeadsUpUnequalAllInAcrossServers(t *testing.T) {
	db := testDynamoClient(t)
	store := tablestore.NewStore(db, "flow_test")
	mustCreatePokerTables(t, db, "flow_test")
	tableID := uniqueTableID(t)
	sharedLeaseBackend := cache.NewMemoryBackend(16)

	const bigStack, shortStack = "big", "short"
	const bigBuyIn, shortBuyIn = int64(2000), int64(1000)

	mgrA := tablemanager.NewManager(tablelease.NewService(sharedLeaseBackend), store, nil, nil)
	mgrB := tablemanager.NewManager(tablelease.NewService(sharedLeaseBackend), store, nil, nil)
	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{
			{ID: bigStack, Stack: bigBuyIn, Ready: true},
			{ID: shortStack, Stack: shortBuyIn, Ready: true},
		}, 10, 20)
	}
	actorA, err := mgrA.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("server A: acquire actor: %v", err)
	}
	actorB, err := mgrB.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("server B: acquire actor: %v", err)
	}
	serverFor := map[string]*table.Actor{bigStack: actorA, shortStack: actorB}

	if err := actorA.Dispatch(table.ReadyCmd{PlayerID: bigStack, Ready: true, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("start hand: %v", err)
	}
	if st, err := store.LoadTable(context.Background(), tableID); err != nil || st.State.Stage != hand.PreFlop {
		t.Fatalf("expected the hand to start, got %+v err=%v", st, err)
	}

	nextActionID := actionIDSeq()
	for i := 0; i < 2; i++ {
		shoveAllInVia(t, serverFor, store, tableID, nextActionID)
	}

	final := waitForHandComplete(t, store, tableID, 10*time.Second)
	tbl := hand.NewTableFromState(final.State)
	payouts := tbl.Payouts()

	if got := payouts[bigStack] + payouts[shortStack]; got != bigBuyIn+shortBuyIn {
		t.Fatalf("chip conservation broken: payouts sum to %d, want %d", got, bigBuyIn+shortBuyIn)
	}
	// The contested main pot is capped at shortBuyIn*2 -- bigStack's excess
	// (bigBuyIn-shortBuyIn) was never callable and always returns to bigStack
	// regardless of outcome, including a chop where the main pot splits evenly.
	uncontested := bigBuyIn - shortBuyIn
	mainPot := shortBuyIn * 2
	switch payouts[bigStack] {
	case bigBuyIn + shortBuyIn:
		if payouts[shortStack] != 0 {
			t.Fatalf("big stack won everything but short stack was still paid %d", payouts[shortStack])
		}
	case uncontested:
		if payouts[shortStack] != mainPot {
			t.Fatalf("big stack lost the contested %d and should leave short stack with %d, got %d", shortBuyIn, mainPot, payouts[shortStack])
		}
	case uncontested + mainPot/2:
		if payouts[shortStack] != mainPot/2 {
			t.Fatalf("chop pot: big stack got %d but short stack got %d, want %d", payouts[bigStack], payouts[shortStack], mainPot/2)
		}
	default:
		t.Fatalf("big stack's payout %d matches none of win (%d), loss (%d), or chop (%d) outcomes", payouts[bigStack], bigBuyIn+shortBuyIn, uncontested, uncontested+mainPot/2)
	}
	for _, p := range tbl.PlayersForActor() {
		if p.Stack != payouts[p.ID] {
			t.Fatalf("expected %s's final stack to equal their payout %d exactly, got %d", p.ID, payouts[p.ID], p.Stack)
		}
	}
}

// TestThreeWaySidePotsAcrossServers is the user's exact worked example: three
// unequal all-in stacks (500 / 1000 / 2000), each player joined and acting
// through a DIFFERENT server instance (three separate tablemanager.Manager
// objects sharing only DynamoDB + the lease backend), forcing a main pot plus
// two side-pot layers. Winner-independent invariants only (real crypto/rand
// shuffle, no fixed winner): every stack's payout stays capped to the layers
// it could possibly contest, and the three-way split conserves every chip.
func TestThreeWaySidePotsAcrossServers(t *testing.T) {
	db := testDynamoClient(t)
	store := tablestore.NewStore(db, "flow_test")
	mustCreatePokerTables(t, db, "flow_test")
	tableID := uniqueTableID(t)
	sharedLeaseBackend := cache.NewMemoryBackend(16)

	const shortStack, midStack, bigStack = "short", "mid", "big"
	stacks := map[string]int64{shortStack: 500, midStack: 1000, bigStack: 2000}

	mgrA := tablemanager.NewManager(tablelease.NewService(sharedLeaseBackend), store, nil, nil)
	mgrB := tablemanager.NewManager(tablelease.NewService(sharedLeaseBackend), store, nil, nil)
	mgrC := tablemanager.NewManager(tablelease.NewService(sharedLeaseBackend), store, nil, nil)
	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{
			{ID: shortStack, Stack: stacks[shortStack], Ready: true},
			{ID: midStack, Stack: stacks[midStack], Ready: true},
			{ID: bigStack, Stack: stacks[bigStack], Ready: true},
		}, 10, 20)
	}
	actorA, err := mgrA.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("server A (short stack): acquire actor: %v", err)
	}
	actorB, err := mgrB.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("server B (mid stack): acquire actor: %v", err)
	}
	actorC, err := mgrC.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("server C (big stack): acquire actor: %v", err)
	}
	serverFor := map[string]*table.Actor{shortStack: actorA, midStack: actorB, bigStack: actorC}

	if err := actorA.Dispatch(table.ReadyCmd{PlayerID: shortStack, Ready: true, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("start hand: %v", err)
	}
	if st, err := store.LoadTable(context.Background(), tableID); err != nil || st.State.Stage != hand.PreFlop {
		t.Fatalf("expected the hand to start, got %+v err=%v", st, err)
	}

	nextActionID := actionIDSeq()
	for i := 0; i < 3; i++ {
		shoveAllInVia(t, serverFor, store, tableID, nextActionID)
	}

	final := waitForHandComplete(t, store, tableID, 10*time.Second)
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
	for _, amount := range payouts {
		totalPayout += amount
	}
	totalPot := stacks[shortStack] + stacks[midStack] + stacks[bigStack]
	if totalPayout != totalPot {
		t.Fatalf("chip conservation broken across side pots: payouts sum to %d, want %d", totalPayout, totalPot)
	}

	mainPot := stacks[shortStack] * 3
	sidePot1 := (stacks[midStack] - stacks[shortStack]) * 2
	excessLayer := stacks[bigStack] - stacks[midStack]

	if got := payouts[shortStack]; got > mainPot {
		t.Fatalf("short stack is only eligible for the main pot (%d) but was paid %d", mainPot, got)
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

// TestAutoFoldOnTurnTimeoutAcrossServers pins down turn-timeout auto-fold
// (table.RunoutStreetDelay's sibling timer, armTurnTimer/handleTurnTimeout)
// under the exact multi-server condition where it matters most: whichever
// server last processed a command re-arms ITS OWN copy of the per-turn timer
// for whoever is current (broadcastAll), even when that current player is
// someone else's connection entirely. Neither actor shares any timer state
// with the other, so this proves the auto-fold fires correctly regardless of
// which server ends up owning the deadline for a given turn. Uses
// SetTurnTimeoutForActor(2s) on both servers -- the same knob
// tablemanager.Manager wires from room config in production -- so the test
// doesn't wait out the 15s default.
func TestAutoFoldOnTurnTimeoutAcrossServers(t *testing.T) {
	db := testDynamoClient(t)
	store := tablestore.NewStore(db, "flow_test")
	mustCreatePokerTables(t, db, "flow_test")
	tableID := uniqueTableID(t)
	sharedLeaseBackend := cache.NewMemoryBackend(16)

	mgrA := tablemanager.NewManager(tablelease.NewService(sharedLeaseBackend), store, nil, nil)
	mgrB := tablemanager.NewManager(tablelease.NewService(sharedLeaseBackend), store, nil, nil)
	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	}
	actorA, err := mgrA.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("server A: acquire actor: %v", err)
	}
	actorB, err := mgrB.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("server B: acquire actor: %v", err)
	}
	actorA.SetTurnTimeoutForActor(2 * time.Second)
	actorB.SetTurnTimeoutForActor(2 * time.Second)

	if err := actorA.Dispatch(table.ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("start hand: %v", err)
	}
	st, err := store.LoadTable(context.Background(), tableID)
	if err != nil || st.State.Stage != hand.PreFlop {
		t.Fatalf("expected the hand to start, got %+v err=%v", st, err)
	}
	before := hand.NewTableFromState(st.State)
	current := before.CurrentPlayerIDForActor()
	if current == "" {
		t.Fatal("expected a current player to act preflop")
	}

	// Nobody acts from here -- both servers' turn timers are ticking down for
	// the same current player, and whichever fires first must auto-fold them
	// via handleTurnTimeout, letting the uncontested pot settle to the other
	// player without any further Act dispatch at all.
	final := waitForHandComplete(t, store, tableID, 8*time.Second)
	tbl := hand.NewTableFromState(final.State)
	if p := findPlayer(tbl, current); p == nil || p.State != hand.Folded {
		t.Fatalf("expected the timed-out player %s to be auto-folded, got %+v", current, p)
	}
	var total int64
	for _, p := range tbl.PlayersForActor() {
		total += p.Stack
	}
	if total != 2000 {
		t.Fatalf("chip conservation broken by auto-fold: stacks sum to %d, want 2000", total)
	}
}

// TestDisconnectKickRemovesSeatAcrossServers exercises the full
// disconnect-driven removal path -- Connect, Disconnect, then the auto-kick
// timer -- entirely through the SAME server that holds the disconnecting
// player's WS connection (server B), while the other server (A) only ever
// observes the effect through DynamoDB. This is the "kickout" the user asked
// for: a genuinely-gone player's seat is freed and their stack cashed out,
// with no action required from the other server at all. disconnectGrace and
// kickGrace are both shortened (SetDisconnectGraceForActor/
// SetKickGraceForActor -- test-only knobs, no room config exposes them
// today) so the test doesn't wait out the real 45s/5min production defaults.
func TestDisconnectKickRemovesSeatAcrossServers(t *testing.T) {
	db := testDynamoClient(t)
	store := tablestore.NewStore(db, "flow_test")
	mustCreatePokerTables(t, db, "flow_test")
	tableID := uniqueTableID(t)
	sharedLeaseBackend := cache.NewMemoryBackend(16)

	mgrA := tablemanager.NewManager(tablelease.NewService(sharedLeaseBackend), store, nil, nil)
	mgrB := tablemanager.NewManager(tablelease.NewService(sharedLeaseBackend), store, nil, nil)
	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	}
	actorA, err := mgrA.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("server A: acquire actor: %v", err)
	}
	actorB, err := mgrB.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("server B: acquire actor: %v", err)
	}
	// Ordering matches production: turnTimeout+disconnectGrace must clear
	// (folding p2 out of any live hand into SittingOut) well before kickGrace
	// fires, or handleKickTimeout's removal would race a hand p2 is still
	// dealt into (RemovePlayerForActor rejects that -- see its doc comment).
	actorA.SetTurnTimeoutForActor(1 * time.Second)
	actorB.SetTurnTimeoutForActor(1 * time.Second)
	actorB.SetDisconnectGraceForActor(1 * time.Second)
	actorB.SetKickGraceForActor(4 * time.Second)

	if err := actorA.Dispatch(table.ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("start hand: %v", err)
	}
	if st, err := store.LoadTable(context.Background(), tableID); err != nil || st.State.Stage != hand.PreFlop {
		t.Fatalf("expected the hand to start, got %+v err=%v", st, err)
	}

	// p2's own server (B) sees the connection open, then die -- server A never
	// receives either event, exactly like a real client whose WS lands on one
	// specific instance.
	if err := actorB.Dispatch(table.ConnectCmd{PlayerID: "p2", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("connect p2 via B: %v", err)
	}
	if err := actorB.Dispatch(table.DisconnectCmd{PlayerID: "p2", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("disconnect p2 via B: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		st, err := store.LoadTable(context.Background(), tableID)
		if err != nil {
			t.Fatalf("load table: %v", err)
		}
		tbl := hand.NewTableFromState(st.State)
		if findPlayer(tbl, "p2") == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for the disconnect-kick to remove p2's seat, last state: %+v", findPlayer(tbl, "p2"))
		}
		time.Sleep(50 * time.Millisecond)
	}

	final, err := store.LoadTable(context.Background(), tableID)
	if err != nil {
		t.Fatalf("load table: %v", err)
	}
	tbl := hand.NewTableFromState(final.State)
	if got := len(tbl.PlayersForActor()); got != 1 {
		t.Fatalf("expected only p1 to remain seated after p2's disconnect-kick, got %d players", got)
	}
	if p := findPlayer(tbl, "p1"); p == nil {
		t.Fatal("expected p1 to remain seated")
	}
}
