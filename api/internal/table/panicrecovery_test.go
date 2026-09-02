package table

import (
	"context"
	"errors"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// A handler panic (here: handleSnapshot dereferencing a nil a.cached, standing
// in for any of the reachable engine panics — an out-of-bounds deal, a
// malformed persisted State, a nil deref in hand.go) must be recovered: the
// caller gets a resyncable tablestore.ErrUnavailable, the cached state is
// dropped, and the actor's Run loop keeps serving subsequent commands.
func TestActorRunSurvivesHandlerPanic(t *testing.T) {
	a := New("panic-table", nil, true, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go a.Run(ctx)
	t.Cleanup(func() {
		cancel()
		<-a.Done()
	})

	// nil store + nil cache => handleSnapshot panics on a.cached.ViewFor.
	reply := make(chan error, 1)
	err := a.Dispatch(SnapshotCmd{PlayerID: "p1", Snapshot: make(chan hand.Snapshot, 1), Reply: reply})
	if err == nil {
		t.Fatal("expected an error from the panicking snapshot command")
	}
	if !errors.Is(err, tablestore.ErrUnavailable) {
		t.Fatalf("panic recovery must return an ErrUnavailable-wrapped error, got %v", err)
	}

	if !a.IsAlive() {
		t.Fatal("actor Run loop died on a handler panic")
	}

	// The loop still processes the next command, and cached state was dropped
	// so this one is served against freshly supplied state.
	a.SetCachedForTest(hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20))
	snapCh := make(chan hand.Snapshot, 1)
	if err := a.Dispatch(SnapshotCmd{PlayerID: "p1", Snapshot: snapCh, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("actor did not recover to serve the next command: %v", err)
	}
	select {
	case <-snapCh:
	case <-time.After(time.Second):
		t.Fatal("no snapshot delivered after a recovered panic")
	}
}

// handleSafely drops the cached fork after a recovered panic so the next
// command reloads authoritative state rather than trusting a mutation that
// never committed.
func TestHandleSafelyDropsCachedStateOnPanic(t *testing.T) {
	a := New("panic-table", nil, true, nil)
	a.SetCachedForTest(hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20))
	a.version = 7
	a.handID = "h-1"

	// Drive a nil-deref panic through handleSnapshot on a cleared cache.
	a.SetCachedForTest(nil)
	err := a.handleSafely(context.Background(), SnapshotCmd{PlayerID: "p1", Snapshot: make(chan hand.Snapshot, 1), Reply: make(chan error, 1)})
	if !errors.Is(err, tablestore.ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if a.cached != nil || a.version != 0 || a.handID != "" {
		t.Fatalf("recovered panic left cached state trusted: cached=%v version=%d handID=%q", a.cached, a.version, a.handID)
	}
}
