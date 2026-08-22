# ctech-poker — API (game server)

Go real-time poker game server. **Sandbox (play-money) mode is live end to end.** Real-money mode is also **implemented
and reachable** (`internal/buyin`, `internal/walletclient`, `cmd/reconcile`) under the Brazil-legal fixed-fee model:
`POST /v1.0/rooms/` accepts `currency_mode: "real"` plus a fixed
`entry_fee_cents`, and the request is rejected unless `REAL_MONEY_ENABLED` is on — a runtime gate that is **off by
default** and blocked on legal sign-off, not on missing code.

> Re-verified against the implementation on **2026-07-28**. All claims here are anchored to `api/`,
> not to `ARCHITECTURE.md`/`OVERVIEW.md`. `../docs/README.md` carries the feature-status index.

## Stack

- Go `1.26.6`, module `gopkg.aoctech.app/poker/api` (`go.mod:1`).
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
  via `docker-compose.test.yml` (in-memory local instance on `:8555`). `mustCreatePokerTables` seeds
  `<env>_poker_pending_cashouts` alongside the table-state/action tables — any manager exercising a system removal
  (AFK/disconnect kick) must call `SetSystemSettlementIntent`, since `handleKickTimeout`/`handleAFKSweep` fail closed
  without one (see `TestDisconnectKickRemovesSeatAcrossServers`).
- `ListUnresolved` (`internal/reconcile/pending.go`) queries the `gsi_status` GSI instead of scanning the whole table
  (added in the same change that made real-money buy-in fee debits durable) — any local table your test creates for
  `PendingStore` needs that GSI too, mirroring `cdk/lib/dynamodb-stack.ts`'s `pendingCashouts.addGlobalSecondaryIndex`
  (see `internal/reconcile/pending_test.go`'s `mustCreateTestTable`). The same change also made real-money `BuyIn`
  fail closed when `EntryFeeCents > 0` and no pending store is wired (`internal/buyin/service.go`) — every real-money
  test needs `.WithPendingStore(...)`.
- `handeval` keeps the normal CI fast with a deterministic 20,000-hand differential sample plus directed
  category/tiebreak tests. After changing
  `internal/engine/handeval/ref`, `hashq`, the generator, or `tables.bin`, run the full 133,784,560-hand proof
  explicitly with
  `go test -tags exhaustive -timeout 60m ./internal/engine/handeval`.
- Dockerfile: `golang:1.26-alpine` builder → `distroless/static-debian12`, `EXPOSE 8003`.
- Deploy: GitHub Actions `api.yml` builds the binary, uploads to the shared deployments S3 bucket, and does a rolling
  SSM deploy across the EC2 Auto-Scaling Group (see [`../cdk/README.md`](../cdk/README.md)).

## Configuration (environment variables)

All keys are read by `env.Parse` into `internal/config.Config` (`internal/config/config.go:10-52`)
unless noted otherwise. `*` = fails closed (server refuses to start) if unset/empty in
`ENVIRONMENT=prod`.

| Key                                               | Default                           | Purpose                                                                                                              |
|---------------------------------------------------|-----------------------------------|----------------------------------------------------------------------------------------------------------------------|
| `APP_VERSION`                                     | `0.0.1`                           | reported version; set by CI                                                                                          |
| `PORT`                                            | `8003`                            | HTTP listen port                                                                                                     |
| `ENVIRONMENT`                                     | `dev`                             | gates the prod-only fail-closed checks below                                                                         |
| `READ_TIMEOUT` / `WRITE_TIMEOUT` / `IDLE_TIMEOUT` | `10` / `10` / `60`                | HTTP server timeouts (seconds)                                                                                       |
| `TRUSTED_PROXIES`                                 | —                                 | comma-separated, Fiber proxy trust list                                                                              |
| `VALKEY_URL` *                                    | —                                 | cache / `ws.Registry` fan-out backend; empty in prod means cross-instance realtime silently breaks                   |
| `CTECH_URL` *                                     | `https://accounts.aoctech.app`    | ctech-account issuer base URL                                                                                        |
| `CTECH_JWKS_URL`                                  | derived from `CTECH_URL` if empty | JWKS endpoint                                                                                                        |
| `SERVICE_AUDIENCE` *                              | `https://poker.aoctech.app`       | expected JWT audience                                                                                                |
| `CORS_ALLOWED_ORIGINS`                            | —                                 | comma-separated                                                                                                      |
| `AWS_REGION`                                      | `us-east-1`                       | AWS SDK region                                                                                                       |
| `DYNAMODB_ENDPOINT`                               | —                                 | local-only override (DynamoDB Local); leave empty in prod                                                            |
| `WALLET_URL`                                      | `https://wallet.aoctech.app`      | ctech-wallet base URL                                                                                                |
| `POKER_CLIENT_ID` / `POKER_CLIENT_SECRET`         | —                                 | poker's M2M client credentials against ctech-wallet                                                                  |
| `AVATAR_BUCKET`                                   | —                                 | avatar bucket; empty disables avatar upload **and** the public read route                                            |
| `AVATAR_BASE_URL`                                 | —                                 | prefix of every avatar URL the API serialises; since the Cloudflare migration, this API's own `/v1.0/avatars`         |
| `REAL_MONEY_ENABLED`                              | `false`                           | Phase 5 gate; see below                                                                                              |
| `LEGAL_SIGNOFF_REF`                               | —                                 | required non-empty if `REAL_MONEY_ENABLED=true`, else `Load()` errors (business sign-off, not an engineering toggle) |

`cmd/server` uses `config.Load()` (all `*` checks apply). `cmd/tablecleanup`/`cmd/reconcile` and the Lambda entrypoints
use `config.LoadForLambda()`, which only enforces the `CTECH_URL`/`CTECH_JWKS_URL`
checks.

`REAL_MONEY_ENABLED` and `LEGAL_SIGNOFF_REF` **are** wired: `cdk/lib/api-stack.ts:251-254` fetches both from SSM
(`/ctech/<env>/poker/real-money-enabled`, `/ctech/<env>/poker/legal-signoff-ref`,
`cdk/lib/constants.ts:112-113`) in the instance `start.sh`, defaulting to `false` when the parameter is absent. Turning
real money on is an SSM parameter change plus an instance refresh — no userdata edit. (An earlier revision of this file
claimed they were unwired; that was wrong.)

Two keys this table also omitted: `TURNSTILE_SECRET` and `TURNSTILE_EXPECTED_HOSTNAME`, both set by
`cdk/lib/api-stack.ts` for `internal/botcheck`.

`AVATAR_BASE_URL` is resolved from SSM (`/ctech/<env>/poker/avatar-base-url`) in the instance `start.sh`, like the two
above. Its value is `https://poker-api[-<env>].aoctech.app/v1.0/avatars` — the API serves the bytes itself now, so
pointing it anywhere else (an old CloudFront path, a bucket URL) produces 404s or exposes the bucket.

Per-binary keys read outside `Config` (not in the struct above):

| Key                                                   | Binary                              | Purpose                                                       |
|-------------------------------------------------------|-------------------------------------|---------------------------------------------------------------|
| `ARCHIVE_BUCKET`                                      | `cmd/archiver`                      | S3 bucket for the DynamoDB Stream archive                     |
| `WALLET_URL_PARAM`                                    | `cmd/tablecleanup`, `cmd/reconcile` | **SSM parameter name** (not the value) holding the wallet URL |
| `POKER_CLIENT_ID_PARAM` / `POKER_CLIENT_SECRET_PARAM` | `cmd/tablecleanup`, `cmd/reconcile` | SSM parameter names for M2M creds                             |

## Real-time transport (WebSocket, binary protobuf)

**The wire format is protobuf, not JSON.** Schema: `../proto/poker.proto`, generated into
`internal/api/v1/proto` (Go) and `../ui/src/lib/api/proto/poker.ts` (ts-proto). Frames are sent as binary;
`ClientMessage` / `ServerMessage` are the two envelopes.

- **Two gateways.** `GET /v1.0/tables/:id/ws` is the table socket; `GET /v1.0/ws` is the lobby/user socket, which
  registers the `lobby` and `user#<playerID>` channels and accepts only `ping`.
- Upgraded by `fasthttp/websocket` `FastHTTPUpgrader`; origin check mirrors HTTP CORS.
- **Auth over the socket is the first frame**, not a header or query param: the client sends its token (plus
  `share_code` for a private room) immediately after upgrade (`readAuthToken`). A missing or invalid frame fails closed.
  `sub` and `sid` are required, M2M is rejected, and the token must have `azp=poker`; WebSocket/game commands belong to
  the first-party browser client and are never exposed through the public read-only scope catalog.
- ⚠️ **Fiber hijacks the connection**, so any string taken from the request context (`c.Params`, locals) must be
  **copied before** the WebSocket goroutine uses it — the underlying buffer is reused once the handler returns. This was
  the cause of a real "no state seeded" bug.
- **Private rooms are invite-only end-to-end**: the WS gate re-checks
  `privateRoomAccessAllowed(room, playerID, shareCode)` with a constant-time share-code compare, mirroring the HTTP join
  gate.
- Fan-out keys: `<tableID>#<viewerID>` (per-viewer, because each seat receives a differently masked snapshot), `lobby`,
  `user#<playerID>`. The registry is **Valkey-backed in prod**; the in-memory fallback is `dev` only and non-dev fails
  fast without Valkey.
- **Client → server**: `ping`, `sync_state`, `ready`, `act`, `preselect_action`, `bot_challenge`,
  `post_big_blind`, `show_cards`, `request_rabbit_hunt`, `request_winner_cards`, `keep_seat`, `chat`, `reaction`.
- **Server → client**: `connected`, `pong`, `state` (full authoritative snapshot on join and on every mutation — no
  delta replay), `chat`, `error`, `removed`, `achievement_unlocked`, `room_created`,
  `room_updated`, `payment_received`, `system_broadcast`, `social_event`, `social_presence_changed` and
  `social_inbox_count`. Social frames are emitted only on the authenticated `user#<playerID>` channel.
- Abuse control: per-seat fixed-window limiter (10 actions/sec/seat), **32 KiB frame cap**, chat truncated to
  `chatMessageMaxLength` (50 chars, mirrored client-side as `CHAT_MESSAGE_MAX_LENGTH`) and masked by
  `internal/chatfilter`, and an adaptive Turnstile challenge (`internal/botcheck`) issued over the socket.
- Heartbeat: 30s ping / 45s pong wait.

## Social graph rollout

`SOCIAL_GRAPH_ENABLED` defaults to `false` and gates friendship, presence and table invitations. It does not gate
the independent player-safety controls (mute, block and report). EC2 reads the switch from
`/ctech/<env>/poker/social-graph-enabled` on each application start. The friendship graph and exact friend-code lookup
are implemented over conditional, mirrored DynamoDB transactions. All `/social` routes require a first-party Poker
session (`azp=poker`, `sub` and `sid`); they are never available through delegated read scopes. Mutations require an
`Idempotency-Key` of at most 128 characters, accepted by the first-party CORS policy.

Friendship is capped at 200 friends and 50 pending outgoing requests. New requests are rate-limited to 30/day/player
and 100/day/IP; all social mutations also have 120/min/player and 240/min/IP limits. Block includes mute and removes
requests/friendship in both directions. Unblock deliberately preserves mute. The public API never exposes whether the
other player blocked the caller, and safety operations remain available when the friendship feature flag is off.

The durable in-app inbox stores friend requests, friendship acceptances and table invitations for 90 days. Unread
items use the sparse `gsi_unread` index; marking an item read removes it from that index without deleting history.
Each mutation also fans out a protobuf `social_event` or `social_inbox_count` frame on `user#<player_id>` so all open
first-party Poker sessions converge without push notifications, e-mail or direct messages.

Table invitations are friend-only, expire after 15 minutes and require the sender to have an open session at that
table. Acceptance never buys chips or reserves a seat. For private rooms it atomically changes the inbox event and
creates `poker_rooms(pk=<room>, sk=invite#<player>)`; GET, table WebSocket auth and join accept that unexpired grant
without returning the room's `share_code`. Room status and capacity are checked before the grant transaction and the
normal buy-in path revalidates capacity before any wallet debit.

Friend presence is ephemeral and fleet-shared in Valkey. Each player owns a sorted set of connection IDs renewed every
30 seconds and expired after 75 seconds; opening and closing sockets use atomic scripts so multiple tabs and API
replicas cannot produce a false offline transition. The latest open `poker_player_sessions` row reconciles `in_table`
on login, while buy-in and settlement update it immediately. Friend-facing protobuf frames contain only `player_id`
and `online`, `offline` or `in_table`—never room, table, stakes, balance or currency. The in-memory adapter is limited
to development and tests.

Completed hands materialize directed opponent pairs in `poker_recent_players` after the authoritative hand commit. A
conditional seven-day hand guard makes retries idempotent; pair rows expire after 90 days. Failures are logged and do
not change the committed hand. The first empty `GET /social/recent` lazily reads at most 100 rows from that caller's own
hand history (no global scan). Results are capped at 50, hydrate profiles with `BatchGetItem`, and omit a pair when
either direction contains a block.

Player reports are accepted only from first-party Poker sessions at `POST /social/reports`, require an
`Idempotency-Key`, and are limited to 10/hour/player and 50/hour/IP. Categories and surfaces are enumerated; optional
details are capped at 500 Unicode characters. For table chat and reactions the client supplies only table, hand and
action IDs: the server resolves the action inside that hand's DynamoDB partition, verifies the reported actor and
copies the already-sanitized message or catalog reaction ID. Neither free text nor copied evidence is returned by HTTP,
logged, or used as a metric dimension. The legacy avatar-report route writes the new moderation queue and retains its
existing profile reporter set during migration.

Open reports have no TTL. `cmd/moderation` is the credential-gated operator interface for `list`, `show`, `review` and
`resolve`; only `show` reveals details/evidence. Resolution adds a 180-day TTL and accepts only the runbook's four
resolution codes. See `../docs/runbooks/poker-social-moderation.md` for triage, escalation and least-access procedures.

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

Auth column: **JWT** means `authMiddleware` (bearer token, `sub` + `sid` required, M2M rejected). Scoped `GET` requests
require their exact public `poker:*:read` permission. HTTP mutations require `azp=poker`, so API-key and other OAuth
clients stay read-only even though the first-party SPA requests those same read scopes.

| Method & path                                | Auth            | Notes                                                                                      |
|----------------------------------------------|-----------------|--------------------------------------------------------------------------------------------|
| `GET /health`                                | none            | liveness                                                                                   |
| `GET /health-check`                          | none            | RFC-health detail (uptime, CPU, memory, `DescribeTable`); ALB target group accepts 200/207 |
| `GET /tables/:id/ws`                         | first-frame JWT | table WebSocket (above)                                                                    |
| `GET /ws`                                    | first-frame JWT | lobby/user WebSocket; registers `lobby` + `user#<id>`                                      |
| `POST /rooms/`                               | JWT             | create room; takes `currency_mode` + `entry_fee_cents`; rate-limited 10/min/IP             |
| `GET /rooms/`                                | JWT             | list public rooms (paginated, 50)                                                          |
| `GET /rooms/stakes`                          | JWT             | stake catalog; `?currency_mode=sandbox\|real`                                              |
| `GET /rooms/code/:code`                      | JWT             | lookup by share code                                                                       |
| `GET /rooms/:id`                             | JWT             | room detail (`share_code` stripped for non-creators)                                       |
| `GET /rooms/:id/seated`                      | JWT             | `{seated, stack}` — server-authoritative seat check                                        |
| `POST /rooms/:id/join`                       | JWT             | join + buy-in; optional `auto_rebuy` (sandbox only, see below); rate-limited 30/min/IP     |
| `POST /rooms/:id/leave`                      | JWT             | leave → `{amount}` cashed out                                                              |
| `POST /rooms/:id/ready`                      | JWT             | **501** — use the table WebSocket's `ready` message                                        |
| `GET /avatars/:userId/:version.jpg`          | **none**        | published avatar bytes, streamed from the bucket's `av/` prefix; 600/min/IP                |
| `GET /players/:playerId/showcase`            | **none**        | public profile showcase; 404 when `showcase_public` is false                               |
| `GET /players/me`                            | JWT             | profile + sandbox/real balances                                                            |
| `POST /players/me`                           | JWT             | update name, wallet mode, deck variant, showcase settings                                  |
| `POST /players/me/terms/accept`              | JWT             | accept the poker ToS addendum                                                              |
| `POST /players/me/avatar/upload-url`         | JWT             | presigned S3 POST; 5/hour/player                                                           |
| `POST /players/me/avatar/confirm`            | JWT             | validate quarantine object and publish avatar                                              |
| `DELETE /players/me/avatar`                  | JWT             | remove the current avatar                                                                  |
| `POST /players/:playerId/avatar/report`      | JWT             | record an avatar abuse report; 5/hour/player                                               |
| `GET /players/me/sessions`                   | JWT             | per-table session P&L, paginated (50)                                                      |
| `GET /players/me/hands`                      | JWT             | hand history, `?table_id`, paginated (50)                                                  |
| `GET /players/me/hand/:id?mode=...`          | JWT             | one hand incl. its fairness proof                                                          |
| `GET /players/me/achievements`               | JWT             | own progress, paginated (100)                                                              |
| `GET /players/me/notes/`                     | JWT             | private opponent notes; `poker:player-notes:read`                                          |
| `POST /players/me/notes/:opponentId`         | JWT             | save/delete a note (`{tag, note}`, ≤500 chars)                                             |
| `GET /players/me/poker-stats`                | JWT             | own VPIP/PFR/3-bet                                                                         |
| `POST /players/me/hand/:id/share`            | JWT             | create a public share link (`mode` in request body)                                        |
| `DELETE /players/me/hand-shares/:token`      | JWT             | revoke a share link                                                                        |
| `GET /hand-shares/:token`                    | **none**        | public shared hand, opponents aliased                                                      |
| `GET /tables/:tableId/hands/:handId/history` | JWT             | action-log replay for one hand                                                             |
| `GET /achievements`                          | **none**        | static achievement catalog                                                                 |
| `GET /leaderboard`                           | JWT             | `?metric=hands_won\|hands_played\|win_rate`, `?limit`, `?cursor`                           |
| `POST /sandbox-credits/`                     | JWT             | daily spin; rate-limited 60/min/IP                                                         |
| `GET /sandbox-credits/`                      | JWT             | `{remaining_time_seconds}` cooldown; scoped tokens require `poker:daily-reward:read`       |
| `GET /wallet/sandbox-purchase/...`           | JWT             | catalog/history/detail reads; `poker:sandbox-purchases:read`                               |
| `GET /wallet/reaction-purchase/...`          | JWT             | catalog/history/detail reads; `poker:reaction-purchases:read`                              |
| `GET /social/friends`                        | first-party JWT | paginated mutual friends                                                                   |
| `GET /social/friend-requests`                | first-party JWT | pending `?direction=incoming\|outgoing`; paginated                                         |
| `GET /social/blocked`                        | first-party JWT | players blocked by the caller; paginated                                                   |
| `GET /social/lookup/:friendCode`             | first-party JWT | exact `PKR-XXXX-XXXX-XXXX` lookup; no fuzzy/name search                                    |
| `GET /social/relationships/:playerId`        | first-party JWT | caller-visible relationship, mute and block state                                          |
| `GET /social/relationships?player_ids=`      | first-party JWT | batch of up to 25 ids for one seat list; edge state only, no profiles                       |
| `GET /social/summary`                        | first-party JWT | current unread inbox count                                                                 |
| `GET /social/inbox`                          | first-party JWT | durable in-app events, newest first; paginated                                             |
| `GET /social/recent`                         | first-party JWT | recent opponents for 90 days; blocked pairs omitted; paginated to 50                      |
| `POST /social/reports`                       | first-party JWT | idempotent player-safety report; authoritative evidence; returns `202`                    |
| `POST /social/friend-requests`               | first-party JWT | request by exactly one player ID or friend code                                            |
| `POST /social/friend-requests/:id/accept`    | first-party JWT | accept an incoming request                                                                 |
| `POST /social/friend-requests/:id/decline`   | first-party JWT | decline an incoming request                                                                |
| `DELETE /social/friend-requests/:id`         | first-party JWT | cancel an outgoing request                                                                 |
| `DELETE /social/friends/:id`                 | first-party JWT | remove mutual friendship                                                                   |
| `PUT\|DELETE /social/mutes/:id`              | first-party JWT | mute or unmute locally                                                                      |
| `PUT\|DELETE /social/blocks/:id`             | first-party JWT | block or unblock locally; unblock preserves mute                                           |
| `POST /social/inbox/read`                    | first-party JWT | mark up to 100 known event IDs read                                                        |
| `POST /social/table-invites`                 | first-party JWT | invite a friend from the sender's open table session                                       |
| `POST /social/table-invites/:id/accept`      | first-party JWT | accept an unexpired invite and create private-room access grant                            |
| `POST /social/table-invites/:id/decline`     | first-party JWT | decline an unexpired invite                                                                |

The batch relationship read is what the table surface uses: the client suppresses a muted or blocked player's chat and
reactions before they enter its state, and one request per seat list beats one request per seat. `GET /social/summary`
answers the unread badge only — the People drawer reads the friend, request, invite and recent lists directly instead of
duplicating them into a summary payload.

All social mutations require `Idempotency-Key` (maximum 128 characters). Table invites are limited to 20/minute per
sender and 5/minute per recipient. A second pending invite for the same sender, recipient and room is rejected.

`achievement_points` is **rejected** as a leaderboard metric — no `gsi_achievement_points` exists, and returning an
error beats silently ranking by a different GSI.

## Authentication & authorization

- `authMiddleware` (`internal/api/v1/auth.go`) verifies the bearer JWT against ctech-account's JWKS (`jwtverify`) and
  requires **both** a non-empty `sub` and a non-empty `sid`. An empty `sid` marks an M2M `client_credentials` token
  (ecosystem convention) and is rejected **403** — machine credentials can never act as a player. The WS gateway applies
  the same check on the first frame.
- `playerID` always comes from `claims.Sub`, never from a request body or path (IDOR safety).
- Four routes are intentionally public: the achievement catalog, a player's opt-in showcase, a shared-hand token, and
  avatar reads. The showcase 404s unless the player set `showcase_public`; the shared hand aliases opponents and
  carries a ≤30-day TTL. Avatars are covered below.
- Poker publishes public, read-only scopes through its resource-server manifest. Scoped `GET` requests are bound to
  their route family; for example, the daily-reward cooldown at `/sandbox-credits/` requires
  `poker:daily-reward:read`. Interactive mutations and WebSockets instead require a user session issued to the
  first-party `poker` client (`azp=poker`). KYC and business-role checks remain separate from OAuth scope checks.
- **`/social/*` is exempt from the read-scope table on purpose.** There is no grantable social scope (public or
  delegated); the whole group is gated by `firstPartyOnly`. Because `requiredReadScope` denies unknown paths, a
  first-party token that also carries any `poker:*` scope was getting
  `403 scope does not grant this poker resource` on every social `GET` — `enforceReadOnlyScope` now short-circuits for
  the `/v1.0/social` prefix and lets `firstPartyOnly` decide. Any new route family that should be readable with a
  public scope must be added to `requiredReadScope`; anything first-party-only belongs behind `firstPartyOnly`.
- **B9 (`sub`-only authz) is fixed.** Older revisions of this file described it as an open risk.
- `api-commons` v1.4.1's `jwtverify.Verify` now rejects any token missing a `token_use: "access"` claim (also tightens
  JWK `use`/`alg` matching). ctech-account's issued access tokens already carry it; only hand-crafted test JWTs
  (`internal/api/v1/auth_test.go`) needed updating after the bump.

### Avatars

One private bucket, two prefixes, and the difference between them is a security boundary:

- `up/` is the quarantine. `POST /players/me/avatar/upload-url` returns a presigned POST that the browser submits
  **directly to S3** (the only request the app makes to a host other than the API, which is why the bucket's dualstack
  origin is in the CSP's `connect-src`). Its contents are whatever the client sent: unvalidated bytes, expired by a
  1-day lifecycle rule.
- `av/` holds only what `avatar.Service.ValidateAndPublish` copied there after decoding the header, bounding the
  dimensions and rejecting EXIF, with `Content-Type` and `Cache-Control` rewritten on the copy.

`GET /avatars/:userId/:version.jpg` serves `av/` and nothing else. Callers never build a key: `avatar.PublishedKey` /
`avatar.UploadKey` own the prefix, and both refuse unless the user ID is a UUID (the shape ctech-account issues as
`sub`) and the version is positive — so a path segment from a URL cannot carry `/`, `%` or `..` into an S3 key, and the
quarantine is unaddressable. The response is the object's bytes with an immutable `Cache-Control` and S3's `ETag`,
**never** a redirect to a presigned URL: an avatar URL must not be tradeable for direct bucket access. Missing or
invalid is 404; a storage failure is 502, because a broken image and an outage are worth telling apart.

Reads are rate-limited separately from uploads — 600/min/IP against `avatarLimiter`'s 5/hour/player — because one table
view legitimately fetches nine images and the route is unauthenticated.

Until the Cloudflare frontend migration these bytes came from a CloudFront behaviour that rewrote `/avatars/*` onto the
bucket over OAC. There is no distribution in front of the app any more, so `AVATAR_BASE_URL`
(`/ctech/{env}/poker/avatar-base-url`) points at this route and the API is the only reader of the bucket.

## Sandbox & real-money ledgers

- `internal/walletclient` talks to `ctech-wallet`'s internal M2M routes: sandbox credit/debit
  (`/v1.0/internal/wallet/sandbox/credit|debit`) for play-money, plus a hold/release/cashout/activation contract
  (`/v1.0/internal/wallet/game/*`) for real money (`HoldGame`/`ReleaseHold`/`CashoutGame`/`IsGamblingActivated`,
  `client.go:130-262`). It authenticates with the poker M2M client using per-call scopes.
- `buyin` (`internal/buyin/service.go`) branches on `room.CurrencyMode`: `sandbox` uses plain credit/debit
  (`NewServiceWithPlayers`); `real` uses the hold-based `GameWallet` path (`NewServiceWithGame`, wired only when
  `REAL_MONEY_ENABLED=true`, `internal/app/app.go:198-203`). Any other value returns `ErrUnsupportedCurrencyMode`.
- Buy-in remains debit-then-seat so unpaid chips never enter a table. Cash-out commits the player removal, table
  snapshot/action log, and immutable `poker_pending_cashouts` settlement intent in the same DynamoDB transaction.
  Building or committing that transaction must succeed before Wallet is called; an error restores the actor's
  in-memory seat and stack. Production services are wired with the pending store and refuse cash-out if it is absent.
  Wallet settlement runs only after the durable table commit. A Wallet failure leaves the intent pending for
  `cmd/reconcile` (every 5 minutes); replay uses the settlement ID, and `Kind` separates a stuck cash-out from a stuck
  fee debit. AFK and disconnect removals use the same invariant: the manager asks `buyin.Service` to build the stable
  `system_leave` intent and the actor co-writes it with removal. If that builder or transaction fails, the system keeps
  the seat; the post-commit hook only performs the already-durable Wallet settlement.
- **Real money is reachable and gated at runtime.** `POST /v1.0/rooms/` accepts
  `currency_mode: "real"` with a fixed `entry_fee_cents` validated against the tier catalog, and returns 400 unless
  `REAL_MONEY_ENABLED` is on. `LEGAL_SIGNOFF_REF` must be non-empty when it is, checked fail-closed in `config.Load`.
- **Residual gap:** the real-money wiring in `app.go` doesn't pass a `players` service into
  `NewServiceWithGame`, so it skips the poker-terms-acceptance check sandbox buy-ins get. Fix before real money faces
  users.

### Auto rebuy (sandbox only)

A player can opt into `auto_rebuy` on `POST /rooms/:id/join` (fresh seats only — a rebuy of an existing seat ignores
it; `hand.Player.AutoRebuy`/`BuyInAmount` are set exactly once, at seat creation). After every completed hand,
`app.autoRebuySweep` (installed via `tablemanager.Manager.SetOnAutoRebuySweep`, wired in `app.wireAutoRebuyHook`)
checks each participant: if their seat busted (`Stack == 0`), has `AutoRebuy` set, and their sandbox wallet balance
covers the original `BuyInAmount`, it calls `buyin.Service.BuyIn` again with a `handID`-derived nonce. Insufficient
balance (including exactly zero) leaves the player sitting out for the client's manual/PIX rebuy flow instead.

The nonce is `handID + "-auto"` — deliberately *not* `handID + "-auto-" + playerID`. `buyin.Service.buyIn` builds the
wallet idempotency key as `roomID#playerID#buyin#nonce`, already prepending `playerID` once; repeating it in the nonce
pushed the compound key past ctech-wallet's `MovementOpRequest.IdempotencyKey` `max=128` validation for any real ULID
table/hand ID + UUID player ID (138 chars), so every sandbox debit came back HTTP 422 and auto-rebuy never actually
rebought anyone in production (fixed 2026-08-11, root-caused from a HAR capture plus prod wallet/poker logs — none of
`autoRebuySweep`'s own unit tests caught it, since their fake `BuyIn` never enforces the real wallet's field-length
rule; see `TestAutoRebuySweepNonceFitsWalletIdempotencyKeyLimit`).

The sweep runs in a **detached goroutine**, never inline: `onHandComplete`-style hooks fire synchronously on the
table actor's own single-goroutine command loop, and both the seat read (`buyin.Service.SeatedSummary`) and the
rebuy itself (`BuyIn`) dispatch back into that same loop — calling either synchronously from the hook deadlocks the
table. Real-money rooms are excluded entirely (`room.CurrencyMode != "sandbox"` short-circuits the sweep), because
the real-money buy-in path re-charges `EntryFeeCents` on every buy-in/rebuy — auto-rebuying there would silently
repeat that charge with no way to opt out per attempt.

## Known issues

- **No WAF at the edge.** `cdk/lib/frontend-stack.ts` builds the CloudFront distribution with no
  `webAclId`. Application-level protection is the per-IP HTTP limiters, the 32 KiB WS frame cap, and Turnstile.
  PLAN.md's Task 9 previously claimed this shipped; it did not.
- **No ASG lifecycle hook.** `tablemanager.DrainAndRelease` runs on the default EC2 shutdown grace period, not a
  guaranteed drain window — scale-in can cut a table mid-hand.
- **No DLQ on either EventBridge Scheduler target** (`cmd/reconcile`, `cmd/tablecleanup`).
- **Real-money buy-in skips the terms check** (above).
- **Two missing ctech-account scopes** block real-money verification calls — see `CLAUDE.md`. Both are config actions in
  ctech-account, not code changes here.

Fixed, for the record, since older revisions of this file listed them as open: **B9** (authz is now
`sub` + `sid`, M2M rejected, leaderboard authenticated), **B10** (archiver has an SQS DLQ and a depth alarm), **B31**
(`achievement_points` is rejected rather than mis-ranked), **B32** (commit-reveal is published and client-verifiable,
with the seed-less partial proof for no-showdown hands).

Fixed 2026-08-03: **`CommitAction`'s `extra` transact items (buyin's settlement/pending-cashout row) were
silently dropped from every `Leave` commit** — `extra` was only appended inside the `actionID != ""` branch, but
`LeaveCmd` never carries an `ActionID`, so a seat removal (manual cash-out or system kick/AFK) could commit with
no recovery row ever written if the follow-up wallet-credit call then failed. See
`docs/plans/2026-08-03-leave-settlement-atomicity.md`.

`docs/plans/2026-07-19-api-audit-remediation.md` remains a useful cross-check: some of its items (T1 actor re-resolve,
T2 prod fail-fast on missing Valkey, M6 rate limiters, stable buy-in idempotency)
are in code, others are not — verify against the tree before relying on any of them.

## Other binaries

- `cmd/server` — the game server (described above).
- `cmd/archiver` — DynamoDB Stream (`poker_action_log`) → S3 JSON Lines audit archive, grouped by partition. Failures go
  to an SQS DLQ with a depth alarm.
- `cmd/reconcile` — scheduled Lambda (every 5 min) sweeping `poker_pending_cashouts` past a 2-minute grace period;
  retries `CashoutGame`, sandbox `Credit`, or `DebitReal` depending on `Kind`.
- `cmd/tablecleanup` — scheduled Lambda (every 30 min) archiving tables idle >15 min via the
  `gsi_active_last_action` GSI, refunding seated players' sandbox chips and deleting the room.
- `cmd/handreplay` — offline CLI replaying a scripted hand through the pure engine; deterministic reconciliation and
  debugging tool.

## Cross-links

- Frontend that consumes these endpoints: [`../ui/README.md`](../ui/README.md)
- Infrastructure that deploys this: [`../cdk/README.md`](../cdk/README.md)
- Source-of-truth status & product spec: [`../README.md`](../README.md),
  [`../OVERVIEW.md`](../OVERVIEW.md), [`../ARCHITECTURE.md`](../ARCHITECTURE.md),
  [`../PLAN.md`](../PLAN.md)

### Real-money entry fees

For a real-money room with an entry fee, Poker commits an immutable fee-debit recovery intent in the same DynamoDB
transaction as seat admission. The wallet debit happens after that durable admission; if it fails, reconciliation
retries the durable intent rather than losing the obligation.

### Reconciliation index

Unresolved settlement records are indexed by sparse `gsi_status=open`; resolved records remove that key. Deploy the GSI
before this code and backfill pre-existing unresolved records with `gsi_status=open`.

### Real-money entry fees and reconciliation

Poker co-writes an immutable fee-debit recovery intent with real-money seat admission. Failed fee collection is retried
from that durable intent. Unresolved settlement records use sparse gsi_status=open; deploy the GSI and backfill existing
unresolved rows before enabling this release.
