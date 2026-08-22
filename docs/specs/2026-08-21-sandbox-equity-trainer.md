# Sandbox Equity Trainer Mode — Design

## Summary

An opt-in, sandbox-only overlay that turns the equity/hand-category numbers the server already
computes into a teaching tool: a post-action breakdown explaining *why* the viewer's win chance is
what it is (outs, hand category, board texture) instead of just a bare percentage. This does **not**
require an automated bot, an AI opponent, or any new equity engine — `internal/engine/equity` and
`snapshot.go` already compute the viewer's own `Equity`/`HandCategory` live, every hand, and already
send it to the client unconditionally (`api/CLAUDE.md`: "the viewer's own seat... sends their true
category unconditionally the same way it does `Equity`"). The client already renders both, gated only
by the local peek state (`Seat.tsx:202-215`, `showEquity`/`showHandCategory`). Training mode is a
client-side explanatory layer on top of data that already exists and already flows over the wire —
new work is presentation and pedagogy, not simulation.

Room-level equity display is also already independent of currency mode: `rooms.go:104-110` defaults
`EquityDisplayEnabled = true` and forces it `true` outright for any public room, sandbox or real —
private rooms are the only ones that can currently opt out. This spec does not touch that gate. It adds
a second, separate, always-sandbox-only layer on top.

## Where

- Server: no changes. `internal/engine/hand/snapshot.go:128-129` (`Equity`, `HandCategory` fields),
  `internal/api/v1/tablews.go:331,352` (`actor.SetEquityEnabledForActor`) are read-only inputs to this
  feature, not touched.
- Client: `ui/src/components/table/Seat.tsx` (existing equity/category rendering, reused not
  replaced), new component `ui/src/components/table/EquityTrainerPanel.tsx`, `ui/src/lib/tablePreferences.ts`
  (new preference flag), `ui/src/app/table/page.tsx` (mount point, gated on room currency mode).

## Design

### Sandbox-only gate

The panel only mounts when `room.currency_mode === 'sandbox'` (or the room object's equivalent field
already read in `ui/src/app/table/page.tsx` for other sandbox-only UI). This is a client-side
presentation gate, not a new security boundary — the underlying `Equity`/`HandCategory` values are
already sent for real-money rooms too (per the room-level `EquityDisplayEnabled` default above); this
spec simply chooses not to add the *teaching* layer where its "quiz me" mechanic (see below) could
read as assistance during money play. No server change enforces the sandbox restriction because none
is needed for data exposure — only for whether this specific optional UI renders.

### What the panel shows

Reuses the exact `chance`/`seat.hand_category` values `Seat.tsx` already computes at
lines 202 (`const chance = seat.equity == null ? null : Math.round(seat.equity * 100)`) and 312-313
(`HAND_CATEGORY_LABELS[seat.hand_category]`) for the viewer's own seat. On top of the existing bare
number, an expandable panel (toggled from the action bar, off by default, `preferences.equityTrainer`
in `tablePreferences.ts`) shows, post-action only (never mid-decision, to avoid becoming a live
solver):

- The current hand category and the specific cards that make it up (already-known client data: the
  viewer's own `HoleCards` plus the visible board — no new hidden-information leak, this is exactly
  what `Seat.tsx` already permits the viewer to see about their own hand).
- A one-line plain-language reason the equity number is what it is (e.g. "Você tem par de reis; um
  adversário pode ter um par melhor ou um flush em formação"), generated client-side from the
  category + board texture (flush/straight draw presence on board), not a new server computation — a
  small pure function in the new component, unit-tested with fixed board/category fixtures.
- After showdown only (never before, to not leak folded hands): how the viewer's equity moved across
  streets, using the same per-street snapshots the client already buffers for the live equity bar
  animation.

### What it does not do

- No opponent hand simulation, no Monte Carlo run, no bot. The "trainer" framing is entirely built on
  the viewer's own already-authorized data (`ViewFor`'s existing per-viewer equity/category), so there
  is nothing here that computes anything the server doesn't already compute and send.
- No live "what should I do" recommendation during the viewer's own turn — the panel is
  post-action/post-street only, so it can never function as a real-time solver assisting an in-progress
  decision, sandbox or not.
- No opponent-facing version — this is a personal learning tool about the viewer's own hand, same
  privacy boundary `Seat.tsx` already draws for `isViewer`.

## Testing

- `EquityTrainerPanel.test.tsx`: renders the panel from a fixed `SeatView`-shaped fixture (category +
  board), asserts the plain-language reason text for a few fixture combinations (pair vs. flush draw
  vs. two pair), asserts it never renders when `room.currency_mode !== 'sandbox'`, asserts it never
  renders for `!isViewer` seats.
- Existing `Seat.test.tsx` equity/category gating tests are unchanged — this feature reads their
  already-tested `showEquity`/`showHandCategory` output, it doesn't alter the gate.
- `tablePreferences.test.tsx`: new `equityTrainer` boolean added to the existing normalize/default
  round-trip tests, same pattern as `dealerVoice`/`voiceCommands`.

## Out of scope

- Any change to the existing room-level `EquityDisplayEnabled` gate or its real-money availability —
  that's a separate, already-shipped decision this spec doesn't revisit.
- A "training mode" for opponents' hands, or any bot/AI to play against — not needed, since the whole
  feature rides on data the server already computes for the viewer's own hand only.
- Historical/replay-based training (studying old hands from `hands/history`) — a different feature
  surface (`ui/src/app/hands/replay`), not addressed here.
