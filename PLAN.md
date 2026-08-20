# ctech-poker — Development Plan

> Phased roadmap and build history, not a bite-sized TDD task list. **Status (re-verified
against the code, 2026-07-28):** Phases 0–5 are **implemented and live** in `api/`, `ui/`,
`cdk/`, plus a Phase 6 of post-launch gameplay/UX work (see below). Real-money mode is
**reachable and off by default**: `POST /v1.0/rooms/` accepts `currency_mode: "real"` and
> rejects it unless `REAL_MONEY_ENABLED` is set (`internal/api/v1/rooms.go:58-66`, `:262`),
> which the API stack fetches from SSM (`cdk/lib/api-stack.ts:251-254`). What still gates
> switching it on is the Brazilian regulatory opinion (OVERVIEW.md §11), not code.
> See `docs/README.md` for the current feature-status index.

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
- Hand equity display, achievements (star-tier catalog), leaderboard, daily sandbox-credit spin
  (OVERVIEW.md § 9).
- Deliverable: the gamified experience the brief asks for, on top of an already-correct engine
  — deliberately sequenced after correctness, not before.

## Phase 5 — Real-money mode (built; runtime gate stays off until legal signs off)

- Prerequisite A — `ctech-wallet` real-funds surface: **met.** `walletclient` calls `HoldGame`,
  `ReleaseHold`, `CashoutGame`, `DebitReal`, `IsGamblingActivated`.
- Prerequisite B — `ctech-wallet` DynamoDB throughput: **re-confirm before volume.** The 5
  RCU/WCU cap was the blocker recorded here; verify against the wallet repo, it is not
  observable from this one.
- Prerequisite C — Brazilian legal opinion: **still open.** This is what keeps
  `REAL_MONEY_ENABLED` off, and it is a business decision engineering cannot resolve.
- Hold/cash-out wallet integration (ARCHITECTURE.md § 4).
- Monetization: shipped as a **fixed entry fee** (`entry_fee_cents`), not a percentage rake —
  the Brazil-legal shape, see `docs/plans/2026-07-25-realmoney-fixed-fee-and-sandbox-rake.md`.
  Sandbox tables keep a nominal rake for gameplay parity only.

**Implementation status (re-verified 2026-07-28):**

- Task 1: GameWallet client (`walletclient.Client`) with `HoldGame`, `ReleaseHold`, `CashoutGame`,
  `IsGamblingActivated` — **DONE**
- Task 2: Fail-closed config gate (`REAL_MONEY_ENABLED` + `LEGAL_SIGNOFF_REF` in `config.Load`) — **DONE**
- Task 3: Real-money buy-in/cash-out routing in `buyin.Service` (walletFor selector, hold_id plumbing) — **DONE**:
  `POST /v1.0/rooms/` takes `currency_mode` and an `entry_fee_cents` (`internal/api/v1/rooms.go:58-66,117,123,262`), the
  fixed-fee model shipped in `docs/plans/2026-07-25-realmoney-fixed-fee-and-sandbox-rake.md`, and the UI exposes a
  wallet-mode switch (`ui/src/components/lobby/ProfileMenu.tsx`). **Residual gap:** the real-money `buyin.Service`
  wiring still skips the terms-acceptance check the sandbox path performs (`internal/app/app.go`)
- Task 4: Durable pending-cashout tracking (`reconcile.PendingStore`) — **DONE**
- Task 5: Reconciliation Lambda job (`cmd/reconcile` + EventBridge schedule stack) — **DONE**
- Task 6: EMF structured metrics (`metrics` package emitting JSON lines for CW) — **REVERTED 2026-08-19** (billed per extracted metric; structured logs remain)
- Task 7: CloudWatch alarms (ALARM log lines, lease failover spike in CDK) — **REVERTED 2026-08-19** (unmonitored, no SNS subscriber, billed past the free tier)
- Task 8: Graceful ASG scale-in drain (`DrainAndRelease` in `tablemanager` + Fx hook) — **DONE**
- Task 9: WAF on CloudFront distribution (AWSManagedRulesCommonRuleSet + IP rate limit) — **NOT DONE** (this line
  previously claimed DONE and was wrong): there is no `aws-wafv2` import and no `webAclId` anywhere in `cdk/` (
  `cdk/lib/frontend-stack.ts:103-121`). Application-level rate limits do exist (`internal/api/v1/ratelimit.go`), and
  Turnstile guards bot traffic at the table, but the edge is unprotected
- Task 10: Hand-history audit endpoint (`GET /v1.0/tables/:tableId/hands/:handId/history`) — **DONE**
- Task 11: Load + multi-table chaos test harness (`tests/load` with build tag `load`) — **DONE**
- Task 12: Player-scoped session P&L + hand index (`GET /v1.0/players/me/sessions`, `GET /v1.0/players/me/hands`) — *
  *DONE**

## Phase 6 — Post-launch gameplay, pacing and integrity (2026-07-26 → 07-28, shipped)

Not part of the original brief; delivered after Phase 5 in response to live play sessions.

- Binary protobuf WebSocket for both gateways, replacing JSON, plus a Valkey-backed lobby
  fan-out (`docs/plans/2026-07-26-lobby-websocket-binary-and-valkey.md`).
- Provably-fair surface end to end: commit + root commit published pre-hand, seed revealed
  only on a full showdown, and a **seed-less per-position proof** for hands that end without
  one, persisted with hand history and verified in the browser.
- Pacing and decision UX: decision time banks, action pre-selection, per-card reveal timing,
  rabbit hunt, reality check, dealer voice and voice-driven actions.
- Player tooling: private opponent notes with colour tags, self-HUD (VPIP/PFR/3-bet), profile
  showcase, hand export, public hand sharing, interactive hand replayer.
- Integrity: Cloudflare Turnstile bot challenge over the table socket.
- A full `vitest` suite for the SPA (`ui/src/**/*.test.tsx`), with coverage thresholds enforced
  in `ui/vitest.config.ts`.

## Explicitly deferred (still not built)

- Tournaments / Sit & Go.
- Spectator mode.
- Run-it-twice.
- Multi-table grid.
- Native mobile apps.
- Player avatars — see `docs/specs/2026-07-28-player-avatars-and-next-features.md`.

Rabbit hunting was on this list and shipped in Phase 6.

## Open decisions

1. Real-money legal/regulatory sign-off (blocks flipping `REAL_MONEY_ENABLED`).
2. Whether the identity-level avatar/display name lives in `ctech-account` or stays
   poker-local — see the spec above.
3. Edge protection: accept the missing WAF, or build Task 9 for real.
