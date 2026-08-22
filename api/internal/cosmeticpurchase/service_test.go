//go:build integration

package cosmeticpurchase

import (
	"context"
	"errors"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/cosmetics"
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

func allCosmeticSKUs() []walletclient.ProductSKU {
	return []walletclient.ProductSKU{
		{ID: "poker_deck_casino", PriceCents: 200}, {ID: "poker_deck_bicycle", PriceCents: 200},
		{ID: "poker_deck_vintage", PriceCents: 200}, {ID: "poker_deck_golden", PriceCents: 500},
		{ID: "poker_deck_pink", PriceCents: 500}, {ID: "poker_deck_alt", PriceCents: 500},
		{ID: "poker_felt_midnight", PriceCents: 200}, {ID: "poker_felt_burgundy", PriceCents: 200},
		{ID: "poker_felt_ocean", PriceCents: 200},
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

// premiumFixture is one premium catalog row per kind used to drive the
// table-driven outer loop the plan calls for (Task 2's test scenarios run for
// both KindDeck and KindFelt).
type premiumFixture struct {
	kind        cosmetics.Kind
	itemID      string
	sku         string
	priceFichas int64
	priceCents  int64
}

func premiumFixtures() []premiumFixture {
	return []premiumFixture{
		{kind: cosmetics.KindDeck, itemID: "golden", sku: "poker_deck_golden", priceFichas: 500_000, priceCents: 500},
		{kind: cosmetics.KindFelt, itemID: "midnight", sku: "poker_felt_midnight", priceFichas: 200_000, priceCents: 200},
	}
}

// Each test below uses its own playerID — the underlying tables are fixed
// names shared across every test run against the same DynamoDB Local
// container (mirrors reactionpurchase's own integration-test convention), so
// a shared player id across sub-tests would leak entitlements between them.

func TestListCatalogMergesFreeAndPremium(t *testing.T) {
	for _, fx := range premiumFixtures() {
		t.Run(string(fx.kind), func(t *testing.T) {
			w := &fakeWallet{skus: allCosmeticSKUs()}
			svc := NewService(w, newTestEntitlementStore(t), newTestStore(t))
			entries, err := svc.ListCatalog(context.Background(), fx.kind)
			if err != nil {
				t.Fatalf("ListCatalog: %v", err)
			}
			var found bool
			for _, e := range entries {
				if e.ID == fx.itemID {
					found = true
					if !e.Premium || e.PriceCents != fx.priceCents || e.PriceFichas != fx.priceFichas {
						t.Fatalf("unexpected %s entry: %+v", fx.itemID, e)
					}
				}
			}
			if !found {
				t.Fatalf("expected %s in the merged catalog", fx.itemID)
			}
		})
	}
}

func TestCreateRealUnknownItemRejected(t *testing.T) {
	for _, kind := range []cosmetics.Kind{cosmetics.KindDeck, cosmetics.KindFelt} {
		svc := NewService(&fakeWallet{}, newTestEntitlementStore(t), newTestStore(t))
		_, _, err := svc.CreateReal(context.Background(), "player-unknown-"+string(kind), kind, "not-a-cosmetic", "idem-1")
		if err != ErrUnknownItem {
			t.Fatalf("expected ErrUnknownItem, got %v", err)
		}
	}
}

func TestCreateRealFreeItemRejected(t *testing.T) {
	cases := map[cosmetics.Kind]string{cosmetics.KindDeck: "four-color", cosmetics.KindFelt: "classic"}
	for kind, id := range cases {
		svc := NewService(&fakeWallet{}, newTestEntitlementStore(t), newTestStore(t))
		_, _, err := svc.CreateReal(context.Background(), "player-free-"+string(kind), kind, id, "idem-1")
		if err != ErrNotPremium {
			t.Fatalf("expected ErrNotPremium, got %v", err)
		}
	}
}

func TestCreateRealReservesWithoutGrantingUntilConfirmed(t *testing.T) {
	for _, fx := range premiumFixtures() {
		t.Run(string(fx.kind), func(t *testing.T) {
			w := &fakeWallet{purchase: &walletclient.ProductPurchase{
				PurchaseID: "prdp-open-" + string(fx.kind), SKU: fx.sku, Amount: fx.priceCents, Status: "pending",
				PixCopiaECola: "000201-pix", QRCodeBase64: "base64-qr", ExpiresAt: "2026-08-22T00:00:00Z",
			}}
			entitlements, store := newTestEntitlementStore(t), newTestStore(t)
			svc := NewService(w, entitlements, store)
			playerID := "player-open-" + string(fx.kind)
			rec, _, err := svc.CreateReal(context.Background(), playerID, fx.kind, fx.itemID, "idem-open-1")
			if err != nil {
				t.Fatalf("CreateReal: %v", err)
			}
			if rec.Status != "pending" || rec.Method != "pix" {
				t.Fatalf("unexpected record: %+v", rec)
			}
			got, err := entitlements.Get(context.Background(), playerID, fx.kind, fx.itemID)
			if err != nil || got == nil || got.Status != "pending" {
				t.Fatalf("expected a non-owned pending reservation, got %+v (err=%v)", got, err)
			}
			owned, err := svc.IsOwned(context.Background(), playerID, fx.kind, fx.itemID)
			if err != nil || owned {
				t.Fatalf("pending reservation must not grant ownership, owned=%v err=%v", owned, err)
			}
		})
	}
}

func TestCreateRealImmediateConfirmationActivatesOwnership(t *testing.T) {
	for _, fx := range premiumFixtures() {
		t.Run(string(fx.kind), func(t *testing.T) {
			wallet := &fakeWallet{purchase: &walletclient.ProductPurchase{
				PurchaseID: "prdp-immediate-" + string(fx.kind), SKU: fx.sku, Amount: fx.priceCents, Status: "confirmed",
			}}
			entitlements, store := newTestEntitlementStore(t), newTestStore(t)
			svc := NewService(wallet, entitlements, store)
			playerID := "player-immediate-" + string(fx.kind)
			rec, _, err := svc.CreateReal(context.Background(), playerID, fx.kind, fx.itemID, "idem-immediate-confirm")
			if err != nil || rec.Status != "confirmed" {
				t.Fatalf("CreateReal: rec=%+v err=%v", rec, err)
			}
			if owned, err := svc.IsOwned(context.Background(), playerID, fx.kind, fx.itemID); err != nil || !owned {
				t.Fatalf("immediately confirmed PIX must grant ownership, owned=%v err=%v", owned, err)
			}
		})
	}
}

func TestCreateSandboxDefinitiveWalletRejectionLeavesNoRows(t *testing.T) {
	for _, fx := range premiumFixtures() {
		t.Run(string(fx.kind), func(t *testing.T) {
			wallet := &fakeWallet{debitErr: &walletclient.Error{Status: 409, Title: "Insufficient balance"}}
			entitlements, store := newTestEntitlementStore(t), newTestStore(t)
			svc := NewService(wallet, entitlements, store)
			playerID := "player-insufficient-" + string(fx.kind)
			_, err := svc.CreateSandbox(context.Background(), playerID, fx.kind, fx.itemID, "idem-insufficient")
			if err == nil {
				t.Fatal("expected insufficient-balance error")
			}
			if ent, getErr := entitlements.Get(context.Background(), playerID, fx.kind, fx.itemID); getErr != nil || ent != nil {
				t.Fatalf("definitive rejection left entitlement %+v (err=%v)", ent, getErr)
			}
			purchaseID := sandboxPurchaseID(purchaseRequestKey(playerID, fx.kind, fx.itemID, "fichas", "idem-insufficient"))
			if rec, getErr := store.Get(context.Background(), playerID, purchaseID); getErr != nil || rec != nil {
				t.Fatalf("definitive rejection left record %+v (err=%v)", rec, getErr)
			}
		})
	}
}

func TestCreateSandboxGrantsEntitlementSynchronously(t *testing.T) {
	for _, fx := range premiumFixtures() {
		t.Run(string(fx.kind), func(t *testing.T) {
			entitlements, store := newTestEntitlementStore(t), newTestStore(t)
			svc := NewService(&fakeWallet{}, entitlements, store)
			playerID := "player-sandbox-sync-" + string(fx.kind)
			rec, err := svc.CreateSandbox(context.Background(), playerID, fx.kind, fx.itemID, "idem-sandbox-3")
			if err != nil {
				t.Fatalf("CreateSandbox: %v", err)
			}
			if rec.Status != "confirmed" || rec.Method != "fichas" || rec.PriceFichas != fx.priceFichas {
				t.Fatalf("unexpected record: %+v", rec)
			}
			owned, err := svc.IsOwned(context.Background(), playerID, fx.kind, fx.itemID)
			if err != nil || !owned {
				t.Fatalf("expected owned=true immediately, got %v (err=%v)", owned, err)
			}
		})
	}
}

func TestCreateSandboxCannotBuyOwnedItemTwice(t *testing.T) {
	for _, fx := range premiumFixtures() {
		t.Run(string(fx.kind), func(t *testing.T) {
			wallet := &fakeWallet{}
			entitlements, store := newTestEntitlementStore(t), newTestStore(t)
			svc := NewService(wallet, entitlements, store)
			playerID := "player-buy-once-" + string(fx.kind)
			if _, err := svc.CreateSandbox(context.Background(), playerID, fx.kind, fx.itemID, "idem-buy-first"); err != nil {
				t.Fatalf("first CreateSandbox: %v", err)
			}
			if _, err := svc.CreateSandbox(context.Background(), playerID, fx.kind, fx.itemID, "idem-buy-second"); !errors.Is(err, ErrAlreadyOwned) {
				t.Fatalf("second CreateSandbox = %v, want ErrAlreadyOwned", err)
			}
			if wallet.debits != 1 {
				t.Fatalf("purchase debited wallet %d times, want exactly once", wallet.debits)
			}
		})
	}
}

func TestRefundRejectedWhenCurrentSelection(t *testing.T) {
	for _, fx := range premiumFixtures() {
		t.Run(string(fx.kind), func(t *testing.T) {
			entitlements, store := newTestEntitlementStore(t), newTestStore(t)
			svc := NewService(&fakeWallet{}, entitlements, store)
			svc.SetCurrentSelectionFunc(func(context.Context, string, cosmetics.Kind) (string, error) {
				return fx.itemID, nil // the player currently has this item applied
			})
			playerID := "player-refund-in-use-" + string(fx.kind)
			rec, err := svc.CreateSandbox(context.Background(), playerID, fx.kind, fx.itemID, "idem-sandbox-4")
			if err != nil {
				t.Fatalf("CreateSandbox: %v", err)
			}
			if _, err := svc.Refund(context.Background(), playerID, rec.PurchaseID, "idem-refund-1"); err != ErrInUse {
				t.Fatalf("expected ErrInUse, got %v", err)
			}
		})
	}
}

func TestRefundSandboxHappyPathRevokesEntitlement(t *testing.T) {
	for _, fx := range premiumFixtures() {
		t.Run(string(fx.kind), func(t *testing.T) {
			entitlements, store := newTestEntitlementStore(t), newTestStore(t)
			svc := NewService(&fakeWallet{}, entitlements, store)
			svc.SetCurrentSelectionFunc(func(context.Context, string, cosmetics.Kind) (string, error) {
				return "", nil // the player has never applied this item
			})
			playerID := "player-refund-happy-" + string(fx.kind)
			rec, err := svc.CreateSandbox(context.Background(), playerID, fx.kind, fx.itemID, "idem-sandbox-5")
			if err != nil {
				t.Fatalf("CreateSandbox: %v", err)
			}
			refunded, err := svc.Refund(context.Background(), playerID, rec.PurchaseID, "idem-refund-2")
			if err != nil || refunded.Status != "refunded" {
				t.Fatalf("Refund: %v, %+v", err, refunded)
			}
			owned, err := svc.IsOwned(context.Background(), playerID, fx.kind, fx.itemID)
			if err != nil || owned {
				t.Fatalf("expected owned=false after refund, got %v (err=%v)", owned, err)
			}
		})
	}
}

func TestRefundWithoutCurrentSelectionFuncWiredAllowsRefund(t *testing.T) {
	// A missing SetCurrentSelectionFunc call (e.g. a wiring mistake) must not
	// leave every refund permanently blocked — it degrades to "never in use,"
	// same as a service that never wires the func at all.
	fx := premiumFixtures()[0]
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(&fakeWallet{}, entitlements, store)
	playerID := "player-refund-no-func"
	rec, err := svc.CreateSandbox(context.Background(), playerID, fx.kind, fx.itemID, "idem-sandbox-6")
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if _, err := svc.Refund(context.Background(), playerID, rec.PurchaseID, "idem-refund-3"); err != nil {
		t.Fatalf("Refund without a wired currentSelection func: %v", err)
	}
}

func TestRefundSandboxCannotCreditTwice(t *testing.T) {
	fx := premiumFixtures()[0]
	wallet := &fakeWallet{}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)
	playerID := "player-refund-once"
	rec, err := svc.CreateSandbox(context.Background(), playerID, fx.kind, fx.itemID, "idem-buy-once")
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if _, err := svc.Refund(context.Background(), playerID, rec.PurchaseID, "caller-key-1"); err != nil {
		t.Fatalf("first Refund: %v", err)
	}
	if _, err := svc.Refund(context.Background(), playerID, rec.PurchaseID, "caller-key-2"); !errors.Is(err, ErrAlreadyRefunded) {
		t.Fatalf("second Refund = %v, want ErrAlreadyRefunded", err)
	}
	if wallet.credits != 1 {
		t.Fatalf("refund credited wallet %d times, want exactly once", wallet.credits)
	}
}

func TestConfirmFromWebhookGrantsEntitlement(t *testing.T) {
	fx := premiumFixtures()[0]
	w := &fakeWallet{
		purchase:  &walletclient.ProductPurchase{PurchaseID: "prdp-confirm-2", SKU: fx.sku, Amount: fx.priceCents, Status: "pending"},
		getResult: &walletclient.ProductPurchase{PurchaseID: "prdp-confirm-2", UserID: "player-confirm", SKU: fx.sku, Amount: fx.priceCents, Status: "confirmed"},
	}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(w, entitlements, store)
	if _, _, err := svc.CreateReal(context.Background(), "player-confirm", fx.kind, fx.itemID, "idem-confirm-2"); err != nil {
		t.Fatalf("CreateReal: %v", err)
	}
	rec, changed, err := svc.ConfirmFromWebhook(context.Background(), "prdp-confirm-2")
	if err != nil || !changed || rec.Status != "confirmed" {
		t.Fatalf("ConfirmFromWebhook: %v, changed=%v, rec=%+v", err, changed, rec)
	}
	got, err := entitlements.Get(context.Background(), "player-confirm", fx.kind, fx.itemID)
	if err != nil || got == nil {
		t.Fatalf("expected entitlement after confirm, got %+v (err=%v)", got, err)
	}
}

func TestCreateSandboxSameRequestReturnsConfirmedPurchaseWithoutSecondDebit(t *testing.T) {
	fx := premiumFixtures()[0]
	wallet := &fakeWallet{}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)
	playerID := "player-sandbox-replay"
	first, err := svc.CreateSandbox(context.Background(), playerID, fx.kind, fx.itemID, "same-key")
	if err != nil {
		t.Fatalf("first CreateSandbox: %v", err)
	}
	second, err := svc.CreateSandbox(context.Background(), playerID, fx.kind, fx.itemID, "same-key")
	if err != nil || second.PurchaseID != first.PurchaseID {
		t.Fatalf("replayed CreateSandbox: rec=%+v err=%v", second, err)
	}
	if wallet.debits != 1 {
		t.Fatalf("replay debited wallet %d times, want 1", wallet.debits)
	}
}
