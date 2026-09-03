# Table module decomposition (2026-09-02)

Closes #67, #99, #100, #98, #81. Partial on #50.

The table page had grown into the app's god component: ~824 lines, eight TanStack queries, the
whole showdown-banner assembly inline, and its own 1 Hz `setNow` re-rendering the entire tree
while a turn timer ran. Nothing here changes what the player sees, except the three fixes called
out below.

## What moved where

| Concern | Now lives in |
| --- | --- |
| Every server read the surface needs | `lib/hooks/useTableSession.ts` (`useTableSession`) |
| The removed-frame reaction + leave recap | `lib/hooks/useTableSession.ts` (`useTableRemoval`) |
| Showdown bookkeeping (remembered pre-blind stack, frozen next-hand deadline, fire-once banner) | `lib/hooks/useTableOutcome.ts` |
| The asides' shared slot, the two standalone dialogs, `E`/`T`, the reaction cooldown | `lib/hooks/useTableOverlays.ts` |
| Action-bar derivation from `legal_actions` | `lib/tableActions.ts` (`actionState`) |
| The banner itself — winner/runner-up/tied hands, per-pot detail, `couldHaveWon`, chip delta | `lib/tableOutcome.ts` (`buildHandOutcome`) |
| The idle-removal alert | `components/table/IdleWarning.tsx` |
| Resilience vocabulary: codes, timeouts, retry budget, `auxRetryDelayMs`, player copy | `lib/tableResilience.ts` |
| Snapshot-transition narration and sound | `lib/tableNarration.ts` |

The page is now 435 lines and derives nothing itself.

## The three behaviour changes

**#100 — one clock.** An active turn used to arm four to six independent intervals (the action
bar's 250 ms deadline clock, one per timed seat, `RealityCheck`'s 15 s sweep, the idle warning's
1 s, the reduced-motion countdown), each with its own React state update.
`lib/hooks/useSharedTicker.ts` now keeps a single `setInterval` at the shortest cadence any
subscriber asked for and notifies each one only when its own period has elapsed, so accuracy is
unchanged and with no subscribers there is no interval at all. `useLiveNow` subscribes to it, and
everything that ticks goes through `useLiveNow` — including `useReducedMotionCountdown` and
`RealityCheck`, which no longer own timers. `IdleWarning` additionally arms nothing during the
quiet part of an idle spell: one timeout waits out the time before the last minute, and the
countdown is scoped to the deadline it armed for so a re-armed deadline starts the wait over.
`tickerIntervalCount()` exists so the "at most one interval" guarantee is assertable.

**#98 — reaction FX aim at live seats.** `TableReactions` located seats with
`document.querySelectorAll('.game-seat[data-player-id]')`, and `ReactionEffect` read
`getBoundingClientRect()` once in a ref callback, then let the CSS custom props it wrote drive the
whole ~3 s flight. Any layout change inside that window — an orientation flip between
`TableStage`'s oval and portrait-ring layouts, the outcome banner reflowing, a seat-ring resize —
left the projectile arcing at a coordinate the recipient had left. `Seat` now publishes its own
element through `lib/seatRects.ts` (a module singleton, like the token in `lib/api/client.ts`:
seat elements are viewport geometry and nothing re-renders when one changes), the effect measures
on demand, and it re-measures on `resize` and `orientationchange`. The class/`data-player-id`
contract is gone, so a `Seat` wrapper rename is now a compile-time link. The reduced-motion path
is untouched.

**#81 — the leave recap waits for its data.** The recap's real join time and buy-in come from the
`['sessions', 'me']` query, which can still be in flight when the server answers a "not dealt in,
instant leave" right after a buy-in. The old effect marked the removal handled before reading it,
so the recap reported a 0-chip buy-in and a join time of "now" and the effect bailed out when the
query later settled. `useTableRemoval` now returns early while `sessionsLoading` is true without
marking anything handled; the removal is still handled exactly once, just later.

## #50 is not closed

Only the closure-free leaves came out of `useTableRealtime.ts` (1086 → 947 lines): the resilience
vocabulary and the narration functions, both moved verbatim, with `lib/tableResilience.test.ts`
covering the retry classification and backoff as a pure module. The remaining bulk is ~40
interdependent refs in a single closure — socket lifecycle, snapshot reconciliation, optimistic
preview, `pendingActionRef`/`auxFramesRef` retry and backoff, keyed resync watchdogs. Splitting
that into `useTableSocket` / `useTableActionQueue` / `useTableSnapshotReducer` means threading
those refs across hook boundaries, which is a real risk to reconnect and resubmit semantics on the
core gameplay surface and wants its own change with its own review. The ≤400-line target and the
three-hook split remain open.

## Tests

New: `lib/seatRects.test.ts`, `lib/hooks/useSharedTicker.test.ts`,
`lib/hooks/useTableRemoval.test.tsx`, `lib/tableResilience.test.ts`,
`components/table/IdleWarning.test.tsx`, plus `buildHandOutcome` fixture cases
(win / lose / mixed / tie / fold-could-have-won / masked rival hand / incomplete board /
run-it-twice / remembered stack / not-dealt-in) in `lib/tableOutcome.test.ts` and the
re-aim-on-orientation-change case in `components/table/TableReactions.test.tsx`.

No player-visible strings, controls, states or timings changed, so `src/app/guide` needs no update.
