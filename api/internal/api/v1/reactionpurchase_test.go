package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

type fakeReactionWallet struct {
	skus []walletclient.ProductSKU
}

func allReactionProductSKUs() []walletclient.ProductSKU {
	return []walletclient.ProductSKU{
		{ID: "poker_reaction_cold", PriceCents: 100}, {ID: "poker_reaction_fire", PriceCents: 100},
		{ID: "poker_reaction_poop", PriceCents: 500}, {ID: "poker_reaction_rofl", PriceCents: 500},
		{ID: "poker_reaction_knife", PriceCents: 500}, {ID: "poker_reaction_turtle", PriceCents: 500},
	}
}

func (f *fakeReactionWallet) ListProductSKUs(context.Context) ([]walletclient.ProductSKU, error) {
	return f.skus, nil
}
func (f *fakeReactionWallet) PurchaseProduct(context.Context, string, string, string) (*walletclient.ProductPurchase, error) {
	return nil, nil
}
func (f *fakeReactionWallet) GetProductPurchase(context.Context, string) (*walletclient.ProductPurchase, error) {
	return nil, nil
}
func (f *fakeReactionWallet) RefundProductPurchase(context.Context, string, string, string) (*walletclient.ProductPurchase, error) {
	return nil, nil
}
func (f *fakeReactionWallet) Debit(context.Context, string, int64, string, string) error  { return nil }
func (f *fakeReactionWallet) Credit(context.Context, string, int64, string, string) error { return nil }

func newReactionPurchaseApp(svc *reactionpurchase.Service) *fiber.App {
	app := fiber.New()
	auth := func(c fiber.Ctx) error {
		c.Locals(localsUserID, "player-1")
		return c.Next()
	}
	RegisterReactionPurchase(app.Group("/v1.0"), auth, svc, nil)
	return app
}

func TestReactionPurchaseCatalogRouteRegistered(t *testing.T) {
	wallet := &fakeReactionWallet{skus: allReactionProductSKUs()}
	svc := reactionpurchase.NewService(wallet, reactionpurchase.NewEntitlementStore(nil, "test"), reactionpurchase.NewStore(nil, "test"))
	app := newReactionPurchaseApp(svc)

	req := httptest.NewRequest(http.MethodGet, "/v1.0/wallet/reaction-purchase/catalog", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /catalog, got %d", resp.StatusCode)
	}
	var entries []reactionpurchase.CatalogEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.ID == "cold" && e.Premium && e.PriceCents == 100 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cold premium entry with price_cents=100, got %+v", entries)
	}
}

// TestCreateReactionPurchaseUnknownReactionRejected proves POST / is wired
// without reaching the store's DynamoDB write path — an unknown reaction_id
// is rejected by CreateReal/CreateSandbox before either ever calls
// store.Create, so this stays a pure-Go route/handler check with nil stores
// (mirrors sandboxpurchase_test.go's TestCreateSandboxPurchaseRouteRegistered).
func TestCreateReactionPurchaseUnknownReactionRejected(t *testing.T) {
	svc := reactionpurchase.NewService(&fakeReactionWallet{}, reactionpurchase.NewEntitlementStore(nil, "test"), reactionpurchase.NewStore(nil, "test"))
	app := newReactionPurchaseApp(svc)

	body, _ := json.Marshal(map[string]string{"reaction_id": "not-a-reaction", "method": "fichas"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/wallet/reaction-purchase/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		t.Fatal("route not registered")
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown reaction_id, got %d", resp.StatusCode)
	}
}

func TestCreateReactionPurchaseInvalidMethodRejected(t *testing.T) {
	svc := reactionpurchase.NewService(&fakeReactionWallet{}, reactionpurchase.NewEntitlementStore(nil, "test"), reactionpurchase.NewStore(nil, "test"))
	app := newReactionPurchaseApp(svc)

	body, _ := json.Marshal(map[string]string{"reaction_id": "cold", "method": "credit_card"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/wallet/reaction-purchase/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invalid method, got %d", resp.StatusCode)
	}
}

func TestCreateReactionPurchaseMissingReactionIDRejected(t *testing.T) {
	svc := reactionpurchase.NewService(&fakeReactionWallet{}, reactionpurchase.NewEntitlementStore(nil, "test"), reactionpurchase.NewStore(nil, "test"))
	app := newReactionPurchaseApp(svc)

	body, _ := json.Marshal(map[string]string{"method": "fichas"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/wallet/reaction-purchase/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for a missing reaction_id, got %d", resp.StatusCode)
	}
}
