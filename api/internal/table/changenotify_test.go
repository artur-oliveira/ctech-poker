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

// TestHandleExternalChangeForcesReloadAndBroadcast reproduces the consumer
// side of the signal: SetChangeNotifierForActor's sibling process, on
// receiving Notify, dispatches ExternalChangeCmd so this instance reloads
// (rearming every timer via rearmTimersFromCache) and re-broadcasts to
// whichever of this table's players are connected to THIS process — even
// though nothing local triggered it.
func TestHandleExternalChangeForcesReloadAndBroadcast(t *testing.T) {
	broadcastedFor := map[string]bool{}
	a := New("table-1", nil, true, func(viewerID string, _ hand.Snapshot) {
		broadcastedFor[viewerID] = true
	})
	a.cached = hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)

	if err := a.handleExternalChange(context.Background(), ExternalChangeCmd{}); err != nil {
		t.Fatalf("handleExternalChange: %v", err)
	}

	for _, id := range []string{"p1", "p2"} {
		if !broadcastedFor[id] {
			t.Fatalf("player %s was never broadcast to after an external change signal", id)
		}
	}
}
