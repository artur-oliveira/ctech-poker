# Table portrait/landscape rework + card-face SVGs (2026-09-06)

Three fixes to the live table surface.

## 1. Card-face SVGs were blurry with a heavy baked shadow on iOS

`scripts/card-templates/*.svg` each carried an internal
`<filter id="a"><feDropShadow .../></filter>` wrapping the whole card in a
`<g filter="url(#a)">`. That SVG filter is what forced iOS Safari to rasterize
the `<img>`-embedded card at a blurry, non-retina resolution (a documented
WebKit quirk — the DPR over-sizing in `PlayingCard.tsx` only half-fixed it),
and its baked drop-shadow was the smeary dark blob under every card.

The `<filter>` and its `<g>` wrapper are removed from every template; the CSS
contact shadow (`.board-card`, `.seat-cards > img.playing-card` in
`renderer.css`) is now a short soft `drop-shadow(0 2px …)` so the card sits on
the felt instead of floating over a blob. `npm run cards:variants` regenerates
`public/svgs/variants/**` from the cleaned templates (committed).

## 2. Portrait: opponents ride the walnut, capsule tightens for small fields

`balancedSeatPosition` no longer has a portrait-specific polyline. Portrait and
the desktop oval now share one even angular spread on the orbit ellipse
(`--table-orbit-*`, the rail band's centreline), so every opponent sits *on the
walnut* with their bet chips on the felt inside it — matching the One Band
Rule. `--table-rail-inset-inline` on `.stage-v` widened (4px → 16px) so a seat
centred on the band keeps its whole avatar on screen.

`.stage-v[data-player-count='2'|'3'|'4'|'5']` deepen the rail's top/bottom
inset so the capsule wraps the seats that are actually there instead of
leaving the lower half a dead green slab (heads-up/3-handed were badly
top-heavy). The band and felt still derive from the inset; the ring keeps
`flex:1` so the viewer HUD stays pinned to the bottom.

## 3. Landscape: two columns — table, then everything you touch

Short landscape (`(max-height: 620px) and (orientation: landscape)`) is a wide,
shallow viewport. `TableStage` keeps the same avatar-ring composition as
portrait/desktop but adds a `stage-h` class, and the CSS makes `.game-table`
`display: contents` so the ring and the viewer's hero seat become direct grid
items of `.game`:

- **Left column** — the oval + the opponent avatar ring. Same `<Seat>`, same
  `balancedSeatPosition`. The viewer is **not** in the ring.
- **Right column** — the viewer's own hole cards / equity (the hero
  `.game-seat.viewer`, row 2) stacked above the action dock (row 3): prepared
  action + Fold/Check/Pagar + collapsed raise UI.
- `today-highlight-wrap` and the keyboard `kbd` hints are hidden (no room, no
  keyboard); `.action-preselectors` wraps to a 2-col grid so it can't blow the
  narrow column out.
- chat / reactions / last-winners / equity-trainer consolidate into the header
  quick-action icons + utility menu, exactly like portrait.

**The rail is a real CSS `border`, not an inset ellipse.** Two `border-radius:
50%` ellipses (rail behind felt) with a fixed-px gap pinch at the diagonals —
invisible on desktop's big mild oval, obvious at this size with nine seats on
it. So in `stage-h` the walnut is `.game-felt`'s own `border: 26px solid
var(--table-rail)` (a constant-width stroke at any `border-radius`), the brown
edge is a `box-shadow` ring outside it, and the separate `.game-rail` element
is `display: none`. `--table-orbit-*` is redefined as the felt inset plus half
the border, so a 24px avatar centred on it stays inside the walnut.

All `stage-h` geometry lives in `renderer.css`. Covered by
`tableComponents.test.tsx` ("short landscape keeps the avatar ring and splits
the viewer into its own column").

Guide: `src/app/(marketing)/guide/table/page.tsx` gains an "A mesa em cada
tela" section describing the three layouts.
