# Premium Table Reactions Implementation Plan

> **Status (2026-08-12): implemented and hardened.** The unchecked boxes below preserve the original
> TDD execution script; they are not a current completion tracker. The final implementation includes
> the consistency corrections recorded in “Post-implementation hardening” at the end of this file.
> **Frontend completion (2026-08-13):** the separately deferred `/impeccable` pass is now implemented;
> see “Frontend implementation” at the end of this file.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make 6 `TABLE_REACTIONS` entries (`cold`, `fire`, `poop`, `rofl`, `knife`, `turtle`)
purchasable once, owned forever, refundable-if-never-used, favoritable (up to 3), and validated
server-side on every send — closing the pre-existing bug where `Actor.handleReaction` accepts any
`reaction_id` string with zero validation.

**Architecture:** A new `reactions` catalog package is the single source of truth for which IDs
exist and which are premium. A new `reactionpurchase` package owns ownership/history persistence and
dual-currency purchase (PIX via `ctech-wallet`'s new product-purchase M2M route, or a direct sandbox-
fichas debit). `Actor.handleReaction` gains a catalog + cached-ownership check. Favorites are a new
field on the existing `PlayerProfile`.

**Tech Stack:** Go 1.27.5, Fiber v3, AWS SDK v2 DynamoDB, `gopkg.aoctech.app/api-commons`
(`cache.Backend`/Valkey, `ws.Registry`), `uber-go/fx`, AWS CDK v2 (TypeScript).

**Spec:** `docs/specs/2026-08-12-premium-reactions.md` — depends on `ctech-wallet`'s
`docs/plans/2026-08-12-product-purchase-skus.md` being implemented and deployed first (this plan's
Task 3 calls the M2M routes it creates).

## Global Constraints

- Premium reaction IDs and their prices are **exactly** these 6, each a fixed multiple of the
  matching wallet SKU price — never derived from the sandbox-purchase exchange rate:

  | ID       | Type              | Real (PIX) | Fichas  | Wallet SKU              |
  |----------|-------------------|------------|---------|--------------------------|
  | `cold`   | emoji (untargeted)| R$ 1,00    | 100.000 | `poker_reaction_cold`    |
  | `fire`   | emoji (untargeted)| R$ 1,00    | 100.000 | `poker_reaction_fire`    |
  | `poop`   | targeted object   | R$ 5,00    | 500.000 | `poker_reaction_poop`    |
  | `rofl`   | targeted object   | R$ 5,00    | 500.000 | `poker_reaction_rofl`    |
  | `knife`  | targeted object   | R$ 5,00    | 500.000 | `poker_reaction_knife`   |
  | `turtle` | targeted object   | R$ 5,00    | 500.000 | `poker_reaction_turtle`  |

- Every other `TABLE_REACTIONS` ID is free and must keep working with no entitlement row and no
  DynamoDB read on the hot path.
- No admin UI/write path for the catalog — `internal/reactions/catalog.go` is a fixed Go table.
- No change to which reactions exist or their glyph/animation (`ui/src/lib/reactions.ts` untouched by
  this plan — frontend work is explicitly deferred to a separate `/impeccable` pass).
- Refund is allowed **only if `Entitlement.UsedAt == ""`** — first-use-wins, checked before either
  refund branch.
- `Actor.handleReaction` on an unknown `reaction_id` must be **rejected**, not silently accepted —
  this closes a real, pre-existing gap for free reactions too.
- The ownership check in front of `handleReaction` is **latency-only** caching (Valkey, 30s TTL) —
  never a correctness mechanism, same posture as `tablelease`.

---

### Task 1: `reactions` catalog package

**Files:**
- Create: `api/internal/reactions/catalog.go`
- Test: `api/internal/reactions/catalog_test.go`

**Interfaces:**
- Produces: `reactions.IsKnown(id string) bool`, `reactions.IsPremium(id string) bool`,
  `reactions.SKUFor(id string) (sku string, priceFichas int64, ok bool)`.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/reactions/catalog_test.go
package reactions

import "testing"

func TestIsKnownAndIsPremium(t *testing.T) {
	if !IsKnown("clap") || IsPremium("clap") {
		t.Fatal("clap must be known and free")
	}
	if !IsKnown("cold") || !IsPremium("cold") {
		t.Fatal("cold must be known and premium")
	}
	if IsKnown("not-a-real-reaction") {
		t.Fatal("unknown id must not be known")
	}
}

func TestSKUForPremiumMatchesPricingTable(t *testing.T) {
	cases := map[string]int64{
		"cold": 100_000, "fire": 100_000,
		"poop": 500_000, "rofl": 500_000, "knife": 500_000, "turtle": 500_000,
	}
	for id, wantFichas := range cases {
		sku, priceFichas, ok := SKUFor(id)
		if !ok || priceFichas != wantFichas || sku != "poker_reaction_"+id {
			t.Fatalf("SKUFor(%q) = %q, %d, %v; want poker_reaction_%s, %d, true", id, sku, priceFichas, ok, id, wantFichas)
		}
	}
}

func TestSKUForFreeReactionNotOK(t *testing.T) {
	if _, _, ok := SKUFor("clap"); ok {
		t.Fatal("SKUFor on a free reaction must return ok=false")
	}
}

func TestEveryFreeTableReactionIsKnown(t *testing.T) {
	// Mirrors ui/src/lib/reactions.ts's TABLE_REACTIONS keys — keep this list in
	// sync by hand on any change to that file (see docs/specs/2026-08-12-
	// premium-reactions.md's Catalog section for why this can't be build-time
	// coupled across languages).
	all := []string{
		"clap", "laugh", "wow", "angry", "cry", "nervous", "cold", "fire", "respect", "sleepy",
		"chip", "coffee", "clover", "horseshoe", "tear", "tomato", "poop", "rofl", "duck", "turtle", "knife", "flowers",
	}
	for _, id := range all {
		if !IsKnown(id) {
			t.Fatalf("catalog is missing frontend reaction id %q", id)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/reactions/... -v`
Expected: FAIL with "no Go files in ..." (package doesn't exist yet)

- [ ] **Step 3: Write minimal implementation**

```go
// api/internal/reactions/catalog.go
package reactions

// ReactionCatalogEntry is the poker-owned source of truth for which reaction
// IDs exist and which are premium — a game-design fact, not a money fact
// (docs/specs/2026-08-12-premium-reactions.md). PriceFichas is fixed here
// (never client-supplied); PriceCents for the same reaction is fixed in
// ctech-wallet's own productSKUCatalog and fetched at request time via
// walletclient.ListProductSKUs — never hardcoded locally.
type ReactionCatalogEntry struct {
	ID          string
	Premium     bool
	PriceFichas int64  // 0 if !Premium
	SKU         string // wallet ProductSKU ID. "" if !Premium.
}

var catalog = []ReactionCatalogEntry{
	{ID: "clap"}, {ID: "laugh"}, {ID: "wow"}, {ID: "angry"}, {ID: "cry"},
	{ID: "nervous"}, {ID: "respect"}, {ID: "sleepy"},
	{ID: "chip"}, {ID: "coffee"}, {ID: "clover"}, {ID: "horseshoe"}, {ID: "tear"},
	{ID: "tomato"}, {ID: "duck"}, {ID: "flowers"},

	{ID: "cold", Premium: true, PriceFichas: 100_000, SKU: "poker_reaction_cold"},
	{ID: "fire", Premium: true, PriceFichas: 100_000, SKU: "poker_reaction_fire"},
	{ID: "poop", Premium: true, PriceFichas: 500_000, SKU: "poker_reaction_poop"},
	{ID: "rofl", Premium: true, PriceFichas: 500_000, SKU: "poker_reaction_rofl"},
	{ID: "knife", Premium: true, PriceFichas: 500_000, SKU: "poker_reaction_knife"},
	{ID: "turtle", Premium: true, PriceFichas: 500_000, SKU: "poker_reaction_turtle"},
}

var byID = func() map[string]ReactionCatalogEntry {
	m := make(map[string]ReactionCatalogEntry, len(catalog))
	for _, e := range catalog {
		m[e.ID] = e
	}
	return m
}()

func IsKnown(id string) bool {
	_, ok := byID[id]
	return ok
}

func IsPremium(id string) bool {
	e, ok := byID[id]
	return ok && e.Premium
}

// SKUFor returns the wallet SKU and fichas price for a premium reaction, or
// ok=false for an unknown or free one.
func SKUFor(id string) (sku string, priceFichas int64, ok bool) {
	e, found := byID[id]
	if !found || !e.Premium {
		return "", 0, false
	}
	return e.SKU, e.PriceFichas, true
}

// All returns every catalog entry — used by ListCatalog (Task 4) to build the
// merged premium-flag + dual-price response.
func All() []ReactionCatalogEntry {
	return catalog
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/reactions/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/reactions/catalog.go api/internal/reactions/catalog_test.go
git commit -m "feat(reactions): add premium reaction catalog"
```

---

### Task 2: Ownership + history data model and stores

**Files:**
- Create: `api/internal/reactionpurchase/store.go`
- Test: `api/internal/reactionpurchase/store_test.go`

**Interfaces:**
- Produces: `reactionpurchase.Entitlement{PlayerID, ReactionID, PurchaseMethod, PurchaseID, UsedAt,
  CreatedAt}`, `reactionpurchase.Record{PlayerID, PurchaseID, ReactionID, Method, PriceCents,
  PriceFichas, Status, CreatedAt, UpdatedAt}`, `reactionpurchase.EntitlementStore` (`NewEntitlementStore(db
  *dynamodb.Client, env string) *EntitlementStore`, `.Put(ctx, Entitlement) error`, `.Get(ctx,
  playerID, reactionID string) (*Entitlement, error)`, `.MarkUsed(ctx, playerID, reactionID string)
  error`, `.Delete(ctx, playerID, reactionID string) error`), `reactionpurchase.Store`
  (`NewStore(db *dynamodb.Client, env string) *Store`, `.Create(ctx, Record) (Record, error)`,
  `.Get(ctx, playerID, purchaseID string) (*Record, error)`, `.UpdateStatus(ctx, playerID,
  purchaseID, status, updatedAt string) (bool, error)`, `.List(ctx, playerID string) ([]Record,
  error)`).

- [ ] **Step 1: Write the failing test**

```go
// api/internal/reactionpurchase/store_test.go
package reactionpurchase

import (
	"context"
	"testing"
)

func TestEntitlementStorePutGetMarkUsedDelete(t *testing.T) {
	db, env := newTestDynamo(t) // existing DynamoDB-Local test helper, mirror sandboxpurchase's own store_test.go
	s := NewEntitlementStore(db, env)
	ctx := context.Background()

	e := Entitlement{PlayerID: "player-1", ReactionID: "cold", PurchaseMethod: "fichas", PurchaseID: "rp-1", CreatedAt: "2026-08-12T00:00:00Z"}
	if err := s.Put(ctx, e); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get(ctx, "player-1", "cold")
	if err != nil || got == nil || got.UsedAt != "" {
		t.Fatalf("Get: %v, %+v", err, got)
	}
	if err := s.MarkUsed(ctx, "player-1", "cold"); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	got, err = s.Get(ctx, "player-1", "cold")
	if err != nil || got.UsedAt == "" {
		t.Fatalf("expected UsedAt set after MarkUsed, got %+v (err=%v)", got, err)
	}
	// First-use-wins: a second MarkUsed must not error and must not clobber the first timestamp.
	firstUsedAt := got.UsedAt
	if err := s.MarkUsed(ctx, "player-1", "cold"); err != nil {
		t.Fatalf("second MarkUsed: %v", err)
	}
	got, _ = s.Get(ctx, "player-1", "cold")
	if got.UsedAt != firstUsedAt {
		t.Fatalf("MarkUsed must be idempotent: got %q, want %q", got.UsedAt, firstUsedAt)
	}
	if err := s.Delete(ctx, "player-1", "cold"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err = s.Get(ctx, "player-1", "cold")
	if err != nil || got != nil {
		t.Fatalf("expected nil after Delete, got %+v (err=%v)", got, err)
	}
}

func TestStoreCreateIsIdempotent(t *testing.T) {
	db, env := newTestDynamo(t)
	s := NewStore(db, env)
	ctx := context.Background()
	rec := Record{PlayerID: "player-1", PurchaseID: "rp-2", ReactionID: "cold", Method: "fichas", PriceFichas: 100_000, Status: "confirmed", CreatedAt: "2026-08-12T00:00:00Z", UpdatedAt: "2026-08-12T00:00:00Z"}
	got1, err := s.Create(ctx, rec)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	got2, err := s.Create(ctx, rec)
	if err != nil || got2.PurchaseID != got1.PurchaseID {
		t.Fatalf("replay Create must return the existing row: %v, %+v", err, got2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/reactionpurchase/... -v`
Expected: FAIL — package doesn't exist yet

- [ ] **Step 3: Write minimal implementation**

```go
// api/internal/reactionpurchase/store.go
package reactionpurchase

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableEntitlements = "poker_reaction_entitlements"
	tablePurchases    = "poker_reaction_purchases"
)

// Entitlement is one row per **owned** premium reaction — free reactions
// never get a row; ownership of a free reaction is universal
// (docs/specs/2026-08-12-premium-reactions.md).
type Entitlement struct {
	PlayerID       string `dynamodbav:"pk" json:"player_id"`
	ReactionID     string `dynamodbav:"sk" json:"reaction_id"`
	PurchaseMethod string `dynamodbav:"purchase_method" json:"purchase_method"` // "pix" | "fichas"
	PurchaseID     string `dynamodbav:"purchase_id" json:"purchase_id"`
	UsedAt         string `dynamodbav:"used_at,omitempty" json:"used_at,omitempty"`
	CreatedAt      string `dynamodbav:"created_at" json:"created_at"`
}

// Record is purchase history — never TTL'd, mirrors poker_sandbox_purchases's shape.
type Record struct {
	PlayerID    string `dynamodbav:"pk" json:"player_id"`
	PurchaseID  string `dynamodbav:"sk" json:"purchase_id"`
	ReactionID  string `dynamodbav:"reaction_id" json:"reaction_id"`
	Method      string `dynamodbav:"method" json:"method"` // "pix" | "fichas"
	PriceCents  int64  `dynamodbav:"price_cents,omitempty" json:"price_cents,omitempty"`
	PriceFichas int64  `dynamodbav:"price_fichas,omitempty" json:"price_fichas,omitempty"`
	Status      string `dynamodbav:"status" json:"status"` // pending | confirmed | refunded
	CreatedAt   string `dynamodbav:"created_at" json:"created_at"`
	UpdatedAt   string `dynamodbav:"updated_at" json:"updated_at"`
}

type EntitlementStore struct{ base dynamo.Base }

func NewEntitlementStore(db *dynamodb.Client, env string) *EntitlementStore {
	return &EntitlementStore{base: dynamo.NewBase(db, env, tableEntitlements)}
}

func (s *EntitlementStore) Put(ctx context.Context, e Entitlement) error {
	encoded, err := dynamo.Encode(e)
	if err != nil {
		return fmt.Errorf("reactionpurchase: encode entitlement: %w", err)
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItem(encoded)}); err != nil {
		return fmt.Errorf("reactionpurchase: put entitlement: %w", err)
	}
	return nil
}

func (s *EntitlementStore) Get(ctx context.Context, playerID, reactionID string) (*Entitlement, error) {
	item, err := s.base.GetItem(ctx, playerID, reactionID)
	if err != nil {
		return nil, fmt.Errorf("reactionpurchase: get entitlement: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	return dynamo.Decode[Entitlement](item)
}

// MarkUsed is a conditional update setting used_at only if empty —
// first-use-wins, idempotent on replay.
func (s *EntitlementStore) MarkUsed(ctx context.Context, playerID, reactionID string) error {
	sk := reactionID
	_, err := s.base.UpdateItemConditional(ctx, playerID, &sk,
		map[string]any{"used_at": dynamo.NowStr()},
		"attribute_not_exists(used_at) OR used_at = :empty",
		map[string]any{":empty": ""},
	)
	if err != nil && !dynamo.IsConditionFailed(err) {
		return fmt.Errorf("reactionpurchase: mark used: %w", err)
	}
	return nil
}

func (s *EntitlementStore) Delete(ctx context.Context, playerID, reactionID string) error {
	if err := s.base.DeleteItem(ctx, playerID, reactionID); err != nil {
		return fmt.Errorf("reactionpurchase: delete entitlement: %w", err)
	}
	return nil
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tablePurchases)}
}

// Create persists rec, or returns the existing row unchanged on a retried
// request — mirrors sandboxpurchase.Store.Create's conditional-put-then-reget idiom.
func (s *Store) Create(ctx context.Context, rec Record) (Record, error) {
	encoded, err := dynamo.Encode(rec)
	if err != nil {
		return Record{}, fmt.Errorf("reactionpurchase: encode record: %w", err)
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(encoded)}); err == nil {
		return rec, nil
	} else if !dynamo.IsConditionFailed(err) {
		return Record{}, fmt.Errorf("reactionpurchase: persist record: %w", err)
	}
	existing, err := s.base.GetItem(ctx, rec.PlayerID, rec.PurchaseID)
	if err != nil {
		return Record{}, fmt.Errorf("reactionpurchase: load existing record: %w", err)
	}
	if existing == nil {
		return Record{}, fmt.Errorf("reactionpurchase: record disappeared")
	}
	decoded, err := dynamo.Decode[Record](existing)
	if err != nil {
		return Record{}, fmt.Errorf("reactionpurchase: decode existing record: %w", err)
	}
	return *decoded, nil
}

func (s *Store) Get(ctx context.Context, playerID, purchaseID string) (*Record, error) {
	item, err := s.base.GetItem(ctx, playerID, purchaseID)
	if err != nil {
		return nil, fmt.Errorf("reactionpurchase: get record: %w", err)
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
		return nil, fmt.Errorf("reactionpurchase: list records: %w", err)
	}
	out := make([]Record, 0, len(result.Items))
	for _, item := range result.Items {
		rec, err := dynamo.Decode[Record](item)
		if err != nil {
			return nil, fmt.Errorf("reactionpurchase: decode record: %w", err)
		}
		out = append(out, *rec)
	}
	return out, nil
}
```

Before writing this file, check `gopkg.aoctech.app/api-commons/dynamo`'s `Base` interface for the
exact names of a plain (non-conditional) put helper and a conditional-update-with-custom-expression
helper — `BuildPutTxItem`/`UpdateItemConditional`/`DeleteItem` above are the expected shapes mirroring
`dynamo.Base`'s other methods (`BuildPutTxItemIfAbsent`, `UpdateItem`, `GetItem` are already confirmed
in use by `sandboxpurchase/store.go`); if the exact method names differ, use the real ones and update
this task's code to match — do not invent a name that doesn't exist in the dependency.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/reactionpurchase/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/reactionpurchase/store.go api/internal/reactionpurchase/store_test.go
git commit -m "feat(reactionpurchase): add entitlement and purchase-history stores"
```

---

### Task 3: `walletclient` additions for the product-purchase M2M route

**Files:**
- Modify: `api/internal/walletclient/client.go`
- Test: `api/internal/walletclient/productpurchase_test.go`

**Interfaces:**
- Consumes: `oauth2client.TokenManager`, `cache.Backend` (existing, already used by `New`).
- Produces: `walletclient.ProductSKU{ID, PriceCents}`, `walletclient.ProductPurchase{PurchaseID,
  UserID, SKU, Amount, AmountExpected, Status, PixCopiaECola, QRCodeBase64, ExpiresAt}`,
  `Client.ListProductSKUs(ctx) ([]ProductSKU, error)`, `Client.PurchaseProduct(ctx, userID, sku,
  idempotencyKey string) (*ProductPurchase, error)`, `Client.GetProductPurchase(ctx, purchaseID
  string) (*ProductPurchase, error)`, `Client.RefundProductPurchase(ctx, userID, purchaseID,
  idempotencyKey string) (*ProductPurchase, error)`.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/walletclient/productpurchase_test.go
package walletclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPurchaseProductRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1.0/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
		case r.URL.Path == pathProductPurchaseCreate && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(ProductPurchase{PurchaseID: "prdp-1", SKU: "poker_reaction_cold", Amount: 100, Status: "pending", PixCopiaECola: "copia", QRCodeBase64: "qr"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL) // existing test helper, see sandboxpurchase_test.go in this package
	p, err := c.PurchaseProduct(context.Background(), "user-1", "poker_reaction_cold", "idem-1")
	if err != nil {
		t.Fatalf("PurchaseProduct: %v", err)
	}
	if p.PurchaseID != "prdp-1" || p.Amount != 100 {
		t.Fatalf("unexpected purchase: %+v", p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/walletclient/... -run TestPurchaseProductRoundTrip -v`
Expected: FAIL with "undefined: pathProductPurchaseCreate" / "undefined: ProductPurchase"

- [ ] **Step 3: Write minimal implementation**

Add to `client.go`'s const block:

```go
pathProductPurchaseSkus   = "/v1.0/internal/wallet/product-purchase/skus"
pathProductPurchaseCreate = "/v1.0/internal/wallet/product-purchase"
pathProductPurchaseGet    = "/v1.0/internal/wallet/product-purchase/%s"
pathProductPurchaseRefund = "/v1.0/internal/wallet/product-purchase/%s/refund"

scopeProductPurchase = "internal:wallet:product-purchase"
```

Add a `productPurchaseTokens *oauth2client.TokenManager` field to `Client`, initialize it in `New`
next to `sandboxPurchaseTokens`:

```go
productPurchaseTokens: oauth2client.New(httpClient, cacheB, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeProductPurchase),
```

Append the types and methods (literal structural copies of `SandboxSKU`/`SandboxPurchase` and
`ListSandboxSKUs`/`PurchaseSandbox`/`GetSandboxPurchase`/`RefundSandboxPurchase`, with
`credits_granted`/`base_credits`/`bonus_percent` dropped):

```go
// ProductSKU mirrors ctech-wallet's M2M GET .../product-purchase/skus response.
type ProductSKU struct {
	ID         string `json:"id"`
	PriceCents int64  `json:"price_cents"`
}

// ProductPurchase mirrors ctech-wallet's M2M product-purchase response shapes.
type ProductPurchase struct {
	PurchaseID     string `json:"purchase_id"`
	UserID         string `json:"user_id"`
	SKU            string `json:"sku"`
	Amount         int64  `json:"amount"`
	AmountExpected int64  `json:"amount_expected"`
	Status         string `json:"status"`
	PixCopiaECola  string `json:"pix_copia_e_cola,omitempty"`
	QRCodeBase64   string `json:"qr_code_base64,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
}

func (p *ProductPurchase) normalizeAmount() {
	if p.Amount == 0 {
		p.Amount = p.AmountExpected
	}
}

func (c *Client) ListProductSKUs(ctx context.Context) ([]ProductSKU, error) {
	token, err := c.productPurchaseTokens.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("walletclient: token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+pathProductPurchaseSkus, nil)
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
	var skus []ProductSKU
	if err := json.NewDecoder(resp.Body).Decode(&skus); err != nil {
		return nil, fmt.Errorf("walletclient: decode: %w", err)
	}
	return skus, nil
}

func (c *Client) PurchaseProduct(ctx context.Context, userID, sku, idempotencyKey string) (*ProductPurchase, error) {
	token, err := c.productPurchaseTokens.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("walletclient: token: %w", err)
	}
	body, err := json.Marshal(map[string]any{"user_id": userID, "sku": sku, "idempotency_key": idempotencyKey})
	if err != nil {
		return nil, fmt.Errorf("walletclient: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+pathProductPurchaseCreate, bytes.NewReader(body))
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
	var p ProductPurchase
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("walletclient: decode: %w", err)
	}
	p.normalizeAmount()
	return &p, nil
}

func (c *Client) GetProductPurchase(ctx context.Context, purchaseID string) (*ProductPurchase, error) {
	token, err := c.productPurchaseTokens.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("walletclient: token: %w", err)
	}
	url := fmt.Sprintf(c.base+pathProductPurchaseGet, purchaseID)
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
	var p ProductPurchase
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("walletclient: decode: %w", err)
	}
	p.normalizeAmount()
	return &p, nil
}

func (c *Client) RefundProductPurchase(ctx context.Context, userID, purchaseID, idempotencyKey string) (*ProductPurchase, error) {
	token, err := c.productPurchaseTokens.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("walletclient: token: %w", err)
	}
	body, err := json.Marshal(map[string]any{"user_id": userID, "idempotency_key": idempotencyKey})
	if err != nil {
		return nil, fmt.Errorf("walletclient: encode: %w", err)
	}
	url := fmt.Sprintf(c.base+pathProductPurchaseRefund, purchaseID)
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
	var p ProductPurchase
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("walletclient: decode: %w", err)
	}
	p.normalizeAmount()
	return &p, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/walletclient/... -run TestPurchaseProductRoundTrip -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/walletclient/client.go api/internal/walletclient/productpurchase_test.go
git commit -m "feat(walletclient): add product-purchase M2M methods"
```

---

### Task 4: `reactionpurchase.Service` — catalog + real (PIX) purchase + webhook confirm

**Files:**
- Create: `api/internal/reactionpurchase/service.go`
- Test: `api/internal/reactionpurchase/service_test.go`

**Interfaces:**
- Consumes: `reactions.SKUFor`/`reactions.All` (Task 1), `walletclient.ListProductSKUs`/
  `PurchaseProduct`/`GetProductPurchase` (Task 3), `EntitlementStore`/`Store` (Task 2).
- Produces: `reactionpurchase.CatalogEntry{ID, Premium, PriceCents, PriceFichas}`,
  `Service.ListCatalog(ctx) ([]CatalogEntry, error)`, `Service.CreateReal(ctx, playerID, reactionID,
  idemKey string) (Record, walletclient.ProductPurchase, error)`, `Service.ConfirmFromWebhook(ctx,
  purchaseID string) (Record, bool, error)`, `reactionpurchase.NewService(wallet wallet, entitlements
  *EntitlementStore, store *Store) *Service`, `reactionpurchase.ErrUnknownReaction`,
  `reactionpurchase.ErrNotPremium`.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/reactionpurchase/service_test.go
package reactionpurchase

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

type fakeWallet struct {
	skus     []walletclient.ProductSKU
	purchase *walletclient.ProductPurchase
	getResult *walletclient.ProductPurchase
}

func (f *fakeWallet) ListProductSKUs(context.Context) ([]walletclient.ProductSKU, error) { return f.skus, nil }
func (f *fakeWallet) PurchaseProduct(_ context.Context, _, _, _ string) (*walletclient.ProductPurchase, error) {
	return f.purchase, nil
}
func (f *fakeWallet) GetProductPurchase(context.Context, string) (*walletclient.ProductPurchase, error) {
	return f.getResult, nil
}
func (f *fakeWallet) RefundProductPurchase(context.Context, string, string, string) (*walletclient.ProductPurchase, error) {
	return nil, nil
}
func (f *fakeWallet) Debit(context.Context, string, int64, string, string) error { return nil }
func (f *fakeWallet) Credit(context.Context, string, int64, string, string) error { return nil }

func TestListCatalogMergesFreeAndPremium(t *testing.T) {
	w := &fakeWallet{skus: []walletclient.ProductSKU{{ID: "poker_reaction_cold", PriceCents: 100}}}
	svc := NewService(w, newTestEntitlementStore(t), newTestStore(t))
	entries, err := svc.ListCatalog(context.Background())
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
	_, _, err := svc.CreateReal(context.Background(), "player-1", "not-a-reaction", "idem-1")
	if err != ErrUnknownReaction {
		t.Fatalf("expected ErrUnknownReaction, got %v", err)
	}
}

func TestCreateRealFreeReactionRejected(t *testing.T) {
	svc := NewService(&fakeWallet{}, newTestEntitlementStore(t), newTestStore(t))
	_, _, err := svc.CreateReal(context.Background(), "player-1", "clap", "idem-1")
	if err != ErrNotPremium {
		t.Fatalf("expected ErrNotPremium, got %v", err)
	}
}

func TestCreateRealOpensNoEntitlementUntilConfirmed(t *testing.T) {
	w := &fakeWallet{purchase: &walletclient.ProductPurchase{PurchaseID: "prdp-1", SKU: "poker_reaction_cold", Amount: 100, Status: "pending"}}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(w, entitlements, store)
	rec, _, err := svc.CreateReal(context.Background(), "player-1", "cold", "idem-1")
	if err != nil {
		t.Fatalf("CreateReal: %v", err)
	}
	if rec.Status != "pending" || rec.Method != "pix" {
		t.Fatalf("unexpected record: %+v", rec)
	}
	got, err := entitlements.Get(context.Background(), "player-1", "cold")
	if err != nil || got != nil {
		t.Fatalf("no entitlement must exist until webhook confirms, got %+v (err=%v)", got, err)
	}
}

func TestConfirmFromWebhookGrantsEntitlement(t *testing.T) {
	w := &fakeWallet{
		purchase:  &walletclient.ProductPurchase{PurchaseID: "prdp-2", SKU: "poker_reaction_cold", Amount: 100, Status: "pending"},
		getResult: &walletclient.ProductPurchase{PurchaseID: "prdp-2", UserID: "player-1", SKU: "poker_reaction_cold", Amount: 100, Status: "confirmed"},
	}
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(w, entitlements, store)
	if _, _, err := svc.CreateReal(context.Background(), "player-1", "cold", "idem-2"); err != nil {
		t.Fatalf("CreateReal: %v", err)
	}
	rec, changed, err := svc.ConfirmFromWebhook(context.Background(), "prdp-2")
	if err != nil || !changed || rec.Status != "confirmed" {
		t.Fatalf("ConfirmFromWebhook: %v, changed=%v, rec=%+v", err, changed, rec)
	}
	got, err := entitlements.Get(context.Background(), "player-1", "cold")
	if err != nil || got == nil {
		t.Fatalf("expected entitlement after confirm, got %+v (err=%v)", got, err)
	}
}
```

(`newTestEntitlementStore`/`newTestStore` are small helpers wrapping Task 2's DynamoDB-Local
`newTestDynamo` — add them to a `_test_helpers.go` or inline in this test file.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/reactionpurchase/... -run 'TestListCatalog|TestCreateReal|TestConfirmFromWebhook' -v`
Expected: FAIL — `Service`/`NewService` don't exist yet

- [ ] **Step 3: Write minimal implementation**

```go
// api/internal/reactionpurchase/service.go
package reactionpurchase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"gopkg.aoctech.app/poker/api/internal/reactions"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

var (
	ErrNotFound       = errors.New("reactionpurchase: not found")
	ErrUnknownReaction = errors.New("reactionpurchase: unknown reaction")
	ErrNotPremium     = errors.New("reactionpurchase: reaction is not premium")
	ErrAlreadyUsed    = errors.New("reactionpurchase: reaction already used, cannot refund")
)

type wallet interface {
	ListProductSKUs(ctx context.Context) ([]walletclient.ProductSKU, error)
	PurchaseProduct(ctx context.Context, userID, sku, idempotencyKey string) (*walletclient.ProductPurchase, error)
	GetProductPurchase(ctx context.Context, purchaseID string) (*walletclient.ProductPurchase, error)
	RefundProductPurchase(ctx context.Context, userID, purchaseID, idempotencyKey string) (*walletclient.ProductPurchase, error)
	Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

// CatalogEntry merges reactions.catalog's premium flag/fichas price with
// wallet's own PriceCents — prices are never cached/hardcoded locally
// (docs/specs/2026-08-12-premium-reactions.md).
type CatalogEntry struct {
	ID          string `json:"id"`
	Premium     bool   `json:"premium"`
	PriceCents  int64  `json:"price_cents,omitempty"`
	PriceFichas int64  `json:"price_fichas,omitempty"`
}

type Service struct {
	wallet       wallet
	entitlements *EntitlementStore
	store        *Store
	now          func() time.Time
}

func NewService(w wallet, entitlements *EntitlementStore, store *Store) *Service {
	return &Service{wallet: w, entitlements: entitlements, store: store, now: time.Now}
}

func (s *Service) ListCatalog(ctx context.Context) ([]CatalogEntry, error) {
	skus, err := s.wallet.ListProductSKUs(ctx)
	if err != nil {
		return nil, err
	}
	priceBySKU := make(map[string]int64, len(skus))
	for _, sku := range skus {
		priceBySKU[sku.ID] = sku.PriceCents
	}
	all := reactions.All()
	out := make([]CatalogEntry, 0, len(all))
	for _, e := range all {
		entry := CatalogEntry{ID: e.ID, Premium: e.Premium}
		if e.Premium {
			entry.PriceFichas = e.PriceFichas
			entry.PriceCents = priceBySKU[e.SKU]
		}
		out = append(out, entry)
	}
	return out, nil
}

// CreateReal opens a real-money (PIX) purchase via wallet's product-purchase
// route. No entitlement row yet — same "confirm before granting" posture as
// sandbox credits; ConfirmFromWebhook is the only place a pix-method
// entitlement is created.
func (s *Service) CreateReal(ctx context.Context, playerID, reactionID, idemKey string) (Record, walletclient.ProductPurchase, error) {
	sku, priceFichas, ok := reactions.SKUFor(reactionID)
	if !ok {
		if !reactions.IsKnown(reactionID) {
			return Record{}, walletclient.ProductPurchase{}, ErrUnknownReaction
		}
		return Record{}, walletclient.ProductPurchase{}, ErrNotPremium
	}
	purchase, err := s.wallet.PurchaseProduct(ctx, playerID, sku, idemKey)
	if err != nil {
		return Record{}, walletclient.ProductPurchase{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	rec := Record{
		PlayerID: playerID, PurchaseID: purchase.PurchaseID, ReactionID: reactionID, Method: "pix",
		PriceCents: purchase.Amount, PriceFichas: priceFichas, Status: purchase.Status,
		CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.Create(ctx, rec)
	if err != nil {
		return Record{}, walletclient.ProductPurchase{}, err
	}
	return created, *purchase, nil
}

// ConfirmFromWebhook re-verifies purchaseID against wallet before ever acting
// on a webhook delivery — mirrors sandboxpurchase.Service.ConfirmFromWebhook.
// On a new "confirmed" status, writes the Entitlement row — the only place a
// pix-method entitlement is created.
func (s *Service) ConfirmFromWebhook(ctx context.Context, purchaseID string) (Record, bool, error) {
	purchase, err := s.wallet.GetProductPurchase(ctx, purchaseID)
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
		return Record{}, false, fmt.Errorf("reactionpurchase: webhook update status: %w", err)
	}
	local.Status, local.UpdatedAt = purchase.Status, now
	if purchase.Status == "confirmed" {
		if err := s.entitlements.Put(ctx, Entitlement{
			PlayerID: local.PlayerID, ReactionID: local.ReactionID, PurchaseMethod: "pix",
			PurchaseID: purchaseID, CreatedAt: now,
		}); err != nil {
			return Record{}, false, fmt.Errorf("reactionpurchase: grant entitlement: %w", err)
		}
	}
	return *local, true, nil
}

func (s *Service) List(ctx context.Context, playerID string) ([]Record, error) {
	records, err := s.store.List(ctx, playerID)
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt > records[j].CreatedAt })
	return records, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/reactionpurchase/... -run 'TestListCatalog|TestCreateReal|TestConfirmFromWebhook' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/reactionpurchase/service.go api/internal/reactionpurchase/service_test.go
git commit -m "feat(reactionpurchase): add catalog, CreateReal, ConfirmFromWebhook"
```

---

### Task 5: `reactionpurchase.Service` — sandbox-fichas purchase, refund, `MarkUsed`, `IsOwned`

**Files:**
- Modify: `api/internal/reactionpurchase/service.go`
- Test: append to `api/internal/reactionpurchase/service_test.go`

**Interfaces:**
- Produces: `Service.CreateSandbox(ctx, playerID, reactionID, idemKey string) (Record, error)`,
  `Service.Refund(ctx, playerID, purchaseID, idemKey string) (Record, error)`, `Service.MarkUsed(ctx,
  playerID, reactionID string) error`, `Service.IsOwned(ctx, playerID, reactionID string) (bool,
  error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestCreateSandboxGrantsEntitlementSynchronously(t *testing.T) {
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(&fakeWallet{}, entitlements, store)
	rec, err := svc.CreateSandbox(context.Background(), "player-1", "cold", "idem-3")
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if rec.Status != "confirmed" || rec.Method != "fichas" || rec.PriceFichas != 100_000 {
		t.Fatalf("unexpected record: %+v", rec)
	}
	owned, err := svc.IsOwned(context.Background(), "player-1", "cold")
	if err != nil || !owned {
		t.Fatalf("expected owned=true immediately, got %v (err=%v)", owned, err)
	}
}

func TestRefundRejectedWhenUsed(t *testing.T) {
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(&fakeWallet{}, entitlements, store)
	rec, err := svc.CreateSandbox(context.Background(), "player-1", "cold", "idem-4")
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if err := svc.MarkUsed(context.Background(), "player-1", "cold"); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	if _, err := svc.Refund(context.Background(), "player-1", rec.PurchaseID, "idem-refund-1"); err != ErrAlreadyUsed {
		t.Fatalf("expected ErrAlreadyUsed, got %v", err)
	}
}

func TestRefundSandboxHappyPathRevokesEntitlement(t *testing.T) {
	entitlements, store := newTestEntitlementStore(t), newTestStore(t)
	svc := NewService(&fakeWallet{}, entitlements, store)
	rec, err := svc.CreateSandbox(context.Background(), "player-1", "cold", "idem-5")
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	refunded, err := svc.Refund(context.Background(), "player-1", rec.PurchaseID, "idem-refund-2")
	if err != nil || refunded.Status != "refunded" {
		t.Fatalf("Refund: %v, %+v", err, refunded)
	}
	owned, err := svc.IsOwned(context.Background(), "player-1", "cold")
	if err != nil || owned {
		t.Fatalf("expected owned=false after refund, got %v (err=%v)", owned, err)
	}
}

func TestMarkUsedNoOpForFreeReaction(t *testing.T) {
	svc := NewService(&fakeWallet{}, newTestEntitlementStore(t), newTestStore(t))
	if err := svc.MarkUsed(context.Background(), "player-1", "clap"); err != nil {
		t.Fatalf("MarkUsed on a free reaction must be a no-op, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/reactionpurchase/... -run 'TestCreateSandbox|TestRefund|TestMarkUsed' -v`
Expected: FAIL — methods don't exist yet

- [ ] **Step 3: Write minimal implementation**

Append to `service.go`:

```go
// CreateSandbox debits priceFichas synchronously (no PIX involved) and, on
// success, writes both the history Record (status "confirmed") and the
// Entitlement row in one call — the sandbox debit is itself the
// confirmation, there is nothing async to wait for
// (docs/specs/2026-08-12-premium-reactions.md).
func (s *Service) CreateSandbox(ctx context.Context, playerID, reactionID, idemKey string) (Record, error) {
	_, priceFichas, ok := reactions.SKUFor(reactionID)
	if !ok {
		if !reactions.IsKnown(reactionID) {
			return Record{}, ErrUnknownReaction
		}
		return Record{}, ErrNotPremium
	}
	if err := s.wallet.Debit(ctx, playerID, priceFichas, idemKey, "reaction_purchase:"+reactionID); err != nil {
		return Record{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	purchaseID := "rp-" + idemKey // poker-minted id — no wallet purchase object exists for this leg
	rec, err := s.store.Create(ctx, Record{
		PlayerID: playerID, PurchaseID: purchaseID, ReactionID: reactionID, Method: "fichas",
		PriceFichas: priceFichas, Status: "confirmed", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return Record{}, err
	}
	if err := s.entitlements.Put(ctx, Entitlement{
		PlayerID: playerID, ReactionID: reactionID, PurchaseMethod: "fichas",
		PurchaseID: purchaseID, CreatedAt: now,
	}); err != nil {
		return Record{}, fmt.Errorf("reactionpurchase: grant entitlement: %w", err)
	}
	return rec, nil
}

// Refund loads the Record to get ReactionID/Method, then the Entitlement to
// check UsedAt, and branches on Method: pix reverses via wallet's
// product-purchase refund; fichas credits the price back directly. Either
// branch deletes the Entitlement and marks the Record refunded.
func (s *Service) Refund(ctx context.Context, playerID, purchaseID, idemKey string) (Record, error) {
	rec, err := s.store.Get(ctx, playerID, purchaseID)
	if err != nil {
		return Record{}, err
	}
	if rec == nil {
		return Record{}, ErrNotFound
	}
	entitlement, err := s.entitlements.Get(ctx, playerID, rec.ReactionID)
	if err != nil {
		return Record{}, err
	}
	if entitlement != nil && entitlement.UsedAt != "" {
		return Record{}, ErrAlreadyUsed
	}

	switch rec.Method {
	case "pix":
		if _, err := s.wallet.RefundProductPurchase(ctx, playerID, purchaseID, idemKey); err != nil {
			return Record{}, err
		}
	case "fichas":
		if err := s.wallet.Credit(ctx, playerID, rec.PriceFichas, idemKey, "reaction_refund:"+rec.ReactionID); err != nil {
			return Record{}, err
		}
	default:
		return Record{}, fmt.Errorf("reactionpurchase: unknown purchase method %q", rec.Method)
	}

	if err := s.entitlements.Delete(ctx, playerID, rec.ReactionID); err != nil {
		return Record{}, fmt.Errorf("reactionpurchase: revoke entitlement: %w", err)
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := s.store.UpdateStatus(ctx, playerID, purchaseID, "refunded", now); err != nil {
		return Record{}, fmt.Errorf("reactionpurchase: update status: %w", err)
	}
	rec.Status, rec.UpdatedAt = "refunded", now
	return *rec, nil
}

// MarkUsed is a no-op for a free reaction (no entitlement row exists) —
// callers only invoke it when reactions.IsPremium(id) is true; kept
// tolerant here too so a caller mistake never breaks a reaction send.
func (s *Service) MarkUsed(ctx context.Context, playerID, reactionID string) error {
	if !reactions.IsPremium(reactionID) {
		return nil
	}
	return s.entitlements.MarkUsed(ctx, playerID, reactionID)
}

// IsOwned is the uncached ownership check — Actor.handleReaction consults it
// through a Valkey-backed cache wrapper (Task 6), never directly on the hot
// path.
func (s *Service) IsOwned(ctx context.Context, playerID, reactionID string) (bool, error) {
	e, err := s.entitlements.Get(ctx, playerID, reactionID)
	if err != nil {
		return false, err
	}
	return e != nil, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/reactionpurchase/... -v`
Expected: PASS (full package suite)

- [ ] **Step 5: Commit**

```bash
git add api/internal/reactionpurchase/service.go api/internal/reactionpurchase/service_test.go
git commit -m "feat(reactionpurchase): add CreateSandbox, Refund, MarkUsed, IsOwned"
```

---

### Task 6: `Actor.handleReaction` catalog + cached-ownership validation

**Files:**
- Modify: `api/internal/table/actor.go`
- Modify: `api/internal/tablemanager/manager.go`
- Create: `api/internal/reactionpurchase/ownershipcache.go`
- Test: append to `api/internal/table/actor_test.go`, create
  `api/internal/reactionpurchase/ownershipcache_test.go`

**Interfaces:**
- Consumes: `cache.Backend` (`gopkg.aoctech.app/api-commons/cache`, existing — `Get(ctx, key)
  ([]byte, bool, error)`, `Set(ctx, key, value []byte, ttlSeconds int) error`), `reactionpurchase.
  Service.IsOwned`/`MarkUsed` (Tasks 4-5), `reactions.IsKnown`/`IsPremium` (Task 1).
- Produces: `reactionpurchase.OwnershipCache{}` (`NewOwnershipCache(svc *Service, backend
  cache.Backend) *OwnershipCache`, `.IsOwned(ctx, playerID, reactionID string) (bool, error)`),
  `Actor.SetReactionOwnershipForActor(fn func(ctx context.Context, playerID, reactionID string)
  (bool, error))`, `Actor.SetReactionMarkUsedForActor(fn func(ctx context.Context, playerID,
  reactionID string) error)`, `Manager.SetReactionOwnership(fn ...)`, `Manager.SetReactionMarkUsed(fn
  ...)`.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/reactionpurchase/ownershipcache_test.go
package reactionpurchase

import (
	"context"
	"testing"
)

type fakeCacheBackend struct {
	store map[string][]byte
	gets  int
}

func newFakeCacheBackend() *fakeCacheBackend { return &fakeCacheBackend{store: map[string][]byte{}} }
func (f *fakeCacheBackend) Get(_ context.Context, key string) ([]byte, bool, error) {
	f.gets++
	v, ok := f.store[key]
	return v, ok, nil
}
func (f *fakeCacheBackend) Set(_ context.Context, key string, value []byte, _ int) error {
	f.store[key] = value
	return nil
}
func (f *fakeCacheBackend) Delete(_ context.Context, key string) error { delete(f.store, key); return nil }
func (f *fakeCacheBackend) DeletePrefix(context.Context, string) error { return nil }
func (f *fakeCacheBackend) Ping(context.Context) error                { return nil }

type countingIsOwnedService struct{ calls int; owned bool }

func (s *countingIsOwnedService) IsOwned(context.Context, string, string) (bool, error) {
	s.calls++
	return s.owned, nil
}

func TestOwnershipCacheHitsBackendOnceWithinTTL(t *testing.T) {
	backend := newFakeCacheBackend()
	svc := &countingIsOwnedService{owned: true}
	c := NewOwnershipCache(svc, backend)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		owned, err := c.IsOwned(ctx, "player-1", "cold")
		if err != nil || !owned {
			t.Fatalf("call %d: owned=%v err=%v", i, owned, err)
		}
	}
	if svc.calls != 1 {
		t.Fatalf("expected exactly 1 underlying IsOwned call, got %d", svc.calls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/reactionpurchase/... -run TestOwnershipCache -v`
Expected: FAIL — `NewOwnershipCache` doesn't exist yet

- [ ] **Step 3: Write minimal implementation**

```go
// api/internal/reactionpurchase/ownershipcache.go
package reactionpurchase

import (
	"context"

	"gopkg.aoctech.app/api-commons/cache"
)

const ownershipCacheTTLSeconds = 30

type isOwnedChecker interface {
	IsOwned(ctx context.Context, playerID, reactionID string) (bool, error)
}

// OwnershipCache wraps Service.IsOwned behind a Valkey-backed cache — latency-
// only, never a correctness mechanism (same category as tablelease). A stale
// "not owned" for up to 30s after a purchase just means the player's first
// attempt right after buying can delay-fail once; the purchase flow already
// returns success from the entitlement write, so the frontend treats
// "just bought" as owned optimistically without waiting on this cache
// (docs/specs/2026-08-12-premium-reactions.md).
type OwnershipCache struct {
	svc     isOwnedChecker
	backend cache.Backend
}

func NewOwnershipCache(svc isOwnedChecker, backend cache.Backend) *OwnershipCache {
	return &OwnershipCache{svc: svc, backend: backend}
}

func (c *OwnershipCache) IsOwned(ctx context.Context, playerID, reactionID string) (bool, error) {
	key := "reaction-owned:" + playerID + ":" + reactionID
	if cached, ok, _ := c.backend.Get(ctx, key); ok && len(cached) == 1 {
		return cached[0] == '1', nil
	}
	owned, err := c.svc.IsOwned(ctx, playerID, reactionID)
	if err != nil {
		return false, err
	}
	value := byte('0')
	if owned {
		value = '1'
	}
	_ = c.backend.Set(ctx, key, []byte{value}, ownershipCacheTTLSeconds)
	return owned, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/reactionpurchase/... -run TestOwnershipCache -v`
Expected: PASS

Now wire it into `Actor`. In `api/internal/table/actor.go`, add a field and setter next to the other
`SetXForActor` hooks (around line 197's `SetSystemSettlementIntentForActor`):

```go
// reactionOwnership/reactionMarkUsed are nil-checked no-ops until wired by
// the manager — mirrors every other optional hook on Actor (onHandComplete,
// systemSettlementIntent, etc).
reactionOwnership func(ctx context.Context, playerID, reactionID string) (bool, error)
reactionMarkUsed  func(ctx context.Context, playerID, reactionID string) error
```

```go
func (a *Actor) SetReactionOwnershipForActor(fn func(ctx context.Context, playerID, reactionID string) (bool, error)) {
	a.reactionOwnership = fn
}
func (a *Actor) SetReactionMarkUsedForActor(fn func(ctx context.Context, playerID, reactionID string) error) {
	a.reactionMarkUsed = fn
}
```

Replace `handleReaction` (lines 315-346) with:

```go
func (a *Actor) handleReaction(ctx context.Context, c ReactionCmd) error {
	if c.ActionID == "" {
		return errors.New("table: action_id is required")
	}
	if c.ReactionID == "" {
		return errors.New("table: reaction_id is required")
	}
	if !reactions.IsKnown(c.ReactionID) {
		return errors.New("table: unknown reaction_id")
	}
	if reactions.IsPremium(c.ReactionID) {
		if a.reactionOwnership == nil {
			return errors.New("table: reaction ownership check unavailable")
		}
		owned, err := a.reactionOwnership(ctx, c.PlayerID, c.ReactionID)
		if err != nil {
			return err
		}
		if !owned {
			return errors.New("table: reaction not owned")
		}
	}
	// Reactions already have a dedicated fan-out frame. Persist them so a
	// reconnect can restore the short-lived effect, but do not also broadcast
	// a full table snapshot for the same cosmetic action.
	return a.commitActivity(ctx, false, func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		if c.TargetPlayerID != "" && (c.TargetPlayerID == c.PlayerID || !a.isSeated(c.TargetPlayerID)) {
			return errors.New("table: invalid reaction target")
		}
		a.markLastAction(c.PlayerID)
		now := timeNowFunc().UnixMilli()
		a.activity.Reactions = append(a.activity.Reactions, tablestore.Reaction{
			ID: c.ActionID, PlayerID: c.PlayerID, ReactionID: c.ReactionID,
			TargetPlayerID: c.TargetPlayerID, Timestamp: now, ExpiresAt: now + reactionLifetime.Milliseconds(),
		})
		if len(a.activity.Reactions) > maxPersistedReactions {
			a.activity.Reactions = append([]tablestore.Reaction(nil), a.activity.Reactions[len(a.activity.Reactions)-maxPersistedReactions:]...)
		}
		if reactions.IsPremium(c.ReactionID) && a.reactionMarkUsed != nil {
			_ = a.reactionMarkUsed(ctx, c.PlayerID, c.ReactionID) // best-effort, never blocks the reaction itself
		}
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "reaction",
			ReactionID: c.ReactionID, TargetPlayerID: c.TargetPlayerID,
		})
	})
}
```

Add `"gopkg.aoctech.app/poker/api/internal/reactions"` to `actor.go`'s import block.

In `api/internal/tablemanager/manager.go`, add fields + setters mirroring
`systemSettlementIntent`/`SetSystemSettlementIntent` (around lines 50/341):

```go
reactionOwnership func(ctx context.Context, playerID, reactionID string) (bool, error)
reactionMarkUsed  func(ctx context.Context, playerID, reactionID string) error
```

```go
func (m *Manager) SetReactionOwnership(fn func(ctx context.Context, playerID, reactionID string) (bool, error)) {
	m.reactionOwnership = fn
}
func (m *Manager) SetReactionMarkUsed(fn func(ctx context.Context, playerID, reactionID string) error) {
	m.reactionMarkUsed = fn
}
```

In `getOrCreateActor` (around line 192, next to `actor.SetSystemSettlementIntentForActor`):

```go
actor.SetReactionOwnershipForActor(func(ctx context.Context, playerID, reactionID string) (bool, error) {
	if m.reactionOwnership == nil {
		return false, errors.New("tablemanager: reaction ownership check unavailable")
	}
	return m.reactionOwnership(ctx, playerID, reactionID)
})
actor.SetReactionMarkUsedForActor(func(ctx context.Context, playerID, reactionID string) error {
	if m.reactionMarkUsed == nil {
		return nil
	}
	return m.reactionMarkUsed(ctx, playerID, reactionID)
})
```

Add test cases to `actor_test.go`: unknown `reaction_id` rejected before `isSeated` is even checked,
premium+not-owned rejected, premium+owned accepted and calls the injected `reactionMarkUsed` fake,
free reaction works with `reactionOwnership` left nil (no ownership call at all).

- [ ] **Step 5: Commit**

```bash
git add api/internal/reactionpurchase/ownershipcache.go api/internal/reactionpurchase/ownershipcache_test.go api/internal/table/actor.go api/internal/table/actor_test.go api/internal/tablemanager/manager.go
git commit -m "feat(table): validate reaction catalog membership and premium ownership"
```

---

### Task 7: HTTP routes (`internal/api/v1/reactionpurchase.go`)

**Files:**
- Create: `api/internal/api/v1/reactionpurchase.go`
- Test: `api/internal/api/v1/reactionpurchase_test.go`

**Interfaces:**
- Consumes: `reactionpurchase.Service` (Tasks 4-5), existing `localsUserID`, `problem.BadRequest`/
  `NotFound`, `walletOrInternalProblem` helpers (same ones `sandboxpurchase.go` uses).
- Produces: `RegisterReactionPurchase(router fiber.Router, auth fiber.Handler, svc
  *reactionpurchase.Service, purchaseLimiter *RateLimiter)`.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/api/v1/reactionpurchase_test.go
package v1

// Mirrors sandboxpurchase_test.go's shape: build a router with
// RegisterReactionPurchase, a fake reactionpurchase.Service double injected
// via the same test harness sandboxpurchase_test.go already uses in this
// package. Read that file in full before writing this one and copy its
// router-setup/auth-injection harness, swapping in these routes:
//   POST /wallet/reaction-purchase/ {reaction_id, method} -> 201
//   GET  /wallet/reaction-purchase/ -> 200, list
//   GET  /wallet/reaction-purchase/:id -> 200 or 404 (ErrNotFound)
//   POST /wallet/reaction-purchase/:id/refund -> 200, 409 (ErrAlreadyUsed), or 404
// Assert method="pix"|"fichas" dispatches to the right service call, and
// that an invalid method value is a 400 (problem.BadRequest), not a panic.
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/api/v1/... -run TestReactionPurchase -v`
Expected: FAIL — `RegisterReactionPurchase` doesn't exist yet

- [ ] **Step 3: Write minimal implementation**

```go
// api/internal/api/v1/reactionpurchase.go
package v1

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"uuid"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
)

type reactionPurchaseHandlers struct{ svc *reactionpurchase.Service }

type ReactionPurchaseCreateRequest struct {
	ReactionID     string `json:"reaction_id"`
	Method         string `json:"method"` // "pix" | "fichas"
	IdempotencyKey string `json:"idem_key,omitempty"`
}

type ReactionPurchaseRefundRequest struct {
	IdempotencyKey string `json:"idem_key,omitempty"`
}

func RegisterReactionPurchase(router fiber.Router, auth fiber.Handler, svc *reactionpurchase.Service, purchaseLimiter *RateLimiter) {
	h := &reactionPurchaseHandlers{svc: svc}
	g := router.Group("/wallet/reaction-purchase", auth)
	g.Get("/catalog", h.catalog)
	g.Post("/", rateLimit(purchaseLimiter, ipKey("reactionpurchase:create")), h.create)
	g.Get("/", h.list)
	g.Get("/:id", h.get)
	g.Post("/:id/refund", h.refund)
}

func (h *reactionPurchaseHandlers) catalog(c fiber.Ctx) error {
	entries, err := h.svc.ListCatalog(c.Context())
	if err != nil {
		return walletOrInternalProblem(err, "list catalog failed", c).Send(c)
	}
	return c.JSON(entries)
}

func (h *reactionPurchaseHandlers) create(c fiber.Ctx) error {
	var req ReactionPurchaseCreateRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	if req.ReactionID == "" {
		return problem.BadRequest("reaction_id is required").Send(c)
	}
	idemKey := req.IdempotencyKey
	if idemKey == "" {
		idemKey = uuid.New().String()
	}
	userID := c.Locals(localsUserID).(string)
	switch req.Method {
	case "pix":
		rec, _, err := h.svc.CreateReal(c.Context(), userID, req.ReactionID, idemKey)
		if err != nil {
			return reactionPurchaseProblem(err, c).Send(c)
		}
		return c.Status(fiber.StatusCreated).JSON(rec)
	case "fichas":
		rec, err := h.svc.CreateSandbox(c.Context(), userID, req.ReactionID, idemKey)
		if err != nil {
			return reactionPurchaseProblem(err, c).Send(c)
		}
		return c.Status(fiber.StatusCreated).JSON(rec)
	default:
		return problem.BadRequest("method must be \"pix\" or \"fichas\"").Send(c)
	}
}

func (h *reactionPurchaseHandlers) list(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	records, err := h.svc.List(c.Context(), userID)
	if err != nil {
		return problem.InternalServer("list reaction purchases failed", c, err).Send(c)
	}
	return c.JSON(records)
}

func (h *reactionPurchaseHandlers) get(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	rec, err := h.svc.Get(c.Context(), userID, c.Params("id")) // Task 4's store.Get exposed via a thin Service.Get, or reuse List()+filter — pick the one already implemented, do not add a new store method
	if errors.Is(err, reactionpurchase.ErrNotFound) {
		return problem.NotFound("purchase not found").Send(c)
	}
	if err != nil {
		return walletOrInternalProblem(err, "get purchase failed", c).Send(c)
	}
	return c.JSON(rec)
}

func (h *reactionPurchaseHandlers) refund(c fiber.Ctx) error {
	var req ReactionPurchaseRefundRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	idemKey := req.IdempotencyKey
	if idemKey == "" {
		idemKey = uuid.New().String()
	}
	userID := c.Locals(localsUserID).(string)
	rec, err := h.svc.Refund(c.Context(), userID, c.Params("id"), idemKey)
	if err != nil {
		return reactionPurchaseProblem(err, c).Send(c)
	}
	return c.JSON(rec)
}

func reactionPurchaseProblem(err error, c fiber.Ctx) *problem.Problem {
	switch {
	case errors.Is(err, reactionpurchase.ErrNotFound):
		return problem.NotFound("purchase not found")
	case errors.Is(err, reactionpurchase.ErrAlreadyUsed):
		return problem.Conflict("reaction already used, cannot refund")
	case errors.Is(err, reactionpurchase.ErrUnknownReaction), errors.Is(err, reactionpurchase.ErrNotPremium):
		return problem.BadRequest(err.Error())
	default:
		return walletOrInternalProblemAsProblem(err, "reaction purchase failed", c)
	}
}
```

Before writing `get`/`reactionPurchaseProblem`, check the exact return type/name of the existing
`walletOrInternalProblem` helper (used verbatim by `sandboxpurchase.go`'s `get`/`refund`) and
`problem.Conflict`/`problem.Problem`'s real names — mirror them exactly rather than inventing
`walletOrInternalProblemAsProblem`, which is a placeholder name for "whatever the real helper is
called." If `Service` has no `Get(ctx, playerID, purchaseID)` method yet, add a thin one to
`service.go` in this task (`return s.store.Get(ctx, playerID, purchaseID)`, translating a nil result
to `ErrNotFound` same as `sandboxpurchase.Service.Refresh` does) rather than duplicating store logic
in the handler.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/api/v1/... -run TestReactionPurchase -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/api/v1/reactionpurchase.go api/internal/api/v1/reactionpurchase_test.go api/internal/reactionpurchase/service.go
git commit -m "feat(api): add reaction-purchase HTTP routes"
```

---

### Task 8: Webhook dispatch-by-prefix

**Files:**
- Modify: `api/internal/api/v1/walletwebhook.go`

**Interfaces:**
- Consumes: `reactionpurchase.Service.ConfirmFromWebhook` (Task 4), existing `sandboxpurchase.
  Service.ConfirmFromWebhook`.

- [ ] **Step 1: Write the failing test**

Extend the existing webhook handler test (find it — likely `walletwebhook_test.go`; if absent, add
one) with a case posting a `purchase_id` starting with `"prdp"` and asserting it dispatches to a fake
`reactionpurchase.Service` instead of the sandbox one, broadcasting `"reaction_purchase_update"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/api/v1/... -run TestWalletWebhook -v`
Expected: FAIL — handler always dispatches to `sandboxpurchase.Service` today

- [ ] **Step 3: Write minimal implementation**

```go
func RegisterWalletWebhook(router fiber.Router, hmacSecret string, sandboxSvc *sandboxpurchase.Service, reactionSvc *reactionpurchase.Service, reg ws.Registry) {
	router.Post("/webhooks/wallet", walletWebhookHandler(hmacSecret, sandboxSvc, reactionSvc, reg))
}

func walletWebhookHandler(hmacSecret string, sandboxSvc *sandboxpurchase.Service, reactionSvc *reactionpurchase.Service, reg ws.Registry) fiber.Handler {
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

		if strings.HasPrefix(payload.PurchaseID, "prdp") {
			record, changed, err := reactionSvc.ConfirmFromWebhook(c.Context(), payload.PurchaseID)
			if err != nil {
				slog.Error("wallet webhook: reaction reverify failed", "purchase_id", payload.PurchaseID, "err", err)
				return c.SendStatus(fiber.StatusInternalServerError)
			}
			if changed {
				data, err := goproto.Marshal(&pokerproto.ServerMessage{
					Type: "reaction_purchase_update", PlayerId: record.PlayerID,
					PurchaseId: record.PurchaseID, Code: record.Status,
				})
				if err == nil {
					reg.Broadcast(c.Context(), "user#"+record.PlayerID, data)
				}
			}
			return c.SendStatus(fiber.StatusOK)
		}

		record, changed, err := sandboxSvc.ConfirmFromWebhook(c.Context(), payload.PurchaseID)
		if err != nil {
			slog.Error("wallet webhook: reverify failed", "purchase_id", payload.PurchaseID, "err", err)
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		if changed {
			data, err := goproto.Marshal(&pokerproto.ServerMessage{
				Type: "sandbox_purchase_update", PlayerId: record.PlayerID,
				PurchaseId: record.PurchaseID, Amount: record.TotalCredits, Code: record.Status,
			})
			if err == nil {
				reg.Broadcast(c.Context(), "user#"+record.PlayerID, data)
			}
		}
		return c.SendStatus(fiber.StatusOK)
	}
}
```

Add `"gopkg.aoctech.app/poker/api/internal/reactionpurchase"` and `"strings"` to the import block (the
sandbox handler already imports the rest). Update the one call site of `RegisterWalletWebhook` (in
`router.go` or `app.go`) to pass the new `reactionSvc` argument.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/api/v1/... -run TestWalletWebhook -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/api/v1/walletwebhook.go
git commit -m "feat(api): dispatch wallet webhook by purchase_id prefix"
```

---

### Task 9: Favorites (`PlayerProfile.FavoriteReactions`)

**Files:**
- Modify: `api/internal/player/model.go`
- Modify: `api/internal/player/service.go`
- Modify: `api/internal/player/store.go`
- Test: append to `api/internal/player/service_test.go`

**Interfaces:**
- Consumes: `reactions.IsKnown` (Task 1).
- Produces: `PlayerProfile.FavoriteReactions []string`, `Service.SetFavoriteReactions(ctx, userID
  string, favorites []string) (*PlayerProfile, error)`, `Store.SetFavoriteReactions(ctx, userID
  string, favorites []string) error`, `ErrInvalidFavoriteReactions`.

- [ ] **Step 1: Write the failing test**

```go
func TestSetFavoriteReactionsValidatesCountAndCatalog(t *testing.T) {
	svc := newTestPlayerService(t) // existing test helper in service_test.go

	if _, err := svc.SetFavoriteReactions(context.Background(), "user-1", []string{"clap", "cold", "fire", "poop"}); err != ErrInvalidFavoriteReactions {
		t.Fatalf("expected rejection of a 4th favorite, got %v", err)
	}
	if _, err := svc.SetFavoriteReactions(context.Background(), "user-1", []string{"not-a-reaction"}); err != ErrInvalidFavoriteReactions {
		t.Fatalf("expected rejection of an unknown reaction id, got %v", err)
	}

	profile, err := svc.SetFavoriteReactions(context.Background(), "user-1", []string{"clap", "cold"})
	if err != nil {
		t.Fatalf("SetFavoriteReactions: %v", err)
	}
	if len(profile.FavoriteReactions) != 2 {
		t.Fatalf("unexpected favorites: %+v", profile.FavoriteReactions)
	}
}

func TestSetFavoriteReactionsAllowsUnownedPremium(t *testing.T) {
	// Favoriting a premium reaction the player doesn't own yet is allowed —
	// it's a UI shortcut to the buy flow, not a claim of ownership
	// (docs/specs/2026-08-12-premium-reactions.md). handleReaction's
	// ownership check is what actually gates use.
	svc := newTestPlayerService(t)
	if _, err := svc.SetFavoriteReactions(context.Background(), "user-1", []string{"cold"}); err != nil {
		t.Fatalf("expected favoriting an unowned premium reaction to succeed, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/player/... -run TestSetFavoriteReactions -v`
Expected: FAIL — `SetFavoriteReactions` doesn't exist yet

- [ ] **Step 3: Write minimal implementation**

Add to `model.go`, next to `FeaturedAchievements`:

```go
FavoriteReactions []string `dynamodbav:"favorite_reactions,omitempty" json:"favorite_reactions,omitempty"`
```

Add to `service.go`, mirroring `SetShowcase`'s validation shape exactly:

```go
var ErrInvalidFavoriteReactions = errors.New("player: invalid favorite reactions")

func (s *Service) SetFavoriteReactions(ctx context.Context, userID string, favorites []string) (*PlayerProfile, error) {
	if len(favorites) > 3 {
		return nil, ErrInvalidFavoriteReactions
	}
	seen := make(map[string]bool, len(favorites))
	normalized := make([]string, 0, len(favorites))
	for _, id := range favorites {
		id = strings.TrimSpace(id)
		if id == "" || !reactions.IsKnown(id) || seen[id] {
			return nil, ErrInvalidFavoriteReactions
		}
		seen[id] = true
		normalized = append(normalized, id)
	}
	if err := s.store.SetFavoriteReactions(ctx, userID, normalized); err != nil {
		return nil, err
	}
	return s.store.GetOrCreate(ctx, userID)
}
```

Add `"gopkg.aoctech.app/poker/api/internal/reactions"` to `service.go`'s imports.

Add to `store.go`, mirroring `SetShowcase`:

```go
func (s *Store) SetFavoriteReactions(ctx context.Context, userID string, favorites []string) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	ok, err := s.base.UpdateItem(ctx, userID, nil, map[string]any{
		"favorite_reactions": favorites,
		"updated_at":         dynamo.NowStr(),
	})
	if err != nil {
		return fmt.Errorf("player: set favorite reactions: %w", err)
	}
	if !ok {
		return fmt.Errorf("player: profile disappeared while setting favorite reactions")
	}
	return nil
}
```

Wire a `PATCH`/similar field on whatever DTO the existing profile-update HTTP handler already binds
(find the handler that calls `Service.SetShowcase` in `internal/api/v1/`, and add the equivalent call
for `SetFavoriteReactions` on the same route or its own — reuse the existing profile PATCH route per
the spec, do not add a new endpoint).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/player/... -run TestSetFavoriteReactions -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/player/model.go api/internal/player/service.go api/internal/player/store.go api/internal/player/service_test.go
git commit -m "feat(player): add favorite reactions"
```

---

### Task 10: CDK — `poker_reaction_entitlements` and `poker_reaction_purchases` tables

**Files:**
- Modify: `cdk/lib/dynamodb-stack.ts`
- Test: `cdk/test/dynamodb-stack.test.ts`

**Interfaces:**
- Produces: two new `TableName` union members and two new tables, no GSIs (no sweep needed — pix
  confirmation is webhook-driven, fichas purchases are synchronous).

- [ ] **Step 1: Write the failing test**

```typescript
// cdk/test/dynamodb-stack.test.ts — mirror the existing poker_sandbox_purchases assertion
test('poker_reaction_entitlements and poker_reaction_purchases tables exist', () => {
  const template = Template.fromStack(stack);
  template.resourceCountIs('AWS::DynamoDB::GlobalTable', /* existing count */ + 2);
});
```

(Read this file's existing table-count/name assertions before writing — match its real style rather
than the sketch above.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd cdk && npx jest dynamodb-stack.test.ts`
Expected: FAIL — the two tables don't exist yet

- [ ] **Step 3: Write minimal implementation**

In `dynamodb-stack.ts`'s `TableName` union (around line 14):

```typescript
'poker_player_notes' | 'poker_hand_shares' | 'poker_player_poker_stats' | 'poker_sandbox_purchases' |
'poker_reaction_entitlements' | 'poker_reaction_purchases';
```

Next to the existing `table('poker_sandbox_purchases', true);` (around line 126):

```typescript
// poker_reaction_entitlements: pk = player_id, sk = reaction_id — one row per
// owned premium reaction. No TTL (permanent), no GSI (Actor.handleReaction
// reads it by exact key, cached in Valkey — see
// docs/specs/2026-08-12-premium-reactions.md).
table('poker_reaction_entitlements', true);
// poker_reaction_purchases: pk = player_id, sk = purchase_id — permanent
// purchase history, mirrors poker_sandbox_purchases. No GSI: pix confirmation
// is webhook-driven (no local pending sweep), fichas purchases are
// synchronous.
table('poker_reaction_purchases', true);
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd cdk && npx jest dynamodb-stack.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cdk/lib/dynamodb-stack.ts cdk/test/dynamodb-stack.test.ts
git commit -m "feat(cdk): add poker_reaction_entitlements and poker_reaction_purchases tables"
```

---

### Task 11: Fx wiring (`internal/app/app.go`, `internal/api/v1/router.go`)

**Files:**
- Modify: `api/internal/app/app.go`
- Modify: `api/internal/api/v1/router.go`

**Interfaces:**
- Consumes: everything from Tasks 1-9.

- [ ] **Step 1: Write the failing test**

No new unit test — pure dependency wiring, same as the wallet plan's Task 8. Verification is Step
2/4's build/boot check.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go build ./...`
Expected: builds today (nothing new is referenced yet) — proceed to Step 3.

- [ ] **Step 3: Write minimal implementation**

In `app.go`, next to `newSandboxPurchaseStore`/`newSandboxPurchaseService` (around line 295-299):

```go
func newReactionEntitlementStore(db *dynamodb.Client, cfg *config.Config) *reactionpurchase.EntitlementStore {
	return reactionpurchase.NewEntitlementStore(db, cfg.Env)
}
func newReactionPurchaseStore(db *dynamodb.Client, cfg *config.Config) *reactionpurchase.Store {
	return reactionpurchase.NewStore(db, cfg.Env)
}
func newReactionPurchaseService(wallet *walletclient.Client, entitlements *reactionpurchase.EntitlementStore, store *reactionpurchase.Store) *reactionpurchase.Service {
	return reactionpurchase.NewService(wallet, entitlements, store)
}
func newReactionOwnershipCache(svc *reactionpurchase.Service, cacheB cache.Backend) *reactionpurchase.OwnershipCache {
	return reactionpurchase.NewOwnershipCache(svc, cacheB)
}
```

Add these four functions to the `fx.Provide(...)` list (next to `newSandboxPurchaseStore,
newSandboxPurchaseService,`). Add `"gopkg.aoctech.app/poker/api/internal/reactionpurchase"` to
`app.go`'s imports.

Find where `mgr.SetSystemSettlementIntent(buyinSvc.BuildSystemSettlementIntent)` is called (around
line 605) and add next to it:

```go
mgr.SetReactionOwnership(reactionOwnershipCache.IsOwned)
mgr.SetReactionMarkUsed(reactionSvc.MarkUsed)
```

(add `reactionOwnershipCache *reactionpurchase.OwnershipCache, reactionSvc
*reactionpurchase.Service` as parameters to whichever `app.go` function currently calls
`mgr.SetSystemSettlementIntent` — check that function's real name/signature first, this plan's
sketch names it generically because the exact wiring function wasn't captured verbatim during
research; find it with `grep -n SetSystemSettlementIntent api/internal/app/app.go`.)

In `router.go`, next to `RegisterSandboxPurchase(router, auth, sandboxPurchaseSvc,
purchaseLimiter)` (around line 86):

```go
RegisterReactionPurchase(router, auth, reactionPurchaseSvc, purchaseLimiter)
```

Add `reactionPurchaseSvc *reactionpurchase.Service` as a parameter to the router-registration
function (mirrors `sandboxPurchaseSvc *sandboxpurchase.Service` at line 55) — Fx auto-wires it from
Task 4's provider.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go build ./...` then whatever boot-check command the wallet plan's Task 8 used
Expected: builds and boots without an Fx "missing dependency" panic.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/app.go api/internal/api/v1/router.go
git commit -m "feat(api): wire reaction-purchase service and ownership cache"
```

---

## Self-Review Notes

- **Spec coverage:** Catalog (Task 1), data model/stores (Task 2), wallet client (Task 3), service —
  real+sandbox+refund+markused+isowned (Tasks 4-5), server-side validation/cache (Task 6), routes
  (Task 7), webhook dispatch (Task 8), favorites (Task 9), CDK (Task 10), wiring (Task 11). Manual
  provisioning (granting `internal:wallet:product-purchase` to poker's M2M client in
  `ctech-account`) is explicitly out of engineering scope per the spec. Frontend work
  (`TableReactions.tsx`, the "Loja" page) is explicitly deferred to a separate `/impeccable` pass per
  the original request — not a task in this plan.
- **Placeholder scan:** Tasks 7 and 10's Step 1 point at existing sibling test files to copy rather
  than inventing scaffolding — same deliberate pattern as the wallet plan's Task 7/9. Task 11 names
  one wiring function generically pending a `grep` because its exact current signature wasn't
  captured during research — the grep command is given explicitly, this is not a TBD.
- **Type consistency:** `wallet` interface in `reactionpurchase/service.go` (Task 4) matches
  `walletclient.Client`'s real method signatures from Task 3 exactly, plus the pre-existing
  `Debit`/`Credit`. `OwnershipCache.svc` (Task 6) is typed as the minimal `isOwnedChecker` interface
  so `*Service` satisfies it without a wrapper. `Actor.reactionOwnership`/`reactionMarkUsed` (Task 6)
  match `Manager.reactionOwnership`/`reactionMarkUsed`'s function signatures exactly, matching
  `OwnershipCache.IsOwned` and the final transactional `Service.BuildMarkUsedIntent` contract.

## Post-implementation hardening (2026-08-12)

The final review found failure windows that the original sequential pseudocode did not cover. The
implemented contract is now:

- one conditional entitlement reservation per `(player_id, reaction_id)` enforces “purchasable
  once” before either wallet operation;
- fichas uses a persisted `processing` intent, an idempotent debit and one DynamoDB transaction to
  activate both history and ownership; definitive 4xx debit failures remove both reservation rows,
  while a later request resumes an ambiguous operation with its persisted original idempotency key;
- PIX confirmation atomically converges history and ownership and can reconstruct a missing local
  record when a webhook beats the create response; `GET /:id` re-verifies PIX with wallet;
- refunds move `confirmed → refunding` while atomically revoking the unused entitlement, use a
  server-derived wallet idempotency key, and then move `refunding → refunded`; a repeated refund
  cannot credit fichas twice;
- premium reaction send includes the conditional `used_at` write in the same DynamoDB transaction
  as the table action, so a concurrent send/refund race has exactly one winner without falsely
  marking a reaction that failed to commit;
- PIX QR/copy-and-paste/expiry fields are persisted and returned by `POST /`;
- purchase, confirmation, terminal PIX states and refund explicitly invalidate the ownership cache;
- the API EC2 role includes both reaction-table ARNs; the proto contract and lobby frontend handle
  `reaction_purchase_update`;
- regression coverage includes duplicate purchase/refund, insufficient fichas cleanup, PIX refund,
  webhook reconstruction, payment payload, cache invalidation and realtime query refresh.

The only remaining prerequisite is the already documented external provisioning grant
`internal:wallet:product-purchase` in `ctech-account`; it is not represented by a writable resource
in this repository.

## Frontend implementation (2026-08-13)

The deferred frontend pass is complete:

- `/store` now loads the server-owned premium catalog and purchase history, shows locked, pending,
  owned, refunding, refunded, expired and failed states, and supports both PIX and sandbox-fichas
  purchase paths without client-supplied prices;
- pending PIX purchases can be resumed and polled, while synchronous fichas purchases immediately
  refresh ownership and the player balance;
- confirmed purchases can request a refund with explicit first-use eligibility copy and clear PIX
  versus fichas consequences;
- the table picker fails closed for the six premium IDs while entitlement data loads, opens the buy
  flow for locked reactions, and keeps every free reaction on its existing hot path;
- players can persist up to three favorite reaction shortcuts from the table, including a locked
  premium reaction as a shortcut into its purchase flow;
- mock mode covers catalog, dual-currency purchase, PIX settlement, refund and favorite persistence;
  responsive browser QA covers the store, purchase confirmation and desktop/mobile table picker.
