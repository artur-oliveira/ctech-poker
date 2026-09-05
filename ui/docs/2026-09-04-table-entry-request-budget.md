# Table entry: the documented minimum, and everything else (#212)

Sitting down used to fan out seven to eight REST reads in parallel with the WebSocket handshake:
room, seat check, this table's hand history, the player's sessions, every player note (up to 500),
the reaction catalog, the first page of reaction purchases and the profile. All of it raced the
socket on the most sensitive path in the product, and none of it except the first two gates a
playable entry.

## The critical set

**Room + seat check + socket.** Nothing else. That is what `useTableSession` reads, and it is what
the page needs to decide between `BuyInPanel` (not seated), the loader (seated, waiting on the
first snapshot) and the table itself.

A visitor who is not seated now pays exactly two reads. Previously the hand-history read was
`enabled: valid` — it fired for visitors too, for a strip that only renders on a seated table.

## Everything else is progressive

`useTableProgressiveSession` owns the other six reads. Every one of them renders something that
only exists *after* the first snapshot — the last-winners strip, the reality-check clock, the seat
note badges, the felt theme, the premium reaction grid — and until that snapshot arrives the page
is a loader, so nothing is displayed late by waiting.

Two one-way latches, both armed **during render** so the first frame that can show the data is
already the frame that asks for it (an effect arms it a commit late — the same trap as
`HandOutcomeRing`'s duration):

| latch | arms on | gates |
| --- | --- | --- |
| `seeded` | the socket's first snapshot | `['hands', id]`, `['sessions','me']`, `['player-notes', <seats>]`, `['player','me']` |
| `reactionsOpen` | the reactions panel opening (or a purchase dialog) | `['wallet','reaction-catalog']`, the reaction purchases first page |

Neither latch disarms. A reconnect that momentarily drops the snapshot therefore does not re-run
the bootstrap, and because these are `enabled`-gated queries rather than remounted ones, a
reconnect spends no read at all — the cache entries are simply still there.

The reactions latch is the same shape as the deferred cosmetic catalogs
(`2026-09-04-deferred-cosmetic-catalogs.md`). It also settles the "don't read purchase history to
decide ownership" half of the issue by never reading it at all until the panel opens: ownership is
the catalog's server-computed `owned` flag, and the purchase page only drives the "refunding"
badge. Everything premium in `TableReactions` renders inside `{open && …}`, and the latch arms in
the same render as the open, so the panel's first painted frame already shows the catalog's
`loading` state rather than a wrong `unavailable`.

## Failure isolation

Each read is still its own query with its own error state, so a failing catalog cannot take the
table down — and now it cannot even be in flight while the socket is establishing state.

## Budget

Per table entry, per player:

| | before | after |
| --- | --- | --- |
| visitor (not seated) | 3 | 2 |
| seated, before the first snapshot | 8 | 2 |
| seated, playing | 8 | 6 |
| seated, playing, opens reactions once | 8 | 8 |
| socket reconnect | — | 0 |

`src/lib/hooks/useTableSession.test.tsx` asserts each row against the mocked API functions.

## Not done here

- No aggregated bootstrap endpoint. That is a backend contract change; the client-side split above
  removes the fan-out from the critical path without one.
- ~~`['player-notes']` still reads every note the player owns rather than only the ones for the
  seats at this table.~~ **Done (#209):** `GET /players/me/notes/?opponent_ids=` scopes the read
  server-side to one `BatchGetItem` over the seated opponents, and the query key carries the sorted
  seat set (`PLAYER_NOTES_KEY`), so a seat change re-keys the query instead of reusing an answer
  about players who left. An empty table spends no note read at all.
