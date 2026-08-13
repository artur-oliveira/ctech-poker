//go:build integration

package reactionpurchase

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

type fakeWallet struct {
	skus      []walletclient.ProductSKU
	purchase  *walletclient.ProductPurchase
	getResult *walletclient.ProductPurchase
}

func (f *fakeWallet) ListProductSKUs(context.Context) ([]walletclient.ProductSKU, error) {
	return f.skus, nil
}
func (f *fakeWallet) PurchaseProduct(_ context.Context, _, _, _ string) (*walletclient.ProductPurchase, error) {
	return f.purchase, nil
}
func (f *fakeWallet) GetProductPurchase(context.Context, string) (*walletclient.ProductPurchase, error) {
	return f.getResult, nil
}
func (f *fakeWallet) RefundProductPurchase(context.Context, string, string, string) (*walletclient.ProductPurchase, error) {
	return nil, nil
}
func (f *fakeWallet) Debit(context.Context, string, int64, string, string) error  { return nil }
func (f *fakeWallet) Credit(context.Context, string, int64, string, string) error { return nil }

// Each test below uses its own playerID — the underlying tables are fixed
// names shared across every test run against the same DynamoDB Local
// container (mirrors sessionlog's own integration-test convention), so a
// shared "player-1"/"cold" key across tests would leak entitlements between
// them and between reruns.

func TestListCatalogMergesFreeAndPremium(t *testing.T) {
	w := &fakeWallet{skus: []walletclient.ProductSKU{{ID: "poker_reaction_cold", PriceCents: 100}}}
	svc := NewService(w, newTestEntitlementStore(t), newTestStore(t))
	entries, err := svc.ListCatalog(context.Background())
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.ID == "cold" {
			found = true
			if !e.Premium || e.PriceCents != 100 || e.PriceFichas != 100_000 {
				t.Fatalf("unexpected cold entry: %+v", e)
			}
		}
	}
	if !found {
		t.Fatal("expected cold in the merged catalog")
	}
}

func TestCreateRealUnknownReactionRejected(t *testing.T) {
	svc := NewService(&fakeWallet{}, newTestEntitlementStore(t), newTestStore(t))
	_, _, err := svc.CreateReal(context.Background(), "player-unknown-reaction", "not-a-reaction", "idem-1")
	if err != ErrUnknownReaction {
		t.Fatalf("expected ErrUnknownReaction, got %v", err)
	}
}

func TestCreateRealFreeReactionRejected(t *testing.T) {
	svc := NewService(&fakeWallet{}, newTestEntitlementStore(t), newTestStore(t))
	_, _, err := svc.CreateReal(context.Background(), "player-free-reaction", "clap", "idem-1")
	if err != ErrNotPremium {
		t.Fatalf("expected ErrNotPremium, got %v", err)
	}
}

func TestCreateRealOpensNoEntitlementUntilConfirmed(t *testing.T) {
	w := &fakeWallet{purchase: &walletclient.ProductPurchase{PurchaseID: "prdp-open-1", SKU: "poker_reaction_cold", Amount: 100, Status: "pending"}}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(w, entitlements, store)
	rec, _, err := svc.CreateReal(context.Background(), "player-open", "cold", "idem-open-1")
	if err != nil {
		t.Fatalf("CreateReal: %v", err)
	}
	if rec.Status != "pending" || rec.Method != "pix" {
		t.Fatalf("unexpected record: %+v", rec)
	}
	got, err := entitlements.Get(context.Background(), "player-open", "cold")
	if err != nil || got != nil {
		t.Fatalf("no entitlement must exist until webhook confirms, got %+v (err=%v)", got, err)
	}
}

func TestCreateSandboxGrantsEntitlementSynchronously(t *testing.T) {
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(&fakeWallet{}, entitlements, store)
	rec, err := svc.CreateSandbox(context.Background(), "player-sandbox-sync", "cold", "idem-sandbox-3")
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if rec.Status != "confirmed" || rec.Method != "fichas" || rec.PriceFichas != 100_000 {
		t.Fatalf("unexpected record: %+v", rec)
	}
	owned, err := svc.IsOwned(context.Background(), "player-sandbox-sync", "cold")
	if err != nil || !owned {
		t.Fatalf("expected owned=true immediately, got %v (err=%v)", owned, err)
	}
}

func TestRefundRejectedWhenUsed(t *testing.T) {
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(&fakeWallet{}, entitlements, store)
	rec, err := svc.CreateSandbox(context.Background(), "player-refund-used", "cold", "idem-sandbox-4")
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if err := svc.MarkUsed(context.Background(), "player-refund-used", "cold"); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	if _, err := svc.Refund(context.Background(), "player-refund-used", rec.PurchaseID, "idem-refund-1"); err != ErrAlreadyUsed {
		t.Fatalf("expected ErrAlreadyUsed, got %v", err)
	}
}

func TestRefundSandboxHappyPathRevokesEntitlement(t *testing.T) {
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(&fakeWallet{}, entitlements, store)
	rec, err := svc.CreateSandbox(context.Background(), "player-refund-happy", "cold", "idem-sandbox-5")
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	refunded, err := svc.Refund(context.Background(), "player-refund-happy", rec.PurchaseID, "idem-refund-2")
	if err != nil || refunded.Status != "refunded" {
		t.Fatalf("Refund: %v, %+v", err, refunded)
	}
	owned, err := svc.IsOwned(context.Background(), "player-refund-happy", "cold")
	if err != nil || owned {
		t.Fatalf("expected owned=false after refund, got %v (err=%v)", owned, err)
	}
}

func TestMarkUsedNoOpForFreeReaction(t *testing.T) {
	svc := NewService(&fakeWallet{}, newTestEntitlementStore(t), newTestStore(t))
	if err := svc.MarkUsed(context.Background(), "player-markused-free", "clap"); err != nil {
		t.Fatalf("MarkUsed on a free reaction must be a no-op, got %v", err)
	}
}

func TestConfirmFromWebhookGrantsEntitlement(t *testing.T) {
	w := &fakeWallet{
		purchase:  &walletclient.ProductPurchase{PurchaseID: "prdp-confirm-2", SKU: "poker_reaction_cold", Amount: 100, Status: "pending"},
		getResult: &walletclient.ProductPurchase{PurchaseID: "prdp-confirm-2", UserID: "player-confirm", SKU: "poker_reaction_cold", Amount: 100, Status: "confirmed"},
	}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(w, entitlements, store)
	if _, _, err := svc.CreateReal(context.Background(), "player-confirm", "cold", "idem-confirm-2"); err != nil {
		t.Fatalf("CreateReal: %v", err)
	}
	rec, changed, err := svc.ConfirmFromWebhook(context.Background(), "prdp-confirm-2")
	if err != nil || !changed || rec.Status != "confirmed" {
		t.Fatalf("ConfirmFromWebhook: %v, changed=%v, rec=%+v", err, changed, rec)
	}
	got, err := entitlements.Get(context.Background(), "player-confirm", "cold")
	if err != nil || got == nil {
		t.Fatalf("expected entitlement after confirm, got %+v (err=%v)", got, err)
	}
}
