# Premium table reactions — purchase, ownership, favorites — design

Date: 2026-08-12

Depends on `ctech-wallet`'s `docs/specs/2026-08-12-product-purchase-skus.md` (generic PIX product
sale, no ledger effect) being implemented first.

## Goal

Some `TABLE_REACTIONS` entries (`ui/src/lib/reactions.ts`) become premium: purchasable once, owned
forever, usable at any table. A player buys one either with real money (PIX, via wallet's new
product-purchase flow) or with sandbox fichas (a direct debit, no PIX) — the fichas price is a fixed
multiple of the real price, not derived from the sandbox-purchase exchange rate, so grinding fichas
is never a way to buy the item cheaper than intended. A player can refund a purchase only if they
never sent that reaction. Each player picks up to 3 favorites for a quick-access shortcut. Every
reaction send is validated server-side against actual ownership — today `Actor.handleReaction`
accepts any `reaction_id` string with no ownership or even catalog check, so a hand-crafted WS frame
can fire a premium reaction for free; this closes that gap for free reactions too (unknown IDs are
now rejected, not silently accepted).

## Non-goals

- No changing which reactions exist or their glyph/animation — that stays 100% client-owned
  (`ui/src/lib/reactions.ts`), same as today.
- No admin UI for pricing. Both catalogs (`internal/reactions/catalog.go` here, `productSKUCatalog`
  in wallet) are fixed Go tables, edited by a deploy — mirrors every other SKU catalog in this
  codebase.
- No changing the free-reaction cooldown/rate-limit UX — out of scope, though the new catalog
  validation happens on the same code path.

## Premium reaction list

Exactly these six `TABLE_REACTIONS` IDs become premium; every other ID stays free:

| ID       | Label (pt-BR)     | Type              | Real (PIX) | Fichas  |
|----------|--------------------|-------------------|------------|---------|
| `cold`   | Frio na mesa        | emoji (untargeted)| R$ 1,00    | 100.000 |
| `fire`   | Sequência quente    | emoji (untargeted)| R$ 1,00    | 100.000 |
| `poop`   | Jogar cocô          | targeted object   | R$ 5,00    | 500.000 |
| `rofl`   | Rir da cara         | targeted object   | R$ 5,00    | 500.000 |
| `knife`  | Jogar faca          | targeted object   | R$ 5,00    | 500.000 |
| `turtle` | Chamar de lento     | targeted object   | R$ 5,00    | 500.000 |

Pricing follows the type-based rule below — untargeted emoji vs. targeted object — not a per-ID
override, so adding a seventh premium reaction later just means flipping its `Premium` flag with the
price that matches its `targeted` shape in `TABLE_REACTIONS`.

## Pricing example

| Reaction           | Real (PIX) | Fichas  |
|--------------------|------------|---------|
| emoji (untargeted) | R$ 1,00    | 100.000 |
| targeted object    | R$ 5,00    | 500.000 |

For reference, the existing sandbox-purchase exchange rate (`SandboxCreditsPerCent = 100`) makes
R$ 1,00 buy 10.000 fichas when buying fichas directly — the fichas price above is a deliberate ~10x
markup over that rate, encoded as its own independent number per reaction, not computed from it.

## Catalog (`internal/reactions/catalog.go`, new package)

```go
// Premium is the poker-owned source of truth for which reaction IDs exist and
// which are premium — a game-design fact, not a money fact, so it lives here
// and not in wallet. PriceFichas is fixed here (never client-supplied);
// PriceCents for the same reaction is fixed in wallet's own productSKUCatalog
// and fetched at runtime via ListProductSKUs, exactly like the existing
// sandbox-purchase flow never hardcodes wallet's prices locally.
type ReactionCatalogEntry struct {
ID          string
Premium     bool
PriceFichas int64 // 0 if !Premium
SKU         string // wallet ProductSKU ID, e.g. "poker_reaction_chip". "" if !Premium.
}

var catalog = []ReactionCatalogEntry{
{ID: "clap", Premium: false},
// ... every other free TABLE_REACTIONS id, Premium: false ...
{ID: "cold", Premium: true, PriceFichas: 100_000, SKU: "poker_reaction_cold"},
{ID: "fire", Premium: true, PriceFichas: 100_000, SKU: "poker_reaction_fire"},
{ID: "poop", Premium: true, PriceFichas: 500_000, SKU: "poker_reaction_poop"},
{ID: "rofl", Premium: true, PriceFichas: 500_000, SKU: "poker_reaction_rofl"},
{ID: "knife", Premium: true, PriceFichas: 500_000, SKU: "poker_reaction_knife"},
{ID: "turtle", Premium: true, PriceFichas: 500_000, SKU: "poker_reaction_turtle"},
}

func IsKnown(id string) bool
func IsPremium(id string) bool
func SKUFor(id string) (sku string, priceFichas int64, ok bool)
```

This list must be kept in sync with `ui/src/lib/reactions.ts`'s `TABLE_REACTIONS` keys — same
duplication already accepted between `TableReactionID` (TS) and whatever the backend validates
today (nothing, which is the bug this closes). A test asserts every catalog ID round-trips through
the same string set the frontend ships (see Testing).

## Data model

### `poker_reaction_entitlements` — ownership, pk `player_id`, sk `reaction_id`

The fast-path table `Actor.handleReaction` checks against. One row per **owned** premium reaction
(free reactions never get a row — ownership of a free reaction is universal and needs no record).

```go
type Entitlement struct {
PlayerID       string `dynamodbav:"pk"`
ReactionID     string `dynamodbav:"sk"`
PurchaseMethod string `dynamodbav:"purchase_method"`   // "pix" | "fichas"
PurchaseID     string `dynamodbav:"purchase_id"`       // wallet purchase_id (pix) or this row's own history sk (fichas)
UsedAt         string `dynamodbav:"used_at,omitempty"` // first time this reaction was ever sent — refund gate
CreatedAt      string `dynamodbav:"created_at"`
}
```

### `poker_reaction_purchases` — history, pk `player_id`, sk `purchase_id`

Mirrors `poker_sandbox_purchases`'s shape (`docs/specs/2026-07-30-sandbox-purchase-design.md`) —
never TTL'd, this is purchase history a player can browse and refund from.

```go
type Record struct {
PlayerID     string `dynamodbav:"pk"`
PurchaseID   string `dynamodbav:"sk"` // wallet purchase_id (pix) or a poker-minted uuid (fichas — synchronous, no wallet purchase object exists)
ReactionID   string `dynamodbav:"reaction_id"`
Method       string `dynamodbav:"method"` // "pix" | "fichas"
PriceCents   int64  `dynamodbav:"price_cents,omitempty"`
PriceFichas  int64  `dynamodbav:"price_fichas,omitempty"`
Status       string `dynamodbav:"status"` // pending | confirmed | refunded (fichas purchases skip "pending" — see below)
CreatedAt    string `dynamodbav:"created_at"`
UpdatedAt    string `dynamodbav:"updated_at"`
}
```

## Service (`internal/reactionpurchase`, new package)

Mirrors `internal/sandboxpurchase`'s shape (service.go + store.go, `dynamo.Base`, conditional-put
idempotent create) with two create paths instead of one, converging on the same entitlement write.

- `ListCatalog(ctx) ([]CatalogEntry, error)` — merges `reactions.catalog` (premium flag, `PriceFichas`)
  with `walletclient.ListProductSKUs()` (`PriceCents` per SKU), same proxy-at-request-time posture as
  `sandboxpurchase.Service.ListSKUs` — prices are never cached/hardcoded locally.
- `CreateReal(ctx, playerID, reactionID, idemKey) (Record, PixPayload, error)` — looks up
  `reactions.SKUFor(reactionID)`, calls `walletclient.PurchaseProduct(ctx, playerID, sku, idemKey)`
  (new `walletclient` method, mirrors `PurchaseSandbox`), persists a `pending` `Record` with
  `method="pix"`. **No entitlement row yet** — same "confirm before granting" posture as sandbox
  credits; the entitlement is written by the webhook handler, mirrored below.
- `CreateSandbox(ctx, playerID, reactionID, idemKey) (Record, error)` — looks up `PriceFichas`,
  calls `walletclient.Debit(ctx, playerID, priceFichas, idemKey, "reaction_purchase:"+reactionID)`
  (existing generic method, no new wallet endpoint needed for this leg — it never touches PIX).
  On success, synchronously writes **both** the `Record` (`method="fichas"`, `status="confirmed"`)
  and the `Entitlement` row in one call, no pending stage, no webhook: sandbox debit is itself the
  confirmation, there is nothing async to wait for.
- `ConfirmFromWebhook(ctx, purchaseID) (Record, bool, error)` — mirrors
  `sandboxpurchase.Service.ConfirmFromWebhook` exactly: re-`GetProductPurchase` from wallet (never
  trust the webhook body), and on a new `confirmed` status, writes the `Entitlement` row (this is the
  only place a `pix`-method entitlement is created) and updates the `Record`.
- `Refund(ctx, playerID, purchaseID, idemKey) (Record, error)` — loads the `Record` by
  `(playerID, purchaseID)` to get `ReactionID`/`Method`, then loads the `Entitlement` by
  `(playerID, ReactionID)` and rejects with `409` if `UsedAt != ""`.
  Then branches on `Method`: `pix` → `walletclient.RefundProduct(...)` (new method, mirrors
  `RefundSandboxPurchase`); `fichas` → `walletclient.Credit(ctx, playerID, priceFichas, idemKey,
  "reaction_refund:"+reactionID)`. Either branch, on success, deletes the `Entitlement` row and marks
  the `Record` `refunded`.
- `MarkUsed(ctx, playerID, reactionID) error` — conditional update setting `UsedAt` only if empty
  (first-use-wins, idempotent on replay). Called from `Actor.handleReaction`, see below. A no-op for
  a free reaction (no entitlement row exists to update) — callers only invoke it when
  `reactions.IsPremium(id)` is true.
- `IsOwned(ctx, playerID, reactionID) (bool, error)` — the ownership check itself, DynamoDB
  `GetItem` on `poker_reaction_entitlements`. This is the function the cache in front of
  `Actor.handleReaction` wraps (see "Server-side validation").

## `walletclient` additions

New methods `PurchaseProduct`, `GetProductPurchase`, `RefundProduct` — literal structural copies of
`PurchaseSandbox`/`GetSandboxPurchase`/`RefundSandboxPurchase` against the new
`/wallet/product-purchase/*` M2M routes, new `TokenManager` scoped to
`internal:wallet:product-purchase`. No new `Debit`/`Credit` method needed — `CreateSandbox`/`Refund`
above call the ones that already exist.

## Server-side validation (`Actor.handleReaction`, `internal/table/actor.go`)

Today (`actor.go:315-346`) accepts any string as `c.ReactionID`. New shape:

```go
func (a *Actor) handleReaction(ctx context.Context, c ReactionCmd) error {
if c.ActionID == "" { ... }
if !reactions.IsKnown(c.ReactionID) {
return errors.New("table: unknown reaction_id")
}
if reactions.IsPremium(c.ReactionID) {
owned, err := a.reactionOwnership.IsOwned(ctx, c.PlayerID, c.ReactionID) // cached, see below
if err != nil { return err }
if !owned {
return errors.New("table: reaction not owned")
}
}
return a.commitActivity(ctx, false, func () error {
// ... existing body, unchanged ...
if reactions.IsPremium(c.ReactionID) {
_ = a.reactionPurchases.MarkUsed(ctx, c.PlayerID, c.ReactionID) // best-effort, never blocks the reaction itself
}
return a.commit(...)
})
}
```

**Cost of the ownership check** (the question this spec was written to answer): a cached read, not a
DynamoDB round-trip per reaction. `reactionOwnership` wraps `reactionpurchase.Service.IsOwned` behind
the existing `cache.Backend` (Valkey — already wired into this service, e.g.
`internal/tablelease`, `internal/api/v1/ratelimit.go`):

```go
key := "reaction-owned:" + playerID + ":" + reactionID
if cached, ok, _ := cache.Get(ctx, key); ok {
return cached[0] == '1', nil
}
owned, err := svc.IsOwned(ctx, playerID, reactionID)
if err == nil {
_ = cache.Set(ctx, key, []byte(boolByte(owned)), 30) // 30s TTL
}
return owned, err
```

Steady state: **one Valkey GET per reaction send**, sub-millisecond, no DynamoDB traffic. A
DynamoDB read only happens on cache miss — first premium reaction after a purchase, or after the
30s TTL expires with no traffic in between. This is latency-only caching, same category as
`tablelease` (never a correctness mechanism) — a stale "not owned" for up to 30s after a purchase
just means the player's first attempt right after buying can 30s-delay-fail once; the purchase flow
already returns success from the entitlement write, so the frontend can locally treat "just bought"
as owned without waiting on this cache to catch up (optimistic UI, matches how `PurchaseModal`
already works for sandbox credits). `MarkUsed`'s explicit cache invalidation is not needed for this
reason — the property being cached (ownership) monotonically becomes true and stays true; there is
no "used" flag being cached, only ownership.

## Webhook (`internal/api/v1/walletwebhook.go`)

The existing handler always dispatches to `sandboxpurchase.Service`. Wallet's product-purchase
webhook reuses the exact same `/v1.0/webhooks/wallet` endpoint and HMAC scheme; dispatch by the
`purchase_id` prefix reserved in the wallet spec (`"sbxp"` vs `"prdp"`) rather than trusting a `kind`
field from the body (defense in depth — the prefix is baked into the deterministic ID itself, not a
separately-attacker-adjacent field):

```go
if strings.HasPrefix(payload.PurchaseID, "prdp") {
record, changed, err := reactionSvc.ConfirmFromWebhook(c.Context(), payload.PurchaseID)
// ... broadcast type "reaction_purchase_update" ...
} else {
record, changed, err := sandboxSvc.ConfirmFromWebhook(c.Context(), payload.PurchaseID)
// ... existing "sandbox_purchase_update" ...
}
```

## Proto (`proto/poker.proto`)

New `ServerMessage.type = "reaction_purchase_update"`, reusing existing `player_id`, `purchase_id`,
`code` (status) fields — no new proto fields needed (same shape reuse as `sandbox_purchase_update`).

## Favorites (`internal/player/model.go`)

```go
FavoriteReactions []string `dynamodbav:"favorite_reactions,omitempty" json:"favorite_reactions,omitempty"`
```

Same shape as the existing `FeaturedAchievements []string`. Validation in `player.Service`'s profile
update path (mirrors wherever `FeaturedAchievements` is validated today): `len() <= 3`, every entry
`reactions.IsKnown(id)`. **Not** gated on ownership — favoriting a premium reaction the player
doesn't own yet is allowed (it's a UI shortcut to the buy flow, not a claim of ownership); the
existing `handleReaction` ownership check is what actually gates use. No new table, no new endpoint —
reuses the existing profile PATCH route.

## Routes (`internal/api/v1/reactionpurchase.go`, new)

Mirrors `sandboxpurchase.go`'s route registration under `/v1.0/wallet/reaction-purchase`:

| Route                                             | Notes                                                                       |
|---------------------------------------------------|-----------------------------------------------------------------------------|
| `GET /catalog`                                    | `ListCatalog` — merged premium flag + both prices                           |
| `POST /` `{reaction_id, method: "pix"\|"fichas"}` | Dispatches to `CreateReal`/`CreateSandbox`                                  |
| `GET /`                                           | List mine                                                                   |
| `GET /:id`                                        | Re-verify against wallet (pix only — fichas purchases are already terminal) |
| `POST /:id/refund`                                | `Refund`                                                                    |

## CDK

Add `poker_reaction_entitlements` and `poker_reaction_purchases` to `dynamodb-stack.ts`, same shape
as the existing tables (on-demand billing, pk/sk string keys).

## Error handling

- `CreateSandbox` debit fails (insufficient fichas) → passthrough error, no rows written (mirrors
  buy-in's debit-then-seat ordering: nothing commits until the debit succeeds).
- `CreateReal` and wallet unreachable → existing breaker trips, 503, no local row.
- Refund attempted on a `UsedAt != ""` entitlement → `409`, same status code the wallet's own
  `SandboxPurchaseUsed` problem uses for the analogous case.
- Webhook HMAC/dispatch failure → identical to the existing sandbox path (5xx so wallet retries).
- `Actor.handleReaction` on an unknown `reaction_id` → rejected (this is the security fix — today it
  is silently accepted).

## Testing

- Go: `reactions` catalog — a test iterating `ui/src/lib/reactions.ts`'s exported ID list (parsed
  once, checked into a fixture, or cross-checked manually on catalog change — no build-time coupling
  between the two languages) against `reactions.IsKnown` for every ID.
- `reactionpurchase` service — mirrors `sandboxpurchase/service_test.go`: `CreateSandbox` happy
  path + insufficient-balance rejection, `CreateReal` + `ConfirmFromWebhook` happy path, `Refund`
  rejected when used, `Refund` happy path for both methods.
- `actor_test.go` — new cases: unknown `reaction_id` rejected, premium+not-owned rejected, premium+
  owned accepted and marks used, free reaction still works unauthenticated-by-ownership (no
  entitlement lookup at all).
- Cache: a small test double for `cache.Backend` verifying `IsOwned` hits the store only once across
  repeated calls inside the TTL window.
- Frontend: covered under `/impeccable`'s own testing pass once built.

## Manual provisioning (outside this change)

- Grant poker's M2M client `internal:wallet:product-purchase` (parallels the existing
  `internal:wallet:sandbox-purchase` grant) — data change in `ctech-account`, not code here.
