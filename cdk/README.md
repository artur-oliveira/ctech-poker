# ctech-poker — CDK (infrastructure)

AWS CDK (TypeScript) for the poker service. **All stacks are implemented and live.**
Deploys in the order **CDK → API → Frontend** via `.github/workflows/deploy.yml`. Every
claim below is anchored to `cdk/lib/`.

## Stacks (`cdk/lib/`)

| Stack                | File                | What it provisions                                      |
|----------------------|---------------------|---------------------------------------------------------|
| `PokerDynamoDBStack` | `dynamodb-stack.ts` | 8 DynamoDB tables + GSIs                                |
| `PokerApiStack`      | `api-stack.ts`      | EC2 ASG game server + ALB wiring + IAM                  |
| `PokerArchiverStack` | `archiver-stack.ts` | Action-log archive Lambda (DynamoDB Streams → S3)       |
| `PokerFrontendStack` | `frontend-stack.ts` | S3 + CloudFront static host + route KeyValueStore       |
| `PokerOidcStack`     | `oidc-stack.ts`     | OIDC / auth integration                                 |
| (bin)                | `bin/poker.ts`      | App entrypoint; `cdk.json` → `npx ts-node bin/poker.ts` |

Shared constants (no magic strings): `lib/constants.ts`. Account `868899309401`,
region `us-east-1` (`constants.ts:13-14`).

## Compute — game server is an **EC2 Auto-Scaling Group** (not Lambda/Fargate)

- `api-stack.ts` uses `@aoctech/cdk`'s `PrivateIpv4Ec2Service` (`api-stack.ts:317`) — the
  shared no-NAT-gateway EC2/ASG pattern used across CTech. **Confirmed: this is a stateful
  game server on EC2, matching `ARCHITECTURE.md §1`, NOT a Lambda fleet.**
- Capacity: `minCapacity: 1`, `maxCapacity: isProd ? 3 : 1` (`api-stack.ts:334-335`).
- The `app` binary runs via systemd (`app.service` written in user data, `api-stack.ts:206-228`),
  behind the shared ctech-cdk ALB at **listener priority 45** (`constants.ts:42`, `api-stack.ts:337`),
  port **8003** (`APP_PORT`, `constants.ts:50`), health check `/v1.0/health-check`
  (`constants.ts:57`). **No nginx** in front (the Go binary is the ALB target directly).
- Continuous deployment: GitHub Actions `api.yml` builds `dist/app` (linux/arm64), uploads to
  the shared deployments S3 bucket, and rolls via SSM `RunCommand` calling `/opt/app/deploy.sh`.
- **Valkey is mandatory in prod**: `start.sh` fetches `VALKEY_URL` from SSM; if it is empty in
  prod, `config.Load()` fails closed (in-memory fallback is dev/stage only) — this is the
  remediation T2 fail-fast behavior.

## WebSocket

- Served by the **same Go binary** on the EC2 ASG (`GET /v1.0/tables/:id/ws`), fronted by the
  ALB (ALB supports WS upgrade). It is **not** an API Gateway WebSocket API.
- Fan-out uses the Redis/Valkey-backed `ws.Registry` from `api-commons` (see
  [`../api/README.md`](../api/README.md#real-time-transport-websocket)).

## DynamoDB (`dynamodb-stack.ts`)

On-demand billing with `maxReadRequestUnits/maxWriteRequestUnits: 1000`
(`dynamodb-stack.ts:38`) — note this is **1000**, far above the `ctech-wallet` 5 WCU cap
flagged in `OVERVIEW.md §5` (a separate service). PITR enabled in prod; encryption AWS-managed.

| Table (prefixed `<env>_`)    | Sort key | TTL   | Stream        | Notes                                                    |
|------------------------------|----------|-------|---------------|----------------------------------------------------------|
| `poker_table_state`          | no       | –     | –             | single authoritative item per table, `version`ed         |
| `poker_action_log`           | yes      | 90d   | **NEW_IMAGE** | feeds the archiver Lambda                                |
| `poker_action_guards`        | no       | 7d    | –             | idempotency guard per action                             |
| `poker_rooms`                | yes      | –     | –             | GSIs `gsi_public`, `gsi_share_code`                      |
| `poker_player_profiles`      | no       | –     | –             | poker-local shadow of ctech-account user                 |
| `poker_achievement_progress` | yes      | –     | –             |                                                          |
| `poker_leaderboard_stats`    | yes      | –     | –             | GSIs `gsi_hands_won`, `gsi_hands_played`, `gsi_win_rate` |
| `poker_daily_reward`         | yes      | daily | –             | one item per player/day (24h cooldown)                   |

> **B31 relevance:** `poker_leaderboard_stats` has **no `achievement_points` GSI** — only
> hands-won / hands-played / win-rate. The API's `Top("achievement_points")` therefore
> silently falls through to `gsi_hands_won` (see [
`../api/README.md`](../api/README.md#known-issues-documented-honestly--do-not-fix-code-here)).

## IAM

`PokerApiStack` (`api-stack.ts:71-108`):

- Instance role with `AmazonSSMManagedInstanceCore` + `CloudWatchAgentServerPolicy`.
- DynamoDB: `GetItem/PutItem/UpdateItem/Query/TransactWriteItems/DescribeTable` on the 8
  tables **and** their indexes (`api-stack.ts:88-94`).
- SSM `GetParameter` for `valkeyUrl`, `walletUrl`, `pokerClientId`, `pokerClientSecret`
  (`api-stack.ts:95-100`).
- S3 `GetObject` on the deployments bucket, `PutObject` on the logs bucket.
- No real-money wallet (hold/capture) permissions — consistent with sandbox-only mode.

## Archiver Lambda — ⚠️ B10 known risk

- `archiver-stack.ts`: `ArchiverFn` (PROVIDED_AL2023, arm64, 30s timeout) subscribed to the
  `poker_action_log` DynamoDB Stream via `DynamoEventSource` (`archiver-stack.ts:71-75`).
- **No dead-letter queue / `onFailure`:** `retryAttempts: 3` with no DLQ means a poison
  record that fails 3× is **dropped**. (The archiver code itself: `cmd/archiver/main.go`.)
- The archive bucket (`poker-action-log-archive-<env>`) is private, S3-managed encrypted,
  RETAIN in prod / DESTROY in dev.

## Cost-relevant notes

- Game server: EC2 ASG (1–3 instances), dual-stack, no NAT gateway (egress via the shared
  VPC pattern) → compute + ALB the main cost.
- DynamoDB: **on-demand** (pay-per-request) with 1000-RU cap — scales to zero, cheap at
  sandbox traffic.
- Frontend: static S3 + CloudFront (no always-on server).
- Archiver: Lambda invocations only on action-log stream writes (low volume).
- Logs: CloudWatch Logs (1 month prod / 1 week else), rotated to S3.

## CI

- `cdk.json` context pins modern CDK feature flags.
- `infra.yml`: `cdk diff` on PR (posts to PR), `cdk deploy "CtechPoker-${ENV^}-*"` on push to
  `main`/`staging`/`dev`. A CI guard greps for hand-rolled `AssociatePublicIpAddress` and for
  suspiciously low DynamoDB throughput caps (`infra.yml:57-65`).
- Node 24, `npm ci`, deploys named `CtechPoker-<Env>-*`.

## Cross-links

- Server this infra runs: [`../api/README.md`](../api/README.md)
- SPA this infra serves: [`../ui/README.md`](../ui/README.md)
