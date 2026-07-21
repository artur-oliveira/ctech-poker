# ctech-poker — Technical Architecture

> Status (2026-07-21): implemented — sandbox mode is live end-to-end; Phase 5 real-money code
> exists behind the `REAL_MONEY_ENABLED` + `LEGAL_SIGNOFF_REF` gate. Sections marked
> "proposal" describe design intent that should still be confirmed against the live systems.

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

## 2. Table-authority model (revised — DynamoDB conditional writes are the source of truth)

**Why this changed from the original lease-based-single-writer-actor proposal:** that design borrows from a
low-latency same-process actor model, which pays for itself when actions arrive at high frequency (every few
milliseconds). Poker action is human-paced — one action every few seconds, one active actor at a time — so the
per-action cost of going through DynamoDB (a few milliseconds of read/write latency) is not a bottleneck, and
paying for it buys a large simplification: **DynamoDB's conditional writes are the correctness mechanism, not a
Redis lease.** This mirrors what `ctech-wallet` already does for money (never read-then-write; every mutation
is a `ConditionExpression`-guarded write against a version/balance field) — the same tool, applied to table
state instead of wallet balance.

- Table + current-hand state lives in DynamoDB as one item (or a small `TransactWriteItems` group), carrying a
  `version` field. **Any** instance in the ASG can receive a player's action: it reads current state, validates
  the action (right player's turn, legal bet size, etc.), and writes the new state back with
  `ConditionExpression: version = :expected`. If another instance's write raced it, the condition fails and the
  handler retries against the freshly-read state (or rejects if the action is now stale, e.g. someone else
  already folded that seat) — no instance ever "owns" a table in the sense of being the only one allowed to
  touch it.
- No Redis/Valkey lease is required for correctness. The `tablelease.Service` built in Phase 0 (shared with
  `ctech-wallet` via `ctech-go-common/lock`) is **downgraded to an optional latency/affinity optimization**: an
  instance that holds a table's "preferred" lease may keep a read-through in-memory cache of that table's
  current state to skip the DynamoDB read on the hot path, but the lease is never load-bearing for
  correctness — if the lease expires, is contested, or is simply not renewed, any instance can still safely
  read-validate-write against DynamoDB directly. Whether to keep this optimization at all (vs. deleting
  `tablelease.Service` outright) is an explicit decision for whoever implements Phase 2, not assumed here — see
  the open question at the end of this section.
- A WebSocket gateway layer still fans out state-change pushes to every connected client at a table, via
  `ctech-go-common/ws`'s `Registry` (Redis pub/sub, no sticky-session requirement) — this concern is entirely
  orthogonal to where table state is persisted and is unchanged by this revision. Any instance can accept a
  player's WebSocket connection; it doesn't need to be the instance that last wrote that table's state.
- On instance failure: nothing to fail over. Table state was never only in that instance's memory — the next
  action against that table, from any instance, reads the current DynamoDB state and proceeds. There is no
  lease to expire and no ownership to hand off.

**Open question for Phase 2 implementation:** keep `tablelease.Service` as a cache-affinity hint (routing a
given table's traffic toward one preferred instance to raise its in-memory cache hit rate under load) or
delete it and rely on DynamoDB reads unconditionally. Recommendation: keep it, since it already exists and is
shared with `ctech-wallet` — but treat any bug in it as a performance issue, never a correctness issue, and
say so explicitly in the Phase 2 plan so nobody re-adds lease-based correctness logic later out of habit.

## 3. Durable state & crash recovery (revised)

- Every player action, once validated, is a single conditional `TransactWriteItems` against DynamoDB: (a) the
  table/hand state item's fields update and its `version` increments, guarded by
  `ConditionExpression: version = :expected`, (b) an `ActionLogEntry` (§ 5) is appended in the same transaction
  for audit/hand-history (§ 8.2) — not for recovery replay, since state itself is already durable after every
  single action, not just checkpointed periodically. (c) Only after the transaction commits does the handler
  broadcast the new state to connected clients (§ 2) — a broadcast never claims a state DynamoDB doesn't yet
  agree happened.
- **Recovery is trivial under this model: there is nothing to recover.** State was never held only in a
  process's memory pending a periodic checkpoint — every action's write already left the table fully
  resumable. A crash mid-hand loses at most the in-flight request that was being validated when the process
  died (the player simply retries the action, or the client's own reconnect/resync — § 4 — re-fetches current
  state and the player sees the action didn't apply). This is a real simplification over the original
  proposal's snapshot-at-hand-start-plus-bounded-replay design, which this revision replaces rather than keeps
  as a fallback.
- Idempotent action de-dup (§ 4, OVERVIEW.md § 4) still applies exactly as before: the `(table_id, hand_id,
  seat, action_id)` de-dup key guards against a double-submitted action being validated twice, independent of
  this section's storage-model change.
- This keeps the same discipline `ctech-wallet` already applies to money (conditional writes, no
  read-then-write, append-only audit trail) — now applied uniformly to *both* systems via the same underlying
  DynamoDB pattern, rather than poker inventing a parallel actor-plus-lease-plus-log mechanism that wallet
  doesn't need for an equivalent correctness/audit guarantee.

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

- `PlayerProfile { user_id(pk), poker_terms_version, poker_terms_accepted_at, created_at, updated_at }` — a
  poker-local shadow row keyed by the same `user_id` ctech-account issues in the JWT. It exists for two reasons,
  neither of which ctech-account's own user table can serve: (1) every poker-owned table (`Seat.player_id`,
  `AchievementProgress.player_id`, the Phase 5 `sessionlog` entries) foreign-keys against this row instead of
  reaching into ctech-account's table directly — no cross-service table sharing; (2) it gates acceptance of a
  poker-specific Terms of Service/fair-play addendum (collusion, chip-dumping, action-is-final-once-submitted),
  a document distinct from both ctech-account's platform ToS and ctech-wallet's gambling addendum. This exactly
  mirrors `ctech-wallet`'s own `wallet.User` row (`api/internal/domain/wallet/user.go`) — a per-service shadow
  record holding only that service's consent state, computed-equality version check, never a stored boolean.
  See Phase 3's plan, Task 11.
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
