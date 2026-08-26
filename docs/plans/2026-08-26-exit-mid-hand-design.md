# Exit mid-hand without waiting for the turn — Design

## Status

Draft, backend engine layer implemented. Written after fixing three `useTableRealtime.ts`
websocket-ordering races in this same session (a premature `pendingActionRef` clear on an
unrelated broadcast, a shared-ref resync watchdog/timer starving concurrent commands, and a stale
`postBigBlindActionRef`) — this feature's countdown UI builds directly on that hook, so the race
fixes landed first on purpose.

**Correction (during implementation):** the "Reused as-is: `Table.SitOutForActor`" claim below is
wrong for the non-current-turn case. `betting.Round.Act` has no turn-order check at all —
`SitOutForActor` folds *any* `Active` player it's called on, not just the one on the clock (the
disconnect/turn-timeout caller only looks turn-gated because of an explicit guard at that call
site, `actor.go:1818`, not inside `SitOutForActor` itself). Calling it unconditionally from
`RequestExit` would force-fold an exiting BB/SB immediately, before their turn ever comes back
around — breaking the "still credited if they win uncontested" requirement, caught by
`TestRequestExitAsBlindStillWinsUncontested` failing during TDD. Fixed by only calling
`SitOutForActor` when the player *is* the current actor; otherwise they're left exactly as they
are, and a new `Actor.processPendingExitAutoFolds` (hooked into `broadcastAll`, the same
per-commit reconciliation point `armTurnTimer`/inline preselections already use) folds them the
instant their own turn actually arrives. See "Backend flow" below for the corrected version.

## Problem

Today, `LeaveDialog.tsx` calls `POST /rooms/:id/leave` (`leaveRoom`), which the engine
(`Table.RemovePlayerForActor`, `api/internal/engine/hand/hand.go:592`) flatly rejects with an
error while the player is still `dealtIntoCurrentHand` — true for `Active`/`AllIn`/`Folded` states
until the hand fully settles at `Complete`. The frontend disables the "Sair" button for the whole
hand (`dealtIn` prop) rather than surface that rejection. A player who wants out mid-hand has to
sit and watch the rest of it play out with no feedback beyond a disabled button.

## Goal

Let a player request exit at any time, including mid-hand:

1. Request goes over the websocket, not HTTP — it needs to reach the same actor that owns turn
   state, and the frontend needs a live snapshot field to render pending-exit UI regardless of
   which client/tab is watching.
2. The request immediately **pauses** the player (no future hands) and, if it's currently their
   turn, auto-folds them out of the live betting round right then. If it is *not* currently their
   turn, they are left exactly as they are and auto-folded the instant their own turn actually
   arrives — see "Backend flow" for why this can't just be a verbatim `SitOutForActor` call.
3. If they win the current hand anyway (e.g. requested exit as BB/SB and action folds around to
   them before their turn comes up again), they are credited normally — this requires no special
   casing, only that we never force-fold someone out of a pot they can still win uncontested.
4. Once no longer dealt into the current hand, they are actually removed and cashed out
   automatically — no second "confirm leave" click needed.
5. The request is cancelable any time before that removal actually happens.

## Reused vs. new

**Reused as-is:**
- `Table.SitOutForActor` (`hand.go:469`) — but only when `playerID` is the player currently on the
  clock. `betting.Round.Act` has no turn-order check, so `SitOutForActor` folds *any* `Active`
  player passed to it — its only existing caller (`actor.go:1818`'s disconnect/turn-timeout path)
  looks turn-gated purely because of an explicit `CurrentPlayerIDForActor() != c.PlayerID` guard at
  that call site, not anything inside `SitOutForActor` itself. `RequestExit` reuses the exact same
  guard.
- `Table.RemovePlayerForActor` / `Actor.applyLeaveAndCommit` / `Actor.handleLeave` — the exact same
  settlement path `leaveRoom` already uses (stack computed post-removal, `SettlementIntent`
  transacted alongside the commit, `ErrVersionConflict` retried). Not duplicated.
- `Table.dealtIntoCurrentHand` (unexported) already answers "can this player be removed right
  now" — `RemovePlayerForActor` calls it internally. It needs one new exported wrapper,
  `DealtIntoCurrentHandForActor`, for the sweep in `internal/table` (a different package) to check
  eligibility *before* attempting a removal, same naming convention as
  `CurrentPlayerIDForActor`/`LastOutcomeForActor`.

**New:**
- `Player.PendingExit bool` (`internal/engine/hand`), persisted like `SittingOut` — survives
  `NewTableFromState` round-trips and actor handoffs. An actor-local map (like `disconnectedSince`)
  was considered and rejected: `api/CLAUDE.md` already documents a real production bug from that
  exact pattern (multiple instances serving one table, an instance-local mark cashing out players
  who had merely reconnected elsewhere).
- `Table.RequestExit(playerID)` / `Table.CancelExit(playerID)` /
  `Table.CurrentPlayerHasPendingExitForActor()`.
- `Actor.processPendingExitAutoFolds` — folds the current actor the instant their turn arrives, if
  they have a pending exit. Hooked into `broadcastAll` (the same per-commit reconciliation point
  `armTurnTimer` and inline preselections are already driven from), immediately before
  `processInlinePreselections` so an exit takes priority over any stale preselection for the same
  turn.
- A post-commit sweep that actually performs the deferred removal once eligible — also hooked into
  `broadcastAll`, right after the two loops above, rather than scattered across individual command
  handlers.
- Two new WS commands and one new `Seat` snapshot field (protocol section below).

## Protocol

`tablews.go:111`'s command switch gains two entries, same shape as `request_rabbit_hunt`:

- `request_exit` — no payload beyond `action_id`.
- `cancel_exit` — no payload beyond `action_id`.

`Seat` (`proto/poker.proto`) gains field 21:

```protobuf
// The player has asked to leave. They are paused (no future hands) and,
// once no longer dealt into the current hand, will be removed and cashed
// out automatically. Cancelable via cancel_exit until that removal commits.
optional bool pending_exit = 21;
```

No new timer/deadline field: the frontend derives "how long until I'm forced out" entirely from
existing fields — `current_player_id` and the seat's own turn deadline — per the countdown design
below.

## Backend flow

### `RequestExit`

```go
func (t *Table) RequestExit(playerID string) error {
    p := t.playerByID(playerID)
    if p == nil {
        return fmt.Errorf("%w: %s", ErrPlayerNotFound, playerID)
    }
    p.PendingExit = true
    if t.currentPlayerToAct() == playerID {
        t.SitOutForActor(playerID) // it's their turn right now: fold immediately, same as a disconnect timeout
        return nil
    }
    p.Ready = false
    if p.State == Active {
        return nil // dealt in, live, not their turn: leave them exactly as they are
    }
    if p.State != AllIn {
        p.State = SittingOut // not currently live in a round (Folded/waiting/etc): just pause
    }
    return nil
}

func (t *Table) CurrentPlayerHasPendingExitForActor() bool {
    p := t.playerByID(t.currentPlayerToAct())
    return p != nil && p.PendingExit
}
```

`Actor.handleRequestExit` commits this, then `broadcastAll` (see below) immediately attempts the
same-shape removal the sweep performs — so a player who is *not* currently dealt in (waiting, or
between hands) leaves right away, same latency as today's `leaveRoom` HTTP call.

### Turn-arrival auto-fold

```go
// processPendingExitAutoFolds folds out, one at a time, whoever is
// currently on the clock and has a pending exit request — the moment their
// turn actually arrives, not when RequestExit was called (an uncontested
// win owed to them before their turn comes back around must still pay
// out). Mirrors processInlinePreselections's loop shape exactly (same
// applyActAndCommit + commitOutcomeLogEntries tail).
func (a *Actor) processPendingExitAutoFolds(ctx context.Context) {
    for a.cached != nil && a.cached.Stage() != hand.Complete && a.cached.CurrentPlayerHasPendingExitForActor() {
        current := a.cached.CurrentPlayerIDForActor()
        autoActionID := fmt.Sprintf("auto-exit-fold-%s-%d", current, a.version)
        applied, err := a.applyActAndCommit(ctx, ActCmd{
            PlayerID: current, ActionID: autoActionID, Action: betting.ActionFold,
        })
        if err != nil || !applied {
            if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
                slog.Error("table reload after pending-exit auto-fold failed", "table_id", a.id, "err", reloadErr)
            }
            return
        }
        if err := a.commitOutcomeLogEntries(ctx); err != nil {
            slog.Error("table pending-exit auto-fold outcome log commit failed", "table_id", a.id, "err", err)
        }
    }
}
```

Called from `broadcastAll`, immediately before the existing `processInlinePreselections` call —
`broadcastAll` runs after every commit that changes table state (it's the single place
`armTurnTimer` is armed from, per its own doc comment), so this is the natural, already-proven-safe
reconciliation point: no new timer machinery, no risk of firing before a commit lands.

### `CancelExit`

Clears `PendingExit` if the player is still seated. No-op error ("already removed") if the sweep
already committed their removal — the frontend treats that error as "no action needed, the
`removed` push is already on its way."

### The sweep

```go
// removeEligiblePendingExits runs after any commit that could change
// dealtIntoCurrentHand (fold, showdown settle, stage advance to Complete,
// StartHand's next-hand prep). Not gated behind claimHandHooks — that guard
// exists to fleet-dedupe optional side effects (gamification), but this is
// a correctness-required removal, and RemovePlayerForActor's own
// conditional commit already makes a duplicate attempt from another
// instance a safe, cheap no-op (same protection handleLeave's
// ErrVersionConflict retry already relies on).
func (a *Actor) removeEligiblePendingExits(ctx context.Context) {
    for _, p := range a.cached.PlayersForActor() {
        if !p.PendingExit || a.cached.DealtIntoCurrentHandForActor(p.ID) {
            continue
        }
        cmd := a.systemLeaveCmd(ctx, p.ID, "exit_requested", nil, nil)
        if err := a.handleLeave(ctx, cmd); err != nil {
            slog.Warn("table exit sweep", "table_id", a.id, "player_id", p.ID, "err", err)
        }
    }
}
```

Call site: `broadcastAll`, immediately after `processPendingExitAutoFolds` (both run on every
commit that changes table state — a fold/call/check, a stage advance, `StartHand`'s next-hand
prep, `RequestExit`/`CancelExit` themselves — so this single hook covers every case the design
originally scattered across `handleAct`'s tail and `handleNextHand` individually). It runs on every
one of those commits but is a no-op until `DealtIntoCurrentHandForActor` actually goes false —
confirmed by the integration tests (`TestRequestExitAsBlindStillPaysOutOnUncontestedWin` et al.)
that `t.handOrder` is *not* cleared merely by a hand reaching `Complete` (per
`RemovePlayerForActor`'s own doc comment: "otherwise only cleared by the next `StartHand`"), so a
pending-exit player stays seated through the entire post-hand `Complete`-stage window (win banner,
recap) and is only actually removed once the *next* hand's `StartHand` runs.

### Uncontested-win case, walked through

Player posts BB, requests exit. They are not the current actor (SB acts first preflop
heads-up-style, or it's simply not their turn in a larger table), so `RequestExit` leaves them
`Active` and untouched — no fold. Everyone else folds preflop. The hand resolves normally: BB is
credited the pot via the existing showdown-less award path, `t.stage` becomes `Complete`,
`dealtIntoCurrentHand` goes false, the sweep (run from `broadcastAll`, triggered by that last
fold's own commit) removes and pays them out. `processPendingExitAutoFolds` never got a chance to
fire because `CurrentPlayerHasPendingExitForActor` was never true for them before the hand ended —
the money and win logic is entirely unmodified.

If instead action *does* come back to the BB (someone raises and it's their turn again),
`processPendingExitAutoFolds` folds them automatically on the very next `broadcastAll` after that
raise commits — same fold mechanics `SitOutForActor`'s current-actor branch already uses, just
triggered by turn arrival instead of by the original `RequestExit` call.

## Frontend flow

- `LeaveDialog`'s confirm action sends `request_exit` over the websocket
  (`useTableRealtime`'s `emit`) instead of the HTTP `leaveRoom` call. Not dealt in → resolves
  effectively instantly, same UX as today. Dealt in → the dialog closes into a persistent status
  rendered on the table (not a modal, since the player should keep watching the hand play out).
- Status copy:
  - Not currently their turn: **"Saindo assim que a mão terminar"** with a "Cancelar saída"
    action. No literal countdown — there is no deterministic number for how long other players'
    turns take, and a fake estimate that visibly self-corrects reads as broken (this was the
    losing option in the brainstorming pass).
  - Currently their turn (`current_player_id === viewer` and `pending_exit` true on their own
    seat): swap to the seat's *existing* turn-countdown ring/timer component — this is genuinely
    deterministic (`turn_timeout_seconds`), and it's heading to an auto-fold either way, so
    reusing the real countdown the player already understands beats inventing a second one.
- `useTableRealtime` grows `requestExit()` / `cancelExit()` alongside the existing auxiliary
  commands (`requestRabbitHunt`, `requestWinnerCards`), same pending/lock/timeout/retry shape —
  including riding on the just-fixed keyed `resyncTimers`/`resyncWatchdogs` and the
  now-not-prematurely-cleared `pendingActionRef`, so this feature does not need to reinvent any of
  that plumbing.
- Final removal arrives as the existing `removed` snapshot event (already used for disconnect
  kicks) carrying the settled stack — closes the loop even if the player has navigated away from
  the tab and comes back later.
- UI implementation (status component, countdown swap, cancel affordance, empty/error states) goes
  through `/impeccable` once this doc is approved, per project convention for anything touching the
  design system.

## Money ordering

Unchanged from today's leave path: remove-then-credit, `SettlementIntent` transacted alongside the
same commit that removes the player (`api/CLAUDE.md`'s documented convention). A settlement
failure here is not a new failure mode — it already retries the identical way an ordinary
`leaveRoom` failure does.

## Out of scope

- Real-money mode: this reuses the exact same settlement path `leaveRoom` already uses under
  `currency_mode`, so nothing here changes that gate either way.
- Mid-hand *join* semantics (`AddMidHandJoiner`) — unrelated, unaffected.
- Disconnect-driven sit-out/kick timers (`disconnectedSince`, `kickTimers`) — this is a distinct,
  voluntary, explicit-intent path; it happens to reuse `SitOutForActor` as a primitive but does not
  change disconnect handling.

## Testing

- `engine/hand`: `RequestExit` mid-round auto-folds the current actor (extends the existing
  `SitOutForActor` fold test with the `PendingExit` flag asserted alongside); `RequestExit` as a
  street-already-acted player who then wins uncontested (asserts payout, no fold); `CancelExit`
  clears the flag; `CancelExit` after the sweep already removed the player errors cleanly.
- `internal/table` (`actor_test.go`, `-tags integration`): sweep runs after fold/showdown/StartHand
  and actually removes+settles; a concurrent duplicate sweep attempt (simulating two instances) is
  a safe no-op via the conditional commit.
- `useTableRealtime.test.tsx`: `requestExit`/`cancelExit` pending/lock/timeout shape mirrors the
  existing auxiliary-command tests; a `pending_exit` seat flips the status UI; turn-timer swap when
  `current_player_id` matches the viewer.
- `RabbitHunt`-style component test for the new status component once `/impeccable` produces it.
