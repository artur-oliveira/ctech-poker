package tablemanager

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
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
// is currently running must force that Actor to reload, which is what closes
// the cross-process staleness window
// docs/specs/2026-09-04-cross-instance-stale-turn-timer.md diagnosed.
//
// It must NOT re-publish. ws.Registry's fan-out is fleet-wide, so the
// instance that committed already reached every player; a second publish here
// only added a duplicate frame carrying this instance's own broadcast-time
// overlays, which is the badge/equity flicker
// docs/specs/2026-09-17-table-snapshot-divergence-and-highlight-winner.md
// diagnosed. The dispatch is observed through the mailbox instead: Run
// processes commands in order, so a later command replying proves the
// notification's command was handled first.
func TestListenForExternalChangesDispatchesToTheMatchingLocalActor(t *testing.T) {
	broadcasted := make(chan struct{}, 8)
	broadcast := func(tableID, viewerID string, _ hand.Snapshot) { broadcasted <- struct{}{} }
	m := NewManager(nil, nil, broadcast, nil)
	seed := func() *hand.Table { return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}}, 10, 20) }
	actor, err := m.GetOrCreateActor(context.Background(), "table-1", seed)
	if err != nil {
		t.Fatal(err)
	}

	listener := &fakeChangeListener{fire: make(chan string, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.ListenForExternalChanges(ctx, listener)

	listener.fire <- "table-1"

	// Dispatch blocks until the actor replies, and Run handles commands in
	// order, so this returning proves the notification's own command ran.
	if err := actor.Dispatch(table.ExternalChangeCmd{Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	select {
	case <-broadcasted:
		t.Fatal("a sibling published a frame for a commit it did not run")
	default:
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

// fakeHandoffListener stands in for tablehandoff.Service: Listen blocks until
// ctx is cancelled, replaying whatever connID sets the test feeds it,
// exactly like a real Valkey subscription replaying published messages.
type fakeHandoffListener struct{ fire chan []string }

func (f *fakeHandoffListener) Listen(ctx context.Context, onClose func(connIDs []string)) {
	for {
		select {
		case <-ctx.Done():
			return
		case ids := <-f.fire:
			onClose(ids)
		}
	}
}

func TestListenForHandoffClosesInvokesCallback(t *testing.T) {
	m := NewManager(nil, nil, nil, nil)
	listener := &fakeHandoffListener{fire: make(chan []string, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan []string, 1)
	go m.ListenForHandoffCloses(ctx, listener, func(connIDs []string) { got <- connIDs })

	listener.fire <- []string{"conn-a"}

	select {
	case ids := <-got:
		if len(ids) != 1 || ids[0] != "conn-a" {
			t.Fatalf("got %v, want [conn-a]", ids)
		}
	case <-time.After(time.Second):
		t.Fatal("ListenForHandoffCloses never invoked the callback")
	}
}
