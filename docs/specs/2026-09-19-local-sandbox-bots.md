# Local sandbox bots — implementation boundary

Issue: [#395](https://github.com/artur-oliveira/ctech-poker/issues/395)

## Implemented foundation

The API accepts an explicit `allow_bots` flag on public `join-or-create` requests. The flag is rejected outside sandbox and above 500/1,000 blinds. Responses expose `match_kind` so the client can distinguish a human match, an empty-table wait, a pending bot fill, and a reserved seat without inferring state from occupancy.

The buy-in ceremony owns consent. It defaults off on every entry and explains the applicable target occupancy before confirmation. Direct/private table joins and real-money rooms do not expose the control.

Automated seats carry a persisted `is_bot` marker through the hand engine, viewer-scoped snapshot, protobuf transport and generated clients. The table renders a textual `BOT` badge without changing the seat box. This is an identity boundary: later strategy code must never rely on display-name conventions to distinguish bots.

`internal/pokerbot` contains the provider-independent policy boundary. Its eight profiles share one decision engine and parameterize participation, aggression, bluffing, calls, traps, sizing, tempo, reveal and cash-out behavior. The package accepts already-authorized legal actions and returns one of them; the hand engine remains authoritative.

Thinking delay is derived from public decision complexity and profile tempo. It does not accept hand strength or the chosen action, preventing timing from becoming a strength oracle. Scheduled actions are revalidated against hand ID, state version and current actor before being committed.

The table snapshot persists `BotPolicy`, including owner, buy-in, format target and activation timestamp. A timer is only a wake-up mechanism: after restart, reload reconstructs the 15-second wait from that timestamp. One human is filled to 2 seats in heads-up, 4 in 6-max and 6 in 9-max. Each bot gets an internal `bot:` identity, one of eight profiles, the human's selected buy-in and no wallet hold.

Bot turns run through the normal legal-action view and `ActIdempotent` commit path. Delays are variable and bounded before the turn deadline. Multiple fleet actors may schedule the same turn; the hand/version/current-player checks and conditional table commit choose one result. After a hand, a bot may muck, show one card or show both through the same reveal method as a human, and may probabilistically request exit. Bot removal never produces a wallet credit or pending cash-out. A replacement receives a fresh seat identity/profile and the configured buy-in.

When a human arrives, `join-or-create` persists one reservation in the versioned table state and marks every bot for exit in the same mutation. Bots already in a hand finish it and removal runs between hands. The arriving player remains on an authenticated screen outside the table; the wallet debit happens only after the bots are gone and immediately before the reserved seat commits. Cancellation, debit failure and the persisted two-minute expiry release the reservation and allow bot fill again. Reloading an actor reconstructs both bot-fill and reservation-expiry timers from durable timestamps. A direct room join cannot bypass this contract.

The shared bot-funding window lives in `poker_achievement_progress` at `(player_id, bot_window)`. It records `started_at`, `expires_at`, net profit and an embedded hashed hand-id map. The conditional update checks both the current window and the absence of the hand key, which makes one write per automated hand atomic and idempotent without a growing second table. A new/restored session performs one consistent read; subsequent hands use the actor's recent state and DynamoDB conditions remain the cross-instance authority. The last hand settles in full. Reaching `BOT_NET_PROFIT_LIMIT` (100,000 by default), an ambiguous store result, or a store outage retires every bot before another deal. Only bot-window rows carry TTL; achievement rows in the shared table remain durable.

`GET /v1.0/rooms/bot-eligibility` lets the buy-in ceremony disable bot consent before any debit and report when the current window expires. Aggregate EMF metrics record update latency, outcome, net delta and consumed write capacity without player, table or hand identifiers as dimensions.

Completed outcomes persist `contains_bot`. Human history keeps the hand and its sandbox net result, while the post-hand pipeline skips achievements, leaderboard counters, public poker stats, matchups, highlights and recent-player suggestions. Bots are also excluded from social lookups and seat action menus in the client.

The buy-in response's `match_kind=bot_pending` survives navigation and reuses the felt's existing center status as “Preparando adversários…”. No new banner or fixed overlay is introduced. The seat's textual `BOT` marker is the persistent disclosure. The guide documents the opt-in, replacement and competitive exclusions.

## Remaining release work

- Add the “start now”/retry controls and replaceable-bot bucket fields described in #395.
- Add mock-runtime scenarios and complete the responsive/reconnection QA matrix.
