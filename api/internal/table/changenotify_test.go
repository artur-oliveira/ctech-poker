package table

import (
	"context"
	"sync"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// fakeChangeNotifier records every Notify call. Safe for concurrent use since
// notifyChange fires it from a detached goroutine.
type fakeChangeNotifier struct {
	mu        sync.Mutex
	notified  []string
	notifiedC chan string
}

func newFakeChangeNotifier() *fakeChangeNotifier {
	return &fakeChangeNotifier{notifiedC: make(chan string, 8)}
}

func (f *fakeChangeNotifier) Notify(_ context.Context, tableID string) {
	f.mu.Lock()
	f.notified = append(f.notified, tableID)
	f.mu.Unlock()
	f.notifiedC <- tableID
}

func (f *fakeChangeNotifier) awaitOne(t *testing.T) string {
	t.Helper()
	select {
	case id := <-f.notifiedC:
		return id
	case <-time.After(time.Second):
		t.Fatal("changeNotifier.Notify was never called")
		return ""
	}
}

// TestCommitNotifiesChangeOnEverySuccess covers both of commit's exit paths —
// the nil-store fast path unit tests use throughout this package, and the
// same path a real persisted commit takes — since notifyChange is called
// from both (see actor_commit.go). A sibling process only ever learns about
// a commit through this signal; missing it on either path silently widens
// the exact staleness window docs/specs/2026-09-04-cross-instance-stale-turn-timer.md
// diagnosed.
func TestCommitNotifiesChangeOnEverySuccess(t *testing.T) {
	a := New("table-1", nil, true, nil)
	notifier := newFakeChangeNotifier()
	a.SetChangeNotifierForActor(notifier)

	if err := a.commit(context.Background(), "action-1", &tablestore.ActionLogEntry{}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if got := notifier.awaitOne(t); got != "table-1" {
		t.Fatalf("notified table_id = %q, want %q", got, "table-1")
	}
}

// A nil notifier (dev/tests without a cache, and every other test in this
// package that never calls SetChangeNotifierForActor) must never be
// dereferenced.
func TestCommitWithoutAChangeNotifierDoesNotPanic(t *testing.T) {
	a := New("table-1", nil, true, nil)
	if err := a.commit(context.Background(), "action-1", &tablestore.ActionLogEntry{}); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// ws.RedisRegistry.Broadcast PUBLISHes to Valkey and every instance delivers
// to its own local conns, so the committing instance's publish already
// reached every player wherever they are connected. A sibling republishing
// the same version sent the client a second frame carrying that sibling's own
// broadcast-time overlays (streak, equity), which the UI cannot order against
// the first — the reported badge flicker. The reload and the sweeps still
// have to run: that is what re-arms this instance's timers.
// See docs/specs/2026-09-17-table-snapshot-divergence-and-highlight-winner.md.
func TestHandleExternalChangeReloadsWithoutRepublishing(t *testing.T) {
	published := 0
	a := New("table-1", nil, true, func(string, hand.Snapshot) { published++ })
	t.Cleanup(func() { a.afkSweepTimer.Stop() })
	a.cached = hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)

	if err := a.handleExternalChange(context.Background(), ExternalChangeCmd{}); err != nil {
		t.Fatalf("handleExternalChange: %v", err)
	}

	if published != 0 {
		t.Fatalf("a sibling published %d frames for a commit it did not run, want 0", published)
	}
}

// The instance that actually committed is the one that publishes, and it
// still publishes to every seat, not only to its own connections.
func TestBroadcastAllPublishesToEverySeat(t *testing.T) {
	publishedFor := map[string]bool{}
	a := New("table-1", nil, true, func(viewerID string, _ hand.Snapshot) {
		publishedFor[viewerID] = true
	})
	t.Cleanup(func() { a.afkSweepTimer.Stop() })
	a.cached = hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)

	a.broadcastAll()

	for _, id := range []string{"p1", "p2"} {
		if !publishedFor[id] {
			t.Fatalf("player %s was never published to by the committing instance", id)
		}
	}
}

// A sibling suppresses the publish because the committing instance already
// reached every player — but the sweeps inside sync COMMIT, and state born
// on this instance has no other publisher. An inline preselection is the
// live case: the preselection rides in the persisted table activity, so the
// instance that reloads on the ChangeNotifier signal is routinely not the
// one that committed the action opening that player's turn, and it is the
// reloading instance whose sweep fires the auto-action.
//
// Production, 2026-09-29, hand 01M3PX5FE66CJ101WY7E4Q9KDK: version 745 (a
// call) was published normally; 746 (the auto-preselect fold that ENDED the
// hand) and its two outcome-log commits went out under syncWithoutPublish
// and were never broadcast by anyone. Every client stayed frozen on the
// river for the whole ~12s next-hand delay, and the winner's trinca and
// 117.000-chip payout only appeared when the next hand's snapshot arrived.
func TestHandleExternalChangePublishesStateItsOwnSweepCommitted(t *testing.T) {
	game := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}

	var published []hand.Snapshot
	a := New("table-1", nil, true, func(_ string, snap hand.Snapshot) {
		published = append(published, snap)
	})
	t.Cleanup(func() { a.afkSweepTimer.Stop() })
	a.cached = game
	a.handID = "hand-1"
	a.version = 1

	current := game.CurrentPlayerIDForActor()
	waiting := "p1"
	if current == "p1" {
		waiting = "p2"
	}
	if err := a.handlePreselect(context.Background(), PreselectCmd{
		PlayerID: waiting, ActionID: "preselect-fold", Selection: "fold",
		ExpectedSnapshotVersion: uint64(a.version), ExpectedHandID: a.handID,
	}); err != nil {
		t.Fatalf("set preselect fold: %v", err)
	}

	// The sibling's action, which opens `waiting`'s turn. Applying it
	// directly (rather than through broadcastAll) is what makes this the
	// reloading instance: the turn is on the preselected player and nothing
	// has published that fact yet.
	if _, _, err := a.applyActAndCommit(context.Background(), ActCmd{
		PlayerID: current, ActionID: "sibling-call", Action: betting.ActionCall,
		Amount: game.ProspectiveCallAmountForActor(current),
	}); err != nil {
		t.Fatalf("sibling action: %v", err)
	}
	versionAfterSibling := a.version
	published = nil

	if err := a.handleExternalChange(context.Background(), ExternalChangeCmd{}); err != nil {
		t.Fatalf("handleExternalChange: %v", err)
	}

	if a.version == versionAfterSibling {
		t.Fatal("the preselection sweep never committed; this test no longer covers the bug")
	}
	if game.Stage() != hand.Complete {
		t.Fatalf("stage after the auto-fold = %v, want the hand to have completed", game.Stage())
	}
	if len(published) == 0 {
		t.Fatalf("the sweep committed version %d -> %d and published nothing; "+
			"no other instance can ever send that state", versionAfterSibling, a.version)
	}
	last := published[len(published)-1]
	if last.SnapshotVersion != uint64(a.version) {
		t.Fatalf("published snapshot_version = %d, want the post-sweep %d",
			last.SnapshotVersion, a.version)
	}
	if last.Stage != "complete" || len(last.Winners) == 0 {
		t.Fatalf("published snapshot stage=%q winners=%v, want the completed hand with its winner",
			last.Stage, last.Winners)
	}
}

// The suppression itself must survive: a sibling signal whose sweeps commit
// nothing still reloads and re-arms timers without putting a duplicate,
// differently-decorated copy of the committing instance's frame on the wire.
// This is the other half of TestHandleExternalChangeReloadsWithoutRepublishing
// — same assertion, but with a preselection present that must NOT fire.
func TestHandleExternalChangeStillSuppressesWhenNoSweepCommits(t *testing.T) {
	game := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
		{ID: "p3", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}

	published := 0
	a := New("table-1", nil, true, func(string, hand.Snapshot) { published++ })
	t.Cleanup(func() { a.afkSweepTimer.Stop() })
	a.cached = game
	a.handID = "hand-1"
	a.version = 1

	// Preselect for someone who is NOT on the clock, so the sweep finds
	// nothing to apply.
	current := game.CurrentPlayerIDForActor()
	var waiting string
	for _, id := range []string{"p1", "p2", "p3"} {
		if id != current {
			waiting = id
			break
		}
	}
	if err := a.handlePreselect(context.Background(), PreselectCmd{
		PlayerID: waiting, ActionID: "preselect-fold", Selection: "fold",
		ExpectedSnapshotVersion: uint64(a.version), ExpectedHandID: a.handID,
	}); err != nil {
		t.Fatalf("set preselect fold: %v", err)
	}
	versionBefore := a.version
	// handlePreselect commits and broadcasts on its own; only what
	// handleExternalChange publishes is under test here.
	published = 0

	if err := a.handleExternalChange(context.Background(), ExternalChangeCmd{}); err != nil {
		t.Fatalf("handleExternalChange: %v", err)
	}

	if a.version != versionBefore {
		t.Fatalf("version moved %d -> %d; the sweep was not supposed to commit",
			versionBefore, a.version)
	}
	if published != 0 {
		t.Fatalf("a sibling published %d frames for a commit it did not run, want 0", published)
	}
}
