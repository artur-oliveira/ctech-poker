# Keyboard Shortcuts: Hole-Card Peek + Preselect Actions — Design

## Summary

Two independent additions to the existing keyboard-shortcut convention already established in
`ui/src/components/table/ActionBar.tsx` (`isPlainKey`/`isTypingTarget`, a `window.addEventListener('keydown', ...)`
effect per component, visible `<kbd>` hints next to the shortcut-bound control):

1. **Peek toggle** — `1`/`2` toggle the viewer's own two hole cards face-up/face-down, mirroring the existing
   click-to-peek affordance in `Seat.tsx`.
2. **Preselect action shortcuts** — `X`/`F`/`C`/`A` drive the "Próxima ação" preselect buttons (`ActionBar.tsx`'s
   `PreselectionControls`), plus a new **All In** preselect option that doesn't exist yet today (`A`), added because the
   user asked for it explicitly alongside the shortcuts.

No protocol version bump: the wire's `Preselection.Selection` is already a bare string, so adding `"all_in"` as a new
valid value is additive, the same shape as every existing selection.

## Part 1 — Peek toggle (`1` / `2`)

- Extract `isPlainKey`/`isTypingTarget` (`ActionBar.tsx:52-58`) into `ui/src/lib/utils.ts`. `ActionBar.tsx` imports them
  from there instead of defining them locally; behavior unchanged.
- `Seat.tsx`: new `useEffect`, active whenever `peekGated` (`Seat.tsx:201`) is true — the exact same condition that
  already gates the click affordance, so the shortcut works whenever and only whenever clicking a hole card would.
  `keydown` on `window`: `'1'` → `togglePeek(0)`, `'2'` → `togglePeek(1)`, guarded by `isPlainKey` +
  `event.preventDefault()`. Not scoped to the viewer's turn — peeking is legal any time before hand-complete, same as
  today's click.
- Visual hint: `PlayingCard.tsx` gains an optional `shortcutKey?: string` prop, rendered inside the existing
  `peek-hint` span (`PlayingCard.tsx:48`) as `<kbd>{shortcutKey}</kbd>` next to "Ver", shown only when `peekable`.
  `Seat.tsx`'s two call sites (`Seat.tsx:273-279`) pass `shortcutKey={peekGated ? String(i + 1) : undefined}`.

No backend change —   peeking is, and stays, a purely client-side visibility gate over data the server already sends
unconditionally for the viewer's own cards (`ui/CLAUDE.md`).

## Part 2 — Preselect action shortcuts + new All In preselection

### Current state

`PreselectionControls` (`ActionBar.tsx:92-151`) renders up to four toggle buttons — `check_fold`, `fold`, `call`
(only when `prospectiveCallAmount > 0`), `call_any` — each calling
`onSelectAction(selection === value ? null : value, ...)` (click again to deselect). There is no keyboard path to any of
them today, and no "All In" preselect exists at all (only a live "All In" raise, via `RaiseControl`'s
`isAllIn`/`RaiseControl.tsx` shove-to-max, which requires it already being your turn).

### Key mapping (this component only; see Part 1 for why there's no collision with `ActionBar`'s live `f`/`c`/`p`

shortcuts or `RaiseControl`'s `r`/`h`/`a` — `canPreselect` and `isTurn` are mutually exclusive,
`page.tsx:586`, so only one shortcut set is ever attached at a time)

- **`X`** → toggle `check_fold` (select if not already selected; press again to clear — identical to clicking the
  button).
- **`F`** → toggle `fold`.
- **`C`** → cycles through the call family, mirroring exactly what's visible: if the fixed `call` button is rendered
  (`prospectiveCallAmount > 0`), first press selects `call`, second press (while already on `call`) switches to
  `call_any`, third press clears. If the fixed button isn't rendered (nothing to call yet), `C` toggles directly between
  `call_any` and cleared — there's no fixed amount to offer, so there's nothing to cycle through. This is the "must
  click C two times" behavior requested: the *first* press is never `call_any` when a fixed call exists, because that
  would silently skip past the more specific, usually-cheaper option a single press already reaches.
- **`A`** → toggle the new `all_in` preselect (below).

Implementation: one new `useEffect` inside `PreselectionControls`, gated the same way `canPreselect`/`selection`
already gate the button row (`ActionBar.tsx:135`), using the shared `isPlainKey`. Each key calls the same
`onSelectAction` the corresponding button's `onClick` already calls — no new state, no duplicated selection logic.

Visual hint: `option()` (`ActionBar.tsx:136-142`) gains a `key?: string` argument, rendered as
`<kbd>{key}</kbd>` inside the existing `<small>` description, same pattern as the main action row's `label()`
helper (`ActionBar.tsx:305-309`).

### New: All In preselect

**Data model / wire** — `"all_in"` becomes a fifth valid `Preselection.Selection` value, alongside the existing four. No
new protobuf field: `Preselection{Selection, Amount, HandID, Stage}` (`tablestore/store.go:75-80`) already carries an
arbitrary string `Selection`.

**Backend** (`api/internal/table/actor.go`):

- `handlePreselect`'s validity check (`actor.go:394-395`) adds `c.Selection != "all_in"` to the allowed list.
- Same as `check_fold`/`fold`/`call_any`, `all_in` carries no client-supplied amount — the existing
  `if c.Selection != "call" { c.Amount = 0 }` (`actor.go:421-423`) already zeroes it for every non-`call` selection,
  `all_in` included, no change needed there.
- `all_in` is **unconditional intent**, like `fold`/`call_any` — it does not care what the facing bet is when it
  resolves, so it needs no entry in the cancel-on-raise pruning loop (`actor.go:853-864`; contrast with the
  `check_fold` fix in `docs/specs/2026-08-21-preselect-checkfold-cancel.md`, which is about `check_fold`
  specifically resolving differently depending on the amount — `all_in` never does).
- New method on `hand.Table` (`api/internal/engine/hand/snapshot.go`, next to `ProspectiveCallAmountForActor`):

  ```go
  // AllInAmountForActor returns the total bet playerID would be shoving to —
  // their current contribution plus their remaining stack — if they went all
  // in right now. The second return is false if playerID has no active round
  // seat to shove from (already folded/all-in, or no betting round open).
  func (t *Table) AllInAmountForActor(playerID string) (int64, bool) {
      if !isBettingStage(t.stage) || t.round == nil {
          return 0, false
      }
      idx, ok := t.roundIdx[playerID]
      if !ok {
          return 0, false
      }
      player := t.round.Players[idx]
      if player.Folded || player.AllIn {
          return 0, false
      }
      return player.Contributed + player.Stack, true
  }
  ```

- `processInlinePreselections`'s switch (`actor.go:1887-1916`) gains:

  ```go
  case "all_in":
      if amt, ok := a.cached.AllInAmountForActor(current); ok {
          action = betting.ActionRaise
          amount = amt
      } else {
          delete(a.activity.Preselections, current)
          continue
      }
  ```

  `applyActAndCommit` → `NormalizedActionForActor` (`hand.go:384-397`, already called for every action) downgrades this
  to `ActionCall` if the player's stack doesn't even cover the current bet, and the underlying betting round already
  treats an under-min raise as a short all-in (`betting.go`'s `goingAllIn` handling) — no new engine logic,
  `all_in` preselection reuses the exact path a live "All In" raise click already goes through.

**Frontend:**

- `ui/src/lib/api/table.ts:33`: `ActionPreselection` union gains `'all_in'`.
- `ui/src/lib/actionPreselection.ts`'s `resolvePreselection` gains, before the existing fallback:
  ```ts
  if (selection === 'all_in') {
    if (legal.has('raise')) return 'raise';
    return legal.has('call') ? 'call' : legal.has('check') ? 'check' : null;
  }
  ```
- `PreselectionControls`'s local `onAct` param type widens from `(action: PokerAction) => boolean` to
  `(action: PokerAction, amount?: number) => boolean` — matches what's actually passed in today (`ActionBar.tsx:351`'s
  `onActAction`, already used elsewhere with an amount via `onRaise`), no parent-type change.
  `PreselectionControls` gains a new `maxRaise: number` prop (threaded from `ActionBar`'s own `maxRaise`,
  `ActionBar.tsx:271`, at the `<PreselectionControls .../>` call site, `ActionBar.tsx:346-351`). The resolution effect
  (`ActionBar.tsx:120-133`) computes `const amount = action === 'raise' ? maxRaise : undefined;` and calls
  `onAct(action, amount)`.
- New button in `PreselectionControls`'s render (`ActionBar.tsx:143-150`):
  `{option('all_in', 'All In', 'Apostar tudo quando chegar sua vez')}` — always offered (no `supportsCallPreselection`
  gate; going all-in doesn't depend on call-preselection support the way `call`/`call_any` do).

## Testing

Backend (`api/internal/table/preselection_test.go`):

- `all_in` preselection resolves to a raise for the player's full stack when it becomes their turn.
- `all_in` preselection resolves to a call (not a raise) when the player's stack doesn't cover the current bet (short
  all-in via call, mirroring `NormalizedActionForActor`'s existing downgrade rule).
- `all_in` is unaffected by another player raising in between — no pruning, unlike `check_fold`.
- `handlePreselect` accepts `"all_in"` as a valid selection; rejects anything else unrecognized, unchanged.

Frontend:

- `actionPreselection.test.ts`: `resolvePreselection('all_in', ...)` returns `'raise'` when raise is legal, `'call'`
  when only call is legal, `null` when neither.
- `ActionBar.test.tsx`: pressing `X`/`F`/`C`/`A` while `canPreselect` selects the matching preselection; pressing `C`
  twice in a row (fixed call available) lands on `call_any`; pressing the same key again after a selection clears it.
  Shortcuts have no effect when `canPreselect` is false or while typing in an input (existing `isTypingTarget`
  guard, reused).
- `Seat.test.tsx`: `1`/`2` toggle `peeked` state only when `peekGated`; no-op otherwise (hand complete, not the viewer,
  or replay view with no `onPeekCards`).

## Out of scope

- Any change to the live (your-turn) action shortcuts (`f`/`c`/`p`) or `RaiseControl`'s `r`/`h`/`a` — those are
  untouched, already documented as the precedent this spec follows.
- A visible "why did my preselection change" toast — same cut as the check/fold cancel spec, no existing precedent for
  surfacing preselection lifecycle events.
- Configurable/rebindable shortcuts.
