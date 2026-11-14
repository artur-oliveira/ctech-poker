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
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
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
func (f *fakeCosmeticWallet) PurchaseProduct(context.Context, string, string, string, string) (*walletclient.ProductPurchase, error) {
	return nil, nil
}
func (f *fakeCosmeticWallet) GetProductPurchase(context.Context, string) (*walletclient.ProductPurchase, error) {
	return nil, nil
}
func (f *fakeCosmeticWallet) RefundProductPurchase(context.Context, string, string, string) (*walletclient.ProductPurchase, error) {
	return nil, nil
}
func (f *fakeCosmeticWallet) Debit(context.Context, string, int64, string, string, string) error {
	return nil
}
func (f *fakeCosmeticWallet) Credit(context.Context, string, int64, string, string, string) error {
	return nil
}

func newCosmeticPurchaseApp(svc *cosmeticpurchase.Service) *fiber.App {
	app := fiber.New()
	auth := func(c fiber.Ctx) error {
		c.Locals(localsUserID, "player-1")
		return c.Next()
	}
	RegisterCosmeticPurchase(app.Group("/v1.0"), auth, nil, svc, nil)
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

// avatarProductSKUs is every purchasable avatar SKU at fichas/1000 cents.
func avatarProductSKUs() []walletclient.ProductSKU {
	skus := allCosmeticProductSKUs()
	for _, kind := range []cosmetics.Kind{cosmetics.KindFrame, cosmetics.KindHat, cosmetics.KindAccessory} {
		for _, e := range cosmetics.All(kind) {
			if e.SKU != "" {
				skus = append(skus, walletclient.ProductSKU{ID: e.SKU, PriceCents: e.PriceFichas / 1000})
			}
		}
	}
	return skus
}

func TestPurchaseRouteAcceptsAvatarKindsRejectsBadge(t *testing.T) {
	svc := cosmeticpurchase.NewService(&fakeCosmeticWallet{skus: avatarProductSKUs()}, cosmeticpurchase.NewEntitlementStore(stubDynamoClient(t), "test"), cosmeticpurchase.NewStore(nil, "test"))
	app := newCosmeticPurchaseApp(svc)
	cases := map[string]int{
		"deck": http.StatusOK, "felt": http.StatusOK,
		"frame": http.StatusOK, "hat": http.StatusOK, "accessory": http.StatusOK,
		"badge": http.StatusBadRequest, "hats": http.StatusBadRequest,
	}
	for kind, want := range cases {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1.0/wallet/cosmetic-purchase/"+kind+"/catalog", nil))
		if err != nil {
			t.Fatalf("%s: app.Test: %v", kind, err)
		}
		if resp.StatusCode != want {
			t.Fatalf("%s catalog: status %d, want %d", kind, resp.StatusCode, want)
		}
	}
}

// An earnable item is refused before any store or wallet call.
func TestCreateCosmeticPurchaseEarnableItemNotPurchasable(t *testing.T) {
	svc := cosmeticpurchase.NewService(&fakeCosmeticWallet{}, cosmeticpurchase.NewEntitlementStore(stubDynamoClient(t), "test"), cosmeticpurchase.NewStore(nil, "test"))
	app := newCosmeticPurchaseApp(svc)
	for _, method := range []string{"fichas", "pix"} {
		body, _ := json.Marshal(map[string]string{"item_id": "crown", "method": method})
		req := httptest.NewRequest(http.MethodPost, "/v1.0/wallet/cosmetic-purchase/hat/", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		var p struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&p)
		if resp.StatusCode != http.StatusBadRequest || p.Code != problemCodeNotPurchasable {
			t.Fatalf("%s: status=%d code=%q, want 400 %q", method, resp.StatusCode, p.Code, problemCodeNotPurchasable)
		}
	}
}

func TestCosmeticPurchaseProblemCodes(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{cosmeticpurchase.ErrNotPurchasable, http.StatusBadRequest, problemCodeNotPurchasable},
		{cosmeticpurchase.ErrNotAvailable, http.StatusConflict, problemCodeNotAvailable},
	}
	for _, tc := range cases {
		p := cosmeticPurchaseProblem(tc.err, nil)
		if p.Status != tc.status || p.Code != tc.code {
			t.Fatalf("%v: status=%d code=%q, want %d %q", tc.err, p.Status, p.Code, tc.status, tc.code)
		}
	}
}
