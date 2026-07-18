# ctech-poker — Technical Architecture (proposal)

## 1. Stack

- **Backend**: Go, `cmd/` + `internal/` + `Dockerfile` + `Makefile` — same convention as every
  other CTech backend. Go's goroutines/channels are a genuinely good fit for a per-table
  actor model (§ 3), not just consistency for its own sake.
- **Real-time transport**: WebSocket. Given the company's existing comfort with ASG+EC2 (no
  NAT Gateway constraint drove that decision — see the `ctech-cdk` audit) and the fact that a
  poker table needs a **single consistent authority per table** for the whole hand (not a
  stateless request/response model), the game-server tier should run as a **stateful
  service on the existing ASG/EC2 pattern** (or Fargate if the team wants managed container
  ops instead) — not as a fleet of independent Lambdas. A poker hand cannot be correctly
  modeled as a series of independent, ordering-agnostic invocations.
- **Frontend**: React SPA (matches the rest of the company's `ui/` folders), `ctech-oauth-client`
  for auth, native WebSocket client (no heavyweight real-time framework needed).
- **Infra**: CDK, importing shared constructs from `ctech-cdk`.

## 2. Table-authority model

- Each table is owned by exactly one process at a time (**single-writer per table**), assigned
  via a directory service (a Redis/Valkey key `table:{id} → instance_id` with a TTL-renewed
  lease, matching the lock pattern already used in `ctech-wallet` — reuse it, don't reinvent).
- A WebSocket gateway layer (can be the same process or a thin front tier) routes each client
  connection to the instance currently holding that table's lease; if a client connects to the
  "wrong" instance, it's redirected (HTTP 307 to the correct instance, or an app-level
  redirect message over the socket before upgrade completes).
- On instance failure: the lease expires, a healthy instance picks up the table by replaying
  its last durable checkpoint (§ 3) — the table resumes mid-hand from the last completed
  action, not from scratch and not lost.

## 3. Durable state & crash recovery

- Every player action, once validated, is: (a) applied to in-memory table state, (b) appended
  to a durable per-table action log (DynamoDB or Redis-with-persistence — reuse whichever the
  wallet's ledger pattern already proved out, for the same operational-simplicity reason cited
  in the billing repo's storage recommendation), (c) broadcast to connected clients — in that
  order, log before broadcast, so a broadcast never claims a state the log doesn't yet agree
  happened.
- Recovery = replay the action log for the current hand from the last full-state snapshot
  (snapshot at the start of each hand, replay deltas within the hand — bounded replay cost).
- This is the same "append-only ledger + idempotent replay" pattern `ctech-wallet` already
  uses well for money; apply the same discipline here even though chips-in-a-hand aren't
  literally cash, because a hand that resumes wrong is exactly the kind of bug that turns into
  a real-money dispute in real mode.

## 4. Wallet integration (real mode) — proposal, confirm against actual `ctech-wallet` API

```
POST /v1/holds                       (buy-in)
Idempotency-Key: <table_id>:<seat>:<buyin_attempt_id>
Body: { customer_ref, amount_cents, reason: "poker_buyin", metadata: { table_id, seat } }
→ 201 { hold_id, status: "held" }

POST /v1/holds/{hold_id}/capture     (cash-out — capture final amount, release remainder)
Body: { capture_amount_cents }
→ 200 { status: "captured" }

POST /v1/holds/{hold_id}/release     (table/hand aborted before any capture — full release)
→ 200 { status: "released" }
```

**Prerequisite, confirmed missing today**: `ctech-wallet` currently exposes only sandbox
credit/debit routes; there is no hold/capture (or equivalent debit) endpoint for real funds,
and its DynamoDB tables are provisioned at a 5 RCU/WCU cap that would throttle under real
table traffic. Do not schedule real-mode wallet integration work until both are resolved on
the `ctech-wallet` side — build and ship sandbox mode against the existing sandbox
credit/debit endpoints in the meantime; the game engine itself doesn't care which ledger it's
pointed at if the `currency_mode` boundary (OVERVIEW.md § 5) is respected from day one.

## 5. Data model (sketch)

- `Table { id, mode(sandbox|real), stakes, max_seats, visibility(public|private), status }`
- `Hand { id, table_id, hand_number, dealer_seat, board_cards, pot_layers[], status }`
- `Seat { table_id, seat_number, player_id, stack, state }`
- `ActionLogEntry { table_id, hand_id, seq, seat, action_type, amount, action_id, occurred_at }`
- `HoldOrSandboxTxn { id, currency_mode, player_id, amount_cents, table_id, status }`
- `Achievement { key, metric, tiers: [{stars, threshold}] }` — static catalog (OVERVIEW.md § 9.2).
- `AchievementProgress { player_id, achievement_key, counter, stars_unlocked }`
- `SandboxRouletteSpin { id, player_id, awarded_cents, spun_at }` — one row per spin, `spun_at`
  enforces the 24h cooldown (OVERVIEW.md § 9.3).
- `LeaderboardStat { player_id, hands_played, hands_won, vpip, achievement_points }` — aggregated,
  non-monetary (OVERVIEW.md § 9.1).

## 6. Gamification compute

- **Hand equity** (OVERVIEW.md § 9.4): computed server-side per active player, per street, via
  Monte Carlo sampling of the remaining deck against each still-active opponent's random range.
  Cheap to fold into the state push already going to that player over their own socket channel;
  never computed or sent client-side (would require exposing the remaining-deck composition,
  which leaks information about other players' hole cards by elimination).
- **Achievements**: counters updated as part of the same durable write that appends the
  `ActionLogEntry`/hand-completion event (§ 3) — an unlocked star is derived state, not a
  separate source of truth, so it can always be recomputed from the action log if needed.

## 7. Observability

- Per-table metrics: hands/hour, average action latency, disconnect rate, lease-failover
  count. A spike in lease-failover count is the earliest signal of an instance going bad.
- Structured audit log of every hand's full action sequence (also doubles as the hand-history
  feature suggested in OVERVIEW.md § 8.2).

## 8. Security

- Server-authoritative everything (OVERVIEW.md § 4) — the client is never trusted with hidden
  information (opponents' hole cards) until showdown reveal; the server must not even *send*
  other players' hole cards to a client before it's their turn to be revealed (a common
  amateur-engine leak: sending the full table state including hidden cards and just hiding
  them in the UI — this is trivially inspectable via browser devtools and must not happen).
- Rate limiting on actions per seat to prevent action-spam / socket abuse.
- `currency_mode` boundary enforced server-side on every wallet-adjacent code path, not just
  in the UI (OVERVIEW.md § 5).
