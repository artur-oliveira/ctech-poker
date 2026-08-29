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
