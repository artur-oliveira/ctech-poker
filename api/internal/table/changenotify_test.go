package table

import (
	"context"
	"sync"
	"testing"
	"time"

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
