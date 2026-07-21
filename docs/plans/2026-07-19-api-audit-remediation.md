# Implementation Plan — ctech-poker `api/` Audit Remediation

**Date:** 2026-07-19
**Source spec:** `docs/specs/2026-07-19-api-audit-remediation.md`
**Principle:** distributed, autoscaling fleet. Correctness rests on DynamoDB conditional writes; the lease bounds
latency only. Every fix must keep the table correct even when an instance's in-memory cache is stale or its actor has
died.

## Decisions (from user)

- **Redis is mandatory in prod.** If `RedisURL` is empty in non-`dev`, the server must **refuse to start** (H2).
  In-memory cache/registry fallback is dev-only.
- **Buy-in idempotency is stable per (room, player).** Repeat rebuys are NOT allowed. A player who runs out of chips
  sits out; the UI offers a rebuy, which is a *new* logical buy-in carrying a fresh client nonce. The idempotency key
  therefore includes a client-supplied nonce so a network retry of one attempt cannot double-debit, while a genuine
  rebuy (new nonce) debits once (H3).
- **Real money is phase 5 (not started).** Rake (L1) is a latent bug; fix the DRY/seed path now so it is correct when
  phase 5 lands. M6 urgency is lower but cheap given Redis is present.

---

## T1 — Dead actor liveness + recreation (H1) — *highest priority*

**Goal:** A request/Dispatch to a lease-killed actor never hangs; the manager always serves a live actor.
**Files:** `table/actor.go`, `tablemanager/manager.go`, `api/v1/tablews.go`, `buyin/service.go`.
**Steps:**

1. `actor.go`: add `func (a *Actor) Done() <-chan struct{} { return a.done }` and
   `var ErrActorStopped = errors.New("table: actor stopped")`.
2. `actor.go Dispatch`: `select { case a.cmds <- cmd: return <-cmd.reply(); case <-a.done: return ErrActorStopped }`.
3. `manager.go GetOrCreateActor`: under `m.mu`, if the cached actor's `Done()` is closed, `delete(m.actors, tableID)`
   then create fresh (no `trustCache`). Add `removeActor(tableID)` helper.
4. `manager.go`: in the `StartHeartbeat` `onLost` callback, also call `m.removeActor(tableID)` (active cleanup); lazy
   cleanup in step 3 backs it up.
5. `tablews.go`: on `ErrActorStopped` from any `Dispatch`, re-resolve via
   `manager.GetOrCreateActor(ctx, tableID, seed(tableID))` once and retry the command; if still failing, close the
   connection. (The WS handler caches `actor` for the connection lifetime, so it must re-resolve on death.)
6. `buyin`: `BuyIn`/`CashOut` already call `GetOrCreateActor` immediately before `Dispatch`, so the recreated actor is
   used; the `ErrActorStopped` guard prevents the residual hang window.
   **Verify:** `go test -race ./api/internal/table/... ./api/internal/tablemanager/...`; integration test: start actor
   with trustCache, force lease loss (cancel runCtx), assert a subsequent `Dispatch` returns `ErrActorStopped` and a
   fresh `GetOrCreateActor` returns a working (non-trustCache) actor.

## T2 — Fail fast without Redis in prod (H2)

**Goal:** Never run prod with in-memory cache/registry (breaks cross-instance realtime).
**Files:** `app/app.go`.
**Steps:**

1. Change `newCacheBackend` and `newWsRegistry` to return `(T, error)`.
2. If `cfg.Env != "dev" && cfg.RedisURL == ""` → return error "redis required in non-dev env".
3. Wire the errors through `fx.Provide` (functions may return error) so the app fails to start.
4. Keep in-memory fallback only when `cfg.Env == "dev"`.
   **Verify:** `RedisURL="" go run ./cmd/server` in `env=prod` exits non-zero with a clear log; `env=dev` starts with
   the warning. `go build ./...`.

## T3 — Stable buy-in/cash-out idempotency (H3, S1)

**Goal:** Retries of one logical buy-in/cash-out cannot double-move chips.
**Files:** `buyin/service.go`, `api/v1/rooms.go` (`JoinRoomRequest`), `api/v1/tablews.go` (if WS triggers buyin —
currently only HTTP `join` does), `api/v1/roomdto.go`.
**Steps:**

1. `roomdto.go`: add `IdempotencyKey string \`json:"idem_key,omitempty"\`` to `JoinRoomRequest`.
2. `buyin.BuyIn`: accept `idemKey string`; build `fmt.Sprintf("%s#%s#buyin#%s", roomID, playerID, idemKey)`. If caller
   passes empty, derive a *stable* default `roomID#playerID#buyin` (one buy-in per seat; repeats dedupe — matches "no
   repeated rebuys"). The UI supplies a fresh nonce per genuine rebuy.
3. `buyin.CashOut`: key `fmt.Sprintf("%s#%s#cashout", roomID, playerID)` (one cash-out per seat) — stable, retry-safe.
4. Drop the `uuid.NewString()` calls in both.
5. `join` handler: pass `req.IdempotencyKey` through.
   **Verify:** unit test — call `BuyIn` twice with same `idemKey`; assert wallet `Debit` invoked once (wallet client
   mock counts calls). `go test ./api/internal/buyin/...`.

## T4 — Auto-fold bookkeeping inside Run (H4, E1)

**Goal:** No goroutine touches actor maps outside `Run`.
**Files:** `table/actor.go`, `table/commands.go`.
**Steps:**

1. `commands.go`: add `autoFoldCheckCmd{ PlayerID string; Reply chan error }`.
2. `actor.go armActionDeadlineIfTheirTurn`: the `time.AfterFunc` closure must ONLY `Dispatch(autoFoldCheckCmd{...})` —
   remove the `consecutiveDisconnectedHands++` and the `disconnectedSince` read from the closure.
3. `handle` switch: add case `autoFoldCheckCmd` → runs in `Run`: increment `consecutiveDisconnectedHands`, then if grace
   exceeded or count>=3 dispatch fold-via-direct-apply (call the same apply path as `ActCmd` fold) and `SitOutForActor`/
   `commit`; else apply the fold directly. All map access now in `Run`.
   **Verify:** `go test -race ./api/internal/table/...` with disconnect+auto-fold simulated; `go run -race` smoke test
   of a timed-out disconnect.

## T5 — SitOut version-conflict retry (M1)

**Files:** `table/actor.go`.
**Steps:** Change `handleSitOut` to mirror the other handlers: on `ErrVersionConflict`, `ensureLoaded(ctx, true)` +
re-apply `SitOutForActor` + `commit` once, then broadcast; surface real errors.
**Verify:** unit test — force a version conflict on sit-out, assert the sit-out is eventually applied and `broadcastAll`
ran. `go test ./api/internal/table/...`.

## T6 — Escalation from authoritative room config (M2)

**Goal:** Blind escalation survives instance/lease moves.
**Files:** `tablemanager/manager.go`, `app/app.go`, `api/v1/rooms.go`.
**Steps:**

1. `manager.go`: add a `roomLoader func(tableID string) (cfg *roomstore.BlindEscalation, ok bool, err error)` field, set
   in `NewManager` (app passes `func(id){ r,_ := rooms.Get(ctx,id); return r.BlindEscalation ... }`).
2. In `GetOrCreateActor`, after creating the actor, load the room; if `BlindEscalation != nil`, call
   `actor.StartEscalation(cfg)`. This re-arms on every creation, on any instance.
3. `createRoom`'s `onCreated` escalation hook becomes redundant; keep or remove (remove to avoid double-start —
   `StartEscalation` already guards via `escalationInterval`/early-return, so harmless; prefer removing the createRoom
   hook to keep one path).
   **Verify:** integration test — create private room with escalation, kill the creating instance's actor (cancel
   runCtx), reconnect from another instance, assert blinds still escalate after `IntervalMinutes`.

## T7 — Lock around actor creation (M3)

**Files:** `tablemanager/manager.go`.
**Steps:** Hold `m.mu` across the load/seed/Acquire/start path (the Dynamo + lease Acquire are fast). Re-check the map
under lock after Acquire; if a concurrent caller already created it, discard the new actor (close its `done`, cancel its
ctx) and return the existing one.
**Verify:** `go test -race` with N concurrent `GetOrCreateActor` for the same tableID; assert exactly one live actor in
`m.actors` and one lease acquire.

## T8 — Conditional seed + single seed helper (M4, L1)

**Files:** `tablestore/dynamo.go`, `app/app.go`, `buyin/service.go`, `api/v1/rooms.go`.
**Steps:**

1. `tablestore`: add `SeedTableIfAbsent` using a conditional `PutItem` (`attribute_not_exists(pk)`); treat
   `ConditionalCheckFailed` as success (already seeded). Keep `SeedTable` name, change impl.
2. Add one `seedTable(room *roomstore.Room) *hand.Table` helper (in `app` or a small `internal` pkg) that always calls
   `ConfigureRake(room.CurrencyMode)`; wire all three current call sites (`app.roomBackedSeed`, `buyin.seedFor`,
   `rooms.createRoom` closure) to it. Removes the createRoom rake omission (L1).
   **Verify:** unit test — `SeedTableIfAbsent` called twice with different state; assert the second is a no-op (
   version/state unchanged). `go test ./api/internal/tablestore/...`.

## T9 — Equity off the Run hot path (M5, E2)

**Files:** `table/actor.go`.
**Steps:** In `broadcastAll`, do NOT block `Run` on `attachEquity`. Instead spawn a goroutine per (viewer, snapshot)
that computes `equity.Estimate` and, when ready, sends a supplementary WS message
`{"type":"equity","player_id":...,"equity":...}` via the registry (not via the actor's serialized commit path). Guard
with a per-snapshot version so stale estimates are dropped. Consider lowering default iterations 500→250.
**Verify:** benchmark — time `broadcastAll` for a 9-seat table before/after; assert `Run` is no longer blocked by
equity. `go test ./api/internal/table/...`.

## T10 — HTTP rate limiting + chat/sanitize/no-op (M6, L2, L3, S5)

**Files:** `app/app.go` (middleware), `api/v1/router.go`, `api/v1/rooms.go`, `api/v1/tablews.go`.
**Steps:**

1. Add a Redis-backed fixed-window (or token-bucket) rate-limiter middleware; register on `POST /rooms`,
   `POST /rooms/:id/join`, `POST /v1.0/sandbox-credits` (429 on exceed). Since Redis is mandatory (T2), use it directly.
2. `getByShareCode`: return `sanitizeRoom(room, claims.Sub)` instead of raw `room` (L3).
3. `tableChatFilter`: document it is cosmetic; optionally replace with case/accent-insensitive matching or drop the
   moderation claim (L2) — low priority, defer.
4. `ListPublic` `cursor`: either implement `roomstore` pagination or remove the accepted param and document it (S5).
   **Verify:** test — burst `POST /rooms` > limit, assert 429. `go test ./api/internal/api/v1/...`.

## T11 — Cash-out idempotency + retry safety (M7, S7)

**Files:** `buyin/service.go`, `tablestore` (new outbox table optional).
**Steps:**

1. Stable cash-out key per T3 step 3.
2. On `wallet.Credit` failure after seat removal: do NOT lose chips silently. MVP: re-seat the player (reverse the
   `LeaveCmd` by re-dispatching a `JoinCmd` with the same stack) so the table state is consistent, return a clear error,
   and log `(player, amount)` for ops. Phase 2: enqueue a durable outbox record (Dynamo) + background worker that
   retries the credit until it lands, then removes the seat.
   **Verify:** unit test — `CashOut` where wallet `Credit` fails; assert (a) key is stable across retries, (b) chips are
   either re-seated or recorded for retry, never silently lost.

## T12 — DRY/dead-code cleanup (L4, L5, L6) + engine note (E3)

**Files:** `table/actor.go`, `api/v1/tablews.go`, `roomstore/dynamo.go`, `engine/deck/deck.go`, `tablelease/lease.go`.
**Steps:**

1. `actor.go`: extract `retryOnConflict(apply func() error) error` and use in all 7 mutating handlers (incl. SitOut via
   T5) (L4).
2. `tablews.go`: promote `readAuthToken`/`wsAllowedOrigin` into `api-commons/ws` (or `ctech-go-common`) and import;
   CTech "one codebase" policy (L5).
3. Delete dead code: `roomstore.SetStatus`, `deck.Verify`, `tablelease.Renew` (only used in test) (L6).
4. Engine E3: no code change (cosmetic equity opponent count); optionally skip `SittingOut`/`Disconnected` players in
   `attachEquity` `opponents` — minor, defer.
   **Verify:** `go build ./...`, `go vet ./...`, `go test ./...`.

---

## Order & dependencies

1. T2 (fail-fast) — independent, do first (cheap, unblocks confidence).
2. T1 (liveness) — independent, do first (fleet safety).
3. T3, T4, T5 — independent, can parallelize.
4. T6, T7, T8 — independent.
5. T9 (perf) — independent.
6. T10, T11 — independent.
7. T12 — cleanup, last.

## Global verification gate

- `go build ./...`
- `go vet ./...`
- `go test ./...`
- `go test -race ./api/internal/table/... ./api/internal/tablemanager/... ./api/internal/buyin/...`
- Manual: deploy two instances against one Redis + DynamoDB; kill the instance holding a table's lease; confirm (a) no
  hung requests, (b) viewers on the other instance keep receiving state, (c) blind escalation continues if configured.
