package reactionpurchase

import (
	"context"
	"testing"
)

type fakeCacheBackend struct {
	store map[string][]byte
	gets  int
}

func newFakeCacheBackend() *fakeCacheBackend { return &fakeCacheBackend{store: map[string][]byte{}} }
func (f *fakeCacheBackend) Get(_ context.Context, key string) ([]byte, bool, error) {
	f.gets++
	v, ok := f.store[key]
	return v, ok, nil
}
func (f *fakeCacheBackend) Set(_ context.Context, key string, value []byte, _ int) error {
	f.store[key] = value
	return nil
}
func (f *fakeCacheBackend) Delete(_ context.Context, key string) error {
	delete(f.store, key)
	return nil
}
func (f *fakeCacheBackend) DeletePrefix(context.Context, string) error { return nil }
func (f *fakeCacheBackend) Ping(context.Context) error                 { return nil }

type countingIsOwnedService struct {
	calls int
	owned bool
}

func (s *countingIsOwnedService) IsOwned(context.Context, string, string) (bool, error) {
	s.calls++
	return s.owned, nil
}

func TestOwnershipCacheHitsBackendOnceWithinTTL(t *testing.T) {
	backend := newFakeCacheBackend()
	svc := &countingIsOwnedService{owned: true}
	c := NewOwnershipCache(svc, backend)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		owned, err := c.IsOwned(ctx, "player-1", "cold")
		if err != nil || !owned {
			t.Fatalf("call %d: owned=%v err=%v", i, owned, err)
		}
	}
	if svc.calls != 1 {
		t.Fatalf("expected exactly 1 underlying IsOwned call, got %d", svc.calls)
	}
}
