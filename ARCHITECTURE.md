# ctech-poker — Technical Architecture

> Status (re-verified 2026-07-28): implemented. Sandbox is live end to end; the real-money
> path is built and reachable but held behind `REAL_MONEY_ENABLED` + `LEGAL_SIGNOFF_REF`,
> both fetched from SSM by the API stack. § 4 (wallet) and § 5 (data model) were written as
> proposals and have since been **replaced by the as-built description** below. `docs/README.md`
> carries the authoritative feature-status index; the two `docs/plans/2026-07-28-*` documents
> carry the current architecture punch list (WS frame racing, graceful drain, scheduler DLQs,
> multi-AZ).

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
  read-validate-write against DynamoDB directly. **Settled:** `tablelease.Service` was kept
  (`api/internal/tablelease`) purely as that latency hint. Treat any bug in it as a performance issue, never a
  correctness one.
- **As built, each live table also has an in-process actor** (`api/internal/table/actor.go`, registered per
  instance in `api/internal/tablemanager`): one goroutine serialising commands for that table, driving turn
  timeouts, AFK sweeps, blind escalation and presence. It is a *serialisation and timer* device on top of the
  conditional writes above — when a `CommitAction` hits a version conflict, the actor discards its uncommitted
  mutation and re-reads rather than trusting its cache.
- A WebSocket gateway layer still fans out state-change pushes to every connected client at a table, via
  `ctech-go-common/ws`'s `Registry` (Redis pub/sub, no sticky-session requirement) — this concern is entirely
  orthogonal to where table state is persisted and is unchanged by this revision. Any instance can accept a
  player's WebSocket connection; it doesn't need to be the instance that last wrote that table's state.
- On instance failure: nothing to fail over. Table state was never only in that instance's memory — the next
  action against that table, from any instance, reads the current DynamoDB state and proceeds. There is no
  lease to expire and no ownership to hand off.

**Transport, as built:** both gateways speak **binary protobuf**, not JSON — `proto/poker.proto`, generated into
`api/internal/api/v1/proto` (Go) and `ui/src/lib/api/proto/poker.ts` (ts-proto). `GET /v1.0/tables/:id/ws` is the
table gateway and `GET /v1.0/ws` the lobby/user gateway; the access token arrives as the **first frame after
upgrade**, not in a query string or header, and frames are capped at 32 KiB. Fan-out keys are
`<tableID>#<viewerID>` (per-viewer, because each seat sees a different masked snapshot), `lobby`, and
`user#<playerID>`.

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

## 4. Wallet integration — as built

`api/internal/walletclient` is an M2M (client-credentials) client for `ctech-wallet`, and
`api/internal/buyin` is the only thing that calls it. Two ledgers, one code path, selected by the
room's `currency_mode`:

- **Sandbox**: `Credit` / `Debit` against the play-money ledger. Non-convertible by construction.
- **Real**: `HoldGame` on buy-in, `ReleaseHold` if seating fails, `CashoutGame` on leave,
  `DebitReal` for the fixed entry fee, `IsGamblingActivated` as a per-player precondition.

Ordering is deliberate and asymmetric, because the two failure directions cost different things:
**debit then seat** on buy-in (never hand out chips that were not paid for), **remove then credit**
on cash-out (never pay out a stack still in play). Anything that can fail *after* chips moved is
written to `poker_pending_cashouts` and retried by the `cmd/reconcile` Lambda every 5 minutes,
keyed so a retry is idempotent.

Monetization is a **fixed entry fee per real-money table** (`entry_fee_cents`), not a percentage
rake — the Brazil-legal shape. Sandbox tables carry a nominal rake for gameplay parity only. See
`docs/plans/2026-07-25-realmoney-fixed-fee-and-sandbox-rake.md`.

## 5. Data model — as built

15 DynamoDB tables, all `TableV2`, on-demand billing, name-prefixed `<env>_`, defined in
`cdk/lib/dynamodb-stack.ts`. Partition key is always `pk` (S), sort key `sk` (S) where present.

**Table + hand state**

- `poker_table_state` — one item per table, `version` for the conditional writes of § 2. GSI
  `gsi_active_last_action` (sparse, KEYS_ONLY) is what `cmd/tablecleanup` scans for idle tables.
- `poker_action_log` — `pk = tableID#handID`, `sk = zero-padded version`. 90-day TTL, DynamoDB
  Stream → `cmd/archiver` → S3 JSON Lines for permanent audit.
- `poker_action_guards` — `pk = tableID#handID#actionID`, 7-day TTL. The idempotency guard behind
  § 3's de-dup key.
- `poker_table_state_history` — best-effort audit copy, `sk` = unix seconds.
- `poker_rooms` — lobby config. Sparse `gsi_public` and `gsi_share_code`.

**Player**

- `poker_player_profiles` — the poker-local shadow row keyed by ctech-account's `sub`. Holds the
  table nickname, `wallet_mode`, `deck_variant`, showcase settings and poker-ToS consent. It exists
  so every poker-owned row foreign-keys here instead of reaching into ctech-account's table, and so
  the poker fair-play addendum (collusion, chip-dumping, action-is-final) can be consented to
  separately from the platform ToS. Mirrors `ctech-wallet`'s own `wallet.User` shadow row.
- `poker_player_sessions` (per-table P&L, TTL), `poker_player_hands` (per-hand history incl. the
  fairness proof, GSI `gsi_table_id`), `poker_player_notes` (private per-viewer opponent notes),
  `poker_player_poker_stats` (materialised VPIP/PFR/3-bet + a per-hand guard row).

**Gamification**

- `poker_achievement_progress` — `sk` = achievement key, `counter` via atomic increment.
- `poker_leaderboard_stats` — GSIs `gsi_hands_won`, `gsi_hands_played`, `gsi_win_rate`. There is
  **no** `achievement_points` GSI, so that metric is rejected at the API rather than silently
  falling through to another ranking.
- `poker_daily_reward` — `sk` = day string, pending/completed, 48h TTL. Makes the 24h spin
  idempotent per day instead of relying on a timestamp comparison.

**Money**

- `poker_pending_cashouts` — the reconcile queue; `kind` separates a stuck cash-out from a stuck
  fee debit.
- `poker_hand_shares` — public shared-hand tokens with opponent aliases and a ≤30-day TTL.

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

- **No custom CloudWatch metrics and no alarms** (2026-08-19). The `api/internal/metrics` EMF
  emitter is gone — every `CtechPoker/<env>` series with it — and so are the `ALARM:` metric filter,
  the `LeaseFailovers` spike alarm, the archiver's DLQ-depth alarm and the operations dashboard.
  EMF lines were auto-extracted by CloudWatch and billed per metric; nobody was subscribed to the
  alarms. **Observability is structured logs**: `slog` JSON to `/ctech-poker/<env>/app`, queried
  with Logs Insights. An `ALARM:` line is something you look for, not something that pages you.
  Reinstate an alarm before anyone depends on being told.
- Every hand's full action sequence is durable in `poker_action_log` and archived to S3, which is
  what both the hand-history feature and dispute resolution read from.
- Health: `GET /v1.0/health` (liveness) and `GET /v1.0/health-check` (RFC-health detail: uptime,
  CPU, memory, a `DescribeTable` probe; 200/207/503).

## 8. Security

- **No hidden information ever leaves the server.** `Table.ViewFor(viewerID)` is the single
  source of truth for per-viewer visibility: hole cards the viewer may not see are replaced with
  the literal `"back"` *before* serialisation, per index, and fan-out is keyed
  `<tableID>#<viewerID>` precisely so no two seats can receive the same snapshot. There is no
  "send everything and hide it in the UI" path to accidentally regress into.
- **The fairness reveal is asymmetric on purpose** (OVERVIEW.md § 3.5): the server seed is
  published only for hands where nothing stayed hidden. Every other hand ships the seed-less
  per-position proof. Publishing the seed for a folded hand would retroactively expose mucked
  hole cards — treat any change that widens seed publication as a security change, not a
  feature.
- **Authorization**: bearer JWT verified against ctech-account's JWKS (`jwtverify`), requiring a
  non-empty `sub` **and** a non-empty `sid`. An empty `sid` marks an M2M client-credentials
  token, and those are rejected with 403 on every player-facing route — machine credentials must
  never be able to act as a player.
- **Rate limiting**: per-IP HTTP limiters on room creation (10/min), joins (30/min) and the daily
  reward (60/min) (`api/internal/api/v1/ratelimit.go`); the WebSocket caps frames at 32 KiB and
  requires the token in the first frame. **There is no WAF at the edge** — see `docs/README.md`.
- **Bot prevention**: adaptive Cloudflare Turnstile challenge issued over the table socket
  (`api/internal/botcheck`), verified server-side against siteverify.
- `currency_mode` boundary enforced server-side on every wallet-adjacent code path, not just
  in the UI (OVERVIEW.md § 5).
- Frontend hardening is at the CloudFront edge: CSP with `default-src 'self'`, HSTS 2y preload,
  `X-Frame-Options: DENY`, nosniff, and a `Permissions-Policy` that grants only
  `on-device-speech-recognition=self` (`cdk/lib/frontend-stack.ts`).
