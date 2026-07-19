package tableowner

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
)

func TestAdvertiseThenLookup(t *testing.T) {
	r := NewRegistry(cache.NewMemoryBackend(16), 15*time.Second)
	ctx := context.Background()

	if _, ok, err := r.Lookup(ctx, "table-1"); err != nil || ok {
		t.Fatalf("expected no owner yet, got ok=%v err=%v", ok, err)
	}

	if err := r.Advertise(ctx, "table-1", "10.0.1.23:8010"); err != nil {
		t.Fatalf("advertise: %v", err)
	}

	addr, ok, err := r.Lookup(ctx, "table-1")
	if err != nil || !ok || addr != "10.0.1.23:8010" {
		t.Fatalf("expected owner 10.0.1.23:8010, got addr=%q ok=%v err=%v", addr, ok, err)
	}
}
