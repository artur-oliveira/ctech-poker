# Achievement-toast replay + post-hand settle refetch

Date: 2026-09-03. Full write-up:
`docs/specs/2026-09-03-post-hand-refresh-latency-and-achievement-toast-replay.md`.

## What changed in `ui/`

- **`AchievementToast`** no longer replays an already-celebrated unlock. It
  remembers the signature of an unlock that finished its full lifecycle and
  never re-shows or re-queues it, and calls a new `onConsumed` callback on
  natural completion. An unlock interrupted mid-celebration by a `blocked`
  window (a hand-outcome / consent card) is still resumed afterwards.
- **`useTableRealtime`** exposes `clearUnlock`; the table page passes it as
  `AchievementToast`'s `onConsumed` so the socket layer drops the held unlock.
- **`lib/settleRefetch.ts`** (`invalidateAfterSettle`): invalidates a query key
  immediately, then re-invalidates on a `[1.5s, 4s, 9s]` backoff, cancellable.
  Used by `useTableOutcome` (`['hands', id]`) and `TodayHighlight`
  (`['highlights', tableId, 'today']`) because the server writes those
  projections on a pipeline that runs after the `complete` snapshot is sent, so
  a single invalidate refetches before the row exists.
  **Superseded on 2026-09-04**: that volley is now a retry that stops as soon as
  the projection is there. See `docs/2026-09-04-post-hand-read-budget.md`.

No visible-behaviour change to document in the in-app guide: the toast now
behaves as the guide already describes (shown once per unlock), and the
last-winners strip / "Maior pote de hoje" pill simply update promptly instead of
a hand late.
