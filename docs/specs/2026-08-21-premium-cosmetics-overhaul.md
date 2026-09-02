# Premium Cosmetics Overhaul (Decks, Felt, Emoji) — Design

## Summary

Today "premium cosmetics" only exists for table reactions (`docs/specs/2026-08-12-premium-reactions.md`:
a Go catalog + `internal/reactionpurchase`'s entitlement/PIX/fichas purchase flow, enforced server-side on every
reaction send). Deck color variants (`ui/src/lib/cardVariants.ts`) and the felt theme (`ui/src/lib/tablePreferences.ts`)
have **no premium concept at all** — `deck_variant` is a free-form string persisted with zero catalog check
(`player/service.go:25-28`, comment: *"not a catalog check"*), and the felt theme isn't even persisted server-side, it's
pure `localStorage`. This spec:

1. Reclassifies which decks are free vs. premium, removes one deck, adds three new ones.
2. Builds the purchase/entitlement machinery decks and felt themes are currently missing, by generalizing
   `reactionpurchase`'s pattern rather than reinventing it.
3. Replaces raw Unicode emoji glyphs with a bundled, consistent emoji asset so reactions render identically across every
   OS.
4. Adds a Store section to preview and buy deck/felt cosmetics.
5. States plainly what "harden against tampering" can and can't mean for a static-export SPA, and proposes the one fix
   that actually matters.

## Part 1 — Deck catalog changes

Current `DECK_VARIANTS` (`ui/src/lib/cardVariants.ts:19-92`): `four-color` (default), `two-color`,
`colorblind`, `high-constrast`, `casino`, `bicycle`, `dark`, `vintage`.

- **Remove `dark`** ("Modo Escuro") entirely — delete its `DeckVariantId` union member and
  `DECK_VARIANTS` entry. Per the request, it reads too washed-out to keep.
- **Stay free** (accessibility or distinct game modes, not premium candidates, per explicit instruction): `four-color`,
  `two-color`, `colorblind`, `high-constrast`.
- **Become premium**: `casino`, `bicycle`, `vintage` (this is "Retro" — `vintage`'s existing label is
  "Vintage", same concept, no rename needed).
- **New premium decks** — suggested starting palettes (a designer should refine these; they're concrete enough to
  implement and preview, chosen so all four suits stay legible on a white card):
    - `golden` (Dourado): spade `#141414`, club `#0E3B2E` (deep emerald), heart `#8B0000`, diamond
      `#C9A227` (gold) — the "VIP casino" black/emerald/gold combination is what reads as premium, not making every suit
      gold (which would kill legibility).
    - `pink` (Rosa): spade `#2E1A2E`, club `#6B2D5C`, heart `#FF4D6D`, diamond `#FF8FB1`.
    - `alt` (Alternativo, Balatro-style — colors as specified in the request): spade `#3A3A3C`
      (preto acinzentado), heart `#D63447`, club `#2E5FA3`, diamond `#D4AF37`.
    - Other suggestions, not required for v1: a "Neon" deck (near-black base + saturated cyan/magenta for two suits) for
      a night-table aesthetic distinct from `midnight` felt; a seasonal deck slot reusing the same catalog mechanism
      later (no code change needed, just a new catalog row).

## Part 2 — Felt catalog changes

Current `TABLE_THEMES` (`ui/src/lib/tablePreferences.ts:13-18`): `classic` (default), `midnight`,
`burgundy`, `ocean`. Per the request, every non-default theme becomes premium: `midnight`,
`burgundy`, `ocean`. No new felt themes requested; none added here (YAGNI — the deck additions already cover "new
premium content" for this pass).

**This requires felt theme to move from pure `localStorage` to a real per-player entitlement,** which means it also
needs server persistence for the first time (see Part 4) — a client-only preference can't be gated, entitlements have to
live where purchases are validated.

## Part 3 — Emoji: stop trusting the OS's font

`TABLE_REACTIONS` (`ui/src/lib/reactions.ts:1-24`) renders raw Unicode glyphs (`👏`, `😄`, ...) as plain text. The report
is correct and is a known, common failure mode: Unicode only specifies the codepoint, not the artwork — Windows, macOS,
and Android ship different emoji fonts, so the same codepoint renders as visibly different art (and on older/uncommon OS
builds, sometimes a fallback tofu box) for different players at the same table.

**Fix**: bundle a fixed emoji artwork set as local SVG assets (e.g. Twemoji or Noto Emoji — both open-licensed, static
files, no network fetch, fits the "no server at runtime" static-export constraint the same way `/svgs/variants/*`
already does for cards) under `/public/emoji/<codepoint>.svg`, and add a small `EmojiGlyph` component that maps a
`TABLE_REACTIONS` glyph string to its codepoint file and renders that SVG instead of the raw character. Every render
site that currently prints
`TABLE_REACTIONS[id].glyph` as text (`TableReactions.tsx`, `Seat.tsx`'s reaction bubble,
`ReactionFavoritesDialog.tsx`, the store's reaction catalog cards) swaps to `<EmojiGlyph glyph={...}/>`
— mechanical, no behavior change beyond consistent rendering.

This also directly answers "expandir base de emojis": once reactions render from a bundled full-set sprite instead of
relying on whatever the OS happens to draw, adding a new `TABLE_REACTIONS` entry is just picking any standard emoji
already covered by the bundled set — no new custom art has to be commissioned per addition, the way it does for the
deck/felt visuals above.

## Part 4 — The purchase/entitlement layer (new)

### Reuse strategy

`internal/reactionpurchase` already solved exactly this problem for reactions: a catalog (owned by poker, not wallet —
game-design fact, not a money fact), a wallet `ProductSKU` per premium item for the real-money price, a fixed fichas
price per item, an `EntitlementStore` for "does this player own this item," and PIX/sandbox purchase + refund flows
keyed by `(player, item)`. Decks and felt themes need the identical shape: buy once, own forever, refund only if never
used. Rather than bolt a second, slightly-different copy of that machinery on, or do a risky in-place refactor of the
already-shipped, tested reactions feature, this spec adds a **sibling package**, `internal/cosmeticpurchase`, that
mirrors `reactionpurchase`'s `Service`/`Store`/`EntitlementStore` file-for-file, generalized only where it must be: the
entitlement key becomes `(playerID, kind, itemID)` instead of
`(playerID, reactionID)`, where `kind` is `"deck"` or `"felt"`.

```go
// internal/cosmetics/catalog.go — same shape as internal/reactions/catalog.go
type Kind string
const (
KindDeck Kind = "deck"
KindFelt Kind = "felt"
)
type CatalogEntry struct {
Kind        Kind
ID          string // DeckVariantId or TableThemeId string
Premium     bool
PriceFichas int64  // 0 if !Premium
SKU         string // wallet ProductSKU id, "" if !Premium
}
var catalog = []CatalogEntry{
{Kind: KindDeck, ID: "four-color", Premium: false},
{Kind: KindDeck, ID: "two-color", Premium: false},
{Kind: KindDeck, ID: "colorblind", Premium: false},
{Kind: KindDeck, ID: "high-constrast", Premium: false},
{Kind: KindDeck, ID: "casino", Premium: true, PriceFichas: 200_000, SKU: "poker_deck_casino"},
{Kind: KindDeck, ID: "bicycle", Premium: true, PriceFichas: 200_000, SKU: "poker_deck_bicycle"},
{Kind: KindDeck, ID: "vintage", Premium: true, PriceFichas: 200_000, SKU: "poker_deck_vintage"},
{Kind: KindDeck, ID: "golden", Premium: true, PriceFichas: 500_000, SKU: "poker_deck_golden"},
{Kind: KindDeck, ID: "pink", Premium: true, PriceFichas: 500_000, SKU: "poker_deck_pink"},
{Kind: KindDeck, ID: "alt", Premium: true, PriceFichas: 500_000, SKU: "poker_deck_alt"},
{Kind: KindFelt, ID: "classic", Premium: false},
{Kind: KindFelt, ID: "midnight", Premium: true, PriceFichas: 200_000, SKU: "poker_felt_midnight"},
{Kind: KindFelt, ID: "burgundy", Premium: true, PriceFichas: 200_000, SKU: "poker_felt_burgundy"},
{Kind: KindFelt, ID: "ocean", Premium: true, PriceFichas: 200_000, SKU: "poker_felt_ocean"},
}
func IsKnown(kind Kind, id string) bool
func IsPremium(kind Kind, id string) bool
func SKUFor(kind Kind, id string) (sku string, priceFichas int64, ok bool)
```

Pricing mirrors the reactions spec's tiering logic (recolor of existing free assets = cheaper tier; new art = pricier
tier), same 10x-over-sandbox-rate reasoning as `docs/specs/2026-08-12-premium-reactions.md`. Real-money (PIX) prices for
each new `SKU` are added to wallet's `productSKUCatalog`, same as every existing SKU — not re-specified here, wallet
owns that table (per `ctech-wallet`'s existing pattern).

`internal/cosmeticpurchase` exposes, per `kind`:

- `ListCatalog(ctx, kind) ([]CatalogEntry, error)` — merges `cosmetics.catalog` with wallet's live PIX price, same as
  `reactionpurchase.Service.ListCatalog`.
- `Purchase(ctx, playerID, kind, itemID, method)` — PIX or fichas, same reservation-then-confirm flow as
  `reactionpurchase.Service.PurchasePIX`/`PurchaseSandbox`.
- `Refund(ctx, playerID, purchaseID)` — allowed only if the entitlement's `UsedAt` is empty, exactly mirroring
  reactions' "never sent" rule, generalized to "never selected as the active deck/felt."
- `IsOwned(ctx, playerID, kind, itemID) (bool, error)` — the ownership check `SetDeckVariant`/
  `SetTableTheme` call before persisting a premium selection (below).

New DynamoDB tables: `poker_cosmetic_entitlements` (mirrors `poker_reaction_entitlements`'s shape, keyed
`player_id#kind#item_id`) and `poker_cosmetic_purchases` (mirrors `poker_reaction_purchases`).

> **Follow-up (2026-09-02, #69):** this spec built `cosmeticpurchase.Service.ConfirmFromWebhook` (mirroring
> `reactionpurchase.Service.ConfirmFromWebhook`) but never wired it into `internal/api/v1/walletwebhook.go`, and never
> gave it a realtime push. Both premium reactions and premium cosmetics buy through wallet's generic product-purchase
> API and get a `"prdp"`-prefixed `purchase_id`, so the prefix alone can't tell the two apart the way it tells
> `"sbxp"` (sandbox) apart from `"prdp"`. The webhook handler now tries `reactionSvc.ConfirmFromWebhook` first for any
> `"prdp"` id; when that returns `reactionpurchase.ErrCatalogMismatch` (the SKU isn't a reaction SKU), it falls
> through to `cosmeticSvc.ConfirmFromWebhook`. On a changed cosmetic status it broadcasts a new
> `ServerMessage{Type: "cosmetic_purchase_update", PlayerId, PurchaseId, Code: status}` to `"user#"+playerID` via
> `ws.Registry.Broadcast` — the same registry/frame shape `sandbox_purchase_update` and `reaction_purchase_update`
> already use (no new proto fields; `proto/poker.proto`'s `ServerMessage.type` comment lists the new value). See
> `internal/api/v1/walletwebhook_cosmetic_test.go` (`-tags integration`) for webhook coverage of a cosmetic purchase
> id, including the reaction-then-cosmetic fallback. **Still open:** the client (`ui/src/lib/hooks/useLobbyRealtime.ts`)
> has no `cosmetic_purchase_update` branch yet — a cosmetic PIX confirmation now broadcasts correctly, but nothing
> in the browser invalidates the wallet/catalog query cache or toasts on receipt (mirror the existing
> `reaction_purchase_update` branch there). `CosmeticPurchaseDialog`'s 4s poll while open still works as a fallback.

### Closing the actual security gap

`SetDeckVariant` (`player/service.go:120-131`) today accepts **any string up to 60 characters** — no catalog check, no
ownership check, by its own comment's admission. That's the real gap, not
"someone reads the CSS": a raw `POST /v1.0/players/me {"deck_variant":"golden"}` already grants a premium deck for free
today, with no purchase at all — this predates and is independent of adding premium decks.

Fix, applied to both `SetDeckVariant` and the new `SetTableTheme` (Part 5):

1. Reject unknown IDs — `!cosmetics.IsKnown(kind, id)` → `ErrInvalidDeckVariant`-equivalent. This also fixes a latent
   rendering bug: `cardPath` (`ui/src/lib/cards.ts:11-17`) builds
   `/svgs/variants/<variant>/...` with **no fallback**, so any invalid `deck_variant` (today: a typo, after this spec: a
   removed `dark` id sitting on an old profile) 404s every card image. `cardPath`
   gains `DECK_VARIANTS[variant] ? variant : DEFAULT_DECK_VARIANT` before building the path, and
   `ProfileMenu.tsx`'s deck picker (which already does this fallback for its *label*, line 179 — extend the same guard
   to the actual applied variant) covers the display side.
2. If `cosmetics.IsPremium(kind, id)` is true, require `cosmeticpurchase.IsOwned(ctx, playerID, kind, id)`
   before persisting — reject with a plain 403-equivalent otherwise. This is the same fix
   `docs/specs/2026-08-12-premium-reactions.md` already made for reaction sends ("today `Actor.handleReaction` accepts
   any `reaction_id` string with no ownership or even catalog check... this closes that gap"); this spec closes the
   identical gap for the two cosmetic kinds that never had it in the first place.

**What this deliberately does *not* attempt**: obfuscating or otherwise "hardening" the static SVG/CSS assets
themselves. Two reasons this isn't worth building: (a) it's not actually preventable — the assets must ship to the
browser to render at all, so any client-side gate is cosmetic, not security; (b) unlike the reactions case, neither the
deck variant nor the felt theme is ever shown to *other*
players — both are purely local rendering preferences for the viewer's own client (confirmed: no
`deck_variant`/theme field is broadcast in any table snapshot). A player who hand-edits their own local CSS to see a
deck they didn't buy has changed nothing anyone else experiences and gained nothing except what their own screen shows
them — there is no cheating surface there, only the account-level bypass in point 2 above, which is the one that
actually matters and is now closed.

## Part 5 — Felt theme gets server persistence

> **Follow-up (2026-08-31):** the `player.Service`/store/catalog side of this
> part shipped, but the HTTP wiring in `internal/api/v1/player.go` was missed —
> `UpdatePlayerRequest` had no `TableTheme` field, `updateMe` never called
> `SetTableTheme`, and `playerResponse` never echoed `table_theme`, so every
> felt change from the client was silently dropped. Fixed in
> `docs/plans/2026-08-31-side-pot-fold-and-health-retry.md`; the same change
> also maps a premium-not-owned `deck_variant` selection to 400 instead of 500.

New `PlayerProfile` field, mirroring `DeckVariant` exactly (`player/model.go`):

```go
TableTheme string `dynamodbav:"table_theme,omitempty" json:"table_theme,omitempty"`
```

`EffectiveTableTheme()` defaults empty to `"classic"`. `SetTableTheme(ctx, userID, themeID string) error`
on `player.Service`, same shape as `SetDeckVariant`, **but written with the ownership check from day one** (Part 4,
point 2) — unlike `deck_variant`, which shipped the gap and is being closed after the fact, `table_theme` never ships
without it.

Frontend: `ui/src/lib/tablePreferences.ts`'s `theme` field stops being `localStorage`-only. It's removed from
`TablePreferences`/`normalize`/`DEFAULTS` (the other three fields — `dealerVoice`,
`voiceCommands`, `realityCheckMinutes` — are pure client conveniences, not monetized, and stay exactly as they are,
untouched). `TablePreferencesDialog.tsx`'s theme `<Select>` (lines 42-60) switches from
`useTablePreferences().update({theme})` to the same `updateMe({...})` mutation `ProfileMenu.tsx`
already uses for `deck_variant` (`save.mutate({table_theme: value})`), reading current value from
`getMe().table_theme` the same way `useDeckVariant` reads `deck_variant`. `PlayerProfile`
(`ui/src/lib/api/player.ts`) gains `table_theme?: TableThemeId`, and `updateMe`'s input type gains the same field — same
pattern as `deck_variant`, no new plumbing shape.

## Part 6 — Store: preview and buy

New `/store` sections, siblings to the existing `id="reactions"` section (`ui/src/app/store/page.tsx`), following the
same `*StoreSection` component shape as `ReactionStoreSection`:

- `DeckStoreSection` — every `DECK_VARIANTS` entry rendered with its real card art (same ace-preview pattern
  `ProfileMenu.tsx` already uses, lines 183-188), a lock badge + price on unowned premium entries, a buy button per
  entry (PIX or fichas, reusing `PurchaseModal`'s existing dialog shape). This is what "preview all cosmetics before
  buying, in the store" actually adds: today, **only**
  `ProfileMenu.tsx`'s picker shows deck art, and only for whatever's already unlockable; the store gets its own
  always-visible-regardless-of-ownership gallery.
- `FeltStoreSection` — same shape, showing the felt gradient swatch (`TABLE_THEMES[id].colors`)
  instead of card art.

Once purchased, the item becomes selectable in its existing picker (`ProfileMenu.tsx` for decks,
`TablePreferencesDialog.tsx` for felt) exactly as it does today for free options — no new picker UI, those pickers
already render every catalog entry; they gain a lock icon + "Ver na loja" affordance on entries the viewer doesn't own
instead of allowing selection, mirroring
`ReactionFavoritesDialog.tsx`'s existing `locked = PREMIUM_REACTION_IDS.has(id) && !owned.has(id)`
pattern (which already proves out-of-store preview of locked items is fine — this spec's "somente na loja" instruction
is about where *buying* happens, not where premium art is ever visible, since the codebase already shows locked reaction
art in the favorites picker today).

## Testing

Backend:

- `cosmetics.IsKnown`/`IsPremium`/`SKUFor` unit tests per catalog entry, both kinds.
- `cosmeticpurchase`: PIX purchase, sandbox (fichas) purchase, refund-before-use succeeds, refund-after-use rejects,
  double-purchase is idempotent — mirroring
  `reactionpurchase`'s existing test suite structure, run for both `KindDeck` and `KindFelt`.
- `SetDeckVariant`/`SetTableTheme`: unknown id rejected; known free id always accepted; known premium id rejected
  without entitlement, accepted with one.
- Regression: removing `dark` from the catalog does not error for a profile that still has
  `deck_variant: "dark"` stored — `EffectiveDeckVariant` still returns the raw stored value (unchanged behavior), and
  the *client's* fallback (below) is what prevents a broken image, not the backend silently rewriting old data.

Frontend:

- `cardPath` unit test: unknown/removed variant id falls back to `DEFAULT_DECK_VARIANT`'s path instead of building a 404
  URL.
- `EmojiGlyph` renders the bundled SVG for a known glyph; falls back to the raw Unicode character for any glyph without
  a bundled asset (defensive, in case a future `TABLE_REACTIONS` entry is added before its art is bundled).
- `DeckStoreSection`/`FeltStoreSection`: unowned premium entries show a price and buy button and no apply action; owned
  entries apply immediately.
- `ProfileMenu`/`TablePreferencesDialog` pickers: unowned premium entries render locked, matching
  `ReactionFavoritesDialog`'s existing locked-item pattern.

## Out of scope

- Any change to how reactions themselves are purchased — `internal/reactionpurchase` is untouched;
  `internal/cosmeticpurchase` is a new, separate package.
- Bundles/discounts across cosmetic kinds.
- Any further deck/felt art beyond what's listed in Parts 1-2 (extra suggestions are noted as optional future catalog
  rows, not built here).
- Client-side asset obfuscation — deliberately rejected, see Part 4.
