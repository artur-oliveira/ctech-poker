# ctech-poker — Docs index

Implementation-anchored documentation for the `ctech-poker` service. **The code is the source of truth.** This index was
last re-verified against the tree on **2026-07-28**, with a handful of stale claims (DynamoDB table
count, frontend infra, ASG lifecycle hook) corrected on **2026-09-06** against `cdk/CLAUDE.md` and
the current `cdk/lib/`; the top-level `OVERVIEW.md` / `ARCHITECTURE.md` / `PLAN.md` are design and
history documents and lag the code by design. O índice de planos foi atualizado em **2026-08-16**.

## Per-directory docs

- [`api/README.md`](../api/README.md) — Go game server: protobuf WebSocket transport, per-table actor model,
  endpoints/events, authz, sandbox and real-money ledgers.
- [`ui/README.md`](../ui/README.md) — Next.js SPA: routes, lobby, table client, realtime hook, auth flow, gamification.
- [`cdk/README.md`](../cdk/README.md) — AWS CDK: EC2 ASG compute, 30 DynamoDB tables,
  archiver/reconcile/tablecleanup Lambdas. The frontend is no longer CloudFront/S3 — it deploys to
  Cloudflare Workers Static Assets; see `cdk/CLAUDE.md` for the current, actively-maintained facts.
- Per-directory `CLAUDE.md` / `AGENTS.md` live in `api/`, `ui/`, `cdk/`, plus a repo-root `CLAUDE.md`
  covering cross-repo (CTech family) awareness.

## Feature status

| Area                                                                     | Status                  | Evidence                                                                                     |
|--------------------------------------------------------------------------|-------------------------|----------------------------------------------------------------------------------------------|
| Sandbox play-money poker (rooms, lobby, ready, engine, realtime)          | **LIVE**                | `api/`, `ui/`                                                                                |
| Engine: betting, side pots, 7-card eval, CSPRNG shuffle, equity           | **LIVE + tested**       | `api/internal/engine/*`                                                                      |
| Binary protobuf WebSocket (table + lobby/user gateways)                   | **LIVE**                | `proto/poker.proto`, `api/internal/api/v1/tablews.go`, `ui/src/lib/ws/utils.ts`               |
| Provably-fair: commit, root commit, seed reveal, seed-less partial proof  | **LIVE**                | `engine/hand/snapshot.go`, `ui/src/lib/deckVerify.ts`, `PartialDeckProof.tsx`                 |
| Hand history, hand replayer, hand export, public hand sharing             | **LIVE**                | `sessionlog`, `handshare`, `ui/src/app/(app)/hands/*`, `ui/src/app/(app)/share`               |
| Gamification: leaderboard, achievements, daily sandbox-credit spin        | **LIVE**                | `leaderboard`, `achievements`, `dailyreward`, `ui/src/app/(app)/{leaderboard,achievements}`   |
| Player tooling: private notes, self-HUD stats, profile showcase           | **LIVE**                | `playernotes`, `pokerstats`, `ProfileShowcaseDialog.tsx`, `SelfHudDialog.tsx`                 |
| Table UX: time banks, action pre-selection, reactions, paid rabbit hunt and paid winner-card reveal | **LIVE** | `engine/hand/timebank*.go`, `lib/actionPreselection.ts`, `reactions.ts`, `RabbitHunt.tsx`, `WinnerCards.tsx` |
| Voice: dealer speech synthesis + voice-driven actions                     | **LIVE**                | `ui/src/lib/hooks/useDealerVoice.ts`, `ui/src/lib/voiceActions.ts`                            |
| Bot prevention (Cloudflare Turnstile challenge over WS)                   | **LIVE**                | `api/internal/botcheck`, `ui/src/components/table/BotChallenge.tsx`                           |
| Real-money mode (fixed-fee model, wallet hold/cash-out, reconcile sweep)  | **LIVE, gated off**     | `walletclient`, `buyin`, `reconcile`, `REAL_MONEY_ENABLED` + `LEGAL_SIGNOFF_REF`              |
| Infra: EC2 ASG, DynamoDB, Cloudflare-hosted frontend, 3 Lambdas           | **LIVE**                | `cdk/lib/*`; frontend moved off CloudFront/S3 to Cloudflare (see `cdk/CLAUDE.md`)             |
| WAF-equivalent edge protection                                            | Cloudflare edge, no AWS WAF | frontend has no `aws-wafv2`/`webAclId` (there is no CloudFront distribution left to attach one to); the API's own edge is `ctech-lbalancer`, not evaluated here |
| ASG lifecycle hook for graceful scale-in drain                            | **LIVE, with a known gap** | `api-stack.ts`'s `asg.addLifecycleHook('TerminationDrainHook', …)` invokes a drain Lambda; `cdk/CLAUDE.md`'s "Known issues" documents a real gap (not every termination reliably invokes it) and the app-side mitigation (spot-termination-notice polling) shipped for it |
| Multi-AZ / HA beyond a single ASG                                         | Partial — spread across 3 AZs (2026-09-02, #35) | see `cdk/CLAUDE.md`'s "ASG resilience" entry; capacity is still 1 base instance, not HA in the redundancy sense |

## Known open issues

- **No WAF equivalent to `aws-wafv2` on the frontend** — the frontend has no CloudFront distribution
  to attach one to anymore (migrated to Cloudflare, whose edge network is the WAF-equivalent
  posture now). `PLAN.md`'s old claim that a CloudFront WAF shipped was already wrong before the
  migration; it is doubly moot now. See `cdk/README.md`/`cdk/CLAUDE.md` for the current frontend
  setup.
- **ASG lifecycle hook for graceful drain is built**, not missing — older drafts of this doc were
  wrong. `cdk/CLAUDE.md`'s "Known issues" section has the current, narrower gap (a termination
  that the Lambda invocation itself misses under spot-rebalance churn) and what mitigates it.
All three Lambdas (`reconcile`, `tablecleanup`, `archiver`) have an SQS DLQ **and**, as of #30, a
DLQ-depth alarm plus a Lambda-`Errors` alarm on the shared `ctech-prod-alerts` SNS topic
(`cdk/lib/alarms.ts`), covering both EventBridge Scheduler targets (`reconcile-stack.ts`,
`tablecleanup-stack.ts`) — same for the DynamoDB throttle alarms (#34). Older docs claiming "no
DLQ" / "no alarm" are stale. **All of these alarms are gated by one flag** (`cloudwatchAlarmsEnabled`,
`bin/poker.ts`'s `CLOUDWATCH_ALARMS_ENABLED` env var), which **defaults to `false`** to keep the
CloudWatch cost at $0 until explicitly turned on — the DLQs and Lambdas themselves are unaffected.
`oidc-stack.ts` and `reconcile-stack.ts` both have CDK tests (`cdk/test/oidc-stack.test.ts`,
`cdk/test/reconcile-stack.test.ts`); older docs claiming otherwise are stale.

Issues **B9** (`sub`-only authz), **B10** (archiver DLQ), **B31** (leaderboard ranking), **B32** (fairness surface) and
the real-money buy-in terms-acceptance gap (both `buyin.NewServiceWithGame` and `NewServiceWithPlayers` wiring chain
`.WithPlayers(players)` in `api/internal/app/app.go`, and `Service.buyIn` calls `RequireAccepted` unconditionally
whenever a players store is wired) are all **fixed**; older docs that still list them as open are stale.

## Other reference material

- `plans/2026-08-16-social-friends-safety-and-recent.md` — plano full-stack de amizade mútua, presença, convites
  in-app, jogadores recentes, mute/block/report e remoção do Pix do pós-derrota. **PRs 1–8 implementados**; o flag
  `SOCIAL_GRAPH_ENABLED` continua controlando amizade/presença/convites no rollout (safety e denúncia não dependem
  dele).
- `plans/` — demais planos faseados/de features. Os planos de auditoria de 2026-07-28
  (`2026-07-28-architecture-state-audit-and-provably-fair.md`, `2026-07-28-audit-implementation-plan.md`, both pt-BR)
  carry the current architecture punch list.
- `specs/` — `2026-07-19-api-audit-remediation.md`, `2026-07-23-poker-reveal-timing-and-runout-pacing.md`,
  `2026-07-28-player-avatars-and-next-features.md` (proposed next features).
- Top-level `OVERVIEW.md` (product/game rules), `ARCHITECTURE.md` (technical design), `PLAN.md` (build history),
  `README.md` (status). Untracked `future.md` / `future_analysis.md` are brainstorm/feasibility notes — much of their
  Fase 1–2 backlog has since shipped.

## Read-this-first

Start at the repo [`README.md`](../README.md) for status, the P0 legal risk (real-money poker under Brazilian
regulation), and the relationship to `ctech-account` / `ctech-wallet` / `ctech-cdk`.
