# cdk/ — AGENTS.md (for autonomous agents)

Goal: extend the AWS CDK for poker. All stacks live; deploy via `deploy.yml` (CDK → API →
Frontend).

## Hard rules

1. **Reuse `@aoctech/cdk`** (`PrivateIpv4Ec2Service`, `Environment`, dual-stack helpers).
   Never hand-roll `AssociatePublicIpAddress` or NAT. CI fails the build if it finds either.
2. **No magic strings** — everything naming-related lives in `lib/constants.ts`.
3. **DynamoDB on-demand** with an explicit `maxRead/WriteRequestUnits` (≥100). Single-digit
   caps fail CI.
4. **Game server is EC2 ASG**, not Lambda/Fargate. Keep WS served by the Go binary on the ASG.
5. **Valkey mandatory in prod** — keep the fail-closed behavior (empty `VALKEY_URL` → app
   refuses to start in prod).
6. IAM least-privilege: the API instance role already scopes DynamoDB to the 8 poker tables +
   their indexes, SSM to the specific parameter paths, and S3 to the deployments/logs buckets
   (`api-stack.ts:71-108`). Match that pattern for any new permission.

## Known issues to fix (not accept)

- **B10:** archiver `DynamoEventSource` has `retryAttempts: 3` and **no `onFailure`/DLQ**
  (`archiver-stack.ts:71-75`). Poison records are dropped. Add a dead-letter queue.

## Verify

`npm ci` then `npx cdk diff --all` / `npx cdk deploy`. `infra.yml` posts a diff on PR and
runs two CI guards (no hand-rolled NAT, no tiny DynamoDB throughput cap). Tests: `test/*`.

## Where things live

`lib/api-stack.ts` (compute+IAM), `lib/dynamodb-stack.ts` (tables/GSIs),
`lib/archiver-stack.ts` (stream→S3 Lambda), `lib/frontend-stack.ts` (S3+CloudFront+KV store),
`lib/oidc-stack.ts`, `lib/constants.ts`.
