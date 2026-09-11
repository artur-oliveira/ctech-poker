# Card-flip stutter on Safari in real play (2026-09-10)

## Symptom

Live-game feedback: "Animação de flip das cartas parece meio travado no safari."
In `npm run dev:mock` the hole-card / board-card reveal flip is smooth; in real
gameplay on Safari the same flip stutters for the first ~100 ms.

## Root cause

The flip is a pure CSS mount animation. `PlayingCard` (`src/components/table/PlayingCard.tsx`)
is keyed `` `${i}-${card}` `` in `Seat` / `Board`, so when a card value goes from
`"back"` to its real face the old node unmounts and a new `.card-reveal` node
mounts, which starts `@keyframes card-back-turn` / `card-front-turn` (a `scaleX`
+ `opacity` "turn") on the two stacked faces and `board-card-reveal` /
`hole-card-reveal` on `.card-reveal-inner`.

That mount happens on exactly the commit that also, synchronously, in the
WebSocket message handler (`useTableRealtimeSession.ts`): decodes the ~150 kB
protobuf `state` frame, runs `reduceTableSnapshot`, `describeSnapshot`,
`playSoundForTransition`, then `setSnapshot`, then React re-renders the whole
felt (`Board` is not memoised; every `Seat` re-renders on a fresh snapshot).

In `dev:mock` there is no frame to decode, so that commit is cheap and the newly
mounted animation gets its first frames on time. In real play on Safari the main
thread is still finishing decode + reconcile + commit when the animation should
be starting. WebKit holds a freshly created CSS animation **pending**
(`startTime: null`, held at time 0) until the rendering loop next ticks, then
starts it late and the compositor plays catch-up — the visible stutter. Same
family as the `seat-join` bug in `docs/2026-09-08-table-polish-bet-presets.md`.

Two aggravators made the late start janky rather than just delayed:

1. **The flip keyframes clobbered the layer promotion.** `.board-card img`
   carries `transform: translateZ(0)` specifically to give each card face its
   own compositing layer (an iOS SVG-blur fix, see the comment there). But
   `card-back-turn` / `card-front-turn` run with `animation-fill-mode: both` and
   every keyframe set `transform: scaleX(...)` — a bare 2-D transform that
   overrides `translateZ(0)` for the whole animation and after it. So during the
   flip the board face had a changing `scaleX` **and** a `filter: drop-shadow()`
   with no stable layer → WebKit re-rasterised the face and its shadow every
   frame. Cheap on an idle thread, dropped frames on a busy one.
2. **No pre-promotion.** Nothing hinted the layer before mount, so the layer had
   to be built at animation start — precisely when the thread was most saturated.

## Fix (`src/app/renderer.css`, CSS only)

- Every frame of `card-back-turn` / `card-front-turn` now carries
  `translateZ(0)` alongside the `scaleX`, so the compositing layer is stable
  across the `both`-filled animation instead of being dropped by it.
- `board-card-reveal` / `hole-card-reveal` use `translate3d(...)` / `translateZ(0)`
  instead of `translate(...)` / `transform: none`, for the same reason (they
  clobbered nothing, but `transform: none` at rest dropped the layer between the
  deal-in and any later transform).
- `.card-reveal .card-back`, `.card-reveal .card-front` and `.card-reveal-inner`
  now declare `will-change: transform, opacity` + `backface-visibility: hidden`,
  so the layer exists before the card mounts and the first rendering-loop tick
  can composite immediately rather than trigger a promotion mid-jank.
- `.static-cards` (finished records: `/hands`, hand-history detail, shared hand)
  resets `will-change: auto` on those elements — no flip is coming there and a
  long virtualised `/hands` list must not pin a compositing layer per card.

No geometry, size, timing, or player-visible behaviour changes — the flip looks
identical, it just composites on one layer. Reduced-motion is unaffected (the
global `prefers-reduced-motion` block still collapses `animation-duration` to
`.01ms`).

### Not changed

- The protobuf decode stays on the main thread; moving it to a worker would
  touch `lib/ws/codec.ts`, the single home of the framing, and is out of scope
  for a motion fix.
- `Board` / `PlayingCard` are still not memoised. `snapshot.board` is a fresh
  array per frame so `memo` would not bite without a `useMemo` keyed on the card
  list; the reveal-frame cost is dominated by the decode, not by re-rendering 5
  cards, so this was left alone.

## Tests

`src/app/card-flip.motion.test.ts` — reads `renderer.css` and asserts the flip
and deal-in keyframes keep a 3-D transform on every frame, that the flip layers
declare `will-change` / `backface-visibility`, and that `.static-cards` releases
the hint. Mirrors the existing `table-reactions.motion.test.ts` pattern.

## Follow-up (same day): reveal never becomes visible

This fix addressed the *stutter*. A second report — blank cards that never
appear at all until F5, on desktop too — was the deeper form of the same trap:
the entrance animation was the card's resting state. Fixed in
`docs/2026-09-10-card-reveal-visibility.md` by gating the deal-in / flip behind
a render-derived `[data-card-revealing]` mark (the pre-promotion `will-change`
and the `card-*-turn` names moved onto that selector) and making the bare
`.card-reveal` face `opacity: 1` at rest. `card-flip.motion.test.ts` was
rewritten to assert the new contract alongside the compositing one here.

## Caveat

Verified against the quality gate (`vitest`, `tsc`, `eslint`, `build`). Not
verified on a real iPhone — WebKit in Playwright is not mobile Safari (no dynamic
viewport, different rasterisation scale). A pass on a real handset is still
recommended before treating this as closed.
