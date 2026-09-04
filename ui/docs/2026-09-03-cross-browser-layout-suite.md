# A layout gate that runs in real engines

**Date:** 2026-09-03
**Scope:** `playwright.config.ts`, `e2e/tableLayout.spec.ts`, `package.json`,
`src/app/base.css`, `src/app/renderer.css`, `src/app/(app)/app.css`,
`.github/workflows/frontend.yml`, `CLAUDE.md`, `DESIGN.md`

## The gap this closes

The unit suite holds 90% on lines, functions, statements and branches, and it
still let a player report a streak badge sliced in half. It always will: vitest
runs in jsdom, which has no layout engine. `getBoundingClientRect()` returns
zeros there, `overflow: hidden` clips nothing, and no engine difference exists
to observe. Every defect in this round was invisible to it:

| reported | what it actually was | jsdom could see it? |
|---|---|---|
| streak badge cut on iPhone | badge hangs 8px past a clipping ancestor | no |
| emoji panel dead on Firefox | mouseenter/click commit order differs by engine | no |
| table looks wrong at 320px | caption anchored 10px outside a 12px padding | no |

Raising the coverage threshold would not have caught one of them. So the gate
that was missing is not more coverage — it is a suite that runs where layout
exists.

## What it asserts

`e2e/tableLayout.spec.ts` runs the mock table across six viewports (320×568,
390×844, 430×932, 768×1024, 1280×800, 1440×900) and four scenarios
(`heads_up`, `flop`, `nine_max`, `showdown`), in Chromium, Firefox and WebKit.
That is deliberately assertion-based rather than pixel-baseline: numbers do not
churn on a font-rendering change, they fail with the offending selector and the
number of pixels, and they need no committed golden images.

1. **Nothing a player needs is clipped.** For every seat, badge, card, caption
   and bet chip, the suite resolves that element's *nearest clipping ancestor*
   (rather than assuming one — the portrait stage clips, the desktop oval lets
   seats sit outside the felt's box on purpose) and fails if the element crosses
   it by more than a rounding pixel.
2. **The page never scrolls sideways** at any of those viewports.
3. **The streak badge fits**, asserted with the badge forced into the DOM
   because no mock fixture carries a streak. This is the reported bug, pinned.
4. **The reactions toggle opens on the first click and stays open**, and still
   closes when the pointer leaves. This is the Firefox bug from
   `2026-09-03-aside-toggle-firefox-and-mock-liveness.md`, pinned in the engine
   that had it.

`npm run e2e` locally (Playwright starts `dev:mock` itself — mock mode needs no
API, no fixtures and no auth). `npm run e2e:report` opens the HTML report. CI
runs it as the `browsers` job in `.github/workflows/frontend.yml`, and `deploy`
now needs it.

## WebKit is not an iPhone

This has already cost this project one bad merge: a `min-height` change that
looked right in DevTools' device toolbar squeezed the real table on iOS Safari
and Chrome. The CSS still carries the scar — see the comment on `.stage-v-ring`
about the flex cross-size transfer that "some mobile engines silently refuse …
never reproduced in desktop DevTools' emulation, same engine as the desktop that
authored it".

Playwright's WebKit is closer than Chromium's device emulation, but it is still
a desktop build. It does not reproduce mobile Safari's dynamic viewport (the
collapsing toolbar), its rasterisation scale, or its flex intrinsic sizing. So:

- a green `browsers` run is **necessary, not sufficient**;
- any change to table geometry still needs one pass on a real handset before it
  ships;
- prefer CSS whose value is resolvable without intrinsic-size feedback. A length
  token beats a percentage that resolves against a different axis per side;
  both beat an `aspect-ratio` that has to travel through a flex cross size.

## The three fixes this round

All three are the same shape: a literal that was tuned against one viewport,
replaced by a token that the rules around it are written in terms of.

**`--seat-badge-overhang: 8px`.** Seat corner badges (`.seat-role`,
`.seat-streak`, `.seat-timebank-badge`) hang 8px outside the seat box. The
portrait stage's `padding` reserved `0` at the bottom, and the docked viewer
seat is its last flex child, so the streak badge rendered 8px past
`.game-table.stage-v`'s `overflow: hidden` edge and was cut in half — exactly
what was reported on an iPhone 16e. Both stage-v padding declarations (the base
one in `renderer.css` and the narrow tier in `(app)/app.css`) now reserve the
token. The ring's `flex: 1` absorbs the difference, so no felt height is lost.

**`--seat-caption-lane: 64px`** (56px below 360px). The caption under each
portrait avatar was sized by three separate literals (64/60/54px) that had to
move together and did not.

**The mid-side caption hang is the overhang token.** `.seat-2`/`.seat-7`'s
captions deliberately sit outside their seat box, at a hardcoded `-10px`,
against a stage that reserves 12px — 2px of caption fell off the clip box at
320px. It is now `calc(var(--seat-badge-overhang) * -1)`, which is by
construction the room the stage actually reserves.

No guide update: none of this changes a control, a label, a state or a default.
The streak badge is already documented in `guide/table` ("Sequência"); it simply
renders whole now.

## Known gap

The rail and the felt are still two independently-authored ellipses
(`.game-rail` at `inset: 8% 4% 7%`, `.game-felt` at `inset: 12% 8% 11%`, plus a
separate `clamp()` triple for `.stage-v`), and percentage insets resolve against
width on the horizontal axis and height on the vertical. On a 390×844 phone that
leaves the walnut band 15px at the sides, 8px at the top and **0px at the
bottom**, where the two ellipses become tangent — the "geometrically wrong"
edge in the report. The seat ring is a third, hand-tuned polyline, unrelated to
either.

Fixing that means one concentric rail/felt/orbit derived from a single band
token, which is a larger change than this one and needs the real-handset pass
above. This suite is the safety net it should be done against.
