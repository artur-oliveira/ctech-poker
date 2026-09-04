package tablemanager

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// fakeChangeListener stands in for tablenotify.Service: Listen blocks until
// ctx is cancelled, replaying whatever table IDs the test feeds it through
// fire, exactly like a real Valkey subscription replaying published messages.
type fakeChangeListener struct{ fire chan string }

func (f *fakeChangeListener) Listen(ctx context.Context, onChange func(tableID string)) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-f.fire:
			onChange(id)
		}
	}
}

// TestListenForExternalChangesDispatchesToTheMatchingLocalActor is the
// consumer side of internal/tablenotify: a signal for a table this process
// is currently running must force that Actor to reload and re-broadcast —
// this is what closes the cross-process staleness window
// docs/specs/2026-09-04-cross-instance-stale-turn-timer.md diagnosed.
func TestListenForExternalChangesDispatchesToTheMatchingLocalActor(t *testing.T) {
	broadcastCount := 0
	broadcast := func(tableID, viewerID string, _ hand.Snapshot) { broadcastCount++ }
	m := NewManager(nil, nil, broadcast, nil)
	seed := func() *hand.Table { return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}}, 10, 20) }
	if _, err := m.GetOrCreateActor(context.Background(), "table-1", seed); err != nil {
		t.Fatal(err)
	}

	listener := &fakeChangeListener{fire: make(chan string, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.ListenForExternalChanges(ctx, listener)

	listener.fire <- "table-1"

	deadline := time.Now().Add(time.Second)
	for broadcastCount == 0 {
		if time.Now().After(deadline) {
			t.Fatal("external change signal for a locally-running table never triggered a broadcast")
		}
		time.Sleep(time.Millisecond)
	}
}

// A signal for a table nothing here is currently serving must be silently
// ignored — there is no local Actor to dispatch to, and GetOrCreateActor
// must never be called as a side effect of a fleet-wide notification (that
// would create and immediately abandon an Actor for every table any OTHER
// process touches).
func TestListenForExternalChangesIgnoresUnknownTables(t *testing.T) {
	m := NewManager(nil, nil, nil, nil)
	listener := &fakeChangeListener{fire: make(chan string, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.ListenForExternalChanges(ctx, listener)

	listener.fire <- "table-nobody-here-runs"
	time.Sleep(20 * time.Millisecond)

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.actors) != 0 {
		t.Fatalf("expected no actor to be created for an unknown table, got %d", len(m.actors))
	}
}
