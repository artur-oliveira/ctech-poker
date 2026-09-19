package sandboxpurchase

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/promocode"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// fakePromo is an in-memory stand-in for promocode.Store: a set of
// already-claimed (playerID, key) pairs, exactly mirroring the real store's
// conditional-put-blocks-duplicate semantics without touching DynamoDB.
type fakePromo struct {
	codes     map[string]promocode.Code // code -> catalog entry
	claimed   map[string]bool           // playerID+"#"+key -> claimed
	redeemErr error
}

func newFakePromo() *fakePromo {
	return &fakePromo{codes: map[string]promocode.Code{}, claimed: map[string]bool{}}
}

func (f *fakePromo) Redeem(_ context.Context, playerID, code string) (*promocode.Code, error) {
	if f.redeemErr != nil {
		return nil, f.redeemErr
	}
	c, ok := f.codes[code]
	if !ok {
		return nil, promocode.ErrNotFound
	}
	k := playerID + "#" + code
	if f.claimed[k] {
		return nil, promocode.ErrAlreadyRedeemed
	}
	f.claimed[k] = true
	return &c, nil
}
func (f *fakePromo) ReleaseRedemption(_ context.Context, playerID, code string) {
	delete(f.claimed, playerID+"#"+code)
}
func (f *fakePromo) ClaimOnce(_ context.Context, playerID, sku string) error {
	k := playerID + "#" + sku
	if f.claimed[k] {
		return promocode.ErrAlreadyRedeemed
	}
	f.claimed[k] = true
	return nil
}
func (f *fakePromo) ReleaseClaim(_ context.Context, playerID, sku string) {
	delete(f.claimed, playerID+"#"+sku)
}

type fakeWallet struct {
	skus          []walletclient.SandboxSKU
	purchase      *walletclient.SandboxPurchase
	purchaseErr   error
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
	if f.purchaseErr != nil {
		return nil, f.purchaseErr
	}
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
func (f *fakeStore) List(_ context.Context, playerID string, _ int, _ map[string]types.AttributeValue) ([]Record, map[string]types.AttributeValue, error) {
	var out []Record
	for _, rec := range f.rows {
		if rec.PlayerID == playerID {
			out = append(out, rec)
		}
	}
	return out, nil, nil
}

func TestServiceCreatePersistsWithSKUBreakdown(t *testing.T) {
	wallet := &fakeWallet{
		skus:     []walletclient.SandboxSKU{{ID: "pack_100", PriceCents: 100, BaseCredits: 1000, BonusPercent: 10}},
		purchase: &walletclient.SandboxPurchase{PurchaseID: "sbxp-1", SKU: "pack_100", Amount: 100, CreditsGranted: 1100, Status: "pending", PixCopiaECola: "copia", QRCodeBase64: "qr", ExpiresAt: "2026-07-30T12:00:00Z"},
	}
	svc := NewService(wallet, newFakeStore())

	rec, err := svc.Create(context.Background(), "player-1", "pack_100", "", "k1")
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

	if _, err := svc.Create(context.Background(), "player-1", "not_a_real_sku", "", "k1"); err == nil {
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

func TestServiceCreateWithPromoCodeResolvesWalletSKUFromCode(t *testing.T) {
	wallet := &fakeWallet{
		skus:     []walletclient.SandboxSKU{{ID: "pack_promo", PriceCents: 50, BaseCredits: 1000, BonusPercent: 50}},
		purchase: &walletclient.SandboxPurchase{PurchaseID: "sbxp-2", SKU: "pack_promo", CreditsGranted: 1500, Status: "pending"},
	}
	promo := newFakePromo()
	promo.codes["WELCOME50"] = promocode.Code{Code: "WELCOME50", SKU: "pack_promo"}
	svc := NewService(wallet, newFakeStore()).WithPromo(promo)

	// The client's own sku is ignored/overridden by the promo's target SKU —
	// it must never be able to combine a cheaper client-chosen sku with a
	// promo's discount.
	rec, err := svc.Create(context.Background(), "player-1", "irrelevant_sku", "WELCOME50", "k1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.PromoCode != "WELCOME50" || rec.SKU != "pack_promo" {
		t.Fatalf("unexpected record: %+v", rec)
	}
}

func TestServiceCreateRejectsDuplicatePromoRedemption(t *testing.T) {
	wallet := &fakeWallet{
		skus:     []walletclient.SandboxSKU{{ID: "pack_promo"}},
		purchase: &walletclient.SandboxPurchase{PurchaseID: "sbxp-3", SKU: "pack_promo", Status: "pending"},
	}
	promo := newFakePromo()
	promo.codes["ONECODE"] = promocode.Code{Code: "ONECODE", SKU: "pack_promo"}
	svc := NewService(wallet, newFakeStore()).WithPromo(promo)

	if _, err := svc.Create(context.Background(), "player-1", "", "ONECODE", "k1"); err != nil {
		t.Fatalf("first redemption: %v", err)
	}
	// Same player, same code, a second time (a different purchase attempt,
	// not a retry with the same idem key) — must be rejected.
	if _, err := svc.Create(context.Background(), "player-1", "", "ONECODE", "k2"); !errors.Is(err, promocode.ErrAlreadyRedeemed) {
		t.Fatalf("expected ErrAlreadyRedeemed, got %v", err)
	}
	if wallet.purchaseCalls != 1 {
		t.Fatalf("expected wallet to be charged exactly once, got %d calls", wallet.purchaseCalls)
	}
}

func TestServiceCreateRejectsExpiredPromoCode(t *testing.T) {
	wallet := &fakeWallet{skus: []walletclient.SandboxSKU{{ID: "pack_promo"}}}
	promo := newFakePromo()
	promo.redeemErr = promocode.ErrExpired
	svc := NewService(wallet, newFakeStore()).WithPromo(promo)

	if _, err := svc.Create(context.Background(), "player-1", "", "EXPIRED", "k1"); !errors.Is(err, promocode.ErrExpired) {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
	if wallet.purchaseCalls != 0 {
		t.Fatal("expected the wallet to never be charged for an expired code")
	}
}

func TestServiceCreateWelcomePackOncePerPlayer(t *testing.T) {
	wallet := &fakeWallet{
		skus:     []walletclient.SandboxSKU{{ID: WelcomePackSKU, PriceCents: 1, BaseCredits: 500}},
		purchase: &walletclient.SandboxPurchase{PurchaseID: "sbxp-4", SKU: WelcomePackSKU, Status: "pending"},
	}
	promo := newFakePromo()
	svc := NewService(wallet, newFakeStore()).WithPromo(promo)

	if _, err := svc.Create(context.Background(), "player-1", WelcomePackSKU, "", "k1"); err != nil {
		t.Fatalf("first welcome pack purchase: %v", err)
	}
	if _, err := svc.Create(context.Background(), "player-1", WelcomePackSKU, "", "k2"); !errors.Is(err, promocode.ErrAlreadyRedeemed) {
		t.Fatalf("expected a second welcome-pack purchase to be rejected, got %v", err)
	}
	if wallet.purchaseCalls != 1 {
		t.Fatalf("expected exactly one wallet charge, got %d", wallet.purchaseCalls)
	}

	// A different player is still eligible.
	if _, err := svc.Create(context.Background(), "player-2", WelcomePackSKU, "", "k3"); err != nil {
		t.Fatalf("second player's welcome pack purchase: %v", err)
	}
}

func TestServiceCreateReleasesClaimWhenWalletPurchaseFails(t *testing.T) {
	wallet := &fakeWallet{skus: []walletclient.SandboxSKU{{ID: WelcomePackSKU}}, purchaseErr: errors.New("wallet down")}
	promo := newFakePromo()
	svc := NewService(wallet, newFakeStore()).WithPromo(promo)

	if _, err := svc.Create(context.Background(), "player-1", WelcomePackSKU, "", "k1"); err == nil {
		t.Fatal("expected the wallet failure to propagate")
	}
	// The claim must have been released — a retry should be allowed to try
	// again, not permanently blocked by the failed attempt.
	wallet.purchaseErr = nil
	wallet.purchase = &walletclient.SandboxPurchase{PurchaseID: "sbxp-5", SKU: WelcomePackSKU, Status: "pending"}
	if _, err := svc.Create(context.Background(), "player-1", WelcomePackSKU, "", "k2"); err != nil {
		t.Fatalf("expected retry after release to succeed, got %v", err)
	}
}
