# Paid Rabbit Hunt — Design

## Summary

Rabbit Hunt (peeking at the runout that would have come after a hand ends without showdown) is currently free and, more
importantly, not actually gated: `TableSnapshot` already carries `RunoutCards` /
`ShuffleServerSeedHex` / `RevealedCardSalts` / `UnrevealedCardHashes` for every connected viewer as soon as
`stage == Complete` and
`WonWithoutShowdown` is true (`api/internal/engine/hand/snapshot.go:349-353`).
`RabbitHunt.tsx`'s "Ver o que viria" button only decides whether to *render*
data the client already has — anyone can read it straight out of the WebSocket frame before clicking. There is no
protocol point at which charging a fee would do anything, since the secret is already on the wire.

This spec withholds those fields from a viewer until that viewer has paid, reusing the per-viewer snapshot fan-out
(`Table.ViewFor`) that already masks hole cards — no new masking mechanism, just a new condition on an existing one.

## Scope

Sandbox (chips) tables only, same reasoning as
`docs/specs/2026-08-10-auto-buyin-design.md`: real money is off by default and a per-hand real-money debit for a
curiosity feature is a separate product/legal decision, not bundled here.

**Correction from an earlier draft of this spec:** Rabbit Hunt is *not*
already absent from real-money tables today — `RabbitHunt.tsx`/`TableStage.tsx`
have no `currency_mode` awareness at all, so on a real-money table (dormant today behind `REAL_MONEY_ENABLED`, but not
something this feature may assume stays dormant forever) the existing free version would already leak the runout, and a
naive paid version would debit real stack chips outside any wallet transaction. `hand.Table` has no currency-mode field
to check today either — `ConfigureRake(currencyMode)` (`hand.go:291-297`) only derives
`rakeBPS` from the mode and discards it. This spec adds a `currencyMode
string` field to `Table`, set the same place `rakeBPS` is (`ConfigureRake`), persisted in `State` the same way `RakeBPS`
is. `RequestRabbitHunt` rejects with `currencyMode != "sandbox"` before anything else — real money is closed at the
engine layer, not assumed closed by UI absence.

## Price

Fixed at the table's big blind, matching the user's original ask. No dynamic pricing — one seat, one price, easy to
reason about, no separate config surface to build or explain in the UI.

## Data model

New state on `Table` (`api/internal/engine/hand/hand.go`):

- `currencyMode string` — set once by `ConfigureRake` (same call site as
  `rakeBPS`), persisted across the table's whole lifetime (not reset per hand, unlike the field below). The sandbox-only
  gate for the whole feature.
- `rabbitHuntPaid map[string]bool` — `playerID -> paid this hand`, reset every hand alongside `rakeCollected`
  (`hand.go:692`). Persisted the same way `SeenActionIDs` is (`state.go`'s `State` struct, no `dynamodbav` tags needed —
  this package's `State` has none), so a paid flag survives an actor restart mid-hand instead of only living in memory.

No fee-collected counter: nothing reads it (unlike rake, which the board displays), so it isn't built — a plain YAGNI
cut, not a deferred TODO. In sandbox mode a collected fee simply leaves the stack, same observable behavior a counter
would have had with zero consumers to show it to.

## Wire changes

No new protobuf fields. The existing flat `ClientMessage` envelope (`proto/poker.proto:178-200`) already carries
`action_id` for idempotency/ack tracking and needs no new payload — the server knows the hand, the board, and the
requester from `PlayerID` and current table state.

Two new `type` values, added to the comment list at `poker.proto:179` and mirrored in
`ui/src/lib/api/proto/poker.ts:243`:

- `"request_rabbit_hunt"` — pay-and-reveal request.
- `"rabbit_hunt_verify_failed"` — client-side verification failure, triggers a refund.

## Server flow

### `RequestRabbitHuntCmd`

New command struct in `api/internal/table/commands.go`, same shape as
`ShowCardsCmd` (`commands.go:103-118`) minus `CardIndex`:
`RequestRabbitHuntCmd{PlayerID, ActionID, Reply chan error}`.

Dispatched in `actor.go` to a new `handleRequestRabbitHunt`, modeled on
`handleShowCards` (`actor.go:1367-1408`):

1. `ensureLoaded`, then `retryOnConflict(apply)`.
2. Inside `apply`, validate against the current hand:
    - `currencyMode == "sandbox"` (real-money tables never reach the charge or the reveal, regardless of UI state)
    - `stage == Complete`
    - `WonWithoutShowdown`
    - `len(board) < 5`
    - the requesting player was `dealt_in` this hand
    - `!rabbitHuntPaid[playerID]` (no double charge)
    - `player.Stack >= bigBlind`
    - Any failure returns a plain error over `Reply` — nothing is charged, nothing is revealed. The client shows an
      inline error; no snapshot changes.
3. On success: `player.Stack -= bigBlind`, `rabbitHuntPaid[playerID] = true`.
4. `a.commit(ctx, ActionID, &tablestore.ActionLogEntry{...})` — the same conditional-write idempotency path every other
   command uses, so a retried `request_rabbit_hunt` (client retry after a dropped ack) is a no-op rather than a second
   charge.
5. `a.broadcastAll()`.

### The reveal gate

`snapshot.go:349` changes from:

```go
if t.stage == Complete {
proof, runout := t.fairnessProofFor(viewerID, wonWithoutShowdown)
...
}
```

to:

```go
if t.stage == Complete && (!wonWithoutShowdown || t.rabbitHuntPaid[viewerID]) {
proof, runout := t.fairnessProofFor(viewerID, wonWithoutShowdown)
...
}
```

Genuine showdown (`!wonWithoutShowdown`) is never gated — that proof is owed to every player unconditionally; only the
rabbit-hunt case (`wonWithoutShowdown == true`) requires payment. `fairnessProofFor` itself is untouched; the gate lives
entirely in its one caller, per
`api/CLAUDE.md`'s rule that visibility rules belong in `ViewFor`/the snapshot builder, never in a handler.

Because `broadcastAll()` re-derives every connected viewer's snapshot via
`ViewFor`, paying viewers get the reveal fields on the very next broadcast and everyone else keeps seeing them masked —
no new fan-out mechanism, this is the same primitive that already keeps hole cards viewer-specific.

### Refund on verification failure

New command `RabbitHuntVerifyFailedCmd{PlayerID, ActionID}`, handled the same way: if `rabbitHuntPaid[playerID]` is true
for the current hand, credit `bigBlind` back to `player.Stack` and clear `rabbitHuntPaid[playerID]`. Clearing the flag
is what re-masks the reveal fields for that viewer on the next broadcast — consistent with "didn't get verifiable data,
didn't pay."
Idempotent via the same `ActionLogEntry` commit path (a retried failure report can't double-refund).

This command trusts the client's claim of verification failure. That's an acceptable trust boundary here: the worst case
is a player claiming a false failure to get their BB back after already reading the (real, valid) runout client-side — a
curiosity fee refund, not a security hole. Cost of building server-side re-verification to close that gap isn't
justified by what's at stake (one BB).

## Frontend

`RabbitHunt.tsx`:

- The button's label becomes `Ver por {fmt(bigBlind)} fichas` instead of the flat "Ver o que viria".
- `onClick` calls a new `requestRabbitHunt()` on `useTableRealtime`
  (mirrors `showCards()`: emits `{type: 'request_rabbit_hunt', action_id}`, tracks a pending lock cleared on ack or
  `action_timeout`, same as
  `showCardsLockRef`/`showCardsTimerRef`).
- The existing verification `useEffect` (lines 19-50) is otherwise unchanged — it already reacts to
  `snapshot.runout_cards` /
  `snapshot.shuffle_server_seed_hex` appearing. The only behavior change is *when* those fields are non-empty:
  previously always, now only after this viewer's payment lands in a subsequent snapshot.
- On verification failure (the existing `catch` branch, line 42-44), also call a new `reportRabbitHuntVerifyFailed()`
  (same shape as
  `requestRabbitHunt()`, different `type`), and change the failure message from "Não foi possível verificar o runout."
  to "Não foi possível verificar o runout. Taxa devolvida."
- A rejected `request_rabbit_hunt` (insufficient stack, already paid, hand no longer eligible) needs no bespoke UI in
  `RabbitHunt.tsx`: it surfaces through the same `useTableRealtime` auxiliary-command error path every other rejected
  command already uses (`finishAuxiliaryCommand`'s failed branch sets `actionError`, rendered by the existing
  `ActionBar` error slot in `page.tsx`) — identical to how a rejected `show_cards` is handled today, with no dedicated
  per-component error state or retry-vs-disable logic invented for this one command.

## Testing

Backend (`api/internal/table/actor_test.go`, mirroring
`TestShowCardsCmdRevealsFoldedWinnerToEveryone`):

- Paying reveals `RunoutCards`/`ShuffleServerSeedHex` to the payer only — a second connected viewer's `ViewFor` snapshot
  still masks them.
- Double `request_rabbit_hunt` from the same player charges once (second call rejected as already-paid, not
  double-debited).
- Insufficient stack rejects the request and leaves `rabbitHuntPaid`
  unset — no partial charge.
- `rabbit_hunt_verify_failed` refunds the stack and re-masks the viewer's next snapshot.
- Genuine showdown (`!WonWithoutShowdown`) is never gated regardless of
  `rabbitHuntPaid` state — existing fairness-proof tests must keep passing unchanged.
- Actor restart mid-hand: replayed `request_rabbit_hunt` via the
  `ActionLogEntry` path doesn't re-charge (idempotency).

Frontend (`RabbitHunt.test.tsx` or wherever the current component's tests live):

- Button shows the big-blind price.
- Click emits `request_rabbit_hunt`; reveal only renders once the snapshot carries non-empty `runout_cards`/
  `shuffle_server_seed_hex` for this viewer.
- Verification failure triggers `reportRabbitHuntVerifyFailed` and shows the refund message.
- A rejected request never renders cards (no reveal fields ever arrive in the snapshot) — the rejection message itself
  is `useTableRealtime`'s concern, covered in that hook's own tests, not duplicated here.

## Out of scope

- Real-money tables (see Scope).
- Dynamic/variable pricing.
- Server-side re-verification of a claimed verification failure.
- Any UI for where collected rabbit-hunt fees go (same as rake today — tracked as a counter, not yet surfaced anywhere
  player-facing).
