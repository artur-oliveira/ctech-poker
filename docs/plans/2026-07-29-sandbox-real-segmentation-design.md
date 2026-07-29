# Sandbox/Real Segmentation for Achievements, Leaderboard, Poker-Stats — Design

**Date:** 2026-07-29
**Status:** Approved design, pending implementation plan.

## Problem

`player.WalletMode` (the Profile sandbox/real toggle) and `roomstore.Room.CurrencyMode` already exist and already
segregate money (see `api/CLAUDE.md` — the `currency_mode` boundary is load-bearing in `buyin`). But gamification
data does not carry this boundary at all: `achievements.Service.RecordHand`, `leaderboard.Service.RecordHand`, and
`pokerstats.Store.RecordHand` — all invoked from `onHandComplete` in `api/internal/app/app.go:295-323` — take no
mode parameter. A hand played at a sandbox table and a hand played at a real-money table bump the exact same
DynamoDB item for a given player. Result: achievements, leaderboard rank, and VPIP/PFR/3-bet stats are one blended
pool today, with no way to tell a player "this achievement only counts real-money hands."

## Non-goals

- No new achievements (the 40+ item brainstorm list) — tracked separately, out of scope for this plan.
- No change to `player.WalletMode`, `roomstore.Room.CurrencyMode`, or the wallet/buyin money paths — those already
  work correctly.
- No new DynamoDB table or GSI.

## Approach

Prefix the existing sort keys (and the leaderboard GSI partition value) with the mode, instead of adding a new
key attribute. `dynamo.QueryOpts.SKPrefix` (in `api-commons`, already vendored) makes prefix-filtered queries free —
no CDK change, no table/GSI redefinition, no new infra to provision or roll back.

| Store | Key today | Key after |
|---|---|---|
| `achievements.Store` (PK=`playerID`) | SK = `<achievement_key>` (e.g. `wins`) | SK = `<mode>#<achievement_key>` (e.g. `real#wins`) |
| `leaderboard.Store` (PK=`playerID`) | SK = `stats`, GSI pk attrs = `"all"` | SK = `stats#<mode>`, GSI pk attrs = `"<mode>"` (same GSIs, now naturally partitioned by mode instead of one shared `"all"` bucket) |
| `pokerstats.Store` (PK=`stats#<playerID>`, no SK) | PK = `stats#<playerID>` | PK = `stats#<mode>#<playerID>` |

The hand-completion guard row in `pokerstats.Store.RecordHand` (`guard#<tableID>#<handID>`) does not need a mode —
a hand belongs to exactly one table, which belongs to exactly one mode, so the guard key is already unambiguous.

### Why prefix, not a new key attribute

The user's original proposal was a dedicated extra PK/SK (DynamoDB supports composite keys on the base table and
per-GSI). Functionally equivalent, but it means redefining key schemas on tables that already have production data —
a live schema migration. Prefixing achieves the same segmentation as an application-level convention change: same
table, same GSI, same CDK stack, zero downtime risk. Recommended.

## Propagation

`onHandComplete` (`api/internal/app/app.go`) already closes over `rooms *roomstore.Store` (used today by the
sibling `roomLoader` closure). Add one `rooms.Get(ctx, tableID)` call at the top of `onHandComplete`, extract
`room.CurrencyMode`, and thread it as a new parameter through:

- `achievements.Service.RecordHand(ctx, tableID, mode, outcome)`
- `leaderboard.Service.RecordHand(ctx, mode, outcome, names)` and `RecordUnlocks(ctx, mode, unlocks)`
- `pokerstats.Store.RecordHand(ctx, mode, tableID, handID, metrics)`

`achievements.Store.Increment`, `leaderboard.Store.IncrementStats`/`IncrementAchievementPoints`, and
`pokerstats.Store.Get` all gain a `mode string` parameter that becomes the SK/PK prefix. `TierCrossed` (pure
function over catalog + counts) is unaffected — tiers are still evaluated against whichever mode's counter changed.

## Migration

Every progress row that exists today predates real-money mode (real-money entry-fee launched 2026-07-25; this plan
lands after) — so every existing row is implicitly a sandbox row. DynamoDB cannot rename a key in place, so a
one-shot batch job is required before the new code paths go live:

1. Scan `poker_achievement_progress`, `poker_leaderboard_stats`, `poker_player_poker_stats`.
2. For each item, `PutItem` a copy under the new `sandbox#`-prefixed key (and, for `leaderboard`, the
   `gsi_hands_won_pk`/`gsi_hands_played_pk`/`gsi_win_rate_pk` attributes rewritten from `"all"` to `"sandbox"`),
   then `DeleteItem` the old key.
3. Run once, before deploying the code that starts writing prefixed keys. No dual-read/fallback path is needed in
   application code afterward — this is a finite, one-time cutover, not an ongoing compatibility shim.

## API surface

`GET /players/me/achievements`, `GET /leaderboard`, `GET /players/me/poker-stats` each gain `?mode=sandbox|real`.
Missing/invalid `mode` defaults to `sandbox` (matches today's only-sandbox-existed behavior, so old UI builds and
any cached links keep working unchanged). `mode=real` must be explicit.

## UI

`achievements`, `leaderboard`, and player-stats pages get a sandbox/real tab or toggle (mirroring the existing
`currency_mode` toggle already used in `CreateRoomDialog`/lobby). Real-money achievement tiers get copy along the
lines of "só conta jogando com dinheiro real" per the user's original ask — exact copy is a UI-task detail, not a
design decision.

## Testing

- `achievements`, `leaderboard`, `pokerstats` package unit tests: assert a sandbox-mode hand and a real-mode hand for
  the same player land in different items/counters (mirrors the existing `TestBuyInSkipsFeeForSandboxRooms`-style
  mode-isolation assertions already used in the buyin package).
- `app.go` integration point: assert `onHandComplete` calls each service with the room's actual `CurrencyMode`, not
  a hardcoded value.
- Migration script: dry-run mode that reports counts without writing, run against DynamoDB Local first.
