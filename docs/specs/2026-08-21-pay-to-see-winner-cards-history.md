# Pay To See Winner's Cards (History) — Design

## Dependency

**`docs/specs/2026-08-21-pay-to-see-winner-cards-live.md` is a hard prerequisite for this spec and must ship first.**
This spec extends that one's payment/reveal model (fixed BB fee, per-viewer paid reveal, `currencyMode == "sandbox"`
gate) to hands that already ended and were archived, instead of a hand currently live at the table. The new per-hand
archive this spec introduces is written by the same post-hand hook the live spec's fee/reveal logic sits next to
(`tablemanager`'s hand-complete hook,
`api/CLAUDE.md`'s "post-hand hooks" convention) — implementing history first, without the live feature existing, would
mean designing that write path against a payment model that doesn't exist yet. Do not implement this spec out of order.

## Summary

A player who won a hand without showdown and never voluntarily showed can be paid-revealed live (see the live spec).
Once that hand ends and the table moves on, today there is **no way at all** to buy that same reveal later from hand
history — worse, there is no way to build one safely without changing storage, because of how history is currently
written.

## The security constraint (why this needs a new store, not a new read gate)

Today, a completed hand's outcome (`hand.HandOutcome`, `api/internal/engine/hand/hand.go:161-204`) is written into **one
durable record per participant**, not one record per hand:
`sessionlog.HandItem` (`api/internal/sessionlog/store.go:50-77`, table `poker_player_hands`, partition key `player_id`).
Each player's own item lists their opponents via `OpponentSummary`
(`store.go:90-99`), and `OpponentSummary.HoleCards` is populated **at write time** only when
`hand.PlayerHandInfo.Revealed` was true for that opponent (genuine showdown or voluntary show) — the comment on that
field is explicit: *"HoleCards is only populated when that opponent's hand was actually shown to the table... never a
folded hand."* An opponent's mucked cards are never written into another player's `HandItem` in the first place — there
is nothing in that row to "unlock" later.

This absence is load-bearing elsewhere: `POST /players/me/hand/:id/share` → `GET /hand-shares/:token`
(`api/internal/api/v1/handshares.go:137-148`) copies `opponent.HoleCards` from that same `HandItem`
straight into a **public, unauthenticated** share link with no further check at all — it relies entirely on "if it's in
this row, it was legitimately shown" as its only security boundary.

Two ways to enable a paid history reveal, and only one is safe:

1. **Widen the write path** to always store every opponent's true `HoleCards` in every player's
   `HandItem`, gating only the *read*. Rejected: this durably puts every mucked hand's true cards into a per-player
   table that a completely unrelated feature (`hand-shares`) already treats as pre-filtered and exposes with zero
   additional gate. One missed check anywhere downstream turns into every fold ever played becoming leakable.
2. **Add a new, separate, per-hand (not per-player) archive**, written once by the same post-hand hook that already
   writes each participant's `HandItem`, holding the hand's true `PlayerHandInfo` map regardless of `Revealed`. This new
   table is read by exactly one thing: the new paid-reveal endpoint below, which checks payment before returning one
   player's cards for one hand to one buyer. Nothing else ever reads it. `sessionlog.HandItem` and `hand-shares` are
   untouched — their existing write-time redaction stays their only (still sufficient) guarantee.

This spec implements **option 2**, per explicit decision.

## Scope

Sandbox tables only (a hand played on a real-money table never reaches this path — same
`currencyMode` gate as the live spec, checked at write time: the archive is only written for hands where
`t.currencyMode == "sandbox"`, so a real-money hand's cards are never written to it regardless of future gating
decisions on the read side). Fixed BB price, same split as the live spec (half to the original winner's wallet... see
Payout timing below for why this needs a wrinkle the live spec didn't).

## Data model

New DynamoDB table `poker_hand_reveals` (new package `api/internal/handreveal`, mirroring
`handshare`'s package shape):

```go
type HandRecord struct {
    PK           string                    `dynamodbav:"pk"` // table_id#hand_id
    TableID      string                    `dynamodbav:"table_id"`
    HandID       string                    `dynamodbav:"hand_id"`
    BigBlind     int64                     `dynamodbav:"big_blind"`
    WinnerID     string                    `dynamodbav:"winner_id"`
    WinnerShown  bool                      `dynamodbav:"winner_shown"` // VoluntarilyShown at hand end
    PlayerHands  map[string]PlayerHandCode `dynamodbav:"player_hands"` // playerID -> their 2 cards
    EndedAt      int64                     `dynamodbav:"ended_at"`
    TTL          int64                     `dynamodbav:"ttl,omitempty"` // matches poker_player_hands retention
}

type PlayerHandCode struct {
    Cards [2]string `dynamodbav:"cards"`
}
```

Written once, at the same point `tablemanager`'s hand-complete hook already iterates `t.handOrder` to build each
player's `sessionlog.HandItem` — one extra `PutItem` per hand (not per player), guarded by
`outcome.WonWithoutShowdown` (a genuine showdown's cards are already public forever via the existing
`Revealed`-gated `OpponentSummary`, nothing to sell). Skipped entirely when `currencyMode != "sandbox"`.

`poker_hand_reveals` (paid to reveal, `viewerID -> paid`) reuses the shape of the live spec's
`winnerCardsPaid`, but scoped per archived hand instead of per live table — a second small table,
`poker_hand_reveal_payments`, keyed `hand_id#viewer_id`, written by the new endpoint below on successful payment. Kept
separate from `HandRecord` (which is written once, by the hand-complete hook, and never touched again) so a payment
write is never racing the archive write.

## Payout timing (the wrinkle live-spec payment doesn't have)

The live spec credits the winner's **stack** directly, mid-table, atomically with the charge (both are in-memory `Table`
mutations committed in the same conditional write). By the time someone buys a history reveal, that table may be long
gone — the winner has since cashed out, left, or the table itself was torn down. There is no live stack to credit.

The payer's half still comes from their **wallet-linked sandbox balance** (not a live table stack) via the existing
sandbox economy path (`internal/buyin`'s ledger, sandbox mode has no real-money constraints — sandbox chips are already
a soft, non-withdrawable balance per `api/CLAUDE.md`'s currency_mode notes). The winner's half must be credited to their
**player balance**, not a table stack — the same balance `GET /players/me`'s `sandbox_balance` reads. This is a new
small money path:
`handreveal.Store.PayForReveal(ctx, buyerID, winnerID, handID, fee)` debits the buyer's sandbox balance, credits half to
the winner's sandbox balance, and records the payment — all inside one DynamoDB transaction (`TransactWriteItems`),
mirroring the existing sandbox-balance adjustment pattern used elsewhere for daily rewards (`internal/dailyreward`)
rather than inventing a new one. The remaining half (rake) is simply not credited to anything, same as the live spec's
rake half.

## API

New authenticated endpoint, alongside the existing hand-history routes (`api/internal/api/v1/handhistory.go`):

`POST /players/me/hands/:handId/reveal-winner` — body: none needed (`playerID` from the JWT `sub`, IDOR-safe per
`api/CLAUDE.md`'s identity rule).

1. Load the `HandRecord` for `handId` (`handreveal.Store.Get`). 404 if absent (hand had a showdown, was real-money, or
   predates this feature).
2. Reject if `record.WinnerShown` (nothing to sell) or `c.PlayerID == record.WinnerID` (can't buy your own hand).
3. Reject if the caller wasn't a participant of that hand — cross-check against the caller's own
   `sessionlog.HandItem` for that `hand_id` (confirms they were dealt in; also the only place that already ties a player
   to a hand ID today).
4. Reject if already paid (`handreveal.Store.HasPaid(handId, playerID)`).
5. `PayForReveal` (see above). On success, record the payment.
6. Return `{"cards": record.PlayerHands[record.WinnerID].Cards}`.

A second endpoint, `GET /players/me/hands/:handId/reveal-winner`, returns the same payload if
`HasPaid` is already true (so a page refresh doesn't require re-buying) — 404 otherwise. This mirrors the live spec's
snapshot-driven reveal: once paid, the cards are visible on demand, not just in the one-shot purchase response.

## Frontend

The existing hand-history / replay view (`ui/src/app/hands/history`, `ui/src/app/hands/replay`) gains the same
`WinnerCards`-shaped button used live, adapted to call the new REST endpoints instead of a WS command (this view is not
a live table connection): on load, call the `GET` endpoint to check for an existing purchase; if absent and the hand
qualifies (`won_without_showdown` from the stored outcome, winner not shown, viewer not the winner), show the buy
button; `onClick` calls `POST`, then renders the returned cards in place of `"back"` for that seat in the replay UI.

## Testing

Backend:

- `handreveal` package: `HandRecord` write happens only for `WonWithoutShowdown` sandbox hands; a showdown hand or a
  real-money hand produces no `poker_hand_reveals` item.
- `PayForReveal` transaction: buyer's sandbox balance decreases by the full fee, winner's increases by exactly half
  (integer split, same rounding rule as the live spec), a double call is rejected (idempotent per `hand_id#viewer_id`).
- `POST .../reveal-winner` integration test: 404 for a non-existent/ineligible hand, 403-equivalent (plain rejection)
  for the winner buying their own hand, success path returns the correct two cards.
- `GET .../reveal-winner` returns the cards after a successful `POST`, 404 before.

Frontend:

- Buy button renders only when the stored hand qualifies and hasn't been bought yet.
- Successful purchase renders the winner's cards in the replay in place of `"back"`.

## Out of scope

- Real-money hands (see Scope; also structurally impossible since the archive is never written for them).
- Any change to `sessionlog.HandItem`, `hand-shares`, or their existing write-time redaction — this spec adds a wholly
  separate store rather than touching either.
- Retroactively backfilling `poker_hand_reveals` for hands that completed before this feature shipped — those hands'
  true cards were never durably kept anywhere outside the per-viewer redacted
  `sessionlog.HandItem`, so there is nothing to backfill from.
