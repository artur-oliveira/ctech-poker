package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

type fakeSandboxWallet struct {
	skus     []walletclient.SandboxSKU
	purchase *walletclient.SandboxPurchase
}

func (f *fakeSandboxWallet) ListSandboxSKUs(context.Context) ([]walletclient.SandboxSKU, error) {
	return f.skus, nil
}
func (f *fakeSandboxWallet) PurchaseSandbox(context.Context, string, string, string) (*walletclient.SandboxPurchase, error) {
	return f.purchase, nil
}
func (f *fakeSandboxWallet) GetSandboxPurchase(context.Context, string) (*walletclient.SandboxPurchase, error) {
	return f.purchase, nil
}
func (f *fakeSandboxWallet) RefundSandboxPurchase(context.Context, string, string, string) (*walletclient.SandboxPurchase, error) {
	return f.purchase, nil
}

func newSandboxPurchaseApp(svc *sandboxpurchase.Service) *fiber.App {
	app := fiber.New()
	auth := func(c fiber.Ctx) error {
		c.Locals(localsUserID, "player-1")
		return c.Next()
	}
	RegisterSandboxPurchase(app.Group("/v1.0"), auth, svc, nil)
	return app
}

func TestListSkusRouteRegisteredBeforeIDRoute(t *testing.T) {
	wallet := &fakeSandboxWallet{skus: []walletclient.SandboxSKU{{ID: "pack_100"}}}
	svc := sandboxpurchase.NewService(wallet, sandboxpurchase.NewStore(nil, "test"))
	app := newSandboxPurchaseApp(svc)

	req := httptest.NewRequest(http.MethodGet, "/v1.0/wallet/sandbox-purchase/skus", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /skus, got %d", resp.StatusCode)
	}
	var page struct {
		Data []walletclient.SandboxSKU `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	skus := page.Data
	if len(skus) != 1 || skus[0].ID != "pack_100" {
		t.Fatalf("unexpected skus: %+v", skus)
	}
}

// TestCreateSandboxPurchaseRouteRegistered proves POST / is wired without
// reaching sandboxpurchase.Store's DynamoDB write path — an unknown sku is
// rejected by Service.Create before it ever calls store.Create, so this test
// stays a pure-Go route/handler check with sandboxpurchase.NewStore(nil, ...).
func TestCreateSandboxPurchaseRouteRegistered(t *testing.T) {
	wallet := &fakeSandboxWallet{skus: []walletclient.SandboxSKU{{ID: "pack_100", PriceCents: 100, BaseCredits: 1000}}}
	svc := sandboxpurchase.NewService(wallet, sandboxpurchase.NewStore(nil, "test"))
	app := newSandboxPurchaseApp(svc)

	body, _ := json.Marshal(map[string]string{"sku": "not_a_real_sku"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/wallet/sandbox-purchase/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		t.Fatal("route not registered")
	}
}
