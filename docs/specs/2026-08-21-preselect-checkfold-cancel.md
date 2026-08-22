# Check/Fold Preselection: Cancel Instead Of Auto-Fold On A New Raise — Design

## Summary

`check_fold` today means "check if free, otherwise fold" evaluated only at the moment it becomes the player's turn
(`api/internal/table/actor.go:1887-1893`, `processInlinePreselections`). It carries no memory of what was facing the
player when they set it. If the amount to call rises *after* the preselection was set but *before* the player's turn
comes around, the player gets an instant, unreviewable auto-fold to a raise they never saw — reported behavior:
preselect check/fold with nothing to call, another player raises, a third player (on `call_any`) calls, and the
preselecting player folds automatically the instant it's their turn, with no chance to react to the raise.

The fixed-`call` preselection already solves the analogous problem: it stores the call amount at selection time
(`Preselection.Amount`) and is atomically deleted the moment another player's action changes what's owed
(`applyActAndCommit`, `actor.go:853-864`, `TestRaiseCancelsFixedCallWhenAmountChanges`). This spec applies the same
mechanism to `check_fold`.

## Behavior change

- `check_fold` now also snapshots the call amount facing the player at the moment it's set (currently forced to `0` —
  `actor.go:421-423`).
- The same pruning loop that already cancels a stale fixed `call` (`actor.go:858-863`) is extended: if the *current*
  call amount for a player holding a `check_fold` preselection is now **greater** than what was snapshotted, the
  preselection is deleted — not resolved as a fold. The player regains full manual control on their turn.
- If the call amount facing the player is unchanged (including the case where it was already >0 when they set
  `check_fold` — i.e., they deliberately queued "fold to this bet") the preselection still resolves exactly as before:
  check if free, fold if not. This is not a behavior change for that case — a player who preselects
  fold-to-an-existing-bet still gets that fold; only a *new* escalation invalidates the intent.
- `fold` (unconditional) and `call_any` are untouched: `fold` always means fold regardless of amount by definition, and
  `call_any` already means "call whatever it is," so neither has an ambiguity to resolve.

## Where

Backend only, `api/internal/table/actor.go`:

- `handlePreselect`: the `if c.Selection != "call" { c.Amount = 0 }` branch (line 421-423) changes to snapshot
  `a.cached.ProspectiveCallAmountForActor(c.PlayerID)` into `c.Amount` for `check_fold` too, not just `call`.
- `applyActAndCommit`'s pruning loop (line 858-863): the existing `if preselection.Selection == "call" &&
  ...` condition gains a `check_fold` branch using `>` instead of `!=` (a `call` preselection is exact-amount, a
  `check_fold` preselection tolerates anything up to what was originally faced).

No frontend change. `ui/src/lib/actionPreselection.ts`'s `resolvePreselection` only decides what the client *previews*
right now from the snapshot's current `ActionPreselection`/`legalActions`/`callAmount` — it never executes anything
itself. The server is what actually resolves preselections (`processInlinePreselections`) and reports the live
preselection state on every broadcast (`actor.go:2010-2013`). Once the server deletes a stale `check_fold`, the next
snapshot simply omits
`ActionPreselection`, and the existing client preview logic already reacts correctly to that with no changes needed.

## Testing

`api/internal/table/preselection_test.go`, mirroring `TestRaiseCancelsFixedCallWhenAmountChanges`:

- **New test — `check_fold` cancelled by a new raise:** a 3-handed hand, preselect `check_fold` for a waiting player
  while they face `call amount == 0`, have the current player raise, assert the preselection is gone from
  `actor.activity.Preselections` (not resolved to fold).
- **New test — `check_fold` still resolves fold when the facing amount doesn't change:** preselect
  `check_fold` for a player already facing a non-zero call amount, advance without any further raise, assert
  `processInlinePreselections` still folds them when it becomes their turn (regression guard — this is the case that
  must keep working exactly as today).
- Existing `TestInlinePreselectionExecution` and `TestRaiseCancelsFixedCallWhenAmountChanges` must keep passing
  unchanged.

## Out of scope

- Any change to `fold` or `call_any` semantics.
- Any frontend UI change — the existing preview/highlight logic already degrades correctly once the server stops
  reporting a cancelled preselection.
- Persisting *why* a preselection was cancelled (e.g., for a toast/notification telling the player "your check/fold was
  cancelled because X raised") — not requested, and the existing UI has no precedent for surfacing preselection
  lifecycle events at all.
