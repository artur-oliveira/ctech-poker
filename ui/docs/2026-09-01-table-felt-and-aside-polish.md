# Table polish: felt mark, street rail, docked asides

**Date:** 2026-09-01
**Scope:** `src/components/table/{TableStage,Chat,TableReactions,LastWinners}.tsx`,
`src/lib/hooks/useHoverPanel.ts`, `src/app/table/page.tsx`, `src/app/globals.css`

Five reported defects on the live table, all of them collisions or asymmetries
that only show up in play. Supersedes the placement half of
`docs/2026-08-31-felt-wordmark.md`.

## 1. The house mark is the felt's mark again

`FeltWordmark` was tone-on-tone (`--felt-text` at 17%) behind an SVG turbulence
filter, sitting at `top: 14%` — the same band the dealer call
(`.table-callout`, `top: 13%`) occupies, so **every** announced action crossed
it. Two changes, one cause:

- The mark moved up into its own band (`top: 8%`, `5%` on the portrait
  capsule) and the callout moved down to `top: 22%`. The felt now reads as
  three stacked bands — mark, dealer call, board + street rail — and nothing
  crosses anything.
- The lockup is now a real lockup rather than a watermark: a 26px monogram and
  `CTECH` in `--on-brand` (white), both sized off the local
  `--felt-wordmark-mark` token so the portrait capsule scales the whole thing
  from one number. The `#felt-weave` turbulence filter, the `.felt-fx-defs`
  SVG and the `prefers-contrast: more` override are **deleted** — they existed
  to keep a tone-on-tone mark from disappearing, and a white mark has no such
  problem. `z-index` moved 0 → 1; the trailing letter-space is pulled back
  (`margin-right: -.28em`) so the lockup optically centres on the felt axis.

Gold is still reserved for value (DESIGN.md's Three Materials Rule): the mark
is white and the monogram keeps its oxblood.

**Known limit:** the two-board run-it-twice layout is the one board tall enough
to claim the felt's full height. Below `820px` of viewport height it reaches
into the mark's band, so `.game-felt:has(.board-runouts) .felt-wordmark` fades
out — the decoration yields, the board does not.

## 2. The street rail travels with the board

`.street-progress` was pinned to `bottom: 9%` of the felt. The bottom-row seats
(`seat-0`, `seat-1`, `seat-7`, `seat-8`) float their bet chip *up* toward the
pot, and that lane runs straight through 9% — so the viewer's own raise covered
the street rail.

`TableStage` now wraps the board and the rail in one `.felt-center` box. The
box is what `.game-felt`'s `place-items: center` centres, and the rail hangs off
it (`top: calc(100% + 14px)`), so the rail is always the same distance below the
last board row and never re-enters the chip lane — including on the taller
run-it-twice board, where the old fixed percentage was furthest wrong.

## 3–4. The two docked asides behave the same way

`.game-chat` floated its toggle (`float: right`) while `.table-reactions` used a
`column-reverse` flex stack. Same hover, opposite motion: the reactions button
stayed at the bottom and grew its panel upward, the chat button was **pushed to
the top** by its own opening panel. `.game-chat` now uses the same stack
(`column-reverse` + `gap`, `column` in the portrait rail where the panel drops
below the toggle), and the float/`margin-bottom` rules are gone.

Closing was also knife-edged: `onMouseLeave` closed the panel the instant the
pointer crossed the toggle's 45px circle, which the natural diagonal toward the
panel does. The new `useHoverPanel` hook (`src/lib/hooks/useHoverPanel.ts`)
replaces the inline `isHoverCapable() && onOpenChange(...)` handlers in all
three docked asides — chat, reactions and last winners — and gives the close a
`HOVER_PANEL_CLOSE_DELAY_MS` (320ms) grace period that re-entering cancels. Its
CSS half is `.table-aside-skirt.open::before`, an out-of-flow invisible skirt
(`inset: -26px -20px -20px -26px`) that widens the aside's own hit area so the
pointer has somewhere to travel. Reactions keep passing `!pendingReaction` as
the hook's `enabled` flag, so a targeted reaction still freezes the aside.

The grace period only works because the shared panel slot is now owned. All the
asides read one `activeTablePanel` state, and the close is deferred, so crossing
the reactions toggle on the way to chat used to open reactions, open chat, and
then let reactions' 320ms close fire *into the chat slot* and shut it again.
`panelOpenChange(panel)` in `page.tsx` is what every aside's
`onOpenChangeAction` goes through: a close only clears the slot when that panel
still holds it (`current === panel`). Any future panel added to the slot must go
through it, not a bare `setActiveTablePanel(open ? 'x' : null)`.

## 5. Keyboard shortcuts

`E` opens/closes reactions, `T` opens/closes chat, matching the action bar's
own single-letter vocabulary (`f`/`c`/`p`/`a`/`h`/`r`, `1`/`2` to peek). The
listener lives in `TableContent` beside `activeTablePanel`, so the panels stay
mutually exclusive, and it is gated on `isPlainKey` — typing "e" or "t" into the
chat input (which `T` focuses on open) never moves a panel. Both toggles and
both header quick-actions carry `aria-keyshortcuts`.

## Tests

- `tableComponents.test.tsx` — the mark on both stages, and the board + street
  rail sharing `.felt-center`.
- `TableReactions.test.tsx` — the grace period, that re-entering cancels the
  pending close, and that a pending targeted reaction disables hover entirely.
- `page.test.tsx` — `E`/`T` toggle their panels, and do nothing while a text
  field holds focus; a stale close from an aside that no longer owns the slot
  leaves the newly opened aside alone.
