# Table reactions — choreography & copy polish

Date: 2026-08-31

Follow-up to `docs/2026-08-25-poker-theater-reactions.md`. Scope: fix per-reaction visual
inconsistencies flagged in live review, rewrite every label/caption in the table's acid
friendly-fire voice, and add two directed reactions. No wire, transport, or state change.

## Choreography fixes (`src/app/table-reactions.css`, `src/components/table/TableReactions.tsx`)

| Reaction    | Before | After |
|-------------|--------|-------|
| `clap`      | BRAVO banner overlapped the glyph; applause ringed the banner | Banner lifted clear above the glyph; applause fans to the sides/below via `reaction-clap-fan` |
| `cry`       | Emoji `💧` "curtain" | Drawn teardrops (gradient + inset highlight) falling from above |
| `fire`      | Yellow radial blob (`reaction-flame-crown`) clashed with the `🔥` glyph | Blob removed; only fanned embers remain, with an orange glow |
| `heartbeat` | ALL IN callout collided with orbiting hearts | Callout moved to the top; hearts offset 45° so none passes straight up |
| `shark`     | Oversized bright-green triangle fin | Smaller slate fin (`--felt-text`→`--ink` mix), lower pass, drop shadow |
| `pokerface` | Redundant shades bar over the `😎` glyph hid the suits | Bar replaced with a shock ring; suits orbit on a wider radius (`reaction-pokerface-suit`) that clears the glyph |
| `tear`      | Six `💧` raining full width — read as a waterfall | One heavy drawn teardrop wells up over the target, swells, and rolls down with a streak |
| `poop`      | Grinning `💩` plop + stink cloud | Brown splat mirroring `tomato` (`--reaction-muck` / `--reaction-muck-deep`), ECA impact word, scattered drops |
| `rofl`      | Three `🤣` stacked on centre | Three `🤣` spread horizontally (22 / 50 / 78 %), staggered roll-in |
| `duck`      | Feathers bunched near centre | `reaction-feather-fall` spread widened (`(piece − 2.5)` spacing) |
| `flowers`   | Petals stacked | Same widened `reaction-feather-fall` spread |

Removed now-dead keyframes: `reaction-flame-crown`, `reaction-poker-shades`, `reaction-poop-plop`,
`reaction-stink-dot`, `reaction-target-rain`.

New token (`src/app/globals.css` `:root`): `--reaction-muck: #6b4a2a`, `--reaction-muck-deep: #3f2b18`
— muck brown for the `poop` splat, deliberately not a rail/wood surface token. Documented in `DESIGN.md`.

## New directed reactions

Added to `src/lib/reactions.ts`, `REACTION_THEATER`, `src/app/table-reactions.css`, and the API
whitelist (`api/internal/reactions/catalog.go` + `catalog_test.go`). Both free and targeted.

- **`cucumber` — "Botar pepino" / "Pulou que nem gato"** (`🥒`, `--tag-green`). The cat-and-cucumber
  scare: the cucumber creeps in low and quiet from the side, settles behind the target, then the seat
  jolts — green shock-ring burst, `SUSTO!` punch-in, and jump lines radiating up. Terminal beat: the
  burst + callout.
- **`boomerang` — "Jogar bumerangue" / "Isso volta pra você"** (`🪃`, `--tag-orange`). Spins out along
  the throw arc, clips the target, then whips back toward the sender (`translate(--reaction-dx/dy * −0.45)`)
  with orange motion-trail crescents and a `VOLTOU` banner. Terminal beat: the catch/return.

Catalog is now 30 reactions (13 self tells, 17 directed). The picker's two-mode split, favourites,
premium entitlement, cooldown, no-opponent, disconnected, and reduced-motion paths are unchanged;
reduced motion still drops every impact and places the glyph at the final seat.

## Copy

Every caption rewritten in pt-BR acid friendly-fire voice (labels mostly unchanged; `pokerface`
"Cara de pôquer" → "Pokerface", `fire` "Sequência quente" → "Pegando fogo"). Sincere gestures
(`respect`, `clover`, `coffee`, `spotlight`) stay warm or lightly barbed. `spotlight` accent
`READ` → `BOA`; `turtle` accent `TANK` dropped (the slow turtle + ZzZ already carry it).

## Verification

- `TableReactions.test.tsx` (identity for all 30 ids, source/target positioning, favourites, premium,
  disconnected, cooldown, no-opponent, missing-seat, visibility persistence, targeting) — updated tab
  counts (`17 gestos`) and the `pokerface` / `fire` label references.
- `reactionStore.test.tsx` — `fire` label reference updated.
- `api/internal/reactions/catalog_test.go` — `cucumber` / `boomerang` added to both mirror lists; `go test ./internal/reactions/` green.
- `npx vitest run` (1168 tests), `npx tsc --noEmit`, `npx eslint src --max-warnings 0` — all clean.
- Live review against `/table?id=…&scenario=winner_cards` (static scenario).
