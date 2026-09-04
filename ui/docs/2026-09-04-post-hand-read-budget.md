# Post-hand read budget: the settle backoff became a retry (#229)

Date: 2026-09-04. Supersedes the `invalidateAfterSettle` half of
`docs/2026-09-03-achievement-toast-replay-and-settle-refetch.md`.

## The problem

`invalidateAfterSettle` fired an invalidate immediately and three more on a
`[1.5s, 4s, 9s]` backoff, unconditionally. Two sequences run per settled hand —
`['hands', tableId]` for the last-winners strip and
`['highlights', tableId, 'today']` for the "Maior pote de hoje" pill — so every
seated player spent **eight reads per hand**, 72 at a full 9-max table, even
when the very first read already carried the projection, and even when nothing
about the hand could have changed the highlight at all.

The backoff exists because the server writes both projections on a pipeline it
detaches *after* broadcasting the `complete` snapshot, so one invalidate races
ahead of the write. That reason justifies a retry, not a fixed volley.

## What changed

- **`lib/settleRefetch.ts`** takes an options object with a `settled` predicate
  that reads the query's own cache entry and answers "is the projection there?".
  It is consulted **before every read, including the first**, and the chain
  stops the moment it is true. Attempts are now chained rather than armed all at
  once (each delay is the gap after the previous attempt), so the cache is
  re-read between them; the last delay is the deadline, after which the query is
  left to its normal stale timer. A `settleRefetchReads()` counter (with
  `resetSettleRefetchReads()` as its test seam) measures the reads a session
  actually spent — the request-budget assertions use it. Deliberately not a
  beacon: `lib/telemetry.ts` is the client-*error* sink and this is not an error.
- **`useTableOutcome`** passes `settled: page => page.data.some(hand => hand.hand_id === settledHandID)`.
  A hand whose history row is already there costs zero reads; one that lands on
  the first read costs exactly one instead of four.
- **`TodayHighlight`** takes the settled hand's contested pot as `handPot` and is
  settled when the row on display either *is* this hand or already holds a pot
  this hand could not beat. `highlights.Store.RecordHand` only overwrites today's
  row on a strictly bigger pot, so for every hand that isn't a new record — the
  overwhelming majority — the pill now spends **no read at all**.
- **`lib/tableOutcome.ts`** gained `highlightPot(snapshot)`: the sum of the
  non-refund pot layers' payouts, which is exactly the number the server weighs.
  The table page passes it down, keeping the derivation out of the page itself.

## Budget, per settled hand per player

| Case | Before | After |
| --- | --- | --- |
| Projection already in cache | 4 | 0 |
| Hand cannot beat today's highlight | 4 | 0 |
| Projection lands on the first read | 4 | 1 |
| Pipeline never writes (worst case) | 4 | 4 |

## Guarantees under test

`settleRefetch.test.ts` covers the zero-read, one-read and give-up-at-the-deadline
cases against `settleRefetchReads()`; `TodayHighlight.test.tsx` asserts no read
when the highlight is already this hand and none when the settled pot cannot beat
the one on record, while keeping the existing late-write backoff case;
`tableOutcome.test.ts` covers `highlightPot`'s refund exclusion.

No in-app guide change: nothing the player sees moved. The strip and the pill
still update as promptly as before — they just stop re-asking once the answer
cannot change.
