# Visible maintenance-migration notice (issue #354)

Desmembrado de #215.

## Problem

When a table moves between server instances for maintenance — a deploy, a spot-instance
rebalance, an ASG drain — the player experience today is a silent socket close followed
by a transparent reconnect. On a slow reconnect that reads as "the table froze". The
issue asks for a soft heads-up before the close so the reconnect reads as expected.

## What shipped

**Backend (`api/internal/app`, `api/internal/wsdrain`).**

- `wsdrain.NoticeAll(payload []byte) int` — fan-outs one application-level binary
  message to every tracked socket on the same bounded pool `CloseAll` already uses, then
  returns. A `Conn` that cannot write data frames (control-only) is skipped. Best-effort:
  a failed write is logged at debug and ignored.
- `internal/app.announceTableMigration(ctx)` marshals
  `ServerMessage{type:"table_migrating", text:<pt-BR copy>}`, calls `NoticeAll`, and
  waits `migrationNoticeGrace` (400 ms, budgeted inside the existing `wsDrainGrace` of
  1.5 s) so the client renders the banner before the socket ends.
- Wired at both planned-drain triggers, which already live in `internal/app`:
  - `startServer`'s `OnStop` hook — before `wsdrain.CloseAll`.
  - `pollSpotTermination` — before `manager.DrainAndRelease`, when IMDS reports a spot
    termination notice.

`internal/tablemanager` and `internal/tablelease` are **untouched**. `tablelease` stays
latency-only and gains no responsibility (the mistake `api/CLAUDE.md` documents as
already fixed). `DrainAndRelease` is deliberately not the trigger point — it must not
depend on the transport-adjacent `wsdrain` package, and it would also fire for
`evictActorWhenIdle`, which only ever evicts a table with zero local connections (nobody
to notify).

**Protocol.** No `proto/poker.proto` schema change: `table_migrating` is a new value of
the free-form `ServerMessage.type` string and reuses the existing `text` field (13). The
`type` enum comment in the `.proto` is not authoritative and regenerating the three
generated files for a comment-only change would only risk `protoc` version skew. The new
message is documented in `api/README.md` and `ui/src/lib/api/table.ts`.

**Web client (`ui`).** `useTableRealtimeSession` tracks a `migrating` string, set on a
`table_migrating` frame (falling back to `MIGRATION_NOTICE_FALLBACK` in
`lib/tableResilience.ts` if the server sends no text) and cleared on the next `connected`
frame from whichever instance takes over. `table/page.tsx` folds it into
`connectionMessage`, so the existing `.reconnect-notice` banner carries the friendlier
copy across the brief socket close instead of "Reconectando à mesa…". The manual "Tentar
agora" button is hidden while the socket is still up (a planned migration reconnects on
its own).

**Terminal client (`cli`).** `Narrator.OnMessage` narrates the `table_migrating` type as
one line, same fallback shape.

## Acceptance criteria

- [x] `tablelease` unchanged — no correctness or notice logic added.
- [x] The notice reaches the player before the socket closes, inside `wsDrainGrace`
      (`wsdrain.TestNoticeAllPrecedesCloseAll`, `app.TestAnnounceTableMigrationPrecedesClose`).
- [x] An unannounced migration (crash, non-spot termination) still degrades exactly as
      before — the notice is best-effort and never blocking.
- [x] **Consistency ADR: evaluated, unnecessary.** No new persisted state is introduced.
      The notice is a fire-and-forget WebSocket message; DynamoDB remains the single
      source of truth for table state and the migration mechanics (lease expiry/release,
      any instance picks the table up on the next command) are unchanged.
- [x] Tests cover that the notice is sent before `CloseAll`, not after (both Go tests
      above, plus `useTableRealtime.test.tsx` and `table/page.test.tsx` for the client).

Ref: #215
