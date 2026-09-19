package botfunding

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type memoryStore struct {
	mu                       sync.Mutex
	window                   *Window
	guards                   map[string]bool
	createCalls, updateCalls int
	fail                     error
	loseReply                bool
}

func (m *memoryStore) Load(context.Context, string) (*Window, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return nil, m.fail
	}
	if m.window == nil {
		return nil, nil
	}
	copy := *m.window
	return &copy, nil
}

func (m *memoryStore) AlreadyRecorded(_ context.Context, tableID, handID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return false, m.fail
	}
	return m.guards[handKey(tableID, handID)], nil
}

func (m *memoryStore) Create(_ context.Context, playerID, tableID, handID string, delta int64, now time.Time) (*Window, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	if m.fail != nil {
		return nil, m.fail
	}
	key := handKey(tableID, handID)
	if m.guards[key] || m.window != nil && m.window.ExpiresAt > now.UnixMilli() {
		return nil, conditionalConflict()
	}
	if m.guards == nil {
		m.guards = make(map[string]bool)
	}
	m.guards[key] = true
	m.window = &Window{PlayerID: playerID, Kind: windowKind, StartedAt: now.UnixMilli(), ExpiresAt: now.Add(24 * time.Hour).UnixMilli(), NetProfit: delta}
	copy := *m.window
	if m.loseReply {
		m.loseReply = false
		return nil, conditionalConflict()
	}
	return &copy, nil
}

func (m *memoryStore) Update(_ context.Context, current *Window, _ string, tableID, handID string, delta int64, now time.Time) (*Window, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	if m.fail != nil {
		return nil, m.fail
	}
	key := handKey(tableID, handID)
	if m.guards[key] || m.window == nil || m.window.StartedAt != current.StartedAt || m.window.NetProfit != current.NetProfit || m.window.ExpiresAt <= now.UnixMilli() {
		return nil, conditionalConflict()
	}
	if m.guards == nil {
		m.guards = make(map[string]bool)
	}
	m.guards[key] = true
	m.window.NetProfit += delta
	copy := *m.window
	if m.loseReply {
		m.loseReply = false
		return nil, conditionalConflict()
	}
	return &copy, nil
}

func conditionalConflict() error {
	return &types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{{Code: aws.String("ConditionalCheckFailed")}}}
}

func TestRecordDeduplicatesHandsAndEnforcesLimit(t *testing.T) {
	store := &memoryStore{}
	svc := NewService(store, 100_000)
	now := time.Unix(1_800_000_000, 0)
	svc.now = func() time.Time { return now }
	for _, tc := range []struct {
		hand    string
		delta   int64
		allowed bool
	}{{"h1", 60_000, true}, {"h1", 60_000, true}, {"h2", 40_000, false}} {
		allowed, err := svc.Record(context.Background(), "player", "table", tc.hand, tc.delta)
		if err != nil || allowed != tc.allowed {
			t.Fatalf("%s allowed=%v err=%v", tc.hand, allowed, err)
		}
	}
	if store.window.NetProfit != 100_000 || store.createCalls != 1 || store.updateCalls != 2 {
		t.Fatalf("profit=%d create=%d update=%d", store.window.NetProfit, store.createCalls, store.updateCalls)
	}
}

func TestLossOffsetsProfitAndLateRetryDoesNotCountInNextWindow(t *testing.T) {
	store := &memoryStore{}
	svc := NewService(store, 100_000)
	now := time.Unix(1_800_000_000, 0)
	svc.now = func() time.Time { return now }
	for _, tc := range []struct {
		hand  string
		delta int64
	}{{"h1", 90_000}, {"h2", -20_000}, {"h3", 29_999}} {
		allowed, err := svc.Record(context.Background(), "p", "t", tc.hand, tc.delta)
		if err != nil || !allowed {
			t.Fatalf("%s allowed=%v err=%v", tc.hand, allowed, err)
		}
	}
	now = now.Add(25 * time.Hour)
	for _, tc := range []struct {
		hand  string
		delta int64
	}{{"h4", 1}, {"h1", 90_000}} {
		allowed, err := svc.Record(context.Background(), "p", "t", tc.hand, tc.delta)
		if err != nil || !allowed || store.window.NetProfit != 1 {
			t.Fatalf("%s allowed=%v profit=%d err=%v", tc.hand, allowed, store.window.NetProfit, err)
		}
	}
}

func TestStaleWindowRetriesWithoutLosingConcurrentProfit(t *testing.T) {
	store := &memoryStore{}
	a, b := NewService(store, 100_000), NewService(store, 100_000)
	now := time.Unix(1_800_000_000, 0)
	a.now, b.now = func() time.Time { return now }, func() time.Time { return now }
	if _, err := a.Record(context.Background(), "p", "t", "h1", 40_000); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Eligibility(context.Background(), "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Record(context.Background(), "p", "t", "h2", 30_000); err != nil {
		t.Fatal(err)
	}
	allowed, err := b.Record(context.Background(), "p", "t", "h3", 30_000)
	if err != nil || allowed || store.window.NetProfit != 100_000 {
		t.Fatalf("allowed=%v profit=%d err=%v", allowed, store.window.NetProfit, err)
	}
}

func TestAmbiguousSuccessfulWriteDoesNotCountTwice(t *testing.T) {
	store := &memoryStore{loseReply: true}
	svc := NewService(store, 100_000)
	svc.now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	allowed, err := svc.Record(context.Background(), "p", "t", "h1", 60_000)
	if err != nil || !allowed || store.window.NetProfit != 60_000 {
		t.Fatalf("allowed=%v profit=%d err=%v", allowed, store.window.NetProfit, err)
	}
}

func TestUnavailableStoreFailsClosed(t *testing.T) {
	svc := NewService(&memoryStore{fail: errors.New("down")}, 100_000)
	if allowed, err := svc.Available(context.Background(), "p"); err == nil || allowed {
		t.Fatalf("available=%v err=%v", allowed, err)
	}
}
