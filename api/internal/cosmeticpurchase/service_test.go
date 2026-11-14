//go:build integration

package cosmeticpurchase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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
		{ID: "poker_felt_midnight", PriceCents: 1000}, {ID: "poker_felt_burgundy", PriceCents: 1000},
		{ID: "poker_felt_ocean", PriceCents: 1000},
	}
}

func (f *fakeWallet) ListProductSKUs(context.Context) ([]walletclient.ProductSKU, error) {
	return f.skus, nil
}
func (f *fakeWallet) PurchaseProduct(_ context.Context, _, _, key, _ string) (*walletclient.ProductPurchase, error) {
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
func (f *fakeWallet) Debit(_ context.Context, _ string, _ int64, key, _, description string) error {
	f.debits++
	f.debitKeys = append(f.debitKeys, key)
	return f.debitErr
}
func (f *fakeWallet) Credit(_ context.Context, _ string, _ int64, key, _, description string) error {
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
		{kind: cosmetics.KindFelt, itemID: "midnight", sku: "poker_felt_midnight", priceFichas: 1_000_000, priceCents: 1000},
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
			entries, err := svc.ListCatalog(context.Background(), "player-1", fx.kind)
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
			svc.SetCurrentSelectionFunc(func(context.Context, string, cosmetics.Kind) ([]string, error) {
				return []string{"other", fx.itemID}, nil // the player currently has this item applied (badges: one of several)
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
			svc.SetCurrentSelectionFunc(func(context.Context, string, cosmetics.Kind) ([]string, error) {
				return []string{"other"}, nil // the player has never applied this item
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

// TestGrantActivatesOwnershipWithoutWallet is issue #292's ownership-reuse
// acceptance: a seasonal frame/badge can be granted directly (no wallet call
// at all) and IsOwned reflects it immediately, the same as a confirmed
// purchase would.
func TestGrantActivatesOwnershipWithoutWallet(t *testing.T) {
	wallet := &fakeWallet{}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(wallet, entitlements, store)
	ctx := context.Background()

	if err := svc.Grant(ctx, "player-1", cosmetics.KindFrame, "season-2026-q3", "season:2026-q3-top10"); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	owned, err := svc.IsOwned(ctx, "player-1", cosmetics.KindFrame, "season-2026-q3")
	if err != nil || !owned {
		t.Fatalf("IsOwned after Grant: owned=%v err=%v", owned, err)
	}
	if wallet.debits != 0 && wallet.credits != 0 {
		t.Fatalf("Grant must never touch the wallet")
	}
}

func TestGrantIsIdempotent(t *testing.T) {
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(&fakeWallet{}, entitlements, store)
	ctx := context.Background()

	if err := svc.Grant(ctx, "player-1", cosmetics.KindBadge, "season-2026-q3-champion", "season:2026-q3-champion"); err != nil {
		t.Fatalf("first Grant: %v", err)
	}
	if err := svc.Grant(ctx, "player-1", cosmetics.KindBadge, "season-2026-q3-champion", "season:2026-q3-champion"); err != nil {
		t.Fatalf("repeat Grant must be a no-op, got: %v", err)
	}
}

func TestGrantUnknownItemRejected(t *testing.T) {
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(&fakeWallet{}, entitlements, store)
	if err := svc.Grant(context.Background(), "player-1", cosmetics.KindBadge, "not-a-real-badge", "season:x"); !errors.Is(err, ErrUnknownItem) {
		t.Fatalf("expected ErrUnknownItem, got %v", err)
	}
}

// Not parallel: swaps the package-level cosmetics catalog.
func TestGrantOutsideWindow(t *testing.T) {
	until := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	defer cosmetics.WithCatalogForTest([]cosmetics.CatalogEntry{
		{Kind: cosmetics.KindHat, ID: "ltd-hat", Source: cosmetics.SourceSeason, Until: until},
	})()
	svc := NewService(&fakeWallet{}, newTestEntitlementStore(t), newTestStore(t))
	ctx := context.Background()

	svc.now = func() time.Time { return until.Add(-time.Hour) }
	if err := svc.Grant(ctx, "p1", cosmetics.KindHat, "ltd-hat", "r"); err != nil {
		t.Fatalf("grant inside window: %v", err)
	}
	svc.now = func() time.Time { return until.Add(time.Hour) }
	if err := svc.Grant(ctx, "p1", cosmetics.KindHat, "ltd-hat", "r"); err != nil {
		t.Fatalf("closed-window regrant of owned item must be a no-op, got %v", err)
	}
	err := svc.Grant(ctx, "p2", cosmetics.KindHat, "ltd-hat", "r")
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("want ErrNotAvailable, got %v", err)
	}
	if owned, oerr := svc.IsOwned(ctx, "p2", cosmetics.KindHat, "ltd-hat"); oerr != nil || owned {
		t.Fatalf("no entitlement may be written: owned=%v err=%v", owned, oerr)
	}
}

func TestGrantRepeatInsideWindowIsNoop(t *testing.T) {
	until := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	defer cosmetics.WithCatalogForTest([]cosmetics.CatalogEntry{
		{Kind: cosmetics.KindHat, ID: "ltd-hat", Source: cosmetics.SourceSeason, Until: until},
	})()
	svc := NewService(&fakeWallet{}, newTestEntitlementStore(t), newTestStore(t))
	svc.now = func() time.Time { return until.Add(-time.Hour) }
	for i := 0; i < 2; i++ {
		if err := svc.Grant(context.Background(), "p1", cosmetics.KindHat, "ltd-hat", "r"); err != nil {
			t.Fatalf("grant %d: %v", i, err)
		}
	}
}

// uniquePlayer keeps these tests re-runnable against a long-lived DynamoDB
// Local container: the tables are shared, ownership is per player.
func uniquePlayer(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// allSKUsWithAvatars is allCosmeticSKUs plus every purchasable avatar SKU, at
// the catalog's fichas price / 1000 (the cents the wallet would report).
func allSKUsWithAvatars() []walletclient.ProductSKU {
	skus := allCosmeticSKUs()
	for _, kind := range []cosmetics.Kind{cosmetics.KindFrame, cosmetics.KindHat, cosmetics.KindAccessory} {
		for _, e := range cosmetics.All(kind) {
			if e.Source == cosmetics.SourcePurchase {
				skus = append(skus, walletclient.ProductSKU{ID: e.SKU, PriceCents: e.PriceFichas / 1000})
			}
		}
	}
	return skus
}

func TestListCatalogIncludesEarnableWithoutPrice(t *testing.T) {
	ctx := context.Background()
	w := &fakeWallet{skus: allSKUsWithAvatars()}
	svc := NewService(w, newTestEntitlementStore(t), newTestStore(t))
	playerID := uniquePlayer("player-catalog-earnable")
	if err := svc.Grant(ctx, playerID, cosmetics.KindFrame, "chip-ring", "achievement:hands_played:3"); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	entries, err := svc.ListCatalog(ctx, playerID, cosmetics.KindFrame)
	if err != nil {
		t.Fatalf("ListCatalog(frame): %v", err)
	}
	byID := map[string]CatalogEntry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	earned := byID["chip-ring"]
	if earned.Source != string(cosmetics.SourceAchievement) || !earned.Premium || !earned.Owned || !earned.Available ||
		earned.PriceCents != 0 || earned.PriceFichas != 0 {
		t.Fatalf("earned achievement frame: %+v", earned)
	}
	locked := byID["season-2026-q3"]
	if locked.Source != string(cosmetics.SourceSeason) || locked.Owned || locked.PriceCents != 0 || locked.PriceFichas != 0 {
		t.Fatalf("locked season frame: %+v", locked)
	}
	sold := byID["felt-ring"]
	if sold.Source != string(cosmetics.SourcePurchase) || sold.PriceCents != 600 || sold.PriceFichas != 600_000 || sold.Owned || !sold.Available {
		t.Fatalf("purchasable frame: %+v", sold)
	}
	raw, _ := json.Marshal(earned)
	if strings.Contains(string(raw), "price") {
		t.Fatalf("earnable entry must carry no price field: %s", raw)
	}

	deck, err := svc.ListCatalog(ctx, playerID, cosmetics.KindDeck)
	if err != nil {
		t.Fatalf("ListCatalog(deck): %v", err)
	}
	for _, e := range deck {
		if e.Source != "" || !e.Available {
			t.Fatalf("deck entry gained a source or lost availability: %+v", e)
		}
	}
}

// assertNothingWritten checks a refused purchase left no reservation, no
// history row and never reached the wallet.
func assertNothingWritten(t *testing.T, w *fakeWallet, entitlements *EntitlementStore, store *Store, playerID string, kind cosmetics.Kind, itemID, idemKey string) {
	t.Helper()
	ctx := context.Background()
	if ent, err := entitlements.Get(ctx, playerID, kind, itemID); err != nil || ent != nil {
		t.Fatalf("refused purchase left entitlement %+v (err=%v)", ent, err)
	}
	purchaseID := sandboxPurchaseID(purchaseRequestKey(playerID, kind, itemID, methodFichas, idemKey))
	if rec, err := store.Get(ctx, playerID, purchaseID); err != nil || rec != nil {
		t.Fatalf("refused purchase left history %+v (err=%v)", rec, err)
	}
	if w.debits != 0 || len(w.purchaseKeys) != 0 {
		t.Fatalf("refused purchase reached the wallet: debits=%d purchases=%d", w.debits, len(w.purchaseKeys))
	}
}

func TestCreateRejectsNonPurchasable(t *testing.T) {
	cases := []struct {
		kind   cosmetics.Kind
		itemID string
	}{
		{cosmetics.KindHat, "crown"},             // achievement
		{cosmetics.KindFrame, "season-2026-q3"},  // season
		{cosmetics.KindBadge, "veteran"},         // badges are never sold
		{cosmetics.KindAccessory, "bandit-mask"}, // achievement
	}
	for _, tc := range cases {
		t.Run(string(tc.kind)+"/"+tc.itemID, func(t *testing.T) {
			ctx := context.Background()
			w := &fakeWallet{purchase: &walletclient.ProductPurchase{PurchaseID: "prdp-never", Status: "pending"}}
			entitlements, store := newTestEntitlementStore(t), newTestStore(t)
			svc := NewService(w, entitlements, store)
			playerID := uniquePlayer("player-not-purchasable")
			if _, err := svc.CreateSandbox(ctx, playerID, tc.kind, tc.itemID, "idem-np"); !errors.Is(err, ErrNotPurchasable) {
				t.Fatalf("CreateSandbox = %v, want ErrNotPurchasable", err)
			}
			if _, _, err := svc.CreateReal(ctx, playerID, tc.kind, tc.itemID, "idem-np"); !errors.Is(err, ErrNotPurchasable) {
				t.Fatalf("CreateReal = %v, want ErrNotPurchasable", err)
			}
			assertNothingWritten(t, w, entitlements, store, playerID, tc.kind, tc.itemID, "idem-np")
		})
	}
}

// Not parallel: swaps the package-level cosmetics catalog.
func TestCreateRejectsOutsideWindow(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	defer cosmetics.WithCatalogForTest([]cosmetics.CatalogEntry{
		{Kind: cosmetics.KindHat, ID: "closed-hat", Premium: true, Source: cosmetics.SourcePurchase, SKU: "poker_hat_closed", PriceFichas: 100_000, Until: now},
		{Kind: cosmetics.KindHat, ID: "future-hat", Premium: true, Source: cosmetics.SourcePurchase, SKU: "poker_hat_future", PriceFichas: 100_000, From: now.Add(time.Hour)},
	})()
	for _, itemID := range []string{"closed-hat", "future-hat"} {
		t.Run(itemID, func(t *testing.T) {
			ctx := context.Background()
			w := &fakeWallet{purchase: &walletclient.ProductPurchase{PurchaseID: "prdp-never", Status: "pending"}}
			entitlements, store := newTestEntitlementStore(t), newTestStore(t)
			svc := NewService(w, entitlements, store)
			svc.now = func() time.Time { return now } // Until is exclusive, From is in the future
			playerID := uniquePlayer("player-window")
			if _, err := svc.CreateSandbox(ctx, playerID, cosmetics.KindHat, itemID, "idem-window"); !errors.Is(err, ErrNotAvailable) {
				t.Fatalf("CreateSandbox = %v, want ErrNotAvailable", err)
			}
			if _, _, err := svc.CreateReal(ctx, playerID, cosmetics.KindHat, itemID, "idem-window"); !errors.Is(err, ErrNotAvailable) {
				t.Fatalf("CreateReal = %v, want ErrNotAvailable", err)
			}
			assertNothingWritten(t, w, entitlements, store, playerID, cosmetics.KindHat, itemID, "idem-window")
		})
	}
}

// Not parallel: swaps the package-level cosmetics catalog.
func TestIdempotentReplayAfterWindowClosedStillResolves(t *testing.T) {
	until := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	const sku = "poker_hat_ltd"
	defer cosmetics.WithCatalogForTest([]cosmetics.CatalogEntry{
		{Kind: cosmetics.KindHat, ID: "ltd-hat", Premium: true, Source: cosmetics.SourcePurchase, SKU: sku, PriceFichas: 100_000, Until: until},
	})()
	ctx := context.Background()

	t.Run("fichas", func(t *testing.T) {
		w := &fakeWallet{}
		svc := NewService(w, newTestEntitlementStore(t), newTestStore(t))
		playerID := uniquePlayer("player-replay-fichas")
		svc.now = func() time.Time { return until.Add(-time.Minute) }
		first, err := svc.CreateSandbox(ctx, playerID, cosmetics.KindHat, "ltd-hat", "idem-replay")
		if err != nil {
			t.Fatalf("CreateSandbox inside window: %v", err)
		}
		svc.now = func() time.Time { return until.Add(time.Hour) }
		second, err := svc.CreateSandbox(ctx, playerID, cosmetics.KindHat, "ltd-hat", "idem-replay")
		if err != nil || second.PurchaseID != first.PurchaseID || second.Status != "confirmed" {
			t.Fatalf("replay after close: rec=%+v err=%v", second, err)
		}
		if w.debits != 1 {
			t.Fatalf("replay debited %d times, want 1", w.debits)
		}
		other := uniquePlayer("player-replay-fichas-other")
		if _, err := svc.CreateSandbox(ctx, other, cosmetics.KindHat, "ltd-hat", "idem-replay"); !errors.Is(err, ErrNotAvailable) {
			t.Fatalf("fresh purchase after close = %v, want ErrNotAvailable", err)
		}
	})

	t.Run("pix", func(t *testing.T) {
		playerID := uniquePlayer("player-replay-pix")
		purchaseID := "prdp-replay-" + playerID
		pending := &walletclient.ProductPurchase{PurchaseID: purchaseID, UserID: playerID, SKU: sku, Amount: 100, Status: "pending"}
		w := &fakeWallet{purchase: pending, getResult: pending}
		svc := NewService(w, newTestEntitlementStore(t), newTestStore(t))
		svc.now = func() time.Time { return until.Add(-time.Minute) }
		first, _, err := svc.CreateReal(ctx, playerID, cosmetics.KindHat, "ltd-hat", "idem-replay")
		if err != nil {
			t.Fatalf("CreateReal inside window: %v", err)
		}
		svc.now = func() time.Time { return until.Add(time.Hour) }
		second, _, err := svc.CreateReal(ctx, playerID, cosmetics.KindHat, "ltd-hat", "idem-replay")
		if err != nil || second.PurchaseID != first.PurchaseID {
			t.Fatalf("replay after close: rec=%+v err=%v", second, err)
		}
		if len(w.purchaseKeys) != 1 {
			t.Fatalf("replay opened %d wallet purchases, want 1", len(w.purchaseKeys))
		}
		if _, _, err := svc.CreateReal(ctx, playerID, cosmetics.KindHat, "ltd-hat", "idem-other-key"); !errors.Is(err, ErrNotAvailable) {
			t.Fatalf("new key after close = %v, want ErrNotAvailable", err)
		}
	})
}

func TestSandboxPurchaseOfAvatarItemGrantsOwnership(t *testing.T) {
	ctx := context.Background()
	w := &fakeWallet{}
	svc := NewService(w, newTestEntitlementStore(t), newTestStore(t))
	playerID := uniquePlayer("player-avatar-buy")
	rec, err := svc.CreateSandbox(ctx, playerID, cosmetics.KindHat, "top-hat", "idem-top-hat")
	if err != nil {
		t.Fatalf("CreateSandbox(top-hat): %v", err)
	}
	if rec.Status != "confirmed" || rec.Kind != string(cosmetics.KindHat) || rec.PriceFichas != 300_000 {
		t.Fatalf("unexpected record: %+v", rec)
	}
	if owned, err := svc.IsOwned(ctx, playerID, cosmetics.KindHat, "top-hat"); err != nil || !owned {
		t.Fatalf("top-hat not owned after purchase: owned=%v err=%v", owned, err)
	}
	if w.debits != 1 {
		t.Fatalf("debits=%d, want 1", w.debits)
	}
}

// The wallet webhook resolves a PIX purchase by SKU alone; an avatar SKU must
// map back to its own kind so the entitlement lands in the right slot.
func TestConfirmFromWebhookGrantsAvatarEntitlement(t *testing.T) {
	ctx := context.Background()
	playerID := uniquePlayer("player-avatar-webhook")
	purchaseID := "prdp-avatar-" + playerID
	w := &fakeWallet{getResult: &walletclient.ProductPurchase{PurchaseID: purchaseID, UserID: playerID, SKU: "poker_accessory_monocle", Amount: 500, Status: "confirmed"}}
	svc := NewService(w, newTestEntitlementStore(t), newTestStore(t))
	if !svc.HandlesSKU("poker_accessory_monocle") {
		t.Fatal("avatar SKU not routed to cosmeticpurchase")
	}
	rec, _, err := svc.ConfirmFromWebhook(ctx, purchaseID)
	if err != nil || rec.Kind != string(cosmetics.KindAccessory) || rec.ItemID != "monocle" {
		t.Fatalf("ConfirmFromWebhook: rec=%+v err=%v", rec, err)
	}
	if owned, err := svc.IsOwned(ctx, playerID, cosmetics.KindAccessory, "monocle"); err != nil || !owned {
		t.Fatalf("monocle not owned after webhook: owned=%v err=%v", owned, err)
	}
}
