---
name: CTech Poker CLI
description: A quiet, keyboard-first cardroom built for the terminal.
colors:
  dealer-gold: "#e6b85c"
  table-felt: "#18765b"
  action-red: "#d9464d"
  terminal-silver: "#8a8a8a"
  terminal-charcoal: "#585858"
  hearts-red: "#ff0000"
  diamonds-blue: "#0000ff"
  clubs-green: "#00ff00"
  spades-white: "#ffffff"
typography:
  title:
    fontFamily: "User-configured terminal monospace"
    fontWeight: 700
  body:
    fontFamily: "User-configured terminal monospace"
    fontWeight: 400
  label:
    fontFamily: "User-configured terminal monospace"
    fontWeight: 700
components:
  title:
    textColor: "{colors.dealer-gold}"
    typography: "{typography.title}"
  prompt:
    textColor: "{colors.dealer-gold}"
    typography: "{typography.label}"
  menu-selected:
    textColor: "{colors.dealer-gold}"
    typography: "{typography.label}"
  status-success:
    textColor: "{colors.table-felt}"
    typography: "{typography.body}"
  status-error:
    textColor: "{colors.action-red}"
    typography: "{typography.body}"
---

# Design System: CTech Poker CLI

## Overview

**Creative North Star: "The Quiet Cardroom"**

The interface should feel like sitting at a composed card table inside the user's own terminal: the game is present, social, and alive, but never louder than the decision in front of the player. It borrows the direct command fluency of Claude Code and Codex CLI, then uses poker language, card symbols, and concise narration to establish its own identity.

The system is flat, compact, and terminal-native. Hierarchy comes from ordering, whitespace, rules, weight, and a small semantic palette—not simulated GUI surfaces. The table state, legal actions, and recovery state must remain legible before personality is added. It explicitly rejects the noisy casino interface named in PRODUCT.md.

**Key Characteristics:**

- Restrained gold, felt green, and red semantic accents on the user's terminal background.
- Monospace alignment and terminal-cell rhythm instead of pixel-based layout.
- Keyboard-first menus and slash commands with state always expressed in words or symbols as well as color.
- Poker character delivered through narration and card notation, not decorative spectacle.
- Width- and height-safe composition that protects the active command surface.

## Colors

The palette is a restrained cardroom vocabulary: warm gold directs attention, felt green confirms healthy play states, and red is reserved for failure or danger.

### Primary

- **Dealer Gold** (`#e6b85c`): Titles, the prompt, selection markers, the user's identity, active URLs, and spinners. It identifies what can be acted on or what currently owns attention.

### Secondary

- **Table Felt** (`#18765b`): Successful authentication, the sandbox badge, and the player's active-turn indicator. It carries table confidence without turning the whole terminal green.

### Tertiary

- **Action Red** (`#d9464d`): Errors and danger only. It is never general chrome because red also belongs to cards and future real-money states require unambiguous risk signaling.

### Neutral

- **Terminal Silver** (`#8a8a8a`, ANSI 256 index `245`): Secondary descriptions, hints, and reconnecting notices.
- **Terminal Charcoal** (`#585858`, ANSI 256 index `240`): Rules, borders, tertiary instructions, and supporting achievement descriptions.
- **Terminal Default:** Unstyled primary copy inherits the user's terminal foreground and background so the interface respects local contrast and theme choices.
- **Suit Colors:** Hearts use bright red, diamonds bright blue, clubs bright green, and spades bold white in color-card mode. ASCII card mode remains the color-independent fallback.

### Named Rules

**The Three Signals Rule.** Gold means attention, felt green means healthy or active, and Action Red means error or danger. Never trade these roles for decoration.

**The Terminal Owns the Canvas Rule.** Never paint a global background. Inherit the user's terminal theme and apply color only to semantic fragments.

**The Suit Isolation Rule.** Suit colors belong to card glyphs only. Never reuse bright ANSI suit colors for navigation, status, or branding.

## Typography

**Display Font:** User-configured terminal monospace

**Body Font:** User-configured terminal monospace

**Label/Mono Font:** User-configured terminal monospace

**Character:** One terminal typeface carries the entire interface. Hierarchy is created with bold weight, concise labels, indentation, and alignment so rendering stays predictable across Linux, macOS, and Windows terminals.

### Hierarchy

- **Title** (bold, one terminal cell): Product identity, room name, choice-step headings, and command-list headings.
- **Body** (regular, one terminal cell): Game narration, profile data, commands, table state, and explanatory copy.
- **Label** (bold, one terminal cell): Selected options, the `›` prompt, `VOCÊ`, `SANDBOX`, and `SUA VEZ`.
- **Muted annotation** (regular, one terminal cell): Hints, descriptions, recovery context, and secondary status.

### Named Rules

**The One-Cell Rule.** Never simulate typographic scale with ASCII art, double-width headings, or ornamental banners. Use weight and position; preserve terminal density.

**The Portuguese Interface Rule.** Player-facing interface copy remains concise Brazilian Portuguese. Protocol vocabulary and conventional poker terms may remain English where they are already understood.

## Elevation

This system uses no shadows and no simulated elevation. Depth is structural: a full-width rule separates the persistent table header, scrollback, and input; indentation groups narration; the selection marker identifies the active row. The command menu receives available rows before scrollback so the user's active choice is never pushed out of view.

### Named Rules

**The Flat Table Rule.** Never imitate cards, modals, glass panels, or floating GUI controls with box-drawing decoration. A border is permitted only when it describes a real region, such as the compact home identity block.

**The Active Surface Rule.** When vertical space is contested, protect the input and command menu first, then reduce scrollback. Interaction outranks history.

## Components

### Home Identity

- **Character:** Compact, calm, and recognizably CTech Poker.
- **Wide terminals:** A three-row outlined header capped at 72 columns combines `♠ CTech Poker`, a `SANDBOX` badge, and a one-line product description.
- **Narrow terminals:** Below 38 columns it collapses to the single-line `♠ CTech Poker · sandbox` identity.
- **Color:** Brand and title use Dealer Gold; the sandbox state uses Table Felt; supporting structure uses Terminal Charcoal and Terminal Silver.

### Command Prompt

- **Style:** A bold Dealer Gold `›` followed by the terminal-native text field.
- **Placeholder:** `/ para ver comandos` teaches discovery without persistent chrome.
- **Focus:** The Bubble Tea input cursor and terminal focus conventions remain authoritative; do not invent a GUI focus ring.
- **Error / Disabled:** Validation appears as a separate Action Red narration line. Busy states keep the prompt context visible and pair a gold spinner with a verb.

### Command Menu

- **Style:** A text list with aligned command names and descriptions, capped at eight visible items.
- **Selected:** Dealer Gold bold text with a leading `›`; unselected options use Terminal Silver.
- **Navigation:** Arrow keys move, Tab completes, Enter accepts, and Escape closes. A concise footer teaches these keys.
- **Constrained state:** The menu clips to available terminal height, follows the active selection, and reports hidden rows above or below.

### Choice Lists

- **Style:** Step headings use Dealer Gold bold; the selected row uses a gold `›`; secondary explanation is muted.
- **Behavior:** Arrow keys change selection, Enter confirms, and Escape moves back. Boolean choices also support Space.
- **Shape:** No container, fill, or simulated button. The cursor and text weight are the affordance.

### Table Header

- **Style:** Three dense lines summarize room, Hold'em variant, blinds, occupancy, pot, board, hand strength, equity, positions, turn timer, and legal actions.
- **Hierarchy:** Room name uses Dealer Gold; `VOCÊ` uses Dealer Gold bold; `SUA VEZ` uses Table Felt bold. Legal actions are always written out after an arrow.
- **Boundaries:** Terminal Charcoal rules separate the header, scrollback, and prompt.
- **Responsive behavior:** Content must never cause the full view to exceed terminal width or height; concise truncation or structural reflow takes priority over completeness on one line.

### Narration and Status

- **Style:** Plain-language event lines form a chronological scrollback. Indentation groups bets, folds, streets, showdown, chat, reactions, achievements, and exits.
- **Success:** Table Felt text plus explicit language such as `Logado.` or `● SUA VEZ`.
- **Error:** Action Red text prefixed with `erro`, `ação inválida`, or a middle dot; color never carries the meaning alone.
- **Recovery:** Reconnecting retains the last table view and adds a muted written notice. Reconnected and retried actions are narrated in place.

### Cards

- **Color mode:** Compact rank-and-suit notation uses isolated ANSI suit colors.
- **ASCII mode:** A fully color-independent representation selected through configuration or accessibility needs.
- **Behavior:** Hole cards, board cards, strength, and approximate equity appear together when available.

## Do's and Don'ts

### Do:

- **Do** use Dealer Gold only for identity, focus, selection, progress, and the player's own position.
- **Do** express every state with wording or a symbol in addition to color.
- **Do** keep keyboard instructions close to the interaction they control.
- **Do** prioritize the current table state, legal actions, and input when terminal space is scarce.
- **Do** narrate social events and outcomes concisely so poker personality emerges from play.
- **Do** preserve `NO_COLOR`, ASCII card mode, and usable layouts in narrow and short terminals.

### Don't:

- **Don't** resemble a noisy casino interface; prohibit ornamental spectacle, blinking chrome, decorative animation, and competing accent colors.
- **Don't** use Action Red for ordinary emphasis, navigation, or cardroom atmosphere.
- **Don't** rely on color alone to identify errors, turns, selections, suits, or connection state.
- **Don't** simulate GUI cards, buttons, shadows, glass surfaces, or modal overlays with box-drawing characters.
- **Don't** let a menu item, header, or narration line silently force terminal wrapping and desynchronize rendering.
- **Don't** replace direct poker terminology with cute copy when the player needs to decide quickly.
