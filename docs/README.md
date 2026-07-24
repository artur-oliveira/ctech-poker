# ctech-poker — Docs index

Implementation-anchored documentation for the `ctech-poker` service. **The code is the source of truth; the top-level
`OVERVIEW.md` / `ARCHITECTURE.md` / `PLAN.md` are design proposals and may describe features not yet built.**
Per-directory READMEs below are written against the actual code.

## Per-directory docs

- [`api/README.md`](../api/README.md) — Go game server: WebSocket transport, per-table actor model, endpoints/events,
  `sub`-only authz (B9), sandbox ledger.
- [`ui/README.md`](../ui/README.md) — Next.js SPA: routes, lobby, table client, realtime hook, auth flow, gamification.
- [`cdk/README.md`](../cdk/README.md) — AWS CDK: EC2 ASG compute, DynamoDB tables/GSIs, IAM, archiver Lambda (B10), cost
  notes.
- [`CLAUDE.md` / `AGENTS.md`](../CLAUDE.md) — repo-wide conventions (see also the per-dir
  `CLAUDE.md`/`AGENTS.md` in `api/`, `ui/`, `cdk/`).

## IMPLEMENTED (sandbox mode) vs DESIGNED-ONLY

| Area                                                                        | Status                                                            | Evidence                                                                                         |
|-----------------------------------------------------------------------------|-------------------------------------------------------------------|--------------------------------------------------------------------------------------------------|
| Sandbox play-money poker (rooms, lobby, ready, game engine, WS realtime)    | **IMPLEMENTED**                                                   | `api/`, `ui/`                                                                                    |
| Engine: betting, side pots, 7-card eval, CSPRNG shuffle                     | **IMPLEMENTED + tested**                                          | `api/internal/engine/*`                                                                          |
| Sandbox ledger, isolated from ctech-wallet real ledger                      | **IMPLEMENTED**                                                   | `api/internal/walletclient`, `buyin` `CurrencyMode` guard                                        |
| Frontend lobby/table/leaderboard/sandbox credits-spin/achievements-toast    | **IMPLEMENTED**                                                   | `ui/src/app/*`                                                                                   |
| Infra: EC2 ASG, DynamoDB, S3+CloudFront frontend, archiver Lambda           | **IMPLEMENTED + live**                                            | `cdk/lib/*`                                                                                      |
| Real-money mode & Hardening (Phase 5 Tasks 1–12)                            | **IMPLEMENTED (Gated by REAL_MONEY_ENABLED + LEGAL_SIGNOFF_REF)** | `walletclient`, `buyin`, `reconcile`, `metrics`, ASG drain, WAF, hand history, load test harness |
| Hand history audit endpoint                                                 | **IMPLEMENTED**                                                   | `GET /v1.0/tables/:tableId/hands/:handId/history`, player sessions/hands history endpoints       |
| Commit-reveal fairness surface (publish `CommitHash` / reveal `ServerSeed`) | **DESIGNED-ONLY**                                                 | primes exist (`deck.go`) but no endpoint — **B32**                                               |
| Lobby stake/mode filters; private-room share-link UI                        | **DESIGNED-ONLY**                                                 | `ui/` RoomList has no filters                                                                    |
| sandbox credits wheel visual; achievements catalog screen; chat moderation  | **DESIGNED-ONLY**                                                 | not in `ui/src`                                                                                  |

## Known issues (documented honestly — do NOT fix code in docs)

- **B9 — authz is `sub`-only.** `api/internal/api/v1/auth.go:20`; `GET /leaderboard`
  unauthenticated (`api/internal/api/v1/leaderboard.go:11`); M2M credentials not distinguished from user credentials.
  Tracked as a **risk to fix**, not accepted.
- **B10 — archiver Lambda has no DLQ.** `cdk/lib/archiver-stack.ts:71-75` (`retryAttempts: 3`, no `onFailure`) → poison
  records dropped.
- **B31 — `leaderboard.Top("achievement_points")` returns wrong ranking.** Falls through to
  `gsi_hands_won` (`api/internal/leaderboard/store.go:105-133`); no `achievement_points` GSI exists
  (`cdk/lib/dynamodb-stack.ts:78-95`).
- **B32 — commit-reveal fairness unverifiable by clients.** `ServerSeed`/`CommitHash` exist
  (`api/internal/engine/deck/deck.go:50-69`) but no endpoint publishes/reveals them.

## Other reference material

- `plans/` — phased build plans. **`2026-07-19-api-audit-remediation.md`** is a concrete remediation plan (H1–H4, M1–M7,
  L1–L6, E1–E3, S1–S7). Note: some fixes are **already in code** (actor re-resolve `tablews.go:185-198`; prod Valkey
  fail-fast; HTTP rate limiters
  `router.go:39-41`) while others are **not yet** applied — verify against the current tree.
- `specs/` — `2026-07-19-api-audit-remediation.md` (the spec behind the plan above).
- Top-level `OVERVIEW.md` (product/functional), `ARCHITECTURE.md` (technical design),
  `PLAN.md` (phased roadmap), `README.md` (status).

## Read-this-first

Start at the repo [`README.md`](../README.md) for status, the P0 legal risk (real-money mode under Brazilian
regulation), and the relationship to `ctech-account` / `ctech-wallet` /
`ctech-billing`.
