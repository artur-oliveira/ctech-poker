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

## 9. The ring caption is a stack, in the chips' own gold

`--seat-stack-ink` (`base.css`, documented in `DESIGN.md`) started this pass as
pure white and finished it as gold — see round 2, item 3 below, which carries
the final tokens and their measured ratios. `--text-secondary` reached only
3.9:1 on the lightest premium felt and plain `--gold` 2.9:1, which is why the
token exists at all.

The portrait ring caption also drops the player name (the avatar's initials and
colour already identify the seat; the name stays in the DOM and in the seat's
`title` for assistive tech) and, ring-wide now rather than portrait-only, the
"fichas" unit — a bold caption overflowed the 64–78px lane in short landscape.

## Not done

- `npm run e2e` (Playwright) was not run in round 1: the environment had no
  browser download for the Playwright bundle. Items 6 and 7 were verified
  instead in Chrome through the DevTools protocol at 320×568, 390×844, 844×390,
  1280×800 and 1920×1080, with 2, 5 and 9 players. Round 2 ran the suite.
- `bundle-budget.json` was not re-pinned, and does not need to be: the "eleven
  routes over budget" reading was a measurement artifact. `npm run build` is
  Turbopack (the Next 16 default) and chunks differently from the webpack-pinned
  budget; `npm run bundle:check` against `npx next build --webpack` reports every
  route within tolerance. See round 2's verification section.

---

# Round 2 — seat state cues, the docked outcome sheet, the turn cue, a typed raise

Same branch, same surface. Eight further items, all of them things the first
pass either could not reach or made newly visible.

## 1. On a coarse pointer the seat's state word goes; its state does not

`Desistiu`, `All-in`, `Ausente` and `Desconectado` each cost a caption line in a
64–78px lane that already has to hold a stack figure, and the longest of them
clipped outright on the tallest portrait ring. The short-portrait tier had
already dropped them for exactly that reason, so the gate moved to
`@media (pointer: coarse)` as a whole rather than to one viewport slice
(`renderer.css`, end of file). The word is *clipped* to the accessibility tree
(`clip-path: inset(50%)`), never `display: none`, so nothing a screen reader
hears changes, and desktop keeps every visible label.

Hiding a word is only allowed where something else carries the state, and hue
alone is not something else (WCAG 1.4.1). Each state that matters therefore got
a cue that differs by **shape**:

| state | cue | why not colour |
|---|---|---|
| all-in | solid `--gold` ring on the avatar, backed by an `--ink` ring | gold reads 3.01:1 on the brightest felt on its own, under 1.4.11's 3:1; the ink ring's own boundary is 10.4:1 wherever it lands |
| folded | **dashed** muted outline, plus the grayscale and half-opacity hole cards it already had | `outline` and not `box-shadow` because only an outline can be dashed, and the dash is the whole point |
| disconnected | an explicit wifi-off badge (`WifiOff`, paper on ink) | folded and disconnected both mean "not acting"; grey alone collapses them into the same seat |
| sitting out | grayscale, no badge, no cards | absence of both cues, distinct from folded's dash |

`Seat.tsx` renders the badge whenever the seat reports a dropped connection; CSS
shows it only where the word is hidden. Covered in `Seat.test.tsx` for both
shapes of "disconnected" and for the four connected states that must not show it.

## 2. Every ring seat parks its stack in its avatar's own lane

Round 1 fixed the portrait-ring caption; the **top-arc** seats still pushed
theirs sideways along the rail (`.stage-v-ring .seat-zone-top .seat-info` had
its own left/right lane). A figure parked beside an avatar chip reads as a loose
number belonging to nobody while every other seat centres its own underneath.
That override is deleted: the caption is now centred in the avatar's column on
every ring seat — underneath, or mirrored above where the hole cards already own
the space below (the top arc, and a left/right seat riding high).

`--seat-caption-reserve: 16px` (`base.css`) is the headroom the portrait stage
keeps above its ring for the mirrored case — the same job `--seat-badge-overhang`
does on the other three sides. Without it the mirrored caption lands outside
`.game-table.stage-v`'s clip box, which is what the 320×568 heads-up ring did:
7px of the only stack figure on screen, gone. The ring's `flex: 1` absorbs it, so
it costs no felt height.

## 3. The stack figure is the chips' own gold, per felt

`--seat-stack-ink` was pure white. It is a chip count, so it now wears the chip
face's own highlight stop — `--gold-pale` (#efe1b7, the light stop of `.chip`'s
radial gradient) lifted toward white only as far as WCAG AA demands.

Default (`base.css`): `color-mix(in srgb, var(--gold-pale) 75%, #ffffff)` =
**#f3e9c9**. Measured against the rendered tokens:

| felt | on felt (light stop) | on rail | on room |
|---|---|---|---|
| classic | **4.58:1** | 5.85:1 | 15.89:1 |
| ocean | **4.71:1** | 7.48:1 | 15.89:1 |

`--gold-pale` unlifted reaches only 4.26:1 on classic, which is what the lift is
for. The two dark felts need no lift and say so in their own `[data-table-theme]`
blocks (`renderer.css`), overriding the token to the full `--gold` (#e6b85c):

| felt | on felt | on rail | on room |
|---|---|---|---|
| midnight | **5.01:1** | 4.72:1 | 10.44:1 |
| burgundy | **5.12:1** | 5.27:1 | 10.44:1 |

Every value is ≥ 4.5:1. `--table-rail-highlight` is *not* in the table on
purpose: it is the felt's 4px inner bevel (`--shadow-table`), not a surface text
lands on. Per-theme overrides are the sanctioned mechanism here — a compromise
value nobody chose, or a text-shadow halo, would both be worse.

The portrait hero HUD's stack takes the same token: its own dark surface could
carry the deeper `--gold` at 10:1, but it sits in the same glance as up to eight
ring captions the felt forces to the lifted tone, and one quantity painted in two
golds on one screen reads as two different things. A stack inside a *desktop*
seat card keeps plain gold — Seat Surface at 10.2:1, and no ring caption beside
it to disagree with.

## 4. The outcome panel scrolls, and the dismiss stays put

`.hand-outcome-card` already capped its height (`min(78dvh, 560px)`, and
`max-height: 62%` of the ring for the portrait docked sheet) with
`overflow-y: auto` and `overscroll-behavior: contain`. What was still broken was
the way out of it: `.hand-outcome-dismiss` was `position: absolute` in the card's
top-right corner, which positions against the **scrolled** content. On a short
viewport — a 320×568 portrait ring, a run-it-twice settlement — the player
scrolled to reach the pot rows and the only control that closes the card went off
the top with them.

It is `position: sticky; top: 0` now, in its own collapsed grid row (negative
bottom margin of button height + gap), so the card's layout is byte-for-byte
what it was and the button rides the whole scroll. A faint `currentColor 10%`
disc keeps it legible once it is floating over settlement rows rather than over
the heading's clear space.

Verified in Chrome at 320×568 and 390×844 portrait and 844×390 landscape,
scenario `run_it_twice` (the tallest card the mock produces, 285px of content in
a 224px box): the sheet scrolls, the scroll does not chain to the page, every
settlement row is reachable, and the dismiss is in the corner at scroll top and
scroll bottom alike. Short cards (`complete_tie`, 307px in a 395px box) gain no
scrollbar.

## 5. The turn cue: glow, haptic, digits

- **Glow.** `.game-seat.viewer.is-turn:before` — a 2px gold ring on a
  pseudo-element the seat was not otherwise using, breathing outward
  (`viewer-turn-halo`, 1.8s). `transform` + `opacity` only, never a layout
  property. Under `prefers-reduced-motion` it freezes at its resting ring rather
  than disappearing: it is the "it is on you" signal, not decoration.
- **Haptic.** `lib/hooks/useTurnHaptic.ts` — one `navigator.vibrate(20)` when the
  viewer's own turn opens. Once per turn (the latch clears only when `isTurn`
  goes false, so a resync or a re-render cannot re-fire it), never for another
  player (the caller passes only the viewer's own turn), never in a hidden tab
  (the latch still arms there, so returning to the tab does not buzz late), and
  feature-detected — most desktop browsers have no Vibration API.
- **Digits.** `useReducedMotionCountdown` is now `lib/hooks/useTurnCountdown.ts`:
  it surfaces the whole-second readout for the last `TURN_COUNTDOWN_SECONDS`
  (10) of any turn, on desktop and mobile alike, and for the *whole* turn under
  reduced motion, where the perimeter ring's CSS animation is frozen and the
  digits are the only surviving signal. It rides `useLiveNow`/`useSharedTicker`
  — no interval of its own, per the one-clock invariant.

`.seat-timer-seconds` moved off `inset: 0` (which dropped the digits onto the
hero card's stack figure once they were no longer reduced-motion-only) into its
own pill hung off the seat's top edge, centred: the two upper corners belong to
the role pill and the player-actions trigger. On the ring the seat *is* the
avatar chip and the hole cards own the edge above it, so the digits go on the
chip. Its own background, so its contrast is the pill's rather than whatever the
seat happens to sit on.

No new timing prop crosses a `memo` boundary — the digits are derived inside the
seat that already receives the deadline, so `tableRenderBudget.test.tsx` is
unchanged.

## 6. The pot leads the felt

It is the single most-read number on the play surface and it was competing with
its own caption and with the street rail: 14px of gold beside a 10px label, both
at the same weight. The figure is now `--font-size-xl` (one step down per stage:
`--font-size-large` on the portrait ring, `--font-size-subtitle` on the shortest
tier) and the word `POTE` steps back to a quiet 9px mono marker,
baseline-aligned so it sits on the number's line rather than floating at its
middle. Round 1's abbreviated sandbox figure is what freed the width for it.

## 7. The raise total is typable

`BetAmountOutput` was a read-only `<output>`. The `+`/`−` hold-repeat tops out at
a stride of 10×`raiseStep` and the range slider is desktop-only, so on a deep
stack there was no way to name an exact number at all. It is `BetAmountField`
now, wrapping a text input.

`type="text"`, never `type="number"`: a number input silently accepts `e`, `+`
and `-`, and reports `value === ''` for anything it considers invalid, which
leaves nothing to filter. `inputMode` is `numeric` in sandbox and `decimal` in
real money.

`lib/betInput.ts`'s `checkBetInput` is the one filter. It runs in `onChange`,
which is the single path a keystroke, a paste, a drop and an IME commit all
reach, so validation cannot be bypassed by pasting; a rejected candidate simply
never becomes state. It answers "may the field *become* this?", never "is this a
legal raise?":

- digits only — letters, `e`, `E`, `+`, `-`, spaces and a pasted `R$ 1.200,00`
  are all rejected;
- a decimal separator (`.` or `,`) only in real money, at most one, at most two
  fractional digits; sandbox has no fractional chip, so a separator there is a
  typo, not a rounding choice;
- no leading zero followed by a digit;
- the **upper** bound blocks the keystroke, on the resulting *value* rather than
  a digit count: with a 1.000 ceiling `999` is accepted, the fourth digit
  (→ `9990`) is not, and `1000` still types cleanly;
- `minRaise` and `raiseStep` deliberately block **nothing**. Blocking below the
  minimum makes a 250.000 raise untypable — you would be stopped at `2`. They
  land once, on blur or Enter, through `clampSnapRaise` (`betShortcuts.ts`, now
  shared with the quick-preset row, which had the same snap-then-clamp inline);
- the empty string is a legal transient state while focused; blur resets it to
  `minRaise`.

A rejected keystroke is never silent: the reason renders in the existing
`.bet-clamped-note` (`role="status"`) under the number — `Apenas números.`,
`Fichas sandbox não têm centavos.`, `Máximo 1.000 fichas.` The clamp note is
suppressed mid-edit, because "ajustado ao mínimo" while somebody is still typing
the first digit of 250.000 is a lie about a value they have not finished naming;
`betCommitNote` replaces it on commit with `ajustado ao mínimo` or
`arredondado para …`. Escape restores the amount the field was entered with —
every accepted keystroke has already published to the slider and the raise
button, so abandoning has to undo those too, via a ref because Escape blurs
synchronously before React flushes.

Focus writes the exact figure to the node *before* the state update so
`select()` selects that rather than the abbreviation it replaces; unfocused, the
field shows the same abbreviated figure every other chip readout shows.
`isTypingTarget` (`lib/utils.ts`) already keeps the action shortcuts and the
hold-repeat off a focused text input, and the per-decision reset through
`RaiseControl`'s `actionKey` is untouched.

`betInput.test.ts` unit-tests the filter directly; `ActionBar.test.tsx` drives it
through the component, including the paste path.

## 8. `touch-action: none` on the bet slider

One line on `.bet-control input[type=range]`. The slider is desktop-only in
practice (the compact tiers hide it for the `+`/`−` stepper), but a touchscreen
laptop has both, and without it the horizontal drag is claimed by the OS
back-swipe gesture.

## Verification (round 2)

- `npx vitest run --coverage`: 153 files, 1667 tests, all passing.
  Statements 94.11 / branches 91.30 / functions 91.07 / lines 95.41 — above the
  90 floor and above where round 1 left them. No threshold lowered.
- `npx tsc --noEmit`, `npx eslint src --max-warnings 0`, `npm run build`: clean.
- `npm run e2e`: 130 passed across Chromium and Firefox. The 65 WebKit specs
  cannot launch in this environment — Playwright's bundled `libWPEWebKit-2.0.so`
  is linked against an older ICU than the host's (`undefined symbol:
  ureldatefmt_format_74`), so every WebKit spec fails at `browserType.launch` in
  1-2ms, before any page loads. Not a layout result either way; CI runs the same
  suite on its own image.
- `npm run bundle:check`, measured against `npx next build --webpack`: **all 23
  routes within tolerance**, `/table` at 1235.0 kB against a 1289.0 kB budget,
  and no dev mock runtime on any route's critical path. Round 1's "eleven routes
  over budget" note was a measurement artifact — `npm run build` is Turbopack
  (the Next 16 default) and chunks differently from the webpack-pinned budget.
  `bundle-budget.json` is not re-pinned, and did not need to be.
- Chrome DevTools at 320×568, 390×844, 844×390, 1280×800 and 1280×800 with a
  coarse pointer, across 2, 5 and 9 players and the `flop`, `all_in`,
  `complete_tie`, `run_it_twice` and `timeout` scenarios.

## Not done (round 2)

- **The state-word gate is `(pointer: coarse)`, not a width breakpoint.** A
  touchscreen laptop at 1280px therefore also loses the visible words, even
  though its seat cards have room for them. That is the gate the brief asked
  for, and the shape cues carry the state there too; a width-qualified gate
  would be the change if that turns out to matter.
- **No non-text cue was added for `Ausente`.** Sitting out is already the only
  state with neither cards nor an outline, which distinguishes it from folded
  (dashed) and from disconnected (badge) without a fifth vocabulary item.
- **The docked outcome sheet's 62% cap is still measured against the ring**, not
  the viewport. Growing it past the ring would push the sheet under
  `.game-table.stage-v`'s clip edge, and covering the whole felt is the thing the
  docked-sheet design deliberately avoided.

---

## Round 3 — the WebKit regression this pass shipped (2026-09-09)

`main` went red on the **Cross-browser layout** job immediately after #376 merged.
Five WebKit specs failed and seven more were flaky; Chromium and Firefox were
green. That job only runs in the Deploy workflow, i.e. only on `main`, so the
branch's own Frontend run never executed WebKit — and WebKit could not be
launched on the author's Fedora host at all (Playwright's bundled
`libWPEWebKit-2.0.so` wants an ICU older than the system's:
`undefined symbol: ureldatefmt_format_74`), so every local run reported
"130 passed" with all 65 WebKit specs erroring at `browserType.launch` in ~1ms.
The whole pass shipped blind to one of the three engines.

### One cause, three numbers

Reproduced in the matching official container
(`mcr.microsoft.com/playwright:v1.63.0-jammy`). Measured against Chromium, every
`.game-seat` in WebKit sat **exactly 12px lower** while `offsetTop` was
byte-identical — a transform residue, not a layout difference. The three
failures are that one 12px seen from three angles:

| assertion | reported | arithmetic |
|---|---|---|
| `every balanced seat sits on the band centreline` | `11.99` vs `<= 1` | the 12px itself |
| `nine_max: nothing a player needs is clipped` | viewer seat `4` past `bottom` | 12 − the 8px `--seat-badge-overhang` the stage pads with |
| `the win/loss streak badge fits inside the stage` | `-11` vs `>= 0` | the badge hangs 8px below a seat that is 12px low |

### Root cause

Round 1 (`7567126`) added the join/leave slide:

```css
@keyframes seat-join { from { opacity: 0; translate: 0 12px } }
.game-table .game-seat { animation: seat-join 260ms var(--ease-out-quart) both }
```

applied to **every** seat, on every mount. `animation-fill-mode: both` holds the
`from` keyframe until the animation advances — and in WebKit it never did:
`document.getAnimations()` reported all 73 animations on the page `pending`,
`startTime: null`, held at `currentTime: 0`, with `requestAnimationFrame` never
firing. Every seat therefore rendered at `opacity: 0` and `translate: 0 12px`
permanently.

The 12px was only what the suite could measure. The real state of the page was
worse: **in WebKit the table had no seats at all** — no players, no stacks, no
hole cards, just the felt, the pot and the board.

The rendering stall itself is *pre-existing and not ours*: the same probe on
`9e2a8d0` (the commit before #376) reports `rAF: 0` and 63 pending animations
too. What changed is that #376 made a permanent element's **resting state** the
output of an entrance animation. Everything else in the app that animates with
`both` animates something that genuinely just appeared, so a stalled animation
costs a transition, not the content.

### The fix

An entrance animation is applied only to what actually just entered.
`useJoinedSeats` (TableStage) mirrors the existing `useDepartedSeats`: it diffs
seat membership **during render** — an effect runs after commit, so the seat
would paint at rest for one frame and then fade in from zero — marks the arrivals
with `data-seat-joining`, and drops the mark after `SEAT_ENTER_MS`. The CSS moves
from `.game-table .game-seat` to `.game-table .game-seat[data-seat-joining]`.

Seats that are simply seated now carry no animation at all, so their resting
state is the stylesheet's in every engine. Round 1's slide on join/leave is
unchanged for the case it was designed for, and the geometry tokens
(`--table-rail-inset-*`, `--table-rail-band`, `--table-orbit-*`) were not touched
— the One Band Rule was never the problem. Even a joining seat now degrades
better: a stalled entrance costs `SEAT_ENTER_MS` of invisibility instead of the
life of the table.

After: **195 passed** across Chromium, Firefox and WebKit (was 190 passed /
5 failed / 7 flaky), with no assertion or tolerance changed.

### Why the gate did not gate

`Cross-browser layout` lives in `.github/workflows/deploy.yml` and runs on
`main` only. Moving the job (or a WebKit-only subset of it) into the `Frontend`
workflow's PR trigger is what would have caught this on the branch; it costs
roughly the 2 minutes the WebKit project takes.
