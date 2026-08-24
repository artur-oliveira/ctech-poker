# player.har review — five fixes shipped

## Summary

A player-reported HAR capture plus a CloudWatch log excerpt surfaced six issues; five had a small, self-contained fix
and shipped directly (this doc). The remaining two need new behavior/design decisions and are specced separately:
`docs/specs/2026-08-24-pay-to-see-cards-consent.md` and
`docs/specs/2026-08-24-graceful-ws-shutdown-on-deploy.md`.

## 1. "Biggest pot of the day" highlight counted refunded chips as won

`highlights.Store.RecordHand` (`api/internal/highlights/store.go`) summed `HandOutcome.Payouts`, which includes
uncalled-bet refunds (e.g. a 100,000-chip all-in everyone folds to — the bettor gets their own chips back, nothing was
actually won). `hand.go`'s showdown logic already tags those pot layers `PotResult.Refund: true` and excludes them from
`Winners`; the frontend's
`playerPotBreakdown` (`ui/src/lib/tableOutcome.ts`) already separates `won` from `refund` for the per-seat win
animation. The highlight store just wasn't using that same distinction.

**Fix:** sum `PotResult.PayoutAmount` over `outcome.PotResults`, skipping `Refund` layers. Side pots are still combined
into one number — only refund layers are excluded — since all non-refund layers represent chips that genuinely changed
hands in that same hand.

## 2. Leave-table button stayed enabled and clickable mid-hand

`Table.RemovePlayerForActor` (`api/internal/engine/hand/hand.go`) has always correctly rejected removing a player still
dealt into the current hand (including after folding, until showdown resolves). The frontend's `LeaveDialog` never
reflected that: the button was always enabled, so clicking it while dealt in opened the dialog and failed with a 409 the
player had no way to anticipate.

**Fix:** `LeaveDialog` takes a new required `dealtIn` prop; `ui/src/app/table/page.tsx` passes
`Boolean(viewerSeat?.dealt_in)` — the same signal the server keys its rejection on, already on the wire. The trigger
button is `disabled` with a `title` explaining why. This follows the project rule: *the backend validates, but the UI
must never send a request it already knows will fail.*

## 3. Insufficient-balance buy-in leaked an internal error chain to the client

`buyin.Service`'s wrapped error (`fmt.Errorf("buyin: debit: %w", err)`) was serialized verbatim into the RFC 9457
`detail` field by `rooms.go`'s join/leave handlers (`problem.Conflict(err.Error())`), producing
`"buyin: debit: walletclient: Insufficient Balance: saldo insuficiente..."` instead of a clean message. `dailyreward.go`
already had the right pattern for this (`errors.As` into `*walletclient.Error`, re-emit its own clean
`Status/Type/Title/Detail`).

**Fix:** promoted that pattern into a shared `problem.FromWalletError(err) (*Problem, bool)`
helper; `dailyreward.go`'s local copy now calls it, and `rooms.go`'s `join`/`leave` handlers try it before falling back
to the generic `problem.Conflict(err.Error())`.

## 4. Kick-retry loop never stopped once the player was already gone

`Actor.handleKickTimeout` (`api/internal/table/actor.go`) rescheduled itself (`armKickRetry`) on *any* error from
`handleLeave`, without distinguishing "still dealt into a hand in progress" (transient — worth retrying) from "player
not found" (terminal — they're already gone, nothing left to do). Separately, `ensureLoaded`'s reload of `a.cached` from
durable state never reconciled `disconnectedSince`/`kickTimers` against the reloaded player list, so a kick timer armed
by an actor instance that's now stale (another node's instance already handled the leave) had no way to notice and stop
itself.

**Fix, two parts:**

- `hand.RemovePlayerForActor` now returns a wrapped `hand.ErrPlayerNotFound` sentinel instead of an ad hoc string.
  `handleKickTimeout` checks `errors.Is(err, hand.ErrPlayerNotFound)` and, on a match, clears the local bookkeeping and
  stops instead of rearming.
- `ensureLoaded` now calls `pruneStalePresence()` after every reload, which stops and drops any
  `kickTimers`/`disconnectedSince` entry for a player no longer in the freshly loaded cache — the same self-healing
  guarantee `rearmTimersFromCache` already gives the turn/runout/next-hand timers on every reload.

## 5. Connection status flashed red on every transient drop

`@aoctech/ws-client`'s `useWebSocket` (an external package) sets `status` to `'error'` then
`'disconnected'` the instant a socket drops, and only flips to `'reconnecting'` once its retry timer actually fires — so
every drop (including a backgrounded-tab throttle resolving itself)
flashes the "connection lost" UI even though a bounded retry is already scheduled and the library hasn't given up
(tracked by its own `attempt` counter vs `MAX_RECONNECT_ATTEMPTS`).

**Fix (local to this repo, no patch to the external package):** `useTableRealtime.ts` now derives its exposed `status`
by remapping a raw `'error'`/`'disconnected'` to `'reconnecting'` whenever
`reconnectAttempt <= MAX_RECONNECT_ATTEMPTS` — i.e. exactly the window in which the library is guaranteed to still
retry. Once attempts are truly exhausted (`attempt > MAX_RECONNECT_ATTEMPTS`), the raw status passes through unchanged,
preserving `table/page.tsx`'s existing "give up, tap to retry" copy (`connectionCopyFor`), which already used that same
threshold.

## Not fixed here (investigated, no bug found)

The "check/fold shown as allowed while facing a bet" report did not reproduce in the engine (`snapshot.go`'s
`legalActionsFor`, `betting.go`'s server-side enforcement, and the frontend's
`ActionBar.tsx` all key off the same live, per-street `owed` computation). The most likely explanation is
`ActionBar.tsx`'s pre-turn "Check/Fold" preselect quick-action, whose label is a standing instruction ("check if free
when your turn comes, else fold") and is unconditionally shown before it's the viewer's turn, regardless of the current
bet — it still resolves correctly at execution time (`actionPreselection.ts`). Left as-is; revisit only if a live
`state` frame capturing an actual illegal `check` in `legal_actions.actions` turns up.
