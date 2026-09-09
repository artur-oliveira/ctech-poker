# Table polish: stage-aware bet presets, abbreviated sandbox chips, seat legibility

2026-09-08 · branch `feat/table-polish-bet-presets` · `ui/`

A single visual pass over the table surface (`src/app/(app)/table/`,
`src/components/table/`). Nine changes, plus the API-side `bet_preset_mode`
preference landed in the same branch (`api/`, #341).

## 1. The street trail names one street

`StreetProgress` (`TableStage.tsx`) rendered four labelled pips — `● Pré · ● Flop
· ● Turn · ● River`. Three of those words name streets nobody is on, and they
spent the whole centre band of the felt. It now renders four bare pips plus one
label: the street the hand is actually on.

The strip used to be `aria-hidden`. It is now `role="img"` with
`aria-label="Flop — etapa 2 de 4"`, because the *position* is progress the
sr-only page heading (`Mesa de poker: Flop`) does not otherwise convey.

## 2. Sandbox chip amounts are abbreviated; real money never is

`lib/chips.ts` gained the one formatter pair:

- `chipsExact(n)` — `n.toLocaleString('pt-BR')`. Always what an accessible name
  carries.
- `chipsShort(n)` — at most four visible characters: `999`, `1,2K`, `600K`,
  `1M`, `1,5B`, `1T`. Truncated, never rounded, so `999.999` is `999K` and never
  `1000K`; one decimal only while the mantissa is a single digit.

Which one a surface uses is `ChipFormatContext` (`lib/chipFormat.ts`), read
through `useChipFormat()`. It defaults to **exact** — abbreviation is opt-in —
and the table page provides `room.currency_mode !== 'real'`. Abbreviating real
money is not a display choice, it is a wrong number.

A context and not a prop: the amount is painted by `Seat`, `Board`, `ActionBar`,
`HandOutcome` and friends, three of which sit behind `memo` boundaries whose
props must stay primitives or stable identities (#230). The value flips at most
once per table session, and both formatters are module-level function
identities, so nothing re-renders for it.

Every abbreviated figure keeps the exact number in an `aria-label`.

**Deliberately not abbreviated:** the buy-in / rebuy / leave / session-recap
dialogs, `RealityCheck`, `WinnerCards`' fee sentences and `VoiceActionButton`'s
confirmation. Those are money-and-consent prose with room for the real figure.

## 3. Favourite bet presets are gone

`QUICK_PRESET_DEFS`, `QuickPresetRow`, the per-pill star buttons, the
`favoriteBetPresets` / `favoriteBetPresetsSaving` /
`onToggleFavoriteBetPresetAction` props, the `favorite_bet_presets` profile
field and all `.bet-quick-preset*` CSS. The server field was never implemented,
so there is nothing to migrate. Supersedes the `QuickPresetRow` half of
`docs/2026-09-05-action-bar-raise-sizing-polish.md`.

## 4. The preset row is stage-aware and configurable

`stageBetPresets` (`lib/betShortcuts.ts`) builds the one row, from the new
`bet_preset_mode` profile field (`'mixed' | 'bb' | 'pot'`, server-normalized,
client-defaulted by `betPresetMode()`):

| mode | pre-flop | flop / turn / river |
|---|---|---|
| `mixed` (default) | `BB` `2BB` `3BB` `All in` | `1/3` `1/2` `2/3` `All in` |
| `bb` | big blinds | big blinds |
| `pot` | pot fractions | pot fractions |

Pot fractions are the **server's own** `*_raise_to` figures, never
`pot * fraction`: a pot-size raise has to absorb the outstanding call, and
re-deriving it client-side would paint a number the server would reject. The
big-blind set needs no help — a "3BB open" is literally a raise-to of three big
blinds. Two consequences:

- `actionState` (`lib/tableActions.ts`) now **omits** a fraction the server did
  not send instead of pricing it at `minRaise`, and `stageBetPresets` keeps the
  pot set strictly increasing. Otherwise a server missing `two_thirds_pot_raise_to`
  painted `2/3` for less money than `1/2`.
- The brief asked for `3/4`; the wire exposes ⅓/½/⅔/Pote and no ¾, so the third
  fraction is `2/3`.

Every value is snapped to `raiseStep` and clamped into `[minRaise, maxRaise]`,
then collapsed duplicates are dropped **keeping the last holder** of each value:
a short stack squeezes several fractions onto the same all-in total, and the
button left standing must be the one that says `All in`.

The setting lives in `TablePreferencesDialog` ("Presets de aposta": *Mista
(padrão)* / *Big blind* / *Pote*) on the same `updateMe` mutation as the felt
theme, with its own pending and error branches.

## 5. The seat is the player menu on touch

`.seat-actions-trigger` was a 28px filled circle floating off the seat's
top-right corner — a second, louder control than the seat it belonged to, and at
9-max it hung over the neighbour's stack. It is now flat and quiet inside the
card (`opacity: .45`, transparent, no border), full strength on
`.game-seat:hover`, `:focus-within` or while the popover is open.

On coarse pointers there is no hover to reveal anything with, and a 22px badge
on a 40px avatar chip is not a tap target — so the trigger stretches over the
whole seat and its icon goes away. It is the same `<button>`, so keyboard
access, focus order and the `Ações para <jogador>` name are unchanged.

The gate is `data-seat-tap`, set by `Seat` and absent whenever
`reactionTargetLabel` is present: while a reaction is being aimed, the seat body
belongs to `.seat-reaction-target` (z-index 8) and the two cannot both claim the
tap. Both branches are asserted in `Seat.test.tsx`.

The note dot stays visible in both modes.

## 6. The prepared-action row is one row

`.action-preselectors` is `grid-auto-flow: column` at every width, so however
many options are legal this street sit side by side instead of wrapping the
fifth onto its own line. The compact tiers narrow the *labels*, never the 44px
tap targets: `Check / Fold` → `C/F`, `Call Any` → `Any`, and the fixed-amount
`Call` drops its figure (which is already on `Pagar` the moment the turn
arrives). The full name stays in each button's `aria-label`.

Measured in Chrome at 320×568 and 390×844: one row, five options, 44px tall,
56px minimum width, no clipping and no horizontal page scroll.

## 7. Occupancy no longer resizes the table

`.stage-v[data-player-count='2'…'5']` deepened `--table-rail-inset-top` and
`-bottom` by up to 20%. Because the felt is *derived* from the rail inset (the
One Band Rule), that resized the felt — a heads-up capsule was visibly smaller
and rounder than a nine-handed one, and the whole table jumped under the player
every time somebody sat down or stood up. Those rules are deleted.

`.stage-v[data-capacity='2'] .stage-v-ring { max-width }` stays: capacity is a
property of the *room*, not of who is currently sitting, so it cannot move
mid-session.

Verified at 390×844: felt 282×504 at nine players, 260×504 at five — identical
height, identical top.

## 8. Seats slide in and out

CSS keyframes in `(app)/table/table.css`, on the independent **`translate`**
property rather than `transform`: a balanced seat is centred on its orbit point
with `transform: translate(-50%, -50%)`, and a keyframe on `transform` would
fling it off the band. `translate` composes with it and stays off the layout
path. `prefers-reduced-motion` drops to a 120ms opacity fade.

The survivors' reflow is `left`/`top` and is deliberately **not** animated, so
the seat that changed is the only motion on screen.

A departing seat is held for one animation by `useDepartedSeats` (`TableStage`),
which remembers each seat's last index *and* last occupancy so the ghost sits
where the player was. The ghost is inert: `pointer-events: none`, `aria-hidden`,
and it does not register with `lib/seatRects.ts`, so the reaction layer never
measures a seat that is on its way out. `SEAT_EXIT_MS` and the CSS duration are
kept in sync by comment.

## 9. The ring caption is a stack, in white

`--seat-stack-ink` (`base.css`, documented in `DESIGN.md`) is pure white. It is
the only value that clears WCAG AA on every felt: classic 5.5:1, ocean 5.1:1,
midnight 8.0:1, burgundy 8.4:1, plus 6.3:1 on the walnut rail.
`--text-secondary` reached 3.9:1 there and `--gold` 2.9:1.

The portrait ring caption also drops the player name (the avatar's initials and
colour already identify the seat; the name stays in the DOM and in the seat's
`title` for assistive tech) and, ring-wide now rather than portrait-only, the
"fichas" unit — a bold caption overflowed the 64–78px lane in short landscape.

## Not done

- `npm run e2e` (Playwright) was not run: this environment has no browser
  download for the Playwright bundle. Items 6 and 7 were verified instead in
  Chrome through the DevTools protocol at 320×568, 390×844, 844×390, 1280×800
  and 1920×1080, with 2, 5 and 9 players.
- `bundle-budget.json` was not re-pinned. All eleven routes were already over
  budget on the branch base (`/`, `/poker-rules` and every `/guide/*` page
  included — none of which this change touches); `/table` moved 1315.5 kB →
  1317.4 kB. Re-pinning here would launder a pre-existing regression.
