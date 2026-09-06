# Mobile hand-outcome sheet + portrait seat-card clearance

## Problem

On portrait handhelds the `HandOutcomeBanner`'s full card rendered where its
base rule puts it — `place-items: center` inside `.hand-outcome` (which is
`position: absolute; inset: 0` over `.stage-v-ring`). At ~390px that is a
dialog-sized card floating dead centre of the ring: it buried the board, the
pot readout and the bottom seats, and because it only covers part of the ring
the surrounding seats bled past its edges and it read as a rendering glitch
rather than a deliberate layer. The desktop layout never had this because
`@media (min-width: 1000px)` anchors the same card to `end end` (bottom-right),
clear of every seat box.

Separately, opponents' revealed hole cards in the portrait ring
(`.stage-v-ring .game-seat .seat-cards`, floated onto the rail band above or
below the avatar with only a 3px gap) sat on top of the seat's own corner
badges — most visibly the D/SB/BB pill (`.seat-role`, top-left) was half
covered by a hole card at showdown.

## Change (CSS only, `renderer.css`)

- Inside `@media (orientation: portrait) and (max-width: 1023px)` — the exact
  match for `TableStage`'s `VERTICAL_STAGE_QUERY` — the full outcome card
  (`.stage-v .hand-outcome:has(.hand-outcome-card)`) now docks as an opaque
  sheet at the foot of the ring: `place-items: end stretch`, full width, top
  corners rounded, `max-height: 62%`, `padding-bottom` honouring
  `env(safe-area-inset-bottom)`, and `z-index: var(--z-notice)` so it clears
  the bottom seats and the winner seat's raised stack. The decorative
  `.hand-outcome::before` spotlight is dropped in this state. The collapsed
  badge / standalone-ring keep their existing top-left dodge — only the full
  card becomes a sheet, so `:has(.hand-outcome-card)` scopes it.
- `.stage-v-ring .game-seat .seat-cards` upward float moved from
  `bottom: calc(100% - 3px)` to `calc(100% + 5px)`, and the top-arc /
  seat-4/5 downward float from `top: calc(100% - 3px)` to `calc(100% + 5px)`,
  so hole cards clear the seat's corner badges. The short-portrait
  (`max-height: 760px`) override that deliberately pushes cards further onto
  the seat (`- 12px`, to dodge a worse caption/neighbour-card collision) is
  unchanged and still wins the cascade there.

No JS/TS change; `HandOutcomeBanner` and `Seat` are untouched. Desktop and
compact-landscape (`stage-h`) layouts are unaffected.

## Guide

`guide/table` §"O resultado da mão" gains one sentence describing the mobile
sheet + the X-to-collapse affordance. The section screenshot
(`table-showdown.webp`) is captured at 1280×800 (desktop), which this change
does not touch, so no re-capture.
