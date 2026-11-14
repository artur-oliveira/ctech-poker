# Avatar Cosmetics, Phase 3 (obtaining: store, achievements, grants) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. UI tasks: invoke `impeccable:impeccable` then `ponytail:ponytail` first (project rule, subagents included).

**Goal:** Players can buy the 10 purchasable avatar items (fichas or PIX), earn the 10 achievement items automatically (with an on-screen notice), and see locked "earn it" items in the store.

**Architecture:** Reuse `cosmeticpurchase` end to end: extend the catalog with the 10 purchasable entries and their `catalog.products` prices, let the existing `/wallet/cosmetic-purchase/:kind` routes accept the avatar kinds, make `ListCatalog` source-aware, and enforce the sale window in `Create*`. Grants: an explicit achievement-to-item table applied in the `handpipeline` unlock loop through `Service.Grant`, plus a self-heal on the owned-list read. Store UI: three new sections (frame, hat, accessory) fed by the registry art.

**Tech Stack:** Go (Fiber v3, DynamoDB, protobuf via `scripts/generate-proto.sh`), React/Next.js, vitest, Playwright layout gate.

**Spec:** `docs/specs/2026-11-14-avatar-cosmetics.md` (phase 3 of 3). Phases 1-2: `docs/plans/2026-11-14-avatar-cosmetics-phase{1,2}.md` (branches `feat/avatar-cosmetics`, `feat/avatar-cosmetics-ui`; this phase stacks on them on `feat/avatar-cosmetics-shop`).

## Global Constraints

- Money is integer cents; `PriceFichas = PriceCents * 1000` for every purchasable avatar item (matches deck 200c/200_000, felt 1000c/1_000_000).
- Prices are set only in the repo: `api/internal/catalog/catalog.go` `products` (cents) and `cosmetics.CatalogEntry.PriceFichas`; Billing receives description and amount only, no external SKU registration (`purchasegw`), so no change in ctech-wallet/ctech-billing.
- `SourcePurchase` entries need SKU and price; `SourceAchievement|SourceSeason` forbid both (existing invariants test stays green). Badges are never sold.
- Window: `From` inclusive, `Until` exclusive, injected clock `s.now()`; closes new purchases and grants, ownership permanent. Errors are RFC 7807 with stable `code`.
- Every purchase POST is idempotent (`Idempotency-Key`), as today; no new purchase code path (reuse `CreateSandbox`/`CreateReal`, `purchasegw` paid-in-full check, refund guard).
- Reads bounded; no scan; no new AWS resource; no new always-on cost. Grants are best-effort in the hand pipeline (never fail or delay a hand) and self-healing.
- UI: no new store request on first paint (store initial-load budget, `ui/CLAUDE.md`: avatar catalogs load lazily via `useInViewOnce`, like the purchase histories, and a deferred section reports loading, never paints half); pt-BR copy; items shown with the registry art; locked earnable items show "Conquista", never a price; keyboard/touch rules as elsewhere.
- Commits carry no Claude/Anthropic attribution. Zero lint warnings; `go test -race`; `scripts/standards-check.sh` (known local failure only from stale `.claude/worktrees/agent-*` dirs); layout gate must stay green (WebKit only in CI on this machine).

## Item prices (cents; fichas = cents x 1000) and SKUs (binding)

| Kind | id | SKU | cents |
|---|---|---|---|
| frame | felt-ring | poker_frame_felt_ring | 600 |
| frame | walnut-rail | poker_frame_walnut_rail | 800 |
| frame | oxblood-edge | poker_frame_oxblood_edge | 1000 |
| hat | top-hat | poker_hat_top_hat | 300 |
| hat | cowboy | poker_hat_cowboy | 400 |
| hat | beanie | poker_hat_beanie | 200 |
| accessory | shades | poker_accessory_shades | 300 |
| accessory | monocle | poker_accessory_monocle | 500 |
| accessory | mustache | poker_accessory_mustache | 200 |
| accessory | headset | poker_accessory_headset | 400 |

## Achievement grants (binding; each key+stars must exist in `achievements.Catalog`)

| Item | Achievement key | Stars | Why |
|---|---|---|---|
| hat dealer-visor | hands_played | 1 (100 hands) | first milestone |
| frame chip-ring | hands_played | 3 (5000 hands) | long-term regular |
| badge veteran | hands_played | 2 (1000 hands) | |
| hat laurel | wins | 3 (100 wins) | |
| hat crown | wins | 4 (1000 wins) | |
| accessory bandit-mask | bluff | 2 (5 bluffs) | |
| badge all-in-hero | all_in | 3 | |
| badge iron-nerve | folded_streak | 1 (100 hands no VPIP) | discipline |
| badge rainmaker | sandbox_chips_earned | 3 | |
| badge social-butterfly | won_full_table | 1 | ruling: no social achievement exists; mapped to the closest table-presence metric until one does (label "Anfitrião" kept; revisit when a hosting metric exists) |

A grant applies when an unlock's stars are `>=` the row's stars (so a skipped tier still grants).

## Review Focus

- A purchase outside `From..Until` or of a `Source != purchase` item is refused with a stable code and writes nothing. Task 2.
- `ListCatalog` for a kind with earnable items must not 500 (today it returns `ErrCatalogMismatch` for any Premium item without a SKU). Task 2.
- A repeated unlock or retried hand never double-grants or errors; a failed `Grant` never blocks the hand pipeline. Task 4.
- The self-heal grants exactly the rows the player qualifies for, once, and a player with no progress costs one read. Task 5.
- The store's first paint issues no avatar-catalog request; scrolling to a section issues exactly one. Task 7.

---

### Task 1: Purchasable catalog entries, prices, consistency test

**Files:** Modify `api/internal/cosmetics/catalog.go`, `api/internal/catalog/catalog.go` (`products`), tests `api/internal/cosmetics/catalog_test.go`, `api/internal/catalog/catalog_test.go`.
**Interfaces:** Produces 10 entries `{Kind, ID, Premium: true, Source: SourcePurchase, SKU, PriceFichas}` per the price table; `catalog.products` gains the 10 SKUs.
- [ ] **Step 1: Failing tests.** `TestAvatarPurchasableCatalog` (every table row exists with the exact SKU/PriceFichas/Source), `TestPurchasableFichasMatchCents` (for every `SourcePurchase` avatar entry: `PriceFichas == catalog.ProductByID(SKU).PriceCents*1000`; this lives in a package that can import both, e.g. `api/internal/cosmeticpurchase` test or a new `catalog` test importing `cosmetics`; check for an import cycle first), the literal price list in `catalog_test.go` extended (the file says changing a price must be deliberate).
- [ ] **Step 2:** `cd api && go test ./internal/cosmetics/ ./internal/catalog/ -run 'Purchasable|Fichas|Catalog' -v`; expect FAIL.
- [ ] **Step 3:** Add entries and products. **Step 4:** Expect PASS; `go test -race ./internal/cosmetics/... ./internal/catalog/...`. Commit `feat(cosmetics): purchasable avatar items and prices`.

### Task 2: Purchase routes, source-aware ListCatalog, window and source enforcement

**Files:** Modify `api/internal/cosmeticpurchase/service.go` (`ListCatalog`, `CreateSandbox`, `CreateReal`, error vars), `api/internal/api/v1/cosmeticpurchase.go` (`validCosmeticKind`, error mapping ~137-145), tests in both packages.
**Interfaces:** Produces `var ErrNotPurchasable`, reuses `ErrNotAvailable` (phase 1); `CatalogEntry` (service type) gains `Source string json:"source"` and `Available bool json:"available"`; `validCosmeticKind` accepts deck, felt, and `IsAvatarKind` kinds except badge; `Create*` return `ErrNotPurchasable` when `Source != SourcePurchase` and `ErrNotAvailable` when `!Available(kind,id,s.now())`; handler maps them to 409/400 with codes `not_purchasable` and `not_available` (follow the neighbouring cases' style).
- [ ] **Step 1: Failing tests.** `TestListCatalogIncludesEarnableWithoutPrice` (frame list returns earnable and purchasable entries; earnable have `Source "achievement"`, no price, `Owned` per entitlement; no `ErrCatalogMismatch`), `TestCreateRejectsNonPurchasable` (both methods), `TestCreateRejectsOutsideWindow` (use `cosmetics.WithCatalogForTest`; fake clock; nothing written), `TestPurchaseRouteAcceptsAvatarKindsRejectsBadge` (route level: `frame|hat|accessory` pass the kind guard, `badge` 400). Existing deck/felt tests unchanged.
- [ ] **Step 2:** Run (integration tag + DynamoDB local on :8555 for cosmeticpurchase); expect FAIL. **Step 3:** Implement; `ListCatalog` price lookup only for `Source == SourcePurchase` (or empty Source with SKU for deck/felt). **Step 4:** Expect PASS, `-race`. Commit `feat(cosmeticpurchase): sell avatar items, source-aware catalog, window enforcement`.

### Task 3: Achievement grant table

**Files:** Create `api/internal/cosmetics/achievementgrants.go`, test `api/internal/cosmeticpurchase/achievementgrants_test.go` (or `app/`, wherever both `cosmetics` and `achievements` import cleanly).
**Interfaces:** Produces `type AchievementGrant struct{Key string; Stars int; Kind Kind; ID string}`; `var AchievementGrants = []AchievementGrant{...}` (the 10 rows above); `func GrantsFor(key string, stars int) []AchievementGrant` (rows whose key matches and `row.Stars <= stars`).
- [ ] **Step 1: Failing tests.** `TestEveryGrantTargetsAnAchievementItem` (each row's item exists, `Source == SourceAchievement`, and every achievement item appears in exactly one row), `TestEveryGrantKeyAndStarsExist` (each key/stars exists in `achievements.Catalog` tiers), `TestGrantsForSkippedTier` (`GrantsFor("hands_played", 3)` returns dealer-visor, veteran and chip-ring).
- [ ] **Step 2:** FAIL. **Step 3:** Implement. **Step 4:** PASS. Commit `feat(cosmetics): achievement to item grant table`.

### Task 4: Grant on unlock, with a screen notice

**Files:** Modify `api/internal/app/handpipeline.go` (unlock loop ~270-284; add a small `cosmeticGranter` interface and a field), the Fx wiring in `app/` where the pipeline is built, `proto/poker.proto` (`ServerMessage`: add `string cosmetic_kind = N; string cosmetic_id = N+1;` with the next free numbers; type value `cosmetic_unlocked` documented in the `type` comment) and regenerated `api`, `cli`, `ui` proto outputs; tests in `app/` and `api/v1/` proto round trip.
**Interfaces:** Consumes `cosmetics.GrantsFor`, `cosmeticpurchase.Service.Grant(ctx, playerID, kind, id, reason)` with reason `achievement:<key>:<stars>`. Produces websocket frame `{type:"cosmetic_unlocked", cosmetic_kind, cosmetic_id}` broadcast to `tableID#playerID` once per newly granted item (a repeat `Grant` that was a no-op sends nothing: extend `Grant` to report whether it created the entitlement, e.g. `Grant(...) (created bool, err error)`, and update every caller/test).
- [ ] **Step 1: Failing tests.** pipeline-level: an unlock with a mapped key grants the item and sends one `cosmetic_unlocked`; a repeated unlock sends none and writes once; `Grant` error is logged and the loop continues (other unlocks, leaderboard, achievement frame still processed); an unmapped unlock grants nothing; proto round trip for the two new fields.
- [ ] **Step 2:** FAIL. **Step 3:** Implement. Regenerate protos with the CI-pinned toolchain exactly as in the phase 1 fix (protoc v36.1 and protoc-gen-go v1.36.12 downloaded into a new empty scratch directory, put first on PATH for the generate run only, never overwrite system binaries; confirm a second run gives no diff and headers read `protoc v7.36.1`; do not hand-edit generated files). **Step 4:** PASS, `go vet -tags integration ./...`, `-race`. Commit `feat(achievements): grant avatar items on unlock`.

### Task 5: Self-heal on the owned-list read

**Files:** Modify `api/internal/api/v1/avatarcosmetics.go` (owned route), the Fx wiring, a small service method `GrantEarned` in `cosmeticpurchase` (or `app/`), tests.
**Interfaces:** `GrantEarned(ctx, playerID, mode string) error`: reads `achievements.Store.AllAchievements(ctx, playerID, mode)` once, derives the highest unlocked stars per key from progress and the catalog tiers, and `Grant`s every row of `AchievementGrants` the player qualifies for (reason `achievement:<key>:<stars>:backfill`); best-effort, error logged and ignored by the route. Called by `GET /players/me/cosmetics/:kind/owned` before listing, for the sandbox mode and real mode both if progress is per mode (read how `AllAchievements` and unlock stars are stored; take the max across modes).
- [ ] **Step 1: Failing tests.** a player with progress that qualifies for two rows and no entitlements gets both after one owned-list call; a second call performs no writes; a player with no progress grants nothing and costs one progress read; a read error does not fail the owned list.
- [ ] **Step 2:** FAIL. **Step 3:** Implement. Ceiling to note with a `ponytail:` comment: one progress read per owned-list call (profile pickers only); add a short per-player cooldown only if measured. **Step 4:** PASS, `-race`. Commit `feat(cosmetics): self-heal earned avatar items on owned read`.

### Task 6: UI: toast for unlocked items

**Files:** Modify `ui/src/lib/hooks/useTableRealtimeSession.ts` (~497, alongside `achievement_unlocked`), its tests, `ui/src/lib/avatarCosmetics.ts` if a label helper is needed.
- [ ] **Step 1: Failing test:** a `cosmetic_unlocked` frame with `cosmetic_kind: 'hat'`, `cosmetic_id: 'crown'` calls the existing notification path with pt-BR copy naming the item ("Novo item desbloqueado: Coroa", label from `avatarItemLabel`); an unknown id still notifies with the id; a frame without an id is ignored. Match how `achievement_unlocked` is tested (`useTableRealtime.test.tsx` ~674).
- [ ] **Step 2:** FAIL. **Step 3:** Implement using `pushNotification`. **Step 4:** PASS. Commit `feat(ui): notice when an avatar item is unlocked`.

### Task 7: UI: store sections for frame, hat and accessory

**Files:** Modify `ui/src/lib/api/cosmeticPurchases.ts` (`CosmeticKind` widened, `CosmeticCatalogEntry` gains `source`, `available`), `ui/src/components/store/CosmeticStoreSection.tsx` (new `FrameStoreSection`, `HatStoreSection`, `AccessoryStoreSection` built on the existing `CosmeticGrid`; preview = the registry art over a neutral avatar chip; locked earnable entries show "Conquista" and no buy button; unavailable (window closed, unowned) shows "Indisponível"; `cosmeticActivityRows` labels for the new kinds), `ui/src/app/(app)/store/page.tsx` (lazy catalogs and histories via `useInViewOnce`, one section per kind, directory counters if the page has them, anchors), `CosmeticPurchaseDialog` if it hard-codes deck/felt copy, tests beside each, `ui/src/dev/mockRuntime.ts` (mock catalog for the three kinds: purchasable and earnable entries, one owned).
**Interfaces:** query keys `['wallet','cosmetic-catalog', kind]` and `['wallet','cosmetic-purchases', kind]` like deck/felt, so wallet invalidation and the `cosmetic_purchase_update` websocket handling already work; the avatar sections' catalog queries are `enabled` only after their section has been in view (`useInViewOnce`), so first paint adds no request.
- [ ] **Step 1: Failing tests.** first render issues no avatar catalog request, section in view issues exactly one; purchasable entry shows price and buy flow; earnable entry shows "Conquista" and no buy button; unowned unavailable entry shows "Indisponível"; owned entry shows "Equipar no perfil" link to `/player-profile`; activity rows label the new kinds; existing deck/felt tests unchanged.
- [ ] **Step 2:** FAIL. **Step 3:** Implement. **Step 4:** PASS; run `npx vitest run src/components/store "src/app/(app)/store"`, eslint zero warnings. One batched real-browser pass on `npm run dev:mock` at 1280px and 390px (screenshots to the scratchpad, never committed). Commit `feat(ui): avatar item store sections`.

### Task 8: Docs, guide, spec, full gate

**Files:** `api/CLAUDE.md` (cosmetics purchase and grants), `ui/CLAUDE.md` (store sections, lazy catalogs, toast), `docs/README.md`, `docs/specs/2026-11-14-avatar-cosmetics.md` (Status "all phases implemented; season grants still not triggered"; the price table; the grant table), the in-app guide store/profile pages (`ui/src/app/(marketing)/guide/**`: names must match the UI exactly), `OVERVIEW.md` section on cosmetics ("broader cosmetics remain open" is stale).
- [ ] **Step 1:** Update docs from the real code. **Step 2:** Gate: `cd api && go vet ./... && go vet -tags integration ./... && go test -race ./...` and, with DynamoDB local on :8555 (and valkey on :6399 for leaderboard), `go test -race -tags integration` for packages changed on this branch; `cd ui && npx vitest run && npx eslint --max-warnings 0 <touched>` and `npm run e2e` with `--output <scratchpad>` (root-owned `ui/test-results/`); `scripts/standards-check.sh`. **Step 3:** Commit `docs: avatar cosmetics phase 3`.

## Self-review

- Spec phase 3 coverage: purchasable kinds in store with fichas and PIX (T1, T2, T7), `products` entries (T1), window enforcement on purchases (T2), achievement grant table and WS notice (T3, T4, T6), backfill (T5), docs (T8). Season grant trigger remains out of scope (items and windows exist; no period-close hook found). Deviations: `Grant` now returns `(created bool, err)` (T4).
- Names consistent: `AchievementGrants`, `GrantsFor`, `ErrNotPurchasable`, `ErrNotAvailable`, `GrantEarned`, frame type `cosmetic_unlocked` with `cosmetic_kind`/`cosmetic_id`.
