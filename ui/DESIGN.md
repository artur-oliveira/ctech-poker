---
name: CTech Poker
description: A vivid, trustworthy online poker table built for social play on any connected device.
colors:
  brand: "#af2a2f"
  brand-bright: "#d9464d"
  wine: "#5b1218"
  ink: "#120d0e"
  paper: "#f6f0e7"
  gold: "#e6b85c"
  felt: "#0d5b45"
  felt-light: "#18765b"
  felt-dark: "#084b38"
  rail: "#7c4d2f"
  rail-edge: "#291714"
  rail-highlight: "#b0774a"
  muted-rose: "#ad9fa0"
  text-secondary: "#cbbfc0"
  felt-text: "#e3f1ea"
  felt-text-muted: "#d9eee7"
  gold-ink: "#30230a"
  success: "#48c98c"
  danger: "#dc2626"
  focus-ring: "#ed777c"
  on-brand: "#ffffff"
  surface-seat: "#161011"
  surface-control: "#211416"
  surface-control-hover: "#3e3133"
  surface-error: "#3b0b0e"
  control-border: "#ffffff24"
  seat-border: "#ffffff26"
  # captured drift tokens (previously literal hex across the stylesheet)
  white-03: "#ffffff03"
  white-08: "#ffffff08"
  white-0b: "#ffffff0b"
  white-0c: "#ffffff0c"
  white-12: "#ffffff12"
  white-13: "#ffffff13"
  white-18: "#ffffff18"
  white-1b: "#ffffff1b"
  white-1c: "#ffffff1c"
  white-1d: "#ffffff1d"
  white-22: "#ffffff22"
  white-2a: "#ffffff2a"
  white-2c: "#ffffff2c"
  black: "#000"
  black-06: "#0006"
  black-07: "#0007"
  black-08: "#0008"
  black-0b: "#000b"
  brand-33: "#af2a2f55"
  brand-53: "#af2a2f88"
  brand-27: "#af2a2f44"
  brand-13: "#af2a2f22"
  brand-08: "#af2a2f14"
  brand-07: "#af2a2f12"
  brand-40: "#d8404766"
  brand-deep-33: "#db4a5055"
  brand-tint: "#c7353b"
  brand-tint-soft: "#e2b7b9"
  brand-bright-2: "#e24a51"
  brand-bright-3: "#d85b61"
  brand-bright-4: "#e65b62"
  brand-bright-5: "#df5a61"
  brand-bright-6: "#d75359"
  brand-bright-7: "#ef747a"
  wine-deep-2: "#6e1d23"
  wine-deep-3: "#5b171a"
  wine-deep-4: "#721d23"
  wine-deep-5: "#45151a"
  wine-deep-6: "#792128"
  wine-deep-7: "#201214"
  wine-edge: "#824b4e"
  wine-edge-2: "#3d2224"
  gold-bright: "#e7bd63"
  gold-bright-2: "#e7c36f"
  gold-bright-3: "#e9c678"
  gold-pale: "#efe1b7"
  cream: "#f1e5ce"
  text-muted-soft: "#d9cccd"
  neutral-rose-2: "#c0b3b4"
  neutral-rose-3: "#c9bcbd"
  neutral-rose-4: "#bcaeb0"
  neutral-wine: "#897d7e"
  neutral-wine-2: "#5e5051"
  neutral-wine-3: "#645758"
  ink-soft: "#130d0e"
  room-grad-1: "#180d10"
  room-grad-2: "#100c0d"
  room-grad-3: "#251012"
  room-grad-4: "#0d090a"
  room-overlay: "#080506d9"
  rail-shadow: "#351d17"
  rail-mid: "#7c5035"
  rail-light: "#a46d42"
  rail-edge-2: "#382018"
  rail-wood-2: "#5b3428"
  felt-bright: "#12694f"
  felt-deep: "#084332"
  success-bright: "#45c489"
  felt-text-soft: "#b6d6c5"
  felt-27: "#0a806044"
  felt-shadow: "#001f18"
  overlay-night: "#001b"
  overlay-night-2: "#0018"
  overlay-night-3: "#001d"
  surface-seat-92: "#171011eb"
typography:
  display:
    fontFamily: "Geist, ui-sans-serif, system-ui, sans-serif"
    fontSize: "clamp(2.375rem, 5vw, 3.875rem)"
    fontWeight: 700
    lineHeight: 1.05
    letterSpacing: "-0.04em"
  title:
    fontFamily: "Geist, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 700
    lineHeight: 1.2
  body:
    fontFamily: "Geist, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "Geist, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 600
    lineHeight: 1.2
  mono:
    fontFamily: "Geist Mono, ui-monospace, SFMono-Regular, monospace"
    fontSize: "0.75rem"
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: "0.1em"
rounded:
  card: "6px"
  control: "12px"
  seat: "14px"
  panel: "16px"
  pill: "999px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "16px"
  lg: "24px"
  xl: "40px"
components:
  button-primary:
    backgroundColor: "{colors.brand}"
    textColor: "{colors.on-brand}"
    rounded: "{rounded.control}"
    height: "40px"
    padding: "0 16px"
    typography: "{typography.label}"
  button-primary-hover:
    backgroundColor: "{colors.brand-bright}"
    textColor: "{colors.on-brand}"
    rounded: "{rounded.control}"
    height: "40px"
    padding: "0 16px"
  button-outline:
    backgroundColor: "#ffffff0d"
    textColor: "{colors.on-brand}"
    rounded: "{rounded.control}"
    height: "40px"
    padding: "0 16px"
  button-light:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.wine}"
    rounded: "{rounded.control}"
    height: "40px"
    padding: "0 16px"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.on-brand}"
    rounded: "{rounded.control}"
    height: "40px"
    padding: "0 16px"
  action-call:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.wine}"
    rounded: "{rounded.control}"
    height: "48px"
    padding: "12px 18px"
  action-raise:
    backgroundColor: "{colors.brand}"
    textColor: "{colors.on-brand}"
    rounded: "{rounded.control}"
    height: "48px"
    padding: "12px 18px"
  seat:
    backgroundColor: "{colors.surface-seat}"
    textColor: "{colors.paper}"
    rounded: "{rounded.seat}"
    padding: "7px 10px"
  input:
    backgroundColor: "{colors.surface-control}"
    textColor: "{colors.paper}"
    rounded: "{rounded.control}"
    height: "40px"
    padding: "0 12px"
---

# Design System: CTech Poker

> Refreshed from the live `:root` tokens in `src/app/globals.css` and the real component styles in `src/components/table/*`, `src/components/ui/button.tsx`, and `src/components/ui/input.tsx`. Token names here match the code, not the legacy `table-red` / `wine-deep` spellings.

## 1. Overview

**Creative North Star: "The Living Table"**

CTech Poker feels like sitting at a responsive physical table with other people, not operating a static card diagram. Cards arrive with purpose, chips travel between meaningful positions, turns are unmistakable, and the table always acknowledges input. The dark room recedes so the felt, cards, people, and the current decision carry the scene.

This is the vivid member of the CTech product family. It inherits Geist typography, direct pt-BR copy, clear state semantics, familiar controls, restrained radii, accessible focus, and trust-first behavior from Account, Wallet, and DFe — then departs through a dark, table-focused composition and richer state motion. Vivid never means noisy: motion reports play, color reports meaning, decoration never competes with a decision.

The system explicitly rejects PokerStars-style frozen play, casino clutter, predatory prompts, childish gamification, and crypto aesthetics. It should feel social and competitive without manufacturing urgency, and physical without imitating a gaudy casino floor.

**Key Characteristics:**
- A near-black wine room concentrates attention on green felt, warm cards, and people.
- Oxblood red signals the primary action, the viewer, or an active moment — not ambient decoration.
- Gold is reserved for chips, pots, winnings, and earned achievement.
- Motion makes state changes legible and immediate; reduced motion preserves every cue without travel.
- Shared CTech control shapes and typography keep the game credible beside Account, Wallet, and DFe.

## 2. Colors: The Table-at-Night Palette

A dark wine room supports three physical materials — green felt, warm card stock, and gold chips — while oxblood red carries CTech Poker's active signal.

### Primary
- **Table Red** (`#af2a2f`): primary actions, the viewer's seat, focus emphasis, and the strongest active state. A signal, not a background wash. (Code token: `--brand`.)
- **Bright Table Red** (`#d9464d`): hover, focus-border, and small high-attention details. Never body text on the dark canvas. (Code token: `--brand-bright`.)

### Secondary
- **Table Felt** (`#0d5b45`): the playing surface and poker-specific spatial context. Felt is not a generic success color. Rendered as a radial from `#18765b` to `#084b38` (code tokens `--table-felt-light` / `--table-felt-dark`).
- **Chip Gold** (`#e6b85c`): pot values, stacks, wins, and earned rewards. Gold always corresponds to value or achievement; text on gold uses `#30230a`.

### Neutral
- **Night Ink** (`#120d0e`): the global room and base canvas (code token `--ink`; aliased by `--table-room`).
- **Card Paper** (`#f6f0e7`): cards, light actions, and rare high-contrast surfaces. A material cue, not the default page background. (Code token: `--paper`.)
- **Muted Rose** (`#ad9fa0`): secondary copy on dark surfaces. Verified ≥7.3:1 on every dark canvas it touches — safe for small text, but do not reduce its opacity further. (Code token: `--muted`.)
- **Text Secondary** (`#cbbfc0`): the brighter secondary text role (≈10.8:1 on ink); use for seat state labels and readouts where `--muted` reads too dim. (Code token: `--text-secondary`.)
- **Felt Text** (`#e3f1ea`): pot label and readouts on the felt (≥4.5:1 on `#18765b`). Raised from the old `#b8d5c9` after an audit found it failing AA at 3.54:1. (Code token: `--felt-text`.)
- **Success Green** (`#48c98c`): connection, availability, confirmed success; pair with text or an icon. (Code token: `--success`.)
- **Destructive Red** (`#dc2626`): errors and irreversible actions. Separate from Table Red's brand/action meaning. (Code token: `--danger`.)

### Structural / material
- **Deep Wine** (`#5b1218`): pressed red states and warm structural depth around the table (code token `--wine`; aliased by `--table-room-wine`).
- **Table Rail** (`#7c4d2f`) with **Rail Edge** `#291714` and **Rail Highlight** `#b0774a`: the brown physical rail around the felt.
- **Surface Seat** (`#161011`), **Surface Control** (`#211416`), **Surface Control Hover** (`#3e3133`), **Surface Error** (`#3b0b0e`): the layered dark surfaces.
- **Focus Ring** (`#ed777c`): the 3px keyboard-focus ring on dark (≈6.9:1 on ink). (Code token: `--focus-ring`.)
- **Control Border** (`#ffffff24`) / **Seat Border** (`#ffffff26`): the single low-contrast white hairlines.

### Named Rules
**The Three Materials Rule.** Felt means play, card paper means cards or light contrast, and gold means value. Do not spread these colors decoratively across generic UI.

**The Red Signal Rule.** Table Red identifies the primary decision, active player context, or focus. One dominant red signal per decision area is enough.

## 3. Typography

**Display Font:** Geist (`ui-sans-serif, system-ui, sans-serif` fallback)
**Body Font:** Geist (`ui-sans-serif, system-ui, sans-serif` fallback)
**Label/Mono Font:** Geist Mono (`ui-monospace, SFMono-Regular, monospace` fallback)

**Character:** One CTech sans keeps the product familiar and fast to scan. Geist Mono acts as the table readout for pots, stakes, timers, equity, and compact state labels; never a crypto or terminal affectation.

> **Token-drift warning (real gap):** the code references `var(--font-sans)` and `var(--font-mono)` but neither is currently bound — no `next/font` import and no `@theme` definition exist. In practice the app renders the system `sans-serif` / `monospace` fallback, not Geist. Wire Geist (and Geist Mono) via `next/font` in `layout.tsx` and expose `--font-sans` / `--font-mono` so this spec becomes true. The family above is the intended target, not the current render.

### Hierarchy
- **Display** (700, `clamp(2.375rem, 5vw, 3.875rem)`, 1.05, `-0.04em`): lobby / landing titles only. In gameplay, fixed compact sizes preserve table space.
- **Title** (700, `1.5rem`, 1.2): dialogs, panels, prominent lobby values.
- **Body** (400, `0.875rem`, 1.5): instructions, chat, descriptions, supporting content; prose stays within 65–75ch.
- **Label** (600, `0.875rem`, 1.2): buttons, form labels, navigation, action controls.
- **Mono** (600, `0.75rem`, `0.1em` tracking, sizes 9–12px in practice): pots, stakes, timers, chip values, brief system state. Use `tabular-nums` for changing values.

### Named Rules
**The Readout Rule.** If a value changes during play or must align with another value, render it in Geist Mono with tabular numerals. Player names and conversational copy stay in Geist.

**The Game-Space Rule.** Marketing titles may be fluid; table UI uses a fixed, tight type scale so responsive adaptation changes structure rather than shrinking critical labels.

## 4. Elevation

Elevation is structural and material. The table rail and felt use inset shadows to imply depth; seats, cards, dialogs, chat, and toasts lift only because they occupy a distinct physical or transient layer. Ordinary controls and lobby containers stay close to the surface. Glow is reserved for a live connection, the viewer's seat, or a focused active signal.

### Shadow Vocabulary
- **Table depth** (`0 30px 80px #000, inset 0 0 0 4px #b0774a`): the outer rail only.
- **Felt depth** (`inset 0 0 70px #001f18`): the playing surface only.
- **Seat lift** (`0 4px 8px #0009`): compact structural separation for player seats, no wide card-like halo.
- **Viewer signal** (`0 0 0 3px #af2a2f40`): a crisp outer ring for the local player's seat or equivalent active identity.
- **Overlay** (`0 30px 100px #000`): dialogs and high-priority overlays.
- **Transient feedback** (`0 15px 50px #0008`): achievement and result toasts.

### Named Rules
**The Physical Layer Rule.** A shadow must answer "what is above or inset from what?" If it cannot, use a tonal change or hairline instead.

**The One Glow Rule.** Glow means live, focused, or personally active. Never apply ambient glows to a whole card grid or inactive controls.

## 5. Components

The component vocabulary is tactile and immediate: controls look pressable, acknowledge interaction within 150–250 ms, and keep the same shape across marketing, lobby, and play.

### Buttons
- **Shape:** 12px radius, 40px default height, 32px compact, 48px large; semibold 14px labels.
- **Primary:** Table Red fill, white label, 16px horizontal padding. One main decision per group: join, create, raise, accept.
- **Hover / Focus:** hover shifts to Bright Table Red; keyboard focus uses a visible 3px red-derived ring; active may press down by 1px. Disabled uses 50% opacity and no pointer events.
- **Outline / Ghost:** translucent white well or transparent surface with white text. Carry secondary table actions without competing with the primary decision.
- **Light:** Card Paper fill and Deep Wine text for a high-contrast call to action on a red surface.
- **Destructive:** semantic red, icon plus explicit verb, for irreversible actions — not for folding a hand.

> **Token-aligned:** `src/components/ui/button.tsx` and the other primitives (checkbox, dialog, select, input, label, CreateRoomDialog) now reference the `--brand` / `--brand-bright` / `--paper` / `--wine` / `--on-brand` / `--surface-control` / `--muted-rose` / `--danger` tokens instead of hard-coded hex. Only neutral `white/xx` translucency and the destructive `red-500` hover (no token) remain as Tailwind defaults.

### Action Bar (table decisions)
- **Layout:** fixed bottom bar, `grid-template-columns: auto minmax(190px,320px) auto`, centered; 48px-tall buttons; a full-width `action-context` status line above the choices.
- **Quick actions (Fold / Check / Mesa / Pagar):** outline style by default (translucent white well, white text, 1px hairline). Disabled state drops to `--surface-seat` bg with `--muted-rose` text and `not-allowed` cursor — opacity stays 1 so the label remains readable.
- **Call:** paper fill, Deep Wine text — the committed, money-moving action.
- **Raise:** Table Red fill, white text — the primary bet. Paired with a range slider (`accent-color: #af2a2f`) and a tabular-nums `output` showing the chosen total.
- **Error:** `role="alert"` row on `--surface-error`, with a dismiss button.

### Chips
- **Style:** compact rounded labels using either a translucent dark surface or a material color with readable text.
- **State:** gold limited to monetary/chip value and earned rewards; status chips use text plus icon or shape, never color alone.

### Cards / Containers (seats, chat, dialogs)
- **Corner Style:** 14–16px for seats, room rows, chat, and panels. Playing cards use 4–6px to retain their physical shape.
- **Background:** `#161011ef` for seats, `#171011ee` for chat, `#211416` for dialogs and controls.
- **Border:** a single low-contrast white hairline (`--seat-border` / `--control-border`). Viewer/active state may shift the complete border to Table Red; never a thick side stripe.
- **Internal Padding:** 7–10px for seats and dense table controls, 16–24px for panels, 24px for dialogs.

### Player Seat
- **Shape:** 14px container, absolute-positioned around the rail (9 seats, `--seat-0` … `--seat-8`), with a `:after` ring for turn state.
- **Turn:** `is-turn` paints the ring Bright Table Red and pulses it (`turn-signal`, 1.5s). Reduced motion freezes it visible.
- **Viewer:** `viewer` shifts the full border to Bright Table Red and adds the 3px viewer-signal ring.
- **Folded:** grayscale + dashed border, cards dimmed — identity stays readable.
- **Winner:** `is-winner` shifts the border to gold; a gold `seat-win` pill animates in.

### Inputs / Fields
- **Style:** 40px height, 12px radius, translucent control surface, white text, `white/15` hairline.
- **Focus:** border changes to Bright Table Red with a 3px focus-ring at low offset.
- **Error / Disabled:** errors use semantic red and linked error copy; disabled controls retain readable labels. Placeholder text meets 4.5:1 (uses `--muted-rose`, verified 7.38:1 on the seat surface).

### Navigation
- **Style:** sparse top navigation, Geist 14px, brand identity on the left and one or two destination/action links on the right.
- **States:** hover moves muted text to white; current location communicated through text and state, not color alone.
- **Mobile:** secondary links may collapse, but lobby, back, account, and responsible-play exits stay available and keyboard reachable. Table header keeps a 44px-min Lobby link and a connection-state indicator.

### Dialogs
- **Surface:** `#211416`, 16px radius, 24px padding, white/15 hairline, 75% black backdrop.
- **Behavior:** trap focus, close on Escape unless a mutation is pending, label the close button, stack actions on narrow screens when needed.
- **Motion:** fast fade/scale; reduced motion uses an instant change or crossfade.

### The Living Table
The signature composition: a brown physical rail around green felt, cards at the center, seats around the perimeter, decisions anchored below. Cards deal from a consistent source, chips move along comprehensible paths, turn ownership combines position, outline, label, and restrained motion. Latency, reconnecting, waiting, all-in, folded, winner, and audit states are explicit; a frozen-looking table is always a defect.

## 6. Do's and Don'ts

### Do:
- **Do** make every card, chip, turn, connection, and result transition communicate a real state change.
- **Do** keep most interaction transitions between 150–250 ms with exponential ease-out curves; dealing may run up to 450 ms when sequence clarity benefits.
- **Do** preserve the shared CTech family vocabulary: Geist, direct copy, 12–16px control/container radii, explicit states, clear focus, WCAG 2.2 AA.
- **Do** reserve Table Red (`#af2a2f`) for the primary decision, active player context, and focus signal.
- **Do** reserve Felt (`#0d5b45`), Card Paper (`#f6f0e7`), and Chip Gold (`#e6b85c`) for their named physical meanings.
- **Do** pair suit, turn, win, error, and connection colors with text, iconography, shape, or position.
- **Do** provide keyboard play, reduced-motion alternatives, reconnection feedback, and responsive table structure from mobile web through desktop.
- **Do** keep money and sandbox semantics unmistakable and surface fairness/audit information in plain language.

### Don't:
- **Don't** create PokerStars-style static or frozen-feeling play; an unchanged table during latency or processing must show an honest state.
- **Don't** build casino clutter: no flashing chrome, competing promotions, or dense calls to action around the table.
- **Don't** use predatory gambling patterns: no urgency pressure, loss-chasing prompts, manipulative rewards, or obscured money semantics.
- **Don't** use childish gamification that weakens trust in real-money play.
- **Don't** drift into crypto aesthetics: no neon speculation, token hype, terminal styling, or volatility theater.
- **Don't** use `border-left` / `border-right` greater than 1px as an accent on a card, seat, callout, or alert.
- **Don't** combine a decorative 1px border with a wide soft shadow on the same ordinary card or button.
- **Don't** use gradient text, decorative grid/stripe backgrounds, default glassmorphism, or 32px-plus card radii.
- **Don't** use tiny uppercase tracked eyebrows as repeated page scaffolding; mono labels are reserved for real game/readout state.
- **Don't** rely on hue alone to distinguish red and black suits, active turns, winners, errors, or connection status.
- **Don't** animate layout properties or use bounce/elastic motion; never make a user wait for choreography before acting.
- **Don't** hard-code brand colors as literals in components (`bg-[#af2a2f]`); reference the `--brand` / `--paper` / `--wine` tokens so theming stays single-source.
