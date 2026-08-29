# Sandbox Credit Purchase (PIX via ctech-wallet) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a poker player buy sandbox credits with real PIX money via ctech-wallet's M2M sandbox-purchase endpoints, with poker tracking its own purchase history, verifying wallet webhooks by HMAC, and pushing a live balance-refresh event over the existing general websocket.

**Architecture:** Two repos. `ctech-wallet` gets two small additive changes (a `GET .../skus` M2M route, `expires_at` on purchase responses). `ctech-poker` gets a new `internal/sandboxpurchase` package (DynamoDB-backed history + wallet-facing business logic), new HTTP routes, an HMAC-verified webhook receiver, one new proto field, a new DynamoDB table, and a new `/store` frontend page built via the `/impeccable` skill.

**Tech Stack:** Go (Fiber v3, DynamoDB, `gopkg.aoctech.app/api-commons`), AWS CDK (TypeScript), Next.js App Router + TanStack Query, protobuf.

**Design doc:** `docs/specs/2026-07-30-sandbox-purchase-design.md` (approved).

## Global Constraints

- Poker's M2M client already has the `internal:wallet:sandbox-purchase` scope granted in ctech-account (user confirmed) — do not add scope-granting steps.
- `WALLET_WEBHOOK_HMAC_SECRET` must be the **same value** on both sides: poker's own env/SSM param (wired in Task 11) and ctech-wallet's SSM M2M-clients registry entry for poker's `client_id` (manual, out of scope for this plan, tracked as a blocker — generate with `openssl rand -hex 32`).
- Never trust the webhook body for money/state decisions — always re-`GET` the purchase from wallet before updating local state or broadcasting (mirrors ctech-wallet's own Invariant #11).
- All new poker HTTP routes are session-authenticated (existing `authMiddleware`/`localsUserID` pattern) except the wallet webhook, which is HMAC-authenticated instead.
- Go tests: `go test ./... -race`. Frontend: `npx vitest run`, `npx tsc --noEmit`, `npx eslint src --max-warnings 0`, `npm run build` — all zero-warning.
- No new abstractions beyond what's specified below (YAGNI) — e.g. no generic "purchase provider" interface, this is wallet-specific.

---

## Task 1 (ctech-wallet): Expose SKU catalog over M2M + JSON tags

**Repo:** `ctech-wallet`

**Files:**
- Modify: `api/internal/domain/wallet/sandbox_sku.go`
- Modify: `api/internal/api/v1/m2m_sandbox_purchase.go`
- Modify: `api/internal/api/v1/router.go:125-128`
- Test: `api/internal/api/v1/m2m_sandbox_purchase_test.go` (new)

**Interfaces:**
- Produces: `GET /v1.0/internal/wallet/sandbox-purchase/skus` (M2M, scope `internal:wallet:sandbox-purchase`) → JSON array of `{id, price_cents, base_credits, bonus_percent, total_credits}`.

- [ ] **Step 1: Add JSON tags to `SandboxSKU`**

In `api/internal/domain/wallet/sandbox_sku.go`, change the struct to:

```go
type SandboxSKU struct {
	ID           string `json:"id"`
	PriceCents   int64  `json:"price_cents"` // preço em centavos
	BaseCredits  int64  `json:"base_credits"` // créditos sem bônus
	BonusPercent int64  `json:"bonus_percent"` // percentual de bônus
}
```

- [ ] **Step 2: Add the handler**

Append to `api/internal/api/v1/m2m_sandbox_purchase.go`:

```go
import "gopkg.aoctech.app/wallet/api/internal/domain/wallet"

// m2mListSandboxSKUs is the M2M counterpart of the internal ListSKUs() —
// callers like ctech-poker need the catalog to render purchase options
// before opening a purchase.
func (h *handlers) m2mListSandboxSKUs(c fiber.Ctx) error {
	skus := wallet.ListSKUs()
	out := make([]fiber.Map, len(skus))
	for i, s := range skus {
		out[i] = fiber.Map{
			"id": s.ID, "price_cents": s.PriceCents, "base_credits": s.BaseCredits,
			"bonus_percent": s.BonusPercent, "total_credits": s.TotalCredits(),
		}
	}
	return c.JSON(out)
}
```

- [ ] **Step 3: Register the route before `/:id`**

In `api/internal/api/v1/router.go`, change:

```go
	sp := internal.Group("/wallet/sandbox-purchase", middleware.RequireScope(middleware.ScopeWalletSandboxPurchase))
	sp.Post("/", h.m2mPurchaseSandbox)
	sp.Get("/:id", h.m2mGetSandboxPurchase)
	sp.Post("/:id/refund", h.m2mRefundSandboxPurchase)
```

to:

```go
	sp := internal.Group("/wallet/sandbox-purchase", middleware.RequireScope(middleware.ScopeWalletSandboxPurchase))
	sp.Get("/skus", h.m2mListSandboxSKUs) // before /:id so "skus" never matches as a purchase id
	sp.Post("/", h.m2mPurchaseSandbox)
	sp.Get("/:id", h.m2mGetSandboxPurchase)
	sp.Post("/:id/refund", h.m2mRefundSandboxPurchase)
```

- [ ] **Step 4: Write the test**

Create `api/internal/api/v1/m2m_sandbox_purchase_test.go`:

```go
package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestM2MListSandboxSKUsRouteRegistered(t *testing.T) {
	app := newSandboxPurchaseTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/internal/wallet/sandbox-purchase/skus", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var skus []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&skus); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(skus) == 0 {
		t.Fatal("expected a non-empty SKU catalog")
	}
	first := skus[0]
	for _, field := range []string{"id", "price_cents", "base_credits", "bonus_percent", "total_credits"} {
		if _, ok := first[field]; !ok {
			t.Fatalf("expected field %q in SKU response, got %+v", field, first)
		}
	}
}

func newSandboxPurchaseTestApp(t *testing.T) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Use(recover.New())
	h := &handlers{}
	app.Get("/internal/wallet/sandbox-purchase/skus", h.m2mListSandboxSKUs)
	return app
}
```

Add the missing imports (`"github.com/gofiber/fiber/v3"`, `"github.com/gofiber/fiber/v3/middleware/recover"`) to match `internal_test.go`'s style.

- [ ] **Step 5: Run the test**

Run: `cd api && go test ./internal/api/v1/... -run TestM2MListSandboxSKUs -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add api/internal/domain/wallet/sandbox_sku.go api/internal/api/v1/m2m_sandbox_purchase.go api/internal/api/v1/router.go api/internal/api/v1/m2m_sandbox_purchase_test.go
git commit -m "feat(wallet): expose sandbox SKU catalog over M2M"
```

---

## Task 2 (ctech-wallet): Add `expires_at` to purchase responses

**Repo:** `ctech-wallet`

**Files:**
- Modify: `api/internal/api/v1/m2m_sandbox_purchase.go`
- Test: `api/internal/api/v1/m2m_sandbox_purchase_test.go`

**Interfaces:**
- Produces: `expires_at` (RFC3339 string) on `POST /sandbox-purchase/`, `GET /sandbox-purchase/:id`, `POST /sandbox-purchase/:id/refund` responses — derived from `wallet.SandboxPurchase.TTL` (unix seconds), which is already the purchase's true expiry (`sandboxPurchaseTTLMinutes` set at creation, see `services/sandbox_purchase.go:63`). No new stored field needed.

- [ ] **Step 1: Write the failing test for the pure helper**

Add to `api/internal/api/v1/m2m_sandbox_purchase_test.go`:

```go
func TestExpiresAtRFC3339(t *testing.T) {
	got := expiresAtRFC3339(1735689600) // 2025-01-01T00:00:00Z
	want := "2025-01-01T00:00:00Z"
	if got != want {
		t.Fatalf("expiresAtRFC3339(1735689600) = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `cd api && go test ./internal/api/v1/... -run TestExpiresAtRFC3339 -v`
Expected: FAIL with "undefined: expiresAtRFC3339"

- [ ] **Step 3: Add the helper and wire it into the three handlers**

In `api/internal/api/v1/m2m_sandbox_purchase.go`, add:

```go
import "time"

// expiresAtRFC3339 converts a SandboxPurchase's TTL (unix seconds, already
// the purchase's real expiry — see sandboxPurchaseTTLMinutes) into an
// RFC3339 string for the frontend countdown.
func expiresAtRFC3339(ttl int64) string {
	return time.Unix(ttl, 0).UTC().Format(time.RFC3339)
}
```

Update `m2mPurchaseSandbox` to add one field to its existing `fiber.Map`:

```go
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"purchase_id":      purchase.PurchaseID,
		"sku":              purchase.SKU,
		"amount":           purchase.AmountExpected,
		"credits_granted":  purchase.CreditsGranted,
		"status":           purchase.Status,
		"pix_copia_e_cola": charge.QRCode,
		"qr_code_base64":   charge.QRCodeB64,
		"expires_at":       expiresAtRFC3339(purchase.TTL),
	})
```

Update `m2mGetSandboxPurchase` and `m2mRefundSandboxPurchase` (both currently `return c.JSON(purchase)`) to embed the computed field instead of passing the bare struct through:

```go
// sandboxPurchaseWithExpiry adds the computed expires_at to a SandboxPurchase's
// JSON output without adding a stored field to the domain model — TTL is
// already the real expiry, this just formats it (see expiresAtRFC3339).
type sandboxPurchaseWithExpiry struct {
	*wallet.SandboxPurchase
	ExpiresAt string `json:"expires_at"`
}

func withExpiry(p *wallet.SandboxPurchase) sandboxPurchaseWithExpiry {
	return sandboxPurchaseWithExpiry{SandboxPurchase: p, ExpiresAt: expiresAtRFC3339(p.TTL)}
}
```

```go
func (h *handlers) m2mGetSandboxPurchase(c fiber.Ctx) error {
	client := middleware.GetClaims(c).AZP
	purchase, err := h.svc.GetSandboxPurchase(c.Context(), c.Params("id"), client)
	if err != nil {
		return sendProblem(c, err)
	}
	return c.JSON(withExpiry(purchase))
}
```

```go
func (h *handlers) m2mRefundSandboxPurchase(c fiber.Ctx) error {
	var body M2MRefundSandboxPurchaseRequest
	if p := bindJSON(c, &body); p != nil {
		return sendProblem(c, p)
	}
	client := middleware.GetClaims(c).AZP
	purchase, err := h.svc.RefundSandboxPurchase(c.Context(), body.UserID, c.Params("id"), body.IdempotencyKey, client)
	if err != nil {
		return sendProblem(c, err)
	}
	return c.JSON(withExpiry(purchase))
}
```

- [ ] **Step 4: Run the test**

Run: `cd api && go test ./internal/api/v1/... -run TestExpiresAtRFC3339 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/api/v1/m2m_sandbox_purchase.go api/internal/api/v1/m2m_sandbox_purchase_test.go
git commit -m "feat(wallet): surface expires_at on sandbox purchase M2M responses"
```

---

## Task 3 (ctech-poker api): Config — `WALLET_WEBHOOK_HMAC_SECRET`

**Repo:** `ctech-poker`

**Files:**
- Modify: `api/internal/config/config.go`
- Test: `api/internal/config/config_test.go` (create if absent, else extend)

**Interfaces:**
- Produces: `config.Config.WalletWebhookHMACSecret string` — consumed by Task 8's webhook handler and Task 9's wiring.

- [ ] **Step 1: Write the failing test**

Check whether `api/internal/config/config_test.go` exists (`ls api/internal/config/`). If absent, create it; if present, add this test alongside existing ones:

```go
func TestLoadRequiresWalletWebhookHMACSecretInProd(t *testing.T) {
	t.Setenv("ENVIRONMENT", "prod")
	t.Setenv("VALKEY_URL", "redis://x")
	t.Setenv("SERVICE_AUDIENCE", "https://poker.aoctech.app")
	t.Setenv("CTECH_URL", "https://accounts.aoctech.app")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://poker.aoctech.app")
	t.Setenv("WALLET_WEBHOOK_HMAC_SECRET", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error when WALLET_WEBHOOK_HMAC_SECRET is empty in prod")
	}
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `cd api && go test ./internal/config/... -run TestLoadRequiresWalletWebhookHMACSecret -v`
Expected: FAIL (no error returned today)

- [ ] **Step 3: Add the field and the fail-closed check**

In `api/internal/config/config.go`, add to the `Config` struct (near `PokerClientSecret`):

```go
	// WalletWebhookHMACSecret verifies inbound POST /v1.0/webhooks/wallet calls
	// (X-Wallet-Signature: sha256=<hex>) — must match the secret registered for
	// poker's client_id in ctech-wallet's SSM M2M-clients param (see this
	// plan's Global Constraints — a cross-repo/config blocker, not a code gap).
	WalletWebhookHMACSecret string `env:"WALLET_WEBHOOK_HMAC_SECRET"`
```

In `Load()`, add alongside the other prod-required checks:

```go
	if cfg.WalletWebhookHMACSecret == "" && cfg.Env == "prod" {
		return nil, fmt.Errorf("config: WALLET_WEBHOOK_HMAC_SECRET must be set in production so wallet webhooks can be verified")
	}
```

- [ ] **Step 4: Run the test**

Run: `cd api && go test ./internal/config/... -run TestLoadRequiresWalletWebhookHMACSecret -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/config/config.go api/internal/config/config_test.go
git commit -m "feat(poker): add WALLET_WEBHOOK_HMAC_SECRET config"
```

---

## Task 4 (ctech-poker api + ui): Proto — `sandbox_purchase_update` message

**Repo:** `ctech-poker`

**Files:**
- Modify: `proto/poker.proto`
- Generated (via script, do not hand-edit): `api/internal/api/v1/proto/poker.pb.go`, `ui/src/lib/api/proto/poker.ts`

**Interfaces:**
- Produces: `ServerMessage.PurchaseId` (Go) / `purchase_id` (TS) — field 19. Consumed by Task 8 (webhook broadcast) and Task 14 (frontend decode).

- [ ] **Step 1: Edit the proto**

In `proto/poker.proto`, update the `ServerMessage` doc comment and add field 19:

```proto
message ServerMessage {
  string type = 1; // "connected" | "pong" | "state" | "chat" | "error" | "removed" | "achievement_unlocked" | "room_created" | "room_updated" | "payment_received" | "system_broadcast" | "sandbox_purchase_update"
  
  // payload fields
  string conn_id = 2; // for connected frame
  TableSnapshot snapshot = 3; // for state frame
  string player_id = 4; // for chat frame, and destination for sandbox_purchase_update
  string message = 5; // for chat frame, error message, or removed reason
  string code = 6; // for error frame, removed frame, or sandbox_purchase_update status (confirmed|refunded|expired|failed)
  string key = 7; // for achievement unlocked frame
  int32 stars = 8; // for achievement unlocked frame
  
  // lobby payload fields
  Room room = 9; // for room_created
  string room_id = 10; // for room_updated
  int32 seats_taken = 11; // for room_updated
  int64 amount = 12; // for payment_received, or credits_granted for sandbox_purchase_update
  string text = 13; // for system_broadcast
  string action_id = 14; // correlation id for action_ack/error
  uint64 snapshot_version = 15; // for equity delta
  optional double equity = 16; // for equity delta (player_id identifies owner)
  string reaction_id = 17; // ephemeral table reaction catalog key
  string target_player_id = 18; // destination for thrown objects
  string purchase_id = 19; // for sandbox_purchase_update
}
```

- [ ] **Step 2: Regenerate**

Run: `./scripts/generate-proto.sh`
Expected: `api/internal/api/v1/proto/poker.pb.go` and `ui/src/lib/api/proto/poker.ts` both change (new `PurchaseId`/`purchase_id` field on `ServerMessage`); nothing else in either file changes structurally.

- [ ] **Step 3: Verify both sides build**

Run: `cd api && go build ./... && cd ../ui && npx tsc --noEmit`
Expected: both succeed.

- [ ] **Step 4: Commit**

```bash
git add proto/poker.proto api/internal/api/v1/proto/poker.pb.go ui/src/lib/api/proto/poker.ts
git commit -m "feat(proto): add purchase_id field for sandbox_purchase_update"
```

---

## Task 5 (ctech-poker api): `walletclient` — sandbox-purchase methods

**Repo:** `ctech-poker`

**Files:**
- Modify: `api/internal/walletclient/client.go`
- Test: `api/internal/walletclient/sandboxpurchase_test.go` (new)

**Interfaces:**
- Consumes: `Client.do`, `Client.sandboxPurchaseTokens` (new field), existing `walletError` helper.
- Produces (for Task 6): `walletclient.SandboxSKU{ID, PriceCents, BaseCredits, BonusPercent, TotalCredits}`; `walletclient.SandboxPurchase{PurchaseID, UserID, SKU, Amount, CreditsGranted, Status, PixCopiaECola, QRCodeBase64, ExpiresAt}`; `(*Client).ListSandboxSKUs(ctx) ([]SandboxSKU, error)`; `(*Client).PurchaseSandbox(ctx, userID, sku, idempotencyKey string) (*SandboxPurchase, error)`; `(*Client).GetSandboxPurchase(ctx, purchaseID string) (*SandboxPurchase, error)`; `(*Client).RefundSandboxPurchase(ctx, userID, purchaseID, idempotencyKey string) (*SandboxPurchase, error)`.

- [ ] **Step 1: Write the failing tests**

Create `api/internal/walletclient/sandboxpurchase_test.go`:

```go
package walletclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/config"
)

func TestListSandboxSKUs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/sandbox-purchase/skus", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "pack_100", "price_cents": 100, "base_credits": 1000, "bonus_percent": 0, "total_credits": 1000},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	skus, err := c.ListSandboxSKUs(t.Context())
	if err != nil {
		t.Fatalf("ListSandboxSKUs: %v", err)
	}
	if len(skus) != 1 || skus[0].ID != "pack_100" || skus[0].TotalCredits != 1000 {
		t.Fatalf("unexpected skus: %+v", skus)
	}
}

func TestPurchaseSandbox(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/sandbox-purchase", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"purchase_id": "sbxp#poker#user-1#k1", "sku": "pack_100", "amount": 100,
			"credits_granted": 1000, "status": "pending",
			"pix_copia_e_cola": "00020126...", "qr_code_base64": "iVBORw0...",
			"expires_at": "2026-07-30T12:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	p, err := c.PurchaseSandbox(t.Context(), "user-1", "pack_100", "k1")
	if err != nil {
		t.Fatalf("PurchaseSandbox: %v", err)
	}
	if p.PurchaseID != "sbxp#poker#user-1#k1" || p.Amount != 100 || p.CreditsGranted != 1000 || p.Status != "pending" {
		t.Fatalf("unexpected purchase: %+v", p)
	}
	if gotBody["user_id"] != "user-1" || gotBody["sku"] != "pack_100" || gotBody["idempotency_key"] != "k1" {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}
}

func TestGetSandboxPurchaseNormalizesAmount(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/sandbox-purchase/sbxp-1", func(w http.ResponseWriter, r *http.Request) {
		// ctech-wallet's GET/refund responses use amount_expected, not amount.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"purchase_id": "sbxp-1", "user_id": "user-1", "sku": "pack_100",
			"amount_expected": 100, "credits_granted": 1000, "status": "confirmed",
			"expires_at": "2026-07-30T12:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	p, err := c.GetSandboxPurchase(t.Context(), "sbxp-1")
	if err != nil {
		t.Fatalf("GetSandboxPurchase: %v", err)
	}
	if p.Amount != 100 {
		t.Fatalf("expected normalized Amount=100, got %d", p.Amount)
	}
	if p.UserID != "user-1" || p.Status != "confirmed" {
		t.Fatalf("unexpected purchase: %+v", p)
	}
}

func TestRefundSandboxPurchase(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/sandbox-purchase/sbxp-1/refund", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"purchase_id": "sbxp-1", "user_id": "user-1", "sku": "pack_100",
			"amount_expected": 100, "credits_granted": 1000, "status": "refunded",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	p, err := c.RefundSandboxPurchase(t.Context(), "user-1", "sbxp-1", "k2")
	if err != nil {
		t.Fatalf("RefundSandboxPurchase: %v", err)
	}
	if p.Status != "refunded" {
		t.Fatalf("unexpected status: %+v", p)
	}
	if gotBody["user_id"] != "user-1" || gotBody["idempotency_key"] != "k2" {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `cd api && go test ./internal/walletclient/... -run 'TestListSandboxSKUs|TestPurchaseSandbox|TestGetSandboxPurchaseNormalizesAmount|TestRefundSandboxPurchase' -v`
Expected: FAIL (compile error — `ListSandboxSKUs` etc. undefined)

- [ ] **Step 3: Implement in `client.go`**

Add to the `const` block near the other paths/scopes:

```go
	pathSandboxPurchaseSkus   = "/v1.0/internal/wallet/sandbox-purchase/skus"
	pathSandboxPurchaseCreate = "/v1.0/internal/wallet/sandbox-purchase"
	pathSandboxPurchaseGet    = "/v1.0/internal/wallet/sandbox-purchase/%s"
	pathSandboxPurchaseRefund = "/v1.0/internal/wallet/sandbox-purchase/%s/refund"

	scopeSandboxPurchase = "internal:wallet:sandbox-purchase"
```

Add a field to `Client` and wire it in `New`:

```go
type Client struct {
	base                  string
	http                  *http.Client
	creditTokens          *oauth2client.TokenManager
	debitTokens           *oauth2client.TokenManager
	debitRealTokens       *oauth2client.TokenManager
	gameHoldTokens        *oauth2client.TokenManager
	gameCashoutTokens     *oauth2client.TokenManager
	gameStatusTokens      *oauth2client.TokenManager
	balanceTokens         *oauth2client.TokenManager
	sandboxPurchaseTokens *oauth2client.TokenManager
	env                   string
	breakersMu            sync.Mutex
	breakers              map[string]breakerState
	retryDelay            func(time.Duration)
}
```

```go
		balanceTokens:         oauth2client.New(httpClient, cacheB, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeBalance),
		sandboxPurchaseTokens: oauth2client.New(httpClient, cacheB, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeSandboxPurchase),
```

Add the types and methods at the end of `client.go`:

```go
// SandboxSKU mirrors ctech-wallet's M2M GET .../sandbox-purchase/skus response.
type SandboxSKU struct {
	ID           string `json:"id"`
	PriceCents   int64  `json:"price_cents"`
	BaseCredits  int64  `json:"base_credits"`
	BonusPercent int64  `json:"bonus_percent"`
	TotalCredits int64  `json:"total_credits"`
}

// SandboxPurchase mirrors ctech-wallet's M2M sandbox-purchase response
// shapes. Amount/AmountExpected carry the same centavos value under two
// different wallet-side JSON keys (create vs get/refund responses) —
// normalizeAmount folds them into Amount so callers only ever read that field.
type SandboxPurchase struct {
	PurchaseID     string `json:"purchase_id"`
	UserID         string `json:"user_id"`
	SKU            string `json:"sku"`
	Amount         int64  `json:"amount"`
	AmountExpected int64  `json:"amount_expected"`
	CreditsGranted int64  `json:"credits_granted"`
	Status         string `json:"status"`
	PixCopiaECola  string `json:"pix_copia_e_cola,omitempty"`
	QRCodeBase64   string `json:"qr_code_base64,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
}

func (p *SandboxPurchase) normalizeAmount() {
	if p.Amount == 0 {
		p.Amount = p.AmountExpected
	}
}

// ListSandboxSKUs fetches the purchasable sandbox-credit pack catalog.
func (c *Client) ListSandboxSKUs(ctx context.Context) ([]SandboxSKU, error) {
	token, err := c.sandboxPurchaseTokens.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("walletclient: token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+pathSandboxPurchaseSkus, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.do(req, true)
	if err != nil {
		return nil, fmt.Errorf("walletclient: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, walletError(resp.StatusCode, raw)
	}
	var skus []SandboxSKU
	if err := json.NewDecoder(resp.Body).Decode(&skus); err != nil {
		return nil, fmt.Errorf("walletclient: decode: %w", err)
	}
	return skus, nil
}

// PurchaseSandbox opens a direct PIX→sandbox-credits sale on userID's behalf.
func (c *Client) PurchaseSandbox(ctx context.Context, userID, sku, idempotencyKey string) (*SandboxPurchase, error) {
	token, err := c.sandboxPurchaseTokens.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("walletclient: token: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"user_id": userID, "sku": sku, "idempotency_key": idempotencyKey,
	})
	if err != nil {
		return nil, fmt.Errorf("walletclient: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+pathSandboxPurchaseCreate, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(req, idempotencyKey != "")
	if err != nil {
		return nil, fmt.Errorf("walletclient: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, walletError(resp.StatusCode, raw)
	}
	var p SandboxPurchase
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("walletclient: decode: %w", err)
	}
	p.normalizeAmount()
	return &p, nil
}

// GetSandboxPurchase re-fetches a purchase's current status from wallet — the
// source of truth callers must consult before crediting or broadcasting
// anything (never the webhook body).
func (c *Client) GetSandboxPurchase(ctx context.Context, purchaseID string) (*SandboxPurchase, error) {
	token, err := c.sandboxPurchaseTokens.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("walletclient: token: %w", err)
	}
	url := fmt.Sprintf(c.base+pathSandboxPurchaseGet, purchaseID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.do(req, true)
	if err != nil {
		return nil, fmt.Errorf("walletclient: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, walletError(resp.StatusCode, raw)
	}
	var p SandboxPurchase
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("walletclient: decode: %w", err)
	}
	p.normalizeAmount()
	return &p, nil
}

// RefundSandboxPurchase reverses an unused sandbox purchase.
func (c *Client) RefundSandboxPurchase(ctx context.Context, userID, purchaseID, idempotencyKey string) (*SandboxPurchase, error) {
	token, err := c.sandboxPurchaseTokens.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("walletclient: token: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"user_id": userID, "idempotency_key": idempotencyKey,
	})
	if err != nil {
		return nil, fmt.Errorf("walletclient: encode: %w", err)
	}
	url := fmt.Sprintf(c.base+pathSandboxPurchaseRefund, purchaseID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(req, idempotencyKey != "")
	if err != nil {
		return nil, fmt.Errorf("walletclient: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, walletError(resp.StatusCode, raw)
	}
	var p SandboxPurchase
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("walletclient: decode: %w", err)
	}
	p.normalizeAmount()
	return &p, nil
}
```

- [ ] **Step 4: Run the tests**

Run: `cd api && go test ./internal/walletclient/... -v`
Expected: PASS (all, including pre-existing tests)

- [ ] **Step 5: Commit**

```bash
git add api/internal/walletclient/client.go api/internal/walletclient/sandboxpurchase_test.go
git commit -m "feat(poker): add walletclient sandbox-purchase methods"
```

---

## Task 6 (ctech-poker api): `internal/sandboxpurchase` package

**Repo:** `ctech-poker`

**Files:**
- Create: `api/internal/sandboxpurchase/store.go`
- Create: `api/internal/sandboxpurchase/service.go`
- Test: `api/internal/sandboxpurchase/service_test.go`

**Interfaces:**
- Consumes: `walletclient.SandboxSKU`, `walletclient.SandboxPurchase` (Task 5).
- Produces (for Tasks 7–9): `sandboxpurchase.Record{PlayerID, PurchaseID, SKU, PriceCents, BaseCredits, BonusPercent, TotalCredits, Status, PixCopiaECola, QRCodeBase64, ExpiresAt, CreatedAt, UpdatedAt}`; `sandboxpurchase.NewStore(db *dynamodb.Client, env string) *Store`; `sandboxpurchase.NewService(wallet, store) *Service`; `(*Service).ListSKUs`, `.Create`, `.List`, `.Refresh`, `.Refund`, `.ConfirmFromWebhook`; `sandboxpurchase.ErrNotFound`.

- [ ] **Step 1: Write `store.go`**

```go
package sandboxpurchase

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const tablePurchases = "poker_sandbox_purchases"

// Record is poker's own copy of a sandbox-credit purchase — a full history
// row, never TTL'd (unlike ctech-wallet's own pending-purchase row).
type Record struct {
	PlayerID      string `dynamodbav:"pk" json:"player_id"`
	PurchaseID    string `dynamodbav:"sk" json:"purchase_id"`
	SKU           string `dynamodbav:"sku" json:"sku"`
	PriceCents    int64  `dynamodbav:"price_cents" json:"price_cents"`
	BaseCredits   int64  `dynamodbav:"base_credits" json:"base_credits"`
	BonusPercent  int64  `dynamodbav:"bonus_percent" json:"bonus_percent"`
	TotalCredits  int64  `dynamodbav:"total_credits" json:"total_credits"`
	Status        string `dynamodbav:"status" json:"status"`
	PixCopiaECola string `dynamodbav:"pix_copia_e_cola,omitempty" json:"pix_copia_e_cola,omitempty"`
	QRCodeBase64  string `dynamodbav:"qr_code_base64,omitempty" json:"qr_code_base64,omitempty"`
	ExpiresAt     string `dynamodbav:"expires_at,omitempty" json:"expires_at,omitempty"`
	CreatedAt     string `dynamodbav:"created_at" json:"created_at"`
	UpdatedAt     string `dynamodbav:"updated_at" json:"updated_at"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tablePurchases)}
}

// Create persists rec, or — if a retried request already created it (same
// deterministic wallet purchase_id) — returns the existing row unchanged.
// Mirrors dailyreward.Store.Claim's conditional-put-then-reget idiom.
func (s *Store) Create(ctx context.Context, rec Record) (Record, error) {
	encoded, err := dynamo.Encode(rec)
	if err != nil {
		return Record{}, fmt.Errorf("sandboxpurchase: encode: %w", err)
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(encoded)}); err == nil {
		return rec, nil
	} else if !dynamo.IsConditionFailed(err) {
		return Record{}, fmt.Errorf("sandboxpurchase: persist: %w", err)
	}
	existing, err := s.base.GetItem(ctx, rec.PlayerID, rec.PurchaseID)
	if err != nil {
		return Record{}, fmt.Errorf("sandboxpurchase: load existing: %w", err)
	}
	if existing == nil {
		return Record{}, fmt.Errorf("sandboxpurchase: record disappeared")
	}
	decoded, err := dynamo.Decode[Record](existing)
	if err != nil {
		return Record{}, fmt.Errorf("sandboxpurchase: decode existing: %w", err)
	}
	return *decoded, nil
}

func (s *Store) Get(ctx context.Context, playerID, purchaseID string) (*Record, error) {
	item, err := s.base.GetItem(ctx, playerID, purchaseID)
	if err != nil {
		return nil, fmt.Errorf("sandboxpurchase: get: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	return dynamo.Decode[Record](item)
}

func (s *Store) UpdateStatus(ctx context.Context, playerID, purchaseID, status, updatedAt string) (bool, error) {
	sk := purchaseID
	return s.base.UpdateItem(ctx, playerID, &sk, map[string]any{"status": status, "updated_at": updatedAt})
}

func (s *Store) List(ctx context.Context, playerID string) ([]Record, error) {
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: playerID, Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("sandboxpurchase: list: %w", err)
	}
	out := make([]Record, 0, len(result.Items))
	for _, item := range result.Items {
		rec, err := dynamo.Decode[Record](item)
		if err != nil {
			return nil, fmt.Errorf("sandboxpurchase: decode: %w", err)
		}
		out = append(out, *rec)
	}
	return out, nil
}
```

- [ ] **Step 2: Write the failing service tests**

Create `api/internal/sandboxpurchase/service_test.go`:

```go
package sandboxpurchase

import (
	"context"
	"errors"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

type fakeWallet struct {
	skus            []walletclient.SandboxSKU
	purchase        *walletclient.SandboxPurchase
	getResult       *walletclient.SandboxPurchase
	refundResult    *walletclient.SandboxPurchase
	purchaseCalls   int
	lastIdemKey     string
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

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]Record{}} }
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
```

- [ ] **Step 3: Run the tests to confirm they fail**

Run: `cd api && go test ./internal/sandboxpurchase/... -v`
Expected: FAIL (package/service undefined)

- [ ] **Step 4: Write `service.go`**

```go
package sandboxpurchase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

var ErrNotFound = errors.New("sandboxpurchase: not found")

type wallet interface {
	ListSandboxSKUs(ctx context.Context) ([]walletclient.SandboxSKU, error)
	PurchaseSandbox(ctx context.Context, userID, sku, idempotencyKey string) (*walletclient.SandboxPurchase, error)
	GetSandboxPurchase(ctx context.Context, purchaseID string) (*walletclient.SandboxPurchase, error)
	RefundSandboxPurchase(ctx context.Context, userID, purchaseID, idempotencyKey string) (*walletclient.SandboxPurchase, error)
}

type store interface {
	Create(ctx context.Context, rec Record) (Record, error)
	Get(ctx context.Context, playerID, purchaseID string) (*Record, error)
	UpdateStatus(ctx context.Context, playerID, purchaseID, status, updatedAt string) (bool, error)
	List(ctx context.Context, playerID string) ([]Record, error)
}

type Service struct {
	wallet wallet
	store  store
	now    func() time.Time
}

func NewService(wallet wallet, store store) *Service {
	return &Service{wallet: wallet, store: store, now: time.Now}
}

func (s *Service) ListSKUs(ctx context.Context) ([]walletclient.SandboxSKU, error) {
	return s.wallet.ListSandboxSKUs(ctx)
}

// Create validates sku against the live catalog (so base_credits/bonus_percent
// can be recorded locally — wallet's purchase response only returns the
// total), opens the PIX charge, and persists the history row. A retry with
// the same idemKey is idempotent end to end: wallet derives the same
// purchase_id, and Store.Create returns the already-persisted row.
func (s *Service) Create(ctx context.Context, playerID, sku, idemKey string) (Record, error) {
	skus, err := s.wallet.ListSandboxSKUs(ctx)
	if err != nil {
		return Record{}, err
	}
	var def *walletclient.SandboxSKU
	for i := range skus {
		if skus[i].ID == sku {
			def = &skus[i]
			break
		}
	}
	if def == nil {
		return Record{}, fmt.Errorf("sandboxpurchase: unknown sku %q", sku)
	}

	purchase, err := s.wallet.PurchaseSandbox(ctx, playerID, sku, idemKey)
	if err != nil {
		return Record{}, err
	}

	now := s.now().UTC().Format(time.RFC3339Nano)
	rec := Record{
		PlayerID: playerID, PurchaseID: purchase.PurchaseID, SKU: purchase.SKU,
		PriceCents: def.PriceCents, BaseCredits: def.BaseCredits, BonusPercent: def.BonusPercent,
		TotalCredits: purchase.CreditsGranted, Status: purchase.Status,
		PixCopiaECola: purchase.PixCopiaECola, QRCodeBase64: purchase.QRCodeBase64,
		ExpiresAt: purchase.ExpiresAt, CreatedAt: now, UpdatedAt: now,
	}
	return s.store.Create(ctx, rec)
}

func (s *Service) List(ctx context.Context, playerID string) ([]Record, error) {
	records, err := s.store.List(ctx, playerID)
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt > records[j].CreatedAt })
	return records, nil
}

// Refresh re-fetches purchaseID from wallet (source of truth) and updates the
// local row if its status changed — the frontend's safety-net poll path.
func (s *Service) Refresh(ctx context.Context, playerID, purchaseID string) (Record, error) {
	local, err := s.store.Get(ctx, playerID, purchaseID)
	if err != nil {
		return Record{}, err
	}
	if local == nil {
		return Record{}, ErrNotFound
	}
	purchase, err := s.wallet.GetSandboxPurchase(ctx, purchaseID)
	if err != nil {
		return Record{}, err
	}
	if purchase.Status != local.Status {
		now := s.now().UTC().Format(time.RFC3339Nano)
		if _, err := s.store.UpdateStatus(ctx, playerID, purchaseID, purchase.Status, now); err != nil {
			return Record{}, fmt.Errorf("sandboxpurchase: update status: %w", err)
		}
		local.Status, local.UpdatedAt = purchase.Status, now
	}
	return *local, nil
}

func (s *Service) Refund(ctx context.Context, playerID, purchaseID, idemKey string) (Record, error) {
	local, err := s.store.Get(ctx, playerID, purchaseID)
	if err != nil {
		return Record{}, err
	}
	if local == nil {
		return Record{}, ErrNotFound
	}
	purchase, err := s.wallet.RefundSandboxPurchase(ctx, playerID, purchaseID, idemKey)
	if err != nil {
		return Record{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := s.store.UpdateStatus(ctx, playerID, purchaseID, purchase.Status, now); err != nil {
		return Record{}, fmt.Errorf("sandboxpurchase: update status: %w", err)
	}
	local.Status, local.UpdatedAt = purchase.Status, now
	return *local, nil
}

// ConfirmFromWebhook re-verifies purchaseID against wallet before ever acting
// on a webhook delivery — the webhook body itself is never trusted (mirrors
// ctech-wallet's own posture for its inbound PIX webhook). changed is false
// on a replay (status already matches) or when poker has no local row for a
// purchase_id wallet knows about.
func (s *Service) ConfirmFromWebhook(ctx context.Context, purchaseID string) (Record, bool, error) {
	purchase, err := s.wallet.GetSandboxPurchase(ctx, purchaseID)
	if err != nil {
		return Record{}, false, err
	}
	local, err := s.store.Get(ctx, purchase.UserID, purchaseID)
	if err != nil {
		return Record{}, false, err
	}
	if local == nil {
		return Record{}, false, nil
	}
	if local.Status == purchase.Status {
		return *local, false, nil
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := s.store.UpdateStatus(ctx, purchase.UserID, purchaseID, purchase.Status, now); err != nil {
		return Record{}, false, fmt.Errorf("sandboxpurchase: webhook update status: %w", err)
	}
	local.Status, local.UpdatedAt = purchase.Status, now
	return *local, true, nil
}
```

- [ ] **Step 5: Run the tests**

Run: `cd api && go test ./internal/sandboxpurchase/... -race -v`
Expected: PASS (all)

- [ ] **Step 6: Commit**

```bash
git add api/internal/sandboxpurchase/
git commit -m "feat(poker): add internal/sandboxpurchase store and service"
```

---

## Task 7 (ctech-poker api): HTTP routes — `/v1.0/wallet/sandbox-purchase`

**Repo:** `ctech-poker`

**Files:**
- Create: `api/internal/api/v1/sandboxpurchase.go`
- Test: `api/internal/api/v1/sandboxpurchase_test.go`

**Interfaces:**
- Consumes: `sandboxpurchase.Service` (Task 6), `problem` package, `localsUserID`, `RateLimiter`/`rateLimit`/`ipKey`, `walletOrInternalProblem` (already defined in `dailyreward.go`, same package).
- Produces: `RegisterSandboxPurchase(router fiber.Router, auth fiber.Handler, svc *sandboxpurchase.Service, purchaseLimiter *RateLimiter)` — mounted in Task 9.

- [ ] **Step 1: Write the failing tests**

Create `api/internal/api/v1/sandboxpurchase_test.go`:

```go
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

func TestCreateSandboxPurchase(t *testing.T) {
	wallet := &fakeSandboxWallet{
		skus:     []walletclient.SandboxSKU{{ID: "pack_100", PriceCents: 100, BaseCredits: 1000}},
		purchase: &walletclient.SandboxPurchase{PurchaseID: "sbxp-1", SKU: "pack_100", Amount: 100, CreditsGranted: 1000, Status: "pending"},
	}
	svc := sandboxpurchase.NewService(wallet, sandboxpurchase.NewStore(nil, "test"))
	_ = svc // store needs a real db only if Create's TransactWrite is reached; see note below

	app := newSandboxPurchaseApp(svc)
	body, _ := json.Marshal(map[string]string{"sku": "pack_100"})
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
	var skus []walletclient.SandboxSKU
	if err := json.NewDecoder(resp.Body).Decode(&skus); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(skus) != 1 || skus[0].ID != "pack_100" {
		t.Fatalf("unexpected skus: %+v", skus)
	}
}
```

`sandboxpurchase.NewStore(nil, "test")` is safe here because these two tests either never reach DynamoDB (`/skus` only calls the fake wallet) or the plan's Step 4 note applies (see below).

- [ ] **Step 2: Run to confirm failure**

Run: `cd api && go test ./internal/api/v1/... -run 'TestCreateSandboxPurchase|TestListSkusRouteRegisteredBeforeIDRoute' -v`
Expected: FAIL (`RegisterSandboxPurchase` undefined)

- [ ] **Step 3: Write `sandboxpurchase.go`**

```go
package v1

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"uuid"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
)

type sandboxPurchaseHandlers struct{ svc *sandboxpurchase.Service }

type SandboxPurchaseCreateRequest struct {
	SKU string `json:"sku"`
	// IdempotencyKey is stable per purchase click and reused across network
	// retries — mirrors JoinRoomRequest.IdempotencyKey's idem_key convention.
	IdempotencyKey string `json:"idem_key,omitempty"`
}

type SandboxPurchaseRefundRequest struct {
	IdempotencyKey string `json:"idem_key,omitempty"`
}

func RegisterSandboxPurchase(router fiber.Router, auth fiber.Handler, svc *sandboxpurchase.Service, purchaseLimiter *RateLimiter) {
	h := &sandboxPurchaseHandlers{svc: svc}
	g := router.Group("/wallet/sandbox-purchase", auth)
	g.Get("/skus", h.listSkus)
	g.Post("/", rateLimit(purchaseLimiter, ipKey("sandboxpurchase:create")), h.create)
	g.Get("/", h.list)
	g.Get("/:id", h.get)
	g.Post("/:id/refund", h.refund)
}

func (h *sandboxPurchaseHandlers) listSkus(c fiber.Ctx) error {
	skus, err := h.svc.ListSKUs(c.Context())
	if err != nil {
		return walletOrInternalProblem(err, "list skus failed", c).Send(c)
	}
	return c.JSON(skus)
}

func (h *sandboxPurchaseHandlers) create(c fiber.Ctx) error {
	var req SandboxPurchaseCreateRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	if req.SKU == "" {
		return problem.BadRequest("sku is required").Send(c)
	}
	idemKey := req.IdempotencyKey
	if idemKey == "" {
		idemKey = uuid.New().String()
	}
	userID := c.Locals(localsUserID).(string)
	rec, err := h.svc.Create(c.Context(), userID, req.SKU, idemKey)
	if err != nil {
		return walletOrInternalProblem(err, "purchase failed", c).Send(c)
	}
	return c.Status(fiber.StatusCreated).JSON(rec)
}

func (h *sandboxPurchaseHandlers) list(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	records, err := h.svc.List(c.Context(), userID)
	if err != nil {
		return problem.InternalServer("list purchases failed", c, err).Send(c)
	}
	return c.JSON(records)
}

func (h *sandboxPurchaseHandlers) get(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	rec, err := h.svc.Refresh(c.Context(), userID, c.Params("id"))
	if errors.Is(err, sandboxpurchase.ErrNotFound) {
		return problem.NotFound("purchase not found").Send(c)
	}
	if err != nil {
		return walletOrInternalProblem(err, "refresh purchase failed", c).Send(c)
	}
	return c.JSON(rec)
}

func (h *sandboxPurchaseHandlers) refund(c fiber.Ctx) error {
	var req SandboxPurchaseRefundRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	idemKey := req.IdempotencyKey
	if idemKey == "" {
		idemKey = uuid.New().String()
	}
	userID := c.Locals(localsUserID).(string)
	rec, err := h.svc.Refund(c.Context(), userID, c.Params("id"), idemKey)
	if errors.Is(err, sandboxpurchase.ErrNotFound) {
		return problem.NotFound("purchase not found").Send(c)
	}
	if err != nil {
		return walletOrInternalProblem(err, "refund failed", c).Send(c)
	}
	return c.JSON(rec)
}
```

- [ ] **Step 4: Run the tests**

Run: `cd api && go test ./internal/api/v1/... -run 'TestCreateSandboxPurchase|TestListSkusRouteRegisteredBeforeIDRoute' -v`
Expected: PASS. If `TestCreateSandboxPurchase` panics on a nil `*dynamodb.Client` (because `Create` reaches `TransactWrite`), change that one test to only assert the route exists (mirror `TestConfirmDepositRequiresScope`'s "nil service still proves wiring" style) rather than asserting a 201 — annotate why, same as that wallet test does.

- [ ] **Step 5: Commit**

```bash
git add api/internal/api/v1/sandboxpurchase.go api/internal/api/v1/sandboxpurchase_test.go
git commit -m "feat(poker): add /v1.0/wallet/sandbox-purchase HTTP routes"
```

---

## Task 8 (ctech-poker api): Wallet webhook receiver

**Repo:** `ctech-poker`

**Files:**
- Create: `api/internal/api/v1/walletwebhook.go`
- Test: `api/internal/api/v1/walletwebhook_test.go`

**Interfaces:**
- Consumes: `sandboxpurchase.Service.ConfirmFromWebhook` (Task 6), `ws.Registry` (existing), `pokerproto.ServerMessage.PurchaseId` (Task 4).
- Produces: `RegisterWalletWebhook(router fiber.Router, hmacSecret string, svc *sandboxpurchase.Service, reg ws.Registry)` — mounted in Task 9, unauthenticated route (HMAC replaces JWT auth here).

- [ ] **Step 1: Write the failing tests**

Create `api/internal/api/v1/walletwebhook_test.go`:

```go
package v1

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/ws"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
	goproto "google.golang.org/protobuf/proto"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

type recordingConn struct{ messages [][]byte }

func (r *recordingConn) WriteMessage(_ int, data []byte) error {
	r.messages = append(r.messages, data)
	return nil
}

func TestValidWalletWebhookSignature(t *testing.T) {
	body := []byte(`{"purchase_id":"sbxp-1"}`)
	valid := sign("secret", body)

	cases := []struct {
		name   string
		secret string
		header string
		want   bool
	}{
		{"valid", "secret", valid, true},
		{"wrong secret", "other", valid, false},
		{"tampered body handled by caller, header malformed here", "secret", "sha256=deadbeef", false},
		{"missing prefix", "secret", hex.EncodeToString([]byte("x")), false},
		{"empty secret", "", valid, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validWalletWebhookSignature(tc.secret, body, tc.header); got != tc.want {
				t.Fatalf("validWalletWebhookSignature(%q, body, %q) = %v, want %v", tc.secret, tc.header, got, tc.want)
			}
		})
	}
}

func TestWalletWebhookBroadcastsOnConfirm(t *testing.T) {
	wallet := &fakeSandboxWallet{purchase: &walletclient.SandboxPurchase{UserID: "player-1", Status: "confirmed"}}
	store := newFakeStoreForWebhookTest()
	store.rows[key("player-1", "sbxp-1")] = sandboxpurchase.Record{PlayerID: "player-1", PurchaseID: "sbxp-1", Status: "pending", TotalCredits: 1000}
	svc := sandboxpurchase.NewService(wallet, store)

	reg := ws.NewMemoryRegistry()
	conn := &recordingConn{}
	reg.Register("user#player-1", "conn-1", conn)

	app := fiber.New()
	RegisterWalletWebhook(app.Group("/v1.0"), "secret", svc, reg)

	body, _ := json.Marshal(map[string]string{"purchase_id": "sbxp-1"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/webhooks/wallet", bytes.NewReader(body))
	req.Header.Set("X-Wallet-Signature", sign("secret", body))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if len(conn.messages) != 1 {
		t.Fatalf("expected exactly one broadcast message, got %d", len(conn.messages))
	}
	var msg pokerproto.ServerMessage
	if err := goproto.Unmarshal(conn.messages[0], &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Type != "sandbox_purchase_update" || msg.PlayerId != "player-1" || msg.PurchaseId != "sbxp-1" || msg.Code != "confirmed" || msg.Amount != 1000 {
		t.Fatalf("unexpected message: %+v", &msg)
	}

	// Replay: same webhook again — status already confirmed, no second broadcast.
	req2 := httptest.NewRequest(http.MethodPost, "/v1.0/webhooks/wallet", bytes.NewReader(body))
	req2.Header.Set("X-Wallet-Signature", sign("secret", body))
	if _, err := app.Test(req2); err != nil {
		t.Fatalf("app.Test replay: %v", err)
	}
	if len(conn.messages) != 1 {
		t.Fatalf("expected replay not to broadcast again, got %d messages", len(conn.messages))
	}
}

func TestWalletWebhookRejectsBadSignature(t *testing.T) {
	svc := sandboxpurchase.NewService(&fakeSandboxWallet{}, newFakeStoreForWebhookTest())
	app := fiber.New()
	RegisterWalletWebhook(app.Group("/v1.0"), "secret", svc, ws.NewMemoryRegistry())

	body, _ := json.Marshal(map[string]string{"purchase_id": "sbxp-1"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/webhooks/wallet", bytes.NewReader(body))
	req.Header.Set("X-Wallet-Signature", "sha256=wrong")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// newFakeStoreForWebhookTest reuses the same in-memory fake as
// sandboxpurchase's own service tests would, defined locally here since it's
// a test-only type private to internal/sandboxpurchase — this file defines
// its own copy against the store interface's exported surface via Record.
func newFakeStoreForWebhookTest() *webhookFakeStore {
	return &webhookFakeStore{rows: map[string]sandboxpurchase.Record{}}
}

type webhookFakeStore struct{ rows map[string]sandboxpurchase.Record }

func (f *webhookFakeStore) Create(_ context.Context, rec sandboxpurchase.Record) (sandboxpurchase.Record, error) {
	f.rows[key(rec.PlayerID, rec.PurchaseID)] = rec
	return rec, nil
}
func (f *webhookFakeStore) Get(_ context.Context, playerID, purchaseID string) (*sandboxpurchase.Record, error) {
	rec, ok := f.rows[key(playerID, purchaseID)]
	if !ok {
		return nil, nil
	}
	return &rec, nil
}
func (f *webhookFakeStore) UpdateStatus(_ context.Context, playerID, purchaseID, status, updatedAt string) (bool, error) {
	k := key(playerID, purchaseID)
	rec, ok := f.rows[k]
	if !ok {
		return false, nil
	}
	rec.Status, rec.UpdatedAt = status, updatedAt
	f.rows[k] = rec
	return true, nil
}
func (f *webhookFakeStore) List(context.Context, string) ([]sandboxpurchase.Record, error) { return nil, nil }
```

Note: `sandboxpurchase.NewService`'s `store` parameter type is an unexported interface, so this test file (package `v1`) can pass any type satisfying that method set structurally — Go interface satisfaction doesn't require the interface to be exported. `key(...)` is the same tiny helper as Task 6's test; redefine it locally in this file (`func key(a, b string) string { return a + "#" + b }`) since it's not exported from `sandboxpurchase`.

- [ ] **Step 2: Run to confirm failure**

Run: `cd api && go test ./internal/api/v1/... -run 'TestValidWalletWebhookSignature|TestWalletWebhookBroadcastsOnConfirm|TestWalletWebhookRejectsBadSignature' -v`
Expected: FAIL (`validWalletWebhookSignature`, `RegisterWalletWebhook` undefined)

- [ ] **Step 3: Write `walletwebhook.go`**

```go
package v1

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/ws"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	goproto "google.golang.org/protobuf/proto"
)

const walletWebhookSignatureHeader = "X-Wallet-Signature"

// RegisterWalletWebhook mounts POST /v1.0/webhooks/wallet, unauthenticated by
// JWT — HMAC-SHA256 over the raw body against hmacSecret is the auth here,
// matching ctech-wallet's own outbound M2M webhook signing.
func RegisterWalletWebhook(router fiber.Router, hmacSecret string, svc *sandboxpurchase.Service, reg ws.Registry) {
	router.Post("/webhooks/wallet", walletWebhookHandler(hmacSecret, svc, reg))
}

func walletWebhookHandler(hmacSecret string, svc *sandboxpurchase.Service, reg ws.Registry) fiber.Handler {
	return func(c fiber.Ctx) error {
		body := c.Body()
		if !validWalletWebhookSignature(hmacSecret, body, c.Get(walletWebhookSignatureHeader)) {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		var payload struct {
			PurchaseID string `json:"purchase_id"`
		}
		if err := json.Unmarshal(body, &payload); err != nil || payload.PurchaseID == "" {
			return c.SendStatus(fiber.StatusBadRequest)
		}

		record, changed, err := svc.ConfirmFromWebhook(c.Context(), payload.PurchaseID)
		if err != nil {
			// Non-2xx makes ctech-wallet retry via its own reconcile sweep.
			slog.Error("wallet webhook: reverify failed", "purchase_id", payload.PurchaseID, "err", err)
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		if changed {
			data, err := goproto.Marshal(&pokerproto.ServerMessage{
				Type:       "sandbox_purchase_update",
				PlayerId:   record.PlayerID,
				PurchaseId: record.PurchaseID,
				Amount:     record.TotalCredits,
				Code:       record.Status,
			})
			if err == nil {
				reg.Broadcast(c.Context(), "user#"+record.PlayerID, data)
			}
		}
		return c.SendStatus(fiber.StatusOK)
	}
}

func validWalletWebhookSignature(secret string, body []byte, header string) bool {
	const prefix = "sha256="
	if secret == "" || !strings.HasPrefix(header, prefix) {
		return false
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(sig, mac.Sum(nil))
}
```

- [ ] **Step 4: Run the tests**

Run: `cd api && go test ./internal/api/v1/... -run 'TestValidWalletWebhookSignature|TestWalletWebhookBroadcastsOnConfirm|TestWalletWebhookRejectsBadSignature' -race -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/api/v1/walletwebhook.go api/internal/api/v1/walletwebhook_test.go
git commit -m "feat(poker): add HMAC-verified wallet webhook receiver"
```

---

## Task 9 (ctech-poker api): Wire it all together

**Repo:** `ctech-poker`

**Files:**
- Modify: `api/internal/app/app.go`
- Modify: `api/internal/api/v1/router.go`
- Modify: `api/internal/app/app_test.go`

**Interfaces:**
- Consumes: everything from Tasks 3, 5, 6, 7, 8.

- [ ] **Step 1: Add Fx providers in `app.go`**

Add after `newRouletteStore`/`newRouletteService` (mirror their shape exactly):

```go
func newSandboxPurchaseStore(db *dynamodb.Client, cfg *config.Config) *sandboxpurchase.Store {
	return sandboxpurchase.NewStore(db, cfg.Env)
}
func newSandboxPurchaseService(wallet *walletclient.Client, store *sandboxpurchase.Store) *sandboxpurchase.Service {
	return sandboxpurchase.NewService(wallet, store)
}
```

Add the import `"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"`.

Add both new functions to the `fx.Provide(...)` list (after `newRouletteService`):

```go
		newRouletteStore,
		newRouletteService,
		newSandboxPurchaseStore,
		newSandboxPurchaseService,
```

- [ ] **Step 2: Add a rate limiter and thread the new service through `registerRoutes`**

In `newFiberApp`'s caller area — actually the limiters are constructed in `v1.Register` today (see `RegisterDailyReward`'s `spinLimiter`), so no new provider is needed here; the limiter is built inside `v1.Register` like `spinLimiter` already is.

Update `registerRoutes`'s signature and body:

```go
func registerRoutes(
	app *fiber.App,
	cfg *config.Config,
	db *dynamodb.Client,
	verifier *jwtverify.Verifier,
	manager *tablemanager.Manager,
	reg ws.Registry,
	cacheBackend cache.Backend,
	rooms *roomstore.Store,
	buyinSvc *buyin.Service,
	players *player.Service,
	leaderboardSvc *leaderboard.Service,
	dailyRewardSvc *dailyreward.Service,
	tableStore *tablestore.Store,
	sessionStore *sessionlog.Store,
	achievementStore *achievements.Store,
	playerNoteStore *playernotes.Store,
	handShareStore *handshare.Store,
	pokerStatsStore *pokerstats.Store,
	avatars *avatar.Service,
	sandboxPurchaseSvc *sandboxpurchase.Service,
) {
	v1.Register(app, cfg, db, verifier, manager, reg, roomBackedSeed(rooms), cacheBackend, rooms, buyinSvc, players, leaderboardSvc, dailyRewardSvc, tableStore, sessionStore, achievementStore, playerNoteStore, handShareStore, pokerStatsStore, avatars, sandboxPurchaseSvc)
}
```

- [ ] **Step 3: Update `v1.Register`'s signature in `router.go`**

```go
func Register(
	app *fiber.App,
	cfg *config.Config,
	db *dynamodb.Client,
	verifier *jwtverify.Verifier,
	manager *tablemanager.Manager,
	reg ws.Registry,
	seed func(string) func() *hand.Table,
	cacheBackend cache.Backend,
	rooms *roomstore.Store,
	buyinSvc *buyin.Service,
	players *player.Service,
	leaderboardSvc *leaderboard.Service,
	dailyRewardSvc *dailyreward.Service,
	tableStore *tablestore.Store,
	sessionStore *sessionlog.Store,
	achievementStore *achievements.Store,
	playerNoteStore *playernotes.Store,
	handShareStore *handshare.Store,
	pokerStatsStore *pokerstats.Store,
	avatars *avatar.Service,
	sandboxPurchaseSvc *sandboxpurchase.Service,
) {
	router := app.Group("/v1.0")

	RegisterHealth(router, cfg, db)

	RegisterTableWS(router, verifier, manager, reg, cfg.CorsAllowedOrigins, seed, rooms, cfg, players, pokerStatsStore)
	RegisterGeneralWS(router, verifier, reg, cfg.CorsAllowedOrigins)
	auth := authMiddleware(verifier)
	RegisterHandHistory(router, auth, &tablestoreAdapter{store: tableStore})
	RegisterAchievementCatalog(router)
	RegisterWalletWebhook(router, cfg.WalletWebhookHMACSecret, sandboxPurchaseSvc, reg)

	createLimiter := NewRateLimiter(cacheBackend, 10, time.Minute)
	joinLimiter := NewRateLimiter(cacheBackend, 30, time.Minute)
	spinLimiter := NewRateLimiter(cacheBackend, 60, time.Minute)
	avatarLimiter := NewRateLimiter(cacheBackend, 5, time.Hour)
	purchaseLimiter := NewRateLimiter(cacheBackend, 10, time.Minute)

	RegisterRooms(router, auth, rooms, buyinSvc, manager, reg, cfg, createLimiter, joinLimiter)
	RegisterPlayers(router, auth, players, sessionStore, achievementStore, cfg, avatars, avatarLimiter, pokerStatsStore)
	RegisterPlayerNotes(router, auth, playerNoteStore)
	RegisterHandShares(router, auth, sessionStore, tableStore, handShareStore)
	RegisterPokerStats(router, auth, pokerStatsStore)
	RegisterLeaderboard(router, auth, leaderboardSvc)
	RegisterDailyReward(router, auth, dailyRewardSvc, spinLimiter)
	RegisterSandboxPurchase(router, auth, sandboxPurchaseSvc, purchaseLimiter)
}
```

Add the import `"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"` to `router.go`.

- [ ] **Step 4: Update `app_test.go`'s `registerRoutes` call**

Find the call (`registerRoutes(app, cfg, nil, verifier, manager, ws.NewMemoryRegistry(), nil, nil, (*buyin.Service)(nil), ...)`) and append one more trailing `nil` argument for `sandboxPurchaseSvc *sandboxpurchase.Service`. Add the same import there.

- [ ] **Step 5: Build and run the full backend test suite**

Run: `cd api && go build ./... && go test ./... -race`
Expected: all packages build and pass.

- [ ] **Step 6: Commit**

```bash
git add api/internal/app/app.go api/internal/app/app_test.go api/internal/api/v1/router.go
git commit -m "feat(poker): wire sandbox-purchase service and webhook into the app"
```

---

## Task 10 (ctech-poker cdk): `poker_sandbox_purchases` table

**Repo:** `ctech-poker`

**Files:**
- Modify: `cdk/lib/dynamodb-stack.ts`
- Test: existing CDK snapshot/assertion tests under `cdk/test/` (run, don't need new ones per se — check for a dynamodb-stack test first)

**Interfaces:**
- Produces: `dynamoStack.tables.get('poker_sandbox_purchases')` — consumed by Task 11 (table ARN threaded to the API stack's IAM policy).

- [ ] **Step 1: Check for an existing DynamoDB stack test**

Run: `ls cdk/test/ | grep -i dynamo`

- [ ] **Step 2: Add the table to the `TableName` union**

In `cdk/lib/dynamodb-stack.ts`, change:

```ts
export type TableName =
  'poker_table_state' | 'poker_table_state_history' | 'poker_action_log' | 'poker_action_guards' |
  'poker_rooms' | 'poker_player_profiles' | 'poker_achievement_progress' | 'poker_leaderboard_stats' |
  'poker_daily_reward' | 'poker_pending_cashouts' | 'poker_player_sessions' | 'poker_player_hands' |
  'poker_player_notes' | 'poker_hand_shares' | 'poker_player_poker_stats';
```

to:

```ts
export type TableName =
  'poker_table_state' | 'poker_table_state_history' | 'poker_action_log' | 'poker_action_guards' |
  'poker_rooms' | 'poker_player_profiles' | 'poker_achievement_progress' | 'poker_leaderboard_stats' |
  'poker_daily_reward' | 'poker_pending_cashouts' | 'poker_player_sessions' | 'poker_player_hands' |
  'poker_player_notes' | 'poker_hand_shares' | 'poker_player_poker_stats' | 'poker_sandbox_purchases';
```

- [ ] **Step 3: Add the table**

Add after the `poker_daily_reward` table call:

```ts
    // One row per purchase, pk=player_id sk=purchase_id — permanent history
    // (no TTL), unlike ctech-wallet's own pending-purchase row.
    table('poker_sandbox_purchases', true);
```

- [ ] **Step 4: Build and test the CDK app**

Run: `cd cdk && npm run build && npx jest` (or the repo's configured test runner — check `cdk/package.json` `scripts.test`)
Expected: compiles; existing snapshot tests pass (a new table doesn't affect other stacks' snapshots).

- [ ] **Step 5: Commit**

```bash
git add cdk/lib/dynamodb-stack.ts
git commit -m "feat(cdk): add poker_sandbox_purchases table"
```

---

## Task 11 (ctech-poker cdk): Wire `WALLET_WEBHOOK_HMAC_SECRET` + table ARN

**Repo:** `ctech-poker`

**Files:**
- Modify: `cdk/lib/constants.ts`
- Modify: `cdk/lib/api-stack.ts`
- Modify: `cdk/bin/poker.ts`

**Interfaces:**
- Produces: `WALLET_WEBHOOK_HMAC_SECRET` fetched from SSM into the instance's `start.sh` env, consumed by `config.Load()` (Task 3) at runtime.

- [ ] **Step 1: Add the SSM param path**

In `cdk/lib/constants.ts`, add to `SSM_POKER`:

```ts
export const SSM_POKER = (env: Environment) => ({
  walletUrl: `/ctech/${env}/poker/wallet-url`,
  clientId: `/ctech/${env}/poker/poker-client-id`,
  clientSecret: `/ctech/${env}/poker/poker-client-secret`,
  turnstileSecret: `/ctech/${env}/poker/turnstile-secret`,
  realMoneyEnabled: `/ctech/${env}/poker/real-money-enabled`,
  legalSignoffRef: `/ctech/${env}/poker/legal-signoff-ref`,
  avatarBaseUrl: `/ctech/${env}/poker/avatar-base-url`,
  // Verifies inbound ctech-wallet webhooks (X-Wallet-Signature). Must match
  // the secret registered for poker's client_id in ctech-wallet's own SSM
  // M2M-clients param — that registration is manual, done outside CDK.
  walletWebhookHmacSecret: `/ctech/${env}/poker/wallet-webhook-hmac-secret`,
});
```

- [ ] **Step 2: Add the prop and thread it through `api-stack.ts`**

Add to `ApiStackProps`:

```ts
  walletWebhookHmacSecretParam: string;
  sandboxPurchasesTableArn: string;
```

Destructure both in the constructor alongside the other params, and add `sandboxPurchasesTableArn` to the `tableArns` array (Step: `dynamodb:*` policy):

```ts
    const tableArns = [
      tableStateArn, tableStateHistoryArn, actionLogArn, actionGuardsArn, roomsTableArn, playerProfilesTableArn,
      achievementProgressTableArn, leaderboardStatsTableArn, dailyRewardTableArn, playerSessionsTableArn,
      playerHandsTableArn, playerNotesTableArn, handSharesTableArn, pokerStatsTableArn, sandboxPurchasesTableArn,
    ];
```

Add `walletWebhookHmacSecretParam` to the `ssm:GetParameter` resources list:

```ts
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['ssm:GetParameter'],
      resources: [
        shared.valkeyUrl, walletUrlParam, pokerClientIdParam, pokerClientSecretParam, turnstileSecretParam,
        realMoneyEnabledParam, legalSignoffRefParam, avatarBaseUrlParam, walletWebhookHmacSecretParam,
      ].map(
        (path) => `arn:${cdk.Aws.PARTITION}:ssm:${this.region}:${this.account}:parameter${path}`,
      ),
    }));
```

Add the fetch line to `start.sh` (mirroring `turnstileSecretParam`'s `--with-decryption` pattern), right after the `TURNSTILE_SECRET` block:

```ts
      `TURNSTILE_SECRET=$(aws ssm get-parameter --name "${turnstileSecretParam}" --with-decryption --query Parameter.Value --output text --region ${this.region} 2>/dev/null || echo "")`,
      `export TURNSTILE_SECRET`,
      `WALLET_WEBHOOK_HMAC_SECRET=$(aws ssm get-parameter --name "${walletWebhookHmacSecretParam}" --with-decryption --query Parameter.Value --output text --region ${this.region} 2>/dev/null || echo "")`,
      `export WALLET_WEBHOOK_HMAC_SECRET`,
```

- [ ] **Step 3: Pass the new params from `bin/poker.ts`**

In the `PokerApiStack` instantiation (the one with `avatarBaseUrlParam`, `tableStateArn`, etc.), add:

```ts
  walletWebhookHmacSecretParam: pokerParameters.walletWebhookHmacSecret,
  sandboxPurchasesTableArn: dynamoStack.tables.get('poker_sandbox_purchases')!.tableArn,
```

- [ ] **Step 4: Build**

Run: `cd cdk && npm run build`
Expected: compiles clean (TS will error on any missed prop wiring, since `ApiStackProps` fields are required).

- [ ] **Step 5: Commit**

```bash
git add cdk/lib/constants.ts cdk/lib/api-stack.ts cdk/bin/poker.ts
git commit -m "feat(cdk): wire WALLET_WEBHOOK_HMAC_SECRET and sandbox-purchases table ARN"
```

---

## Task 12 (ctech-poker ui): `lib/api/wallet.ts`

**Repo:** `ctech-poker`

**Files:**
- Create: `ui/src/lib/api/wallet.ts`
- Test: `ui/src/lib/api/wallet.test.ts`

**Interfaces:**
- Consumes: `apiClient` from `./client` (existing).
- Produces (for Task 15): `SandboxSKU`, `SandboxPurchase` types; `listSkus()`, `createPurchase(sku: string)`, `listPurchases()`, `getPurchase(id: string)`, `refundPurchase(id: string)`.

- [ ] **Step 1: Write the failing test**

Create `ui/src/lib/api/wallet.test.ts`:

```ts
import {describe, expect, test, vi} from 'vitest';

const get = vi.fn();
const post = vi.fn();
vi.mock('./client', () => ({apiClient: {get: (...a: unknown[]) => get(...a), post: (...a: unknown[]) => post(...a)}}));

import {createPurchase, getPurchase, listPurchases, listSkus, refundPurchase} from './wallet';

describe('wallet api', () => {
  test('listSkus GETs the catalog', async () => {
    get.mockResolvedValueOnce({data: [{id: 'pack_100', price_cents: 100, base_credits: 1000, bonus_percent: 0, total_credits: 1000}]});
    const skus = await listSkus();
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/sandbox-purchase/skus');
    expect(skus).toHaveLength(1);
  });

  test('createPurchase POSTs sku with a fresh idem_key', async () => {
    post.mockResolvedValueOnce({data: {purchase_id: 'sbxp-1', status: 'pending'}});
    await createPurchase('pack_100');
    expect(post).toHaveBeenCalledWith(
      '/v1.0/wallet/sandbox-purchase/',
      expect.objectContaining({sku: 'pack_100', idem_key: expect.any(String)}),
      {silentError: true},
    );
  });

  test('listPurchases GETs the history', async () => {
    get.mockResolvedValueOnce({data: []});
    await listPurchases();
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/sandbox-purchase/');
  });

  test('getPurchase GETs by id', async () => {
    get.mockResolvedValueOnce({data: {purchase_id: 'sbxp-1', status: 'confirmed'}});
    await getPurchase('sbxp-1');
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/sandbox-purchase/sbxp-1');
  });

  test('refundPurchase POSTs a fresh idem_key', async () => {
    post.mockResolvedValueOnce({data: {purchase_id: 'sbxp-1', status: 'refunded'}});
    await refundPurchase('sbxp-1');
    expect(post).toHaveBeenCalledWith(
      '/v1.0/wallet/sandbox-purchase/sbxp-1/refund',
      expect.objectContaining({idem_key: expect.any(String)}),
      {silentError: true},
    );
  });
});
```

- [ ] **Step 2: Run to confirm failure**

Run: `cd ui && npx vitest run src/lib/api/wallet.test.ts`
Expected: FAIL (module not found)

- [ ] **Step 3: Write `wallet.ts`**

```ts
import {apiClient} from './client';

export interface SandboxSKU {
  id: string;
  price_cents: number;
  base_credits: number;
  bonus_percent: number;
  total_credits: number;
}

export interface SandboxPurchase {
  player_id?: string;
  purchase_id: string;
  sku: string;
  price_cents?: number;
  base_credits?: number;
  bonus_percent?: number;
  total_credits?: number;
  status: string;
  pix_copia_e_cola?: string;
  qr_code_base64?: string;
  expires_at?: string;
  created_at?: string;
  updated_at?: string;
}

export async function listSkus() {
  return (await apiClient.get<SandboxSKU[]>('/v1.0/wallet/sandbox-purchase/skus')).data;
}

export async function createPurchase(sku: string) {
  // idem_key fresh per purchase click, stable across this click's own retries
  // — same convention as rooms.ts's joinRoom/leaveRoom.
  return (await apiClient.post<SandboxPurchase>(
    '/v1.0/wallet/sandbox-purchase/',
    {sku, idem_key: crypto.randomUUID()},
    {silentError: true},
  )).data;
}

export async function listPurchases() {
  return (await apiClient.get<SandboxPurchase[]>('/v1.0/wallet/sandbox-purchase/')).data;
}

export async function getPurchase(id: string) {
  return (await apiClient.get<SandboxPurchase>(`/v1.0/wallet/sandbox-purchase/${id}`)).data;
}

export async function refundPurchase(id: string) {
  return (await apiClient.post<SandboxPurchase>(
    `/v1.0/wallet/sandbox-purchase/${id}/refund`,
    {idem_key: crypto.randomUUID()},
    {silentError: true},
  )).data;
}
```

- [ ] **Step 4: Run the test**

Run: `cd ui && npx vitest run src/lib/api/wallet.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ui/src/lib/api/wallet.ts ui/src/lib/api/wallet.test.ts
git commit -m "feat(ui): add wallet sandbox-purchase API client"
```

---

## Task 13 (ctech-poker ui): `lib/api/dailyReward.ts`

**Repo:** `ctech-poker`

**Files:**
- Create: `ui/src/lib/api/dailyReward.ts`
- Test: `ui/src/lib/api/dailyReward.test.ts`

**Interfaces:**
- Produces (for Task 15): `getCooldown()`, `spin()` against the already-live `/v1.0/sandbox-credits/` backend routes (`api/internal/api/v1/dailyreward.go`, unmodified by this plan).

- [ ] **Step 1: Write the failing test**

Create `ui/src/lib/api/dailyReward.test.ts`:

```ts
import {describe, expect, test, vi} from 'vitest';

const get = vi.fn();
const post = vi.fn();
vi.mock('./client', () => ({apiClient: {get: (...a: unknown[]) => get(...a), post: (...a: unknown[]) => post(...a)}}));

import {getCooldown, spin} from './dailyReward';

describe('dailyReward api', () => {
  test('getCooldown GETs remaining time', async () => {
    get.mockResolvedValueOnce({data: {remaining_time_seconds: 3600}});
    const res = await getCooldown();
    expect(get).toHaveBeenCalledWith('/v1.0/sandbox-credits/');
    expect(res.remaining_time_seconds).toBe(3600);
  });

  test('spin POSTs and returns amount + remaining time', async () => {
    post.mockResolvedValueOnce({data: {amount: 10000, remaining_time_seconds: 86400}});
    const res = await spin();
    expect(post).toHaveBeenCalledWith('/v1.0/sandbox-credits/', undefined, {silentError: true});
    expect(res.amount).toBe(10000);
  });
});
```

- [ ] **Step 2: Run to confirm failure**

Run: `cd ui && npx vitest run src/lib/api/dailyReward.test.ts`
Expected: FAIL (module not found)

- [ ] **Step 3: Write `dailyReward.ts`**

```ts
import {apiClient} from './client';

export interface DailyRewardCooldown {
  remaining_time_seconds: number;
}

export interface DailyRewardSpinResult {
  amount: number;
  remaining_time_seconds: number;
}

export async function getCooldown() {
  return (await apiClient.get<DailyRewardCooldown>('/v1.0/sandbox-credits/')).data;
}

export async function spin() {
  return (await apiClient.post<DailyRewardSpinResult>('/v1.0/sandbox-credits/', undefined, {silentError: true})).data;
}
```

- [ ] **Step 4: Run the test**

Run: `cd ui && npx vitest run src/lib/api/dailyReward.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ui/src/lib/api/dailyReward.ts ui/src/lib/api/dailyReward.test.ts
git commit -m "feat(ui): add daily-reward API client"
```

---

## Task 14 (ctech-poker ui): `useLobbyRealtime.ts` — `sandbox_purchase_update`

**Repo:** `ctech-poker`

**Files:**
- Modify: `ui/src/lib/hooks/useLobbyRealtime.ts`
- Modify: `ui/src/lib/hooks/useLobbyRealtime.test.tsx`

**Interfaces:**
- Consumes: `purchase_id`/`type: 'sandbox_purchase_update'`/`code`/`amount` (Task 4's proto field, decoded as a plain object per `decodeServerMessage`).
- Produces: invalidates `['wallet', 'balance']` and a new `['wallet', 'sandbox-purchases']` query key — consumed by Task 15's page (`useQuery({queryKey: ['wallet', 'sandbox-purchases'], ...})`).

- [ ] **Step 1: Write the failing test**

Add to `ui/src/lib/hooks/useLobbyRealtime.test.tsx`, inside the existing `describe` block:

```tsx
  test('invalidates wallet queries and notifies on sandbox_purchase_update', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({type: 'sandbox_purchase_update', purchase_id: 'sbxp-1', code: 'confirmed', amount: 110000}));
    expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet', 'balance']});
    expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet', 'sandbox-purchases']});
    expect(state.notify).toHaveBeenCalledWith(expect.stringContaining('confirmada'), 'info');
  });
```

- [ ] **Step 2: Run to confirm failure**

Run: `cd ui && npx vitest run src/lib/hooks/useLobbyRealtime.test.tsx`
Expected: FAIL (no invalidation/notification happens for this message type today)

- [ ] **Step 3: Add the branch**

In `ui/src/lib/hooks/useLobbyRealtime.ts`, extend the `LobbyMessage` interface:

```ts
interface LobbyMessage {
  type: string;
  code?: string;
  room?: Room;
  room_id?: string;
  seats_taken?: number;
  amount?: number;
  text?: string;
  purchase_id?: string;
}
```

Add a branch in `receive` (after the `payment_received` branch):

```ts
    } else if (message.type === 'sandbox_purchase_update') {
      queryClient.invalidateQueries({queryKey: ['wallet', 'balance']});
      queryClient.invalidateQueries({queryKey: ['wallet', 'sandbox-purchases']});
      const statusLabel: Record<string, string> = {
        confirmed: 'Compra confirmada — créditos adicionados!',
        refunded: 'Compra estornada.',
        expired: 'Compra expirou sem pagamento.',
        failed: 'Falha na compra.',
      };
      pushNotification(statusLabel[message.code || ''] || 'Atualização na sua compra de créditos.', 'info');
    } else if (message.type === 'payment_received') {
```

(Order the new branch before `payment_received` or after — either works since they're mutually exclusive `else if` checks on `type`; keeping it adjacent to `payment_received` groups the two wallet-related notifications together.)

- [ ] **Step 4: Run the test**

Run: `cd ui && npx vitest run src/lib/hooks/useLobbyRealtime.test.tsx`
Expected: PASS (all tests in the file, including pre-existing ones)

- [ ] **Step 5: Commit**

```bash
git add ui/src/lib/hooks/useLobbyRealtime.ts ui/src/lib/hooks/useLobbyRealtime.test.tsx
git commit -m "feat(ui): handle sandbox_purchase_update over the lobby websocket"
```

---

## Task 15 (ctech-poker ui): `/store` page — via `/impeccable`

**Repo:** `ctech-poker`

**Files:**
- Create: `ui/src/app/store/page.tsx` (and any sub-components `/impeccable` decides to split out under `ui/src/components/store/`)
- Test: component test(s) alongside, per `/impeccable`'s own output convention

**Interfaces:**
- Consumes: `listSkus`, `createPurchase`, `listPurchases`, `getPurchase`, `refundPurchase` (Task 12); `getCooldown`, `spin` (Task 13); `['wallet', 'sandbox-purchases']` / `['wallet', 'balance']` query keys invalidated by Task 14; `FilterGroup` (`ui/src/components/FilterGroup.tsx`, reuse directly — do not wrap it); the achievements page's inline tab pattern (`ui/src/app/achievements/page.tsx:128-141`) as the concrete reference for the two-tab layout.

This is the one task in this plan that is a **UI/UX design task, not a mechanical code task** — per explicit instruction, it must go through the `/impeccable` skill rather than being hand-built from a prescribed diff.

- [ ] **Step 1: Confirm dependencies are in place**

Run: `cd ui && npx vitest run src/lib/api/wallet.test.ts src/lib/api/dailyReward.test.ts src/lib/hooks/useLobbyRealtime.test.tsx`
Expected: PASS (Tasks 12–14 already merged)

- [ ] **Step 2: Invoke `/impeccable` to design and build the page**

Brief for the `/impeccable` invocation:

> Build a new flat route `app/store/page.tsx` (matching the `app/achievements`, `app/leaderboard` convention — nav header with the same `ProfileMenu`/back-link pattern as `achievements/page.tsx`). Two tabs via an inline `FilterGroup` (mirror `achievements/page.tsx:128-141`'s usage exactly — do not create a new tabs wrapper), labeled **"Recompensas"** and **"Compras"**:
> - **Recompensas**: daily-reward widget using `getCooldown`/`spin` from `lib/api/dailyReward.ts` — cooldown countdown, spin button, amount-won feedback.
> - **Compras**: SKU grid (from `listSkus`) → tapping a pack calls `createPurchase(sku)` and opens a purchase modal showing the PIX QR (`qr_code_base64`), a copy-to-clipboard "copia e cola" code (`pix_copia_e_cola`), and a countdown to `expires_at`. While the modal is open, poll `getPurchase(id)` every 5 seconds as a safety net (the websocket in `useLobbyRealtime.ts` is the primary confirmation path — Task 14 already invalidates `['wallet','sandbox-purchases']` and shows a toast on `sandbox_purchase_update`). Below the SKU grid, a purchase-history list (`listPurchases`, query key `['wallet','sandbox-purchases']`) with a refund button shown only when `status === 'confirmed'` (calls `refundPurchase`).
> Follow `ui/CLAUDE.md`'s conventions (CSS in `globals.css`, no animation library, TanStack Query for all server data, `zod`/`react-hook-form` if a form is needed for anything, honor `prefers-reduced-motion`).

- [ ] **Step 3: Verify the quality gate**

Run: `cd ui && npx vitest run && npx tsc --noEmit && npx eslint src --max-warnings 0 && npm run build`
Expected: all pass with zero errors/warnings (per `ui/CLAUDE.md`'s quality gate).

- [ ] **Step 4: Manual check in a browser**

Start the dev server (`cd ui && npm run dev`), log in, navigate to `/store`, and walk both tabs: spin the daily reward (or confirm the cooldown display if already spun today), and open a purchase modal to confirm the QR/copia-e-cola/countdown render correctly. A real PIX payment can't be completed in dev — confirm the modal's poll and the websocket toast path at least don't error (mock or short-circuit the wallet call if `USE_MOCK` is in play, per `ui/src/dev/mockRuntime`).

- [ ] **Step 5: Commit**

```bash
git add ui/src/app/store/ ui/src/components/store/ 2>/dev/null
git commit -m "feat(ui): add /store page for sandbox-credit purchases and daily rewards"
```

---

## Final check

- [ ] Run the full backend suite: `cd api && go build ./... && go test ./... -race`
- [ ] Run the full frontend suite: `cd ui && npx vitest run && npx tsc --noEmit && npx eslint src --max-warnings 0 && npm run build`
- [ ] Run the wallet repo's suite for the two touched files: `cd ~/Documents/Projects/Ctech/ctech-wallet/api && go build ./... && go test ./internal/api/v1/... ./internal/domain/wallet/... -race`
- [ ] Confirm the CDK app still synthesizes: `cd cdk && npx cdk synth CtechPoker-Dev-API > /dev/null` (or the repo's actual stack id pattern from `bin/poker.ts`'s `id()` helper)
- [ ] Remind the user: generate `openssl rand -hex 32`, register it (plus poker's `client_id` and `https://<poker-domain>/v1.0/webhooks/wallet` as the webhook URL) in ctech-wallet's SSM M2M-clients registry, and set the same value as poker's `/ctech/{env}/poker/wallet-webhook-hmac-secret` SSM parameter — this plan wires the parameter path but does not populate it.
