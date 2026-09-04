# One band: rail, felt and seats from the same numbers

**Date:** 2026-09-03
**Scope:** `src/app/base.css`, `src/app/(app)/app.css`, `src/app/renderer.css`,
`src/components/table/{TableStage,Seat}.tsx`, `e2e/tableLayout.spec.ts`,
`CLAUDE.md`, `DESIGN.md`

## What was wrong

The table was three shapes authored independently:

| shape | where | how |
|---|---|---|
| walnut rail | `.game-rail` | `inset: 8% 4% 7%`, and `28px 4px 12px` on the portrait capsule |
| green felt | `.game-felt` | `inset: 12% 8% 11%`, plus a separate `clamp()` triple for `.stage-v`, plus `inset-inline: 18%` for heads-up |
| seat ring | `balancedSeatPosition` | a hand-tuned 9-point polyline in percentages, and `50 + cos*46 / 50 + sin*42` on desktop |

Nothing tied them together, and `inset` percentages resolve against **width** on
the horizontal axis and **height** on the vertical — so "the same number on four
sides" was never the same thickness. Measured on a 390×844 phone:

| occupancy | band top | right | bottom | left |
|---|---|---|---|---|
| 9-max, before | 28px | 26px | **14px** | 26px |
| heads-up, before | 28px | **63px** | **14px** | **63px** |

The heads-up row is the table in the bug report. `inset-inline: 18%` squeezed
the felt inside a full-width rail, so the walnut grew to four and a half times
its bottom thickness on the sides. That is the "geometrically wrong" edge.

## What it is now

One derivation, in `base.css` and `(app)/app.css`:

```
--table-rail-inset-top / -inline / -bottom   place the rail
--table-rail-band                            the walnut's thickness
.game-felt  inset = rail inset + band        (per side, so the band is uniform)
--table-orbit-*  = rail inset + band / 2     the band's centreline
```

The rail's inset stays authored per side on purpose — the portrait capsule is
pushed down inside its ring so the top seats' cards clear the header — but
because the felt is *derived*, the walnut still reads the same thickness the
whole way round. Measured after: **26/26/26/26** at every portrait viewport and
occupancy, 22px on short phones, 32px on the desktop oval. Asserted in the
browser suite, not just measured once.

Seats moved onto that same derivation. `balancedSeatPosition` now returns
normalised `{s, t}` in `[0, 1]` on the orbit box (`s` = 0 at the left orbit line
and 1 at the right, `t` = 0 at the top and 1 at the bottom) instead of
percentages of the stage. `Seat.tsx` publishes them as `--seat-s`/`--seat-t` and
the CSS resolves them against `--table-orbit-*`:

```css
left: calc(var(--table-orbit-inline) + var(--seat-s) * (100% - 2 * var(--table-orbit-inline)));
top:  calc(var(--table-orbit-top) + var(--seat-t) * (100% - var(--table-orbit-top) - var(--table-orbit-bottom)));
```

Every balanced seat centre now lands on the band's centreline to **0px** at
320×568, 390×844 and 1440×900. Change the band and the rail, the felt and every
seat move together — which is the actual answer to "every time I touch the table
something breaks".

Occupancy narrows the **ring**, not the felt:
`.stage-v[data-capacity='2'] .stage-v-ring { max-width: min(82%, 640px) }`. Rail
and felt shrink together and the band is untouched.

## Two traps worth knowing

**`--table-orbit-*` is declared on `.game-table`, not on `:root`.** A custom
property whose value contains `var()` is resolved where it is *declared*. A copy
on `:root` computes against the desktop rail numbers there and inherits down
already frozen, so every `.stage-v` / `.stage-h` override on `.game-table`
itself is ignored. That cost one debugging round: seats were placed with
`calc(4% + 32px / 2)` on a phone whose rail is `4px` with a `26px` band.

**Lengths, not percentages, for the band.** See the Length-Not-Percentage Rule
in `DESIGN.md`. The rail's own inset may stay a percentage where it is *meant*
to scale with one axis; the band never can.

## Verification

`e2e/tableLayout.spec.ts` gained two invariants, run at all six viewports in
Chromium, Firefox and WebKit:

- the rail band's four sides are within 1px of each other and none is zero;
- every balanced seat's centre is within 1px of the band centreline computed
  from its own `--seat-s`/`--seat-t`.

Both fail on the old CSS. Full run: 56/56 in Chromium and Firefox locally,
WebKit in CI; 1475 unit tests, `tsc`, `eslint` and `next build` clean.

**Still needs a real handset.** Playwright's WebKit is a desktop build — it does
not reproduce mobile Safari's dynamic viewport or its flex intrinsic sizing, and
this repo has already shipped a `min-height` change that looked right in the
DevTools device toolbar and squeezed the table on real iOS. Nothing here goes
through a flex cross size and every geometry value is a resolvable length or a
plain percentage, which is the shape that has survived before — but the check is
still owed before merge.
