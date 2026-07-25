# ctech-poker — API (game server)

Go real-time poker game server. **Sandbox (play-money) mode is implemented end-to-end.**
Real-money mode (Phase 5) has its buy-in/cash-out/reconciliation logic implemented (`internal/buyin`,
`internal/walletclient`, `cmd/reconcile`), gated behind `REAL_MONEY_ENABLED=true` + `LEGAL_SIGNOFF_REF`
(see Configuration below) — **but is unreachable in production**: `POST /rooms` hardcodes
`CurrencyMode: "sandbox"` with no way to request `real`, so the gate never actually opens. See
[`../docs/plans/2026-07-19-poker-phase5-realmoney-and-hardening.md`](../docs/plans/2026-07-19-poker-phase5-realmoney-and-hardening.md)'s
Status section for the verified task-by-task state.

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
- Deploy: GitHub Actions `api.yml` builds the binary, uploads to the shared deployments S3 bucket, and does a rolling
  SSM deploy across the EC2 Auto-Scaling Group (see [`../cdk/README.md`](../cdk/README.md)).

## Configuration (environment variables)

All keys are read by `env.Parse` into `internal/config.Config` (`internal/config/config.go:10-52`)
unless noted otherwise. `*` = fails closed (server refuses to start) if unset/empty in
`ENVIRONMENT=prod`.

| Key | Default | Purpose |
|---|---|---|
| `APP_VERSION` | `0.0.1` | reported version; set by CI |
| `PORT` | `8003` | HTTP listen port |
| `ENVIRONMENT` | `dev` | gates the prod-only fail-closed checks below |
| `READ_TIMEOUT` / `WRITE_TIMEOUT` / `IDLE_TIMEOUT` | `10` / `10` / `60` | HTTP server timeouts (seconds) |
| `TRUSTED_PROXIES` | — | comma-separated, Fiber proxy trust list |
| `VALKEY_URL` * | — | cache / `ws.Registry` fan-out backend; empty in prod means cross-instance realtime silently breaks |
| `CTECH_URL` * | `https://accounts.aoctech.app` | ctech-account issuer base URL |
| `CTECH_JWKS_URL` | derived from `CTECH_URL` if empty | JWKS endpoint |
| `SERVICE_AUDIENCE` * | `https://poker.aoctech.app` | expected JWT audience |
| `CORS_ALLOWED_ORIGINS` | — | comma-separated |
| `AWS_REGION` | `us-east-1` | AWS SDK region |
| `DYNAMODB_ENDPOINT` | — | local-only override (DynamoDB Local); leave empty in prod |
| `WALLET_URL` | `https://wallet.aoctech.app` | ctech-wallet base URL |
| `POKER_CLIENT_ID` / `POKER_CLIENT_SECRET` | — | poker's M2M client credentials against ctech-wallet |
| `REAL_MONEY_ENABLED` | `false` | Phase 5 gate; see below |
| `LEGAL_SIGNOFF_REF` | — | required non-empty if `REAL_MONEY_ENABLED=true`, else `Load()` errors (business sign-off, not an engineering toggle) |

`cmd/server` uses `config.Load()` (all `*` checks apply). `cmd/tablecleanup`/`cmd/reconcile` and the
Lambda entrypoints use `config.LoadForLambda()`, which only enforces the `CTECH_URL`/`CTECH_JWKS_URL`
checks.

**⚠️ `REAL_MONEY_ENABLED` and `LEGAL_SIGNOFF_REF` are not wired in any `cdk/` stack.** Every other key
above is set by `cdk/lib/api-stack.ts`'s instance userdata; these two are not — turning on real-money
mode in prod today requires editing the ASG launch template/userdata by hand, outside CDK.

Per-binary keys read outside `Config` (not in the struct above):

| Key | Binary | Purpose |
|---|---|---|
| `ARCHIVE_BUCKET` | `cmd/archiver` | S3 bucket for the DynamoDB Stream archive |
| `WALLET_URL_PARAM` | `cmd/tablecleanup`, `cmd/reconcile` | **SSM parameter name** (not the value) holding the wallet URL |
| `POKER_CLIENT_ID_PARAM` / `POKER_CLIENT_SECRET_PARAM` | `cmd/tablecleanup`, `cmd/reconcile` | SSM parameter names for M2M creds |

## Real-time transport (WebSocket)

- Endpoint: `GET /v1.0/tables/:id/ws` (`internal/api/v1/tablews.go:133`).
- Upgraded by `fasthttp/websocket` `FastHTTPUpgrader`; origin check mirrors HTTP CORS (`wsAllowedOrigin`,
  `tablews.go:75`).
- **Auth over the socket is a first frame**, not a header: the client sends
  `{"token":"…","share_code":"…"}` (or a raw bearer token) immediately after upgrade (`readAuthToken`, `tablews.go:49`).
  A missing/invalid frame fails closed (`tablews.go:143-153`).
- **Private rooms are invite-only end-to-end**: the WS gate re-checks
  `privateRoomAccessAllowed(room, playerID, shareCode)` with a constant-time share-code compare (`tablews.go:166`),
  mirroring the HTTP join gate.
- Fan-out: the actor broadcasts via `ws.Registry` (`reg.Broadcast`, `tablews.go:273`). The registry is
  **Redis/Valkey-backed in prod**; an in-memory fallback exists for `dev`
  only (absence of Valkey in non-dev must fail fast — remediation T2).
- **Client → server** message types (`tablews.go:33-40`, handler `:247-274`):
  `ping`, `ready{ready}`, `act{action,amount,action_id}`, `post_big_blind`, `chat{message}`.
- **Server → client** message types: `connected{conn_id}`, `state{snapshot}`
  (full authoritative snapshot pushed on join and on every mutation — no replay-based resync), `pong`,
  `chat{player_id,message}`, `error{code}` (`unauthorized` /
  `unavailable` / `forbidden` / `rate_limited` / `invalid_action` / `invalid_post` /
  `message_too_long`), and `achievement_unlocked{key,stars}` delivered through the same broadcast channel from the
  actor.
- Abuse control: per-seat fixed-window limiter, **10 actions/sec/seat** (`seatLimiter`,
  `tablews.go:225`); chat is truncated to 500 chars and run through a trivial 2-word filter (`tableChatFilter`,
  `tablews.go:42` — see Known Issues).
- Heartbeat: 30s ping / pong-wait 45s (`tablews.go:26-29`).

## Game-server model (per-table actor + DynamoDB conditional writes)

This matches the **revised** model in `ARCHITECTURE.md §2` — *not* a Redis-lease authority.

- Each table is served by a **per-table actor** (`internal/table/actor.go`, `Run` loop, command channel `cmds`). Any
  instance may create/serve any table's actor via
  `tablemanager.Manager.GetOrCreateActor` (`internal/tablemanager/manager.go`) — there is no owner/proxy.
- **Correctness rests on DynamoDB conditional writes**, not on-process state: every player action commits via
  `tablestore.CommitAction` with a `version` equality
  `ConditionExpression` plus a per-`(table_id, hand_id, seat, action_id)` idempotency guard (`poker_action_guards`
  table). A version conflict is retried exactly once by the actor (the `retryOnConflict` pattern).
- **`tablelease` is latency-only.** It is a Valkey affinity hint for read-through caching; if the lease is lost the
  actor still re-reads DynamoDB directly. The socket handler re-resolves a live actor on `table.ErrActorStopped`
  (`tablews.go:185-198`) so a lease-killed actor cannot hang a request (remediation T1 is already in code).
- **Crash recovery is trivial**: state is durable after every single action, so a crashed instance loses at most the
  in-flight request; the next action (any instance) re-reads DynamoDB and proceeds.
- Engine (pure logic, no networking): `internal/engine/{hand,betting,sidepots,equity,deck}`.
  `sidepots.ComputeSidePots`, 7-card evaluator, and HMAC-SHA256 Fisher–Yates shuffle with rejection sampling are present
  and unit-tested. A scripted **hand-replay harness** lives at `cmd/handreplay` (`script.example.json`,
  `script.fold.json`) — the Phase-1 deliverable.

## HTTP endpoints (`/v1.0`)

| Method & path                   | Auth            | Notes                                                                       |
|---------------------------------|-----------------|-----------------------------------------------------------------------------|
| `GET /health`                   | none            | liveness (`health.go:106`)                                                  |
| `GET /health-check`             | none            | detailed dep report; ALB target group (accepts 200/207) (`health.go:110`)   |
| `GET /tables/:id/ws`            | first-frame JWT | WebSocket (above) (`tablews.go:133`)                                        |
| `POST /rooms/`                  | JWT             | create room; rate-limited (10/min/IP) (`rooms.go:29`)                       |
| `GET /rooms/`                   | JWT             | list public rooms (`rooms.go:30`)                                           |
| `GET /rooms/stakes`             | JWT             | curated stake list (`rooms.go:31`)                                          |
| `GET /rooms/code/:code`         | JWT             | lookup by share code (`rooms.go:32`)                                        |
| `GET /rooms/:id`                | JWT             | room detail (sanitized for non-creators) (`rooms.go:33`)                    |
| `POST /rooms/:id/join`          | JWT             | join + buy-in; rate-limited (30/min/IP) (`rooms.go:34`)                     |
| `POST /rooms/:id/leave`         | JWT             | leave (`rooms.go:35`)                                                       |
| `POST /rooms/:id/ready`         | JWT             | ready toggle (`rooms.go:36`)                                                |
| `GET /players/me`               | JWT             | player profile + terms state (`player.go:14`)                               |
| `POST /players/me/terms/accept` | JWT             | accept poker ToS addendum (`player.go:15`)                                  |
| `GET /leaderboard`              | **none**        | see Known Issues B9 (`leaderboard.go:11`)                                   |
| `POST /sandbox-credits`         | JWT             | daily sandbox-chip spin; rate-limited (60/min/IP) (`sandbox credits.go:10`) |

Auth group wiring: `RegisterRooms/Players/sandbox credits` all receive `auth` (`router.go:43-46`);
`RegisterLeaderboard` is registered **without** `auth` (`router.go:45`) — intentional per the audit but see B9.

## Authentication & authorization — ⚠️ B9 known risk

- `authMiddleware` (`internal/api/v1/auth.go:13-25`) verifies the bearer JWT and sets
  `c.Locals("user_id", claims.Sub)`. The **only** authorization check is
  `claims.Sub == ""` → reject (`auth.go:20`). There is **no scope, KYC, or role check**.
- The WebSocket gate derives `playerID` from `claims.Sub` (not the client body), so a player cannot act for another —
  good. But the *same* `sub`-only guard is the entire authz surface for every player/room endpoint.
- **`GET /leaderboard` is unauthenticated** (`leaderboard.go:11`, `router.go:45`). Public read-only leaderboard is a
  deliberate product choice, but it means the endpoint performs no auth at all.
- **Machine (M2M) credentials are not distinguished from user credentials.** The server itself uses an M2M client
  (`SSM_POKER` client-id/secret, `cdk/lib/constants.ts:103-107`)
  to call `ctech-wallet`. A token that carries any non-empty `sub` (including an M2M client credential with no
  session/sid) satisfies the `sub`-only guard and could call player/room endpoints. **Hypothesis to confirm against the
  token issuer (ctech-account):**
  M2M tokens populate `sub` with the client id and lack a user `sid`, so they pass today. Not yet exploitable for real
  funds (sandbox-only), but must be fixed before real-money.
- **Tracked as a known risk to fix, not accepted.** See [`api/CLAUDE.md`](./CLAUDE.md).

## Sandbox & real-money ledgers

- `internal/walletclient` talks to `ctech-wallet`'s internal M2M routes: sandbox
  credit/debit (`/v1.0/internal/wallet/sandbox/credit|debit`) for play-money, plus a
  hold/release/cashout/activation contract (`/v1.0/internal/wallet/game/*`) for real money
  (`HoldGame`/`ReleaseHold`/`CashoutGame`/`IsGamblingActivated`, `client.go:130-262`). It authenticates
  with the poker M2M client using per-call scopes.
- `buyin` (`internal/buyin/service.go`) branches on `room.CurrencyMode`: `sandbox` uses plain
  credit/debit (`NewServiceWithPlayers`); `real` uses the hold-based `GameWallet` path
  (`NewServiceWithGame`, wired only when `REAL_MONEY_ENABLED=true`, `internal/app/app.go:198-203`).
  Any other value returns `ErrUnsupportedCurrencyMode`.
- **The real-money path is implemented but currently unreachable**: `POST /rooms` always creates
  `CurrencyMode: "sandbox"` rooms (`internal/api/v1/rooms.go:93`) — there is no request field to ask for
  a real-money room, so the `real` branch above never executes outside tests. Also, the real-money wiring
  in `app.go` doesn't pass a `players` service into `NewServiceWithGame`, so it skips the
  terms-of-service acceptance check sandbox buy-ins get (`buyin/service.go:140-144`) — fix before exposing
  real-money room creation. Full status: see the Phase 5 plan's Status section referenced above.

## Known issues (documented honestly — do NOT fix code here)

- **B9 — authz is `sub`-only** (above). `auth.go:20`; leaderboard unauthenticated
  `leaderboard.go:11`; M2M not distinguished.
- **B10 — archiver Lambda has no DLQ** (`cdk/lib/archiver-stack.ts:71-75`):
  `DynamoEventSource` sets `retryAttempts: 3` but **no `onFailure` / dead-letter queue**. A poison record that fails 3×
  is dropped. Archiver Lambda: `cmd/archiver/main.go`
  (30s timeout, default memory).
- **B31 — `leaderboard.Top("achievement_points")` returns the wrong ranking**
  (`internal/leaderboard/store.go:105-133`): for `achievement_points` the code sets only
  `queryLimit=1000` but still queries the **`gsi_hands_won`** index (the function default at
  `store.go:106`). The `poker_leaderboard_stats` table has GSIs only for `hands_won`,
  `hands_played`, and `win_rate` (`cdk/lib/dynamodb-stack.ts:78-95`) — **there is no
  `achievement_points` GSI** — so the call silently returns a *hands-won* ranking, not an achievement-points ranking.
- **B32 fixed** — commit-reveal fairness is verifiable by clients via the WS snapshot
  (`internal/engine/hand/snapshot.go:160-165`): `ShuffleCommitHash` is published in every
  snapshot as soon as `StartHand` sets `t.shuffle` (before any hole card is dealt), and
  `ShuffleServerSeedHex` is added once `Stage == Complete` (the reveal). A player can hash
  their own copy of `ServerSeed` client-side and compare it against the `CommitHash` they
  received before the hand started. Persisted long-term too, hex-encoded, on
  `hand.HandOutcome`/`sessionlog.HandItem` (`ServerSeed`/`CommitHash`). Covered by
  `snapshot_test.go:245-265`.
- **Remediation context:** `docs/plans/2026-07-19-api-audit-remediation.md` (and its spec)
  is a separate audit covering H1–H4, M1–M7, L1–L6, E1–E3, S1–S7. Several fixes are **already in the code** (T1 actor
  re-resolve `tablews.go:185-198`; T2 prod fail-fast on missing Valkey via `start.sh` in `cdk/lib/api-stack.ts`; M6 rate
  limiters
  `router.go:39-41`). Others (stable buy-in idempotency H3/M7, escalation-from-config M2, equity off the hot path M5,
  SitOut version-retry M1) are **not yet** applied — verify against the current tree before relying on them.

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
