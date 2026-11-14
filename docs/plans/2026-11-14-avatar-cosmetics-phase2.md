# Avatar Cosmetics, Phase 2 (art and rendering) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every UI task: invoke `impeccable:impeccable` (then `ponytail:ponytail`) before editing, per `ctech-dev-standards` and the project's rule that pixel-perfect UI goes through impeccable, including subagents.

**Goal:** Equipped frames, hats, accessories and badges are drawn on the avatar at the table seat, in the profile (equip UI with live preview) and in the leaderboard, with a launch set of about 24 items.

**Architecture:** One 64x64 SVG art contract. Frames, hats and accessories are small inline React SVG components registered by id in `lib/avatarCosmetics.ts`; badges are a lucide glyph plus a tone token (no custom art). `PlayerAvatar` takes four primitive props and layers `image < accessory < hat < frame`. Seat-scope CSS lives in `renderer.css`; profile-only CSS stays in `app.css`.

**Tech Stack:** Next.js/React, TypeScript, CSS tokens in `src/app/base.css`, lucide-react (already installed), vitest, Playwright layout gate.

**Spec:** `docs/specs/2026-11-14-avatar-cosmetics.md` (phase 2 of 3). Phase 1 plan: `docs/plans/2026-11-14-avatar-cosmetics-phase1.md` (backend and wire, branch `feat/avatar-cosmetics`; this phase stacks on it).

## Global Constraints

- Art box: every frame/hat/accessory is drawn in `viewBox="0 0 64 64"` with the avatar circle at centre, r=32. Nothing is drawn outside that box (spec: no overflow, no `overflow: visible` ancestors needed).
- Layer order, bottom to top: avatar image, accessory (eye line, y 24-40), hat (top, y 0-26), frame (ring, stroke inside the box).
- Must read at seat sizes 24, 34, 36, 46, 52px and at profile 72px; a detail finer than 1.5px at 24px must be dropped, not shrunk.
- No new raw hex/px in component CSS or SVG attributes: colours are named tokens in `src/app/base.css` `:root` (new ones prefixed `--cosmetic-`), documented in `DESIGN.md`. SVG fills use `var(--token)`.
- No animation in this release. No width/height/padding/margin animation ever; no entrance animation that makes content visible (WebKit).
- Seat CSS in `renderer.css` only; profile CSS in `app.css`; never the reverse (narrowest-stylesheet rule).
- `Seat` stays memoised: pass only primitive props (`frameId`, `hatId`, `accessoryId`, and a stable `badgeIds` array reference or a joined string); `tableRenderBudget.test.tsx` count must not change.
- Decorations are `aria-hidden` and non-interactive (`pointer-events: none`); the avatar's accessible name is unchanged. Equip pickers keep radiogroup/checkbox semantics, visible focus, 44px targets.
- The frame must not use `outline` (collides with the folded outline and all-in `box-shadow`); use the SVG ring overlay.
- Names and copy are pt-BR hardcoded in the registry (no i18n library exists).
- Unknown ids render nothing (fail open). Items present in the registry but absent from the server catalog are never offered in the picker (the picker lists owned ids only).
- Zero ESLint warnings, `tsc` clean for touched files, vitest green, `npm run e2e` layout gate green on Chromium/Firefox/WebKit. Commit messages carry no Claude/Anthropic attribution.

## Item set (ids are binding; labels pt-BR)

| Slot | id | Label | Source | Art note |
|---|---|---|---|---|
| frame | `season-2026-q3` | Moldura da Temporada | season (exists) | double ring, gold-pale |
| frame | `champion-gold` | Moldura de Campeão | season (exists) | solid gold ring with inner highlight |
| frame | `chip-ring` | Borda de Fichas | achievement | casino chip edge: 12 alternating dash segments, oxblood and card-paper |
| frame | `felt-ring` | Borda de Feltro | purchase | felt-green ring, inner rail line |
| frame | `walnut-rail` | Borda de Nogueira | purchase | wood-rail ring with highlight arc |
| frame | `oxblood-edge` | Borda Oxblood | purchase | brand-red ring, thin gold inner line |
| hat | `dealer-visor` | Viseira do Dealer | achievement | green eyeshade visor |
| hat | `crown` | Coroa | achievement | 3-point gold crown |
| hat | `laurel` | Louros | achievement | gold laurel wreath arc |
| hat | `top-hat` | Cartola | purchase | black top hat, oxblood band |
| hat | `cowboy` | Chapéu de Cowboy | purchase | walnut cowboy hat |
| hat | `beanie` | Gorro | purchase | wine beanie, pompom |
| accessory | `bandit-mask` | Máscara de Bandido | achievement | eye-line mask |
| accessory | `shades` | Óculos Escuros | purchase | flat sunglasses |
| accessory | `monocle` | Monóculo | purchase | single gold monocle, chain omitted |
| accessory | `mustache` | Bigode | purchase | mustache at mouth line (y 40-48) |
| accessory | `headset` | Headset | purchase | headband arc plus one ear cup |
| badge | `season-2026-q3-top10` | Top 10 da Temporada | season (exists) | lucide `Medal` |
| badge | `season-2026-q3-champion` | Campeão da Temporada | season (exists) | lucide `Trophy` |
| badge | `all-in-hero` | Herói do All-in | achievement | lucide `Flame` |
| badge | `iron-nerve` | Sangue Frio | achievement | lucide `ShieldCheck` |
| badge | `rainmaker` | Chuva de Fichas | achievement | lucide `Gem` |
| badge | `veteran` | Veterano | achievement | lucide `Spade` |
| badge | `social-butterfly` | Anfitrião | achievement | lucide `Users` |

Server catalog entries are added in this phase only for `Source` achievement/season items (Task 1). Purchasable items' server entries, SKUs and prices land in phase 3; their art ships now and stays unobtainable until then.

## Review Focus

- A hat or ring must never cause horizontal scroll or clipping at any of the six layout-gate viewports. Pinned by the e2e gate in Task 7.
- A folded or all-in seat with a frame must keep its folded dashed outline / all-in glow visible. Test in Task 4.
- An id the registry does not know (new server item, old client) renders no decoration and no crash. Test in Task 4.
- A seat with no decoration renders byte-identical markup to today (no wrapper, no extra node). Test in Task 4.
- The picker never offers an item the player does not own, and un-equipping works for every slot. Test in Task 6.

---

### Task 1: Server catalog entries for earnable items

**Files:** Modify `api/internal/cosmetics/catalog.go`, `api/internal/cosmetics/catalog_test.go`.
**Interfaces:** Produces catalog entries (Premium true, no SKU/price): frame `chip-ring`; hats `dealer-visor`, `crown`, `laurel`; accessory `bandit-mask`; badges `all-in-hero`, `iron-nerve`, `rainmaker`, `veteran`, `social-butterfly`; all `SourceAchievement`. The two existing frames and two badges keep `SourceSeason`.

- [ ] **Step 1:** Extend `TestAvatarCatalogInvariants` with `TestAvatarCatalogLaunchIDs`: every earnable id in the table above exists with the right kind and source.
- [ ] **Step 2:** Run `cd api && go test ./internal/cosmetics/ -run LaunchIDs -v`; expect FAIL.
- [ ] **Step 3:** Add the entries; keep the invariant test green.
- [ ] **Step 4:** Run `go test -race ./internal/cosmetics/...`; expect PASS. Commit `feat(cosmetics): earnable avatar items in the catalog`.

### Task 2: Tokens, registry, art contract

**Files:** Modify `ui/src/app/base.css` (`--cosmetic-*` tokens), `ui/DESIGN.md`, `ui/src/lib/avatarCosmetics.ts`; Test `ui/src/lib/avatarCosmetics.test.ts`.
**Interfaces:**
- Produces `AVATAR_ITEMS: Record<AvatarSlot, Record<string, {label: string}>>` for slots `frame|hat|accessory|badge` with exactly the ids in the Item set table; `type AvatarSlot`; `avatarItemLabel(slot: AvatarSlot, id: string): string` (falls back to the id); keep `avatarFrameLabel`/`avatarBadgeLabel` as thin wrappers so existing callers compile; `listOwnedAvatarCosmetics(kind)` widened to the four slots.
- Produces tokens: `--cosmetic-gold`, `--cosmetic-gold-pale`, `--cosmetic-oxblood`, `--cosmetic-felt`, `--cosmetic-walnut`, `--cosmetic-paper`, `--cosmetic-ink`, `--cosmetic-wine`, each aliasing an existing palette token (no new hex unless the palette truly lacks it; state which in DESIGN.md).

- [ ] **Step 1: Failing tests.** `TestRegistryHasLaunchSet` (24-minus-none: 6/6/5/7 ids, labels match the table), `TestUnknownIdFallsBackToId`.
- [ ] **Step 2:** `cd ui && npx vitest run src/lib/avatarCosmetics.test.ts`; expect FAIL.
- [ ] **Step 3:** Implement registry and tokens; document tokens and the 64x64 art contract in `DESIGN.md`.
- [ ] **Step 4:** Expect PASS; `npx eslint src/lib/avatarCosmetics.ts`. Commit `feat(ui): avatar cosmetics registry and tokens`.

### Task 3: Frame art

**Files:** Create `ui/src/components/avatar/frames.tsx`, `frames.test.tsx`.
**Interfaces:** Produces `FRAME_ART: Record<string, () => JSX.Element>` (one SVG per frame id, `viewBox="0 0 64 64"`, `aria-hidden`, `focusable="false"`, fills via `var(--cosmetic-*)`, ring stroke fully inside the box). Consumes Task 2 tokens/ids.

- [ ] **Step 1: Failing tests.** `TestEveryFrameIdHasArt` (all six ids), `TestFrameSvgContract` (viewBox, aria-hidden, no hex literal and no `outline` in the markup).
- [ ] **Step 2:** Run; expect FAIL. **Step 3:** Draw the six frames per the Item set art notes. Render each at 24/36/52/72px in a throwaway page (not committed), screenshot, and attach the images to the report; drop any detail unreadable at 24px.
- [ ] **Step 4:** Expect PASS. Commit `feat(ui): avatar frame art`.

### Task 4: PlayerAvatar layering

**Files:** Modify `ui/src/components/ui/player-avatar.tsx`, `ui/src/app/renderer.css` (seat scope), `ui/src/app/(app)/app.css` (remove the `[data-frame]` outline rules; keep profile-only badge styling if still used); Test `ui/src/components/ui/player-avatar.test.tsx`, `ui/src/components/table/Seat.test.tsx`.
**Interfaces:** `PlayerAvatar` props `frameId?: string; hatId?: string; accessoryId?: string; badgeIds?: string[]` (primitives; `badgeIds` memoised by caller). With none set the markup is byte-identical to today. With any set, a wrapper `span.player-avatar-decorated` (position relative, size = avatar size) holds the avatar plus one `span.player-avatar-art` (absolute inset 0, `pointer-events: none`, `aria-hidden`) containing accessory, hat, frame in that order. Badges keep the existing row.
**Consumes:** `FRAME_ART` (Task 3); hat/accessory/badge art registries are looked up through `HAT_ART`, `ACCESSORY_ART`, `BADGE_ART` exported from `components/avatar/*` (Tasks 5-6; until they exist, import them as empty records so this task compiles, and Tasks 5-6 fill them).

- [ ] **Step 1: Failing tests.** `TestNoDecorIsByteIdentical`, `TestLayerOrder` (accessory before hat before frame in the DOM), `TestUnknownIdsRenderNothing`, `TestFoldedAndAllInSeatKeepStateStyling` (seat with frame still carries the folded/all-in class hooks), `Seat` render-budget test unchanged.
- [ ] **Step 2:** Run; expect FAIL. **Step 3:** Implement; seat CSS offsets scale with `--seat-size` (like `.seat-note-trigger`); the wrapper must not change the avatar's hit target geometry (`.seat-avatar-trigger` ::before/::after stay on the avatar).
- [ ] **Step 4:** Expect PASS. Commit `feat(ui): PlayerAvatar renders equipped cosmetics`.

### Task 5: Hat and accessory art

**Files:** Create `ui/src/components/avatar/hats.tsx`, `accessories.tsx` and tests.
**Interfaces:** Produces `HAT_ART`, `ACCESSORY_ART` with the same contract as `FRAME_ART`. Hats occupy y 0-26 and overlap the avatar top; accessories y 24-40 (mustache y 40-48).

- [ ] **Step 1: Failing tests:** `TestEveryHatAndAccessoryIdHasArt`, `TestArtContract` (same assertions as frames, plus hats stay within y 0-26 and accessories within their band by checking the SVG `viewBox` and a declared `data-band`).
- [ ] **Step 2:** Run; expect FAIL. **Step 3:** Draw 6 hats and 5 accessories; same screenshot-at-4-sizes proof. **Step 4:** Expect PASS. Commit `feat(ui): avatar hat and accessory art`.

### Task 6: Badges

**Files:** Create `ui/src/components/avatar/badges.tsx`, test; modify `lib/avatarCosmetics.ts` only for glyph mapping.
**Interfaces:** Produces `BADGE_ART: Record<string, LucideIcon>` with the glyphs from the Item set table; every badge keeps the existing 20px gold chip, tone by `--cosmetic-*` token per id; `title` from `avatarItemLabel('badge', id)`.

- [ ] **Step 1: Failing tests:** `TestEveryBadgeIdHasGlyph`, `TestBadgeChipUsesRegistryLabel`. **Step 2:** Run; expect FAIL. **Step 3:** Implement and wire into `PlayerAvatar`. **Step 4:** Expect PASS. Commit `feat(ui): avatar badge glyphs`.

### Task 7: Wire the data (seat, leaderboard, people, hands)

**Files:** Modify `ui/src/lib/api/table.ts` (`SeatView`) and the place that converts the proto `Seat` to `SeatView` (grep `playstyle_badge` outside `proto/`; read it first), `ui/src/components/table/Seat.tsx` (~:389-391), `ui/src/lib/api/gamification.ts` (`Entry`), `ui/src/app/(app)/leaderboard/page.tsx` (`RankingRow`, podium), `ui/src/lib/api/player.ts`, `social.ts`, `PeopleList.tsx`, `FriendCodeLookup.tsx`, `hands/history/page.tsx`, `app/(app)/profile/page.tsx`, `ProfileMenu.tsx`; Tests beside each plus `ui/e2e/tableLayout.spec.ts` seed with a decorated seat.
**Interfaces:** `SeatView`/`Entry`/people/showcase types gain `equipped_frame_id?`, `equipped_hat_id?`, `equipped_accessory_id?`, `equipped_badge_ids?` (names match the phase 1 wire exactly; proto Seat uses `frame_id`, `hat_id`, `accessory_id`, `badge_ids`). Leaderboard rows render `PlayerAvatar` (28px row, larger on podium) using `avatar_url` plus decoration; the 5-minute `LEADERBOARD_STALE_MS` and the two-GET budget are unchanged.

- [ ] **Step 1: Failing tests:** proto Seat to `SeatView` maps the four fields (absent -> undefined); `Seat` forwards them; leaderboard row shows the avatar and decoration; `tableRenderBudget` unchanged; mock runtime (`src/dev/mockRuntime.ts`) seeds one decorated seat.
- [ ] **Step 2:** Run; expect FAIL. **Step 3:** Implement; memoise `badgeIds` per seat. **Step 4:** Run vitest plus `npm run e2e` (layout gate, 3 browsers, 6 viewports) against a decorated seat; expect PASS. Commit `feat(ui): equipped cosmetics at the seat, leaderboard and people`.

### Task 8: Profile equip UI

**Files:** Modify `ui/src/app/(app)/player-profile/AvatarDecorationSection.tsx` and test, `ui/src/lib/api/player.ts` (`equipped_hat_id`, `equipped_accessory_id` on `PlayerProfile` and `updateMe`).
**Interfaces:** Pickers for frame (radiogroup), hat (radiogroup), accessory (radiogroup), badges (checkboxes, max 3); each lists only owned ids from `listOwnedAvatarCosmetics(slot)` plus a "Nenhum" option; live preview at 72px and at seat size (36px) using the same `PlayerAvatar`; Save calls `updateMe` with the changed fields and `setQueryData(PLAYER_ME_KEY, profile)`; errors from the API show in the existing error pattern.

- [ ] **Step 1: Failing tests:** owned-only listing, "Nenhum" unequips each slot, 4th badge blocked, preview reflects the pending selection, save sends only changed fields.
- [ ] **Step 2:** Run; expect FAIL. **Step 3:** Implement. **Step 4:** Expect PASS. Commit `feat(ui): equip hats and accessories in the profile`.

### Task 9: Visual QA, docs

**Files:** Modify `ui/CLAUDE.md` (avatar section), `docs/README.md`, the Guide page for store/profile; no new product code unless QA finds a defect.
- [ ] **Step 1:** Using the dev mock (`npm run dev:mock`) and a real browser, one batched pass at desktop and 390px: a seat ring with 9 decorated seats, a folded seat, an all-in seat, the profile preview, the leaderboard podium; screenshots to the report; fix everything found in one batch. **Step 2:** Document the 64x64 contract, layer order, registry, no-`outline` rule and the "unknown id renders nothing" behaviour. **Step 3:** Run `npm run lint`, `npx tsc --noEmit` on touched files (the repo baseline already has unrelated `params` errors), `npm test`, `npm run e2e`. Commit `docs(ui): avatar cosmetics rendering`.

## Self-review

- Spec coverage (phase 2): SVG set (T3, T5, T6), PlayerAvatar and seat (T4, T7), profile preview and pickers (T8), leaderboard rows (T7), tokens and DESIGN.md (T2), layout gate and render budget (T4, T7, T9), earnable catalog ids (T1). Purchasable catalog entries, SKUs, store sections and grants stay in phase 3.
- Names consistent: slots `frame|hat|accessory|badge`, props `frameId|hatId|accessoryId|badgeIds`, wire `equipped_*` (REST) and `frame_id|hat_id|accessory_id|badge_ids` (proto).
