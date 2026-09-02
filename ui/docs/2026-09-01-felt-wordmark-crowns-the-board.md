# The felt wordmark moves down to crown the board

**Date:** 2026-09-01
**Scope:** `src/components/table/TableStage.tsx`, `src/app/globals.css`, `DESIGN.md`
**Supersedes:** the placement decision in
`docs/2026-09-01-table-felt-and-aside-polish.md` §1 (`top: 8%` / `5%`), which in
turn superseded `docs/2026-08-31-felt-wordmark.md`.

## The defect

The mark owned the felt's top arc. That arc is not empty: the top-row seats
float their bet chip *down* off the seat toward the pot, and that lane runs
straight through 8%. Every raise from an upper seat covered the lockup — the
same class of collision that had already pushed the street rail off the felt's
bottom edge (`docs/2026-09-01-table-felt-and-aside-polish.md` §2), on the
opposite side of the table.

The felt has exactly one region no chip travels through: the centre, where the
chips *stop*. That is where the pot readout already lives.

## The fix

`FeltWordmark` moved inside `.felt-center` (ahead of `<Board>`), and
`.felt-wordmark` swapped `top: 8%` for `bottom: calc(100% + 10px)` — still
absolute, so the lockup adds no height to the vertically centred board group,
but now anchored to the board rather than to a percentage of the felt. It sits
directly above `POTE`, reading as one mark-and-value stack, and it tracks the
board through every stage instead of drifting relative to it.

The portrait capsule keeps its own size token (`--felt-wordmark-mark: 19px`) and
uses a tighter `bottom: calc(100% + 7px)`.

Unchanged: the white-on-oxblood lockup, the reserved gold rule, `z-index: 1`
under the transient `.table-callout` (z 2), and the `max-height: 820px` fade for
the two-board run-it-twice layout — that board is still the one tall enough to
reach the mark, and the decoration still yields to it.

## Known limit

`.stage-v:is([data-player-count='2'|'4'|'6'|'7'|'8']) .felt-wordmark` still sets
`display: none`. Its stated reason — the top-centre seat lane on even
occupancies — no longer applies now that the mark is nowhere near the capsule's
top arc, so it can be dropped to restore the mark on those occupancies. Left in
place here only because portrait seat placement was being changed in parallel.

## Tests

`src/components/table/tableComponents.test.tsx` — "flies the house mark on the
felt on both stages" covers the lockup, `aria-hidden`, and its presence inside
`.game-felt` (desktop) and `.stage-v-ring .game-felt` (portrait); the mark is
now a `.felt-center` child in both.
