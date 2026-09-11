# Community / hole cards sometimes never appear (2026-09-10)

## Symptom

Intermittent, production, both desktop (Chrome/Firefox) and Safari; never
reproducible with devtools open; **F5 always fixes it**. Users suspected "a
network hiccup at the moment the cards should appear."

1. The community/board cards don't render — the felt space where the board
   goes stays empty.
2. The viewer's own hole cards at showdown don't render — an empty transparent
   space where the cards should be.

## Root cause

A revealed card's visibility depended on things that can each fail *after* the
node is in the DOM, with **no visible resting fallback for either**:

### 1. The entrance animation was the card's resting state

`PlayingCard` renders a `.card-reveal` node whose faces animate in:
`board-card-reveal` / `hole-card-reveal` on `.card-reveal-inner` and
`card-front-turn` on `.card-front`. Every one starts at `opacity: 0` and ran
with `animation-fill-mode: both`, so **until the animation actually advances the
card is invisible**.

That animation is created on exactly the commit that also, synchronously in the
WebSocket handler (`useTableRealtimeSession`), decodes the ~150 kB protobuf
`state` frame, runs `reduceTableSnapshot`, and re-renders the whole felt. WebKit
leaves a CSS animation created on a saturated frame **`pending`**
(`startTime: null`, held at time 0) until the rendering loop next ticks — and
if that stalls, `both` keeps the card at `opacity: 0` indefinitely. This is the
exact trap already documented for `seat-join`
(`docs/2026-09-08-table-polish-bet-presets.md`) and for the compositing stutter
(`docs/2026-09-10-card-flip-safari-compositing.md`). It is worst on WebKit but
the same shape (`opacity: 0` held by an animation that hasn't run) can bite any
engine.

### 2. The face image was lazy-loaded, with no error fallback

`.card-front` was a `next/image` with the default `loading="lazy"`. Its `src`
was only fetched on an `IntersectionObserver` tick, which the same saturated
reveal frame (and re-key remounts across streets) could delay. And neither face
had an `onError` handler, so a failed SVG fetch on a real network blip left a
permanent transparent gap — no retry, no fallback. This is engine-agnostic and
matches "F5 fixes it" (fresh load, warm cache).

The snapshot/state layer was ruled out: `reduceTableSnapshot` preserves poker
state (board, seats, hole cards) byte-for-byte and only rejects *regressive*
versions — a fresh frame always carries the whole board.

## Fix

The choke point is `PlayingCard` and its CSS in `src/app/renderer.css`.

### Resting state is unconditionally visible

- `renderer.css`: the resting `.card-reveal .card-front` is `opacity: 1`,
  `.card-back` is `opacity: 0` (the *finished* flip). The deal-in / flip
  animations and their `will-change` now attach **only** through a
  `[data-card-revealing]` attribute selector — never a bare `.card-reveal`
  node. A card that mounts already face-up, or whose animation an engine
  stranded, or under `prefers-reduced-motion`, wears its stylesheet's resting
  (visible) state.
- `PlayingCard` takes a `revealing` prop and sets `data-card-revealing` for the
  one render span after a card turns face-up.
- `useEnteredKeys` (`src/lib/hooks/useEnteredKeys.ts`) — the generalisation of
  `useJoinedSeats` — derives "just entered" **during render** (not in an
  effect, which paints one resting frame first) and drops the mark after a
  bounded `holdMs`. `Board` keys it by board slot, `Seat` by hole-card slot;
  `useJoinedSeats` now delegates to it. `CARD_REVEAL_MS` (1500) caps how long a
  stalled entrance can look wrong — never how long the card is invisible.
- `Board` / `Seat` now key `PlayingCard` by **slot** (`key={i}`), not by value,
  so a hole-card node persists across its own back→face transition.

### Image robustness

- `.card-front` (and the `revealable-card` face) load `eager` — a card being
  revealed is on-screen and wanted now; the lazy `IntersectionObserver` path is
  wrong for it.
- `onError` on `.card-front` sets `.card-reveal.face-fallback`, which shows the
  card **back** instead of a transparent hole; it clears when the slot goes
  face-down again so the next hand retries.

## Consequences (intentional)

- `/poker-rules` hand-ranking examples and the `/hands/history` final board (a
  `.static-cards` context) fade in instead of flipping — they are illustrative
  / finished records, not a deal, and per doctrine an "already dealt" card must
  not depend on an animation to be visible. No guide copy or screenshot
  documents the flip itself, so no guide change.
- Replay *forward* playback still flips each street as it's dealt; a scrubber
  jump that reveals several streets at once no longer flips them all
  simultaneously.

## Tests

- `src/lib/hooks/useEnteredKeys.test.ts` — first-render keys never mark; only
  added keys mark; mark drops after `holdMs`; reset survives.
- `src/components/table/tableComponents.test.tsx` — Board flips only the slot
  dealt since the last render, not a re-entered full board; the mark drops
  after the window and the card stays present; a failed face fetch falls back
  to the back.
- `src/app/card-flip.motion.test.ts` — rewritten: the resting `.card-front` is
  `opacity: 1`, the flip keyframes are named only under `[data-card-revealing]`,
  `CARD_REVEAL_MS` covers the longest board reveal + stagger, plus the existing
  Safari compositing contract.

## Real-engine follow-up

`npm run e2e` (`e2e/tableLayout.spec.ts`) exercises the table across engines;
worth a manual pass confirming a live flop/turn/river still flips and that a
re-entered table shows a full board instantly. Not verified on a real handset.
