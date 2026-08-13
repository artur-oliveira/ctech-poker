//go:build integration

package reactionpurchase

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

func TestEntitlementStorePutGetMarkUsedDelete(t *testing.T) {
	s := newTestEntitlementStore(t)
	ctx := context.Background()

	e := Entitlement{PlayerID: "player-1", ReactionID: "cold", PurchaseMethod: "fichas", PurchaseID: "rp-1", CreatedAt: "2026-08-12T00:00:00Z"}
	if err := s.Put(ctx, e); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get(ctx, "player-1", "cold")
	if err != nil || got == nil || got.UsedAt != "" {
		t.Fatalf("Get: %v, %+v", err, got)
	}
	if err := s.MarkUsed(ctx, "player-1", "cold"); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	got, err = s.Get(ctx, "player-1", "cold")
	if err != nil || got.UsedAt == "" {
		t.Fatalf("expected UsedAt set after MarkUsed, got %+v (err=%v)", got, err)
	}
	// First-use-wins: a second MarkUsed must not error and must not clobber the first timestamp.
	firstUsedAt := got.UsedAt
	if err := s.MarkUsed(ctx, "player-1", "cold"); err != nil {
		t.Fatalf("second MarkUsed: %v", err)
	}
	got, _ = s.Get(ctx, "player-1", "cold")
	if got.UsedAt != firstUsedAt {
		t.Fatalf("MarkUsed must be idempotent: got %q, want %q", got.UsedAt, firstUsedAt)
	}
	if err := s.Delete(ctx, "player-1", "cold"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err = s.Get(ctx, "player-1", "cold")
	if err != nil || got != nil {
		t.Fatalf("expected nil after Delete, got %+v (err=%v)", got, err)
	}
	if err := s.MarkUsed(ctx, "player-1", "cold"); !errors.Is(err, ErrMissingEntitlement) {
		t.Fatalf("MarkUsed after entitlement revocation = %v, want ErrMissingEntitlement", err)
	}
}

func TestStoreCreateIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rec := Record{PlayerID: "player-1", PurchaseID: "rp-2", ReactionID: "cold", Method: "fichas", PriceFichas: 100_000, Status: "confirmed", CreatedAt: "2026-08-12T00:00:00Z", UpdatedAt: "2026-08-12T00:00:00Z"}
	got1, err := s.Create(ctx, rec)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	got2, err := s.Create(ctx, rec)
	if err != nil || got2.PurchaseID != got1.PurchaseID {
		t.Fatalf("replay Create must return the existing row: %v, %+v", err, got2)
	}
}

func TestBuildMarkUsedTxItemIsIdempotentAndRequiresActiveEntitlement(t *testing.T) {
	s := newTestEntitlementStore(t)
	ctx := context.Background()
	e := Entitlement{
		PlayerID: "player-atomic-use", ReactionID: "cold", PurchaseMethod: methodFichas,
		PurchaseID: "rpf-atomic-use", Status: statusActive, CreatedAt: "2026-08-12T00:00:00Z",
	}
	if err := s.Put(ctx, e); err != nil {
		t.Fatalf("Put: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.BuildMarkUsedTxItem(e.PlayerID, e.ReactionID)}); err != nil {
			t.Fatalf("mark-used transaction %d: %v", i+1, err)
		}
	}
	if err := s.Delete(ctx, e.PlayerID, e.ReactionID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.BuildMarkUsedTxItem(e.PlayerID, e.ReactionID)}); !dynamo.IsConditionFailed(err) {
		t.Fatalf("mark-used transaction after refund = %v, want condition failure", err)
	}
}
