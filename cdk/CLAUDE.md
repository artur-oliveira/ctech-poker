# cdk/ — CLAUDE.md

AWS CDK (TypeScript) for the poker service. All stacks are implemented and live.
Deploy order: **CDK → API → Frontend** (`.github/workflows/deploy.yml`).

## Conventions

- **Reuse `@aoctech/cdk`** shared constructs (`PrivateIpv4Ec2Service`, `Environment`,
  dual-stack helpers). Do NOT hand-roll `AssociatePublicIpAddress` or NAT gateways — use the
  shared no-NAT EC2/ASG pattern (CI guards this: `infra.yml:57-65`).
- **Named constants in `lib/constants.ts`** — no magic strings for names, ports, domains,
  SSM paths, role names, or ARNs. AWS resource names must never be inlined at a call site.
- **DynamoDB:** on-demand (`Billing.onDemand`) with an explicit `maxRead/WriteRequestUnits`
  cap (currently 1000) — never a single-digit RCU/WCU cap (CI guard rejects `<100`).
- **Resource naming:** tables carry a `poker_` segment and are prefixed `<env>_` so they
  never collide with other services in the shared account (`868899309401`, `us-east-1`).

## Architecture facts (verified in code)

- **7 stacks**: OIDC (global), DynamoDB, Archiver, API, Frontend, Reconcile, TableCleanup.
- **Game server = EC2 Auto-Scaling Group** via `@aoctech/cdk`'s `PrivateIpv4Ec2Service`, capacity
  1–3, behind the shared ctech-cdk ALB (listener priority 45, port 8003). **Not Lambda/Fargate.**
  The Go binary is the ALB target directly (no nginx). The **ALB, its listener, its security group
  and the VPC are all imported** from SSM/lookup, never created here.
- **15 DynamoDB tables** (`dynamodb-stack.ts`), not 8 — an older revision of this file undercounted.
- **WebSocket is served by the Go binary** on the ASG (not API Gateway); binary protobuf frames on
  two gateways (`/v1.0/tables/:id/ws`, `/v1.0/ws`).
- **Valkey is mandatory in prod** (in-memory fallback is dev/stage only; prod fails closed).
- **`REAL_MONEY_ENABLED` / `LEGAL_SIGNOFF_REF` are wired** from SSM in the instance `start.sh`,
  defaulting to `false` — enabling real money is a parameter change plus an instance refresh.
- **Three Lambdas**: the archiver (DynamoDB Stream → S3, with an SQS DLQ and alarm), plus
  `reconcile` (`rate(5 minutes)`) and `tablecleanup` (`rate(30 minutes)`) on EventBridge Scheduler.
- **Frontend**: private S3 + CloudFront via OAC, a route KeyValueStore with a viewer-request
  rewrite Function, and a `ResponseHeadersPolicy` carrying the CSP, HSTS and Permissions-Policy.

- **Secrets live in SSM Parameter Store, not Secrets Manager**, and the parameters are provisioned
  out of band — CDK reads them, never creates them. No Cognito; auth is external ctech-account OIDC.
- **The frontend bucket is private + OAC-only** and the deploy is `s3 sync --delete`. Do not put
  anything in it that a frontend deploy must not wipe, and do not grant the instance role write
  access to it.

## ⚠️ Known issues

- **No WAF** on the CloudFront distribution — no `aws-wafv2` import, no `webAclId`. `PLAN.md`'s
  Task 9 claimed this shipped; it did not.
- **No ASG lifecycle hook** here or in `PrivateIpv4Ec2Service`, so `tablemanager.DrainAndRelease`
  gets only the default EC2 shutdown grace period.
- **No DLQ on either EventBridge Scheduler target** (`reconcile-stack.ts`, `tablecleanup-stack.ts`).
- **No test** for `reconcile-stack.ts` or `oidc-stack.ts`.
- **B10 (fixed)** — archiver `DynamoEventSource` has `bisectBatchOnError` + `onFailure: SqsDlq`, and
  a CloudWatch alarm fires on any visible DLQ message.
- **B31 relevance** — `poker_leaderboard_stats` has GSIs only for `hands_won` / `hands_played` /
  `win_rate`. The API rejects any other metric (incl. `achievement_points`); adding a new ranking
  metric requires its own GSI here first.

## Layout

`bin/poker.ts` (entry) · `lib/{constants,api-stack,dynamodb-stack,archiver-stack,frontend-stack,
oidc-stack,reconcile-stack,tablecleanup-stack,bundle}.ts` · `test/*` (Jest/CDK assertions).
Compiled `.d.ts`/`.js` artifacts are checked in alongside sources — edit the `.ts`.

## Mandatory Documentation Policy

**Every code change MUST be documented.**

There are NO exceptions.

Any modification affecting behavior, architecture, APIs, integrations, configuration, deployment, security, business rules, or developer workflow MUST include the corresponding documentation update in the same change.
