# cdk/ — CLAUDE.md

AWS CDK (TypeScript) for the poker service. All stacks are implemented and live.
Deploy order: **CDK → API → Frontend** (`.github/workflows/deploy.yml`).

## Conventions

- **Reuse `@aoctech/cdk`** shared helpers (`Environment`, dual-stack user-data helpers).
  `PrivateIpv4Ec2Service` still creates retired ALB resources, so the HAProxy API stack owns the
  private-IPv4 launch-template override locally. CI permits it only in `lib/api-stack.ts` and
  rejects copies elsewhere; NAT gateways remain forbidden.
- **Named constants in `lib/constants.ts`** — no magic strings for names, ports, domains,
  SSM paths, role names, or ARNs. AWS resource names must never be inlined at a call site.
- **DynamoDB:** on-demand (`Billing.onDemand`) with an explicit `maxRead/WriteRequestUnits`
  cap (currently 1000) — never a single-digit RCU/WCU cap (CI guard rejects `<100`).
- **Resource naming:** tables carry a `poker_` segment and are prefixed `<env>_` so they
  never collide with other services in the shared account (`868899309401`, `us-east-1`).

## Architecture facts (verified in code)

- **8 stacks**: OIDC (global), DynamoDB, Storage, Archiver, API, Frontend, Reconcile, TableCleanup.
  `Storage` holds only the avatars bucket. It used to live in `FrontendStack` because CloudFront read from it;
  the API serves `/v1.0/avatars/*` itself now, so the bucket moved to a stack whose lifecycle is player data
  rather than a CDN — and, deliberately, not into `API`, which replaces instances on every release.
- **Game server = EC2 Auto-Scaling Group**, capacity 1–3, routed by the shared HAProxy edge.
  **Not Lambda/Fargate.** The Go binary is the HAProxy target directly on port 8080 (no nginx).
  The retained edge security group and VPC are imported from SSM/lookup; no ALB target group or
  listener rule is created.
- **29 DynamoDB tables** (`dynamodb-stack.ts`) — an older revision of this file undercounted (15, before it, 8, then 26,
  then 27). The last two, `poker_hand_reveals` / `poker_hand_reveal_payments`, back the paid history winner-cards
  reveal (`docs/specs/2026-08-21-pay-to-see-winner-cards-history.md`).
- **WebSocket is served by the Go binary** on the ASG (not API Gateway); binary protobuf frames on
  two gateways (`/v1.0/tables/:id/ws`, `/v1.0/ws`).
- **Valkey is mandatory in prod** (in-memory fallback is dev/stage only; prod fails closed).
- **`REAL_MONEY_ENABLED` / `LEGAL_SIGNOFF_REF` are wired** from SSM in the instance `start.sh`,
  defaulting to `false` — enabling real money is a parameter change plus an instance refresh.
- **Three Lambdas**: the archiver (DynamoDB Stream → S3, with an SQS DLQ), plus
  `reconcile` (`rate(5 minutes)`) and `tablecleanup` (`rate(30 minutes)`) on EventBridge Scheduler.
  **The CDK creates no CloudWatch alarms at all** (2026-08-19): the archiver's DLQ alarm and
  the three Lambdas' DLQ-count/throttle/missed-run alarms went 2026-08-17, and `reconcile`/
  `tablecleanup`'s `*ErrorsAlarm` followed — all unmonitored, no SNS subscriber, billed past
  the CloudWatch free tier. Lambda errors are a console/Logs Insights check now.
- **Frontend**: private S3 + CloudFront via OAC, a route KeyValueStore with a viewer-request
  rewrite Function, and a `ResponseHeadersPolicy` carrying the CSP, HSTS and Permissions-Policy.
  **Being retired** — the app deploys to Cloudflare Workers Static Assets from
  `ctech-cdk/.github/workflows/frontend-cloudflare.yml`; this whole stack goes in that migration's Phase 4.
  Its `/avatars/*` behaviour, the `AvatarRewrite` CloudFront Function and the avatars bucket are already gone.
- **Avatars**: the instance role has `s3:GetObject` on `av/*` (`api-stack.ts`) because the API is now the only
  reader of the bucket. `up/*` keeps its separate put/get/delete grant and its 1-day quarantine lifecycle rule.
  `/ctech/{env}/poker/avatar-base-url` must read `https://poker-api[-env].aoctech.app/v1.0/avatars`; like every
  parameter here it is provisioned out of band, so a CDK deploy will not correct a stale value.

- **Secrets live in SSM Parameter Store, not Secrets Manager**, and the parameters are provisioned
  out of band — CDK reads them, never creates them. No Cognito; auth is external ctech-account OIDC.
- **The frontend bucket is private + OAC-only** and the deploy is `s3 sync --delete`. Do not put
  anything in it that a frontend deploy must not wipe, and do not grant the instance role write
  access to it.

## ⚠️ Known issues

- **No WAF** on the CloudFront distribution — no `aws-wafv2` import, no `webAclId`. `PLAN.md`'s
  Task 9 claimed this shipped; it did not.
- **Termination drain works but is not reliably triggered for every termination** (re-verified
  2026-09-01, incident on table `01M1C5GQR7HWXSNSSX8Q49XN9X`). `ENABLE_SSM_AGENT` is hardcoded
  `true` in `cdk/bin/poker.ts` (not the `false` this file previously claimed), and the SSM agent
  does run on the Alpine AMI — confirmed from `/ctech-poker/prod/app` logs, `rc-service app stop`
  actually reaches the Go binary's Fx `OnStop`/`DrainAndRelease` (`"shutting down ctech-poker-api,
  draining table manager leases"`) for terminations it fires for. The gap: during a spot-instance
  rebalance storm (`capacityRebalance: true`, most replacement launches failing on
  `UnfulfillableCapacity`), the ASG recorded at least 4-5 real terminations in ~30 minutes but
  `/aws/lambda/<env>-ctech-poker-termination-drain` only invoked 3 times — one terminated instance
  got no drain attempt at all (no Lambda invocation, no `OnStop` log line), and that is the
  instance whose in-flight hand ended up corrupted. Root cause of the missed invocation
  (SNS/EventBridge delivery under high churn vs. some other path) is not yet isolated — do that
  before changing this mechanism. Application-side, `internal/table/actor.go`'s commit-time
  duplicate-seat guard (`docs/specs/2026-09-01-duplicate-seat-commit-guard.md`) now makes a missed
  drain fail safe (refuse + reload) instead of corrupting state, but it does not make the drain
  itself reliable — a dropped-without-draining instance still costs the game in-progress hands.
- **No DLQ on either EventBridge Scheduler target** (`reconcile-stack.ts`, `tablecleanup-stack.ts`).
- **`oidc-stack.ts` (issue #41, 2026-09-02)**: OIDC trust is now pinned with `StringEquals` to
  exact `sub`s — `repo:<repo>:ref:refs/heads/{main,staging,dev}` for the api/scopes roles, plus
  `repo:<repo>:pull_request` for the infra role (its `cdk diff` PR job). No bare `:*`; the old
  malformed second pattern is gone. `infraRole` dropped `AdministratorAccess` for
  `PowerUserAccess` + a scoped IAM block (service + `cdk-*`/`CtechPoker-*` roles/profiles/policies
  only) + an explicit `Deny` on IAM user/access-key/login-profile/MFA/SAML/OIDC-provider creation
  and `organizations:*`/`account:*`. **Interim** — follow-up is a permissions boundary on the
  roles CDK creates + a CloudFormation-only allowlist. `apiRole`'s `ssm:SendCommand` is scoped to
  instances tagged `Project=ctech-poker` + the `AWS-RunShellScript` document;
  `autoscaling:StartInstanceRefresh` pinned to `*-ctech-poker` ASGs. Covered by `test/oidc-stack.test.ts`.
- **B10 (fixed)** — archiver `DynamoEventSource` has `bisectBatchOnError` + `onFailure: SqsDlq`.
  The DLQ-visible-message alarm was removed 2026-08-17 (see alarm note above); the DLQ itself
  is unchanged.
- **B31 relevance** — `poker_leaderboard_stats` has GSIs only for `hands_won` / `hands_played` /
  `win_rate`. The API rejects any other metric (incl. `achievement_points`); adding a new ranking
  metric requires its own GSI here first.

## Layout

`bin/poker.ts` (entry) · `lib/{constants,api-stack,dynamodb-stack,storage-stack,archiver-stack,frontend-stack,
oidc-stack,reconcile-stack,tablecleanup-stack,bundle}.ts` · `test/*` (Jest/CDK assertions).
Compiled `.d.ts`/`.js` artifacts are ignored build outputs. Edit the `.ts` and run `npm run build`
before Jest so stale local JavaScript cannot shadow the TypeScript modules.

## Mandatory Documentation Policy

**Every code change MUST be documented.**

There are NO exceptions.

Any modification affecting behavior, architecture, APIs, integrations, configuration, deployment, security, business rules, or developer workflow MUST include the corresponding documentation update in the same change.
