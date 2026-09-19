# Local sandbox bots — implementation boundary

Issue: [#395](https://github.com/artur-oliveira/ctech-poker/issues/395)

## Implemented foundation

The API accepts an explicit `allow_bots` flag on public `join-or-create` requests. The flag is rejected outside sandbox and above 500/1,000 blinds. Responses expose `match_kind` so the client can distinguish a human match, an empty-table wait, a pending bot fill, and a future reserved seat without inferring state from occupancy.

The buy-in ceremony owns consent. It defaults off on every entry and explains the applicable target occupancy before confirmation. Direct/private table joins and real-money rooms do not expose the control.

Automated seats carry a persisted `is_bot` marker through the hand engine, viewer-scoped snapshot, protobuf transport and generated clients. The table renders a textual `BOT` badge without changing the seat box. This is an identity boundary: later strategy code must never rely on display-name conventions to distinguish bots.

`internal/pokerbot` contains the provider-independent policy boundary. Its eight profiles share one decision engine and parameterize participation, aggression, bluffing, calls, traps, sizing, tempo, reveal and cash-out behavior. The package accepts already-authorized legal actions and returns one of them; the hand engine remains authoritative.

Thinking delay is derived from public decision complexity and profile tempo. It does not accept hand strength or the chosen action, preventing timing from becoming a strength oracle. Scheduled actions are revalidated against hand ID, state version and current actor before being committed.

The table snapshot persists `BotPolicy`, including owner, buy-in, format target and activation timestamp. A timer is only a wake-up mechanism: after restart, reload reconstructs the 15-second wait from that timestamp. One human is filled to 2 seats in heads-up, 4 in 6-max and 6 in 9-max. Each bot gets an internal `bot:` identity, one of eight profiles, the human's selected buy-in and no wallet hold.

Bot turns run through the normal legal-action view and `ActIdempotent` commit path. Delays are variable and bounded before the turn deadline. Multiple fleet actors may schedule the same turn; the hand/version/current-player checks and conditional table commit choose one result. After a hand, a bot may muck, show one card or show both through the same reveal method as a human, and may probabilistically request exit. Bot removal never produces a wallet credit or pending cash-out. A replacement receives a fresh seat identity/profile and the configured buy-in.

When a human joins, every bot is marked for exit in the same versioned join mutation. Bots already in a hand fold only when their legal turn arrives, and removal runs between hands. The lobby occupancy mirror counts humans, so replaceable bots do not make a public room look full. Heads-up currently uses a temporary `PendingEntry` seat while the hand finishes; the separate no-debit reservation contract from issue #395 remains required before release.

Completed outcomes persist `contains_bot`. Human history keeps the hand and its sandbox net result, while the post-hand pipeline skips achievements, leaderboard counters, public poker stats, matchups, highlights and recent-player suggestions. Bots are also excluded from social lookups and seat action menus in the client.

The buy-in response's `match_kind=bot_pending` survives navigation and reuses the felt's existing center status as “Preparando adversários…”. No new banner or fixed overlay is introduced. The seat's textual `BOT` marker is the persistent disclosure. The guide documents the opt-in, replacement and competitive exclusions.

## Required before release

- Replace the heads-up temporary pending seat with the durable `reserved` flow: no wallet debit before the real seat, authenticated confirmation/cancel, and an intermediate screen outside the table layout.
- Add the shared, atomic 100,000-net-profit/24-hour counter and fail closed for another bot hand when its persistence is unavailable.
- Expose limit availability before buy-in, the “start now”/retry controls, replaceable-bot bucket fields and the reservation states described in #395.
- Add the aggregate operational metrics, mock-runtime scenarios, statistical strategy checks and the full responsive/reconnection QA matrix.
