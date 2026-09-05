package presence

import (
	"context"
	"testing"
	"time"
)

type fakeFriends map[string][]string

func (f fakeFriends) FriendIDs(_ context.Context, id string) ([]string, error) { return f[id], nil }

type fakeSessions string

func (f fakeSessions) FindLatestOpenSession(context.Context, string) (string, error) {
	return string(f), nil
}

func TestRoomIDSurvivesSetAndReconcile(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, fakeFriends{}, fakeSessions("room-9"), func(context.Context, string, string, Status) {})
	ctx := context.Background()
	if err := svc.Open(ctx, "p1", "c1"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetMany(ctx, []string{"p1"})
	if err != nil {
		t.Fatal(err)
	}
	if got["p1"].Status != StatusInTable || got["p1"].RoomID != "room-9" {
		t.Fatalf("want in_table at room-9, got %+v", got["p1"])
	}
	if err := svc.SetInTable(ctx, "p1", ""); err != nil {
		t.Fatal(err)
	}
	got, err = svc.GetMany(ctx, []string{"p1"})
	if err != nil {
		t.Fatal(err)
	}
	if got["p1"].Status != StatusOnline || got["p1"].RoomID != "" {
		t.Fatalf("want online with no room, got %+v", got["p1"])
	}
}

func TestMultipleConnectionsAndOutOfOrderClose(t *testing.T) {
	store := NewMemoryStore()
	now := time.Unix(100, 0)
	store.now = func() time.Time { return now }
	var notifications []Status
	svc := NewService(store, fakeFriends{"p1": {"friend"}}, fakeSessions(""), func(_ context.Context, recipient, player string, status Status) {
		if recipient != "friend" || player != "p1" {
			t.Fatalf("unexpected fanout %s %s", recipient, player)
		}
		notifications = append(notifications, status)
	})
	svc.now = func() time.Time { return now }

	if err := svc.Open(context.Background(), "p1", "c1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Open(context.Background(), "p1", "c2"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(context.Background(), "p1", "c1"); err != nil {
		t.Fatal(err)
	}
	status, _ := svc.GetMany(context.Background(), []string{"p1"})
	if status["p1"].Status != StatusOnline {
		t.Fatalf("got %+v", status["p1"])
	}
	if err := svc.Close(context.Background(), "p1", "c2"); err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 2 || notifications[0] != StatusOnline || notifications[1] != StatusOffline {
		t.Fatalf("unexpected transitions: %v", notifications)
	}
}

func TestExpiryAndSessionReconciliation(t *testing.T) {
	store := NewMemoryStore()
	now := time.Unix(100, 0)
	store.now = func() time.Time { return now }
	svc := NewService(store, nil, fakeSessions("room-1"), nil)
	svc.now = func() time.Time { return now }
	if err := svc.Open(context.Background(), "p1", "c1"); err != nil {
		t.Fatal(err)
	}
	status, _ := svc.GetMany(context.Background(), []string{"p1"})
	if status["p1"].Status != StatusInTable {
		t.Fatalf("got %+v", status["p1"])
	}
	now = now.Add(ConnectionTTL + time.Second)
	status, _ = svc.GetMany(context.Background(), []string{"p1"})
	if status["p1"].Status != StatusOffline {
		t.Fatalf("expired connection got %+v", status["p1"])
	}
}

// hangingStore never answers until the caller's context expires — an
// unreachable Valkey, from the WebSocket lifecycle's point of view.
type hangingStore struct{}

func (hangingStore) Open(ctx context.Context, _, _ string, _ time.Time) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

func (hangingStore) Heartbeat(ctx context.Context, _, _ string, _ time.Time) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

func (hangingStore) Close(ctx context.Context, _, _ string) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

func (hangingStore) SetInTable(ctx context.Context, _, _ string) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

func (hangingStore) GetMany(ctx context.Context, _ []string) (map[string]PlayerPresence, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestLifecycleOperationsAreBounded pins #223's presence half: Open and
// Heartbeat run under the socket's own context (alive as long as the
// connection) and Close under context.Background(), so without a budget of
// their own an unreachable store parks the connect path, a heartbeat tick or a
// socket teardown indefinitely. Each must fail fast instead — presence has its
// own TTL, so a dropped update self-heals.
func TestLifecycleOperationsAreBounded(t *testing.T) {
	svc := NewService(hangingStore{}, fakeFriends{}, nil, func(context.Context, string, string, Status) {})
	svc.opBudget = 30 * time.Millisecond
	svc.openBudget = 30 * time.Millisecond

	for name, call := range map[string]func() error{
		"Open":      func() error { return svc.Open(context.Background(), "p1", "c1") },
		"Heartbeat": func() error { return svc.Heartbeat(context.Background(), "p1", "c1") },
		"Close":     func() error { return svc.Close(context.Background(), "p1", "c1") },
	} {
		started := time.Now()
		err := call()
		elapsed := time.Since(started)
		if err == nil {
			t.Fatalf("%s: want an error once the budget expires", name)
		}
		if elapsed > 2*time.Second {
			t.Fatalf("%s took %v, want it bounded by its own budget", name, elapsed)
		}
	}
}
