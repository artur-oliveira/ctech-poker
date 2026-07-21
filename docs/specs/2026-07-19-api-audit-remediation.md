# API Audit & Remediation Spec — ctech-poker `api/`

**Date:** 2026-07-19
**Scope:** `api/` — security, concurrency, behavior bugs, duplication, cache/distributed-fleet problems, performance.
**Assumption (from task):** the service runs as a distributed, autoscaling fleet. Each instance owns its own
process-local state (`tablemanager.Manager.actors`, `tablelease` affinity, in-memory `ws.Registry`, `seatLimiter`); some
of that state is *correctness*-critical and some is only *latency*-critical. Findings below separate the two.

## Architectural context (as built)

- Correctness of table state rests **solely** on DynamoDB conditional writes (`tablestore.CommitAction`: version check +
  per-hand `action_id` idempotency guard). This holds across instances — verified.
- `tablelease` is explicitly *latency*, not correctness: a stale `trustCache` actor is always caught by the version
  conflict + single retry. This design is sound **provided the actor keeps serving**; the bugs below break that premise.
- Any instance may accept any table's WS connection / create any table's actor (no owner/proxy). Therefore the *
  *broadcast path must be fleet-wide**, and an instance that loses a lease must keep serving the table (just without
  cache affinity).

---

## HIGH severity

### H1. Dead Actor after lease loss → `Dispatch` hangs forever (distributed liveness)

**Where:** `api/internal/table/actor.go` (`Run` closes `a.done` at line 73), `api/internal/tablemanager/manager.go`
`GetOrCreateActor` (lines 77-90), `api/internal/tablelease/lease.go`.
**What:** For a `trustCache` actor the lease heartbeat's `onLost` calls `cancel()`, `Run` returns, `a.done` closes. But
the actor is **never removed from `m.actors`**. A subsequent `GetOrCreateActor` returns the dead actor; `actor.Dispatch`
does `a.cmds <- cmd; return <-cmd.reply()` — `cmds` is buffered so the send succeeds, but nothing reads `reply` ever
again → **request blocks until the HTTP read/write timeout**. This is triggered whenever an instance keeps running but
loses the lease (GC pause > TTL, Redis blip, network partition to Valkey) and a player stays connected/reconnects to
that instance. Under autoscaling this is the common path, not the edge.
**Fix:** Detect liveness in `GetOrCreateActor`: if the cached actor's `done` is closed, drop it and recreate (without
`trustCache`, since the lease is gone). Simplest: expose `(*Actor).Done() <-chan struct{}`, and in `GetOrCreateActor` do
`if existing.done closed { delete; recreate }` under the lock. Alternatively make `Dispatch` select on `a.done` and
return `ErrActorStopped` so callers fail fast and can recreate.

### H2. WebSocket realtime breaks when Redis is absent/falls back to in-memory (distributed realtime)

**Where:** `api/internal/app/app.go` `newCacheBackend` (112-122), `newWsRegistry` (128-136);
`api/internal/tablemanager/manager.go` `broadcast` (183-186) → `reg.Broadcast(tableID+"#"+viewerID, …)`.
**What:** When `RedisURL` is empty (or Redis errors) the app silently falls back to `cache.NewMemoryBackend` (logged "
NOT fleet-shared") and `ws.NewMemoryRegistry`. The `broadcast` closure then only reaches WS connections **on the same
instance**. Because any instance may host an actor for any table, a viewer connected to instance B receives **no state
updates** from an actor running on instance A. In a fleet this silently breaks the real-time game for the majority of
viewers. The fallback is acceptable for local dev but must not be a silent prod mode.
**Fix:** Treat absence of a shared registry in non-dev env as fatal (refuse to start, or at minimum refuse to serve
`/ws` with a 503). Keep the in-memory fallback only behind an explicit dev flag. Same reasoning applies to the JWT
verify cache (latency-only, lower priority).

### H3. `buyin` idempotency keys are unique per call → no replay protection (money correctness)

**Where:** `api/internal/buyin/service.go` `BuyIn` (line 97, `uuid.NewString()`) and `CashOut` (line 152).
**What:** The wallet idempotency key is `…#buyin#<uuid>` / `…#cashout#<uuid>` — a fresh random key every invocation. The
wallet's idempotency guard therefore **cannot** dedupe a retried request (client retry, ALB retry on a timeout after the
debit already succeeded). Result: a single logical buy-in can **double-debit** the sandbox wallet, and a cash-out can
double-credit. The sandbox credits service (same file area) correctly uses a *stable* per-day key — buyin should follow that
pattern.
**Fix:** Derive the key from stable inputs: `fmt.Sprintf("%s#%s#buyin", roomID, playerID)` (one buy-in per player per
room is already the model) plus, if re-buys are allowed, a client-supplied nonce or a monotonic seq persisted on the
player. Same for cash-out. Do **not** embed `uuid`.

### H4. Auto-fold timer data race on actor maps (concurrency)

**Where:** `api/internal/table/actor.go` `armActionDeadlineIfTheirTurn` (393-409).
**What:** The `time.AfterFunc` closure runs on a **separate goroutine** from `Run`, and mutates
`a.consecutiveDisconnectedHands[playerID]++` and reads `a.disconnectedSince[playerID]` directly, while `Run` writes both
maps via `handleDisconnect`/`handleReconnect`. Concurrent map access from two goroutines is a data race (can panic/throw
under race detector, and yields undefined behavior in prod). The `a.cached` read is avoided (commands are dispatched),
but the bookkeeping maps are not.
**Fix:** Do the consecutive-count / grace-window check **inside `Run`**. The timer should only `Dispatch` a single
envelope command (e.g. an `autoFoldCheckCmd`) that runs in `Run` and reads/writes those maps. Or move the bookkeeping
into the dispatched `SitOutCmd`/`ActCmd` handling. Do not touch actor fields from the timer goroutine.

---

## MEDIUM severity

### M1. `handleSitOut` silently drops a version conflict (behavior/correctness)

**Where:** `api/internal/table/actor.go` `handleSitOut` (302-312):
`if err := a.commit(...); err != nil && !errors.Is(err, tablestore.ErrVersionConflict) { return err }`.
**What:** Unlike every other mutating handler (Ready/Act/Join/Leave/PostBigBlind/Escalate, which retry exactly once on
`ErrVersionConflict`), SitOut **swallows** the conflict and returns success without applying the sit-out. The player's
sit-out is lost with no error to the caller. Inconsistent and data-losing.
**Fix:** Mirror the other handlers: on `ErrVersionConflict`, `ensureLoaded(ctx, true)` and re-apply once, then
broadcast.

### M2. Private-room blind escalation dies on instance/lease move (distributed behavior)

**Where:** `api/internal/api/v1/rooms.go` `createRoom` (lines 90-98) → `actor.StartEscalation(cfg)`;
`api/internal/table/escalation.go`.
**What:** `StartEscalation` is only ever called on the actor created during `createRoom`, i.e. on whatever instance
happened to create the room. The escalation worker correctly exits when that actor's `done` closes. But when the table
later moves to another instance (lease lost/expired, or another instance becomes authoritative), the **new** actor is
created via `GetOrCreateActor` without `StartEscalation`, so blinds stop escalating. Escalation is a property of the
*table*, not of one instance's actor.
**Fix:** Persist escalation config on the table (it already lives on `roomstore.Room.BlindEscalation`); have the actor (
re)arm escalation from authoritative config on creation/load, independent of which instance created the room. Or drive
escalation from a fleet-wide scheduler keyed by table ID in Valkey rather than a per-instance timer.

### M3. `Manager.GetOrCreateActor` TOCTOU → two live actors for one table on one instance (concurrency/correctness)

**Where:** `api/internal/tablemanager/manager.go` lines 44-95: map read (46) then unlock (50), then
load/seed/acquire/start, then re-lock and overwrite (88-90).
**What:** Two concurrent calls for the same `tableID` can both miss the map, both create an actor, last write wins; the
loser's actor is orphaned (may even hold the lease) and never serves traffic, while the map's actor may have **no**
lease (so it re-reads DynamoDB every command). Wasteful and a latent source of thundering-herd re-reads.
**Fix:** Hold the lock (or a per-table `sync.Once`) across the create path, or re-check the map under lock after
acquiring the lease and discard a duplicate.

### M4. `SeedTable` uses `PutItem` (overwrite), not a conditional create (latent)

**Where:** `api/internal/tablestore/dynamo.go` `SeedTable` (57-70). Comment claims "no-op if the table already exists"
but `PutItem` **unconditionally overwrites**.
**What:** `GetOrCreateActor` only calls `SeedTable` when `LoadTable` returned nil, so in practice it won't clobber a
live table. But the first-touch race window (two instances both see nil) allows a second `PutItem` to overwrite
state/version — currently harmless only because both seeds are identical. If seed inputs can ever differ (e.g. room
config edited between calls, or real-money rake), this resets a live table to version 1.
**Fix:** Use a conditional `PutItem` with `attribute_not_exists(pk)` (return `ErrVersionConflict`/ignore if present).
Matches the no-op claim.

### M5. Equity computed synchronously on the broadcast hot path (performance)

**Where:** `api/internal/table/actor.go` `broadcastAll` (426-437) → `attachEquity` (443-467) —
`equity.Estimate(hole, board, nil, opponents, 500)` per viewer, every committed action.
**What:** `broadcastAll` runs on the single `Run` goroutine (the table's serialization point). For an N-seat table every
action triggers N Monte-Carlo estimations (500 iters each) **sequentially**, blocking all further commands for that
table until equity is computed for every viewer. This is the dominant latency cost of the actor under load and scales
with seats.
**Fix:** (a) Compute equity **once per stage** and reuse across viewers (only the viewer's own hole cards differ, but
opponents/board are shared — a single estimate with the viewer's hole cards is still N calls, but caching per
`(stage, board)` is possible if hole cards are abstracted); (b) move estimation off the `Run` goroutine (async, attach
on arrival); (c) lower iterations or gate behind a flag (already gated by `equityEnabled`, but still synchronous when
on). At minimum, do not run it inside the serialization critical section.

### M6. No HTTP rate limiting / abuse controls on mutating endpoints (security/abuse)

**Where:** `api/internal/api/v1/rooms.go` (`createRoom`, `join`), `sandbox credits.go` (`Spin`), `leaderboard.go`.
**What:** Only the WS `seatLimiter` (per-connection, in-memory) exists. HTTP `POST /rooms`, `POST /rooms/:id/join`, and
`POST /v1.0/sandbox-credits` have no rate limit, so a script can spam room creation or (sandbox) chip spins. Low financial
risk today (sandbox) but abuse/DoS surface that grows with real money.
**Fix:** Add a shared (Redis-backed) rate limiter at the gateway for mutating endpoints; reuse `api-commons` rate-limit
if available, else a Valkey token-bucket. Note the WS `seatLimiter` is per-connection and therefore per-instance — under
a fleet a player with two connections gets 2× the allowance; acceptable but document.

---

## LOW severity

### L1. Seed-building logic duplicated 3×, and `createRoom` seed omits `ConfigureRake` (DRY + latent real-money bug)

**Where:** `app/app.go` `roomBackedSeed` (207-222), `buyin/service.go` `seedFor` (57-68), `api/v1/rooms.go` `createRoom`
closure (92-94).
**What:** Three near-identical closures build the first `hand.Table`. The `createRoom` one calls `hand.NewTable(...)` *
*without** `ConfigureRake`; the others call it. Sandbox rake is 0 so today it's harmless, but the moment real-money
ships the createRoom-path seed would produce a rake-misconfigured table (`rakeBPS=0`, no rake collected). Three copies
also drift. **Severity escalates to HIGH once real-money ships.**
**Fix:** Single `seedTable(room *roomstore.Room) *hand.Table` helper (or extend `roomBackedSeed` to be the one source)
that always calls `ConfigureRake(room.CurrencyMode)`. Wire all three call sites to it.

### L4. Retry-once version-conflict loop duplicated 6× (DRY)

**Where:** `api/internal/table/actor.go` — `handlePostBigBlind` (119-124), `handleEscalate` (140-145), `handleReady` (
180-193), `handleAct` (219-225), `handleJoin` (319-329), `handleLeave` (361-366). Only `handleSitOut` (302-312) omits
it (see M1).
**What:** Identical `if errors.Is(err, ErrVersionConflict) { ensureLoaded(true); re-apply }` block copy-pasted into 6
handlers; `handleSitOut` is the inconsistent one. Drift risk: any future handler will likely forget the retry.
**Fix:** Extract `retryOnConflict(apply func() error) error` that does the load+reapply once, and have all mutating
handlers (including SitOut) call it.

### L5. WS auth/origin helpers hand-mirror ctech-wallet (DRY / shared-code policy)

**Where:** `api/internal/api/v1/tablews.go` `readAuthToken` (48-63), `wsAllowedOrigin` (69-83). Comments say "Mirrors
ctech-wallet's internal/api/v1/ws.go".
**What:** Bearer-frame read + origin check are copy-pasted from the sibling repo rather than imported. Per CTech "one
codebase" policy (reuse `ctech-go-common`/`api-commons`), this belongs in `api-commons/ws` (or `ctech-go-common`) as a
shared helper.
**Fix:** Promote `readAuthToken`/`wsAllowedOrigin` (or their equivalents) into `api-commons/ws`; import here.

### L6. Dead code (cleanup)

- `api/internal/roomstore/dynamo.go` `SetStatus` — defined, zero call sites.
- `api/internal/engine/deck/deck.go` `Verify` — defined, zero call sites.
- `api/internal/tablelease/lease.go` `Renew` — only referenced from `lease_test.go`; `StartHeartbeat` uses
  `Locker.Renew` internally.
  **Fix:** Delete or wire up. (Verified alive, not dead: `EquityDisplayEnabled`, `BlindEscalation`, `StartEscalation` —
  all referenced.)

### L2. Chat filter is trivially weak (security/quality)

**Where:** `api/internal/api/v1/tablews.go` `tableChatFilter = chatfilter.New([]string{"idiota", "burro"})` (line 41).
**What:** Two exact-match words, no normalization (accent/case/leetspeak). Easy to bypass; gives false confidence.
**Fix:** Move to a configurable list + case/accent-insensitive matching, or drop the claim of moderation. Low priority.

### L3. `getByShareCode` returns the full room incl. `ShareCode` (info hygiene)

**Where:** `api/internal/api/v1/rooms.go` `getByShareCode` (128-137) returns `room` (not `sanitizeRoom`).
**What:** The caller already knows the code, so leaking it back is low risk, but it's inconsistent with `getRoom` which
sanitizes. Minor.
**Fix:** Return `sanitizeRoom(room, claims.Sub)` for consistency.

### L4. `applyReadyAndCommit` swallows `StartHand` "need ≥2 ready players" (acceptable but undocumented)

**Where:** `api/internal/table/actor.go` (204-211). Intentional (table keeps waiting). Documented in code; no action.

---

## Distributed-fleet summary (the headline risk)

| Concern                                                 | Status today             | Breaks under fleet? |
|---------------------------------------------------------|--------------------------|---------------------|
| Table state correctness (Dynamo version + action guard) | Correct                  | No                  |
| WS realtime broadcast                                   | Memory registry fallback | **Yes (H2)**        |
| Actor liveness after lease loss                         | Hangs (H1)               | **Yes**             |
| Blind escalation continuity                             | Dies on move (M2)        | **Yes**             |
| JWT verify cache sharing                                | Memory fallback          | No (latency only)   |
| Per-instance actor map                                  | Leaks + stale (H1/M3)    | Yes                 |

---

## Remediation plan (suggested order)

1. **H1** — actor liveness detection + recreation (unblocks safe autoscaling).
2. **H2** — fail closed (or dev-gate) when no shared `ws.Registry`.
3. **H3** — stable buyin/cashout idempotency keys.
4. **H4** — move auto-fold bookkeeping into `Run`.
5. **M1** — SitOut version-conflict retry.
6. **M2** — escalation from authoritative table config.
7. **M3** — lock/Once around actor create.
8. **M4 / L1** — conditional seed + single seed helper.
9. **M5** — offload/parallelize equity.
10. **M6 / L2 / L3 / S5** — rate limits, chat filter, sanitize `getByShareCode`, document/implement `cursor` pagination.
11. **M7** — stable cash-out idempotency key + durable retry/outbox for the post-removal credit.
12. **Engine E3** — no fix required (cosmetic). H4/M5 cover engine-layer work.

## ENGINE behavior & concurrency (hand/betting/sidepots/equity/deck/handeval)

Manual review of the core engine found **no money-correctness defects** in `betting.Round`, `sidepots.ComputeSidePots`,
`equity.Estimate`, or `deck` shuffle:

- Betting: min-raise / short-all-in (sub-min raise does not reopen action) logic is correct; `IsComplete` gating
  correct.
- Side pots: ascending level layering with `delta * eligibleCount` is correct; folded players excluded from `Eligible`
  by the caller.
- Equity: tie-counting (`tiedWinners` with the `score == bestScore && score == myScore` guard) is correct across
  win/tie/loss/three-way cases.
- Deck: HMAC-SHA256 Fisher-Yates with rejection sampling is unbiased; `Verify` is sound (dead code, see L6).

Engine-layer issues that remain:

- **E1 (HIGH, concurrency):** auto-fold timer data race — see **H4** above (the only engine-layer race; it lives in
  `table/actor.go`, not in the pure engine, but it mutates engine state off the `Run` goroutine).
- **E2 (MED, performance):** equity computed synchronously inside `broadcastAll` on the `Run` goroutine — see **M5**
  above. For a 9-seat table this is ~9 Monte-Carlo runs (500 iters) per committed action, on the table's single
  serialization point.
- **E3 (LOW, accuracy):** `attachEquity` counts a `SittingOut`/`Disconnected` (still-dealt-in) player as an *opponent*
  when `opponents` is computed, slightly inflating the estimated equity denominator. Cosmetic, not money. No fix
  required for correctness.

No further engine changes recommended beyond H4/M5.

## SECURITY (HTTP/WS/services)

**Positives (verified):**

- WS `act`/`chat`/`ready` derive `PlayerID` from `claims.Sub` (JWT), **not** the client message → no IDOR (player A
  cannot act for player B).
- Private rooms: WS gate + HTTP join gate both call `privateRoomAccessAllowed` with `subtle.ConstantTimeCompare` on the
  share code; `sanitizeRoom` strips `ShareCode` for non-creators on `GET /rooms/:id`.
- CORS: dev = wildcard, no credentials; prod = explicit origins + `AllowCredentials`. `CheckOrigin` mirrors this on the
  WS upgrade.
- JWT bearer enforced on all `/rooms`, `/players`, `/sandbox credits` groups. `createRoom`/`join` re-derive the actor from the
  token user, not the body.

**Findings:**

- **S1 (HIGH, money):** `buyin` idempotency keys are unique per call (`uuid.NewString()`) — see **H3**. Under
  at-least-once delivery a retried buy-in/cash-out double-moves chips. (Contrast: `sandbox credits.Spin` correctly uses a
  stable per-day key.)
- **S2 (MED, abuse/DoS):** No HTTP rate limiting on `POST /rooms`, `POST /rooms/:id/join`, `POST /v1.0/sandbox-credits`. A
  script can spam room creation (Dynamo write amplification) or sandbox chip spins. See **M6**. The WS `seatLimiter` is
  only per-connection/per-instance.
- **S3 (LOW, info hygiene):** `GET /rooms/code/:code` returns the full `Room` (incl. `ShareCode`) instead of
  `sanitizeRoom` — see **L3**.
- **S4 (LOW, quality):** chat profanity filter is two exact-match words, no normalization — see **L2**.
- **S5 (LOW, no-op param):** `GET /rooms` accepts `cursor` but `roomstore.ListPublic` always returns `""` (pagination
  out of scope). Silent no-op; document or implement.
- **S6 (INFO):** `GET /leaderboard` is intentionally **unauthenticated** (public read-only); `metric`/`limit` are
  validated. Acceptable, no change.
- **S7 (LOW, resilience):** `buyin.CashOut` credits the wallet *after* removing the seat with no compensating action on
  failure — chips can be lost between table and wallet (already flagged in code as a known gap). See **M7** below.

### M7. `buyin.CashOut` has no compensating action on credit failure (money)

**Where:** `api/internal/buyin/service.go` `CashOut` (127-157).
**What:** The seat is removed (committed) and then `wallet.Credit` is called with a fresh `uuid` idempotency key. If the
credit fails (wallet down, timeout), the player's chips are gone from the table but not in their wallet, and the unique
key means a retry can double-credit. The code comment acknowledges this as a known gap.
**Fix:** (a) make the cash-out idempotency key stable (e.g. `roomID#playerID#cashout` — one cash-out per seat per room)
so a retry is safe; (b) on credit failure, either re-add the player to the seat (reverse the LeaveCmd) or enqueue a
durable retry/outbox so the credit eventually lands. At minimum, surface a clear "manual reconciliation" error and log
the exact (player, amount) for ops.

## Open questions for the user

- Is `redisURL` expected to always be present in prod? If yes, H2 becomes a "fail fast if absent" change; if no, the
  in-memory fallback needs a different design.
- Are repeat buy-ins into the same room allowed? Determines the idempotency key shape for H3.
- Real-money timeline — gates urgency of L1 (rake) and M6.

---
*All three review lenses (core/distributed, DRY/dead-code/cache, engine, security) MERGED. Findings: H1-H4, M1-M7,
L1-L6, E1-E3, S1-S7.*
