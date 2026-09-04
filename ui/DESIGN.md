---
name: CTech Poker
description: A vivid, trustworthy social poker room whose interface stays alive around every hand.
colors:
  oxblood-signal: "#af2a2f"
  oxblood-active: "#d9464d"
  deep-wine: "#5b1218"
  night-ink: "#120d0e"
  card-paper: "#f6f0e7"
  chip-gold: "#e6b85c"
  value-ink: "#30230a"
  table-felt: "#0d5b45"
  felt-light: "#18765b"
  felt-dark: "#084b38"
  rail-walnut: "#7c4d2f"
  rail-edge: "#291714"
  rail-highlight: "#b0774a"
  muted-rose: "#ad9fa0"
  text-secondary: "#cbbfc0"
  felt-text: "#e3f1ea"
  success: "#48c98c"
  danger: "#dc2626"
  danger-soft: "#f5b0b3"
  danger-text: "#ef4444"
  focus-ring: "#ed777c"
  on-brand: "#ffffff"
  seat-surface: "#161011"
  control-surface: "#211416"
  control-hover: "#3e3133"
  error-surface: "#3b0b0e"
typography:
  display:
    fontFamily: "IBM Plex Sans, ui-sans-serif, system-ui, sans-serif"
    fontSize: "clamp(52px, 6vw, 82px)"
    fontWeight: 400
    lineHeight: 0.98
    letterSpacing: "-0.04em"
  headline:
    fontFamily: "IBM Plex Sans, ui-sans-serif, system-ui, sans-serif"
    fontSize: "clamp(2.5rem, 5vw, 3.875rem)"
    fontWeight: 700
    lineHeight: 1.02
    letterSpacing: "-0.035em"
  compact-heading:
    fontFamily: "IBM Plex Sans, ui-sans-serif, system-ui, sans-serif"
    fontSize: "clamp(1.75rem, 4vw, 2.25rem)"
    fontWeight: 700
    lineHeight: 1.1
  title:
    fontFamily: "IBM Plex Sans, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 700
    lineHeight: 1.2
  body:
    fontFamily: "IBM Plex Sans, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "IBM Plex Sans, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 600
    lineHeight: 1.2
  readout:
    fontFamily: "IBM Plex Mono, ui-monospace, SFMono-Regular, monospace"
    fontSize: "0.75rem"
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: "0.1em"
  compact:
    fontFamily: "IBM Plex Sans, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.6875rem"
    fontWeight: 600
    lineHeight: 1.2
  tiny:
    fontFamily: "IBM Plex Sans, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.625rem"
    fontWeight: 600
    lineHeight: 1.2
rounded:
  card-thumb: "2px"
  xs: "4px"
  card: "6px"
  sm: "8px"
  control: "12px"
  seat: "14px"
  panel: "16px"
  pill: "999px"
spacing:
  xs: "4px"
  sm: "8px"
  compact: "12px"
  md: "16px"
  lg: "24px"
  xl: "32px"
  2xl: "48px"
  3xl: "64px"
  4xl: "96px"
components:
  button-primary:
    backgroundColor: "{colors.oxblood-signal}"
    textColor: "{colors.on-brand}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    height: "44px"
    padding: "0 16px"
  button-primary-hover:
    backgroundColor: "{colors.oxblood-active}"
    textColor: "{colors.on-brand}"
    rounded: "{rounded.control}"
    height: "44px"
    padding: "0 16px"
  button-outline:
    backgroundColor: "#ffffff0d"
    textColor: "{colors.on-brand}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    height: "44px"
    padding: "0 16px"
  button-light:
    backgroundColor: "{colors.card-paper}"
    textColor: "{colors.deep-wine}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    height: "44px"
    padding: "0 16px"
  button-destructive:
    backgroundColor: "{colors.danger}"
    textColor: "{colors.on-brand}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    height: "44px"
    padding: "0 16px"
  input:
    backgroundColor: "#ffffff0d"
    textColor: "{colors.on-brand}"
    typography: "{typography.body}"
    rounded: "{rounded.control}"
    height: "44px"
    padding: "0 12px"
  dialog:
    backgroundColor: "{colors.control-surface}"
    textColor: "{colors.on-brand}"
    rounded: "{rounded.panel}"
    padding: "24px"
    width: "min(448px, calc(100% - 32px))"
  player-seat:
    backgroundColor: "{colors.seat-surface}"
    textColor: "{colors.card-paper}"
    rounded: "{rounded.seat}"
    padding: "7px 10px"
  action-call:
    backgroundColor: "{colors.card-paper}"
    textColor: "{colors.deep-wine}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    height: "48px"
    padding: "12px 18px"
  action-raise:
    backgroundColor: "{colors.oxblood-signal}"
    textColor: "{colors.on-brand}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    height: "48px"
    padding: "12px 18px"
---

# Design System: CTech Poker

## Overview

**Creative North Star: "The Living Table"**

CTech Poker should feel like joining people around a responsive physical table, not operating a static card diagram. The near-black wine room recedes; felt, cards, chips, people, and the current decision hold the scene. Every meaningful state change is acknowledged through position, copy, contrast, or motion, so speed and fairness remain visible rather than merely promised.

The visual voice is vivid, trustworthy, and social. It combines an expansive, atmospheric public experience with restrained operational pages and a tactile game surface. IBM Plex, direct pt-BR copy, compact controls, explicit states, and disciplined spacing keep the interface credible; material color and purposeful motion give Poker its own identity within the CTech family.

The system rejects frozen-feeling play, casino clutter, manufactured urgency, childish reward language, and crypto-style spectacle. Vivid means present and legible—not louder, shinier, or more promotional.

**Key Characteristics:**

- A dark wine room concentrates attention on the active layer.
- Oxblood marks the primary commitment, active identity, or navigational location.
- Felt, card paper, walnut rail, and chip gold retain physical meanings.
- Structural depth explains how table parts and transient layers relate.
- Tactile controls answer quickly and keep states unmistakable.
- Motion reports play; reduced motion preserves the same information without travel.

### Where the CSS lives

The system ships as a small global foundation plus route-scoped cascades (#84):

| Sheet | Loaded by | Owns |
|---|---|---|
| `src/app/base.css` | root `layout.tsx` (every route) | Tailwind, `:root` tokens, reset, typography, the public shell, error boundaries, `/unavailable`, and shared loading states |
| `src/app/renderer.css` | `(app)/layout.tsx`, the landing page, and `/guide` | the mature poker renderer and the richer surfaces that reuse its cards, seats, boards, and motion |
| `src/app/(app)/app.css` | `(app)/layout.tsx`, after `renderer.css` | rules only the authenticated shell can render — lobby, hands, store, people, profile, achievements, leaderboard and the live table |
| `src/app/(app)/table/table.css` | `(app)/table/layout.tsx` | rules only `/table` can render |
| `src/app/(marketing)/poker-rules/rules.css` | `/poker-rules` layout | only the reference page's chrome, hand-ranking cards, responsive rules, and their required keyframes |

Plus `src/app/(app)/table-reactions.css` (imported by `TableReactions`) and
`src/app/forms-and-gate.css` (`@import`ed by `base.css`).

**Every colour and radius is still a token defined in `base.css`'s `:root`**, so every scoped sheet
resolves the same values. A new rule belongs in the narrowest sheet that can render it. Seat,
board, card and hand-outcome rules remain shared by the authenticated subtree because `/hands`,
`/hands/history`, `/hands/replay`, `/share`, `/profile` and `/lobby` render those components; the
landing and guide opt into that same cascade explicitly. `/poker-rules` instead owns a compact
extraction, keeping the live felt/rail/seat renderer and `board-deal` off its critical path.

On `/table`, `base.css → renderer.css → app.css → table.css` preserves the pre-split cascade order.
Moving a rule between these sheets still requires checking equal-specificity overrides; a later,
narrower sheet wins.

## Colors

The palette reads as a cardroom after dark: oxblood signals intent, green felt locates play, warm paper carries cards and contrast, walnut gives the table weight, and gold represents value or earned outcomes.

### Primary

- **Oxblood Signal** (colors.oxblood-signal): primary actions, desktop viewer identity, selected controls, and the strongest brand signal. Use it sparingly inside any one decision area.
- **Oxblood Active** (colors.oxblood-active): hover borders, small active details, and focused emphasis. It is an accent, not small body text.
- **Deep Wine** (colors.deep-wine): warm structural depth, pressed red contexts, and the atmospheric public canvas.

### Secondary

- **Table Felt** (colors.table-felt, with colors.felt-light and colors.felt-dark): the playing surface and poker-specific spatial context. Felt does not stand in for generic success.
- **Chip Gold** (colors.chip-gold): pots, stacks, wins, turn timing, and earned achievements. Pair filled gold with colors.value-ink.
- **Walnut Rail** (colors.rail-walnut, edged by colors.rail-edge and lifted by colors.rail-highlight): the physical boundary between the room and play.

### Tertiary

- **Success** (colors.success): availability, connection, and confirmed completion; always pair it with text, an icon, shape, or position.
- **Danger** (colors.danger): destructive fills, error icons, and irreversible actions. Small error copy uses colors.danger-text; supporting warning copy uses colors.danger-soft.
- **Focus Ring** (colors.focus-ring): the universal high-contrast keyboard focus signal on dark surfaces.

### Neutral

- **Night Ink** (colors.night-ink): the global room and base page canvas.
- **Card Paper** (colors.card-paper): playing cards, light action buttons, and rare high-contrast material surfaces—not the default page background.
- **Seat Surface** (colors.seat-surface): compact player identity and dense table modules.
- **Control Surface** (colors.control-surface) and **Control Hover** (colors.control-hover): fields, dialogs, action controls, and their lifted interaction state.
- **Muted Rose** (colors.muted-rose): secondary copy that remains readable on dark surfaces. Do not fade it further for essential information.
- **Text Secondary** (colors.text-secondary): brighter supporting labels and state readouts.
- **Felt Text** (colors.felt-text): labels and values placed directly over the brightest felt.

### Named Rules

**The Three Materials Rule.** Felt means play, card paper means cards or deliberate light contrast, and gold means value, time, or achievement. Never distribute these colors as generic decoration.

**The Oxblood Signal Rule.** Oxblood identifies the primary commitment, current location, or active identity. One dominant oxblood signal per decision group is enough; a player's turn is gold so it cannot be confused with danger.

**The Semantic Pair Rule.** Suit, connection, turn, win, and error colors always travel with text, iconography, shape, or position. Hue alone never carries game state.

## Typography

**Display Font:** IBM Plex Sans, with ui-sans-serif, system-ui, and sans-serif fallbacks

**Body Font:** IBM Plex Sans, with ui-sans-serif, system-ui, and sans-serif fallbacks

**Label/Mono Font:** IBM Plex Mono, with ui-monospace, SFMono-Regular, and monospace fallbacks

**Character:** IBM Plex Sans is direct, contemporary, and highly legible without becoming anonymous. IBM Plex Mono acts as the table readout for stakes, pots, timers, equity, and aligned values; it is an instrument, never a terminal aesthetic.

### Hierarchy

- **Display** (typography.display): public landing statements only. Its fluid scale earns attention without entering dense game UI.
- **Headline** (typography.headline): editorial arrivals such as the guide hub and rare feature pages.
- **Compact Heading** (typography.compact-heading): task pages, lobby, store, ranking, people, and other operational surfaces.
- **Title** (typography.title): dialogs, panels, room names, and prominent values.
- **Body** (typography.body): instructions, descriptions, chat, and supporting content. Long prose stays near 65–75 characters per line.
- **Label** (typography.label): buttons, navigation, form labels, and actions.
- **Readout** (typography.readout): live or aligned numeric state, with tabular numerals when values change.
- **Compact / Tiny** (typography.compact, typography.tiny): closed-vocabulary table labels and dense status only; never general prose.

The table permits an evidence-backed micro-scale below the token floor only inside constrained geometry: 10px opponent names, 11px stacks, and 8–9px single-token badges or card hints. Nothing rendered in CSS pixels goes below 8px, and unbounded names or localized sentences never use this scale.

### Named Rules

**The Readout Rule.** If a value changes during play or must align with another value, use IBM Plex Mono with tabular numerals. Player names and conversational copy stay in IBM Plex Sans.

**The Game-Space Rule.** Marketing and editorial headings may be fluid. Table UI adapts its structure instead of shrinking critical labels below readability.

## Layout

Public and authenticated content share a centered shell capped at 1160px, with 20px desktop gutters and 16px mobile gutters. Public landing layouts use generous vertical sections, asymmetric two-column storytelling, and a responsive table preview. Operational pages use a sticky 72px application bar, compact page headings, and scannable grids or lists; only true editorial arrivals opt into centered feature headings.

The live table is a distinct blocking layout. It occupies the available 100dvh, subtracts the global API-unavailable strip when present, and divides the viewport into table chrome, a flexible stage, and the action dock. Desktop and landscape use an oval rail up to 920px wide. Portrait handhelds use a dedicated vertical seat ring; the viewer leaves the ring and becomes a bottom hero HUD so identity and actions stay readable.

Responsive adaptation is structural. At 600px, the seven authenticated routes leave the top bar for a fixed four-slot bottom navigation; every page using it reserves bottom safe-area clearance. Table actions collapse into an opaque bottom sheet at 800px or in short landscape. Content indexes and guide topics become native disclosures near 720px. Compact 520px and 380px tiers resolve dense components without horizontal page scrolling. Touch targets remain at least 44×44px even when their visible badge or icon is smaller.

Spacing follows a 4px base rhythm with recurring 8, 12, 16, 24, 32, 48, 64, and 96px steps. Table geometry may use tighter physical offsets, but content and controls return to the shared rhythm.

### Named Rules

**The Blocking Table Rule.** Portrait play owns exactly one viewport: header, flexible table, viewer HUD, and safe-area action dock. Do not add document scroll to solve table crowding.

**The Structure-Before-Shrink Rule.** Reflow, disclose, or relocate before reducing type or touch targets. Critical actions never fall below 44px.

**The Stable Seat Rule.** Seat positions remain stable during a hand; vacancies fill in server turn order, and hidden cards render exactly as received.

**The Seat Overhang Rule.** A seat's corner badges hang outside the seat box by exactly `--seat-badge-overhang` (8px), and every container that clips a seat reserves at least that much padding on the edges it clips. The portrait stage's own padding, and the outward hang of the mid-side captions, are both written in terms of that one token, so no tier can reserve less room than a badge needs. The reported symptom of breaking this is a win/loss streak badge sliced in half on the viewer's dock.

**The Caption Lane Rule.** The portrait seat caption (the stack figure under the avatar) is sized from `--seat-caption-lane` (64px, narrowing to 56px below 360px), never from a per-seat literal. Captions ellipsise; they do not widen the lane, and they do not reach past the stage.

## Elevation & Depth

Depth is structural. The rail sits above the room, felt is inset within the rail, seats lift slightly from the table, and dialogs or transient feedback occupy a clearly higher layer. Ordinary page cards remain close to the canvas through tonal layering and a single hairline. Glow is reserved for focus, live connection, turn timing, the viewer, or a resolved win.

### Shadow Vocabulary

- **Table Depth** (--shadow-table): 0 30px 80px #000 plus a 4px inset rail highlight; outer table only.
- **Felt Depth** (--shadow-felt): an inset 70px dark-green shade; playing surface only.
- **Seat Lift** (--shadow-seat): a compact 0 4px 8px #0009 separation for player seats.
- **Modal Lift:** a strong overlay shadow behind the 16px dialog surface and a 75% black, lightly blurred backdrop.
- **Signal Glow:** a crisp ring or tight glow attached to focus, live status, turn timing, viewer identity, or winner feedback—not ambient decoration. The achievement-arrival ring on the "Recém-desbloqueadas" rail is the earned-value member of this family and uses `--gold-33` (33% Chip Gold, the gold counterpart of `--brand-33`); it expands and fades twice, and reduced motion keeps the gold border while dropping the pulse.

Motion reinforces the same layer model. Most state changes complete in 120–250ms with quartic or quintic ease-out. Entry choreography may reach about 420ms; card dealing can run longer when sequence itself conveys information. Reduced motion removes travel, pulsing, and sequential waits while retaining visible state.

### Named Rules

**The Physical Layer Rule.** Every shadow must answer what is above or inset from what. If it cannot, use a tonal shift or hairline.

**The One Glow Rule.** Glow means live, focused, personally active, timed, or victorious. Inactive card grids and ordinary controls do not glow.

**The Information Motion Rule.** Motion may explain dealing, collection, turn ownership, or panel state. It never delays the next valid action.

## Shapes

The system uses compact, gently curved geometry. Playing-card thumbnails use 2px corners, physical cards 6px, small controls 8px, standard controls 12px, seats 14px, and panels or dialogs 16px. Pills are reserved for categorical filters, compact statuses, and values that benefit from a continuous capsule. Avatars and icon-only voice controls are circular.

Borders are single low-contrast hairlines at rest. Active, viewer, winner, error, or focus states may replace the whole border or add a complete ring. Thick side accents, stacked ornamental borders, and oversized 32px-plus radii do not belong to this system. Playing cards and the oval table are the two signature silhouettes; generic content containers should not imitate either.

### Named Rules

**The Complete Outline Rule.** State surrounds the component with a complete border or ring. Never bolt a thick accent stripe onto one edge.

**The Radius Ladder Rule.** Use the established 2/4/6/8/12/14/16px ladder or the full pill. A new radius requires a reusable geometric role.

## Components

Components feel tactile and decisive: pressable, fast, explicit about state, and consistent across public, operational, and table contexts.

### Buttons

- **Shape:** 12px corners, 44px default height, 48px large height, semibold 14px labels, and 16px horizontal padding.
- **Primary:** Oxblood Signal with white text; reserve it for the principal commitment in a group.
- **Outline / Ghost:** translucent or transparent dark controls with white text for secondary actions.
- **Light:** Card Paper with Deep Wine text for a high-contrast action on an oxblood surface and for the table's call action.
- **Destructive:** semantic Danger with an explicit verb; do not use brand red as a substitute.
- **States:** hover changes surface, focus receives the 3px focus ring, active presses by 1px, and disabled stays readable while losing interaction.
- **Pending:** `<Button loading>` is the one pending affordance — a spinner ahead of the label, `aria-busy` for the
  screen reader, and `disabled` so a second click cannot fire the same request twice. Swap the label to the present
  progressive ("Saindo…", "Verificando…", "Buscando…"); never drop it, or the button resizes mid-press. No screen
  hand-rolls `disabled={isPending}` plus a manual spinner.

### Chips

- **Style:** pill geometry, compact label, hairline or tonal fill, and at least a 44px interaction height when clickable.
- **Selected:** Oxblood Signal with white text for filters; gold with Value Ink for table preselection or value-bearing state.
- **Semantics:** status chips pair color with copy or icon. The six private player-note tag hues form a user-assigned categorical palette and are never reused as product status colors.

### Cards / Containers

- **Room and content cards:** 16px corners, one low-contrast border, dark tonal fill, and 16–24px internal padding. Hover may lift up to 5px when the whole card is actionable.
- **Player seats:** 14px corners, Seat Surface, a one-pixel border, 7×10px padding, and compact shadow. Turn and winner use gold; desktop viewer identity uses oxblood.
- **Playing cards:** warm card stock, 6px physical corners, and proportional 2px corners only at thumbnail scale. Archives reveal together; live play and replay may retain deal sequence.
- **Empty states:** dashed 1px outline, plain explanation, and one clear recovery or creation path.

### Table Reactions

- **Picker:** Keep favorites first, then split the catalog into “Na minha cadeira” and “Mandar para alguém.” The
  two-mode structure protects the timed poker decision from a 30-option wall while keeping every reaction directly
  discoverable.
- **Identity:** Every reaction owns a label, a short supporting line, a color family, and one recognizable
  choreography. Self tells rise from the sender's seat; directed gestures travel in an arc and resolve at the
  recipient's seat with a distinct terminal beat (splat, banner, ring, catch — never a plain fade).
- **Voice:** Labels name the gesture; captions carry the table's acid friendly-fire humour (pt-BR, e.g. “Chora mais,
  campeão”, “Meus pêsames”). Sincere gestures (respect, flowers-as-tribute is the exception) stay warm.
- **Reaction fx colours:** the standard `--tag-*` / `--gold*` / felt families carry every effect except one — the
  “Jogar cocô” splat uses `--reaction-muck` / `--reaction-muck-deep`, a dedicated muck brown that is deliberately not
  a rail/wood surface token.
- **Scale:** Effects stay within roughly one seat's visual territory and complete inside the existing ephemeral
  reaction lifetime. They may briefly celebrate a seat, but never move cards, alter table state, or delay an action.
- **Control:** The eye toggle hides all transient effects and persists locally. Locked, owned, loading, unavailable,
  disconnected, cooldown, no-opponent, and targeting states remain explicit.
- **Reduced motion:** Skip travel and particles. Place the reaction's identifying glyph directly at its final
  seat so the same social information remains visible.

**The Seat-Anchored Theater Rule.** A reaction begins with the player who sent it and, when directed, ends with the
player who receives it. Spectacle may decorate that relationship; it never becomes an unrelated full-screen event.

### Inputs / Fields

- **Style:** 44px height, 12px corners, translucent dark fill, white text, muted placeholder, and a one-pixel hairline.
- **Focus:** Oxblood Active border plus a visible 3px Focus Ring.
- **Error:** semantic red border with linked readable error copy; do not rely on border color alone.
- **Disabled:** preserve the label and explanation instead of collapsing to opacity-only ambiguity.
- **Field:** `<Field label description error>` is the labelled-control primitive. It owns the generated ids and wires
  them with `aria-describedby` (description **and** error together, so the hint is not lost the moment the field goes
  invalid) plus `aria-invalid` / `aria-errormessage`. It takes a render prop because the control varies — a plain
  `Input`, a search row with a trailing button, a range — while the association rules do not. A labelled control that
  wires its own `aria-describedby` by hand is a bug, not a variant.

### Navigation

- **Desktop:** sparse sticky top bar; brand on the left, route destinations and profile on the right. Each destination owns a 44px target, 12px corners, icon plus label, and a tonal current-location state.
- **Mobile:** top bar keeps brand and profile; a fixed four-slot bottom bar exposes Lobby, Pessoas, Loja, and a context-aware More destination. It respects safe-area insets and never overlays the last actionable row.
- **Public:** the landing header may simplify to brand, anchor links, and Entrar. Public profiles retain the shared application chrome when authenticated.

### Dialogs

- **Surface:** Control Surface, 16px corners, 24px padding, white hairline, strong modal shadow, and a 75% black blurred backdrop.
- **Sizing:** width is capped near 448px by default; height never exceeds the viewport minus 32px. Content scrolls internally with overscroll containment.
- **Behavior:** focus is trapped, close is labeled, Escape works unless a pending mutation makes dismissal unsafe, and narrow-screen actions stack.

### The Living Table

The signature composition places a walnut oval or portrait ring around radial green felt, cards at center, seats around the perimeter, and decisions in a bottom dock. Cards deal from a consistent source, chips move along comprehensible paths, current turn combines gold outline, timing, and copy, and the local viewer remains easy to identify without competing with errors. Reconnection, waiting, all-in, folded, winner, audit, and API-unavailable states are explicit; a frozen-looking table is a defect.

The action dock separates decision roles: neutral dark buttons for fold/check, Card Paper for call, and Oxblood Signal for raise. Desktop retains sliders, presets, and shortcuts. Compact layouts open an opaque raise sheet with presets and 48px step controls; horizontal gestures are never required.

**Felt wordmark.** The `{PokerLogo} CTECH` house mark crowns the board group on both stages (and in replay), matching the landing hero's table preview. It is decorative (`aria-hidden`; the stage name lives on the page `h1`) but not a watermark: the `CTECH` lettering is `on-brand` **white** and the P monogram keeps its oxblood, so the table reads as CTech's from across the room. White, never gold — the Three Materials Rule reserves gold for value, time, and earned outcomes, and a gold mark on the play surface would be decoration wearing value's colour. The whole lockup scales from one local token (`--felt-wordmark-mark`, 26px on the oval, 19px on the portrait capsule). It carries no filter, and it travels with the board rather than with the felt.

**The felt's bands.** Everything on the felt owns a horizontal band and nothing crosses another's. Top arc: nothing of ours — it is the lane the top-row seats' bet chips travel down through, and anything parked there is covered on every raise. Middle-top: the transient dealer call (`.table-callout`). Centre: the board group (`.felt-center`) — the wordmark hung directly above the pot readout, the board, and the street rail hung directly beneath it, never pinned to the felt's bottom edge, because that lower band belongs to the bottom-row seats' bet chips travelling up toward the pot. Anything new on the felt attaches to the board group, never to a fresh percentage of the felt: the percentages are where the chips move.

**Docked asides.** Chat, reactions and last winners are one pattern, not three: a 45px round toggle pinned to the bottom of a `column-reverse` stack with its panel growing upward off it (dropping downward in the portrait rail, where the toggles live at the top). Hover opens them through `useHoverPanel`, which gives the close a grace period, and `.table-aside-skirt` widens the hit area so the pointer can travel from button to panel without falling out. A fourth docked aside uses the same three pieces or it does not ship.


## Do's and Don'ts

### Do:

- **Do** make every card, chip, turn, connection, result, and recovery transition communicate a real state change.
- **Do** keep Oxblood Signal rare enough to identify the primary commitment or active location immediately.
- **Do** reserve felt for play, paper for cards or deliberate light contrast, and gold for value, time, and earned outcomes.
- **Do** use IBM Plex Mono with tabular numerals for changing values and IBM Plex Sans for names and language.
- **Do** pair color-coded suits and states with text, shape, iconography, or position.
- **Do** keep keyboard focus visible, preserve 44px touch targets, and provide reduced-motion equivalents.
- **Do** keep sandbox and wallet semantics explicit; the interface must never imply real-money mode is enabled by default.
- **Do** render hidden cards exactly as delivered and let browser-side fairness checks determine verification.

### Don't:

- **Don't** create static or frozen-feeling play; latency and processing require honest visible state.
- **Don't** build casino clutter, flashing chrome, competing promotions, urgency pressure, or loss-chasing prompts.
- **Don't** drift into crypto aesthetics, terminal theater, childish gamification, gradient text, or decorative grid backgrounds.
- **Don't** use thick side accents, decorative double borders, ambient card glows, or oversized container radii.
- **Don't** use tiny uppercase tracked eyebrows as repeated task-page scaffolding; reserve mono labels for real readout or category state.
- **Don't** shrink critical labels or controls to fit. Reflow, disclose, or relocate first.
- **Don't** animate layout properties, use elastic motion, or make users wait for choreography before acting.
- **Don't** hard-code semantic brand colors inside components when the live CSS token already exists.
