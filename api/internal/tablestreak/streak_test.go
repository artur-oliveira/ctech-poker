package tablestreak

import (
	"context"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
)

func TestMergeIsVisibleToEveryInstance(t *testing.T) {
	ctx := context.Background()
	backend := cache.NewMemoryBackend(16)
	// Two API instances, one shared Valkey — the production shape.
	instanceA, instanceB := NewService(backend), NewService(backend)

	if _, err := instanceA.Merge(ctx, "table-1", map[string]int{"p1": 2, "p2": -2}); err != nil {
		t.Fatalf("merge on A: %v", err)
	}
	merged, err := instanceB.Merge(ctx, "table-1", map[string]int{"p1": 3, "p3": 1})
	if err != nil {
		t.Fatalf("merge on B: %v", err)
	}
	want := map[string]int{"p1": 3, "p2": -2, "p3": 1}
	for id, streak := range want {
		if merged[id] != streak {
			t.Fatalf("merged = %v, want %v", merged, want)
		}
	}
	loaded, err := instanceA.Load(ctx, "table-1")
	if err != nil {
		t.Fatalf("load on A: %v", err)
	}
	for id, streak := range want {
		if loaded[id] != streak {
			t.Fatalf("A loaded %v, want %v", loaded, want)
		}
	}
}

func TestLoadSeparatesTablesAndEmptyFromUnset(t *testing.T) {
	ctx := context.Background()
	svc := NewService(cache.NewMemoryBackend(16))
	if _, err := svc.Merge(ctx, "table-1", map[string]int{"p1": 1}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	other, err := svc.Load(ctx, "table-2")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if other == nil || len(other) != 0 {
		t.Fatalf("table-2 = %v, want an empty (not nil) map", other)
	}
	// A nil service is "no shared store at all" — the caller keeps whatever it
	// has instead of blanking every seat.
	var absent *Service
	if got, err := absent.Load(ctx, "table-1"); err != nil || got != nil {
		t.Fatalf("nil service load = %v, %v; want nil, nil", got, err)
	}
	if got, err := absent.Merge(ctx, "table-1", map[string]int{"p1": 1}); err != nil || got != nil {
		t.Fatalf("nil service merge = %v, %v; want nil, nil", got, err)
	}
}

func TestMergeOfNothingIsANoOp(t *testing.T) {
	svc := NewService(cache.NewMemoryBackend(16))
	got, err := svc.Merge(context.Background(), "table-1", nil)
	if err != nil || got != nil {
		t.Fatalf("merge(nil) = %v, %v; want nil, nil", got, err)
	}
}
