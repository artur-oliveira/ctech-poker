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

- Reaction glyphs, supporting copy and animation stay 100% client-owned
  (`ui/src/lib/reactions.ts` + `ui/src/app/table-reactions.css`). The server owns only the
  fixed ID whitelist, premium flag and targeted shape, so every new client ID must be mirrored in
  `internal/reactions/catalog.go` before it can be sent in production.
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

## Free Poker Theater expansion (2026-08-25)

The Poker Theater UI expansion adds six free reactions. They participate in the same fixed catalog
validation and favorites flow but create no entitlement, purchase SKU, or wallet product:

| ID | Label (pt-BR) | Type |
|---|---|---|
| `heartbeat` | Coração all-in | untargeted self tell |
| `shark` | Modo tubarão | untargeted self tell |
| `pokerface` | Cara de pôquer | untargeted self tell |
| `spotlight` | Boa leitura | targeted gesture |
| `crown` | Passar a coroa | targeted gesture |
| `bandage` | Curar bad beat | targeted gesture |

The wire remains unchanged: `reaction_id` plus `target_player_id` for the three directed
gestures. The frontend's complete choreography catalog is documented in
`ui/docs/2026-08-25-poker-theater-reactions.md`.

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

The fast-path table `Actor.handleReaction` checks against. One row per claimed premium reaction:
`pending` reserves the one-time purchase slot and `active` grants ownership. Free reactions never
get a row because their ownership is universal.

```go
type Entitlement struct {
PlayerID       string `dynamodbav:"pk"`
ReactionID     string `dynamodbav:"sk"`
PurchaseMethod string `dynamodbav:"purchase_method"`   // "pix" | "fichas"
PurchaseID     string `dynamodbav:"purchase_id"`       // wallet purchase_id (pix) or this row's own history sk (fichas)
Status         string `dynamodbav:"status,omitempty"`  // pending | active; empty is legacy-active
RequestKey     string `dynamodbav:"request_key,omitempty"` // deterministic reservation owner
IdemKey        string `dynamodbav:"idem_key,omitempty"`    // private recovery key
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
Status       string `dynamodbav:"status"` // processing | pending | confirmed | refunding | refunded | failed | expired
IdemKey      string `dynamodbav:"idem_key,omitempty"` // private recovery data, never returned
PixCopiaECola string `dynamodbav:"pix_copia_e_cola,omitempty"`
QRCodeBase64 string `dynamodbav:"qr_code_base64,omitempty"`
ExpiresAt    string `dynamodbav:"expires_at,omitempty"`
CreatedAt    string `dynamodbav:"created_at"`
UpdatedAt    string `dynamodbav:"updated_at"`
}
```

## Service (`internal/reactionpurchase`, new package)

Uses `dynamo.Base` conditional writes and cross-table `TransactWriteItems`. Both create paths first
claim the single entitlement key as `pending`; only a confirmed wallet operation makes it `active`.
This reservation is the enforcement point for “purchasable once”, including concurrent requests.

- `ListCatalog(ctx) ([]CatalogEntry, error)` — merges `reactions.catalog` (premium flag, `PriceFichas`)
  with `walletclient.ListProductSKUs()` (`PriceCents` per SKU), same proxy-at-request-time posture as
  `sandboxpurchase.Service.ListSKUs` — prices are never cached/hardcoded locally.
- `CreateReal(ctx, playerID, reactionID, idemKey) (Record, PixPayload, error)` — reserves ownership,
  looks up
  `reactions.SKUFor(reactionID)`, calls `walletclient.PurchaseProduct(ctx, playerID, sku, idemKey)`
  and atomically links the wallet purchase to a `pending` history record. The QR code, PIX
  copy-and-paste value and expiry are persisted and returned. The pending reservation does not grant
  ownership. An ambiguous retry also resumes with the original persisted idempotency key, even when
  the route had generated that key for a client that omitted it.
- `CreateSandbox(ctx, playerID, reactionID, idemKey) (Record, error)` — looks up `PriceFichas`,
  atomically persists a `processing` intent plus pending reservation, calls the idempotent
  `walletclient.Debit`, then atomically activates both rows. A retry resumes the same intent.
  Definitive wallet 4xx errors remove both rows; transport/5xx ambiguity preserves them for recovery.
  If the next HTTP request carries a different/generated key, the persisted original key is used so
  an omitted client key can never strand the reservation or cause another debit.
- `ConfirmFromWebhook(ctx, purchaseID) (Record, bool, error)` — mirrors
  `sandboxpurchase.Service.ConfirmFromWebhook`: re-`GetProductPurchase` from wallet (never trust the
  webhook body), and atomically activates entitlement + history on `confirmed`. It can reconstruct a
  missing history row when the webhook races ahead of the create path.
- `Refund(ctx, playerID, purchaseID, idemKey) (Record, error)` — loads the `Record` by
  `(playerID, purchaseID)` to get `ReactionID`/`Method`, then loads the `Entitlement` by
  `(playerID, ReactionID)` and rejects with `409` if `UsedAt != ""`.
  It atomically transitions `confirmed → refunding` and deletes the unused entitlement before the
  external refund. Then it branches on `Method`: `pix` → `walletclient.RefundProductPurchase(...)`;
  `fichas` → `walletclient.Credit(...)`. Both use a server-derived key
  `reaction-refund:<purchase_id>`, so resuming `refunding` cannot issue a second refund. Success moves
  the record to `refunded`; subsequent refund requests return `409`.
- `BuildMarkUsedIntent(ctx, playerID, reactionID)` — builds a conditional `used_at =
  if_not_exists(used_at, now)` transaction item. `Actor.handleReaction` includes it in the same
  DynamoDB transaction as the table action. A missing/revoked entitlement aborts that transaction,
  making it the serialization point against concurrent refunds without recording use for a failed
  action. `MarkUsed` remains an idempotent store/service primitive and free reactions are no-ops.
- `IsOwned(ctx, playerID, reactionID) (bool, error)` — the ownership check itself, DynamoDB
  `GetItem` on `poker_reaction_entitlements`. This is the function the cache in front of
  `Actor.handleReaction` wraps (see "Server-side validation").

## `walletclient` additions

New methods `PurchaseProduct`, `GetProductPurchase`, `RefundProductPurchase` — structural copies of
`PurchaseSandbox`/`GetSandboxPurchase`/`RefundSandboxPurchase` against the new
`/wallet/product-purchase/*` M2M routes, new `TokenManager` scoped to
`internal:wallet:product-purchase`. No new `Debit`/`Credit` method needed — `CreateSandbox`/`Refund`
above call the ones that already exist.

## Server-side validation (`Actor.handleReaction`, `internal/table/actor.go`)

Before this change, `Actor.handleReaction` accepted any string as `c.ReactionID`. The implemented
shape is:

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
var extra []types.TransactWriteItem
if reactions.IsPremium(c.ReactionID) {
intent, err := a.reactionPurchases.BuildMarkUsedIntent(ctx, c.PlayerID, c.ReactionID)
if err != nil { return err }
extra = append(extra, *intent)
}
return a.commit(..., extra...) // table action + first use are atomic
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

Steady state: **one Valkey GET per reaction send**, sub-millisecond, no DynamoDB traffic. A DynamoDB
read only happens on cache miss. Every activation, terminal PIX transition and refund deletes the
exact ownership-cache key, preventing stale false after purchase and stale true after refund. The
30-second TTL is a fallback for a failed best-effort invalidation, not the expected UX.

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
| `GET /:id`                                        | Re-verify/reconstruct from wallet (PIX); resume a processing fichas intent   |
| `POST /:id/refund`                                | `Refund`                                                                    |

## CDK

Add `poker_reaction_entitlements` and `poker_reaction_purchases` to `dynamodb-stack.ts`, same shape
as the existing tables (on-demand billing, pk/sk string keys).

## Error handling

- `CreateSandbox` debit fails definitively (for example insufficient fichas) → passthrough error and
  transactional removal of intent + reservation. An ambiguous failure keeps both for safe retry.
- `CreateReal` and wallet unreachable → existing breaker trips, 503; its pending reservation is kept
  so retrying the same idempotency key cannot create a second purchase. Definitive 4xx removes it.
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
- Frontend: lobby realtime handles `reaction_purchase_update`, invalidates reaction catalog/history
  queries and emits localized status feedback; covered by `useLobbyRealtime.test.tsx`.

## Manual provisioning (outside this change)

- Grant poker's M2M client `internal:wallet:product-purchase` (parallels the existing
  `internal:wallet:sandbox-purchase` grant) — data change in `ctech-account`, not code here.
