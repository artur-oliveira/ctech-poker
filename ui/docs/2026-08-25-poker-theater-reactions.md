# Poker Theater table reactions

Date: 2026-08-25

## Outcome

`TableReactions` now treats reactions as seat-anchored poker moments rather than a flat emoji tray. The picker keeps
three favorites first and separates the complete catalog into two modes:

- **Na minha cadeira:** sends immediately and renders above the sender.
- **Mandar para alguém:** selects a pending gesture, closes the picker, and asks the player to choose an occupied
  opponent seat.

The wire shape did not change. The browser still sends `reaction_id` and optional `target_player_id` through
`useTableRealtime`; the server remains authoritative for known IDs, targeting shape, premium ownership, cooldown, and
persistence.

## Catalog and choreography

### Self tells

| ID          | Label            | Visual identity                                 |
|-------------|------------------|-------------------------------------------------|
| `clap`      | Aplausos         | Snapping BRAVO banner with radial gold applause |
| `laugh`     | Risada           | Bouncing face with staggered HA bubbles         |
| `wow`       | Uau              | Blue exclamation and concentric shock rings     |
| `angry`     | Raiva            | Tight seat shake, heat pulse, and steam jets    |
| `cry`       | Choro            | A curtain of falling tears                      |
| `nervous`   | Nervoso          | Trembling glyph with orbiting sweat drops       |
| `cold`      | Frio na mesa     | Growing frost crystal and snow orbit            |
| `fire`      | Sequência quente | Flame crown and fanned embers                   |
| `respect`   | Respeito         | Gold GG mark, opening laurel, and sparks        |
| `sleepy`    | Sono             | Slow staggered dream letters                    |
| `heartbeat` | Coração all-in   | Pulsing heart, ALL IN callout, and ECG scan     |
| `shark`     | Modo tubarão     | Passing fin, felt-blue wake, and table waves    |
| `pokerface` | Cara de pôquer   | Dropping shades with orbiting card suits        |

### Directed gestures

| ID          | Label           | Visual identity                                            |
|-------------|-----------------|------------------------------------------------------------|
| `chip`      | Jogar ficha     | Arcing chip followed by a twelve-stack jackpot burst       |
| `coffee`    | Mandar café     | Cup landing with three rising steam trails                 |
| `clover`    | Dar sorte       | Clover bloom, green seat glow, and lucky orbit             |
| `horseshoe` | Jogar ferradura | Weighted horseshoe slam and gold star ring                 |
| `tear`      | Jogar lágrima   | Targeted rain shower and expanding puddle                  |
| `tomato`    | Jogar tomate    | Felt splat, scattered drops, and impact word               |
| `poop`      | Jogar cocô      | Playful plop with a soft rising stink cloud                |
| `rofl`      | Rir da cara     | Rolling laughter trio and HA callout                       |
| `duck`      | Jogar pato      | Waddling duck, QUACK callout, and falling feathers         |
| `turtle`    | Chamar de lento | Deliberately slow turtle arrival with TANK dreams          |
| `knife`     | Jogar faca      | Red impact flash followed by eight descending bleed trails |
| `flowers`   | Mandar flores   | Bouquet opening with falling petals                        |
| `spotlight` | Boa leitura     | Sweeping table spotlight, READ mark, and gold sparks       |
| `crown`     | Passar a coroa  | Crown landing, REI plaque, and orbiting suits              |
| `bandage`   | Curar bad beat  | Bandage seal, green recovery ring, and hearts              |

The six new IDs are `heartbeat`, `shark`, `pokerface`, `spotlight`, `crown`, and `bandage`. They are free. The API's
`internal/reactions/catalog.go` whitelist mirrors the frontend keys; adding a frontend key without that server entry
would make production reject the socket action.

## Interaction states

- Disconnected and cooldown states disable every catalog choice.
- Directed gestures are disabled when no opponent is seated.
- Premium loading, locked, owned, unavailable, and refunding states keep their previous entitlement behavior.
- Pending targeting names the chosen reaction and offers a one-tap cancel path.
- The visibility toggle persists at `poker:table-reactions-muted`; it suppresses rendering only, not transport.
- The favorites editor still accepts up to three IDs and can include a locked premium reaction.

## Accessibility and performance

- Picker buttons expose the full label and supporting line as their accessible name.
- The two scopes use a labeled tablist and tabpanel.
- Header and picker controls retain at least 44px targets.
- Effects are decorative to assistive technology beyond the reaction's single `role="img"` label.
- `prefers-reduced-motion` removes travel and particles and places the identifying glyph at its final seat.
- Effects use CSS transforms, opacity, clip paths, and filters only. No animation library, audio, canvas, or WebGL is
  initialized, and the existing 3.4-second realtime expiry still bounds every effect.
- The panel was measured at 320×568, 360×640, 375×667, 390×844, and 430×932 with no document overflow.

## Verification

- `src/components/table/TableReactions.test.tsx` covers both scopes, every catalog identity, source/target positioning,
  favorites, visibility persistence, targeting cancellation, premium states, disconnection, cooldown, no-opponent, and
  missing-seat paths.
- API catalog tests assert the same targeting shape and complete ID set.
- Manual mock-table checks cover the desktop picker, Coração all-in, Passar a coroa, and the five documented portrait
  viewports.
