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

- **Game server = EC2 Auto-Scaling Group** via `@aoctech/cdk`'s `PrivateIpv4Ec2Service`
  (`api-stack.ts:317`), capacity 1–3, behind the shared ctech-cdk ALB (listener priority 45,
  port 8003). **Not Lambda/Fargate.** The Go binary is the ALB target directly (no nginx).
- **WebSocket is served by the Go binary** on the ASG (not API Gateway).
- **Valkey is mandatory in prod** (in-memory fallback is dev/stage only; prod fails closed).
- **Archiver:** DynamoDB Stream (`poker_action_log`) → S3 Lambda (`archiver-stack.ts`).
  B10 fixed: failures bisect + land in an SQS DLQ with a CloudWatch alarm on visible messages.

## ⚠️ Known issues

- **B10 (fixed)** — archiver `DynamoEventSource` now has `bisectBatchOnError` +
  `onFailure: SqsDlq`, and a CloudWatch alarm fires on any visible DLQ message.
- **B31 relevance** — `poker_leaderboard_stats` has GSIs only for `hands_won` /
  `hands_played` / `win_rate`. The API rejects any other metric (incl. `achievement_points`);
  adding a new ranking metric requires its own GSI here first.

## Layout

`bin/poker.ts` (entry) · `lib/{constants,api-stack,dynamodb-stack,archiver-stack,
frontend-stack,oidc-stack}.ts` · `test/*` (Jest/CDK assertions).
