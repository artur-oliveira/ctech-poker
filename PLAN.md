# ctech-poker — Development Plan

> Phased roadmap, not a bite-sized TDD task list. **Status (verified against the code,
> 2026-07-20):** Phases 0–4 (foundations, engine, sandbox end-to-end, frontend/gamification)
> are **implemented and live** in `api/`, `ui/`, `cdk/`. **Phase 5 (real-money mode) is NOT
> started** — gated on ctech-wallet hold/capture endpoints + throughput, and on a Brazilian
> regulatory opinion (see OVERVIEW.md §11). The OVERVIEW/ARCHITECTURE specs may describe
> features not yet built (e.g. commit-reveal fairness surface B32, hand history); see
> `docs/README.md` for the implemented-vs-designed breakdown.

## Phase 0 — Foundations

- Repo skeleton matching company convention (`cmd/`, `internal/`, `Dockerfile`, `Makefile`).
- CDK stack importing shared constructs from `ctech-cdk`.
- Table-lease directory service on Redis/Valkey (reuse `ctech-wallet`'s lock pattern —
  confirm its exact implementation once accessible).
- CI pipeline mirroring the existing company pattern.

## Phase 1 — Game engine (pure logic, no networking, no wallet)

- Hand lifecycle state machine (OVERVIEW.md § 3.1).
- Betting-round logic incl. min-raise and short-all-in-does-not-reopen-action rules
  (OVERVIEW.md § 3.3) — property/table-driven tests covering every edge case explicitly listed
  in the spec before moving on; this is the highest-bug-risk code in the whole project.
- `ComputeSidePots` as an isolated, heavily-tested function (OVERVIEW.md § 3.3).
- 7-card hand evaluator using a vetted algorithm/lookup approach, regression-tested against a
  large table of known hand comparisons (OVERVIEW.md § 3.4).
- CSPRNG shuffle + commit-reveal fairness proof generation (OVERVIEW.md § 3.5).
- Deliverable: a CLI or test harness that can play out a full hand from a scripted action
  sequence and produce the correct pot distribution — no UI, no sockets yet.

## Phase 2 — Table server + real-time transport

- WebSocket gateway + per-table single-writer authority (ARCHITECTURE.md § 2).
- Durable action log + crash-recovery replay (ARCHITECTURE.md § 3).
- Idempotent action de-dup (OVERVIEW.md § 4).
- Disconnect/reconnect handling with grace window and auto-fold/auto-sit-out.
- Deliverable: two browser tabs can play a full hand against each other over a real socket
  connection, and killing the server process mid-hand and restarting it resumes correctly.

## Phase 3 — Sandbox mode end to end

- Room creation/joining (public/private), lobby, ready system (OVERVIEW.md § 2).
- Blind escalation config on private rooms (OVERVIEW.md § 2).
- Sandbox buy-in/cash-out against `ctech-wallet`'s existing sandbox credit/debit endpoints.
- `currency_mode` boundary enforced (OVERVIEW.md § 5).
- Deliverable: a fully playable sandbox-money multi-table product — this is the MVP's
  actual ship target; real-money mode is explicitly Phase 5+, not part of getting to a usable
  product.

## Phase 4 — Frontend polish & gamification

- Card animations (deal, flip, flop reveal, chip movement, pot award) using the provided SVGs.
- Lobby UX, table UX, buy-in/cash-out flow, basic chat (+ moderation, OVERVIEW.md § 8.4).
- Hand equity display, achievements (star-tier catalog), leaderboard, sandbox credit roulette
  (OVERVIEW.md § 9).
- Deliverable: the gamified experience the brief asks for, on top of an already-correct engine
  — deliberately sequenced after correctness, not before.

## Phase 5 — Real-money mode (gated — do not start until prerequisites below are met)

- Prerequisite A: `ctech-wallet` exposes a hold/capture (or equivalent) endpoint for real
  funds (confirmed absent as of the current wallet audit).
- Prerequisite B: `ctech-wallet`'s DynamoDB throughput cap is fixed (confirmed as a hard 5
  RCU/WCU cap as of the current wallet audit — would throttle under real table load).
- Prerequisite C: a legal opinion on real-money poker's regulatory status in Brazil
  (OVERVIEW.md § 11) — a business decision, tracked here because it gates engineering start,
  not because engineering can resolve it.
- Hold/capture wallet integration (ARCHITECTURE.md § 4).
- Rake mechanism, if the monetization question (OVERVIEW.md § 8.1) is resolved in favor of one.

## Explicitly deferred (post-MVP, do not build now)

- Tournaments.
- Spectator mode.
- Run-it-twice / rabbit hunting.
- Native mobile apps.

## Open decisions that should be resolved before Phase 5

1. Rake/monetization model, or explicit decision to launch real-money mode without one.
2. Real-money legal/regulatory sign-off.
3. Confirm `ctech-wallet`'s real-fund API contract (this doc's § 4 is a proposal).
