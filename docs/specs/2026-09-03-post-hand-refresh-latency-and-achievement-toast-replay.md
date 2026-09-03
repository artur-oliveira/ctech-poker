# Post-hand refresh latency & achievement-toast replay

Date: 2026-09-03
Source: `quebrou_as.har` (table `01M1KWY9ZVJ2THEF6ESXYQ71RX`) + code audit.

Two independent bugs, one report.

## Bug 1 — the same achievement toast replays every hand

**Symptom:** after unlocking one achievement, the "CONQUISTA DESBLOQUEADA" toast
reappears at the end of every subsequent hand.

**Not the server.** The HAR's 461 table-socket frames contain exactly one
`achievement_unlocked` frame (`beat_pocket_aces`, 1 star). The server never
re-sends it.

**Root cause (frontend).** `useTableRealtime` sets `unlock` on the
`achievement_unlocked` frame and never cleared it. `AchievementToast` receives
it together with `blocked={Boolean(handOutcome)}`; `handOutcome` is truthy for
the whole `complete` window, so `blocked` flips `false→true→false` once per
resolved hand. On each flip the toast's effect re-captured the still-set
`unlock` into its `queued` ref and replayed it when `blocked` cleared.

**Fix.**
- `AchievementToast` records the signature (`key-stars`) of an unlock that has
  finished its full lifecycle in a `consumed` ref and never re-shows or
  re-queues that same signature. An unlock interrupted mid-celebration by a
  `blocked` window is still resumed afterwards (unchanged).
- On natural completion the toast calls a new `onConsumed` prop; the table page
  wires it to `useTableRealtime`'s new `clearUnlock`, so the socket layer drops
  the unlock it was holding.

Tests: `ui/src/components/AchievementToast.test.tsx`,
`ui/src/lib/hooks/useTableRealtime.test.tsx`.

## Bug 2 — last-winners strip and "maior pote de hoje" update a hand late

**Symptom:** after a hand ends, `LastWinners` (recent hands) and `TodayHighlight`
("Maior pote de hoje") take a long time to reflect the hand that just finished —
usually only after the *next* hand resolves.

**Root cause (race).** The client invalidates `['hands', tableId]` and
`['highlights', tableId, 'today']` the instant it sees the `complete` snapshot.
The server writes those projections on the **detached** post-hand gamification
pipeline (`app.dispatchGamificationPipeline`, #61), which only starts *after*
`broadcastAll` has already sent that snapshot — and the two player-visible
writes sat at the *end* of the pipeline, behind achievements/leaderboard/
pokerstats/matchup (tens-to-hundreds of sequential DynamoDB round trips at a
full table). The client's refetch therefore raced ahead of the write and
returned stale data; nothing re-invalidated until the 30s `staleTime` expired or
the next hand completed.

**Fix — both ends.**
- **Backend** (`internal/app/app.go`, `onHandComplete`): `persistHandHistory`,
  `persistHandReveal` and `highlightsStore.RecordHand` now run first in the
  pipeline, right after the room-mode load. All three are idempotent overwrites
  that depend only on the outcome, so nothing else moves.
- **Frontend** (`ui/src/lib/settleRefetch.ts`): `invalidateAfterSettle`
  invalidates immediately and then re-invalidates on a `[1.5s, 4s, 9s]` backoff,
  cancelled on unmount / next hand. Wired into `useTableOutcome` (for
  `['hands', id]`) and `TodayHighlight` (for the highlight key). This is the
  actual safety net — it fixes the symptom even if a future pipeline change
  reintroduces the ordering problem.

Tests: `ui/src/lib/settleRefetch.test.ts`,
`ui/src/components/table/TodayHighlight.test.tsx`.

CloudWatch was not consulted (no credentials available in the working
environment); the HAR plus the code paths above are conclusive.
