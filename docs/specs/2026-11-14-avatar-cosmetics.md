# Avatar cosmetics: borders, hats, accessories, badges

Status: approved; phase 1 implemented, phases 2-3 pending. Date: 2026-11-14. Builds on issue #292 (seasonal frames and badges).

## Intent

Players show off at the table. An avatar can wear a border, a hat, an accessory and up to 3 badges, and
other players see them at the seat, in profiles and in the leaderboard. Items come from achievements, from
purchases (fichas or PIX) or from seasons. The model must allow seasonal drops later without new code paths.

## Goals

- Four slots: `frame` (border), `hat`, `accessory`, `badge` (max 3, already exists).
- Three sources per item: `achievement`, `purchase`, `season`.
- Equipped ids visible on the table seat, `/profile` showcase, people lists and the leaderboard (avatar + decoration).
- ~24 launch items drawn as inline SVG in the CTech identity (tokens, no new hex values).
- Seasonal windows: an item can be sold or granted only between two instants; ownership is permanent.

## Non-goals

- Hats that overflow the avatar circle (see Constraints).
- A generic `equipped: map[slot]id` or migrating frame/badge off their current columns (approach B, rejected).
- Outfit loadouts for avatar items (`cosmeticloadout` stays deck/felt only).
- English copy. The app is pt-BR only; item names live in a client label map like `avatarFrameLabel` today.
- Trading, gifting, resale, any path that turns an item into chips or money.
- Animated items in the first release (reduced-motion and WebKit pending-animation rules make this a later, separate decision).

## Anti-goals (failures even if it technically works)

- A seat render that costs more than today: `Seat` is memoised and counted in `tableRenderBudget.test.tsx`.
- Layout-gate regressions in Chromium/Firefox/WebKit (clipping, horizontal scroll).
- A purchased item that is granted twice, or granted for an invoice that was not paid in full.
- Equipping an item the player does not own, or one outside its window for non-owners.

## What exists today (verified in code)

- `cosmetics/catalog.go`: static `catalog []CatalogEntry{Kind, ID, Premium, PriceFichas, SKU}`; kinds `deck`, `felt`, `frame`, `badge`.
  Frames `season-2026-q3`, `champion-gold`; badges `season-2026-q3-top10`, `season-2026-q3-champion`. No price or SKU on them.
- Ownership: `poker_cosmetic_entitlements` (pk=player, sk=`kind#itemID`); `cosmeticpurchase.Service.Grant` is idempotent. Only a test calls it.
- Equipped state: `EquippedFrameID`, `EquippedBadgeIDs` on the player (`player/model.go`); validated by `SetEquippedFrame`/`SetEquippedBadges`, written by PATCH profile.
- Prices for PIX live in `catalog/catalog.go` (`products`); Billing receives only description and amount. Selling an item by PIX needs only a `products` entry.
  `purchasegw` re-reads the invoice and checks it was paid at the catalogue price before anything is granted.
- Purchase routes `/wallet/cosmetic-purchase/:kind` accept only `deck`/`felt`; `validFrameOrBadgeKind` guards the owned-read route.
- Seat identity pushed to the table is only `{Name, AvatarURL, PlaystyleBadge}` (`tableidentity.go`, `table.SetIdentityCmd`); proto `Seat` last field is 21.
- Leaderboard `Entry` has no avatar; `resolveProfiles` (`api/v1/leaderboard.go`) already BatchGets profiles per page.
- UI: `PlayerAvatar` accepts `frameId`/`badgeIds`; only own profile passes them. Decoration CSS lives in `(app)/app.css`, seats in `renderer.css`.
- Achievements: `RecordHand` returns `[]TierUnlock{PlayerID, Key, Stars}`; the only consumer is the unlock loop in `app/handpipeline.go`.

## Approaches considered

- **A. Extend the catalog (chosen).** `Source` and the sale window on `CatalogEntry` (Kind is the slot); one profile field per new slot; ids on seat, profile and leaderboard payloads.
  Trade-off: one field per slot scales poorly, but slots are rare and items are many.
- **B. Generic `equipped` map, migrate frame/badge.** Cleaner, but a data and proto migration for no user-visible gain now.
- **C. Copy frame for `hat` and `accessory` only.** Smallest diff; leaves achievement grants, purchase and seasons unanswered.

## Design

### Catalog (`cosmetics`)

`CatalogEntry` gains `Source`, `From`/`Until` (`time.Time`, zero = open). `Kind` stays the storage key prefix
(`frame`, `hat`, `accessory`, `badge`); for avatar items Kind equals Slot. New kinds: `KindHat`, `KindAccessory`.
`Source=purchase` requires a `SKU` and `PriceFichas`; `Source=achievement|season` forbids a price. A catalog test pins this.
Window rules: `Until` closes new purchases and new grants; `From` opens them. Already-owned items keep working forever.
Phase 1 enforces the window in `Grant` only (`ErrNotAvailable`); purchase-window enforcement lands in phase 3 with the purchasable kinds.
Time source is the injected clock (never `time.Now` in services).

### Obtaining

- **Purchase:** add the item's SKU to `catalog.products` (price in cents) and a `PriceFichas` in the cosmetics catalog; widen `validCosmeticKind`
  in the purchase routes to the new purchasable kinds. `ListCatalog`, `CreateSandbox` (fichas) and `CreateReal` (PIX via `purchasegw`) are reused unchanged.
  Purchases outside the window return a RFC 7807 problem with a stable `code`.
- **Achievement:** an explicit table `{achievement key, stars} -> item` in `cosmetics`, applied in the `handpipeline` unlock loop by calling
  `Grant(player, kind, id, "achievement:<key>:<stars>")`. `handpipeline` receives the cosmetics service as a small interface. Grant failure is logged
  and retried by the next unlock event only for new unlocks; a missed grant must be recoverable: add a reconcile-style backfill (see Open for builder).
  The grant also emits the existing WS unlock frame extended with the item, so the toast can say what was earned.
- **Season:** same `Grant`, triggered by the season job that closes a period (leaderboard month/quarter results). Reason `season:<id>:<rank-band>`.
  The season trigger is out of scope for phase 1-3 unless the builder finds the leaderboard period close hook already present; items and windows are in scope.

### Equip

- Profile gains `equipped_hat_id`, `equipped_accessory_id` beside the existing frame/badge fields; PATCH profile accepts them; `SetEquipped*` follow
  `SetEquippedFrame` exactly (IsKnown, then `requireCosmetic`, fail closed). Refund-while-equipped guard is extended to the new kinds
  (`app/app.go` `isCurrentSelection` currently returns "" for frame/badge, a latent bug fixed here for all four).
- Equipping pushes identity to every table the player sits at, through the same `tableIdentityPusher` rename already uses.

### Wire and snapshots

- Proto `Seat` adds `frame_id=22`, `hat_id=23`, `accessory_id=24`, repeated `badge_ids=25` (ids only, optional). Regenerate Go, TS and CLI clients.
- `hand.Player` and `SeatView` carry the four fields (dynamodbav, snapshot.go), `SetIdentityCmd`/`SetPlayerIdentityForActor` extend, and the
  other writers of identity (`actor_loading.go`, `buyin/service.go`) pass them. Ids only; never ownership reads per snapshot.
- Public showcase, people/social and hands payloads add the same ids where they already return `avatar_url`.
- Old clients ignore unknown proto fields; new clients treat missing ids as "none". No flag needed.

### Leaderboard

`leaderboard.Entry` response adds `avatar_url`, `equipped_frame_id`, `equipped_hat_id`, `equipped_accessory_id`, `equipped_badge_ids`, filled in
`resolveProfiles` from the profiles it already batch-loads (not stored in the leaderboard row, so equipping never rewrites rankings).
UI: `RankingRow` and the podium render `PlayerAvatar` with the decoration. The 5-minute `LEADERBOARD_STALE_MS` caching stays; request count stays at two GETs.

### Rendering (ui)

- One `AvatarCosmetic` module replaces the hardcoded id lists in `lib/avatarCosmetics.ts`: id -> `{slot, label, Svg}`. Labels pt-BR.
- `PlayerAvatar` takes `frameId`, `hatId`, `accessoryId`, `badgeIds` (primitives only, so `Seat` memoisation holds) and layers: image, accessory, hat, frame.
- Frame uses a pseudo-element ring (not `outline`) so folded outline and all-in `box-shadow` keep working. Hat and accessory SVGs are drawn inside a box of
  `--seat-size`; offsets scale with `--seat-size` like `.seat-note-trigger`. Seat-scope CSS goes in `renderer.css`; profile-only CSS stays in `app.css`.
- Named colour tokens in `globals.css` `:root` and `DESIGN.md`; no raw hex/px in component CSS. SVG is inline React (no network, no layout shift).
- Launch set (names are working titles): 6 frames, 6 hats, 5 accessories, 7 badges. About half earnable, half purchasable, with a few seasonal.
- Store: new sections in `CosmeticStoreSection` for the purchasable kinds; `CosmeticKind` widens. Profile: `AvatarDecorationSection` gets hat and accessory pickers
  and a live preview at seat size and profile size.

### Resilience, scale, cost (CTech pillars)

- Purchases keep today's idempotency (`Idempotency-Key`, reservation, `purchasegw` paid-in-full check). No new external dependency.
- Equip validation reads DynamoDB once per equip (low frequency); table paths carry stored ids only. Ownership reads for the owned list are bounded by catalogue size (<100 items).
- Seat payload grows by four short strings; no new DynamoDB table, no new AWS resource, no cost note beyond a few bytes per snapshot. Leaderboard adds no reads.
- Fails closed on equip (nil ownership checker rejects); fails open on render (unknown id renders no decoration).

## Phases (one PR each)

1. **Backend and wire:** catalog fields, new kinds, window enforcement, profile fields, equip validation, refund guard, proto + snapshots + identity push, leaderboard payload.
2. **Art and rendering:** SVG set, `PlayerAvatar`, seat, profile preview, leaderboard rows, tokens, `DESIGN.md`, layout-gate and render-budget tests, real handset pass.
3. **Obtaining:** purchasable kinds in store (fichas + PIX), `products` entries, achievement grant table and WS toast, backfill, season trigger if the hook exists.

## Tests

- Catalog invariants (source vs price/SKU, unique ids, window ordering), window enforcement with a fake clock, grant idempotency.
- Equip validation and refund-while-equipped for all four kinds.
- Proto round trip and snapshot persistence of the new seat fields; leaderboard payload includes decoration.
- UI: `PlayerAvatar` unit tests, `AvatarDecorationSection`, store section, `tableRenderBudget.test.tsx` unchanged count, `npm run e2e` layout gate on 3 browsers.
- Zero lint warnings, `go test -race`, `scripts/standards-check.sh`.

## Constraints

- Seat avatar is 24 to 52px depending on breakpoint (36px trigger); everything must read at that size.
- Narrowest-stylesheet rule, no animated layout properties, `prefers-reduced-motion`, no entrance animation that makes content visible (WebKit).
- Every change documented: this spec, `api/CLAUDE.md`, `ui/CLAUDE.md`, `docs/README.md` index, and the Guide page for the store/profile.
- Family check: nothing here is reusable outside poker except a possible shared avatar-frame renderer in `ctech-ui` later; not proposed now.

## My calls (not yet confirmed by you)

- Slot names, the 24-item count and its split, and the pt-BR names.
- Indicative price bands (builder sets exact values next to existing deck/felt prices: deck 200 to 500 cents, felt 1000): frames 300 to 1000, hats 200 to 500, accessories 200 to 500 cents; badges are never sold.
- Season grants are out of scope unless a period-close hook already exists.
- No animation in the first release.

## Open for the builder

- Exact achievement-to-item table (read `achievements/catalog.go`; give common tiers badges, rare tiers frames/hats).
- Backfill mechanism for missed achievement grants (reconcile Lambda vs on-login sweep); pick the one the repo already uses for pending work.
- Whether `Seat` needs a packed single string field instead of four to keep snapshot size flat.
