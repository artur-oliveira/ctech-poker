# Reveal timing grace + all-in runout pacing

**Goal:** two engine/frontend timing changes so board-card reveals feel paced and give players enough time to react,
instead of the current instant/simultaneous reveal:

1. Whenever a new street (flop/turn/river) is dealt, the next-to-act player's action timer gets a small grace period on
   top of the normal `turn_timeout_seconds` so the reveal animation has time to finish before the countdown visually
   starts pressuring the player.
2. When an all-in is accepted before all board cards are out (i.e. betting is over but community cards remain to be
   dealt), the engine reveals the remaining streets one at a time with a real delay between them — instead of dealing
   flop+turn+river synchronously in one broadcast like today.

A third item was raised (should achievements/made-hand stats count hands that ended by early fold, for the hand the
winner *could* have made with cards that were never revealed?) — investigation confirmed current behavior already does
the right thing and needs no change; see
[Confirmed: no change needed](#confirmed-no-change-needed-achievements).

## Current behavior (as read from code)

- `api/internal/table/turntimeout.go:6` — `DefaultTurnTimeout = 15 * time.Second`, overridable per room via
  `turn_timeout_seconds` (`TurnTimeoutFor`, lines 9-15).
- `api/internal/table/actor.go` — `armTurnTimer` (~line 661) arms the per-turn deadline via
  `time.AfterFunc`; the deadline is stamped into the snapshot as `ActionDeadlineUnixMs` (line 743).
  `armNextHandTimer` (~line 687) arms `NextHandDelay = 5 * time.Second`
  (`turntimeout.go:18`) after a hand reaches `Complete`, stamped as `NextHandUnixMs` (line 745). Every `Act` call
  results in exactly one `broadcastAll()` after commit (line 324).
- `api/internal/engine/hand/hand.go`:
    - `advanceStage()` (lines 723-766) deals the next street: PreFlop→Flop (+3 cards), Flop→Turn (+1), Turn→River (+1),
      River→showdown.
    - The all-in / can't-act-anymore case is lines 745-749: when `canStillAct <= 1` (at most one non-all-in player left
      with a decision), it calls `runoutBoard()` then immediately
      `runShowdown()` in the same synchronous call — no delay, no intermediate broadcast.
    - `runoutBoard()` (lines 772-788) is a tight loop dealing every remaining street back-to-back with zero pacing.
- Frontend:
    - `ui/src/components/table/Board.tsx` passes `index < 3 ? index : 0` to `PlayingCard` — only the 3 flop cards get a
      stagger index (0/1/2); turn/river cards always get `index=0`.
    - `ui/src/app/globals.css:1525-1527` — `.board-card .card-reveal-inner` animation is `560ms`
      with `animation-delay: calc(var(--deal-index, 0) * 220ms)`. Flop's 3 cards finish revealing
      ~1000ms after appearing (`440ms` last delay + `560ms` animation); a single turn/river card finishes at `560ms`.
    - Because the all-in runout sends one broadcast with the full final board, and only the first 3 cards get a stagger
      index, a runout today visually looks like up to 5 cards appearing at once (flop staggers, turn/river don't).

## Section A: reveal grace period

**Behavior:** every time `advanceStage()` transitions to a new street (Flop, Turn, or River) and a new betting round
starts, the first action-timer arm for that round gets **+1.5s** added on top of the normal `turn_timeout_seconds`
deadline. Any subsequent action within the same street (if betting continues among multiple players) uses the normal
timeout with no extra grace — only the first to-act player after a reveal needs the animation to finish before their
clock visibly matters.

**Why 1.5s:** covers the flop's worst case (~1000ms for the 3rd card's reveal animation to finish)
with headroom for network/render jitter, and is applied uniformly to flop/turn/river for consistency even though
turn/river's own animation is shorter (a single card is done at 560ms).

**Where:** the grace is applied at the point in `actor.go` where `armTurnTimer` is called immediately following a stage
transition (as opposed to being called for every `Act`) — the extra duration is added to the deadline computation only
on that call, not baked into
`DefaultTurnTimeout`/`TurnTimeoutFor` themselves (those stay the room-configurable base value).

## Section B: all-in runout pacing

**Behavior:** when `advanceStage()` detects the existing all-in condition (`canStillAct <= 1 &&
remaining > 1`, lines 745-749), it no longer calls `runoutBoard()` (deal-everything-at-once). It deals **one street at a
time**, using the same real-timer pattern the engine already uses for
`armTurnTimer`/`armNextHandTimer`:

1. Deal the next missing street now (e.g. if at PreFlop, deal the flop) and let the normal
   `broadcastAll()` (already fired once per `Act`, `actor.go:324`) carry it to clients exactly like a normal street
   transition — no new wire field needed, the frontend's existing flop/turn/river-arrival handling and reveal animation
   (`Board.tsx`, `globals.css`) already do the right thing for a board array that grows over time.
2. If a street still remains after that, arm a new timer (same `time.AfterFunc` pattern as
   `armNextHandTimer`) for a fixed **2 seconds**. When it fires: reacquire the table lock, deal the next street, commit,
   broadcast, and re-arm if a street is still missing.
3. Once the river is dealt and its 2s elapse, call `runShowdown()` and broadcast the final result. Only **after** that
   does the engine arm the normal `NextHandDelay` (5s) countdown — this ordering is what prevents the next-hand
   countdown from racing the runout reveal: the backend owns the full timeline (runout pacing → showdown → next-hand
   delay) end to end, and the frontend only ever renders state it receives, it never fakes timing on its own.

**Why not a frontend-only fake delay:** rejected — the frontend has no way to know when the backend's own
`NextHandDelay` clock starts, so a client-side animation delay could run past (or race) the real next-hand countdown,
which is backend-authoritative. Pacing the reveal in the engine itself means there's only ever one clock.

**Frontend detail (independent of the 2s backend pacing):** the river card's own flip animation duration should be
subtly slower than the turn's, in `globals.css`/`PlayingCard.tsx`. Applies whenever the river is revealed — during a
normal hand or during an all-in runout alike — so the frontend needs no runout-specific state to know when to use the
slower timing, it only needs to know "this is the river card."

**Scope note:** if the all-in happens after the flop (e.g. only turn+river remain), only those two streets get the paced
2s-apart treatment — no re-revealing the flop. If it happens after the turn (only river remains), there's nothing to
pace: a single missing street reveals immediately via the existing `advanceStage` path, same as normal play, since
Section A's grace period already covers the single-card case adequately (there's no meaningful "runout" with one
street).

## Confirmed: no change needed (achievements)

Investigated `api/internal/engine/hand/hand.go` `runShowdown()` (lines 800-916) and
`api/internal/achievements/service.go` `RecordHand` (lines 25-61):

- `wonWithoutShowdown := nonFolded == 1` (hand.go:807) is true for any hand that ends by opponents folding, at any
  street.
- `outcome.WinningCategory` is only ever set `if !wonWithoutShowdown` (hand.go:905-907) — i.e. only on a genuine
  showdown where hands are actually compared with a complete board.
- `RecordHand` only bumps a category-based achievement (`KeyWinByCategory`, `service.go:49-53`) when
  `outcome.WinningCategory != ""`.

So a hand won by early fold — regardless of how strong the winner's cards *would have been* once the board finished —
never contributes to a made-hand achievement (pair/trips/straight/etc.), matching the desired behavior ("se não houve
aposta [até o showdown], aquilo não ocorreu" for achievement purposes). No code change requested for this item.

## Testing

- Section A: table-level test asserting `ActionDeadlineUnixMs` includes the +1.5s grace immediately after a stage
  transition, and does not for a same-street follow-up action.
- Section B: hand/table-level test driving a preflop all-in to confirm the board is dealt in three separate broadcasts
  (3/4/5 cards) roughly 2s apart (using a fake clock/injectable timer, not a real sleep), and that `NextHandUnixMs` is
  only armed after the final broadcast + showdown.
- No test needed for the achievements item — already covered by existing showdown/fold-to-one test coverage confirming
  `WinningCategory` is empty on non-showdown wins.
