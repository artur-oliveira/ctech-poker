package tablelease

import (
	"context"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
)

// The CAS acquire/renew/release semantics are covered by the shared
// gopkg.aoctech.app/api-commons/lock package's own tests. This file only
// confirms the wrapper wires up correctly: NewService returns a working
// *Service using poker's own TTL/heartbeat defaults and "table:" key
// namespace.

func TestNewServiceAcquireAndRelease(t *testing.T) {
	svc := NewService(cache.NewMemoryBackend(16))
	ctx := context.Background()

	release, ok, err := svc.Acquire(ctx, "table-1")
	if err != nil || !ok {
		t.Fatalf("expected first acquire to succeed, got ok=%v err=%v", ok, err)
	}

	// Contended acquire on the same table confirms the wrapper actually
	// delegates to the shared CAS locker rather than no-oping.
	_, ok2, err2 := svc.Acquire(ctx, "table-1")
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if ok2 {
		t.Fatal("expected contended acquire to fail while lease is held")
	}

	release()
	_, ok3, err3 := svc.Acquire(ctx, "table-1")
	if err3 != nil || !ok3 {
		t.Fatalf("expected acquire after release to succeed, got ok=%v err=%v", ok3, err3)
	}
}

func TestNewServiceRenewWiresThrough(t *testing.T) {
	svc := NewService(cache.NewMemoryBackend(16))
	ctx := context.Background()

	_, ok, err := svc.Acquire(ctx, "table-2")
	if err != nil || !ok {
		t.Fatalf("expected acquire to succeed, got ok=%v err=%v", ok, err)
	}

	if err := svc.Renew(ctx, "table-2"); err != nil {
		t.Fatalf("renew failed: %v", err)
	}

	if err := svc.Renew(ctx, "table-never-acquired"); err == nil {
		t.Fatal("expected renew to fail for a table this process never acquired")
	}
}

func TestNewServiceStartHeartbeatWiresThrough(t *testing.T) {
	svc := NewService(cache.NewMemoryBackend(16))
	ctx := context.Background()

	_, ok, err := svc.Acquire(ctx, "table-3")
	if err != nil || !ok {
		t.Fatalf("expected acquire to succeed, got ok=%v err=%v", ok, err)
	}

	stop := svc.StartHeartbeat(ctx, "table-3", nil)
	stop()
}

// TestNewServiceAppliesTableKeyPrefix proves the "table:" prefix is actually
// applied to the underlying store, not just self-consistent within the
// wrapper. It bypasses the wrapper's own Acquire by calling the embedded
// sharedlock.Locker directly (svc.Locker) — the exact same store instance
// the wrapper's Acquire wrote to — so a raw Acquire on the literal prefixed
// string must find it contended, and a raw Acquire on the bare id must find
// it free (proving the wrapper didn't lock the unprefixed key instead).
//
// A second, independent NewService/sharedlock.New instance can't be used for
// this: the in-memory backend's CAS state lives in a per-Locker memStore,
// not in the cache.Backend argument itself — two separate instances never
// share lock state even when constructed against "the same" MemoryBackend.
func TestNewServiceAppliesTableKeyPrefix(t *testing.T) {
	svc := NewService(cache.NewMemoryBackend(16))
	ctx := context.Background()

	release, ok, err := svc.Acquire(ctx, "table-9")
	if err != nil || !ok {
		t.Fatalf("acquire: ok=%v err=%v", ok, err)
	}
	defer release()

	if _, okPrefixed, err := svc.Locker.Acquire(ctx, "table:table-9"); err != nil {
		t.Fatalf("raw acquire on prefixed key: %v", err)
	} else if okPrefixed {
		t.Fatal(`expected "table:table-9" to already be held — the wrapper's Acquire must apply exactly this prefix`)
	}

	if _, okBare, err := svc.Locker.Acquire(ctx, "table-9"); err != nil {
		t.Fatalf("raw acquire on bare key: %v", err)
	} else if !okBare {
		t.Fatal(`expected the bare key "table-9" to remain free — the wrapper must not lock the unprefixed key`)
	}
}
