# 2026-09-29 — a hand that ended with nobody publishing it, and a recap that said the session lost nothing

Two independent production bugs, both reported from the same sitting at table
`01M3PV0VYG386A6V8GP1WY7K7K` (player `78fd4d57-88a7-4eec-a997-7d7c09f58a1b`), both diagnosed from
a browser HAR of the table socket cross-referenced against `prod_poker_action_log` and
`/ctech-poker/prod/app`.

---

## 1. `syncWithoutPublish`'s own sweeps committed state nobody broadcast

### Symptom

Hand `01M3PX5FE66CJ101WY7E4Q9KDK`: the player made three-of-a-kind on the river, raised, was
called and won the 120.000 pot. The client's table **froze on the river for ~12 seconds** — no
showdown, no winner, no payout animation — and only repainted when the *next* hand's first
snapshot arrived, at which point it was already their turn with a full board on screen.

### Evidence

Decoded from the HAR, this was the only one of the sitting's 14 hands whose `complete` snapshot
never reached the client:

| hand | stages delivered |
|---|---|
| …HJETSB | pre_flop, flop, turn, river, **complete** |
| **…4Q9KDK** | pre_flop, flop, turn, river — *(no complete)* |
| …1MQA0S | pre_flop, flop, **complete** |

The socket was healthy throughout: the keepalive ping sent at 15:41:12.8, inside the gap, got its
pong back in 230 ms. `snapshot_version` jumped 745 → 749, and `prod_poker_action_log` shows
exactly what lived in that gap:

```
745  1790696472949  953ac369  call                                        → published
746  1790696472979  60ac2555  fold  (auto-preselect-60ac2555-…-fold-745)  → never published
747  1790696473013  78fd4d57  won   117000                                → never published
748  1790696473035  953ac369  lost                                        → never published
749  1790696484979            next_hand                                   → published
```

### Root cause

`Actor.sync(publish bool)` runs three sweeps — `processPendingExitAutoFolds`,
`processInlinePreselections`, `removeEligiblePendingExits` — and **all three commit**. It then
publishes only when `publish` is true.

`handleExternalChange` (the `internal/tablenotify` ChangeNotifier path) calls
`syncWithoutPublish()`, on the rule established by
`2026-09-17-table-snapshot-divergence-and-highlight-winner.md`: `ws.RedisRegistry.Broadcast` is
fleet-wide from a single publish, so a sibling republishing the committing instance's version only
produces a duplicate frame carrying that sibling's own streak/equity overlays, which the UI cannot
order against the first.

That rule is right about the **sibling's** state and wrong about state the sweeps create *here*.
Two Go processes serve this instance (`app`, `app2` behind nginx round-robin — both visible arming
timers for this table in the logs). Kelizinha's `call` committed 745 on one process, which
published it. The ChangeNotifier signal reached the other, which forced a reload, ran the sweeps,
found Luvas' preselected fold now on the clock, and committed 746 — ending the hand and paying the
pot — plus 747/748 from `commitOutcomeLogEntries`, all with `publish=false`. No other instance
could send that state: it did not exist anywhere until this sweep created it.

The contrast is in the same hand. Version 744 was also an auto-preselect fold, and it *was*
delivered — it fired on the instance handling the raise that opened that player's turn, so it ran
under `broadcastAll` (`publish=true`).

Nothing was lost or mispaid: the pot was credited correctly (the stack goes 918.625 → 1.035.625 in
v749). The failure is entirely in what the players were shown.

### Fix

`internal/table/actor_views.go` — `sync` captures `a.version` before the sweeps and re-enables the
publish when they moved it:

```go
versionBeforeSweeps := a.version
a.processPendingExitAutoFolds(sweepCtx)
a.processInlinePreselections(sweepCtx)
a.removeEligiblePendingExits(sweepCtx)
if a.version != versionBeforeSweeps {
    publish = true
}
```

The suppression keeps doing its job — a signal whose sweeps commit nothing still reloads, re-arms
timers and stays off the wire — so #2026-09-17's flicker fix is intact. This also covers the other
two sweeps, which could otherwise fold a pending-exit player or settle and remove a seat with no
broadcast at all.

### Regression tests

`internal/table/changenotify_test.go`:

- `TestHandleExternalChangePublishesStateItsOwnSweepCommitted` — heads-up, a preselected fold
  waiting, the sibling's action applied without a broadcast, then `handleExternalChange`. Asserts
  the sweep committed, the hand completed, something was published, and that the published
  snapshot carries the post-sweep version, `stage: complete` and a winner. Fails on the
  pre-fix code with *"the sweep committed version 3 -> 5 and published nothing"*.
- `TestHandleExternalChangeStillSuppressesWhenNoSweepCommits` — the same signal with a
  preselection that is *not* on the clock. Asserts the version did not move and nothing was
  published, pinning the suppression against a regression in the other direction.

---

## 2. The session recap reduced the final stack against a pre-rebuy buy-in

### Symptom

Leaving the table showed **"Resultado da sessão: 0"** after a sitting that had in fact lost a full
1.000.000-chip buy-in.

### Evidence

`GET /v1.0/players/me/sessions` for the same session, before and after the exit:

| moment | buyin | cashout | net_pnl |
|---|---|---|---|
| during the sitting (last read 15:36:50) | 1.000.000 | 0 | 0 |
| after the exit (15:44:42) | 2.000.000 | 1.000.000 | **−1.000.000** |

On the final hand (`…CDM08V`, 15:44:33) the player went all-in for 975.625 and lost; `auto_rebuy`
restored the stack to 1.000.000, taking the session's total buy-in to 2.000.000. They cashed out
1.000.000 six seconds later. The server's arithmetic was correct the whole time.

### Root cause

`useTableRemoval` built the recap from the React Query cache:

```ts
buyIn: openSessionAtRemoval?.buyin_amount || 0,   // cached ['sessions'] page
finalStack: amount                                // the `removed` frame
```

and `SessionRecap` rendered `finalStack - buyIn`. The cached page was read at 15:36:50, nine
minutes before the auto-rebuy that doubled the buy-in, so the subtraction was
`1.000.000 − 1.000.000 = 0`. The recap is computed once and latched by `handledRemovalRef`, so the
refetch that arrived three seconds later with the right number could not correct it.

### Fix

`GET /v1.0/players/me/sessions/:sessionId/recap` (`sessionlog.Store.SessionRecap`, shipped by #310)
already derives the authoritative figures server-side and was unused by the web client. The client
now asks it:

- `PlayerSession` exposes `sk`, and `useTableRemoval` passes it through as `SessionRecap.sessionId`.
- `getSessionRecap(sessionId, mode)` in `lib/api/player.ts`.
- `SessionRecap` renders the props optimistically (so the dialog is never blank or delayed) and
  replaces buy-in, result, hands played and biggest pot once the recap resolves.

`sessionResult` is exported and unit-tested because the result has **two** authoritative sources
and which one applies depends on the row's state. `net_pnl` is only written by `CloseSession`, and
the `removed` frame can reach the client before that settlement lands, so an open row still reports
0. The rule:

| recap state | result |
|---|---|
| not fetched / failed | `finalStack - buyIn` (the caller's cached figures) |
| fetched, `ended_at === 0` | `finalStack - recap.buyin_amount` |
| fetched, `ended_at !== 0` | `recap.net_pnl` |

`buyin_amount` is correct immediately either way: `AddBuyin`'s atomic `ADD` lands with the rebuy,
independent of the close.

This also removed the component's own 3-page, 150-hand `getHands` walk — `hands_played` and
`biggest_win` come from the same single bounded call, so the recap costs one request instead of up
to four, and the cap label moved from "últimas 150" to the server's "últimas 200"
(`sessionlog.RecapHandScan`).

### Regression tests

`ui/src/components/table/SessionRecap.test.tsx`:

- `uses the recap buy-in, not the stale cached one, for the result` — the incident, verbatim:
  props `buyIn: 1_000_000, finalStack: 1_000_000`, recap `buyin_amount: 2_000_000`. Asserts the
  optimistic `0` is replaced by `-1.000.000`.
- `prefers net_pnl once the session row is closed`.
- `falls back to the caller figures when the recap fetch rejects`, and
  `skips the fetch entirely without a session id` (a removal answered before the sessions query
  ever resolved).

Both of the first two fail against the pre-fix arithmetic.

`ui/src/lib/hooks/useTableRemoval.test.tsx` asserts the recap carries the open session's `sk`, and
carries `sessionId: undefined` when the settled sessions hold no open seat.

---

## Cross-repo

Neither fix is shared-code. #1 is poker's own actor model (`internal/table`); no sibling service
runs a per-table actor with commit-bearing broadcast sweeps. #2 is the poker table surface. The
`ChangeNotifier`/`syncWithoutPublish` pattern is local to this repo — `ctech-go-common`'s `ws`
package only provides the fleet-wide `Broadcast` both halves of the reasoning rest on, and that
behaved exactly as documented.

## Not addressed

- The `stale_state` error at 15:41:10.96 in the same hand (the client sent its raise against v741
  while v742 was in flight) is a benign race the client already recovers from with `sync_state`.
  Left alone.
- The client has no independent recovery for a missing `complete` snapshot — it waits for the next
  frame. The server-side fix removes the known cause; a client-side watchdog was not added.
