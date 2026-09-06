# Action bar — raise sizing polish (2026-09-05)

Visual pass over `RaiseControl` / `QuickPresetRow` in `src/components/table/ActionBar.tsx`.
No behaviour, wire, or copy change — the favourite lifecycle, keyboard shortcuts, clamp
dedup and preselection are untouched.

## What changed

- **Favourite affordance is a corner badge, not a segmented button.** #341/#362 rendered a
  full-height star button welded to the right of every quick-preset pill, doubling the item
  count and leaving the row a wall of outlined stars. The star is now a small absolutely-
  positioned toggle in each pill's top-right corner (`.bet-quick-preset-favorite`, scoped
  under `.action-bar` so it beats the base `.action-bar button` box model). It sits at
  `opacity: .4` (`.5` on coarse pointers, where there is no hover to reveal it) and goes
  full-opacity gold on hover / `:focus-within` / once pinned; a pinned pill also gets a gold
  border + label via `[data-pinned]`. `aria-pressed` and the `Marcar/Remover … dos favoritos`
  labels are unchanged, so the guide copy ("toque na estrela…") still holds.
- **Quick-preset pills** are pill-shaped (`--rounded-pill`), `white-space: nowrap` (kills the
  "All-\nin" wrap on narrow phones), with right padding reserving the badge corner.
- **The server ⅓/½/⅔ row** (`.bet-presets`) now matches: `--rounded-pill`, 40px min-height, so
  the two preset rows read as one cluster instead of two mismatched bands.
- **`.bet-control` is a contained panel** — 1px border, `--rounded-control`, faint
  `surface-control` tint — so the presets, slider, stepper and total read as one control
  rather than fragments floating in the middle grid column.
- **Desktop `.action-bar` grid**: middle track widened to `minmax(268px, 380px)` (quick
  presets fit on one line) and `align-items: end` so Fold/Check/Pagar and Aumentar sit on the
  same baseline as the slider instead of centring against the tall sizing column.
- **Portrait stepper** (`−  TOTAL  +`) is centre-grouped instead of spread edge-to-edge.

## Known redundancy (not addressed here)

`.bet-presets` (Mín / ½ pote / Pote / Máx) and `QuickPresetRow` (¼ / ½ / ¾ / 1× / All-in)
overlap heavily — ½ pote ≈ ½, Pote ≈ 1×, Máx ≈ All-in. Only ¼, ¾ and favouriting are
genuinely additive. Collapsing the two into one row is a behaviour/guide/test change, out of
scope for a polish pass.

## Screenshots

`npm run og:capture -- --guide` re-captured `public/guide/table-preflop.webp` and
`table-flop.webp` (the two guide shots where the sizing UI is on screen).
