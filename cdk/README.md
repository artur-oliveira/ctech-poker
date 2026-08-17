# ctech-poker — CDK (infrastructure)

> HAProxy migration: the API ASG no longer creates an ALB target group or listener
> rule. `ctech-lbalancer` discovers it through its `poker` route; the retained
> `/ctech/{env}/network/alb-sg-id` identifies the shared edge trusted by the API SG.
> `HaproxyEc2Service` from `@aoctech/cdk` now owns the common ASG resources.
> Route creation remains disabled because the existing `poker` route parameter is
> owned by `ctech-lbalancer`.

AWS CDK (TypeScript) for the poker service. **All stacks are implemented and live.**
Deploys in the order **CDK → OAuth scopes → API → Frontend** via `.github/workflows/deploy.yml`. Every claim below is
anchored to `cdk/lib/` and re-verified on **2026-08-16**.

## Stacks (7)

Named `CtechPoker-<Env>-<Name>`, except the global OIDC stack. Entry: `bin/poker.ts`.

| Stack | File | What it provisions |
|---|---|---|
| `CtechPoker-Global-OIDC` | `oidc-stack.ts` | GitHub OIDC deploy roles (frontend / api / infra / scope publisher) |
| `…-DynamoDB` | `dynamodb-stack.ts` | **22** DynamoDB tables + GSIs |
| `…-Archiver` | `archiver-stack.ts` | Action-log archive Lambda (DynamoDB Stream → S3) + SQS DLQ |
| `…-API` | `api-stack.ts` | EC2 ASG game server, HAProxy route, IAM, userdata, alarms |
| `…-Frontend` | `frontend-stack.ts` | S3 + CloudFront, route KeyValueStore, URL-rewrite Function, CSP |
| `…-Reconcile` | `reconcile-stack.ts` | Cash-out reconcile Lambda, EventBridge Scheduler `rate(5 minutes)` |
| `…-TableCleanup` | `tablecleanup-stack.ts` | Stale-table cleanup Lambda, Scheduler `rate(30 minutes)` |

Shared constants (no magic strings): `lib/constants.ts`. Account `868899309401`, region `us-east-1`.
Go Lambdas are bundled by `lib/bundle.ts` (`localGoBundling` — local `go build`, Docker fallback).

## Compute — game server is an **EC2 Auto-Scaling Group** (not Lambda/Fargate)

- `api-stack.ts` uses the shared `HaproxyEc2Service` for its private-IPv4/no-NAT
  security group, encrypted launch template, log groups, ASG and CPU target
  tracking. Poker-specific user data, alarms and termination draining remain local.
  This remains a stateful game server on EC2, matching `ARCHITECTURE.md §1`.
- Capacity: `minCapacity: 1`, `maxCapacity: isProd ? 3 : 1`.
- **No ALB target group or listener rule is synthesized.** The retained edge security group comes
  from `/ctech/<env>/network/alb-sg-id`, and HAProxy discovers the ASG through its `poker` route.
  The Go binary listens directly on port **8080** and serves `/v1.0/health-check`; there is no nginx.
- Continuous deployment: `api.yml` builds `dist/app` (linux/arm64), uploads to the shared deployments
  bucket, and rolls via SSM `RunCommand` calling `/opt/app/deploy.sh`.
- **Valkey is mandatory in prod**: `start.sh` fetches `VALKEY_URL` from SSM; empty in prod means
  `config.Load()` fails closed. The in-memory registry fallback is dev/stage only.
- User data downloads only the official Cloudflare Origin CA RSA root, verifies
  its pinned SHA-256 and installs it, so internal TLS calls to
  `*.internal.aoctech.app` retain full certificate verification.
- Alarms: a metric filter on log lines containing `ALARM:` → `AlarmLogLines`, plus a `LeaseFailovers`
  spike alarm (threshold 5 over 2 periods).
- CloudWatch Agent publishes four bounded 60-second host series under
  `CtechPoker/<env>/Host`: memory %, swap %, root-disk %, and application RSS.
  EC2's native `CPUUtilization`/`CPUCreditBalance` remain the CPU source.
- An ASG termination lifecycle hook gives `tablemanager.DrainAndRelease` up to 120 seconds to
  stop the app through SSM before completing termination; its Lambda fails open so a broken SSM
  agent cannot strand an instance in `Terminating:Wait`.

## WebSocket

Served by the **same Go binary** on the ASG — `GET /v1.0/tables/:id/ws` (table) and `GET /v1.0/ws`
(lobby/user), fronted by HAProxy, which handles the upgrade. **Not** an API Gateway WebSocket API.
Frames are binary protobuf. Fan-out uses the Valkey-backed `ws.Registry` from `api-commons`.

## DynamoDB (`dynamodb-stack.ts`) — 22 tables

All `TableV2`, partition key `pk` (S), on-demand billing with
`maxRead/maxWriteRequestUnits: 1000`, AWS-managed encryption, PITR in prod only, names prefixed
`<env>_`. Removal policy `DESTROY` in dev, `RETAIN` otherwise.

| Table | Sort key | TTL | Stream | GSIs / notes |
|---|---|---|---|---|
| `poker_table_state` | – | – | – | `gsi_active_last_action` (sparse, KEYS_ONLY) — drives `cmd/tablecleanup` |
| `poker_table_state_history` | ✓ | – | – | best-effort audit copy |
| `poker_action_log` | ✓ | 90d | **NEW_IMAGE** | feeds the archiver Lambda |
| `poker_action_guards` | – | 7d | – | per-action idempotency guard |
| `poker_rooms` | ✓ | ✓ | – | `gsi_public` (sparse), `gsi_share_code`; TTL only on temporary invite grants |
| `poker_player_profiles` | – | – | – | poker-local shadow; `gsi_friend_code` exact lookup |
| `poker_player_sessions` | ✓ | ✓ | – | per-table session P&L |
| `poker_player_hands` | ✓ | – | – | hand history incl. fairness proofs; `gsi_table_id` |
| `poker_player_notes` | ✓ | – | – | private per-viewer opponent notes |
| `poker_player_poker_stats` | – | ✓ | – | materialised VPIP/PFR/3-bet + per-hand guard rows |
| `poker_achievement_progress` | ✓ | – | – | `counter` via atomic increment |
| `poker_leaderboard_stats` | ✓ | – | – | `gsi_hands_won`, `gsi_hands_played`, `gsi_win_rate` |
| `poker_daily_reward` | ✓ | 48h | – | one item per player/day, pending → completed |
| `poker_sandbox_purchases` | ✓ | – | – | permanent PIX→fichas purchase history |
| `poker_reaction_entitlements` | ✓ | – | – | premium-reaction ownership and first-use refund gate |
| `poker_reaction_purchases` | ✓ | – | – | permanent PIX/fichas reaction purchase history |
| `poker_pending_cashouts` | ✓ | – | – | reconcile queue; `kind` = cashout \| fee_debit |
| `poker_hand_shares` | – | ✓ | – | public shared-hand tokens, ≤30d |
| `poker_social_edges` | ✓ | – | – | directed friendship/mute/block rows; mirrored mutations are transactional |
| `poker_recent_players` | ✓ | ✓ | – | 90d opponent history; `gsi_recent` chronological pagination |
| `poker_social_events` | ✓ | ✓ | – | 90d in-app inbox; `gsi_inbox`, sparse `gsi_unread` |
| `poker_player_reports` | ✓ | ✓ | – | unresolved rows omit TTL; `gsi_status` moderation queue |

There is deliberately **no `achievement_points` GSI** — the API rejects that metric rather than
silently ranking by another index. Adding a ranking metric means adding its GSI here first.

## Frontend (`frontend-stack.ts`)

- `createNextjsStaticFrontend` from `@aoctech/cdk` creates the private/versioned S3
  bucket, OAC, route KVS, base rewrite, security headers and distribution. The
  poker stack adds avatar storage/rewrites and its application-specific CSP.
- CloudFront distribution: `CACHING_OPTIMIZED` default behavior, HTTP2+3, `PRICE_CLASS_100`, TLS
  1.2_2021, wildcard cert imported by ARN, domain `poker[-env].aoctech.app`.
- A **KeyValueStore** (`<env>-ctech-poker-routes`) plus a CloudFront **Function** (viewer-request)
  rewrite SPA paths to `.html` / `/404.html`. Extensionless keys go through the rewrite; keys with an
  extension pass unmodified.
- `/v1.0/*` and `/.well-known/*` are API-origin behaviors: HTTPS_ONLY, `CACHING_DISABLED`,
  `ALL_VIEWER_EXCEPT_HOST_HEADER`, all methods allowed.
- `ResponseHeadersPolicy`: CSP `default-src 'self'` (with `img-src 'self' data:` and `connect-src`
  allowing ctech-account + `challenges.cloudflare.com` for Turnstile), HSTS 2y preload,
  `X-Frame-Options: DENY`, nosniff, and `Permissions-Policy: on-device-speech-recognition=self`.
- ⚠️ **No WAF.** There is no `aws-wafv2` import and no `webAclId`. `PLAN.md` Task 9 previously
  claimed otherwise; it was wrong.
- ⚠️ The deploy step is `aws s3 sync out/ s3://$S3_BUCKET/ --delete`, so **anything else stored in
  that bucket under a synced prefix is deleted on every frontend deploy** — relevant if user-uploaded
  assets are ever hosted there. Only the GHA frontend role can write to it; the EC2 instance role
  cannot.

## IAM

Instance role `<env>-ctech-poker-api-role` (`api-stack.ts`), managed policies
`AmazonSSMManagedInstanceCore` + `CloudWatchAgentServerPolicy`, plus inline:

- DynamoDB — 9 actions (incl. `BatchGetItem`, `DeleteItem` and `ConditionCheckItem`) over the **22** table ARNs the
  server touches and their `/index/*`.
- SSM `GetParameter` covers the existing game configuration plus account internal
  transport/JWKS, the public account issuer, poker audience and the wallet internal
  transport URL.
- S3 `GetObject` on `<deployments>/ctech-poker/*`, `PutObject` on `<logs>/ctech-poker/*`.

OIDC roles (`oidc-stack.ts`): `ctech-poker-gha-frontend` (S3 frontend RW, CloudFront invalidation +
KeyValueStore writes), `ctech-poker-gha-api` (deployments prefix, `ssm:GetParameter` on `/ctech/*`,
`ssm:SendCommand`, ASG describe + `StartInstanceRefresh`), `ctech-poker-gha-infra`
(**AdministratorAccess**).

## Secrets & config

**No Secrets Manager and no Cognito.** All configuration comes from **SSM Parameter Store** (paths in
`constants.ts`); `POKER_CLIENT_SECRET` and `TURNSTILE_SECRET` are fetched `--with-decryption`. The
parameters themselves are **not created by CDK** — they are provisioned out of band. Auth is external
ctech-account OIDC.

Run `CTECH_AWS_PROFILE=ctech ./scripts/configure-service-url-parameters.sh {env}`
from `ctech-cdk` before deployment. The EC2 API reads account, poker and wallet URLs
from SSM at each service start, so URL changes require only an SSM update and service
restart/instance refresh, not a template change. EC2-to-EC2 transport and JWKS use
`*.internal.aoctech.app`; the OIDC issuer remains public. Reconcile/cleanup Lambdas
remain on public service URLs because they run outside the shared VPC/private zone.

One-time migration note: the currently deployed old API stack owns
`/ctech/{env}/poker/avatar-base-url`. Destroying that stack deletes the parameter.
Run the shared helper again after the old Poker API stack is destroyed and before
deploying the new API stack; the new template only reads it and no longer owns it.

`REAL_MONEY_ENABLED` and `LEGAL_SIGNOFF_REF` **are** wired (`api-stack.ts` fetches both in
`start.sh`, defaulting to `false`), so enabling real money is an SSM change plus an instance refresh.

## Lambdas

| Lambda | Trigger | Notes |
|---|---|---|
| `<env>-poker-action-log-archiver` | `poker_action_log` DynamoDB Stream | TRIM_HORIZON, batch 100, `retryAttempts: 3`, `bisectBatchOnError`, `onFailure: SqsDlq` → 14-day DLQ + depth alarm. **B10 is fixed.** |
| `<env>-ctech-poker-reconcile` | EventBridge Scheduler `rate(5 minutes)` | Sweeps `poker_pending_cashouts`; needs 3 SSM params. ⚠️ no DLQ on the schedule |
| `<env>-ctech-poker-tablecleanup` | EventBridge Scheduler `rate(30 minutes)` | Archives tables idle >15 min, refunds sandbox chips, deletes the room. ⚠️ no DLQ on the schedule |

All three are PROVIDED_AL2023 / arm64.

## `@aoctech/cdk` usage

Direct shared APIs: `HaproxyEc2Service`, `buildCloudWatchAgentConfig`,
`createNextjsStaticFrontend`, `addSwapCommands`, dual-stack agent fragments,
`addCloudflareOriginCaCommands`, and `Environment`. The shared VPC, edge SG,
wildcard ACM certificate, OIDC provider, buckets and Valkey are still imported by
their established ARN/name/SSM contracts.

## Cost-relevant notes

- Game server: EC2 ASG (1–3 instances), dual-stack, **no NAT gateway**; shared HAProxy replaces the ALB cost.
- DynamoDB: on-demand with a 1000-RU cap — scales to zero, cheap at sandbox traffic.
- Frontend: static S3 + CloudFront, no always-on server.
- Lambdas: stream-driven (archiver) plus two low-frequency schedules.
- Logs: CloudWatch Logs (1 month prod / 1 week otherwise), rotated to S3.

## CI & tests

- `infra.yml`: `cdk diff` on PR, `cdk deploy "CtechPoker-${ENV^}-*"` on push to
  `main`/`staging`/`dev`; CI also rejects suspiciously low DynamoDB throughput caps.
  Node 24, `npm ci`.
- `test/`: Jest + `aws-cdk-lib/assertions` for the api, archiver, dynamodb, frontend, reconcile and
  tablecleanup stacks. ⚠️ **No test for `oidc-stack.ts`.** Run `npm run build` before
  Jest: ignored local `.js` outputs can otherwise shadow the `.ts` modules with stale code.

## Cross-links

- Server this infra runs: [`../api/README.md`](../api/README.md)
- SPA this infra serves: [`../ui/README.md`](../ui/README.md)
- Feature-status index: [`../docs/README.md`](../docs/README.md)
