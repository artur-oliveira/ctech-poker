package tableconn

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
)

func TestSyncMergesEveryInstancesConnections(t *testing.T) {
	ctx := context.Background()
	backend := cache.NewMemoryBackend(16)
	// Two API instances, one shared Valkey — the production shape.
	instanceA, instanceB := NewService(backend), NewService(backend)

	if _, err := instanceA.Sync(ctx, "table-1", map[string][]string{"p1": {"c1"}}); err != nil {
		t.Fatalf("sync on A: %v", err)
	}
	connected, err := instanceB.Sync(ctx, "table-1", map[string][]string{"p2": {"c2"}})
	if err != nil {
		t.Fatalf("sync on B: %v", err)
	}
	// B never saw p1's socket. Without the shared set its dot for p1 would be
	// wrong, which is the whole point of this package.
	if !connected["p1"]["c1"] || !connected["p2"]["c2"] {
		t.Fatalf("connected = %v, want both p1/c1 and p2/c2", connected)
	}
}

func TestSyncTracksPerConnectionNotJustPlayer(t *testing.T) {
	ctx := context.Background()
	svc := NewService(cache.NewMemoryBackend(16))

	if _, err := svc.Sync(ctx, "table-1", map[string][]string{"p1": {"conn-a", "conn-b"}}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// A second sync only refreshing conn-a must not evict conn-b — merge
	// semantics, same as the old per-player map, now at connection
	// granularity.
	connected, err := svc.Sync(ctx, "table-1", map[string][]string{"p1": {"conn-a"}})
	if err != nil {
		t.Fatalf("resync: %v", err)
	}
	if !connected["p1"]["conn-a"] || !connected["p1"]["conn-b"] {
		t.Fatalf("connected = %v, want both conn-a and conn-b for p1", connected)
	}
}

func TestSyncRetractsAConnectionThisInstanceNoLongerHolds(t *testing.T) {
	ctx := context.Background()
	svc := NewService(cache.NewMemoryBackend(16))
	if _, err := svc.Sync(ctx, "table-1", map[string][]string{"p1": {"c1"}, "p2": {"c2"}}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	// p2 disconnected: the handler re-syncs with the shrunken local set, but
	// p2's entry is still live until it expires, so it is still reported.
	connected, err := svc.Sync(ctx, "table-1", map[string][]string{"p1": {"c1"}})
	if err != nil {
		t.Fatalf("resync: %v", err)
	}
	if !connected["p2"]["c2"] {
		t.Fatalf("connected = %v, want p2/c2 still inside its TTL", connected)
	}

	// Past EntryTTL with nobody refreshing it, p2 drops out.
	restore := timeNowFunc
	t.Cleanup(func() { timeNowFunc = restore })
	timeNowFunc = func() time.Time { return restore().Add(EntryTTL + time.Second) }

	connected, err = svc.Sync(ctx, "table-1", map[string][]string{"p1": {"c1"}})
	if err != nil {
		t.Fatalf("expired sync: %v", err)
	}
	if len(connected["p2"]) != 0 {
		t.Fatalf("connected = %v, want p2 expired", connected)
	}
	if !connected["p1"]["c1"] {
		t.Fatalf("connected = %v, want p1/c1 refreshed", connected)
	}
}

func TestSyncSeparatesTablesAndSkipsBlankIDs(t *testing.T) {
	ctx := context.Background()
	svc := NewService(cache.NewMemoryBackend(16))
	if _, err := svc.Sync(ctx, "table-1", map[string][]string{"p1": {"c1", ""}, "": {"c-orphan"}}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if connected, err := svc.Sync(ctx, "table-1", nil); err != nil {
		t.Fatalf("resync: %v", err)
	} else if len(connected[""]) != 0 {
		t.Fatalf("connected = %v, want no entry for a blank player id", connected)
	} else if connected["p1"]["c1"] == false {
		t.Fatalf("connected = %v, want p1/c1 to survive a blank connID in the same call", connected)
	}

	other, err := svc.Sync(ctx, "table-2", nil)
	if err != nil {
		t.Fatalf("other table: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("table-2 = %v, want empty", other)
	}
}

// A nil service is "no shared store at all" — the caller keeps its local view
// instead of treating every seat as disconnected.
func TestNilServiceReportsNothingShared(t *testing.T) {
	var absent *Service
	got, err := absent.Sync(context.Background(), "table-1", map[string][]string{"p1": {"c1"}})
	if err != nil || got != nil {
		t.Fatalf("nil service sync = %v, %v; want nil, nil", got, err)
	}
	if got, err := NewService(nil).Sync(context.Background(), "t", nil); err != nil || got != nil {
		t.Fatalf("nil backend sync = %v, %v; want nil, nil", got, err)
	}
}

func TestSyncSurfacesACorruptStoredValue(t *testing.T) {
	ctx := context.Background()
	backend := cache.NewMemoryBackend(16)
	if err := backend.Set(ctx, key("table-1"), []byte("not json"), 60); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := NewService(backend).Sync(ctx, "table-1", map[string][]string{"p1": {"c1"}}); err == nil {
		t.Fatal("want a decode error, got nil")
	}
}
