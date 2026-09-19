package chatprefs

import (
	"context"
	"testing"
)

type fakeCacheBackend struct {
	store map[string][]byte
}

func newFakeCacheBackend() *fakeCacheBackend { return &fakeCacheBackend{store: map[string][]byte{}} }
func (f *fakeCacheBackend) Get(_ context.Context, key string) ([]byte, bool, error) {
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

type countingStore struct {
	calls int
	prefs *Preferences
}

func (s *countingStore) Get(context.Context, string) (*Preferences, error) {
	s.calls++
	return s.prefs, nil
}

func TestExtraWordsCacheHitsStoreOnceWithinTTL(t *testing.T) {
	backend := newFakeCacheBackend()
	store := &countingStore{prefs: &Preferences{PlayerID: "p1", ExtraWords: []string{"chato"}}}
	c := NewExtraWordsCache(store, backend)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		words, err := c.ExtraWords(ctx, "p1")
		if err != nil || len(words) != 1 || words[0] != "chato" {
			t.Fatalf("call %d: words=%v err=%v", i, words, err)
		}
	}
	if store.calls != 1 {
		t.Fatalf("expected exactly 1 underlying Get call, got %d", store.calls)
	}
}

func TestExtraWordsCacheInvalidateForcesFreshRead(t *testing.T) {
	backend := newFakeCacheBackend()
	store := &countingStore{prefs: nil}
	c := NewExtraWordsCache(store, backend)
	ctx := context.Background()
	if _, err := c.ExtraWords(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	store.prefs = &Preferences{PlayerID: "p1", ExtraWords: []string{"lento"}}
	if err := c.Invalidate(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	words, err := c.ExtraWords(ctx, "p1")
	if err != nil || len(words) != 1 || words[0] != "lento" {
		t.Fatalf("expected fresh read after invalidate, got words=%v err=%v", words, err)
	}
	if store.calls != 2 {
		t.Fatalf("expected 2 underlying Get calls, got %d", store.calls)
	}
}
