# 2026-09-29 — Reaction impacts are materials, not particle clouds

`cry`, `tear` and `chip` were the three reactions that read as decoration while
`knife`, `turtle`, `tomato` and `poop` read as events. This pass rebuilt the three
to the grammar the good ones already shared. Nothing about the catalogue, the
targeting flow, the cooldown, the mute toggle or the wire changed — this is
presentation only, in `src/app/(app)/table-reactions.css` plus the markup
`TableReactions.tsx` hands it — and, for the chip, in the table-wide `.chip` in
`src/app/renderer.css`.

## What was wrong

- **`cry`** spread five flat blue lozenges *horizontally* beside the 😭
  (`reaction-tear-curtain` offset each piece by `(--piece - 2) * 15px`). They never
  left the eyes, never accelerated, and never landed: a row of blue pills drifting
  sideways.
- **`tear`** was one 34×44 blue blob with a 5px vertical bar under it
  (`reaction-single-tear` + `reaction-tear-streak`). On the felt it read as a
  balloon on a stick.
- **`chip`** threw twelve three-high stacks of `.chip` — 2px *dashed* gold-ink
  border, flat gold radial — over a filled `--gold-pale` radial flash. At 22px the
  dashes read as scalloped fuzz, the stacks overlapped into one amorphous mass, and
  the flash smeared gold over all of it. This is the "yellow strange effect".

## The grammar the good ones share

A solid silhouette, **one decisive physical event**, gravity, and irregular sizing.
`knife` is the reference: a flash at the wound, then eight drips of eight different
lengths released on eight different delays, falling. Nothing about it is a particle
system; it is a material behaving.

## What the three do now

**`cry`** — one shared water material (`.reaction-impact-cry i`,
`.reaction-impact-tear::after`, `.reaction-impact-tear .reaction-impact-particles i`):
a drop pointed at the *top* (`border-radius: 50% / 100% 100% 58% 58%` — the trailing
edge of something falling), a specular catchlight up and left, a shaded underside.
Odd pieces well up at the left eye, even at the right (`--eye`), and
`reaction-eye-tear` runs the real sequence: swell while surface tension holds → neck
off → stretch under acceleration (scale is anisotropic throughout; a drop that keeps
its aspect ratio while falling is a bead) → flatten on landing. Two offset ripple
rings below mark where they land.

**`tear`** — the thrown drop breaks on the target. Three parts of one impact: the
wet ring spreading outward (`::before`), the **rebound jet** that leaps back out of
the crater, pinches off into a bead and falls (`::after`, `reaction-splash-jet`),
and eight crown droplets on ballistic arcs (`reaction-splash-arc`, per-`nth-child`
apex and landing points). The jet is the single read that makes an impact look like
water rather than like a shape appearing.

**`chip`** — a chip is a disc with a milled rim, not a gold dot. Five background
layers on one element, all `closest-side` so the percentages resolve against the
chip's own radius and not against `farthest-corner`:

1. a small specular catchlight at 36% 28%;
2. an outer rim ring from 88%, which insets the spots so they never touch the edge
   (spots reaching the edge read as cog teeth, which is the failure the dashed
   border already had);
3. the face disc out to 71%, ending in a dark inner ring;
4. `repeating-conic-gradient` — six `--paper` edge spots — over a `--gold-deep` →
   `--gold-ink` rim.

This is the table-wide `.chip` in `renderer.css`, so the pot, every seat bet and
every thrown chip are the same object — the change started scoped to the reaction
stacks and was promoted, because a burst of real chips landing next to the old
dashed-gold blobs read worse than either did alone. `--chip-face` is the one thing
a value tier repaints; the rim, the spots and the bottom edge are what still say
"chip" at 15px, so tier never touches the anatomy.

The burst is twelve single chips (every fourth a pair, for irregularity) that leave on their own bearing, hop, land flat — `scale(_, .82)`
is a disc seen from above — and settle, each carrying a `drop-shadow` so it sits
*on* the felt. The projectile in flight is one chip, not a column. The gold radial
flash is gone; the impact is now one crisp `--gold-pale` rim that expands and dies.

## Constraints kept

- Tokens only. No new colours, no library, no filter, no canvas.
- Every value is in the impact box's own space; the CSS owns all timing.
- `prefers-reduced-motion` is unchanged and still covers this: the block in
  `table-reactions.css` sets `.reaction-impact { display: none }` and lands the
  projectile at rest, and everything added here lives inside `.reaction-impact`.
- The render budget is untouched: the burst is still twelve nodes, and no new
  props cross the `TableReactions` memo boundary.

## Reach of the chip change

`.chip` is one rule with one geometry, so promoting the anatomy to it repaints every
surface that already rendered a chip — the table's pot and seat bets, plus `/hands`,
`/hands/replay`, `/share`, `/profile` and `/lobby` — with no layout change anywhere:
identical box, identical stacking, identical `chip-drop`. The landing page's
`.chip-orbit`/`.chip-a`/`.chip-b` decorations are separate classes and are untouched.

Two dead rules went with it: `.reaction-jackpot-stack .chip { border-width: 2px }`
(the chip has no border any more) and the `reaction-chip-pop` keyframe, replaced by
`reaction-chip-scatter`.

`npm run og:capture` was re-run for both sets, since a chip appears in most of them.
Several baselines turned out to be stale for unrelated reasons as well — `og/table`
still showed the pre-preset `Mín`/`Máx` raise row, `2.775 fichas` instead of
`2,7K fichas`, and the old "Maior pote de hoje" ticker — so the new captures correct
those too.

One capture was deliberately **not** taken: `public/og/poker-rules.webp`.
`(marketing)/poker-rules/page.tsx` renders `AppPageChrome` but, unlike
`(marketing)/page.tsx` and `(marketing)/guide/layout.tsx`, never imports
`renderer.css` — so `.app-tab-bar { display: none }` does not apply and the mobile
tab bar renders as an unstyled block over the top of the page. It reproduces in the
browser with these changes stashed, so it predates them; re-capturing would only bake
the defect into the docs. Adding the import is one line but changes that whole route's
cascade, which is a design call, not a side effect of this pass.
