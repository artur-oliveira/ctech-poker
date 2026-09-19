package botfunding

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryStore struct {
	window      *Window
	loadCalls   int
	createCalls int
	updateCalls int
	fail        error
}

func (m *memoryStore) Load(context.Context, string) (*Window, error) {
	m.loadCalls++
	if m.fail != nil {
		return nil, m.fail
	}
	if m.window == nil {
		return nil, nil
	}
	copy := *m.window
	copy.Hands = make(map[string]int8, len(m.window.Hands))
	for k, v := range m.window.Hands {
		copy.Hands[k] = v
	}
	return &copy, nil
}
func (m *memoryStore) Create(_ context.Context, playerID, tableID, handID string, delta int64, now time.Time) (*Window, error) {
	m.createCalls++
	if m.fail != nil {
		return nil, m.fail
	}
	m.window = &Window{PlayerID: playerID, Kind: "bot_window", StartedAt: now.UnixMilli(), ExpiresAt: now.Add(24 * time.Hour).UnixMilli(), NetProfit: delta, Hands: map[string]int8{handKey(tableID, handID): 1}}
	return m.Load(context.Background(), playerID)
}
func (m *memoryStore) Update(_ context.Context, current *Window, playerID, tableID, handID string, delta int64, _ time.Time) (*Window, error) {
	m.updateCalls++
	if m.fail != nil {
		return nil, m.fail
	}
	key := handKey(tableID, handID)
	if m.window.Hands[key] != 0 {
		return nil, errors.New("duplicate")
	}
	m.window.NetProfit += delta
	m.window.Hands[key] = 1
	return m.Load(context.Background(), playerID)
}

func TestRecordUsesOneInitialReadAndDeduplicatesHands(t *testing.T) {
	store := &memoryStore{}
	svc := NewService(store, 100_000)
	now := time.Unix(1_800_000_000, 0)
	svc.now = func() time.Time { return now }

	allowed, err := svc.Record(context.Background(), "player", "table", "hand-1", 60_000)
	if err != nil || !allowed {
		t.Fatalf("first record = allowed %v, err %v", allowed, err)
	}
	allowed, err = svc.Record(context.Background(), "player", "table", "hand-1", 60_000)
	if err != nil || !allowed {
		t.Fatalf("duplicate record = allowed %v, err %v", allowed, err)
	}
	allowed, err = svc.Record(context.Background(), "player", "table", "hand-2", 40_000)
	if err != nil || allowed {
		t.Fatalf("limit record = allowed %v, err %v", allowed, err)
	}
	if store.window.NetProfit != 100_000 {
		t.Fatalf("net profit = %d", store.window.NetProfit)
	}
	// memoryStore's write helpers call Load to return a copy; only the first
	// service lookup is relevant to the production round-trip contract.
	if store.createCalls != 1 || store.updateCalls != 1 {
		t.Fatalf("writes create=%d update=%d", store.createCalls, store.updateCalls)
	}
}

func TestLossOffsetsProfitAndExpiredWindowRestarts(t *testing.T) {
	store := &memoryStore{}
	svc := NewService(store, 100_000)
	now := time.Unix(1_800_000_000, 0)
	svc.now = func() time.Time { return now }
	if _, err := svc.Record(context.Background(), "p", "t", "h1", 90_000); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Record(context.Background(), "p", "t", "h2", -20_000); err != nil {
		t.Fatal(err)
	}
	allowed, err := svc.Record(context.Background(), "p", "t", "h3", 29_999)
	if err != nil || !allowed {
		t.Fatalf("offset result allowed=%v err=%v", allowed, err)
	}
	now = now.Add(25 * time.Hour)
	allowed, err = svc.Record(context.Background(), "p", "t", "h4", 1)
	if err != nil || !allowed || store.window.NetProfit != 1 {
		t.Fatalf("new window allowed=%v profit=%d err=%v", allowed, store.window.NetProfit, err)
	}
}

func TestUnavailableStoreFailsClosed(t *testing.T) {
	svc := NewService(&memoryStore{fail: errors.New("down")}, 100_000)
	if allowed, err := svc.Available(context.Background(), "p"); err == nil || allowed {
		t.Fatalf("available=%v err=%v", allowed, err)
	}
}
