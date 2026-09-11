# Hand-outcome banner: stray horizontal scrollbar on a long loss

## Problem (reported from a live game)

On desktop, the loss variant of `HandOutcomeBanner` showed a horizontal
scrollbar **on the card itself** in hands where the outcome text ran long — a
long rival name, an unknown/long hand-category token, next to a wide five-card
showdown row.

## Root cause

`.hand-outcome-card` (`(app)/app.css`) set only `overflow-y: auto` (for the
`max-height` scroll on tall cards). Per CSS, when one axis is a scrolling value
the other's `visible` computes to `auto` — so the card silently had
`overflow-x: auto` and produced a horizontal scrollbar the moment any inner
content exceeded its content box.

Two things could exceed it on a loss:

- `.hand-outcome-comparison-row` used `grid-template-columns: minmax(92px, 1fr) auto`
  (and `minmax(74px, 1fr)` in the compact media block). The 92px floor on the
  name column plus the intrinsic width of a 5-card `.hand-outcome-cards` row
  summed past the 410px `.hand-outcome-card.lose` (`renderer.css`).
- `.hand-outcome-hand-name strong` (the category label) had no wrap control, so
  an unbroken token (a category key the client has no label for) pushed the
  column wide.

## Fix (CSS only, `(app)/app.css`)

- `.hand-outcome-card`: `overflow-y: auto` → `overflow: clip auto` — the
  x-axis can no longer scroll. It computes to `hidden` where `clip` isn't
  allowed next to a scroll axis; either way, no scrollbar.
- `.hand-outcome-comparison-row`: both column templates → `minmax(0, 1fr) auto`.
  The name column yields space to the cards instead of forcing a floor.
- `.hand-outcome-cards`: `flex-wrap: wrap` + `max-width: 100%` so a wide card
  row wraps rather than stretching its track.
- `.hand-outcome-hand-name strong`: `overflow-wrap: anywhere`.

Nothing is clipped in practice — the content wraps within the card. The change
is inert for short names (the common case) and for win/tie/mixed/fold, which
share the same row/card rules and benefit from the same wrapping.

`renderer.css`'s portrait sheet override (`width: 100%`, `max-width: none`) is
unchanged and still inherits the new `overflow`.

## Tests

- `src/components/table/HandOutcome.test.tsx` — new case: a long rival name on a
  loss stays inside the truncating `.hand-outcome-hand-name small` slot and the
  comparison renders without a width-forcing wrapper. (jsdom has no layout
  engine; this pins the structure the CSS relies on.)
- `e2e/tableLayout.spec.ts` — new `hand-outcome loss banner` describe: at
  390/1280/1440 widths, loads `?scenario=complete_loss`, forces a long name and
  an unknown category token into the winner row, and asserts the comparison row
  does not overflow the card, the card's computed `overflow-x` is not a
  scrolling value, and the page never scrolls sideways. Green in Chromium and
  Firefox (WebKit could not launch in the local sandbox — missing system lib;
  runs in CI).

## Guide

No guide change. The panel's content and behaviour are unchanged — this only
removes a scrollbar in an edge case. `guide/table` §"O resultado da mão" and
its `table-showdown.webp` screenshot (1280×800, short fixture names) are
unaffected, so no re-capture.
