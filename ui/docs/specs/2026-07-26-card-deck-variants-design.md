# Card deck color variants — design

## Goal
Componentize card rendering so the suit color scheme is a variant, not baked
into a single static SVG set. Backend will eventually pick the variant per
player via `/v1.0/players/me`; frontend must be ready to consume that field
today even though the backend doesn't send it yet.

## Variants
Defined in `src/lib/cardVariants.ts`, `Record<DeckVariantId, {label, colors: Record<Suit, string>}>`.
Grew past the initial 5 (four-color, two-color, four-color-alt-1,
four-color-alt-2, colorblind — the Okabe-Ito safe palette) to include
several more (casino, neon, pastel, cyber, royal, candy, mono, solarized,
synthwave, amoled, material, vibrant, retro). The generator has no hardcoded
variant list — it walks `Object.entries(DECK_VARIANTS)`, so adding a variant
is just adding an entry to that map and rerunning `npm run cards:variants`.

## Asset generation
Shape templates (52 SVGs, one per suit+rank, colored in the `four-color`
palette) live in `scripts/card-templates/` — a build input, not a served
asset. `scripts/generate-card-variants.ts` (`npm run cards:variants`, uses
`node --experimental-strip-types` like the existing test script) reads each
template and, for every variant including `four-color` itself, does a
literal hex replace of that suit's `four-color` value with the variant's
value, writing to `public/svgs/variants/{variantId}/{suit}-{rank}.svg`.
`public/svgs/` root no longer holds any per-rank card file — only backs,
chips, the logo, and the decorative suit icons (`*-card.svg`, `*-chip.svg`)
that aren't part of the rank grid. Output is committed like any other
static asset — no runtime SVG parsing, no new dependency, `next/image`
usage in `PlayingCard` is untouched.

## Wiring
- `cardPath(card, variant = DEFAULT_DECK_VARIANT)` in `lib/cards.ts` always
  resolves to `/svgs/variants/{variant}/{suit}-{rank}.svg`.
- `PlayerProfile` (`lib/api/player.ts`) gains an optional `deck_variant?: DeckVariantId`
  field — absent today, backend fills it in later.
- `useDeckVariant()` hook reads the existing `['player', 'me']` TanStack
  Query cache (same query `TermsGate`/`useOptionalSession` already populate,
  no extra fetch) and returns `data?.deck_variant ?? DEFAULT_DECK_VARIANT`.
- `PlayingCard` calls the hook internally and passes the variant into
  `cardPath`. The 9 existing call sites are unchanged.

## Out of scope
No in-app variant switcher UI — selection is entirely backend-driven once
that field exists. No new card shapes/premium art — color only, for now.
