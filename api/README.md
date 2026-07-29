# ctech-poker — API (game server)

Go real-time poker game server. **Sandbox (play-money) mode is live end to end.** Real-money mode is
also **implemented and reachable** (`internal/buyin`, `internal/walletclient`, `cmd/reconcile`) under
the Brazil-legal fixed-fee model: `POST /v1.0/rooms/` accepts `currency_mode: "real"` plus a fixed
`entry_fee_cents`, and the request is rejected unless `REAL_MONEY_ENABLED` is on — a runtime gate that
is **off by default** and blocked on legal sign-off, not on missing code.

> Re-verified against the implementation on **2026-07-28**. All claims here are anchored to `api/`,
> not to `ARCHITECTURE.md`/`OVERVIEW.md`. `../docs/README.md` carries the feature-status index.

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
- `handeval` keeps the normal CI fast with a deterministic 20,000-hand
  differential sample plus directed category/tiebreak tests. After changing
  `internal/engine/handeval/ref`, `hashq`, the generator, or `tables.bin`, run
  the full 133,784,560-hand proof explicitly with
  `go test -tags exhaustive -timeout 60m ./internal/engine/handeval`.
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

`REAL_MONEY_ENABLED` and `LEGAL_SIGNOFF_REF` **are** wired: `cdk/lib/api-stack.ts:251-254` fetches both
from SSM (`/ctech/<env>/poker/real-money-enabled`, `/ctech/<env>/poker/legal-signoff-ref`,
`cdk/lib/constants.ts:112-113`) in the instance `start.sh`, defaulting to `false` when the parameter is
absent. Turning real money on is an SSM parameter change plus an instance refresh — no userdata edit.
(An earlier revision of this file claimed they were unwired; that was wrong.)

Two keys this table also omitted: `TURNSTILE_SECRET` and `TURNSTILE_EXPECTED_HOSTNAME`, both set by
`cdk/lib/api-stack.ts` for `internal/botcheck`.

Per-binary keys read outside `Config` (not in the struct above):

| Key | Binary | Purpose |
|---|---|---|
| `ARCHIVE_BUCKET` | `cmd/archiver` | S3 bucket for the DynamoDB Stream archive |
| `WALLET_URL_PARAM` | `cmd/tablecleanup`, `cmd/reconcile` | **SSM parameter name** (not the value) holding the wallet URL |
| `POKER_CLIENT_ID_PARAM` / `POKER_CLIENT_SECRET_PARAM` | `cmd/tablecleanup`, `cmd/reconcile` | SSM parameter names for M2M creds |

## Real-time transport (WebSocket, binary protobuf)

**The wire format is protobuf, not JSON.** Schema: `../proto/poker.proto`, generated into
`internal/api/v1/proto` (Go) and `../ui/src/lib/api/proto/poker.ts` (ts-proto). Frames are sent as
binary; `ClientMessage` / `ServerMessage` are the two envelopes.

- **Two gateways.** `GET /v1.0/tables/:id/ws` is the table socket; `GET /v1.0/ws` is the lobby/user
  socket, which registers the `lobby` and `user#<playerID>` channels and accepts only `ping`.
- Upgraded by `fasthttp/websocket` `FastHTTPUpgrader`; origin check mirrors HTTP CORS.
- **Auth over the socket is the first frame**, not a header or query param: the client sends its token
  (plus `share_code` for a private room) immediately after upgrade (`readAuthToken`). A missing or
  invalid frame fails closed. Same claim rules as HTTP — `sub` and `sid` both required, M2M rejected.
- ⚠️ **Fiber hijacks the connection**, so any string taken from the request context (`c.Params`,
  locals) must be **copied before** the WebSocket goroutine uses it — the underlying buffer is reused
  once the handler returns. This was the cause of a real "no state seeded" bug.
- **Private rooms are invite-only end-to-end**: the WS gate re-checks
  `privateRoomAccessAllowed(room, playerID, shareCode)` with a constant-time share-code compare,
  mirroring the HTTP join gate.
- Fan-out keys: `<tableID>#<viewerID>` (per-viewer, because each seat receives a differently masked
  snapshot), `lobby`, `user#<playerID>`. The registry is **Valkey-backed in prod**; the in-memory
  fallback is `dev` only and non-dev fails fast without Valkey.
- **Client → server**: `ping`, `sync_state`, `ready`, `act`, `preselect_action`, `bot_challenge`,
  `post_big_blind`, `show_cards`, `keep_seat`, `chat`, `reaction`.
- **Server → client**: `connected`, `pong`, `state` (full authoritative snapshot on join and on every
  mutation — no delta replay), `chat`, `error`, `removed`, `achievement_unlocked`, `room_created`,
  `room_updated`, `payment_received`, `system_broadcast`.
- Abuse control: per-seat fixed-window limiter (10 actions/sec/seat), **32 KiB frame cap**, chat
  truncated to 500 chars and masked by `internal/chatfilter`, and an adaptive Turnstile challenge
  (`internal/botcheck`) issued over the socket.
- Heartbeat: 30s ping / 45s pong wait.

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
- Engine (pure logic, no networking): `internal/engine/{hand,betting,sidepots,equity,deck,handeval}`.
  `sidepots.ComputeSidePots`, 7-card evaluator, and HMAC-SHA256 Fisher–Yates shuffle with rejection sampling are present
  and unit-tested. A scripted **hand-replay harness** lives at `cmd/handreplay` (`script.example.json`,
  `script.fold.json`) — the Phase-1 deliverable.

## HTTP endpoints (`/v1.0`)

Auth column: **JWT** means `authMiddleware` (bearer token, `sub` + `sid` required, M2M rejected).

| Method & path | Auth | Notes |
|---|---|---|
| `GET /health` | none | liveness |
| `GET /health-check` | none | RFC-health detail (uptime, CPU, memory, `DescribeTable`); ALB target group accepts 200/207 |
| `GET /tables/:id/ws` | first-frame JWT | table WebSocket (above) |
| `GET /ws` | first-frame JWT | lobby/user WebSocket; registers `lobby` + `user#<id>` |
| `POST /rooms/` | JWT | create room; takes `currency_mode` + `entry_fee_cents`; rate-limited 10/min/IP |
| `GET /rooms/` | JWT | list public rooms (paginated, 50) |
| `GET /rooms/stakes` | JWT | stake catalog; `?currency_mode=sandbox\|real` |
| `GET /rooms/code/:code` | JWT | lookup by share code |
| `GET /rooms/:id` | JWT | room detail (`share_code` stripped for non-creators) |
| `GET /rooms/:id/seated` | JWT | `{seated, stack}` — server-authoritative seat check |
| `POST /rooms/:id/join` | JWT | join + buy-in; rate-limited 30/min/IP |
| `POST /rooms/:id/leave` | JWT | leave → `{amount}` cashed out |
| `POST /rooms/:id/ready` | JWT | **501** — use the table WebSocket's `ready` message |
| `GET /players/:playerId/showcase` | **none** | public profile showcase; 404 when `showcase_public` is false |
| `GET /players/me` | JWT | profile + sandbox/real balances |
| `POST /players/me` | JWT | update name, wallet mode, deck variant, showcase settings |
| `POST /players/me/terms/accept` | JWT | accept the poker ToS addendum |
| `POST /players/me/avatar/upload-url` | JWT | presigned S3 POST; 5/hour/player |
| `POST /players/me/avatar/confirm` | JWT | validate quarantine object and publish avatar |
| `DELETE /players/me/avatar` | JWT | remove the current avatar |
| `POST /players/:playerId/avatar/report` | JWT | record an avatar abuse report; 5/hour/player |
| `GET /players/me/sessions` | JWT | per-table session P&L, paginated (50) |
| `GET /players/me/hands` | JWT | hand history, `?table_id`, paginated (50) |
| `GET /players/me/hands/:handId` | JWT | one hand incl. its fairness proof |
| `GET /players/me/achievements` | JWT | own progress, paginated (100) |
| `GET /players/me/notes/` | JWT | private opponent notes |
| `POST /players/me/notes/:opponentId` | JWT | save/delete a note (`{tag, note}`, ≤500 chars) |
| `GET /players/me/poker-stats` | JWT | own VPIP/PFR/3-bet |
| `POST /players/me/hands/:handId/share` | JWT | create a public share link |
| `DELETE /players/me/hand-shares/:token` | JWT | revoke a share link |
| `GET /hand-shares/:token` | **none** | public shared hand, opponents aliased |
| `GET /tables/:tableId/hands/:handId/history` | JWT | action-log replay for one hand |
| `GET /achievements` | **none** | static achievement catalog |
| `GET /leaderboard` | JWT | `?metric=hands_won\|hands_played\|win_rate`, `?limit`, `?cursor` |
| `POST /sandbox-credits/` | JWT | daily spin; rate-limited 60/min/IP |
| `GET /sandbox-credits/` | JWT | `{remaining_time_seconds}` cooldown |

`achievement_points` is **rejected** as a leaderboard metric — no `gsi_achievement_points` exists, and
returning an error beats silently ranking by a different GSI.

## Authentication & authorization

- `authMiddleware` (`internal/api/v1/auth.go`) verifies the bearer JWT against ctech-account's JWKS
  (`jwtverify`) and requires **both** a non-empty `sub` and a non-empty `sid`. An empty `sid` marks an
  M2M `client_credentials` token (ecosystem convention) and is rejected **403** — machine credentials
  can never act as a player. The WS gateway applies the same check on the first frame.
- `playerID` always comes from `claims.Sub`, never from a request body or path (IDOR safety).
- Three routes are intentionally public: the achievement catalog, a player's opt-in showcase, and a
  shared-hand token. The showcase 404s unless the player set `showcase_public`; the shared hand aliases
  opponents and carries a ≤30-day TTL.
- There is still **no scope / KYC / role check** on the player surface, because none is defined for
  poker in ctech-account's scope catalog. Revisit if scopes are added — see `CLAUDE.md`'s blocker list,
  which includes two missing wallet scopes that still gate real-money verification calls.
- **B9 (`sub`-only authz) is fixed.** Older revisions of this file described it as an open risk.

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
- **Ordering is deliberate and asymmetric**: debit-then-seat on buy-in (never hand out chips nobody paid
  for), remove-then-credit on cash-out (never pay out a stack still in play). Anything that can fail
  *after* chips moved is written to `poker_pending_cashouts` and retried by `cmd/reconcile` every 5
  minutes; `Kind` separates a stuck cash-out from a stuck fee debit.
- **Real money is reachable and gated at runtime.** `POST /v1.0/rooms/` accepts
  `currency_mode: "real"` with a fixed `entry_fee_cents` validated against the tier catalog, and returns
  400 unless `REAL_MONEY_ENABLED` is on. `LEGAL_SIGNOFF_REF` must be non-empty when it is, checked
  fail-closed in `config.Load`.
- **Residual gap:** the real-money wiring in `app.go` doesn't pass a `players` service into
  `NewServiceWithGame`, so it skips the poker-terms-acceptance check sandbox buy-ins get. Fix before
  real money faces users.

## Known issues

- **No WAF at the edge.** `cdk/lib/frontend-stack.ts` builds the CloudFront distribution with no
  `webAclId`. Application-level protection is the per-IP HTTP limiters, the 32 KiB WS frame cap, and
  Turnstile. PLAN.md's Task 9 previously claimed this shipped; it did not.
- **No ASG lifecycle hook.** `tablemanager.DrainAndRelease` runs on the default EC2 shutdown grace
  period, not a guaranteed drain window — scale-in can cut a table mid-hand.
- **No DLQ on either EventBridge Scheduler target** (`cmd/reconcile`, `cmd/tablecleanup`).
- **Real-money buy-in skips the terms check** (above).
- **Two missing ctech-account scopes** block real-money verification calls — see `CLAUDE.md`. Both are
  config actions in ctech-account, not code changes here.

Fixed, for the record, since older revisions of this file listed them as open: **B9** (authz is now
`sub` + `sid`, M2M rejected, leaderboard authenticated), **B10** (archiver has an SQS DLQ and a
depth alarm), **B31** (`achievement_points` is rejected rather than mis-ranked), **B32** (commit-reveal
is published and client-verifiable, with the seed-less partial proof for no-showdown hands).

`docs/plans/2026-07-19-api-audit-remediation.md` remains a useful cross-check: some of its items
(T1 actor re-resolve, T2 prod fail-fast on missing Valkey, M6 rate limiters, stable buy-in idempotency)
are in code, others are not — verify against the tree before relying on any of them.

## Other binaries

- `cmd/server` — the game server (described above).
- `cmd/archiver` — DynamoDB Stream (`poker_action_log`) → S3 JSON Lines audit archive, grouped by
  partition. Failures go to an SQS DLQ with a depth alarm.
- `cmd/reconcile` — scheduled Lambda (every 5 min) sweeping `poker_pending_cashouts` past a 2-minute
  grace period; retries `CashoutGame`, sandbox `Credit`, or `DebitReal` depending on `Kind`.
- `cmd/tablecleanup` — scheduled Lambda (every 30 min) archiving tables idle >15 min via the
  `gsi_active_last_action` GSI, refunding seated players' sandbox chips and deleting the room.
- `cmd/handreplay` — offline CLI replaying a scripted hand through the pure engine; deterministic
  reconciliation and debugging tool.

## Cross-links

- Frontend that consumes these endpoints: [`../ui/README.md`](../ui/README.md)
- Infrastructure that deploys this: [`../cdk/README.md`](../cdk/README.md)
- Source-of-truth status & product spec: [`../README.md`](../README.md),
  [`../OVERVIEW.md`](../OVERVIEW.md), [`../ARCHITECTURE.md`](../ARCHITECTURE.md),
  [`../PLAN.md`](../PLAN.md)
