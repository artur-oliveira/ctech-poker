# Table Highlights Feed — Design

## Summary

A new, automatic, system-detected "biggest pot of the day" highlight per table — no player action
required. This is distinct from the existing `api/internal/handshare` flow (`POST
/players/me/hand/:id/share`), which is manual and player-initiated (the player picks `brag`/`bad_beat`,
opts into revealing their own hole cards, gets a shareable link). This feature is the opposite
trigger: the server itself decides a hand was notable and records it, with no owner and no manual
opt-in step.

New backend package only, one new endpoint, one small UI surface. No production code from
`handshare` is reused directly (see "Why not reuse handshare" below) — the underlying pattern
(conditional-write record-keeping, `HandOutcome`-driven hooks) is the same one already used
throughout `internal/app/app.go`'s `onHandComplete`.

## Where

- New package: `api/internal/highlights` (`store.go`, `store_test.go`), same shape as
  `internal/handshare` and `internal/pokerstats` (a `Store` wrapping `dynamo.Base`, no service
  layer needed — YAGNI, this is a single conditional write plus a single read).
- New DynamoDB table: `poker_table_highlights`. PK `table_id`, SK `date` (`YYYY-MM-DD`, UTC) — one
  item per table per day, holding whatever hand currently holds that day's biggest pot at that
  table. Overwritten in place as bigger pots come in during the day; naturally rolls over at UTC
  midnight since the SK changes. No TTL needed — a 30-day table-item table costs nothing to keep,
  but if churn matters later a 90-day TTL is a one-line addition, not now.
- Hook wiring: `api/internal/app/app.go`'s `onHandComplete` closure (`app.go:419`), the exact same
  place `persistHandHistory`, `achv.RecordHand`, `leaderboardSvc.RecordHand`,
  `pokerStatsStore.RecordHand`, and `recentSvc.RecordHand` already fire from — all driven by the
  same `outcome hand.HandOutcome` value. Add one more call: `highlightsStore.RecordHand(ctx,
  tableID, outcome)`.
- New endpoint: `GET /rooms/:id/highlights/today` in `internal/api/v1` (new
  `highlights.go`, registered in `router.go` next to `handshares.go`'s registration), behind the
  existing `authMiddleware`.
- UI: one small banner/card in the live table view (`ui/src/app/table/page.tsx`), fetched once on
  mount via a `['highlights', tableId, 'today']` TanStack Query key, refreshed the same way
  `RealityCheck`'s parent already refreshes on hand completion (`s.hand_id` change) rather than
  polling.

## Data captured and "notable" threshold

Ponytail rung: one criterion, one comparison, no scoring engine. "Notable" = **strictly bigger than
the current day's recorded pot at that table.** No stakes-relative percentile, no bad-beat
detection, no multi-factor score — those are all real ideas but not needed for a first version, and
each one adds a dimension of tuning that has no data behind it yet.

`hand.HandOutcome` (`api/internal/engine/hand/hand.go:161`) already carries everything needed at the
exact moment a hand completes:

- **Pot amount**: `sum(outcome.Payouts)` — `Payouts map[string]int64` is already net-of-rake, so the
  highlight shows the same number the players actually saw won, not a raw contribution total.
- **Board**: `outcome.Board` (added in the 2026-07-24 `HandOutcome.Board`/`PlayerHands` work — see
  `[[ctech-poker-snapshot-and-history-fixes]]`), used as-is; empty when the hand never reached a
  board (pre-flop all-fold).
- **Revealed hands**: iterate `outcome.PlayerHands` (`map[string]PlayerHandInfo`), keep only entries
  where `Revealed == true` — the exact same flag `sessionlog.OpponentSummary.HoleCards` and
  `handshare`'s `anonymizedOpponents` already gate on. **This is the one invariant that must not be
  re-derived**: a highlight must never show a folded/mucked hand that wasn't voluntarily shown, so
  the store only ever copies `PlayerHandInfo` where `Revealed` is already true — never a raw
  `HoleCards` field, never a client-side visibility decision.
- **Names**: same `names map[string]string` already passed into `onHandComplete` for the other
  hooks (`app.go:419`'s second parameter) — no new lookup needed.

`highlights.Store.RecordHand` conditional-write shape (mirrors the "correctness = DynamoDB
conditional writes" rule in `api/CLAUDE.md`, same idea `tablestore.CommitAction` uses for table
state):

```go
func (s *Store) RecordHand(ctx context.Context, tableID string, outcome hand.HandOutcome, names map[string]string) error {
    pot := int64(0)
    for _, amount := range outcome.Payouts {
        pot += amount
    }
    if pot <= 0 {
        return nil // no chips changed hands (e.g. a walkover) — nothing to highlight
    }
    item := Highlight{
        TableID: tableID, Date: time.Now().UTC().Format("2006-01-02"),
        HandID: outcome.HandID, Pot: pot, Board: outcome.Board,
        Revealed: revealedHandsOf(outcome, names), RecordedAt: time.Now().UnixMilli(),
    }
    // ConditionExpression: only overwrite if this table+day has no highlight yet,
    // or the new pot beats the one on record — same "update only if better"
    // pattern a leaderboard Top-N write would use.
    return s.base.PutItemIf(ctx, item, "attribute_not_exists(pot) OR pot < :pot",
        map[string]any{":pot": pot})
}
```

(`PutItemIf` — check `gopkg.aoctech.app/api-commons/dynamo`'s `dynamo.Base` for whatever the actual
conditional-put helper is named at implementation time; every other package in this repo already
does a conditional write through `dynamo.Base`, so this is reusing an existing helper, not adding
one.)

## Why not reuse `handshare` directly

`handshare.Share` requires a non-empty `OwnerID` (`Store.Create` returns `ErrNotOwner` otherwise,
`store.go:103-105`) because its whole model is "one player owns and can revoke their own share." A
table highlight has no single owner — every player who was at the table should be able to see it,
and nobody should be able to revoke a factual "this table had a $X pot today" record. Forcing an
owner onto it (e.g. the pot's winner) would be modeling a fact that isn't true and would let one
player delete a highlight the *other* participants also appeared in. A separate, ownerless store is
the smaller, more honest model — not a missed reuse opportunity.

## Access control

`GET /rooms/:id/highlights/today` is scoped to players who actually had a session at that table:
reuse the same lookup `RealityCheck`'s data source already needs — `sessionlog.Store`'s per-player
session records (`poker_player_sessions`) — checking the requesting player has a `SessionItem` with
that `TableID` (open or closed, any time). This keeps the same privacy boundary the rest of the
match-history surface already uses (`table/CLAUDE.md`'s hidden-information rules): a highlight is
visible to people who were actually there, not the whole lobby. A stranger who never sat at the
table gets 404, same shape as `handshares.go`'s `public` handler returning `NotFound` for an unknown
token.

## Testing

- `highlights/store_test.go` (unit, no DynamoDB): `revealedHandsOf` only copies entries where
  `Revealed == true` — a table-driven test with a folded participant asserts their `HoleCards` never
  appear in the built `Highlight`.
- `highlights/store_integration_test.go` (DynamoDB Local, `//go:build integration`): a second
  `RecordHand` call with a smaller pot for the same table+day does not overwrite the first (asserts
  the conditional expression); a larger pot does overwrite.
- `internal/api/v1/highlights_test.go`: 404 for a player with no session at that table; 200 with the
  stored highlight for one who has; 404 when no highlight exists yet for today.
- UI: a small `TodayHighlight.test.tsx` covering the empty state (nothing rendered) and the
  populated state (pot amount + revealed cards, if any, rendered) — following the same
  `RealityCheck.test.tsx` mocking pattern for `Dialog`/query hooks already established.

## Out of scope

- Any cross-table or lobby-wide "highlight of the site" feed — scoping this to one table one day
  keeps the access-control question trivial (session-at-that-table) instead of needing a public
  visibility model.
- Bad-beat detection, hand-category rarity, or any multi-factor "highlight score" — pot-size record
  only, per the Ponytail rung above; revisit only if this MVP ships and feels too narrow.
- Real-time push the instant a new highlight is set (e.g. a toast to everyone still seated) — the
  UI fetches on hand completion already, which is good enough for "today's biggest pot so far"; a
  live push through the existing invalidation-only lobby/table socket pattern
  (`useLobbyRealtime.ts`'s `social_event`-style messages) is a natural follow-up, not required for
  v1.
- A public, shareable link for a highlight (the way `handshare` produces one) — nothing here
  prevents adding a "share this highlight" button later that hands the same data to
  `handshare.Store.Create` with the viewing player as `OwnerID`, but that's a second, opt-in feature
  layered on top, not part of this one.
