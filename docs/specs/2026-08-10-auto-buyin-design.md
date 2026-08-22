# Auto Buy-In (Auto Rebuy) — Design

## Summary

Let a player opt into "auto rebuy" when they join a table. If they bust (stack hits
zero), the server automatically buys them back in for the same amount as their
original buy-in, so they don't have to wait or manually rebuy to keep playing the
next hand. If they don't have enough sandbox chips to cover the auto-rebuy amount,
they're left sitting out; if their balance is genuinely zero, the client offers a
simplified in-place PIX chip purchase instead of a dead-end error.

## Scope

**Sandbox (chips) rooms only.** Real-money rooms are explicitly out of scope for
this feature:

- Real-money buy-ins/rebuys charge `room.EntryFeeCents` on every call
  (`buyin/service.go:230-238`, `296-304`) — a known, currently-manual-only cost. Wiring
  auto-rebuy into real money would silently and repeatedly re-charge a player's real,
  withdrawable wallet with no chance to opt out per-attempt. That's a separate, already
  tracked problem (`../plans/2026-08-21-entry-fee-entitlement.md`) and not something
  this feature should compound.
- `REAL_MONEY_ENABLED` is off by default; the UI is explicitly built assuming real
  money is off (`ui/CLAUDE.md`).

The auto-rebuy hook only acts when `room.CurrencyMode == "sandbox"`. The `AutoRebuy`
flag can still be stored for a real-money seat (harmless, unused) — no need to
special-case rejection at the API layer.

## Data model

Two new fields on `hand.Player` (`api/internal/engine/hand/hand.go`), following the
existing `RunItTwice bool` precedent (per-seat, `dynamodbav`-tagged, persisted for
free through the existing `tablestore.Store.CommitAction` conditional-write path —
survives actor restarts, lease hand-off, rolling deploys):

- `AutoRebuy bool` `dynamodbav:"auto_rebuy,omitempty"`
- `BuyInAmount int64` `dynamodbav:"buy_in_amount,omitempty"`

Both are set **once**, at seat creation (`AddWaitingPlayer` / `AddMidHandJoiner` in
`hand.go`, when a brand-new `Player` is constructed for a joining user) — never
touched again by `rebuyExisting` or any later manual/automatic rebuy. This means:

- The auto-rebuy amount is always the player's **original** buy-in for that sitting,
  even if they later manually rebuy for a different amount.
- Toggling auto-rebuy requires leaving and rejoining the table — there's no
  in-session toggle. (No new `ClientMessage`/table command needed for this feature.)

## API changes

`POST /rooms/:id/join` request body gains an optional `auto_rebuy: bool` field
(default `false`). Threaded through:

`roomHandlers.join` → `buyin.Service.BuyIn` → `table.JoinCmd` → `AddWaitingPlayer` /
`AddMidHandJoiner`, which set `AutoRebuy` and `BuyInAmount` on the newly created
`Player` only. If the player already has a seat (this is a rebuy, not a fresh join),
the flag is ignored — it cannot be changed after the initial join.

## Trigger: post-hand auto-rebuy sweep

Hooked into the existing `onHandComplete` closure in `app/app.go` (already runs
achievements/leaderboard/stats bookkeeping after every completed hand).

**Critical constraint:** `onHandComplete` runs synchronously on the table actor's own
single-goroutine command loop (`table/actor.go`: `broadcastAll` → `notifyHandComplete`
→ `onHandComplete`, all inline, before the actor loop reads its next command).
`buyin.Service.BuyIn` calls `actor.Dispatch(table.JoinCmd{...})` and blocks waiting
for a reply from that same loop. Calling `BuyIn` directly from inside
`onHandComplete` is a guaranteed self-deadlock: the actor can never service the new
command because the only goroutine that services it is the one currently blocked
inside `onHandComplete`. This would freeze the entire table (every player's actions,
snapshots, disconnects) indefinitely.

**Auto-rebuy attempts therefore run in a detached goroutine**, spawned from inside
`onHandComplete`, never blocking the actor's own call stack — the same pattern the
codebase already uses for `SettleSystemRemoval`/`onPlayerRemoved` to avoid re-entering
the actor from a synchronous hook.

Algorithm, for each hand participant:

1. Read the seat's post-hand state (`buyin.Service.Seated`, the existing
   reconnect/GET-seated primitive).
2. If `Stack == 0 && AutoRebuy && room.CurrencyMode == "sandbox"`:
   - `go func() { ... }()`:
     - Check `wallet.Balances(ctx, playerID).SandboxBalance >= BuyInAmount`.
       - If balance `< BuyInAmount` (including balance `== 0`): do nothing further.
         The player stays sitting out. (The `buyin.Service.walletMover` interface
         needs `Balances` added — `walletclient.Client` already implements it, it's
         just not exposed on that interface today.)
       - If balance is sufficient: call
         `buyin.Service.BuyIn(ctx, roomID, playerID, BuyInAmount, false, nonce)`
         with `nonce = handID + "-auto-" + playerID` (defensively idempotent, though
         `notifyHandComplete`'s once-per-hand guard already prevents duplicate firing).
       - If that `BuyIn` call fails for any other reason (transient wallet error,
         race, etc.): no retry. Player stays sitting out — same outcome as
         insufficient balance, surfaced identically to the client.

No special handling is needed for "did the rebuy succeed" from the actor's side:
`BuyIn`'s existing `JoinCmd` → `rebuyExisting` path already flips the seat's
eligibility for the next hand the same way a manual rebuy does today.

## Wire changes

`Seat` proto (`proto/poker.proto`) gains `optional bool auto_rebuy` — same pattern as
the existing `run_it_twice` field. This lets the client distinguish "this bust will
likely self-resolve" from "this is a normal manual-rebuy bust" without polling or
guessing.

## Frontend

**`BuyInPanel.tsx`** — adds an "Auto rebuy" checkbox, default unchecked. Passes
`auto_rebuy` through to `joinRoom`.

**`RebuyDialog.tsx`** — currently opens unconditionally whenever
`viewerSeat.stack === 0 && sitting_out` (`page.tsx:429-431`), showing a manual
buy-in slider that hits the same `joinRoom`/`BuyIn` path.

New behavior, gated on the seat's `auto_rebuy` flag:

- `auto_rebuy === false`: unchanged — today's manual rebuy dialog.
- `auto_rebuy === true`: because the auto-rebuy attempt runs in a detached
  goroutine after the client has already received the bust snapshot, there's a
  short window where the outcome isn't known yet. To avoid flashing a scary
  "you're out of chips" dialog right before the auto-rebuy silently succeeds:
  1. Show a lightweight "Auto-rebuy in progress…" state for a short grace window
     (~1.5s — generous for one wallet-service HTTP round trip).
     `ponytail:` this is a fixed-timeout heuristic, not an explicit
     success/failure event from the server — good enough for a single local
     wallet call; if wallet latency ever becomes unpredictable enough to make this
     flaky, replace with an explicit "rebuy_failed" push instead of guessing a timeout.
  2. If the seat's `stack` becomes `> 0` before the window elapses, the dialog's
     render condition (`stack === 0`) goes false on its own and it unmounts —
     no explicit "close" logic needed.
  3. If `stack` is still `0` after the grace window, decide between two states
     using the player's already-known sandbox balance (already available
     client-side, e.g. via the existing wallet balance display):
     - Balance `=== 0`: embed the existing PIX purchase flow
       (`PurchaseModal.tsx`'s sandbox-pack PIX flow) directly in the dialog,
       instead of the manual buy-in slider, so the player can top up without
       leaving the table.
     - Balance `> 0` but insufficient: fall back to today's normal manual rebuy
       dialog (no PIX offer) — the player has chips, just not enough for a full
       auto-rebuy; per product decision, PIX is offered only when the player is
       truly at zero.

## Out of scope / explicitly deferred

- Real-money rooms (see Scope).
- In-session toggling of auto-rebuy after joining.
- Partial auto-rebuy using whatever balance is available when it's insufficient
  for the full amount (player sits out and decides manually instead).
- A distinct "why is this seat sitting out" wire signal beyond the timeout
  heuristic described above.
