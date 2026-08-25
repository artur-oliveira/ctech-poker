//go:build integration

package reactionpurchase

import (
	"context"
	"errors"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

type fakeWallet struct {
	skus         []walletclient.ProductSKU
	purchase     *walletclient.ProductPurchase
	purchaseErr  error
	getResult    *walletclient.ProductPurchase
	debitErr     error
	creditErr    error
	debits       int
	credits      int
	refunds      int
	creditKeys   []string
	refundKeys   []string
	purchaseKeys []string
	debitKeys    []string
}

func allPremiumSKUs() []walletclient.ProductSKU {
	return []walletclient.ProductSKU{
		{ID: "poker_reaction_cold", PriceCents: 100},
		{ID: "poker_reaction_fire", PriceCents: 100},
		{ID: "poker_reaction_poop", PriceCents: 500},
		{ID: "poker_reaction_rofl", PriceCents: 500},
		{ID: "poker_reaction_knife", PriceCents: 500},
		{ID: "poker_reaction_turtle", PriceCents: 500},
	}
}

func (f *fakeWallet) ListProductSKUs(context.Context) ([]walletclient.ProductSKU, error) {
	return f.skus, nil
}
func (f *fakeWallet) PurchaseProduct(_ context.Context, _, _, key string) (*walletclient.ProductPurchase, error) {
	f.purchaseKeys = append(f.purchaseKeys, key)
	return f.purchase, f.purchaseErr
}
func (f *fakeWallet) GetProductPurchase(context.Context, string) (*walletclient.ProductPurchase, error) {
	return f.getResult, nil
}
func (f *fakeWallet) RefundProductPurchase(_ context.Context, _, _ string, key string) (*walletclient.ProductPurchase, error) {
	f.refunds++
	f.refundKeys = append(f.refundKeys, key)
	return &walletclient.ProductPurchase{Status: "refunded"}, nil
}
func (f *fakeWallet) Debit(_ context.Context, _ string, _ int64, key, _ string) error {
	f.debits++
	f.debitKeys = append(f.debitKeys, key)
	return f.debitErr
}
func (f *fakeWallet) Credit(_ context.Context, _ string, _ int64, key, _ string) error {
	f.credits++
	f.creditKeys = append(f.creditKeys, key)
	return f.creditErr
}

// Each test below uses its own playerID — the underlying tables are fixed
// names shared across every test run against the same DynamoDB Local
// container (mirrors sessionlog's own integration-test convention), so a
// shared "player-1"/"cold" key across tests would leak entitlements between
// them and between reruns.

func TestListCatalogMergesFreeAndPremium(t *testing.T) {
	w := &fakeWallet{skus: allPremiumSKUs()}
	svc := NewService(w, newTestEntitlementStore(t), newTestStore(t))
	entries, err := svc.ListCatalog(context.Background(), "player-1")
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

func TestCreateRealReservesWithoutGrantingUntilConfirmed(t *testing.T) {
	w := &fakeWallet{purchase: &walletclient.ProductPurchase{
		PurchaseID: "prdp-open-1", SKU: "poker_reaction_cold", Amount: 100, Status: "pending",
		PixCopiaECola: "000201-pix", QRCodeBase64: "base64-qr", ExpiresAt: "2026-08-13T00:00:00Z",
	}}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(w, entitlements, store)
	rec, _, err := svc.CreateReal(context.Background(), "player-open", "cold", "idem-open-1")
	if err != nil {
		t.Fatalf("CreateReal: %v", err)
	}
	if rec.Status != "pending" || rec.Method != "pix" {
		t.Fatalf("unexpected record: %+v", rec)
	}
	if rec.PixCopiaECola != "000201-pix" || rec.QRCodeBase64 != "base64-qr" || rec.ExpiresAt == "" {
		t.Fatalf("PIX payment payload was not preserved: %+v", rec)
	}
	got, err := entitlements.Get(context.Background(), "player-open", "cold")
	if err != nil || got == nil || got.Status != "pending" {
		t.Fatalf("expected a non-owned pending reservation, got %+v (err=%v)", got, err)
	}
	owned, err := svc.IsOwned(context.Background(), "player-open", "cold")
	if err != nil || owned {
		t.Fatalf("pending reservation must not grant ownership, owned=%v err=%v", owned, err)
	}
}

func TestCreateRealImmediateConfirmationActivatesOwnership(t *testing.T) {
	wallet := &fakeWallet{purchase: &walletclient.ProductPurchase{
		PurchaseID: "prdp-immediate-confirm", SKU: "poker_reaction_cold", Amount: 100, Status: "confirmed",
	}}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)
	rec, _, err := svc.CreateReal(context.Background(), "player-immediate-confirm", "cold", "idem-immediate-confirm")
	if err != nil || rec.Status != "confirmed" {
		t.Fatalf("CreateReal: rec=%+v err=%v", rec, err)
	}
	if owned, err := svc.IsOwned(context.Background(), "player-immediate-confirm", "cold"); err != nil || !owned {
		t.Fatalf("immediately confirmed PIX must grant ownership, owned=%v err=%v", owned, err)
	}
}

func TestCreateSandboxDefinitiveWalletRejectionLeavesNoRows(t *testing.T) {
	wallet := &fakeWallet{debitErr: &walletclient.Error{Status: 409, Title: "Insufficient balance"}}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)
	_, err := svc.CreateSandbox(context.Background(), "player-insufficient", "cold", "idem-insufficient")
	if err == nil {
		t.Fatal("expected insufficient-balance error")
	}
	if ent, getErr := entitlements.Get(context.Background(), "player-insufficient", "cold"); getErr != nil || ent != nil {
		t.Fatalf("definitive rejection left entitlement %+v (err=%v)", ent, getErr)
	}
	purchaseID := sandboxPurchaseID(purchaseRequestKey("player-insufficient", "cold", "fichas", "idem-insufficient"))
	if rec, getErr := store.Get(context.Background(), "player-insufficient", purchaseID); getErr != nil || rec != nil {
		t.Fatalf("definitive rejection left record %+v (err=%v)", rec, getErr)
	}
}

func TestCreateSandboxNewRequestResumesAmbiguousDebitWithOriginalKey(t *testing.T) {
	wallet := &fakeWallet{debitErr: errors.New("transport timeout")}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)
	if _, err := svc.CreateSandbox(context.Background(), "player-resume-fichas", "cold", "original-key"); err == nil {
		t.Fatal("first CreateSandbox must surface the ambiguous timeout")
	}
	wallet.debitErr = nil
	rec, err := svc.CreateSandbox(context.Background(), "player-resume-fichas", "cold", "new-generated-key")
	if err != nil || rec.Status != "confirmed" {
		t.Fatalf("resumed CreateSandbox: rec=%+v err=%v", rec, err)
	}
	if len(wallet.debitKeys) != 2 || wallet.debitKeys[0] != "original-key" || wallet.debitKeys[1] != "original-key" {
		t.Fatalf("ambiguous debit was not resumed with its original key: %v", wallet.debitKeys)
	}
}

func TestCreateRealNewRequestResumesAmbiguousPurchaseWithOriginalKey(t *testing.T) {
	wallet := &fakeWallet{purchaseErr: errors.New("transport timeout")}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)
	if _, _, err := svc.CreateReal(context.Background(), "player-resume-pix", "cold", "original-pix-key"); err == nil {
		t.Fatal("first CreateReal must surface the ambiguous timeout")
	}
	wallet.purchaseErr = nil
	wallet.purchase = &walletclient.ProductPurchase{PurchaseID: "prdp-resume-pix", SKU: "poker_reaction_cold", Amount: 100, Status: "pending"}
	rec, _, err := svc.CreateReal(context.Background(), "player-resume-pix", "cold", "new-generated-pix-key")
	if err != nil || rec.PurchaseID != "prdp-resume-pix" {
		t.Fatalf("resumed CreateReal: rec=%+v err=%v", rec, err)
	}
	if len(wallet.purchaseKeys) != 2 || wallet.purchaseKeys[0] != "original-pix-key" || wallet.purchaseKeys[1] != "original-pix-key" {
		t.Fatalf("ambiguous PIX purchase was not resumed with its original key: %v", wallet.purchaseKeys)
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

func TestRefreshReverifiesPendingPIXAndGrantsEntitlement(t *testing.T) {
	wallet := &fakeWallet{
		purchase:  &walletclient.ProductPurchase{PurchaseID: "prdp-refresh-1", SKU: "poker_reaction_cold", Amount: 100, Status: "pending"},
		getResult: &walletclient.ProductPurchase{PurchaseID: "prdp-refresh-1", UserID: "player-refresh", SKU: "poker_reaction_cold", Amount: 100, Status: "confirmed"},
	}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)
	if _, _, err := svc.CreateReal(context.Background(), "player-refresh", "cold", "idem-refresh"); err != nil {
		t.Fatalf("CreateReal: %v", err)
	}
	rec, err := svc.Refresh(context.Background(), "player-refresh", "prdp-refresh-1")
	if err != nil || rec.Status != "confirmed" {
		t.Fatalf("Refresh: rec=%+v err=%v", rec, err)
	}
	if owned, err := svc.IsOwned(context.Background(), "player-refresh", "cold"); err != nil || !owned {
		t.Fatalf("Refresh must reconcile ownership, owned=%v err=%v", owned, err)
	}
}

func TestRefreshReconstructsMissingPendingPIXRecord(t *testing.T) {
	wallet := &fakeWallet{getResult: &walletclient.ProductPurchase{
		PurchaseID: "prdp-refresh-repair", UserID: "player-refresh-repair", SKU: "poker_reaction_cold",
		Amount: 100, Status: "pending", PixCopiaECola: "000201-repair", QRCodeBase64: "repair-qr",
	}}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)
	rec, err := svc.Refresh(context.Background(), "player-refresh-repair", "prdp-refresh-repair")
	if err != nil || rec.Status != "pending" || rec.PixCopiaECola != "000201-repair" {
		t.Fatalf("Refresh repair: rec=%+v err=%v", rec, err)
	}
	stored, err := store.Get(context.Background(), "player-refresh-repair", "prdp-refresh-repair")
	if err != nil || stored == nil || stored.QRCodeBase64 != "repair-qr" {
		t.Fatalf("missing pending record was not durably reconstructed: rec=%+v err=%v", stored, err)
	}
	if owned, err := svc.IsOwned(context.Background(), "player-refresh-repair", "cold"); err != nil || owned {
		t.Fatalf("reconstructed pending PIX must not grant ownership, owned=%v err=%v", owned, err)
	}
}

func TestAttachRealPurchaseHydratesRecordCreatedByEarlyWebhook(t *testing.T) {
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	requestKey := purchaseRequestKey("player-early-webhook", "cold", methodPIX, "idem-early-webhook")
	if _, _, err := entitlements.Reserve(context.Background(), Entitlement{
		PlayerID: "player-early-webhook", ReactionID: "cold", PurchaseMethod: methodPIX,
		Status: statusPending, RequestKey: requestKey, CreatedAt: "2026-08-12T00:00:00Z",
	}); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	confirmed := Record{
		PlayerID: "player-early-webhook", PurchaseID: "prdp-early-webhook", ReactionID: "cold", Method: methodPIX,
		PriceCents: 100, PriceFichas: 100_000, Status: statusConfirmed,
		CreatedAt: "2026-08-12T00:00:01Z", UpdatedAt: "2026-08-12T00:00:01Z",
	}
	if _, _, err := store.GrantConfirmed(context.Background(), entitlements, confirmed); err != nil {
		t.Fatalf("GrantConfirmed: %v", err)
	}
	details := confirmed
	details.IdemKey = "idem-early-webhook"
	details.PixCopiaECola = "000201-early"
	details.QRCodeBase64 = "early-qr"
	details.ExpiresAt = "2026-08-13T00:00:00Z"
	if err := store.AttachRealPurchase(context.Background(), entitlements, details, requestKey); err != nil {
		t.Fatalf("AttachRealPurchase after webhook: %v", err)
	}
	stored, err := store.Get(context.Background(), details.PlayerID, details.PurchaseID)
	if err != nil || stored == nil || stored.PixCopiaECola != "000201-early" || stored.QRCodeBase64 != "early-qr" {
		t.Fatalf("early-webhook record was not hydrated: rec=%+v err=%v", stored, err)
	}
}

func TestRefundSandboxCannotCreditTwice(t *testing.T) {
	wallet := &fakeWallet{}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)
	rec, err := svc.CreateSandbox(context.Background(), "player-refund-once", "cold", "idem-buy-once")
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if _, err := svc.Refund(context.Background(), "player-refund-once", rec.PurchaseID, "caller-key-1"); err != nil {
		t.Fatalf("first Refund: %v", err)
	}
	if _, err := svc.Refund(context.Background(), "player-refund-once", rec.PurchaseID, "caller-key-2"); !errors.Is(err, ErrAlreadyRefunded) {
		t.Fatalf("second Refund = %v, want ErrAlreadyRefunded", err)
	}
	if wallet.credits != 1 {
		t.Fatalf("refund credited wallet %d times, want exactly once", wallet.credits)
	}
}

func TestRefundPIXHappyPathUsesDeterministicKeyAndRevokesEntitlement(t *testing.T) {
	wallet := &fakeWallet{
		purchase:  &walletclient.ProductPurchase{PurchaseID: "prdp-refund-pix", SKU: "poker_reaction_cold", Amount: 100, Status: "pending"},
		getResult: &walletclient.ProductPurchase{PurchaseID: "prdp-refund-pix", UserID: "player-refund-pix", SKU: "poker_reaction_cold", Amount: 100, Status: "confirmed"},
	}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)
	if _, _, err := svc.CreateReal(context.Background(), "player-refund-pix", "cold", "idem-buy-pix"); err != nil {
		t.Fatalf("CreateReal: %v", err)
	}
	if _, _, err := svc.ConfirmFromWebhook(context.Background(), "prdp-refund-pix"); err != nil {
		t.Fatalf("ConfirmFromWebhook: %v", err)
	}
	refunded, err := svc.Refund(context.Background(), "player-refund-pix", "prdp-refund-pix", "caller-controlled")
	if err != nil || refunded.Status != "refunded" {
		t.Fatalf("Refund: rec=%+v err=%v", refunded, err)
	}
	if wallet.refunds != 1 || len(wallet.refundKeys) != 1 || wallet.refundKeys[0] != "reaction-refund:prdp-refund-pix" {
		t.Fatalf("unexpected refund calls/keys: calls=%d keys=%v", wallet.refunds, wallet.refundKeys)
	}
	if owned, err := svc.IsOwned(context.Background(), "player-refund-pix", "cold"); err != nil || owned {
		t.Fatalf("PIX refund must revoke ownership, owned=%v err=%v", owned, err)
	}
}

func TestCreateSandboxCannotBuyOwnedReactionTwice(t *testing.T) {
	wallet := &fakeWallet{}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)
	if _, err := svc.CreateSandbox(context.Background(), "player-buy-once", "cold", "idem-buy-first"); err != nil {
		t.Fatalf("first CreateSandbox: %v", err)
	}
	if _, err := svc.CreateSandbox(context.Background(), "player-buy-once", "cold", "idem-buy-second"); !errors.Is(err, ErrAlreadyOwned) {
		t.Fatalf("second CreateSandbox = %v, want ErrAlreadyOwned", err)
	}
	if wallet.debits != 1 {
		t.Fatalf("purchase debited wallet %d times, want exactly once", wallet.debits)
	}
}

func TestCreateSandboxSameRequestReturnsConfirmedPurchaseWithoutSecondDebit(t *testing.T) {
	wallet := &fakeWallet{}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)
	first, err := svc.CreateSandbox(context.Background(), "player-sandbox-replay", "cold", "same-key")
	if err != nil {
		t.Fatalf("first CreateSandbox: %v", err)
	}
	second, err := svc.CreateSandbox(context.Background(), "player-sandbox-replay", "cold", "same-key")
	if err != nil || second.PurchaseID != first.PurchaseID {
		t.Fatalf("replayed CreateSandbox: rec=%+v err=%v", second, err)
	}
	if wallet.debits != 1 {
		t.Fatalf("replay debited wallet %d times, want 1", wallet.debits)
	}
}

func TestConfirmFromWebhookReconstructsMissingLocalRecord(t *testing.T) {
	wallet := &fakeWallet{getResult: &walletclient.ProductPurchase{
		PurchaseID: "prdp-repair-1", UserID: "player-repair", SKU: "poker_reaction_cold",
		Amount: 100, Status: "confirmed",
	}}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)

	rec, changed, err := svc.ConfirmFromWebhook(context.Background(), "prdp-repair-1")
	if err != nil || !changed || rec.Status != "confirmed" || rec.ReactionID != "cold" {
		t.Fatalf("ConfirmFromWebhook repair: err=%v changed=%v rec=%+v", err, changed, rec)
	}
	owned, err := svc.IsOwned(context.Background(), "player-repair", "cold")
	if err != nil || !owned {
		t.Fatalf("reconstructed purchase must grant ownership, owned=%v err=%v", owned, err)
	}
}
