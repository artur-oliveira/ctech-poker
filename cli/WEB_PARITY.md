# Web parity

Audited against `ui/` on 2026-09-06. The CLI aims for behavioral parity where
the interaction makes sense in a terminal; it does not reproduce browser-only
presentation.

## Available in the CLI

- Full sandbox table loop, keyboard actions, chat, reactions, peek privacy,
  showdown controls, run-it-twice preference, rabbit hunt, preselection,
  player notes, session summary, and resilient reconnect/exit behavior.
- Career profile and achievement ledger.
- Friends, incoming/outgoing requests, blocked players, recent opponents, and
  social activity reads. Every list supports cursor pagination.
- Interactive hand archive with cursor pagination, per-page W/E/D and net
  summary, day grouping, hand detail, cards, opponents, action timeline, and
  complete/partial shuffle-proof data.

## Remaining web capabilities

### Hand study

- Outcome/table filters and saved filters.
- Review markers, per-street notes, and named collections.
- Step-by-step visual frame replay.
- Local cryptographic verification of complete and partial deck proofs. The CLI
  currently exposes the proof inputs but deliberately does not label them as
  verified.
- Hand export, public share creation/revocation, and paid winner-card reveal.

### People and safety

- Lookup/add by friend code; accept, decline, cancel, or remove friendship.
- Mute/unmute, block/unblock, table invites, inbox read state, and player
  reports. The CLI currently provides the read-side ledgers and joinable-room
  shortcuts only.
- Public showcase and head-to-head matchup views.

### Progression and account

- Leaderboards and the player's exact global rank.
- Lifetime VPIP, PFR, and 3-bet statistics.
- Profile editing, privacy controls, featured achievements, playstyle badges,
  favorite reactions/bet presets, avatar management, and showcase layout.
- Daily sandbox reward.

### Store and table utilities

- Sandbox-chip, reaction, and cosmetic catalogs; purchase/refund history and
  management.
- Equity trainer, hand-ranking reference, today's table highlight, public hand
  share viewer, and the web table-preferences surface.

Real-money play and multi-table presentation remain intentional CLI non-goals.
