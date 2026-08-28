# Social UX polish + "Sem pressa" achievement — Design

## Status

**Implemented 2026-08-28.** Four independent deliverables in one spec; they share no code, but
§3 is the only one that changes a privacy model and was reviewed on its own terms.

Shipped as designed. Two notes:

- The `no_rush` progress counter stores raw **milliseconds**, so its stored count grows far
  faster than any other achievement's. `achievementValueFormat` in the frontend is the only
  thing that makes those numbers readable — a new duration-metric achievement must be added to
  its `DURATION_KEYS` set.
- The actor-level time-bank tests live in `api/internal/table/timebank_test.go` next to the
  existing ones, not in `actor_test.go`.

## Motivation

Player feedback on the social surfaces:

1. The unread badge shows up both in the top bar and on the lobby's "Pessoas" button, but only
   the lobby one actually leads anywhere that clears it.
2. A table invite arrives as a text-only toast. Accepting means finding the activity list by
   hand.
3. A friend is visibly playing, but there is no way to sit at their table.

Plus a new achievement: **"Sem pressa"** — consuming the time bank.

---

## 1. Unread badge leads to the wrong tab

### Current behavior

`PeopleNavBadge` counts **inbox** events (`GET /social/summary` → `unread_count`, fed by the
`social_inbox_count` WebSocket message). It renders in three places:
`AppPageChrome`'s desktop nav, `AppPageChrome`'s mobile tab bar, and `PeopleDrawer`'s trigger.

Only the drawer renders `SocialInbox`, whose mount effect calls `markInboxRead`. The nav links
point at `/people`, which opens with `tab = 'friends'` — a tab that never touches the inbox. So
the badge is accurate everywhere and actionable in exactly one place.

The badge itself is not redundant: it is the same counter in both places and belongs in both.
The bug is the destination.

### Design

- Extract the badge's query into `useSocialUnread()` (`lib/hooks/`), returning the count. Both
  `PeopleNavBadge` and the nav link consume it; `SOCIAL_KEYS.summary` already dedupes the fetch
  across all three render sites.
- In `AppPageChrome`, the `people` route's `href` becomes `/people?tab=activity` when the count
  is above zero, and plain `/people` otherwise. Applies to the desktop nav and the tab bar.
- `app/people/page.tsx` seeds its `useState<PeopleTab>` from `useSearchParams().get('tab')`,
  accepted only when the value is one of `TABS`; otherwise `'friends'`.

Nothing else changes: landing on the Atividades tab mounts `SocialInbox`, which marks the events
read and clears the badge through the existing invalidation.

### Testing

- `people/page.test.tsx`: `?tab=activity` opens Atividades; `?tab=garbage` falls back to Amigos.
- `AppPageChrome.test.tsx`: with a non-zero summary in the query cache, the Pessoas link points
  at `/people?tab=activity`; with zero, at `/people`.

---

## 2. Actionable table-invite toast

### Current behavior

`useLobbyRealtime` turns a `social_event` message into `pushNotification(copy, 'info')` — text
only. `lib/notify.ts` has no concept of an action, and `Notifier.tsx` renders a message plus a
dismiss button.

The push payload already carries what an action needs: `SocialEvent.event_id` and
`SocialEvent.room_id` (`proto/poker.proto`). No contract change.

### Design

- `lib/notify.ts`: `AppNotification` gains `actions?: {label: string; run: () => void | Promise<void>}[]`,
  and `pushNotification(message, variant, actions?)` accepts them. Auto-dismiss and dedupe are
  unchanged — an ignored invite toast disappears on the normal 6s timer, and the durable event
  stays in the inbox either way.
- `Notifier.tsx` renders the actions as buttons inside the toast; the toast dismisses itself
  after an action resolves.
- `useLobbyRealtime`, on `social_event`:
  - `type === 'table_invite'` with a `room_id` → two actions, **Entrar** and **Recusar**, hitting
    the same `accept-invite` / `decline-invite` endpoints the inbox rows use. Both server
    handlers already call `notifyUnread`, so answering from the toast marks the activity read —
    the badge clears without opening the list. On accept, navigate to `/table?id=<room_id>`.
  - any other type → one action, **Ver atividades**, routing to `/people?tab=activity` (§1's
    entry point).
- Invite acceptance stays authoritative on the server: expiry, friendship, room status and
  capacity are revalidated by `Service.AcceptTableInvite`, exactly as from the inbox.

### Testing

- `Notifier.test.tsx`: actions render, clicking runs the handler and dismisses.
- `useLobbyRealtime` test: a `table_invite` event produces a toast with both actions; accepting
  calls the accept endpoint and routes to the table.

---

## 3. Join a friend's table — opt-in, public rooms only

### Current behavior

`presence` is deliberately room-blind: *"No table or room identifier is part of the public
model"* (`presence/model.go`), `Status` is `offline | online | in_table`, and
`sessionlog.FindLatestOpenSession` returns a bare bool so reconciliation cannot leak a table id
either. `/people` states the guarantee to players.

Exposing a friend's room is therefore a deliberate, player-controlled relaxation — not a bug fix.

### Design

**Opt-in flag.** `player.Profile` gains `TablePublic bool` (`dynamodbav:"table_public,omitempty"`,
`json:"table_public"`), following `ShowcasePublic` exactly: `*bool` in the PATCH `/players/me`
body, echoed in the profile response, toggled in `ProfileShowcaseDialog`. Default false —
existing players keep today's guarantee until they choose otherwise.

**Presence carries the room id.**

- `Store.SetInTable(ctx, playerID, roomID string)` replaces the bool. The Valkey adapter already
  writes a per-player table key; the value becomes the room id instead of `'1'`, and the
  read script returns it alongside the status. Existing keys holding `'1'` are treated as
  "in a table, room unknown" — they simply produce no join button and expire on their own TTL.
- `GetMany` returns `map[string]PlayerPresence` where `PlayerPresence` gains `RoomID string`.
  Callers that only want the status keep reading `.Status`.
- `sessionlog.FindLatestOpenSession` returns `(tableID string, err error)`; empty means no open
  session. `presence.Service.Reconcile` and `Open` pass it straight through, so a restart or
  reconnect restores the room id rather than losing it.
- `buyin.Service` passes the `roomID` it already has at both call sites.

**Exposure is gated at the API edge, not in presence.** `GET /social/friends` includes a
`room_id` for a friend only when **all** of these hold:

1. the friend's profile has `TablePublic` true;
2. presence reports `in_table` with a non-empty room id;
3. the room exists and `Visibility == "public"`;
4. `Status` is `waiting` or `active`;
5. `SeatsTaken < MaxSeats`.

Otherwise the field is omitted entirely. A private room never appears, and a player who has not
opted in never appears, regardless of what presence holds. Room lookups are batched over the
distinct room ids on the page, so a friends page costs at most one extra read per occupied room.

**UI.** `PeopleList`'s `friends` variant renders an "Entrar na mesa" button when the row carries
a `room_id`, routing to `/table?id=…`. It is a shortcut, not an authorization: the normal join
flow revalidates terms, currency, buy-in and capacity. The `/people` header copy is updated —
presence still never reveals a *private* table, and a public one is shown only by the player's
own choice.

### Testing

- `presence` service tests: room id survives `SetInTable` → `GetMany`, and `Reconcile` restores
  it from an open session.
- `social` handler tests, one per gate: opted-out friend, private room, full room, closed room,
  and the happy path — the first four omit `room_id`.
- `PeopleList` test: button appears only with `room_id`.

---

## 4. "Sem pressa" achievement

### Current behavior

`Actor.consumeTimeBank` (`table/actor.go`) charges the post-deadline portion of a decision
against the seat's durable balance and logs it, returning nothing. Two call sites: a normal act
and a disconnected player's timeout sit-out. Nothing accumulates the total anywhere.

### Design

**Catalog.** `KeyNoRush = "no_rush"`, metric `time_bank_ms_consumed`, tiers in **milliseconds**:

| Stars | Threshold (ms) | Label |
|-------|----------------|-------|
| 1 | 60 000 | 1 minuto |
| 2 | 3 600 000 | 1 hora |
| 3 | 86 400 000 | 1 dia |
| 4 | 604 800 000 | 1 semana |
| 5 | 2 592 000 000 | 1 mês (30 dias) |

Milliseconds, not seconds: the engine's unit, no rounding per hand, and `int64` covers the top
tier with room to spare.

**Capture.** `consumeTimeBank` returns the milliseconds it charged (0 when it charges nothing).
Both call sites write it to a new `tablestore.ActionLogEntry.TimeBankMs`
(`dynamodbav:"time_bank_ms,omitempty"`) on the entry they were already about to commit — no
extra write, no new persisted table state, and it survives an actor moving between instances
because the action log is the durable record.

**Award.** `app.go`'s `onHandComplete` already scans the hand's action log to derive `peeked`;
it sums `TimeBankMs` per player in the same pass and puts it on
`achievements.HandMetric.TimeBankMs`. `Service.RecordHand` calls `bumpBy(id, KeyNoRush, ms)` for
each player with a non-zero total, so tier crossing works through the existing
`TierCrossed`/`TierUnlock` path — including the unlock broadcast and leaderboard points.

Older log rows have no field and read as zero; the counter is monotonic, so a hand whose log
failed to load simply awards nothing (the same safe direction the rest of `onHandComplete`
takes).

**Presentation.** `lib/utils.ts` label "Sem pressa"; `lib/achievements.ts` description
("Deixou o relógio correr e usou seu tempo extra.") and example cards.

Thresholds are durations, not counts, so the raw `toLocaleString('pt-BR')` used everywhere would
print "2.592.000.000". Add `achievementValueFormat(key)` returning a formatter — duration for
`no_rush`, the existing number formatting for everything else — and route the four threshold
renders in `AchievementCard` (star `aria-label`, tooltip, progress `strong`, locked ladder) plus
the "faltam N" line in `app/achievements/page.tsx` through it. Progress-bar math is untouched:
it is proportional and unit-agnostic.

### Testing

- `actor` test: a decision past the base deadline returns non-zero and lands on the committed
  log entry; one inside the deadline returns 0 and writes nothing.
- `achievements` service test: a metric with `TimeBankMs` crossing 60 000 yields a 1-star unlock.
- `AchievementCard` test: `no_rush` thresholds render as "1 minuto" / "1 hora" etc., and an
  unrelated achievement still renders plain numbers.

---

## Out of scope

- Any presence change beyond the room id (still no "watching", no spectator surface).
- Requesting an invite from a friend who has not opted in — rejected in favor of the opt-in flag.
- Retroactive time-bank credit for hands played before the log field exists.
