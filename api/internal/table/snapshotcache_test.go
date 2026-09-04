package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// snapshotActor pairs a cache with unreachableStore (see nexthand_test.go):
// any LoadTable this actor performs fails, which is what makes "did this
// snapshot read DynamoDB?" observable without a DynamoDB Local — a snapshot
// served from cache succeeds, one that reloads returns the load error.
func snapshotActor(t *testing.T) *Actor {
	t.Helper()
	actor := New("table-snap", unreachableStore(), true, func(string, hand.Snapshot) {})
	t.Cleanup(func() { actor.afkSweepTimer.Stop() })
	actor.cached = hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	return actor
}

func snapshotCmd(allowCached bool) SnapshotCmd {
	return SnapshotCmd{
		PlayerID: "p1", Snapshot: make(chan hand.Snapshot, 1),
		Reply: make(chan error, 1), AllowCached: allowCached,
	}
}

// A cancelled context makes the failing load return immediately instead of
// waiting out the SDK's dial/retry budget.
func deadCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// sync_state used to force a DynamoDB read per frame, at up to the
// connection's own rate limit (#218). A freshly loaded cache answers it.
func TestSnapshotServesTheCacheWithinTheReloadInterval(t *testing.T) {
	actor := snapshotActor(t)
	actor.lastLoadedAt = time.Now()

	if err := actor.handleSnapshot(deadCtx(t), snapshotCmd(true)); err != nil {
		t.Fatalf("cached snapshot hit the store: %v", err)
	}
}

// The window is a backstop: once it lapses, the next snapshot reads the
// authoritative item again.
func TestSnapshotReloadsOnceTheIntervalLapses(t *testing.T) {
	actor := snapshotActor(t)
	actor.lastLoadedAt = time.Now().Add(-SnapshotReloadInterval - time.Second)

	if err := actor.handleSnapshot(deadCtx(t), snapshotCmd(true)); err == nil {
		t.Fatal("expected a stale cache to force an authoritative reload")
	}
}

// The seat/buy-in read paths never opt in, so they keep their authoritative
// read even against a cache loaded a moment ago.
func TestSnapshotWithoutAllowCachedAlwaysReloads(t *testing.T) {
	actor := snapshotActor(t)
	actor.lastLoadedAt = time.Now()

	if err := actor.handleSnapshot(deadCtx(t), snapshotCmd(false)); err == nil {
		t.Fatal("expected an authoritative reload when AllowCached is false")
	}
}
