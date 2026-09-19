# Local sandbox bots — implementation boundary

Issue: [#395](https://github.com/artur-oliveira/ctech-poker/issues/395)

## First implementation slice

The API accepts an explicit `allow_bots` flag on public `join-or-create` requests. The flag is rejected outside sandbox and above 500/1,000 blinds. Responses expose `match_kind` so the client can distinguish a human match, an empty-table wait, a pending bot fill, and a future reserved seat without inferring state from occupancy.

The buy-in ceremony owns consent. It defaults off on every entry and explains the applicable target occupancy before confirmation. Direct/private table joins and real-money rooms do not expose the control.

Automated seats carry a persisted `is_bot` marker through the hand engine, viewer-scoped snapshot, protobuf transport and generated clients. The table renders a textual `BOT` badge without changing the seat box. This is an identity boundary: later strategy code must never rely on display-name conventions to distinguish bots.

`internal/pokerbot` contains the provider-independent policy boundary. Its eight profiles share one decision engine and parameterize participation, aggression, bluffing, calls, traps, sizing, tempo, reveal and cash-out behavior. The package accepts already-authorized legal actions and returns one of them; the hand engine remains authoritative.

Thinking delay is derived from public decision complexity and profile tempo. It does not accept hand strength or the chosen action, preventing timing from becoming a strength oracle. Scheduled actions must eventually be revalidated against hand ID, state version and current actor before being committed.

This slice does not yet enable automatic seating. The next slice adds the durable coordinator, 15-second human-search state, bot funding counter, between-hand replacement and reservation flow. The UI contract is present on this feature branch to develop and test that coordinator end to end; it must not be released independently with bot seating disabled.
