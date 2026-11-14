# Avatar Cosmetics, Phase 1 (backend and wire) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Players can own and equip `frame`, `hat`, `accessory` and `badge` items, and every other player sees the equipped ids on the table seat, in showcase/people payloads and in the leaderboard.

**Architecture:** Extend `cosmetics.CatalogEntry` with `Source` and a sale window; one profile field per slot; a single `cosmetics.Decor` value travels profile -> `SetIdentityCmd` -> `hand.Player` -> `SeatView` -> proto `Seat`. Leaderboard fills decoration in `resolveNames` from profiles it already batch-loads. Ids only, no new table, no new AWS resource.

**Tech Stack:** Go (Fiber v3, DynamoDB via api-commons), protobuf (`scripts/generate-proto.sh`), `go test -race`.

**Spec:** `docs/specs/2026-11-14-avatar-cosmetics.md` (phase 1 of 3). UI rendering is phase 2; store routes, PIX products and achievement grants are phase 3.

## Global Constraints

- Four slots, kinds equal slots: `frame`, `hat`, `accessory`, `badge` (max 3, existing `maxEquippedBadges`).
- Sources: `achievement`, `purchase`, `season`. `purchase` requires `SKU` and `PriceFichas`; the other two forbid both.
- A sale window (`From`/`Until`, zero = open) closes only new grants and purchases; ownership is permanent.
- Proto `Seat` new fields: `frame_id=22`, `hat_id=23`, `accessory_id=24`, `repeated badge_ids=25`, all optional/ids only.
- Equip fails closed (nil ownership checker rejects, existing `requireCosmetic`); render fails open (unknown id means none).
- Errors are RFC 7807 via `problem`; use the injected clock (`Service.now`), never `time.Now` in services.
- No new AWS resource, no new DynamoDB table, no new external call. Leaderboard stays at the same request count.
- Docs updated in the same PR (`api/CLAUDE.md`, `docs/README.md`). Commit messages carry no Claude/Anthropic attribution.

## Review Focus

- Unequipping everything while name/avatar/badge are all empty must still reach the seat (today's `SetPlayerIdentityForActor` no-ops on all-empty). Test in Task 5.
- A table item persisted before this change (no decor attributes) must load as empty decor. Test in Task 5.
- A leaderboard row whose profile is missing keeps empty decoration and is never dropped or blanked. Test in Task 7.
- `Grant` outside the window returns an error and writes nothing; a repeat grant inside it stays a no-op. Test in Task 2.
- Equipping a badge list preserves order and rejects duplicates and a fourth badge. Existing tests stay green, plus one for order. Task 3.

---

## File Structure

- `api/internal/cosmetics/catalog.go`: kinds, `Source`, window, `Available`, `Decor`.
- `api/internal/player/{model,store,service}.go`: new profile fields, equip, `PlayerProfile.Decor()`.
- `api/internal/engine/hand/{hand,snapshot}.go`, `api/internal/table/{commands,actor_loading}.go`: decor on the seat.
- `api/internal/api/v1/{tableidentity,tablews,player,leaderboard,social,avatarcosmetics}.go`, `api/internal/buyin/service.go`, `api/internal/app/app.go`: wiring.
- `proto/poker.proto` plus generated files for api, cli and ui.

### Task 1: Catalog kinds, source, window, Decor

**Files:** Modify `api/internal/cosmetics/catalog.go`; Test `api/internal/cosmetics/catalog_test.go`.

**Interfaces:**
- Produces: `KindHat`, `KindAccessory` (`Kind` consts `"hat"`, `"accessory"`); `type Source string` with `SourcePurchase`, `SourceAchievement`, `SourceSeason`; `CatalogEntry` gains `Source Source`, `From, Until time.Time`; `func Available(kind Kind, id string, now time.Time) bool`; `func IsAvatarKind(kind Kind) bool`; `type Decor struct { Frame, Hat, Accessory string; Badges []string }` with `func (d Decor) Empty() bool` and `func (d Decor) Equal(o Decor) bool`.
- Existing deck/felt entries keep `Source` empty and are exempt from the source invariants.

- [ ] **Step 1: Failing tests.** `TestAvatarCatalogInvariants` (every avatar-kind entry has a `Source`; `SourcePurchase` has SKU and `PriceFichas > 0`; the others have neither; ids unique per kind; `From` before `Until` when both set). `TestAvailable` with `now` before `From` (false), inside (true), after `Until` (false), zero window (true), unknown id (false). `TestDecorEmptyEqual` (nil badges equals empty badges; order of badges matters).
- [ ] **Step 2:** Run `cd api && go test ./internal/cosmetics/ -run 'Avatar|Available|Decor' -v`; expect FAIL (undefined).
- [ ] **Step 3:** Implement the types and functions above. Set `Source: SourceSeason` on the four existing frame/badge entries. Add no new items yet (phase 2 ships art and ids).
- [ ] **Step 4:** Same command; expect PASS. Run `go test ./internal/cosmetics/...` fully.
- [ ] **Step 5:** `git commit -m "feat(cosmetics): hat/accessory kinds, item source, sale window, Decor"`.

### Task 2: Grant respects the window

**Files:** Modify `api/internal/cosmeticpurchase/service.go` (`Grant`, ~:603); Test `cosmeticpurchase/service_test.go`.

**Interfaces:** Consumes `cosmetics.Available`. Produces `var ErrNotAvailable = errors.New("cosmeticpurchase: item is outside its sale window")`.

- [ ] **Step 1: Failing test** `TestGrantOutsideWindow`: fake `now` after `Until` of a test entry (use a catalog test seam, or an entry added to the catalog in `_test` via the package's existing override approach; if none exists, add an unexported `catalogOverride` in `cosmetics` used only by tests). Assert `errors.Is(err, ErrNotAvailable)` and that no entitlement row exists. Add `TestGrantRepeatInsideWindowIsNoop`.
- [ ] **Step 2:** `go test ./internal/cosmeticpurchase/ -run Grant -v`; expect FAIL.
- [ ] **Step 3:** In `Grant`, after `IsKnown`, return `ErrNotAvailable` when `!cosmetics.Available(kind, itemID, s.now())`. A repeat grant of an already owned item must still be a no-op: check ownership first only if the window is closed and the entitlement is active, otherwise error (decide by test).
- [ ] **Step 4:** Expect PASS; run the package with `-race`.
- [ ] **Step 5:** Commit `feat(cosmeticpurchase): grant honours the item sale window`.

### Task 3: Profile slots (hat, accessory), Decor(), equip routes

**Files:** Modify `player/model.go`, `player/store.go`, `player/service.go`, `api/v1/player.go` (PATCH ~:383), `api/v1/avatarcosmetics.go`; Tests `player/service_test.go`, `api/v1/avatarcosmetics_test.go`, `api/v1/player_test.go`.

**Interfaces:**
- Produces: `PlayerProfile.EquippedHatID`, `EquippedAccessoryID` (`dynamodbav:"equipped_hat_id,omitempty" json:"equipped_hat_id,omitempty"`, same for accessory); `func (p *PlayerProfile) Decor() cosmetics.Decor`; `Service.SetEquippedHat(ctx, userID, id string) (*PlayerProfile, error)`, `SetEquippedAccessory` (same shape); `ErrInvalidHat`, `ErrInvalidAccessory`; `Store.SetEquippedHat/SetEquippedAccessory`; PATCH body `equipped_hat_id`, `equipped_accessory_id` (`*string`).
- `validFrameOrBadgeKind` becomes `validAvatarKind` accepting the four kinds, error text lists all four.

- [ ] **Step 1: Failing tests.** `TestSetEquippedHat` and `...Accessory` mirroring the existing frame tests (unknown id -> `ErrInvalidHat`; unowned -> `ErrCosmeticNotOwned`; empty unequips). `TestDecor` (profile with all slots maps to `Decor`, badges order preserved). `TestEquipBadgesKeepsOrder`. `TestOwnedRouteAcceptsHatAndAccessory`, `TestPatchProfileEquipHat` (400 for unknown, 400 for not owned, 200 and persisted for owned).
- [ ] **Step 2:** `go test ./internal/player/ ./internal/api/v1/ -run 'Equipped|Decor|Owned|PatchProfile' -v`; expect FAIL.
- [ ] **Step 3:** Implement. Collapse the three single-id store setters into one private `setEquipped(ctx, userID, attr, value string)` and the three service setters into one private `setEquippedItem(ctx, userID string, kind cosmetics.Kind, id string, invalid error)` that the exported methods call. Reuse `requireCosmetic`.
- [ ] **Step 4:** Expect PASS; `go test -race ./internal/player/ ./internal/api/v1/`.
- [ ] **Step 5:** Commit `feat(player): equip hat and accessory, PlayerProfile.Decor`.

### Task 4: Proto fields and regenerated clients

**Files:** Modify `proto/poker.proto` (`message Seat`, after `pending_exit = 21`); regenerate via `scripts/generate-proto.sh` (api, cli, ui outputs); Test `api/internal/api/v1/tablews_conversion_test.go`.

**Interfaces:** Produces Go fields `FrameId, HatId, AccessoryId *string`, `BadgeIds []string` on `pokerproto.Seat`.

- [ ] **Step 1:** Add `optional string frame_id = 22; optional string hat_id = 23; optional string accessory_id = 24; repeated string badge_ids = 25;` with a one-line comment that ids are public and may be unknown to older clients.
- [ ] **Step 2:** Run `scripts/generate-proto.sh`; expect generated diffs in `api/internal/api/v1/proto`, `cli/internal/proto`, `ui/src/lib/api/proto`. Run `cd api && go build ./... && cd ../cli && go build ./... && cd ../ui && npx tsc --noEmit`; expect clean.
- [ ] **Step 3:** Commit `feat(proto): seat carries equipped cosmetic ids`.

### Task 5: Decor on the seat (engine, snapshot, actor)

**Files:** Modify `engine/hand/hand.go` (`Player`, `SetPlayerIdentityForActor` ~:490), `engine/hand/snapshot.go` (`SeatView` ~:140, build ~:275), `table/commands.go` (`SetIdentityCmd`), `table/actor_loading.go` (`handleSetIdentity`); Tests beside each.

**Interfaces:**
- Consumes `cosmetics.Decor`. Produces `hand.Player.Decor cosmetics.Decor` (`dynamodbav:"decor,omitempty"`), `SeatView.FrameID, HatID, AccessoryID string` and `BadgeIDs []string` (`json:"...,omitempty"`), `SetIdentityCmd.Decor cosmetics.Decor`, and `func (t *Table) SetPlayerIdentityForActor(playerID, name, avatarURL, playstyleBadge string, decor cosmetics.Decor) bool`.

- [ ] **Step 1: Failing tests.** `TestSetPlayerIdentityDecorChangeReportsChanged`; `TestSetPlayerIdentityClearsDecorWhenAllElseEmpty` (name/avatar/badge empty, decor goes from set to empty: returns true and the seat is cleared); `TestSeatViewCarriesDecor`; `TestPlayerLoadsWithoutDecorAttributes` (round trip an item missing `decor`, decor empty); an actor-level test that `SetIdentityCmd{Decor: ...}` commits once and a repeat is a no-op without a version bump.
- [ ] **Step 2:** `go test ./internal/engine/hand/ ./internal/table/ -run 'Decor|Identity' -v`; expect FAIL.
- [ ] **Step 3:** Implement. The no-op guard becomes "nothing to set" only when decor is also empty AND the seat has no decor; changed-detection compares `Decor.Equal`. `handleSetIdentity`'s early return uses the same rule.
- [ ] **Step 4:** Expect PASS; `go test -race ./internal/engine/hand/ ./internal/table/`.
- [ ] **Step 5:** Commit `feat(table): seat identity carries equipped cosmetics`.

### Task 6: Wire identity end to end (gateway, buy-in, proto, equip push)

**Files:** Modify `api/v1/tableidentity.go` (`identityCmdFor`), `buyin/service.go` (~:371), `api/v1/tablews.go` (seat build ~:1048), `api/v1/player.go` (call the existing `tableIdentityPusher.push` after any `equipped_*` change, as rename does); Tests `tableidentity_test.go`, `tablews_conversion_test.go`, `buyin/service_test.go`, `player_test.go`.

**Interfaces:** Consumes `PlayerProfile.Decor()`, `SetIdentityCmd.Decor`, `pokerproto.Seat` fields from Task 4. Produces nothing new.

- [ ] **Step 1: Failing tests.** `TestIdentityCmdForIncludesDecor`; `TestSeatConversionCarriesDecor` (empty fields stay nil/absent, badges keep order); `TestBuyinIdentityIncludesDecor`; `TestEquipPushesIdentityToTable` (PATCH `equipped_hat_id` triggers one push, a PATCH that changes nothing equipped does not).
- [ ] **Step 2:** Run those tests; expect FAIL.
- [ ] **Step 3:** Implement. Update `identityCmdFor`'s doc comment: it now carries four things together for the same reason (one unit replace). Both writers (`identityCmdFor`, buyin) must use `profile.Decor()`; do not hand-assemble.
- [ ] **Step 4:** Expect PASS; `go test -race ./internal/api/v1/ ./internal/buyin/`.
- [ ] **Step 5:** Commit `feat(api): equipped cosmetics reach every seat`.

### Task 7: Leaderboard avatar and decoration

**Files:** Modify `leaderboard/service.go` (`Entry`), `api/v1/leaderboard.go` (`resolveNames`, `top`, `me` if it returns an entry); Test `api/v1/leaderboard_test.go`.

**Interfaces:** Produces `Entry` response fields `AvatarURL string json:"avatar_url,omitempty"`, `EquippedFrameID`, `EquippedHatID`, `EquippedAccessoryID string` and `EquippedBadgeIDs []string` (all `omitempty`, `json:"equipped_..."`), tagged `dynamodbav:"-"` so nothing is stored in the row. `resolveNames` becomes `resolveProfiles` and fills them from the same `GetMany`; the avatar base URL comes from the handler (inject `cfg.AvatarBaseURL` the way `social.go` does).

- [ ] **Step 1: Failing tests.** `TestTopIncludesAvatarAndDecor`; `TestTopMissingProfileKeepsRow` (row stays, name stays from the stored value, decoration empty); `TestTopStillOneBatchGet` (the fake players store is called once per page).
- [ ] **Step 2:** `go test ./internal/api/v1/ -run 'Top' -v`; expect FAIL.
- [ ] **Step 3:** Implement. Do not add a second profile read.
- [ ] **Step 4:** Expect PASS; `-race`.
- [ ] **Step 5:** Commit `feat(leaderboard): rows carry avatar and equipped cosmetics`.

### Task 8: Showcase, people and hand-history payloads

**Files:** Modify `api/v1/player.go` (showcase maps ~:657 and ~:743; hand opponents ~:586), `api/v1/social.go` (~:736 state, ~:276 actor); Tests in the matching `_test.go`.

**Interfaces:** Add the same five `equipped_*` fields next to every `avatar_url` these payloads already return, built from `profile.Decor()`. A private/anonymised profile must expose nothing new (reuse the existing privacy branches; a test per payload).

- [ ] **Step 1: Failing tests:** one per payload, plus `TestPrivateShowcaseExposesNoDecor`.
- [ ] **Step 2:** Run; expect FAIL. **Step 3:** Implement with one small helper `decorFields(profile) map[string]any` (or struct embedding) reused by all four sites. **Step 4:** Expect PASS, `-race`.
- [ ] **Step 5:** Commit `feat(api): showcase, people and hand opponents expose equipped cosmetics`.

### Task 9: Refund guard for avatar kinds

**Files:** Modify `app/app.go` `wireCosmeticCurrentSelection` (~:445); Test `app/app_test.go` or the existing cosmetic selection test.

**Interfaces:** Consumes `PlayerProfile.EquippedFrameID/EquippedHatID/EquippedAccessoryID/EquippedBadgeIDs`. For `badge` the check is membership, not equality: extend the callback's contract minimally (return a second value or an `IsCurrent(kind, id)` style func) and keep deck/felt behaviour identical.

- [ ] **Step 1: Failing tests:** refund of an equipped frame, hat, accessory and badge is refused with the existing "current selection" error; refund of a non-equipped one is allowed.
- [ ] **Step 2:** Run; expect FAIL. **Step 3:** Implement. **Step 4:** Expect PASS, `-race`.
- [ ] **Step 5:** Commit `fix(cosmetics): block refund of an equipped avatar item`.

### Task 10: Docs and spec sync

**Files:** Modify `api/CLAUDE.md` (cosmetics/identity section), `docs/README.md` (index entry), `docs/specs/2026-11-14-avatar-cosmetics.md`.

- [ ] **Step 1:** Document: the four slots, `Source` and window semantics, `Decor` as the single identity unit, leaderboard fields, why ids only. In the spec, drop the `Slot` field (redundant: Kind is the slot) and note that purchase-window enforcement lands in phase 3 with the purchasable kinds.
- [ ] **Step 2:** Run the full gate: `cd api && go vet ./... && go test -race ./... && cd .. && scripts/standards-check.sh`; expect all green.
- [ ] **Step 3:** Commit `docs: avatar cosmetics phase 1`.

## Self-review

- Spec phase 1 coverage: catalog fields and kinds (T1), window (T1/T2; purchase side in phase 3), profile fields and validation (T3), refund guard (T9), proto, snapshot, identity push (T4-T6), leaderboard (T7), other payloads (T8), docs (T10). Deviations: `Slot` dropped, noted in T10.
- Types checked across tasks: `cosmetics.Decor` (T1) is the value in T3, T5, T6; `SetPlayerIdentityForActor` has one new signature (T5) used by T6 and `handleSetIdentity`.
