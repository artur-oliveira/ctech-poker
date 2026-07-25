package cachekit

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
)

func TestGetOrLoadCachesAndInvalidates(t *testing.T) {
	backend := cache.NewMemoryBackend(10)
	ctx := context.Background()
	loads := 0
	load := func() (*string, error) {
		loads++
		v := "value"
		return &v, nil
	}

	v, err := GetOrLoad(ctx, backend, "k", time.Minute, load)
	if err != nil || v == nil || *v != "value" || loads != 1 {
		t.Fatalf("first call: v=%v err=%v loads=%d", v, err, loads)
	}

	v, err = GetOrLoad(ctx, backend, "k", time.Minute, load)
	if err != nil || v == nil || *v != "value" || loads != 1 {
		t.Fatalf("second call should hit cache: v=%v err=%v loads=%d", v, err, loads)
	}

	Invalidate(ctx, backend, "k")
	v, err = GetOrLoad(ctx, backend, "k", time.Minute, load)
	if err != nil || v == nil || loads != 2 {
		t.Fatalf("after invalidate should reload: v=%v err=%v loads=%d", v, err, loads)
	}
}

func TestGetOrLoadDoesNotCacheNil(t *testing.T) {
	backend := cache.NewMemoryBackend(10)
	ctx := context.Background()
	loads := 0
	load := func() (*string, error) {
		loads++
		return nil, nil
	}

	if _, err := GetOrLoad(ctx, backend, "missing", time.Minute, load); err != nil {
		t.Fatalf("GetOrLoad: %v", err)
	}
	if _, err := GetOrLoad(ctx, backend, "missing", time.Minute, load); err != nil {
		t.Fatalf("GetOrLoad: %v", err)
	}
	if loads != 2 {
		t.Fatalf("nil result must never be cached, got loads=%d", loads)
	}
}

func TestJitteredSecondsWithinRange(t *testing.T) {
	for i := 0; i < 100; i++ {
		got := jitteredSeconds(100 * time.Second)
		if got < 80 || got > 120 {
			t.Fatalf("jitteredSeconds(100s) = %d, want within [80,120]", got)
		}
	}
}
