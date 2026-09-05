package table

import (
	"context"
	"errors"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// fakeHandHooks stands in for internal/handhook: one shared claim set, exactly
// like the Valkey key is in production.
type fakeHandHooks struct {
	claimed map[string]bool
	err     error
	calls   int
}

func newFakeHandHooks() *fakeHandHooks { return &fakeHandHooks{claimed: map[string]bool{}} }

func (f *fakeHandHooks) Claim(_ context.Context, tableID, handID string) (bool, error) {
	f.calls++
	if f.err != nil {
		return false, f.err
	}
	k := tableID + ":" + handID
	if f.claimed[k] {
		return false, nil
	}
	f.claimed[k] = true
	return true, nil
}

// fakeConnStore stands in for internal/tableconn.
type fakeConnStore struct {
	shared map[string]map[string]bool // playerID -> connID -> alive
	err    error
	calls  int
}

func newFakeConnStore() *fakeConnStore { return &fakeConnStore{shared: map[string]map[string]bool{}} }

func (f *fakeConnStore) Sync(_ context.Context, _ string, local map[string][]string) (map[string]map[string]bool, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	for playerID, connIDs := range local {
		if f.shared[playerID] == nil {
			f.shared[playerID] = map[string]bool{}
		}
		for _, connID := range connIDs {
			f.shared[playerID][connID] = true
		}
	}
	out := make(map[string]map[string]bool, len(f.shared))
	for playerID, conns := range f.shared {
		alive := make(map[string]bool, len(conns))
		for connID, v := range conns {
			alive[connID] = v
		}
		out[playerID] = alive
	}
	return out, nil
}

// completeActor builds an actor sitting on a hand that has already reached
// Complete, which is the state every instance can load and broadcast. label
// names the simulated instance; the table ID is deliberately the same for all
// of them, because that is the situation being tested — several instances
// running an Actor for ONE table.
func completeActor(t *testing.T, label string) (*Actor, *[]string) {
	t.Helper()
	table := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	// Fold the hand out so it reaches Complete with a real lastOutcome.
	for table.Stage() != hand.Complete {
		current := table.CurrentPlayerIDForActor()
		if current == "" {
			t.Fatal("no current player before Complete")
		}
		if err := table.Act(current, "fold", 0); err != nil {
			t.Fatalf("fold %s: %v", current, err)
		}
	}
	fired := &[]string{}
	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	t.Cleanup(func() { actor.afkSweepTimer.Stop() })
	actor.cached = table
	actor.handID = "hand-1"
	actor.SetOnHandCompleteForActor(func(handID string, _ hand.HandOutcome, _ map[string]string) {
		*fired = append(*fired, label+"/"+handID)
	})
	return actor, fired
}

// The post-hand hooks credit non-idempotent counters (achievements.RecordHand,
// RecordTableStreak, the auto-rebuy sweep). lastOutcome is persisted state, so
// a second instance that merely broadcasts an already-Complete table — a chat
// message during the 12s countdown is enough — used to re-credit the whole
// hand off its own process-local guard.
func TestHandHooksRunOnceAcrossInstances(t *testing.T) {
	hooks := newFakeHandHooks()
	first, firstFired := completeActor(t, "instance-a")
	second, secondFired := completeActor(t, "instance-b")
	first.SetHandHookClaimerForActor(hooks)
	second.SetHandHookClaimerForActor(hooks)

	first.notifyHandComplete()
	second.notifyHandComplete()

	if len(*firstFired) != 1 {
		t.Fatalf("first instance fired %v, want exactly one", *firstFired)
	}
	if len(*secondFired) != 0 {
		t.Fatalf("second instance fired %v, want none — A already claimed the hand", *secondFired)
	}
}

// Re-broadcasting the same hand must not re-ask the shared store: broadcastAll
// runs on every chat message and reaction while the table sits on Complete.
func TestHandHookClaimIsAskedOncePerHand(t *testing.T) {
	hooks := newFakeHandHooks()
	actor, fired := completeActor(t, "instance-a")
	actor.SetHandHookClaimerForActor(hooks)

	actor.notifyHandComplete()
	actor.notifyHandComplete()
	actor.notifyHandComplete()

	if len(*fired) != 1 {
		t.Fatalf("fired %v, want exactly one", *fired)
	}
	if hooks.calls != 1 {
		t.Fatalf("claim calls = %d, want 1", hooks.calls)
	}
}

// An unreachable Valkey must degrade to the previous at-least-once behaviour.
// Skipping the hooks would lose the hand's achievements and streak for good,
// which is strictly worse than a bounded, visible double credit.
func TestHandHookClaimFailureStillCreditsTheHand(t *testing.T) {
	hooks := newFakeHandHooks()
	hooks.err = errors.New("valkey down")
	actor, fired := completeActor(t, "instance-a")
	actor.SetHandHookClaimerForActor(hooks)

	actor.notifyHandComplete()

	if len(*fired) != 1 {
		t.Fatalf("fired %v, want the hand credited despite the claim error", *fired)
	}
}

// Without a claimer (dev, tests, single instance) nothing changes.
func TestHandHooksRunWithoutASharedClaimer(t *testing.T) {
	actor, fired := completeActor(t, "instance-a")
	actor.notifyHandComplete()
	actor.notifyHandComplete()
	if len(*fired) != 1 {
		t.Fatalf("fired %v, want exactly one", *fired)
	}
}

// The dot has to reflect sockets terminating on other instances. Before this,
// applyPresence defaulted an unknown player to "connected" and trusted a stale
// local mark for one who had reconnected elsewhere.
func TestConnectionDotFollowsTheFleetNotOneInstance(t *testing.T) {
	store := newFakeConnStore()
	first, _ := completeActor(t, "instance-a")
	second, _ := completeActor(t, "instance-b")
	first.SetConnStoreForActor(store)
	second.SetConnStoreForActor(store)

	// p1's socket lives on A; A also holds a stale disconnect mark for p2.
	first.activeConns["p1"] = map[string]struct{}{"c1": {}}
	first.disconnectedSince["p2"] = timeNowFunc()
	first.syncFleetConns(true)

	// B has seen neither socket, yet must agree with A about both seats.
	second.syncFleetConns(true)
	seats := second.cached.ViewFor("p1").Seats
	second.applyPresence(seats)
	states := map[string]string{}
	for _, seat := range seats {
		states[seat.PlayerID] = seat.ConnectionState
	}
	if states["p1"] != "connected" {
		t.Fatalf("p1 = %q, want connected — its socket is live on the other instance", states["p1"])
	}
	if states["p2"] != "disconnected" {
		t.Fatalf("p2 = %q, want disconnected — no instance holds its socket", states["p2"])
	}
}

// A seat whose socket is on THIS instance stays connected even if the shared
// set is stale or a write was lost: the local view can only add, never remove.
func TestLocalSocketWinsOverAStaleFleetSet(t *testing.T) {
	actor, _ := completeActor(t, "instance-a")
	actor.SetConnStoreForActor(newFakeConnStore())
	actor.fleetConnIDs = map[string]map[string]bool{}
	actor.activeConns["p1"] = map[string]struct{}{"c1": {}}

	seats := actor.cached.ViewFor("p1").Seats
	actor.applyPresence(seats)
	for _, seat := range seats {
		if seat.PlayerID == "p1" && seat.ConnectionState != "connected" {
			t.Fatalf("p1 = %q, want connected", seat.ConnectionState)
		}
	}
}

// A failed sync keeps the last known answer rather than blanking every dot.
func TestConnSyncFailureKeepsTheLastKnownSet(t *testing.T) {
	store := newFakeConnStore()
	actor, _ := completeActor(t, "instance-a")
	actor.SetConnStoreForActor(store)
	actor.activeConns["p1"] = map[string]struct{}{"c1": {}}
	actor.syncFleetConns(true)
	if !actor.fleetConnIDs["p1"]["c1"] {
		t.Fatalf("fleetConnIDs = %v, want p1/c1", actor.fleetConnIDs)
	}

	store.err = errors.New("valkey down")
	actor.syncFleetConns(true)
	if !actor.fleetConnIDs["p1"]["c1"] {
		t.Fatalf("fleetConnIDs = %v, want the previous answer kept", actor.fleetConnIDs)
	}
}

// Steady-state broadcasts are paced; connect/disconnect force the round trip
// because every other instance's dot depends on it immediately.
func TestConnSyncIsPacedButForcedOnLifecycleEvents(t *testing.T) {
	store := newFakeConnStore()
	actor, _ := completeActor(t, "instance-a")
	actor.SetConnStoreForActor(store)

	actor.syncFleetConns(false)
	actor.syncFleetConns(false)
	actor.syncFleetConns(false)
	if store.calls != 1 {
		t.Fatalf("paced sync calls = %d, want 1", store.calls)
	}
	actor.syncFleetConns(true)
	if store.calls != 2 {
		t.Fatalf("forced sync calls = %d, want 2", store.calls)
	}
}

// Without a store the dot keeps its previous, local-only meaning.
func TestPresenceFallsBackToTheLocalViewWithoutAStore(t *testing.T) {
	actor, _ := completeActor(t, "instance-a")
	actor.syncFleetConns(true) // no store: must be a no-op, not a panic
	actor.disconnectedSince["p2"] = timeNowFunc()

	seats := actor.cached.ViewFor("p1").Seats
	actor.applyPresence(seats)
	for _, seat := range seats {
		want := "connected"
		if seat.PlayerID == "p2" {
			want = "disconnected"
		}
		if seat.ConnectionState != want {
			t.Fatalf("seat %s = %q, want %q", seat.PlayerID, seat.ConnectionState, want)
		}
	}
}

// The post-hand countdown used to be a bare in-process time.Time, so every
// instance started its own 12s window and only the one that armed it published
// next_hand_unix_ms at all.
func TestNextHandCountdownResumesThePersistedDeadline(t *testing.T) {
	actor, _ := completeActor(t, "instance-b")
	persisted := timeNowFunc().Add(4 * time.Second).UnixMilli()
	actor.pendingNextHandDeadline = persisted

	actor.armNextHandTimer(true)
	t.Cleanup(func() { actor.nextHandTimer.Stop() })

	if got := actor.nextHandDeadline.UnixMilli(); got != persisted {
		t.Fatalf("deadline = %d, want the persisted %d", got, persisted)
	}
	if actor.pendingNextHandDeadline != 0 {
		t.Fatal("pending deadline must be consumed exactly once")
	}
}

// With nothing persisted (a hand completing right now) the countdown starts
// fresh, and what it computed is what the next commit persists.
func TestNextHandCountdownPersistsWhatItArmed(t *testing.T) {
	actor, _ := completeActor(t, "instance-a")
	actor.armNextHandTimer(true)
	t.Cleanup(func() { actor.nextHandTimer.Stop() })

	if got := actor.nextHandDeadlineForPersist(); got != actor.nextHandDeadline.UnixMilli() {
		t.Fatalf("persisted %d, armed %d — they must agree", got, actor.nextHandDeadline.UnixMilli())
	}
}

// TestNextHandDeadlinePersistedBeforeArmMatchesWhatGetsArmed reproduces
// production's real call order — commit() (which calls
// nextHandDeadlineForPersist) always runs *before* broadcastAll's
// armNextHandTimer, the opposite of TestNextHandCountdownPersistsWhatItArmed
// above. Before the 2026-09-04 fix, each call independently called
// timeNowFunc(), so the value actually persisted to DynamoDB and the value
// actually armed (and broadcast to clients) differed by however long the
// work between them took — tens of milliseconds in production, enough that
// a client latching onto whichever value it saw first never matched the
// other and permanently froze the hand-outcome ring's countdown at 0. Also
// covers commitOutcomeLogEntries' pattern of several more
// nextHandDeadlineForPersist calls for the same hand before broadcastAll
// ever runs: every one of them must keep returning the same stashed value.
func TestNextHandDeadlinePersistedBeforeArmMatchesWhatGetsArmed(t *testing.T) {
	actor, _ := completeActor(t, "instance-a")

	// A fake clock that advances 30ms on every call simulates the real
	// production gap: commitOutcomeLogEntries's DynamoDB round trips and
	// hand-outcome hooks genuinely take tens of milliseconds between the
	// commit that calls nextHandDeadlineForPersist and the broadcastAll
	// that calls armNextHandTimer. A real, unmocked clock in a tight test
	// loop advances by microseconds between calls — not enough to survive
	// UnixMilli()'s truncation to milliseconds — so this is what actually
	// exercises the bug the fix closes.
	base := timeNowFunc()
	calls := 0
	old := timeNowFunc
	timeNowFunc = func() time.Time {
		calls++
		return base.Add(time.Duration(calls) * 30 * time.Millisecond)
	}
	t.Cleanup(func() { timeNowFunc = old })

	first := actor.nextHandDeadlineForPersist()
	second := actor.nextHandDeadlineForPersist()
	if second != first {
		t.Fatalf("a second persist call before arming drifted: first=%d second=%d", first, second)
	}

	actor.armNextHandTimer(true)
	t.Cleanup(func() { actor.nextHandTimer.Stop() })

	if got := actor.nextHandDeadline.UnixMilli(); got != first {
		t.Fatalf("armed deadline = %d, want the exact persisted value %d", got, first)
	}
}

// A table that is not on Complete must clear the stored countdown instead of
// inheriting the previous hand's expiry.
func TestNextHandDeadlineIsClearedOffComplete(t *testing.T) {
	table := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	actor := New("instance-a", nil, true, func(string, hand.Snapshot) {})
	t.Cleanup(func() { actor.afkSweepTimer.Stop() })
	actor.cached = table
	actor.handID = "hand-2"
	actor.pendingNextHandDeadline = timeNowFunc().Add(time.Minute).UnixMilli()

	if got := actor.nextHandDeadlineForPersist(); got != 0 {
		t.Fatalf("persisted %d mid-hand, want 0", got)
	}
	actor.armNextHandTimer(false)
	if actor.pendingNextHandDeadline != 0 {
		t.Fatal("leaving Complete must drop the pending deadline")
	}
}

// A table where everyone is connected and quiet broadcasts nothing, so pacing
// the sync off broadcastAll alone let the shared key lapse and every other
// instance showed the whole seat row as disconnected. ensureLoaded is the
// heartbeat: any traffic, down to a keepalive ping, refreshes the entries.
func TestEnsureLoadedKeepsTheFleetSetAlive(t *testing.T) {
	store := newFakeConnStore()
	actor, _ := completeActor(t, "instance-a")
	actor.SetConnStoreForActor(store)
	actor.activeConns["p1"] = map[string]struct{}{"c1": {}}

	// A ping's ReconnectCmd reaches ensureLoaded without ever broadcasting.
	if err := actor.ensureLoaded(context.Background(), false); err != nil {
		t.Fatalf("ensureLoaded: %v", err)
	}
	if store.calls == 0 {
		t.Fatal("ensureLoaded did not refresh the fleet set")
	}
	if !actor.fleetConnIDs["p1"]["c1"] {
		t.Fatalf("fleetConnIDs = %v, want p1/c1", actor.fleetConnIDs)
	}
}

// Re-arming for a hand already counting down must consume the persisted
// deadline too. Left set, the NEXT hand's arm picked up this hand's expired
// value and started immediately, skipping the countdown entirely.
func TestReArmingTheSameHandDoesNotStrandThePersistedDeadline(t *testing.T) {
	actor, _ := completeActor(t, "instance-a")
	actor.armNextHandTimer(true)
	t.Cleanup(func() { actor.nextHandTimer.Stop() })
	firstDeadline := actor.nextHandDeadline

	// A reload of the same hand re-publishes the stored deadline.
	actor.pendingNextHandDeadline = timeNowFunc().Add(-time.Minute).UnixMilli()
	actor.armNextHandTimer(true)
	if actor.nextHandDeadline != firstDeadline {
		t.Fatal("re-arming the same hand must not move its deadline")
	}
	if actor.pendingNextHandDeadline != 0 {
		t.Fatal("the stale pending deadline must be dropped")
	}

	// The next hand must get a full, future window.
	actor.handID = "hand-2"
	actor.armNextHandTimer(true)
	if !actor.nextHandDeadline.After(timeNowFunc()) {
		t.Fatalf("hand-2 deadline %v is not in the future", actor.nextHandDeadline)
	}
}

// A next-hand countdown lost to a transient failure (handleNextHand's timer
// fires, then ensureLoaded or commit errors out) leaves the table on Complete
// with no pending timer. Nothing else on this instance re-derives it: a
// keepalive ping, a reconnect and the sweep all reach armNextHandTimer, which
// used to see nextHandArmedFor still set and suppress the re-arm. The sweep is
// the only unconditional tick, so it has to be the watchdog.
func TestAFKSweepRearmsALostNextHandCountdown(t *testing.T) {
	actor, _ := completeActor(t, "instance-a")
	actor.armNextHandTimer(true)
	if actor.nextHandArmedFor != actor.handID {
		t.Fatalf("setup: armed for %q, want %q", actor.nextHandArmedFor, actor.handID)
	}
	// The state handleNextHand leaves behind when it bails out: the timer has
	// fired (so it is gone) and the marker is cleared, but the table is still
	// Complete and no countdown exists.
	actor.nextHandTimer.Stop()
	actor.nextHandArmedFor = ""
	actor.nextHandTimer = nil

	if err := actor.handleAFKSweep(context.Background(), afkSweepCmd{}); err != nil {
		t.Fatalf("handleAFKSweep: %v", err)
	}
	t.Cleanup(func() {
		if actor.nextHandTimer != nil {
			actor.nextHandTimer.Stop()
		}
	})

	if actor.nextHandArmedFor != actor.handID {
		t.Fatalf("armed for %q after the sweep, want %q — the countdown was never re-derived", actor.nextHandArmedFor, actor.handID)
	}
	if actor.nextHandTimer == nil {
		t.Fatal("no next-hand timer after the sweep")
	}
	if !actor.nextHandDeadline.After(timeNowFunc().Add(-time.Second)) {
		t.Fatalf("deadline %v is stale", actor.nextHandDeadline)
	}
}

// The sweep must not disturb a countdown that is already correct: every arm
// function is idempotent per hand, so a healthy table keeps its deadline.
func TestAFKSweepLeavesAHealthyCountdownAlone(t *testing.T) {
	actor, _ := completeActor(t, "instance-a")
	actor.armNextHandTimer(true)
	t.Cleanup(func() { actor.nextHandTimer.Stop() })
	deadline := actor.nextHandDeadline

	if err := actor.handleAFKSweep(context.Background(), afkSweepCmd{}); err != nil {
		t.Fatalf("handleAFKSweep: %v", err)
	}
	if actor.nextHandDeadline != deadline {
		t.Fatalf("deadline moved from %v to %v", deadline, actor.nextHandDeadline)
	}
}

// handleNextHand must not leave the armed marker behind when it returns
// without starting a hand: the timer that dispatched it is already gone, so a
// stale marker is what blocked every later re-arm.
func TestNextHandClearsTheArmedMarkerWhenItStartsNothing(t *testing.T) {
	table := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	t.Cleanup(func() { actor.afkSweepTimer.Stop() })
	actor.cached = table
	actor.handID = "hand-1"
	// A spurious/late fire: the marker says a countdown is pending for this
	// hand, but the table is mid-hand so handleNextHand returns early.
	actor.nextHandArmedFor = "hand-1"

	if err := actor.handleNextHand(context.Background(), nextHandCmd{}); err != nil {
		t.Fatalf("handleNextHand: %v", err)
	}
	if actor.nextHandArmedFor != "" {
		t.Fatalf("armed marker = %q, want cleared", actor.nextHandArmedFor)
	}
}

// TestTurnDeadlinePersistedBeforeArmMatchesWhatGetsArmed mirrors
// TestNextHandDeadlinePersistedBeforeArmMatchesWhatGetsArmed above, for the
// ordinary per-turn deadline: production always calls turnDeadlineForPersist
// (from commit) before armTurnTimer (from broadcastAll, right after). Before
// this fix each independently called timeNowFunc() to compute a fresh
// deadline the first time a turn began, so a real gap between the two calls
// (in production, other processes reacting to internal/tablenotify's signal
// each ran their own armTurnTimer too) let the persisted value and the
// broadcast value diverge — a client could see several broadcasts of one
// snapshot_version, each with a different action_base_deadline_unix_ms,
// arriving in no guaranteed order.
func TestTurnDeadlinePersistedBeforeArmMatchesWhatGetsArmed(t *testing.T) {
	tbl := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	a := New("table-1", nil, true, func(string, hand.Snapshot) {})
	t.Cleanup(func() { a.afkSweepTimer.Stop() })
	a.cached = tbl
	a.handID = "hand-1"

	// See TestNextHandDeadlinePersistedBeforeArmMatchesWhatGetsArmed for why
	// this needs a fake clock advancing per call rather than a real one.
	base := timeNowFunc()
	calls := 0
	old := timeNowFunc
	timeNowFunc = func() time.Time {
		calls++
		return base.Add(time.Duration(calls) * 30 * time.Millisecond)
	}
	t.Cleanup(func() { timeNowFunc = old })

	first := a.turnDeadlineForPersist()
	second := a.turnDeadlineForPersist()
	if second != first {
		t.Fatalf("a second persist call before arming drifted: first=%d second=%d", first, second)
	}

	current := tbl.CurrentPlayerIDForActor()
	a.armTurnTimer(current, tbl.Stage(), 0)
	t.Cleanup(func() { a.turnTimer.Stop() })

	if got := a.turnDeadline.UnixMilli(); got != first {
		t.Fatalf("armed deadline = %d, want the exact persisted value %d", got, first)
	}
}
