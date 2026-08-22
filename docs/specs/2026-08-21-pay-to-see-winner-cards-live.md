# Pay To See Winner's Cards (Live) — Design

## Summary

When a hand ends without showdown (everyone else folded), the winner's hole cards stay hidden unless they voluntarily
reveal them (the existing free `show_cards`/`RevealHoleCard` mechanic,
`api/internal/table/actor.go:1367-1408`). This spec adds a second, paid path: any *other* seated player at the table can
pay a fixed fee to see that winner's hole cards, without the winner's cooperation. This is a sibling feature to Paid
Rabbit Hunt (`docs/specs/2026-08-21-paid-rabbit-hunt.md`) — same trigger condition (`wonWithoutShowdown`), same fixed-BB
pricing, same per-viewer reveal mechanism — but it unmasks the winner's actual
`SeatView.HoleCards` rather than the post-hand runout/seed proof, and unlike Rabbit Hunt it needs no client-side
cryptographic re-verification (the winner's hole cards are trusted the same way any opponent's revealed cards are
trusted today — `ui/CLAUDE.md`: "the client's job is to display what it got, never to reconstruct what it didn't"), so
there is no refund-on-verification-failure path.

## Scope

Sandbox (chips) tables only, live hands only. Same reasoning as
`docs/specs/2026-08-21-paid-rabbit-hunt.md`'s Scope section: real money is a separate product/legal decision, and this
spec reuses the exact same `currencyMode` field that spec adds to `hand.Table`
(set once by `ConfigureRake`) — `RequestWinnerCards` gates on `currencyMode == "sandbox"` the same way
`RequestRabbitHunt` does.

**Hand history / replay (seeing a past hand's mucked winner cards after the fact) is explicitly out of scope here** —
see `docs/specs/2026-08-21-pay-to-see-winner-cards-history.md`, which depends on this spec shipping first (its per-hand
archive is written by the same post-hand hook this spec's fee-split logic lives next to, and its payment/reveal model is
the one this spec establishes, applied to archived hands instead of the live table).

## Price and split

Fixed at the table's big blind, matching Rabbit Hunt. Unlike Rabbit Hunt (where the fee simply leaves the stack as
implicit rake), this fee splits: half credited to the winner's stack, half added to
`rakeCollected`. Integer division (`fee / 2`) goes to the winner; the remainder (`fee - fee/2`) goes to rake, so the two
halves always sum to exactly `fee` regardless of an odd big blind — no chip is created or destroyed.

## Data model

New state on `Table` (`api/internal/engine/hand/hand.go`), alongside Rabbit Hunt's fields:

- `winnerCardsPaid map[string]bool` — `viewerID -> paid to see the winner's cards this hand`, reset every hand alongside
  `rakeCollected`/`rabbitHuntPaid` (`hand.go:692`). Persisted in `State` the same way `rabbitHuntPaid` is (no
  `dynamodbav` tags needed, this package's `State` has none).

No new field is needed to identify "the winner" — `wonWithoutShowdown == true` implies exactly one player was left
uncontested (everyone else folded), so `t.lastOutcome.Winners[0]` is always the single target. `RequestWinnerCards`
returns an error if `len(t.lastOutcome.Winners) != 1` as a defensive check, but this should be unreachable given the
`wonWithoutShowdown` gate.

## Wire changes

One new `type` value, added to the comment list at `poker.proto:179` and mirrored in
`ui/src/lib/api/proto/poker.ts:243`: `"request_winner_cards"`.

No new protobuf fields — same flat `ClientMessage` envelope as every other auxiliary command.

## Server flow

### `RequestWinnerCardsCmd`

New command struct in `api/internal/table/commands.go`, same shape as `RequestRabbitHuntCmd`:
`RequestWinnerCardsCmd{PlayerID, ActionID, Reply chan error}`.

Dispatched to a new `handleRequestWinnerCards`, modeled on `handleRequestRabbitHunt`:

1. `ensureLoaded`, then `retryOnConflict(apply)`.
2. Inside `apply`, validate:
    - `currencyMode == "sandbox"`
    - `stage == Complete`
    - `WonWithoutShowdown`
    - `len(t.lastOutcome.Winners) == 1` (defensive; see Data model)
    - the winner is `winner := t.lastOutcome.Winners[0]`; reject if `c.PlayerID == winner` (can't pay to see your own
      cards)
    - the winner has **not** voluntarily shown (`!winnerPlayer.VoluntarilyShown &&
      !winnerPlayer.VoluntarilyShownCards[0] && !winnerPlayer.VoluntarilyShownCards[1]`) — if the winner already showed
      for free, reject with a plain error (nothing to sell; the client should not have shown this option, but the server
      is the actual gate)
    - the requesting player was dealt in this hand (`dealtIn[c.PlayerID]`, mirrors Rabbit Hunt's requirement)
    - `!winnerCardsPaid[c.PlayerID]` (no double charge)
    - `requester.Stack >= bigBlind`
3. On success: `requester.Stack -= bigBlind`; `winnerPlayer.Stack += bigBlind / 2`; `t.rakeCollected +=
   bigBlind - bigBlind/2`; `winnerCardsPaid[c.PlayerID] = true`.
4. `a.commit(ctx, ActionID, &tablestore.ActionLogEntry{...})` — same conditional-write idempotency path, so a retried
   `request_winner_cards` after a dropped ack is a no-op, not a second charge.
5. `a.broadcastAll()`.

No refund path: there is no client-side verification step to fail. If the winner's identity or hole cards were somehow
wrong, that would be an engine bug, not a "verification failed, refund"
scenario — out of scope the same way a `show_cards` bug would be.

### The reveal gate

`snapshot.go`'s seat-building loop (`snapshot.go:237-310`) computes `publicReveal` per seat from
`p.VoluntarilyShown`/`p.VoluntarilyShownCards`/`revealAll` (global, same for every viewer's snapshot). This spec adds
one more per-viewer condition, evaluated once per `ViewFor(viewerID)` call (so it only ever affects the payer's own
snapshot, never anyone else's):

```go
paidWinnerReveal := p.ID == winnerID && t.winnerCardsPaid[viewerID]
```

(`winnerID` is `t.lastOutcome.Winners[0]` when `t.stage == Complete && t.lastOutcome.WonWithoutShowdown`, empty string
otherwise — computed once at the top of the seat loop, mirroring how `wonWithoutShowdown`
is already computed once at `snapshot.go:210`.)

Every place that currently reads `publicReveal[i]` to decide whether to show a card or compute
`HandCategory` for a non-viewer seat (`snapshot.go:265`, `268`, `277`, `300`, `303`) changes from
`publicReveal[i]` to `publicReveal[i] || paidWinnerReveal`. `sv.HoleCardsRevealed` (line 263) gets the same
`|| paidWinnerReveal` — this is safe because `HoleCardsRevealed` is already computed fresh inside each viewer's own
`ViewFor` call, not a single shared value; setting it true only in the payer's own snapshot instance leaks nothing to
any other viewer's separately-computed snapshot.

Because `broadcastAll()` re-derives every connected viewer's snapshot via `ViewFor`, the payer sees the winner's real
cards on the very next broadcast; every other viewer's own `ViewFor(theirID)` still computes `paidWinnerReveal = false`
(since `t.winnerCardsPaid[theirID]` is unset) and keeps seeing
`"back"` — no new fan-out mechanism, same primitive Rabbit Hunt and hole-card masking already share.

## Frontend

New component `ui/src/components/table/WinnerCards.tsx`, sibling to `RabbitHunt.tsx` but simpler (no verification
effect, no refund path):

- Rendered under the same condition as `RabbitHunt`'s availability check
  (`snapshot.stage === 'complete' && snapshot.won_without_showdown`), plus: the winner (`snapshot.winners[0]`) has not
  already voluntarily shown (checked via that seat's
  `hole_cards_revealed`/`hole_cards` already being non-`"back"` in the viewer's own snapshot — if already revealed, this
  component renders nothing, the cards are just visible normally) **and** the viewer is not the winner themselves.
- Button label: `Ver a mão de {winnerName} por {fmt(bigBlind)} fichas`.
- `onClick` calls a new `requestWinnerCards()` on `useTableRealtime` (mirrors `requestRabbitHunt()`:
  emits `{type: 'request_winner_cards', action_id}`, tracks a pending lock cleared on ack or
  `action_timeout`).
- No local verification/refund UI: once the request succeeds, the winner's seat's `hole_cards` in the next snapshot are
  the real cards instead of `"back"` — the existing `Seat.tsx` rendering path already displays whatever `hole_cards`
  it's given, no new rendering logic needed there.
- A rejected `request_winner_cards` (insufficient stack, already paid, winner already showed, hand no longer eligible)
  surfaces through the existing generic `useTableRealtime` auxiliary-command error path (`finishAuxiliaryCommand` →
  `actionError` → `ActionBar`'s existing error slot) — identical precedent to Rabbit Hunt and `show_cards`, no bespoke
  error UI.
- Wired into `TableStage.tsx`/`page.tsx` the same way `RabbitHunt` is (both render sites: base layout and
  `.stage-v-ring` vertical layout).

## Testing

Backend (`api/internal/table/actor_test.go`, mirroring the Rabbit Hunt integration tests):

- Paying reveals the winner's `HoleCards`/`HoleCardsRevealed` to the payer only — a second connected viewer's `ViewFor`
  snapshot still shows `"back"` for that seat.
- Fee splits correctly: winner's stack increases by `bigBlind/2`, `RakeCollected()` increases by
  `bigBlind - bigBlind/2`, payer's stack decreases by exactly `bigBlind`.
- Rejects if the winner already voluntarily showed (`VoluntarilyShown` true) — no charge.
- Rejects if the requester is the winner themselves.
- Double `request_winner_cards` from the same player charges once.
- Insufficient stack rejects the request, no partial charge.
- Genuine showdown (`!WonWithoutShowdown`) is not a valid target — `RequestWinnerCards` rejects with a plain error
  (cards are already public via `revealAll`/showdown reveal in that case, nothing to sell).
- Real-money table (`currencyMode != "sandbox"`) rejects unconditionally.

Frontend (`WinnerCards.test.tsx`):

- Button shows the winner's name and the big-blind price.
- Not rendered when the viewer is the winner, or when the winner already voluntarily showed.
- Click emits `request_winner_cards`; the winner's seat cards render once the snapshot's `hole_cards`
  for that seat are no longer `"back"`.
- A rejected request never renders the cards (no reveal ever arrives in the snapshot) — rejection messaging is
  `useTableRealtime`'s concern, covered there, not duplicated here.

## Out of scope

- Hand history / replay reveal — `docs/specs/2026-08-21-pay-to-see-winner-cards-history.md`.
- Real-money tables.
- Dynamic/variable pricing.
- Any UI for where the rake-half of the fee goes (same as rake today — tracked as a counter, not surfaced anywhere
  player-facing beyond what already exists for rake).
