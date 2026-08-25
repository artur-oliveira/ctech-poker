package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/cosmeticpurchase"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

type fakeCosmeticWallet struct {
	skus []walletclient.ProductSKU
}

func allCosmeticProductSKUs() []walletclient.ProductSKU {
	return []walletclient.ProductSKU{
		{ID: "poker_deck_casino", PriceCents: 200}, {ID: "poker_deck_bicycle", PriceCents: 200},
		{ID: "poker_deck_vintage", PriceCents: 200}, {ID: "poker_deck_golden", PriceCents: 500},
		{ID: "poker_deck_pink", PriceCents: 500}, {ID: "poker_deck_alt", PriceCents: 500},
		{ID: "poker_felt_midnight", PriceCents: 200}, {ID: "poker_felt_burgundy", PriceCents: 200},
		{ID: "poker_felt_ocean", PriceCents: 200},
	}
}

func (f *fakeCosmeticWallet) ListProductSKUs(context.Context) ([]walletclient.ProductSKU, error) {
	return f.skus, nil
}
func (f *fakeCosmeticWallet) PurchaseProduct(context.Context, string, string, string) (*walletclient.ProductPurchase, error) {
	return nil, nil
}
func (f *fakeCosmeticWallet) GetProductPurchase(context.Context, string) (*walletclient.ProductPurchase, error) {
	return nil, nil
}
func (f *fakeCosmeticWallet) RefundProductPurchase(context.Context, string, string, string) (*walletclient.ProductPurchase, error) {
	return nil, nil
}
func (f *fakeCosmeticWallet) Debit(context.Context, string, int64, string, string) error  { return nil }
func (f *fakeCosmeticWallet) Credit(context.Context, string, int64, string, string) error { return nil }

func newCosmeticPurchaseApp(svc *cosmeticpurchase.Service) *fiber.App {
	app := fiber.New()
	auth := func(c fiber.Ctx) error {
		c.Locals(localsUserID, "player-1")
		return c.Next()
	}
	RegisterCosmeticPurchase(app.Group("/v1.0"), auth, svc, nil)
	return app
}

func TestCosmeticPurchaseCatalogRouteRegistered(t *testing.T) {
	wallet := &fakeCosmeticWallet{skus: allCosmeticProductSKUs()}
	svc := cosmeticpurchase.NewService(wallet, cosmeticpurchase.NewEntitlementStore(stubDynamoClient(t), "test"), cosmeticpurchase.NewStore(nil, "test"))
	app := newCosmeticPurchaseApp(svc)

	req := httptest.NewRequest(http.MethodGet, "/v1.0/wallet/cosmetic-purchase/deck/catalog", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /catalog, got %d", resp.StatusCode)
	}
	var page struct {
		Data []cosmeticpurchase.CatalogEntry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	entries := page.Data
	var found bool
	for _, e := range entries {
		if e.ID == "golden" && e.Premium && e.PriceCents == 500 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected golden premium entry with price_cents=500, got %+v", entries)
	}
}

func TestCosmeticPurchaseCatalogUnknownKindRejected(t *testing.T) {
	svc := cosmeticpurchase.NewService(&fakeCosmeticWallet{}, cosmeticpurchase.NewEntitlementStore(stubDynamoClient(t), "test"), cosmeticpurchase.NewStore(nil, "test"))
	app := newCosmeticPurchaseApp(svc)

	req := httptest.NewRequest(http.MethodGet, "/v1.0/wallet/cosmetic-purchase/hats/catalog", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown kind, got %d", resp.StatusCode)
	}
}

func TestCreateCosmeticPurchaseUnknownItemRejected(t *testing.T) {
	svc := cosmeticpurchase.NewService(&fakeCosmeticWallet{}, cosmeticpurchase.NewEntitlementStore(stubDynamoClient(t), "test"), cosmeticpurchase.NewStore(nil, "test"))
	app := newCosmeticPurchaseApp(svc)

	body, _ := json.Marshal(map[string]string{"item_id": "not-a-cosmetic", "method": "fichas"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/wallet/cosmetic-purchase/deck/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		t.Fatal("route not registered")
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown item_id, got %d", resp.StatusCode)
	}
}

func TestCreateCosmeticPurchaseInvalidMethodRejected(t *testing.T) {
	svc := cosmeticpurchase.NewService(&fakeCosmeticWallet{}, cosmeticpurchase.NewEntitlementStore(stubDynamoClient(t), "test"), cosmeticpurchase.NewStore(nil, "test"))
	app := newCosmeticPurchaseApp(svc)

	body, _ := json.Marshal(map[string]string{"item_id": "golden", "method": "credit_card"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/wallet/cosmetic-purchase/deck/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invalid method, got %d", resp.StatusCode)
	}
}

func TestCreateCosmeticPurchaseMissingItemIDRejected(t *testing.T) {
	svc := cosmeticpurchase.NewService(&fakeCosmeticWallet{}, cosmeticpurchase.NewEntitlementStore(stubDynamoClient(t), "test"), cosmeticpurchase.NewStore(nil, "test"))
	app := newCosmeticPurchaseApp(svc)

	body, _ := json.Marshal(map[string]string{"method": "fichas"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/wallet/cosmetic-purchase/felt/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for a missing item_id, got %d", resp.StatusCode)
	}
}

func TestCreateCosmeticPurchaseUnknownKindRejectedBeforeCreate(t *testing.T) {
	svc := cosmeticpurchase.NewService(&fakeCosmeticWallet{}, cosmeticpurchase.NewEntitlementStore(stubDynamoClient(t), "test"), cosmeticpurchase.NewStore(nil, "test"))
	app := newCosmeticPurchaseApp(svc)

	body, _ := json.Marshal(map[string]string{"item_id": "golden", "method": "fichas"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/wallet/cosmetic-purchase/hats/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown kind, got %d", resp.StatusCode)
	}
}
