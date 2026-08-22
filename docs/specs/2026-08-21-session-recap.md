# Session Recap Screen — Design

## Summary

Today, leaving a table (`LeaveDialog.tsx:31-50` → `onLeftAction` → `router.push('/lobby')`,
`ui/src/app/table/page.tsx:514-517`) drops the player straight back to the lobby with no summary — the only
session-level recap that exists is `RealityCheck.tsx`'s periodic "Pausa consciente" dialog, shown *during* play, never
at the end. This adds a one-time recap dialog shown at the moment of leaving: session duration, hands played, biggest
pot won, and net result. No new backend endpoint — everything needed is already served by
`GET /v1.0/players/me/sessions` and
`GET /v1.0/players/me/hands?table_id=`, both already called from the client today.

## Where

New file: `ui/src/components/table/SessionRecap.tsx` (sibling of `RealityCheck.tsx`, same dialog shape and Portuguese
copy register). Wired into `ui/src/app/table/page.tsx`'s `LeaveDialog`
`onLeftAction` callback (`page.tsx:514-517`), replacing the immediate `router.push('/lobby')` with
"open `SessionRecap`, navigate to `/lobby` when its own close button is pressed."

## Data — reuse only, no new backend fields

`page.tsx:483` already computes `openSession = sessions.find(session => session.table_id === id &&
session.ended_at === 0)`, giving `buyin_amount` and `joined_at` (`api/internal/sessionlog/store.go`'s
`SessionItem`, lines 42/45) — the exact fields `RealityCheck` already reads (`page.tsx:596-599`).
`LeaveDialog.confirm()` (`LeaveDialog.tsx:35`) already receives `{amount}` from `leaveRoom(roomId)` — the final
cashed-out stack. Between these two, duration and net result (`amount - buyin_amount`) need no new call at all.

**Hands played + biggest pot won** are not tracked client-side today outside `RealityCheck`'s
`completedHands` set (which only counts, doesn't look at outcome). Reuse `getHands({tableId, mode})`
(`ui/src/lib/api/player.ts:123-129`, backed by `sessionlog.Store.ListHandsByTable`,
`store.go:264-287`, itself already exposed and paginated newest-first): on leave, page through it filtered to
`table_id: id` until either a page is exhausted or a hand's `ended_at` falls before
`openSession.joined_at`, accumulating:

- `handsPlayed`: count of items with `ended_at >= joined_at`.
- `biggestPot`: `max(net_change)` across those items where `net_change > 0` (a big loss isn't "the biggest pot," so only
  wins count toward this stat — if no hand was won this session, the stat is omitted from the dialog rather than shown
  as 0 or negative).

`HandItem.net_change` (`player.ts:100`) is already the per-hand profit/loss figure used elsewhere (match history,
leaderboard), so no new interpretation is introduced — this is the same number the player already sees per-hand in
`/hands/history`, just maxed over one session's hands.

Cap the pagination at 3 pages (150 hands) — a session that long is already an outlier, and the recap is a courtesy
summary, not an audit; if the cap is hit, label the stat as "hands played (últimas 150)"
rather than silently under/over-counting. This mirrors `ListRecentHandsAcrossModes`'s own bounded lazy-read pattern
(`store.go:242-262`) rather than inventing a new one.

## Component

`SessionRecap` takes `{joinedAt, buyIn, finalStack, tableId, mode, onCloseAction}` and, on mount, fires the `getHands`
pagination loop above into local state (loading state while it resolves — the duration/result/buy-in stats render
immediately since they need no fetch; hands-played/biggest-pot render once the fetch settles, or are skipped if it
fails, since `getHands` already sets
`silentError: true`, matching this codebase's existing "never let a stats fetch block a core flow"
convention). Visually, reuse `RealityCheck`'s `<dl className="reality-check-stats">` layout and CSS (rename the shared
class to `session-stats` if `RealityCheck` is touched in the same change, or leave it as-is and add a second class — a
one-line CSS decision at implementation time, not a design question) rather than inventing a new stat-grid component.
Title: "Resumo da sessão". Single action:
"Voltar ao lobby" → `onCloseAction()` → the existing `router.push('/lobby')`.

## Testing

- `SessionRecap.test.tsx`: renders with a mocked `getHands` resolving hands both under and over the 150-hand cap;
  asserts duration/result/buy-in render without waiting on the fetch; asserts hands-played and biggest-pot render once
  it resolves; asserts the biggest-pot stat is omitted when every hand in the sample has `net_change <= 0`; asserts a
  rejected `getHands` still renders the dialog with the fetch-derived stats simply absent.
- `page.test.tsx`: leaving a table opens `SessionRecap` instead of navigating immediately; closing it performs the
  `/lobby` navigation that used to happen on leave directly.

## Out of scope

- A stack-over-time graph — nothing today buffers stack values at intermediate points during a session (only the current
  stack), and building that buffer is a bigger feature than "recap on leave." If wanted later, it is a separate spec
  once there's a concrete need for it (YAGNI).
- A persisted/re-visitable history of past recaps — this is a one-time dialog at the moment of leaving, not a new page.
  `/hands/history` already exists for anyone who wants the underlying detail.
- Any change to `RealityCheck.tsx`'s own periodic-reminder logic or its "Pausa consciente" copy — this spec only adds a
  second, end-of-session use of the same visual pattern.
