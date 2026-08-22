//go:build integration

package cosmeticpurchase

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/cosmetics"
)

func TestEntitlementStorePutGetDelete(t *testing.T) {
	s := newTestEntitlementStore(t)
	ctx := context.Background()

	e := Entitlement{PlayerID: "player-1", Kind: string(cosmetics.KindDeck), ItemID: "golden", PurchaseMethod: "fichas", PurchaseID: "cp-1", CreatedAt: "2026-08-21T00:00:00Z"}
	if err := s.Put(ctx, e); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get(ctx, "player-1", cosmetics.KindDeck, "golden")
	if err != nil || got == nil || got.ItemID != "golden" {
		t.Fatalf("Get: %v, %+v", err, got)
	}
	// A felt entitlement with the same item id must not collide with the deck one.
	if got, err := s.Get(ctx, "player-1", cosmetics.KindFelt, "golden"); err != nil || got != nil {
		t.Fatalf("cross-kind lookup must not find the deck entitlement: %+v (err=%v)", got, err)
	}
	if err := s.Delete(ctx, "player-1", cosmetics.KindDeck, "golden"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err = s.Get(ctx, "player-1", cosmetics.KindDeck, "golden")
	if err != nil || got != nil {
		t.Fatalf("expected nil after Delete, got %+v (err=%v)", got, err)
	}
}

func TestStoreCreateIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rec := Record{PlayerID: "player-1", PurchaseID: "cp-2", Kind: string(cosmetics.KindFelt), ItemID: "midnight", Method: "fichas", PriceFichas: 200_000, Status: "confirmed", CreatedAt: "2026-08-21T00:00:00Z", UpdatedAt: "2026-08-21T00:00:00Z"}
	got1, err := s.Create(ctx, rec)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	got2, err := s.Create(ctx, rec)
	if err != nil || got2.PurchaseID != got1.PurchaseID {
		t.Fatalf("replay Create must return the existing row: %v, %+v", err, got2)
	}
}
