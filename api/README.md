# ctech-poker — API (game server)

Go real-time poker game server. **Sandbox (play-money) mode is implemented end-to-end.**
Real-money mode is **not started** (Phase 5, gated on a Brazilian regulatory opinion —
see [`../PLAN.md`](../PLAN.md) and [`../OVERVIEW.md`](../OVERVIEW.md) §11).

> All claims below are anchored to the implementation (`api/`), not to the design docs
> (`ARCHITECTURE.md`/`OVERVIEW.md`), which are proposals and may describe features not yet built.

## Stack

- Go `1.26.5`, module `gopkg.aoctech.app/poker/api` (`go.mod:1`).
- HTTP: **Fiber v3** (`go.mod:14`); WebSocket: **`fasthttp/websocket`** (`go.mod:13`).
- State/cache/registry: **Valkey** (`valkey-io/valkey-go`, `go.mod:16`) — used for the
  `ws.Registry` fan-out, the JWT-verify cache, and the `tablelease` latency hint.
- Store: **DynamoDB** (`aws-sdk-go-v2/service/dynamodb`), Streams on the action log
  (`aws-sdk-go-v2/service/dynamodbstreams`).
- Shared CTech code: **`gopkg.aoctech.app/api-commons` v1.2.0** (`go.mod:19`) provides
  `jwtverify`, `ws.Registry`, `cache.Backend`, `problem`.
- Archiver: **`aws-lambda-go`** (`go.mod:6`) — a separate Lambda binary (see `cmd/archiver`).

## Build / test / run

- `make build` → `dist/app` (linux/arm64, CGO off) — binary **must** be named `app`
  (the CDK user-data expects `/opt/app/current/app`; see `Makefile:1`).
- `make test` → `go test ./... -race -coverprofile=coverage.out`.
- Integration tests (`tests/integration/tableflow_test.go`) run against **DynamoDB Local**
  via `docker-compose.test.yml` (in-memory local instance on `:8555`).
- Dockerfile: `golang:1.26-alpine` builder → `distroless/static-debian12`, `EXPOSE 8003`.
- Deploy: GitHub Actions `api.yml` builds the binary, uploads to the shared deployments S3
  bucket, and does a rolling SSM deploy across the EC2 Auto-Scaling Group (see [`../cdk/README.md`](../cdk/README.md)).

## Real-time transport (WebSocket)

- Endpoint: `GET /v1.0/tables/:id/ws` (`internal/api/v1/tablews.go:133`).
- Upgraded by `fasthttp/websocket` `FastHTTPUpgrader`; origin check mirrors HTTP CORS
  (`wsAllowedOrigin`, `tablews.go:75`).
- **Auth over the socket is a first frame**, not a header: the client sends
  `{"token":"…","share_code":"…"}` (or a raw bearer token) immediately after upgrade
  (`readAuthToken`, `tablews.go:49`). A missing/invalid frame fails closed
  (`tablews.go:143-153`).
- **Private rooms are invite-only end-to-end**: the WS gate re-checks
  `privateRoomAccessAllowed(room, playerID, shareCode)` with a constant-time share-code
  compare (`tablews.go:166`), mirroring the HTTP join gate.
- Fan-out: the actor broadcasts via `ws.Registry` (`reg.Broadcast`, `tablews.go:273`).
  The registry is **Redis/Valkey-backed in prod**; an in-memory fallback exists for `dev`
  only (absence of Valkey in non-dev must fail fast — remediation T2).
- **Client → server** message types (`tablews.go:33-40`, handler `:247-274`):
  `ping`, `ready{ready}`, `act{action,amount,action_id}`, `post_big_blind`, `chat{message}`.
- **Server → client** message types: `connected{conn_id}`, `state{snapshot}`
  (full authoritative snapshot pushed on join and on every mutation — no replay-based
  resync), `pong`, `chat{player_id,message}`, `error{code}` (`unauthorized` /
  `unavailable` / `forbidden` / `rate_limited` / `invalid_action` / `invalid_post` /
  `message_too_long`), and `achievement_unlocked{key,stars}` delivered through the same
  broadcast channel from the actor.
- Abuse control: per-seat fixed-window limiter, **10 actions/sec/seat** (`seatLimiter`,
  `tablews.go:225`); chat is truncated to 500 chars and run through a trivial 2-word filter
  (`tableChatFilter`, `tablews.go:42` — see Known Issues).
- Heartbeat: 30s ping / pong-wait 45s (`tablews.go:26-29`).

## Game-server model (per-table actor + DynamoDB conditional writes)

This matches the **revised** model in `ARCHITECTURE.md §2` — *not* a Redis-lease
authority.

- Each table is served by a **per-table actor** (`internal/table/actor.go`, `Run` loop,
  command channel `cmds`). Any instance may create/serve any table's actor via
  `tablemanager.Manager.GetOrCreateActor` (`internal/tablemanager/manager.go`) — there is
  no owner/proxy.
- **Correctness rests on DynamoDB conditional writes**, not on-process state: every player
  action commits via `tablestore.CommitAction` with a `version` equality
  `ConditionExpression` plus a per-`(table_id, hand_id, seat, action_id)` idempotency
  guard (`poker_action_guards` table). A version conflict is retried exactly once by the
  actor (the `retryOnConflict` pattern).
- **`tablelease` is latency-only.** It is a Valkey affinity hint for read-through caching;
  if the lease is lost the actor still re-reads DynamoDB directly. The socket handler
  re-resolves a live actor on `table.ErrActorStopped` (`tablews.go:185-198`) so a
  lease-killed actor cannot hang a request (remediation T1 is already in code).
- **Crash recovery is trivial**: state is durable after every single action, so a crashed
  instance loses at most the in-flight request; the next action (any instance) re-reads
  DynamoDB and proceeds.
- Engine (pure logic, no networking): `internal/engine/{hand,betting,sidepots,equity,deck}`.
  `sidepots.ComputeSidePots`, 7-card evaluator, and HMAC-SHA256 Fisher–Yates shuffle with
  rejection sampling are present and unit-tested. A scripted **hand-replay harness** lives
  at `cmd/handreplay` (`script.example.json`, `script.fold.json`) — the Phase-1 deliverable.

## HTTP endpoints (`/v1.0`)

| Method & path | Auth | Notes |
|---|---|---|
| `GET /health` | none | liveness (`health.go:106`) |
| `GET /health-check` | none | detailed dep report; ALB target group (accepts 200/207) (`health.go:110`) |
| `GET /tables/:id/ws` | first-frame JWT | WebSocket (above) (`tablews.go:133`) |
| `POST /rooms/` | JWT | create room; rate-limited (10/min/IP) (`rooms.go:29`) |
| `GET /rooms/` | JWT | list public rooms (`rooms.go:30`) |
| `GET /rooms/stakes` | JWT | curated stake list (`rooms.go:31`) |
| `GET /rooms/code/:code` | JWT | lookup by share code (`rooms.go:32`) |
| `GET /rooms/:id` | JWT | room detail (sanitized for non-creators) (`rooms.go:33`) |
| `POST /rooms/:id/join` | JWT | join + buy-in; rate-limited (30/min/IP) (`rooms.go:34`) |
| `POST /rooms/:id/leave` | JWT | leave (`rooms.go:35`) |
| `POST /rooms/:id/ready` | JWT | ready toggle (`rooms.go:36`) |
| `GET /players/me` | JWT | player profile + terms state (`player.go:14`) |
| `POST /players/me/terms/accept` | JWT | accept poker ToS addendum (`player.go:15`) |
| `GET /leaderboard` | **none** | see Known Issues B9 (`leaderboard.go:11`) |
| `POST /sandbox-credits` | JWT | daily sandbox-chip spin; rate-limited (60/min/IP) (`sandbox credits.go:10`) |

Auth group wiring: `RegisterRooms/Players/sandbox credits` all receive `auth` (`router.go:43-46`);
`RegisterLeaderboard` is registered **without** `auth` (`router.go:45`) — intentional per
the audit but see B9.

## Authentication & authorization — ⚠️ B9 known risk

- `authMiddleware` (`internal/api/v1/auth.go:13-25`) verifies the bearer JWT and sets
  `c.Locals("user_id", claims.Sub)`. The **only** authorization check is
  `claims.Sub == ""` → reject (`auth.go:20`). There is **no scope, KYC, or role check**.
- The WebSocket gate derives `playerID` from `claims.Sub` (not the client body), so a
  player cannot act for another — good. But the *same* `sub`-only guard is the entire
  authz surface for every player/room endpoint.
- **`GET /leaderboard` is unauthenticated** (`leaderboard.go:11`, `router.go:45`). Public
  read-only leaderboard is a deliberate product choice, but it means the endpoint performs
  no auth at all.
- **Machine (M2M) credentials are not distinguished from user credentials.** The server
  itself uses an M2M client (`SSM_POKER` client-id/secret, `cdk/lib/constants.ts:103-107`)
  to call `ctech-wallet`. A token that carries any non-empty `sub` (including an M2M client
  credential with no session/sid) satisfies the `sub`-only guard and could call
  player/room endpoints. **Hypothesis to confirm against the token issuer (ctech-account):**
  M2M tokens populate `sub` with the client id and lack a user `sid`, so they pass today.
  Not yet exploitable for real funds (sandbox-only), but must be fixed before real-money.
- **Tracked as a known risk to fix, not accepted.** See [`api/CLAUDE.md`](./CLAUDE.md).

## Sandbox ledger (play-money, isolated from ctech-wallet)

- `internal/walletclient` calls **only** `ctech-wallet`'s internal sandbox credit/debit
  routes (`/v1.0/internal/wallet/sandbox/credit|debit`,
  `internal/walletclient/client.go:23-24`). It authenticates with the poker M2M client and
  scopes `internal:wallet:credit` / `internal:wallet:debit` (`client.go:26-27`).
- `buyin` (`internal/buyin/service.go`) gates every buy-in/cash-out on
  `room.CurrencyMode == "sandbox"`, returning `ErrUnsupportedCurrencyMode` otherwise
  (`service.go:82-83`, `:182-183`). **There is no real-money code path** — the
  `currency_mode` boundary is enforced server-side (OVERVIEW §5).
- Sandbox chips are non-convertible by construction: the only wallet calls are the sandbox
  endpoints, and there is no hold/capture or conversion route. Real-money integration
  (Phase 5) depends on `ctech-wallet` first gaining hold/capture endpoints and raising its
  DynamoDB throughput cap (see `OVERVIEW.md §5` / `ARCHITECTURE.md §4`).

## Known issues (documented honestly — do NOT fix code here)

- **B9 — authz is `sub`-only** (above). `auth.go:20`; leaderboard unauthenticated
  `leaderboard.go:11`; M2M not distinguished.
- **B10 — archiver Lambda has no DLQ** (`cdk/lib/archiver-stack.ts:71-75`):
  `DynamoEventSource` sets `retryAttempts: 3` but **no `onFailure` / dead-letter queue**. A
  poison record that fails 3× is dropped. Archiver Lambda: `cmd/archiver/main.go`
  (30s timeout, default memory).
- **B31 — `leaderboard.Top("achievement_points")` returns the wrong ranking**
  (`internal/leaderboard/store.go:105-133`): for `achievement_points` the code sets only
  `queryLimit=1000` but still queries the **`gsi_hands_won`** index (the function default at
  `store.go:106`). The `poker_leaderboard_stats` table has GSIs only for `hands_won`,
  `hands_played`, and `win_rate` (`cdk/lib/dynamodb-stack.ts:78-95`) — **there is no
  `achievement_points` GSI** — so the call silently returns a *hands-won* ranking, not an
  achievement-points ranking.
- **B32 — commit-reveal fairness is not verifiable by clients**
  (`internal/engine/deck/deck.go:50-69`): the shuffle produces a `ServerSeed` and
  `CommitHash` and the code comment says the `CommitHash` is safe to publish
  (`deck.go:51`) — **but no HTTP or WS endpoint publishes `CommitHash` or reveals
  `ServerSeed`**. The primes exist; the surface to verify them does not. Fairness is
  therefore currently unprovable by players (OVERVIEW §3.5 / ARCHITECTURE.md §2's
  "commit-reveal surface" remain DESIGNED-ONLY).
- **Remediation context:** `docs/plans/2026-07-19-api-audit-remediation.md` (and its spec)
  is a separate audit covering H1–H4, M1–M7, L1–L6, E1–E3, S1–S7. Several fixes are
  **already in the code** (T1 actor re-resolve `tablews.go:185-198`; T2 prod fail-fast on
  missing Valkey via `start.sh` in `cdk/lib/api-stack.ts`; M6 rate limiters
  `router.go:39-41`). Others (stable buy-in idempotency H3/M7, escalation-from-config M2,
  equity off the hot path M5, SitOut version-retry M1) are **not yet** applied — verify
  against the current tree before relying on them.

## Other binaries

- `cmd/server` — the game server (described above).
- `cmd/archiver` — DynamoDB Stream → S3 archive Lambda (see B10).
- `cmd/handreplay` — offline hand-replay harness (engine test/debug).

## Cross-links

- Frontend that consumes these endpoints: [`../ui/README.md`](../ui/README.md)
- Infrastructure that deploys this: [`../cdk/README.md`](../cdk/README.md)
- Source-of-truth status & product spec: [`../README.md`](../README.md),
  [`../OVERVIEW.md`](../OVERVIEW.md), [`../ARCHITECTURE.md`](../ARCHITECTURE.md),
  [`../PLAN.md`](../PLAN.md)
