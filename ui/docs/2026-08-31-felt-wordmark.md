# Felt wordmark on the live table

**Date:** 2026-08-31
**Superseded in part:** placement, colour and the `#felt-weave` filter were
reworked on 2026-09-01 — see `docs/2026-09-01-table-felt-and-aside-polish.md`.
**Scope:** `src/components/table/TableStage.tsx`, `src/app/globals.css`, `DESIGN.md`

## What changed

The live table felt now carries the house mark — the `{PokerLogo} CTECH` lockup
already shown on the landing hero's table preview (`.table-logo` in
`globals.css`). It renders once per stage, above the community cards in the calm
top arc of the felt, on both the desktop/landscape oval and the portrait
`stage-v` capsule. Replay (`hands/replay`), which reuses `TableStage`, shows it
too, for the same felt identity.

## How it's built

- **Component:** `FeltWordmark` in `TableStage.tsx`, rendered inside
  `feltContent` so it appears in every branch (`waiting_for_players` through
  `complete`). It is `aria-hidden` — purely decorative; the accessible table
  name stays on the page's `h1` (`STAGE_LABELS`).
- **Direction — "woven thread".** The `CTECH` lettering is tone-on-tone
  (`color-mix(in srgb, var(--felt-text) 17%, transparent)` with a
  `felt-shadow` relief); the P monogram keeps its oxblood at `opacity: .62`,
  matching the landing hero's `.table-logo`. An inline SVG filter
  (`#felt-weave`: `feTurbulence` + `feDisplacementMap`, fixed `seed="7"`,
  `scale="0.7"`) frays the glyph and logo edges into the felt weave.
- **Why the lettering is tone-on-tone, not gold:** DESIGN.md's Three Materials
  Rule reserves gold for value, time, and earned outcomes. A gold watermark
  would be generic gold decoration on the play surface, so the text stays
  felt-coloured and the turbulence carries its character.
- **Placement:** `top: 14%` of the felt — the narrow clear band between the
  top-centre opponent seat (~10%) and the pot readout (which rides as high as
  ~17% at `complete`). A transient `.table-callout` (z 2) can briefly pass
  over it; fine for an aria-hidden mark.
- **`prefers-contrast: more`:** the filter is dropped and the mark switches to
  `--felt-text-muted` at higher logo opacity for a clean, legible lockup.
- **Motion:** none. `seed` is fixed, so the fray is identical across renders and
  needs no `prefers-reduced-motion` branch.
- **Layering:** `z-index: 0`, behind `.board` (`z-index: 1`) and the transient
  `.table-callout` (`z-index: 2`); it never obscures cards or the dealer call.

## Tests

`src/components/table/tableComponents.test.tsx` — "weaves the decorative house
mark into the felt on both stages": asserts the `CTECH` lockup, `aria-hidden`,
the fixed turbulence seed, and presence inside `.game-felt` on the desktop oval
and inside `.stage-v-ring .game-felt` on the portrait ring.
