package handhook

import (
	"context"
	"testing"
)

// Without a Valkey there is one instance, so it is also the whole fleet: the
// Actor's own completedHandNotified is a sufficient guard and every claim must
// be granted. Refusing here would silently stop crediting hands in dev.
func TestClaimIsGrantedWithoutASharedCache(t *testing.T) {
	ctx := context.Background()
	for name, svc := range map[string]*Service{
		"nil service": nil,
		"nil client":  NewService(nil),
	} {
		claimed, err := svc.Claim(ctx, "table-1", "hand-1")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !claimed {
			t.Fatalf("%s: claim refused, want granted", name)
		}
	}
}

// A hand with no ID is a table that never dealt one. There is nothing to
// deduplicate against, and notifyHandComplete already refuses it upstream.
func TestClaimIsGrantedForAnEmptyHandID(t *testing.T) {
	claimed, err := NewService(nil).Claim(context.Background(), "table-1", "")
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v; want true, nil", claimed, err)
	}
}

func TestKeyNamespacesTableAndHand(t *testing.T) {
	if got := key("table-1", "hand-1"); got != "poker:handhook:table-1:hand-1" {
		t.Fatalf("key = %q", got)
	}
	// Two hands on one table, and one hand ID across two tables, must never
	// collide — either would silently drop a hand's credits.
	if key("t1", "h1") == key("t1", "h2") || key("t1", "h1") == key("t2", "h1") {
		t.Fatal("keys collide")
	}
}
