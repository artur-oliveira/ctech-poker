package presence

import (
	"context"
	"testing"
	"time"
)

type fakeFriends map[string][]string

func (f fakeFriends) FriendIDs(_ context.Context, id string) ([]string, error) { return f[id], nil }

type fakeSessions bool

func (f fakeSessions) FindLatestOpenSession(context.Context, string) (bool, error) {
	return bool(f), nil
}

func TestMultipleConnectionsAndOutOfOrderClose(t *testing.T) {
	store := NewMemoryStore()
	now := time.Unix(100, 0)
	store.now = func() time.Time { return now }
	var notifications []Status
	svc := NewService(store, fakeFriends{"p1": {"friend"}}, fakeSessions(false), func(_ context.Context, recipient, player string, status Status) {
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
	if status["p1"] != StatusOnline {
		t.Fatalf("got %s", status["p1"])
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
	svc := NewService(store, nil, fakeSessions(true), nil)
	svc.now = func() time.Time { return now }
	if err := svc.Open(context.Background(), "p1", "c1"); err != nil {
		t.Fatal(err)
	}
	status, _ := svc.GetMany(context.Background(), []string{"p1"})
	if status["p1"] != StatusInTable {
		t.Fatalf("got %s", status["p1"])
	}
	now = now.Add(ConnectionTTL + time.Second)
	status, _ = svc.GetMany(context.Background(), []string{"p1"})
	if status["p1"] != StatusOffline {
		t.Fatalf("expired connection got %s", status["p1"])
	}
}
