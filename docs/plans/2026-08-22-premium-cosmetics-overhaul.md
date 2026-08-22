# Premium Cosmetics Overhaul Implementation Plan

> **For agentic workers:** Executed inline in the same session that wrote this plan (author already
> holds full spec + codebase context from research). A fresh executor should still use
> superpowers:executing-plans task-by-task; steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reclassify deck/felt cosmetics as free-vs-premium, add a `cosmeticpurchase` entitlement
package (sibling to `reactionpurchase`) so decks and felt themes can be bought and owned like
reactions, close the unguarded `deck_variant`/`table_theme` write gap, bundle emoji as local SVG
assets (scaffolding — see Task 9 scope note), and add Store sections to preview/buy cosmetics.

**Architecture:** Backend mirrors `internal/reactionpurchase`'s catalog/entitlement/purchase shape
into a new `internal/cosmetics` (catalog) + `internal/cosmeticpurchase` (purchase/entitlement)
package pair, keyed `(playerID, kind, itemID)` instead of `(playerID, reactionID)`. `player.Service`
gains an ownership-checker dependency (mirrors its existing `wallet` dependency) so
`SetDeckVariant`/new `SetTableTheme` can reject unowned premium ids. Frontend catalogs
(`cardVariants.ts`, `tablePreferences.ts`) gain premium-id sets; new Store sections and picker lock
badges reuse `ReactionFavoritesDialog`'s existing locked-item pattern.

**Tech Stack:** Go (Fiber v3, DynamoDB, `api-commons/dynamo`), Next.js/React/TanStack Query, CDK
(TypeScript).

**Spec:** `docs/specs/2026-08-21-premium-cosmetics-overhaul.md`

## Global Constraints

- No client-side asset obfuscation (spec Part 4, "What this deliberately does not attempt").
- `deck_variant`/`table_theme` are never broadcast to other players — no snapshot/wire changes.
- Wallet's `productSKUCatalog` (real-money prices per SKU) is owned by `ctech-wallet`, not this
  repo — out of scope here, same as the reactions spec.
- **Scope cuts made explicit here** (ponytail/YAGNI, consistent with the spec's own "not required
  for v1" notes):
  - No Neon deck, no seasonal catalog slot (spec Part 1: "not required for v1").
  - No new felt themes beyond reclassifying the existing three as premium (spec Part 2: explicit
    YAGNI call).
  - No ownership-cache wrapper for cosmetics (`reactionpurchase.OwnershipCache`'s Valkey layer
    exists because `Actor.handleReaction` is a hot per-action path; `SetDeckVariant`/
    `SetTableTheme` are low-frequency profile writes and the Store's catalog reads already go
    through TanStack Query caching client-side — a server cache adds nothing here).
  - `EmojiGlyph` ships as real, tested scaffolding (bundled-codepoint lookup + fallback) but with
    an **empty bundled-asset manifest** — sourcing/licensing actual Twemoji/Noto SVG files per
    codepoint is an asset-acquisition task, not a code task, and out of scope for this plan. Every
    reaction therefore keeps rendering its raw Unicode glyph today, unchanged in appearance, until
    real SVGs are dropped into `/public/emoji/`.
  - One shared `CosmeticPurchaseDialog`/`CosmeticRefundDialog` pair parameterized by `kind`
    (`"deck" | "felt"`), not four separate components — reactions only ever had one kind, so its
    dialogs never needed the parameter; decks/felt do from day one.

---

## Task 1: Backend catalog — `internal/cosmetics`

**Files:**
- Create: `api/internal/cosmetics/catalog.go`
- Test: `api/internal/cosmetics/catalog_test.go`

**Interfaces:**
- Produces: `type Kind string; const (KindDeck Kind = "deck"; KindFelt Kind = "felt")`,
  `IsKnown(kind Kind, id string) bool`, `IsPremium(kind Kind, id string) bool`,
  `SKUFor(kind Kind, id string) (sku string, priceFichas int64, ok bool)`,
  `ItemForSKU(sku string) (kind Kind, id string, priceFichas int64, ok bool)`,
  `All(kind Kind) []CatalogEntry`.

- [ ] Write `catalog.go` mirroring `internal/reactions/catalog.go`'s shape exactly (`CatalogEntry`,
  `byID`/`bySKU` maps built at init, same doc-comment style), with the catalog rows from the spec's
  Part 4 code block (`four-color`/`two-color`/`colorblind`/`high-constrast`/`classic` free;
  `casino`/`bicycle`/`vintage`/`golden`/`pink`/`alt` premium decks;
  `midnight`/`burgundy`/`ocean` premium felt). Key `byID`/`bySKU` on `(kind, id)` composite (map key
  `string(kind) + "#" + id`) since deck and felt ids don't collide today but the map must not assume
  that forever.
- [ ] Write `catalog_test.go`: table-driven, one case per catalog row, asserting `IsKnown`,
  `IsPremium`, and `SKUFor`'s `(sku, priceFichas, ok)` for both kinds — mirrors
  `reactions/catalog_test.go`'s structure.
- [ ] Run `go test ./internal/cosmetics/...` — expect PASS.
- [ ] Commit: `feat(poker-api): add cosmetics catalog for decks and felt`

## Task 2: Backend entitlement/purchase store — `internal/cosmeticpurchase`

**Files:**
- Create: `api/internal/cosmeticpurchase/store.go`, `api/internal/cosmeticpurchase/service.go`
- Test: `api/internal/cosmeticpurchase/store_test.go`, `api/internal/cosmeticpurchase/service_test.go`

**Interfaces:**
- Consumes: `cosmetics.Kind`, `cosmetics.IsKnown/IsPremium/SKUFor/ItemForSKU` (Task 1);
  `walletclient.Client` (existing `wallet` interface, copied verbatim from
  `reactionpurchase.Service`'s unexported `wallet` interface).
- Produces: `type Entitlement struct{ PlayerID, Kind, ItemID, PurchaseMethod, PurchaseID, Status,
  RequestKey, IdemKey, UsedAt, CreatedAt string }` (dynamodbav `pk`=PlayerID,
  `sk`=`Kind+"#"+ItemID`, `kind`, `item_id` as plain attributes); `type Record struct{ PlayerID,
  PurchaseID, Kind, ItemID, Method, ... }` (same fields as `reactionpurchase.Record` with
  `ReactionID` replaced by `Kind`+`ItemID`); `NewEntitlementStore(db, env) *EntitlementStore`,
  `NewStore(db, env) *Store`, `NewService(wallet, entitlements, store) *Service`; `Service` methods
  `ListCatalog(ctx, kind)`, `CreateReal(ctx, playerID, kind, itemID, idemKey)`,
  `CreateSandbox(ctx, playerID, kind, itemID, idemKey)`, `Refund(ctx, playerID, purchaseID, idemKey)`,
  `IsOwned(ctx, playerID, kind, itemID) (bool, error)`, `Refresh(ctx, playerID, purchaseID)`,
  `List(ctx, playerID)`, `ConfirmFromWebhook(ctx, purchaseID)`.

- [ ] Write `store.go`: copy `reactionpurchase/store.go`'s structure (table names
  `poker_cosmetic_entitlements` / `poker_cosmetic_purchases`, same status/method constants, same
  `EntitlementStore`/`Store` method set — `Reserve`, `CancelReservation`, `Get`, `Delete`,
  `CreateSandboxReservation`, `ConfirmSandbox`, `CancelSandboxReservation`, `AttachRealPurchase`,
  `HydratePIXDetails`, `RecoverPendingPIX`, `ClosePIXTerminal`, `GrantConfirmed`, `BeginRefund`,
  `CompleteRefund`, `Create`, `UpdateStatus`, `List`), replacing every `reactionID` parameter/field
  with `kind Kind, itemID string` (entitlement key) and every entitlement lookup with the composite
  sort key helper `entitlementSK(kind, itemID) string { return string(kind) + "#" + itemID }`. Drop
  `MarkUsed`/`BuildMarkUsedTxItem` and the `active()` "used" fields tied to the table-actor
  transaction pattern — deck/felt entitlements are never marked "used" by a hand-completion
  transaction the way a sent reaction is; `Refund`'s "never used" check instead means "never
  selected as the active deck/felt" and is enforced in the `Service.Refund` step below by checking
  the *current* `player.PlayerProfile.DeckVariant`/`TableTheme` isn't the item being refunded
  (Task 4's `player.Service` exposes `EffectiveDeckVariant`/`EffectiveTableTheme` already — inject a
  narrow `currentSelection(ctx, playerID, kind) (string, error)` function type into
  `cosmeticpurchase.Service` the same way `SetOwnershipInvalidator` injects a callback, to avoid an
  import cycle with `player`).
- [ ] Write `service.go`: copy `reactionpurchase/service.go`'s `Service` methods, same signatures
  with `reactionID string` replaced by `kind Kind, itemID string` everywhere, `reactions.SKUFor` /
  `reactions.IsKnown` / `reactions.ReactionForSKU` calls replaced with the `cosmetics` package
  equivalents from Task 1. Drop `MarkUsed`/`BuildMarkUsedIntent` (see above). Keep
  `ListCatalog(ctx, kind)`, `CreateReal`, `CreateSandbox`, `Refund`, `IsOwned`, `Refresh`, `List`,
  `ConfirmFromWebhook` with the same reservation/idempotency/webhook-recovery logic as the reaction
  version — this is the part of the spec's "mirror file-for-file" instruction that actually matters
  (money correctness), so no behavior changes, only the key shape.
- [ ] Write `store_test.go` / `service_test.go` mirroring `reactionpurchase`'s existing test files'
  scenarios (PIX purchase, sandbox/fichas purchase, refund-before-use succeeds, refund-after-use
  (i.e. refund while the item is the player's current selection) rejects, double-purchase
  idempotent), run for both `cosmetics.KindDeck` and `cosmetics.KindFelt` via a table-driven outer
  loop over `[]cosmetics.Kind{cosmetics.KindDeck, cosmetics.KindFelt}`.
- [ ] Run `go test ./internal/cosmeticpurchase/... -race` — expect PASS.
- [ ] Commit: `feat(poker-api): add cosmeticpurchase entitlement and purchase flow`

## Task 3: Backend HTTP handler + router wiring

**Files:**
- Create: `api/internal/api/v1/cosmeticpurchase.go`
- Test: `api/internal/api/v1/cosmeticpurchase_test.go`
- Modify: `api/internal/api/v1/router.go`, `api/internal/app/app.go`

**Interfaces:**
- Consumes: `cosmeticpurchase.Service` (Task 2).
- Produces: `RegisterCosmeticPurchase(router fiber.Router, auth fiber.Handler, svc
  *cosmeticpurchase.Service, purchaseLimiter *RateLimiter)` mounted at
  `/wallet/cosmetic-purchase/:kind` (`kind` path param validated against `cosmetics.KindDeck`/
  `KindFelt` before any service call, `problem.BadRequest` otherwise) with the same
  `catalog`/`create`/`list`/`get`/`refund` route shape as `RegisterReactionPurchase`.

- [ ] Write `cosmeticpurchase.go`: copy `reactionpurchase.go` handler shape, threading the `:kind`
  path param through to every service call and mapping errors via a `cosmeticPurchaseProblem`
  function that mirrors `reactionPurchaseProblem`'s switch.
- [ ] Write `cosmeticpurchase_test.go` mirroring `reactionpurchase_test.go`'s handler tests (catalog
  200, create 201 for both methods, unknown kind 400, refund conflict mapping).
- [ ] In `app.go`: add `newCosmeticsEntitlementStore`, `newCosmeticsPurchaseStore`,
  `newCosmeticsPurchaseService` constructors next to the existing reaction ones (same shape as
  lines 292-299 in the current file); add `*cosmeticpurchase.Service` to the Fx provider list; wire
  it into `newPlayerService` (Task 4) instead of `player.NewService(store).WithWallet(wallet)`
  alone — add `.WithCosmetics(cosmeticsSvc)`; add `cosmeticPurchaseSvc *cosmeticpurchase.Service` as
  a parameter to `registerRoutesWithSocialRuntime`, `registerRoutes`, and `v1.Register`, passed
  straight through to `RegisterCosmeticPurchase` in `router.go` next to the existing
  `RegisterReactionPurchase(router, auth, reactionPurchaseSvc, purchaseLimiter)` line, reusing the
  same `purchaseLimiter`.
- [ ] Run `go build ./...` then `go test ./internal/api/v1/... ./internal/app/... -race` — expect
  PASS.
- [ ] Commit: `feat(poker-api): expose cosmetic purchase HTTP routes`

## Task 4: `player.Service` — catalog/ownership enforcement + `table_theme`

**Files:**
- Modify: `api/internal/player/model.go`, `api/internal/player/service.go`
- Test: existing `api/internal/player/service_test.go` (extend)

**Interfaces:**
- Consumes: `cosmetics.IsKnown/IsPremium` (Task 1); a narrow ownership-checker interface
  `cosmeticsOwnershipChecker interface{ IsOwned(ctx context.Context, playerID string, kind
  cosmetics.Kind, itemID string) (bool, error) }` satisfied by `*cosmeticpurchase.Service` (Task 2).
- Produces: `PlayerProfile.TableTheme string` (`dynamodbav:"table_theme,omitempty"
  json:"table_theme,omitempty"`); `(p *PlayerProfile) EffectiveTableTheme() string` (defaults empty
  to `"classic"`); `(s *Service) WithCosmetics(c cosmeticsOwnershipChecker) *Service`;
  `ErrDeckVariantNotOwned`/`ErrTableThemeNotOwned` (or one shared `ErrCosmeticNotOwned`) alongside
  the existing `ErrInvalidDeckVariant`; `(s *Service) SetTableTheme(ctx, userID, themeID string)
  (*PlayerProfile, error)` mirroring `SetDeckVariant`'s shape.

- [ ] `model.go`: add `TableTheme` field (Part 5's exact snippet) next to `DeckVariant`; add
  `EffectiveTableTheme()` mirroring `EffectiveDeckVariant()`; add `DefaultTableTheme = "classic"`
  constant.
- [ ] `service.go`: replace `SetDeckVariant`'s trim/length-cap body with: trim, empty check, then
  `if !cosmetics.IsKnown(cosmetics.KindDeck, variant) { return nil, ErrInvalidDeckVariant }`, then
  `if cosmetics.IsPremium(cosmetics.KindDeck, variant) { owned, err := s.requireCosmetic(ctx,
  userID, cosmetics.KindDeck, variant); ... }` before `s.store.SetDeckVariant`. Add a private
  `requireCosmetic(ctx, userID string, kind cosmetics.Kind, itemID string) error` helper used by
  both `SetDeckVariant` and the new `SetTableTheme` (fails closed with a plain error if
  `s.cosmetics == nil`, since the ownership check must never silently pass just because wiring is
  missing). Add `SetTableTheme` with the identical validate-then-persist shape (needs a
  `SetTableTheme(context.Context, string, string) error` method added to the `profileStore`
  interface and to `player.Store`, Task 6).
- [ ] Extend `service_test.go`: unknown deck id rejected; known free id (e.g. `two-color`) always
  accepted with no cosmetics dependency wired; known premium id rejected without entitlement (fake
  `cosmeticsOwnershipChecker` returning `false, nil`), accepted with one (`true, nil`); same three
  cases for `SetTableTheme`.
- [ ] Run `go test ./internal/player/... -race` — expect PASS.
- [ ] Commit: `feat(poker-api): enforce cosmetic ownership on deck/table theme writes`

## Task 5: `player.Store` — persist `table_theme`

**Files:**
- Modify: `api/internal/player/store.go` (wherever `SetDeckVariant` is implemented today)
- Test: existing store test file for the package

**Interfaces:**
- Consumes: nothing new.
- Produces: `(s *Store) SetTableTheme(ctx context.Context, userID, theme string) error` — identical
  `UpdateItem`/`dynamodbav` shape to the existing `SetDeckVariant` store method.

- [ ] Add `SetTableTheme` next to `SetDeckVariant` in the store implementation, same single-field
  conditional update pattern.
- [ ] Extend the store's existing test file with one `SetTableTheme` round-trip case (set, then
  `Get`, assert `TableTheme` persisted).
- [ ] Run `go test ./internal/player/... -race` — expect PASS.
- [ ] Commit: `feat(poker-api): persist table_theme on player profile`

## Task 6: CDK — two new DynamoDB tables

**Files:**
- Modify: `cdk/lib/dynamodb-stack.ts`
- Test: `cdk/test/dynamodb-stack.test.ts`

**Interfaces:**
- Produces: `TableName` union gains `'poker_cosmetic_entitlements' | 'poker_cosmetic_purchases'`.

- [ ] Add `table('poker_cosmetic_entitlements', true)` and `table('poker_cosmetic_purchases', true)`
  next to the `poker_reaction_*` tables, with the same doc-comment shape explaining pk/sk and "no
  TTL, no GSI, no stream".
- [ ] Extend `dynamodb-stack.test.ts` with assertions for the two new tables (existing tests already
  assert per-table `TableName`/on-demand billing — add two more cases in the same style).
- [ ] Run `cd cdk && npm run build && npx jest test/dynamodb-stack.test.ts` — expect PASS.
- [ ] Commit: `feat(poker-cdk): add cosmetic entitlement and purchase tables`

## Task 7: Frontend deck catalog — remove `dark`, add `golden`/`pink`/`alt`, mark premium

**Files:**
- Modify: `ui/src/lib/cardVariants.ts`, `ui/src/lib/cards.ts`
- Test: `ui/src/lib/cardVariants.test.ts` (new or extend if one exists), `ui/src/lib/cards.test.ts`

**Interfaces:**
- Produces: `DeckVariantId` union loses `'dark'`, gains `'golden' | 'pink' | 'alt'`;
  `PREMIUM_DECK_IDS: Set<DeckVariantId>` (mirrors `PREMIUM_REACTION_IDS`'s shape) containing
  `'casino' | 'bicycle' | 'vintage' | 'golden' | 'pink' | 'alt'`; `cardPath` gains the
  `DECK_VARIANTS[variant] ? variant : DEFAULT_DECK_VARIANT` fallback (spec Part 4, point 1).

- [ ] Edit `cardVariants.ts`: delete the `dark` union member and its `DECK_VARIANTS` entry; add
  `golden`/`pink`/`alt` entries with the spec's exact hex palettes (Part 1); add
  `export const PREMIUM_DECK_IDS = new Set<DeckVariantId>(['casino','bicycle','vintage','golden','pink','alt'])`.
- [ ] Edit `cards.ts`: `cardPath` becomes
  `const safe = DECK_VARIANTS[variant] ? variant : DEFAULT_DECK_VARIANT; return \`/svgs/variants/${safe}/...\``.
- [ ] Test: `cardPath('As', 'dark' as DeckVariantId)` (a removed/unknown id) returns the
  `DEFAULT_DECK_VARIANT` path, not a 404 path; one test per new premium id resolves a real
  `DECK_VARIANTS` entry.
- [ ] Run `npx vitest run src/lib/cardVariants.test.ts src/lib/cards.test.ts` — expect PASS.
- [ ] Commit: `feat(poker-ui): reclassify deck catalog and add golden/pink/alt decks`

## Task 8: Frontend deck art — generate `golden`/`pink`/`alt` SVGs, delete `dark`

**Files:**
- Create: `ui/public/svgs/variants/golden/*.svg`, `ui/public/svgs/variants/pink/*.svg`,
  `ui/public/svgs/variants/alt/*.svg` (52 files each)
- Delete: `ui/public/svgs/variants/dark/`

- [ ] Every existing variant folder is the same 52-file structural template as `four-color`'s,
  differing only in each suit's single `fill:#RRGGBB` hex (verified: `club-*.svg` files all use one
  color, `spade-*`/`heart-*`/`diamond-*` likewise, plus the constant `#FFFFFF` card background).
  Generate the three new folders by copying `four-color` and running one `sed` substitution per
  suit, mapping the source suit hex to the new deck's suit hex from the spec's palette:
  - `golden`: spade `#141414`, club `#0E3B2E`, heart `#8B0000`, diamond `#C9A227`
  - `pink`: spade `#2E1A2E`, club `#6B2D5C`, heart `#FF4D6D`, diamond `#FF8FB1`
  - `alt`: spade `#3A3A3C`, club `#2E5FA3`, heart `#D63447`, diamond `#D4AF37`
- [ ] Delete `ui/public/svgs/variants/dark/` (spec Part 1: remove entirely).
- [ ] Spot-check one generated file per new deck renders (`file` command / open in browser) and that
  `#FFFFFF` backgrounds were not accidentally touched by the suit-color `sed`.
- [ ] Commit: `feat(poker-ui): add golden/pink/alt deck art, remove dark deck`

## Task 9: `EmojiGlyph` component (scaffolding — see Global Constraints)

**Files:**
- Create: `ui/src/components/ui/EmojiGlyph.tsx`, `ui/src/components/ui/EmojiGlyph.test.tsx`

**Interfaces:**
- Produces: `EmojiGlyph({glyph}: {glyph: string})` — renders `<img src="/emoji/<codepoint>.svg"
  alt="" aria-hidden="true">` if `glyph`'s codepoint is in `BUNDLED_EMOJI_CODEPOINTS`, else
  `<span aria-hidden="true">{glyph}</span>`. `codepointFor(glyph: string): string` (hex codepoint,
  handling multi-codepoint glyphs like `🗡️` which includes a variation selector — join with `-`,
  matching Twemoji's own naming convention so real assets can drop in later without a rename).

- [ ] Write `codepointFor`: `Array.from(glyph).map(c => c.codePointAt(0)!.toString(16)).join('-')`.
- [ ] Write `EmojiGlyph` with `export const BUNDLED_EMOJI_CODEPOINTS = new Set<string>([])` — empty
  today (Global Constraints scope cut); the lookup and fallback branch are real and tested, only the
  manifest is empty pending real asset sourcing.
- [ ] Test: a glyph whose codepoint is manually added to a local test-only bundled set renders an
  `<img>` with the expected `src`; a glyph not in the set renders the raw character in a `<span>`.
- [ ] Run `npx vitest run src/components/ui/EmojiGlyph.test.tsx` — expect PASS.
- [ ] Commit: `feat(poker-ui): add EmojiGlyph bundled-asset component`

## Task 10: Swap glyph render sites to `EmojiGlyph`

**Files:**
- Modify: `ui/src/components/table/TableReactions.tsx`, `ui/src/components/reactions/ReactionFavoritesDialog.tsx`,
  `ui/src/components/reactions/ReactionStoreSection.tsx`, `ui/src/components/reactions/ReactionPurchaseDialog.tsx`,
  `ui/src/components/reactions/ReactionRefundDialog.tsx`

- [ ] `TableReactions.tsx`'s `ReactionGlyph` (the `chip`-vs-plain-glyph switch) renders
  `<EmojiGlyph glyph={glyph}/>` instead of `<span aria-hidden="true">{glyph}</span>` for the
  non-`chip` branch; the three other raw `{definition.glyph}` render sites in the same file
  (favorites bar, quick-emote grid — lines ~241/255) switch likewise.
- [ ] `ReactionFavoritesDialog.tsx`, `ReactionStoreSection.tsx`, `ReactionPurchaseDialog.tsx`,
  `ReactionRefundDialog.tsx`: same mechanical swap of `{definition.glyph}` /
  `{entry-derived glyph}` to `<EmojiGlyph glyph={...}/>`.
- [ ] Existing snapshot/rendering tests for these components keep passing (the fallback path renders
  identical DOM text content today since the bundled set is empty — only wrapped in the same
  `aria-hidden` span/img semantics).
- [ ] Run `npx vitest run src/components/table/TableReactions.test.tsx src/components/reactions/`
  — expect PASS.
- [ ] Commit: `refactor(poker-ui): render reaction glyphs through EmojiGlyph`

## Task 11: Frontend felt catalog — premium ids, drop `theme` from local prefs

**Files:**
- Modify: `ui/src/lib/tablePreferences.ts`
- Test: `ui/src/lib/tablePreferences.test.ts` (extend if present)

**Interfaces:**
- Produces: `TABLE_THEMES` unchanged (still lives here — it's cosmetic art data, not a preference);
  `export const PREMIUM_FELT_IDS = new Set<TableThemeId>(['midnight','burgundy','ocean'])`;
  `TablePreferences` type drops `theme`; `DEFAULTS` drops `theme`; `normalize` drops the `theme`
  branch.

- [ ] Remove `theme` from `TablePreferences`, `DEFAULTS`, and `normalize` (keep `dealerVoice`,
  `voiceCommands`, `realityCheckMinutes` untouched, per spec Part 5).
- [ ] Add `PREMIUM_FELT_IDS` export.
- [ ] Update/extend existing tests: `normalize` no longer accepts/returns a `theme` field; a
  previously-stored `{theme: 'midnight', ...}` blob in `localStorage` from before this change
  round-trips through `normalize` harmlessly (extra key ignored, not an error).
- [ ] Run `npx vitest run src/lib/tablePreferences.test.ts` — expect PASS.
- [ ] Commit: `feat(poker-ui): move felt theme out of local table preferences`

## Task 12: Frontend `PlayerProfile`/`updateMe` gains `table_theme`

**Files:**
- Modify: `ui/src/lib/api/player.ts`

**Interfaces:**
- Produces: `PlayerProfile.table_theme?: TableThemeId`; `updateMe`'s input type gains
  `table_theme?: TableThemeId`.

- [ ] Import `TableThemeId` from `../tablePreferences`; add the field to both the `PlayerProfile`
  interface and `updateMe`'s input type, same optional-field shape as `deck_variant`.
- [ ] No new test needed — this is a type-only change exercised by Task 13's dialog test.
- [ ] Commit folded into Task 13 (files change together).

## Task 13: `TablePreferencesDialog` — server-backed felt selection with lock badges

**Files:**
- Modify: `ui/src/components/table/TablePreferencesDialog.tsx`
- Test: `ui/src/components/table/TablePreferencesDialog.test.tsx` (extend existing)

**Interfaces:**
- Consumes: `getMe`/`updateMe` (`ui/src/lib/api/player.ts`, Task 12);
  `listCosmeticCatalog`/`listCosmeticPurchases`/`ownedCosmeticIDs` (Task 14's
  `ui/src/lib/api/cosmeticPurchases.ts`); `PREMIUM_FELT_IDS` (Task 11).
- Produces: the theme `<Select>` reads `getMe().table_theme` (via a
  `useQuery({queryKey: ['player','me'], queryFn: getMe})`, the same query key `ProfileMenu.tsx`
  already populates, so this dialog gets a cache hit rather than a duplicate fetch) and writes via
  `useMutation({mutationFn: updateMe})`'s `save.mutate({table_theme: value})`, mirroring
  `ProfileMenu.tsx`'s `deck_variant` mutation exactly; unowned premium entries render a lock badge
  and, on click, call a new `onLockedFeltAction?: (id: TableThemeId) => void` prop instead of
  selecting (wired from the table page to open the Store's felt section — mirrors
  `TableReactions`'s `onLockedReactionAction` prop).

- [ ] Replace the `useTablePreferences().update({theme})` call with the `getMe`/`updateMe`
  TanStack Query pair described above; add `catalog`/`purchases` queries for
  `cosmetics.KindFelt`; compute `owned = ownedCosmeticIDs(purchases.data ?? [])`.
- [ ] Each `<SelectItem>` for a `PREMIUM_FELT_IDS` member the viewer doesn't own renders disabled
  with a `LockKeyhole` icon (mirrors `ReactionFavoritesDialog`'s `locked` rendering) instead of
  being selectable; clicking it calls `onLockedFeltAction` instead of `update`.
- [ ] Extend the existing test file: selecting a free theme still calls the (now `updateMe`-backed)
  mutation; selecting an unowned premium theme does not mutate and instead fires
  `onLockedFeltAction`; selecting an owned premium theme mutates normally.
- [ ] Run `npx vitest run src/components/table/TablePreferencesDialog.test.tsx` — expect PASS.
- [ ] Commit: `feat(poker-ui): persist felt theme via profile with ownership gating`

## Task 14: `ui/src/lib/api/cosmeticPurchases.ts`

**Files:**
- Create: `ui/src/lib/api/cosmeticPurchases.ts`
- Test: `ui/src/lib/api/cosmeticPurchases.test.ts`

**Interfaces:**
- Produces: `type CosmeticKind = 'deck' | 'felt'`; `CosmeticCatalogEntry`/`CosmeticPurchase`
  (same shape as `ReactionCatalogEntry`/`ReactionPurchase` plus a `kind`/`item_id` pair instead of
  `reaction_id`); `listCosmeticCatalog(kind)`, `listCosmeticPurchases(kind)`,
  `createCosmeticPurchase(kind, itemId, method)`, `getCosmeticPurchase(kind, purchaseId)`,
  `refundCosmeticPurchase(kind, purchaseId)`, `ownedCosmeticIDs(purchases)`,
  `currentCosmeticPurchase(purchases, itemId)` — mirrors every function in
  `reactionPurchases.ts` one-for-one, hitting `/v1.0/wallet/cosmetic-purchase/${kind}/...`
  (Task 3's route shape).

- [ ] Write the file mirroring `reactionPurchases.ts` exactly, substituting `reaction_id` for
  `item_id` and threading `kind` into every URL and payload.
- [ ] Test: mirrors any existing `reactionPurchases.test.ts` coverage (`currentCosmeticPurchase`'s
  status-priority sort, `ownedCosmeticIDs`'s confirmed-only filter) — if no such test file exists
  for reactions today, write the equivalent minimal coverage here instead of skipping it (spec's
  Testing section requires it either way).
- [ ] Run `npx vitest run src/lib/api/cosmeticPurchases.test.ts` — expect PASS.
- [ ] Commit: `feat(poker-ui): add cosmetic purchase API client`

## Task 15: `ProfileMenu` deck picker — lock badges + Store link

**Files:**
- Modify: `ui/src/components/lobby/ProfileMenu.tsx`
- Test: `ui/src/components/lobby/ProfileMenu.test.tsx` (extend existing)

**Interfaces:**
- Consumes: `listCosmeticCatalog`/`listCosmeticPurchases`/`ownedCosmeticIDs` (Task 14);
  `PREMIUM_DECK_IDS` (Task 7).

- [ ] Add `catalog`/`purchases` queries for `cosmetics.KindDeck`; compute `owned`. Each
  `<SelectItem>` for a `PREMIUM_DECK_IDS` member the viewer doesn't own renders disabled with a
  `LockKeyhole` icon and, instead of `save.mutate({deck_variant: id})`, links to `/store#decks`
  (`render={<Link href="/store#decks"/>}` on that item the same way other menu rows already use
  `render` for navigation) — matches spec Part 6's "lock icon + Ver na loja" affordance.
- [ ] Extend the existing test: an unowned premium deck option is disabled/non-selecting; an owned
  one (or a free one) selects normally.
- [ ] Run `npx vitest run src/components/lobby/ProfileMenu.test.tsx` — expect PASS.
- [ ] Commit: `feat(poker-ui): lock unowned premium decks in the profile picker`

## Task 16: Store — `DeckStoreSection` / `FeltStoreSection` + shared purchase/refund dialogs

**Files:**
- Create: `ui/src/components/store/CosmeticStoreSection.tsx` (exports both `DeckStoreSection` and
  `FeltStoreSection`, sharing one generic list renderer parameterized by `kind` and a
  `renderPreview(itemId)` prop), `ui/src/components/store/CosmeticPurchaseDialog.tsx`,
  `ui/src/components/store/CosmeticRefundDialog.tsx`
- Test: `ui/src/components/store/CosmeticStoreSection.test.tsx`,
  `ui/src/components/store/CosmeticPurchaseDialog.test.tsx`
- Modify: `ui/src/app/store/page.tsx`

**Interfaces:**
- Consumes: `cosmeticPurchases.ts` (Task 14); `DECK_VARIANTS`/`cardPath` for deck preview art
  (mirrors `ProfileMenu.tsx`'s ace-preview pattern, lines 183-188); `TABLE_THEMES` for felt gradient
  swatches.
- Produces: `DeckStoreSection({catalog, purchases, ...}: ...)`, `FeltStoreSection({catalog,
  purchases, ...}: ...)` — same prop shape as `ReactionStoreSection` (`onBuyAction`,
  `onRefundAction`, `onResumeAction`, `isLoading`, `isError`, `onRetryAction`); `CosmeticPurchaseDialog`
  / `CosmeticRefundDialog` take a `kind: CosmeticKind` prop alongside the same props
  `ReactionPurchaseDialog`/`ReactionRefundDialog` take, driving `EmojiGlyph`-free previews (card art
  or felt swatch instead of a glyph hero).
- [ ] Write `CosmeticStoreSection.tsx`: one internal `<CosmeticGrid kind itemIds catalog purchases
  ... renderPreview>` list component (copy `ReactionStoreSection`'s grid markup/logic, substituting
  `entry.id`/`item_id` and the preview slot); `DeckStoreSection` passes
  `renderPreview={id => <img src={cardPath('As', id)} alt=""/>}` over `Object.keys(DECK_VARIANTS)`;
  `FeltStoreSection` passes a `<span style={{'--theme-a':...,'--theme-b':...}}>` swatch over
  `Object.keys(TABLE_THEMES)`. Both render every catalog entry (owned and unowned, premium and
  free) — the "always-visible gallery" the spec calls for — not just unlockable ones.
- [ ] Write `CosmeticPurchaseDialog.tsx` / `CosmeticRefundDialog.tsx` copying
  `ReactionPurchaseDialog.tsx` / `ReactionRefundDialog.tsx` structurally, replacing the glyph hero
  with the same preview slot pattern and threading `kind` into every `cosmeticPurchases.ts` call.
- [ ] Wire both sections into `store/page.tsx` as new `id="decks"` / `id="felt"` sections (siblings
  to `id="reactions"`), following the same heading/nav-directory pattern already used for reactions
  (add two `store-directory` nav entries).
- [ ] Test: unowned premium entries show a price and buy button and no apply action; owned entries
  show an "apply"/"in use" state instead of a buy button (deck/felt have no send-and-consume action
  like reactions — "use" means "select in the picker", so the store item itself doesn't need a
  distinct used-vs-unused affordance beyond owned-vs-not).
- [ ] Run `npx vitest run src/components/store/CosmeticStoreSection.test.tsx
  src/components/store/CosmeticPurchaseDialog.test.tsx` — expect PASS.
- [ ] Commit: `feat(poker-ui): add deck and felt store sections`

## Task 17: Full quality gate + docs

**Files:**
- Modify: `api/CLAUDE.md`, `ui/CLAUDE.md` (mandatory documentation policy)

- [ ] Backend: `cd api && go build ./... && go test ./... -race`.
- [ ] Frontend: `cd ui && npx vitest run && npx tsc --noEmit && npx eslint src --max-warnings 0 &&
  npm run build`.
- [ ] CDK: `cd cdk && npm run build && npx jest`.
- [ ] Update `api/CLAUDE.md`'s layout line to mention `cosmetics`/`cosmeticpurchase`; note the closed
  `deck_variant` gap (the file's own comment currently documents it as open — Part 4 of the spec
  fixes it, so the doc must stop describing it as a live gap).
- [ ] Update `ui/CLAUDE.md` if the "Not built" or layout sections reference anything this plan
  changes (deck/felt pickers, Store sections).
- [ ] Commit: `docs(poker): document cosmetic purchase layer and closed deck_variant gap`

## Task 18: Final commit + push

- [ ] `git add -A` (review `git status` output first — this plan's own file plus the spec plus every
  file above, and nothing else).
- [ ] `git commit -m "feat: added premium cosmetics overhaul"` including the spec, this plan, and
  all implementation changes in one commit (per explicit instruction — not split per task here).
- [ ] `git push origin main`.
