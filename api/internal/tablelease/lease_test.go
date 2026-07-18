package tablelease

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
)

func TestAcquireThenContendedAcquireFails(t *testing.T) {
	svc := NewService(cache.NewMemoryBackend(100))
	ctx := context.Background()

	release, ok, err := svc.Acquire(ctx, "table-1")
	if err != nil || !ok {
		t.Fatalf("expected first acquire to succeed, got ok=%v err=%v", ok, err)
	}
	defer release()

	_, ok2, err2 := svc.Acquire(ctx, "table-1")
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if ok2 {
		t.Fatal("expected contended acquire to fail while lease is held")
	}
}

func TestReleaseFreesLeaseForNewAcquire(t *testing.T) {
	svc := NewService(cache.NewMemoryBackend(100))
	ctx := context.Background()

	release, ok, _ := svc.Acquire(ctx, "table-2")
	if !ok {
		t.Fatal("expected first acquire to succeed")
	}
	release()

	_, ok2, err2 := svc.Acquire(ctx, "table-2")
	if err2 != nil || !ok2 {
		t.Fatalf("expected acquire after release to succeed, got ok=%v err=%v", ok2, err2)
	}
}

func TestRenewExtendsLeaseBeforeExpiry(t *testing.T) {
	svc := NewService(cache.NewMemoryBackend(100))
	svc.leaseTTL = 50 * time.Millisecond
	ctx := context.Background()

	_, ok, _ := svc.Acquire(ctx, "table-3")
	if !ok {
		t.Fatal("expected acquire to succeed")
	}

	time.Sleep(30 * time.Millisecond)
	if err := svc.Renew(ctx, "table-3"); err != nil {
		t.Fatalf("renew failed: %v", err)
	}

	time.Sleep(30 * time.Millisecond) // 60ms since acquire, but only 30ms since renew
	_, ok2, _ := svc.Acquire(ctx, "table-3")
	if ok2 {
		t.Fatal("expected contended acquire to still fail — renew should have extended the lease")
	}
}

func TestLeaseExpiresAndCanBeReacquiredWithoutRenew(t *testing.T) {
	svc := NewService(cache.NewMemoryBackend(100))
	svc.leaseTTL = 20 * time.Millisecond
	ctx := context.Background()

	_, ok, _ := svc.Acquire(ctx, "table-4")
	if !ok {
		t.Fatal("expected acquire to succeed")
	}

	time.Sleep(40 * time.Millisecond) // let it expire, never renewed

	_, ok2, err2 := svc.Acquire(ctx, "table-4")
	if err2 != nil || !ok2 {
		t.Fatalf("expected acquire after expiry to succeed, got ok=%v err=%v", ok2, err2)
	}
}
