package sandboxpurchase

import (
	"context"
	"errors"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

type fakeWallet struct {
	skus          []walletclient.SandboxSKU
	purchase      *walletclient.SandboxPurchase
	getResult     *walletclient.SandboxPurchase
	refundResult  *walletclient.SandboxPurchase
	purchaseCalls int
	lastIdemKey   string
}

func (f *fakeWallet) ListSandboxSKUs(context.Context) ([]walletclient.SandboxSKU, error) {
	return f.skus, nil
}
func (f *fakeWallet) PurchaseSandbox(_ context.Context, _ string, _ string, idemKey string) (*walletclient.SandboxPurchase, error) {
	f.purchaseCalls++
	f.lastIdemKey = idemKey
	return f.purchase, nil
}
func (f *fakeWallet) GetSandboxPurchase(context.Context, string) (*walletclient.SandboxPurchase, error) {
	return f.getResult, nil
}
func (f *fakeWallet) RefundSandboxPurchase(context.Context, string, string, string) (*walletclient.SandboxPurchase, error) {
	return f.refundResult, nil
}

type fakeStore struct {
	rows map[string]Record // keyed by playerID+"#"+purchaseID
}

func newFakeStore() *fakeStore               { return &fakeStore{rows: map[string]Record{}} }
func key(playerID, purchaseID string) string { return playerID + "#" + purchaseID }

func (f *fakeStore) Create(_ context.Context, rec Record) (Record, error) {
	k := key(rec.PlayerID, rec.PurchaseID)
	if existing, ok := f.rows[k]; ok {
		return existing, nil
	}
	f.rows[k] = rec
	return rec, nil
}
func (f *fakeStore) Get(_ context.Context, playerID, purchaseID string) (*Record, error) {
	rec, ok := f.rows[key(playerID, purchaseID)]
	if !ok {
		return nil, nil
	}
	return &rec, nil
}
func (f *fakeStore) UpdateStatus(_ context.Context, playerID, purchaseID, status, updatedAt string) (bool, error) {
	k := key(playerID, purchaseID)
	rec, ok := f.rows[k]
	if !ok {
		return false, nil
	}
	rec.Status, rec.UpdatedAt = status, updatedAt
	f.rows[k] = rec
	return true, nil
}
func (f *fakeStore) List(_ context.Context, playerID string) ([]Record, error) {
	var out []Record
	for _, rec := range f.rows {
		if rec.PlayerID == playerID {
			out = append(out, rec)
		}
	}
	return out, nil
}

func TestServiceCreatePersistsWithSKUBreakdown(t *testing.T) {
	wallet := &fakeWallet{
		skus:     []walletclient.SandboxSKU{{ID: "pack_100", PriceCents: 100, BaseCredits: 1000, BonusPercent: 10}},
		purchase: &walletclient.SandboxPurchase{PurchaseID: "sbxp-1", SKU: "pack_100", Amount: 100, CreditsGranted: 1100, Status: "pending", PixCopiaECola: "copia", QRCodeBase64: "qr", ExpiresAt: "2026-07-30T12:00:00Z"},
	}
	svc := NewService(wallet, newFakeStore())

	rec, err := svc.Create(context.Background(), "player-1", "pack_100", "k1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.BaseCredits != 1000 || rec.BonusPercent != 10 || rec.TotalCredits != 1100 || rec.Status != "pending" {
		t.Fatalf("unexpected record: %+v", rec)
	}
	if wallet.lastIdemKey != "k1" {
		t.Fatalf("expected idem key k1, got %q", wallet.lastIdemKey)
	}
}

func TestServiceCreateRejectsUnknownSKU(t *testing.T) {
	wallet := &fakeWallet{skus: []walletclient.SandboxSKU{{ID: "pack_100"}}}
	svc := NewService(wallet, newFakeStore())

	if _, err := svc.Create(context.Background(), "player-1", "not_a_real_sku", "k1"); err == nil {
		t.Fatal("expected an error for an unknown sku")
	}
	if wallet.purchaseCalls != 0 {
		t.Fatal("expected PurchaseSandbox not to be called for an unknown sku")
	}
}

func TestServiceRefreshUpdatesLocalStatusOnChange(t *testing.T) {
	store := newFakeStore()
	store.rows[key("player-1", "sbxp-1")] = Record{PlayerID: "player-1", PurchaseID: "sbxp-1", Status: "pending"}
	wallet := &fakeWallet{getResult: &walletclient.SandboxPurchase{Status: "confirmed"}}
	svc := NewService(wallet, store)

	rec, err := svc.Refresh(context.Background(), "player-1", "sbxp-1")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if rec.Status != "confirmed" {
		t.Fatalf("expected status confirmed, got %q", rec.Status)
	}
}

func TestServiceRefreshUnknownPurchaseReturnsErrNotFound(t *testing.T) {
	svc := NewService(&fakeWallet{}, newFakeStore())
	if _, err := svc.Refresh(context.Background(), "player-1", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestServiceConfirmFromWebhookBroadcastsOnlyOnChange(t *testing.T) {
	store := newFakeStore()
	store.rows[key("player-1", "sbxp-1")] = Record{PlayerID: "player-1", PurchaseID: "sbxp-1", Status: "pending"}
	wallet := &fakeWallet{getResult: &walletclient.SandboxPurchase{UserID: "player-1", Status: "confirmed"}}
	svc := NewService(wallet, store)

	rec, changed, err := svc.ConfirmFromWebhook(context.Background(), "sbxp-1")
	if err != nil {
		t.Fatalf("ConfirmFromWebhook: %v", err)
	}
	if !changed || rec.Status != "confirmed" {
		t.Fatalf("expected a change to confirmed, got changed=%v rec=%+v", changed, rec)
	}

	// Replay: wallet still reports confirmed, local is already confirmed — no-op.
	_, changedAgain, err := svc.ConfirmFromWebhook(context.Background(), "sbxp-1")
	if err != nil {
		t.Fatalf("ConfirmFromWebhook replay: %v", err)
	}
	if changedAgain {
		t.Fatal("expected replay to report no change")
	}
	_ = time.Now // keep time imported for readability of future assertions
}
