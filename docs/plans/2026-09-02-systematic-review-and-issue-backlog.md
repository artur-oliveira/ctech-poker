# CTech Poker — Full Codebase Review & Issue Backlog (v2)

> Date: 2026-09-02 · Status: **FILED** — all 95 items are open GitHub issues **#29–#123** in
> `artur-oliveira/ctech-poker`, labelled and assigned. The label taxonomy in §4 was created.
> **This document's numbering (1–92, D1/D2/D6) = GitHub issue number − 28** (issue 1 → #29,
> issue 92 → #120, D1/D2/D6 → #121/#122/#123). Cross-references inside the filed issue bodies use
> the GitHub numbers; cross-references in *this document* use the 1–92 scheme. Full mapping in §11.
>
> v2 replaced a v1 draft that had unverified claims (SSM agent, WAF) and speculative features.
> v2.1 added the D3/D4/D5 deep-dives (§7). v2.2 added §8–10 — the four parallel per-module
> frontend reviews (Issues 41–92; full tables in `docs/plans/2026-09-02-frontend-module-review/`).

---

## 0. What was actually read (be honest about coverage)

**Read in full, line by line — findings below are code-anchored:**

| Area        | Files                                                                                                                                                                                                                                                |
|-------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Engine      | `betting.go`, `sidepots.go`, `deck.go`, `equity.go`, `hand/state.go`, `hand/hand.go` (2058 L), `hand/snapshot.go` (810 L)                                                                                                                            |
| Table actor | `table/actor.go` (2838 L, full), `commands.go`, `escalation.go`, `turntimeout.go`, `seed.go`                                                                                                                                                         |
| Storage     | `tablestore/{store,dynamo}.go`, `tablemanager/manager.go`, `roomstore/{dynamo,room}.go`, `tablelease/lease.go`                                                                                                                                       |
| Money       | `buyin/service.go` (full), `walletclient/client.go` (full), `entitlement/{entitlement,store}.go`, `reconcile/pending.go`                                                                                                                             |
| API         | `api/v1/{router,ratelimit,tablews,auth,walletwebhook}.go`, `rooms.go` (validation half)                                                                                                                                                              |
| Lambdas     | `cmd/{reconcile,tablecleanup,archiver}/main.go`                                                                                                                                                                                                      |
| Config      | `config/config.go`                                                                                                                                                                                                                                   |
| CDK         | `bin/poker.ts`, `lib/{api-stack,dynamodb-stack,reconcile-stack,tablecleanup-stack,archiver-stack,storage-stack,oidc-stack}.ts`                                                                                                                       |
| CI/CD       | all 6 workflows                                                                                                                                                                                                                                      |
| Frontend    | `useTableRealtime.ts` (1086 L, full), `useLobbyRealtime.ts`, `api/client.ts`, `network/liveness.ts`, `auth/session.ts`, `deckVerify.ts`, `ws/{utils,origin}.ts`, `api/table.ts`, `providers/RealtimeBridge.tsx` + risk-pattern grep of all of `src/` |

**Reviewed only by pattern/grep, NOT line by line — flagged `needs-deep-dive` where it matters:**

- Backend peripheral modules (~28): `achievements`, `leaderboard`, `dailyreward`, `social`,
  `presence`, `avatar`, `cosmetics`, `cosmeticpurchase`, `reactions`, `reactionpurchase`,
  `sandboxpurchase`, `botcheck`, `chatfilter`, `pokerstats`, `matchup`, `sessionlog`, `handshare`,
  `handreveal`, `highlights`, `playernotes`, `recentplayers`, `reports`, `handhook`, `tablestreak`,
  `tableconn`, `wsdrain`, `player`, `oauthresource`, `problem`.
- `internal/app/app.go` (960 L Fx wiring) — skimmed only.
- 29 `api/v1/*.go` handler files beyond the 6 read.
- `handeval/*` (evaluator + generator) — trusted on the strength of its exhaustive-proof test.
- Frontend page/component files (`app/table/page.tsx` 824 L, `ActionBar` 516 L, `Seat`,
  `TableStage`, lobby components, `store` pages) — read the hooks they consume, not the components.
- `cmd/{server,moderation}/main.go`.

---

## 1. Executive summary

The codebase is **mature and disciplined** — extensive design docs, near-zero `TODO`/`FIXME`, 90%-enforced frontend
coverage, an exhaustively-tested hand evaluator, and a revised architecture (DynamoDB conditional writes as the sole
correctness mechanism, actor as a serialisation/timer device) that holds up under scrutiny. The engine math (betting,
side pots, showdown, fairness proofs, per-viewer masking) was read closely and **no correctness bug was found** — the
areas that warrant a focused audit (`hand.go` multi-way all-in + run-it-twice settlement, gamification abuse)
are called out, not claimed clean.

The real risk is concentrated:

1. **Process-crash surface in the table actor.** `Actor.Run`/`handle` has **no `recover()`** (only the two WS handlers
   do). Any panic in the engine — an unchecked `dealCard()` slice index, a nil deref, a decode of malformed persisted
   state — takes down the whole process and every table on that instance, feeding straight into the
   ungraceful-termination path below. (Issue 1)

2. **Instance-wide serialization on `tablemanager` cold-start.** `GetOrCreateActor` holds one process-global
   `sync.Mutex` across a DynamoDB `LoadTable`, a Valkey lease `Acquire`, **and** a
   `roomLoader` DynamoDB read. Under a post-deploy reconnect storm (every client re-resolving its table) this serialises
   the entire instance behind whichever table is slowest to load. (Issue 3)

3. **Observability is effectively blind.** Every CloudWatch alarm was deleted 2026-08-19. The three Lambda DLQs
   (`reconcile`, `tablecleanup`, `archiver`) exist but **none has an alarm** — a message landing in one is invisible.
   Worse, `cmd/reconcile` catches per-entry errors and returns `nil`, so a permanently-stuck money movement never even
   reaches its DLQ; it just logs `ALARM:` (a grep target, not a page) every 5 minutes forever, with no attempt counter.
   (Issues 2, 4)

4. **Infra is minimal to a degree that is now a liability** — but the fixes are cheap. Single-AZ, spot-only ASG (`max 2`
   `t4g.nano`, 512 MB, two app processes), a blanket **1000 RCU/WCU cap on every DynamoDB table** including
   `poker_table_state` with no throttle alarm, and a termination-drain lifecycle hook that (per `api/CLAUDE.md` + the
   2026-09-01 incident) fired for only 3 of ≥4–5 terminations in a spot rebalance storm — producing the triple-seat bug.
   None of these need expensive AWS services to address: instance-type diversification, `GOMEMLIMIT`, a byte-bounded
   equity cache, a handful of alarms, and a code-level idempotent drain are all near-zero cost. (Issues 5–9)

5. **Real-money gaps** (real money is gated off, so these are *blockers-when-unblocked*, not P0-now): the real-money
   buy-in path skips the poker-terms check the sandbox path enforces (Issue 10); two `ctech-account` scopes are
   ungranted (Issue 11); `config.LoadForLambda` does **not** mirror the `REAL_MONEY_ENABLED`→`LEGAL_SIGNOFF_REF` gate;
   `cmd/tablecleanup` has **no real-money cleanup path** and treats a missing room row as sandbox (a currency-boundary
   hole if sweep ordering ever breaks); and the loser of an `entitlement.Claim` race proceeds to seat the player while
   the winner's `DebitReal` may still be failing (Issue 12).

6. **GitHub OIDC is over-privileged.** `oidc-stack.ts`'s infra role has `AdministratorAccess` with a trust condition of
   `repo:owner/repo:*` — **not scoped to a branch or environment** — and the API role has `ssm:SendCommand` on
   `Resource: '*'` (any instance in the account). (Issue 13)

The rest is a normal long tail: a `buyin` refund idempotency-key collision (Issue 14), per-instance WS rate limiters
(Issue 15), WS accepts an absent `Origin` header (Issue 16), the fixed-window rate-limiter TTL race (Issue 17),
`unsafe-inline` in the script CSP (Issue 18), a dead duplicate frontend workflow (Issue 19), no
SAST/secret-scanning/dependency-review in CI (Issue 20), unpinned CI tools (Issue 21), `useTableRealtime.ts` at 1086
lines (Issue 22), `actor.go` at 2838 lines with rollback enforced only by convention (Issues 23, 24), no client-side
error telemetry (Issue 25), and assorted dead code / silent error swallows (Issue 26).

---

## 2. Priority matrix

| #      | Title (short)                                                                                                                                                                                                                                                                                                                                                                                                                        | Subsystem / Module               | Type                | Prio     | Effort | Cost to fix                                         |
|--------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------|---------------------|----------|--------|-----------------------------------------------------|
| 1      | Table actor `Run`/`handle` has no panic recovery → full-process crash                                                                                                                                                                                                                                                                                                                                                                | backend / table                  | reliability         | **High** | S      | $0                                                  |
| 2      | No alarm on any of the 3 Lambda DLQs; nothing surfaces a message landing in one                                                                                                                                                                                                                                                                                                                                                      | infra                            | reliability         | **High** | S      | ~R$3/mo (6 alarms) → existing ctech-prod-alerts SNS |
| 3      | `tablemanager.GetOrCreateActor` serializes the whole instance behind one mutex + 3 network calls                                                                                                                                                                                                                                                                                                                                     | backend / tablemanager           | performance         | **High** | M      | $0                                                  |
| 4      | `cmd/reconcile` swallows per-entry errors → stuck money never reaches DLQ, no attempt counter                                                                                                                                                                                                                                                                                                                                        | backend / reconcile              | correctness         | **High** | M      | $0                                                  |
| 5      | Termination-drain hook is best-effort (3/5 in a spot storm → triple-seat incident)                                                                                                                                                                                                                                                                                                                                                   | infra / backend                  | reliability         | **High** | M      | $0                                                  |
| 6      | Blanket 1000 RCU/WCU cap on every DynamoDB table + no throttle alarm                                                                                                                                                                                                                                                                                                                                                                 | infra                            | performance         | **High** | S      | ~$0 (cap raise only bills on use)                   |
| 7      | Single-AZ, spot-only, single instance-type ASG → correlated non-self-healing outage                                                                                                                                                                                                                                                                                                                                                  | infra                            | reliability         | **High** | S      | $0 (type diversification) / small (on-demand base)  |
| 8      | `t4g.nano` 512 MB vs unbounded in-process actors/timers/equity cache                                                                                                                                                                                                                                                                                                                                                                 | infra / backend                  | reliability         | Medium   | M      | $0 (GOMEMLIMIT + cache bound)                       |
| 9      | `broadcastAll` runs equity Monte-Carlo ×players + preselection/exit cascades on the actor goroutine per commit                                                                                                                                                                                                                                                                                                                       | backend / table                  | performance         | Medium   | M      | $0                                                  |
| 10     | Real-money buy-in skips the poker-terms acceptance check                                                                                                                                                                                                                                                                                                                                                                             | backend / buyin                  | security            | **High** | S      | $0                                                  |
| 11     | Two `ctech-account` scopes ungranted → wallet verification impossible                                                                                                                                                                                                                                                                                                                                                                | backend / walletclient           | bug                 | **High** | S      | $0                                                  |
| 12     | Real-money: `LoadForLambda` skips the legal gate; no real-money table cleanup; Claim-race free seat                                                                                                                                                                                                                                                                                                                                  | backend / money                  | correctness         | **High** | M      | $0                                                  |
| 13     | GitHub OIDC infra role = `AdministratorAccess`, trust not branch-scoped; API role `ssm:SendCommand` on `*`                                                                                                                                                                                                                                                                                                                           | infra / .github                  | security            | **High** | S      | $0                                                  |
| 14     | `buyin` refund idempotency key collides across players on empty `idemKey`                                                                                                                                                                                                                                                                                                                                                            | backend / buyin                  | bug                 | **High** | S      | $0                                                  |
| 15     | WS rate limiters are per-instance → N× the limit across the fleet                                                                                                                                                                                                                                                                                                                                                                    | backend / api                    | security            | Medium   | M      | $0                                                  |
| 16     | WS upgrade accepts any request with no `Origin` header (prod/staging)                                                                                                                                                                                                                                                                                                                                                                | backend / api                    | security            | Medium   | S      | $0                                                  |
| 17     | Fixed-window rate limiter can leave a TTL-less key if EXPIRE fails after INCR                                                                                                                                                                                                                                                                                                                                                        | backend / api                    | bug                 | Medium   | S      | $0                                                  |
| 18     | `unsafe-inline` in the `script-src` CSP                                                                                                                                                                                                                                                                                                                                                                                              | frontend / infra                 | security            | Medium   | M      | $0                                                  |
| 19     | Dead/duplicate `frontend-cloudfront.yml` still runs on every UI PR                                                                                                                                                                                                                                                                                                                                                                   | infra / .github                  | tech-debt           | Medium   | S      | $0                                                  |
| 20     | No SAST / secret-scanning / dependency-review in CI                                                                                                                                                                                                                                                                                                                                                                                  | infra / .github                  | security            | **High** | S      | $0 (GitHub-native)                                  |
| 21     | `staticcheck@latest` / `govulncheck@latest` unpinned; `reconcile-stack`/`oidc-stack` untested                                                                                                                                                                                                                                                                                                                                        | infra / .github                  | tech-debt           | Medium   | S      | $0                                                  |
| 22     | `useTableRealtime.ts` (1086 L) — decompose the resilience state machine                                                                                                                                                                                                                                                                                                                                                              | frontend / table                 | tech-debt           | Medium   | L      | $0                                                  |
| 23     | `actor.go` cache-rollback obligation is convention-only across mutating handlers                                                                                                                                                                                                                                                                                                                                                     | backend / table                  | correctness         | **High** | L      | $0                                                  |
| 24     | `actor.go` (2838 L) — decompose (timers, presence, hooks)                                                                                                                                                                                                                                                                                                                                                                            | backend / table                  | tech-debt           | Medium   | XL     | $0                                                  |
| 25     | No client-side error telemetry (crashes/JS errors invisible)                                                                                                                                                                                                                                                                                                                                                                         | frontend                         | reliability         | Medium   | S      | $0 (POST to own API)                                |
| 26     | Dead code + silent error swallows (`saveHandHistorySnapshot`, empty `if errors.Is{}`, `Dispatch` no-op, frontend `equity` msg, `deck.commitHash`)                                                                                                                                                                                                                                                                                    | backend + frontend               | tech-debt           | Low      | S      | $0                                                  |
| 27     | `archiver` renders numeric attrs as float64 → audit precision risk                                                                                                                                                                                                                                                                                                                                                                   | backend / archiver               | correctness         | Low      | S      | $0                                                  |
| 28     | `reconcile-stack` IAM grants unused `dynamodb:Scan`; `apiRole` `ssm:*` on `*`                                                                                                                                                                                                                                                                                                                                                        | infra                            | security            | Low      | S      | $0                                                  |
| 29     | `buyin.tableUnavailable` spins up a full Actor (goroutine+timers+lease) just to count seats                                                                                                                                                                                                                                                                                                                                          | backend / buyin                  | performance         | Low      | S      | $0                                                  |
| 30     | `ReconnectCmd` dispatched to the actor on every inbound WS frame incl. `ping`                                                                                                                                                                                                                                                                                                                                                        | backend / table                  | performance         | Medium   | S      | $0                                                  |
| 31     | ~10 `react-hooks/set-state-in-effect` suppressions; ~93 `any`/`as any` casts                                                                                                                                                                                                                                                                                                                                                         | frontend                         | tech-debt           | Low      | M      | $0                                                  |
| 32     | No WCAG/axe gate, no bundle-size budget, no Lighthouse CI on the frontend                                                                                                                                                                                                                                                                                                                                                            | frontend                         | testing             | Medium   | M      | $0                                                  |
| **D1** | `hand.go` multi-way all-in + run-it-twice + odd-chip + folded-money settlement audit                                                                                                                                                                                                                                                                                                                                                 | backend / engine                 | needs-deep-dive     | **High** | L      | $0                                                  |
| **D2** | `reconcile` + `entitlement` concurrency / lost-update audit                                                                                                                                                                                                                                                                                                                                                                          | backend / money                  | needs-deep-dive     | **High** | M      | $0                                                  |
| **D3** | Gamification fairness + abuse audit — **DONE this pass**, see §7; dailyreward is sound, findings became Issues 35/38                                                                                                                                                                                                                                                                                                                 | backend / gamification           | needs-deep-dive     | —        | —      | $0                                                  |
| **D4** | Peripheral-module + `api/v1` handler sweep — **DONE this pass** (leaderboard, pokerstats, achievements, player, dailyreward, sessionlog, matchup, handshare, handreveal, social, recentplayers + read handlers); findings in §7. Still light: `avatar`, `cosmetics*`, `reactions*`, `botcheck`, `chatfilter`, `presence`, `highlights`, `playernotes`, `reports`, `handhook`, `tablestreak`, `tableconn`, `wsdrain`, `oauthresource` | backend                          | needs-deep-dive     | Low      | M      | $0                                                  |
| **D5** | Frontend component sweep — **DONE this pass** (`table/page.tsx`, `leaderboard/page.tsx`, `ActionBar` head, `lib/api/{player,leaderboard}`); findings → Issues 39/40. Still light: `Seat`, `HandOutcome`, `HandReplayer`, lobby + store dialogs                                                                                                                                                                                       | frontend                         | needs-deep-dive     | Low      | M      | $0                                                  |
| 33     | `onHandComplete` runs the entire gamification pipeline (~50–150 sequential DynamoDB calls) synchronously on the table actor goroutine                                                                                                                                                                                                                                                                                                | backend / table                  | performance         | **High** | M      | $0                                                  |
| 34     | Leaderboard has no "my rank"; frontend only ranks within the fetched first page ("#N de 50", not of total); single-partition GSI is a global hotspot                                                                                                                                                                                                                                                                                 | backend + frontend / leaderboard | correctness+perf    | **High** | M      | $0 (Valkey ZSET already deployed)                   |
| 35     | `win_rate` leaderboard is gameable — a 1-hand 100% account tops it; no min-hands floor in the GSI                                                                                                                                                                                                                                                                                                                                    | backend / leaderboard            | correctness         | **High** | S      | $0                                                  |
| 36     | Display-name changes never propagate to denormalized copies (leaderboard, hand history, public hand shares, live seats)                                                                                                                                                                                                                                                                                                              | backend / player                 | correctness+privacy | Medium   | M      | $0                                                  |
| 37     | `matchup.RecordHand` = C(9,2)=36 pairs × 2 items = 72 in one transaction (72/100 hard limit) + ~144 WCU/hand                                                                                                                                                                                                                                                                                                                         | backend / matchup                | performance         | Medium   | M      | $0                                                  |
| 38     | achievements + leaderboard counters double-count on a Valkey blip (`claimHandHooks` fails open, `Increment` is not idempotent)                                                                                                                                                                                                                                                                                                       | backend / gamification           | correctness         | Medium   | S      | $0                                                  |
| 39     | `table/page.tsx` is a ~824-line god component (~30 useState / ~15 useEffect / 8+ queries); 1 Hz full re-render during a turn                                                                                                                                                                                                                                                                                                         | frontend / table                 | tech-debt           | Medium   | L      | $0                                                  |
| 40     | Frozen `avatar_url` in hand-history opponent summaries 404s after `ClearAvatar`                                                                                                                                                                                                                                                                                                                                                      | backend / sessionlog             | tech-debt           | Low      | S      | $0                                                  |
| **D6** | Load / soak test at target concurrency before wider release                                                                                                                                                                                                                                                                                                                                                                          | backend / infra                  | testing             | **High** | L      | small (test env)                                    |

**No new-feature section.** Session recap already exists (not shareable yet — noted, not filed); multi-table grid and
waitlist/auto-seat were considered and rejected as not fitting the product. Feature ideas belong in a separate
conversation with the product owner, not this defect backlog.

---

## 3. Detailed issues

### Issue 1 — [BACKEND/TABLE] Table actor `Run`/

`handle` has no panic recovery — a single engine panic crashes the whole process

**Module:** `api/internal/table/actor.go` (`Run`, `handle`)
**Priority:** High · **Effort:** S · **Cost:** $0

**Problema**
`Actor.Run` is `for { select { case cmd := <-a.cmds: err := a.handle(ctx, cmd); ... } }` with a
`defer close(a.done)` and **no `recover()`**. The two WebSocket handlers in `tablews.go` recover their own goroutine
panics (with an explicit comment that an unrecovered panic there "kills the whole process"), but the actor goroutine —
which runs all engine logic — does not. Panic sources reachable in principle:

- `hand.dealCard()` / `t.shuffle.Cards[t.nextCard]` — no bounds check (safe at 9-max today, not defensively guaranteed).
- `NewTableFromState` / `dynamodbav` decode of a persisted `hand.State` after a schema change.
- Any nil deref in the ~2000 lines of `hand.go` reachable from a malformed but committed state.

**Impacto**
One panic → `Run` unwinds → `close(a.done)` → **unrecovered goroutine panic terminates the process** → every table Actor
on that instance dies at once → the (already unreliable, Issue 5)
drain does not run → stranded leases, and the same conditions that caused the 2026-09-01 triple-seat incident.

**Solução proposta**
Wrap `a.handle` in a `defer/recover` that: (a) logs the panic with table/hand/command context, (b) replies to the
command's `Reply` channel with a synthetic `tablestore.ErrUnavailable`-wrapped error (client resyncs, not "invalid
action"), (c) forces `a.cached = nil` so the next command reloads authoritative state. The actor stays alive; the
poisoned command fails cleanly.

**Critérios de aceitação**

- [x] A panic in any handler is recovered; the process survives (`Actor.handleSafely`, `internal/table/actor.go`)
- [x] The panicking command's caller gets a resyncable error, not `invalid_action` (`tablestore.ErrUnavailable`-wrapped)
- [x] `a.cached` is discarded (along with `version`/`handID`/`activity`) so the next command reloads
- [x] Panic logged with table_id / hand_id / command type + full stack
- [x] Defensive bounds check added to `hand.dealCard()` — panics with table/stage context (recovered by the actor loop) instead of a bare index-out-of-range; a full `error`-return refactor across its ~10 call sites was deliberately deferred as out of proportion to the fix
- [x] Test: injected panic → survives + reloads (`internal/table/panicrecovery_test.go`)

Done 2026-09-02 (#29).

---

### Issue 2 — [INFRA] No alarm on any Lambda DLQ; a message landing in one is invisible

**Module:** `cdk/lib/{reconcile,tablecleanup,archiver}-stack.ts`
**Priority:** High · **Effort:** S · **Cost:** ~cents/month

**Problema**
All three Lambdas **do** have SQS DLQs (contrary to `docs/README.md` / `api/CLAUDE.md`, which are stale). But there is
**not a single `cloudwatch.Alarm` anywhere in `cdk/lib/`** — every alarm was deleted 2026-08-19 (ARCHITECTURE.md §7),
including "the archiver's DLQ-depth alarm". So a poison stream record (archiver) or a failed invocation
(reconcile/tablecleanup) lands in a DLQ nobody watches, with a 14-day retention after which it is gone.

**Impacto**
Silent data loss for the audit archive; silent failure of the money-reconciliation safety net.

**Solução proposta** (cheap, AWS-native)

- One alarm per DLQ on `ApproximateNumberOfMessagesVisible >= 1`, `treatMissingData: NOT_BREACHING`.
- One alarm per Lambda on `Errors`.
- A DynamoDB `WriteThrottleEvents`/`ReadThrottleEvents` alarm on `poker_table_state` (ties Issue 6).
- **Alarm action → the existing SNS topic `arn:aws:sns:us-east-1:868899309401:ctech-prod-alerts`**
  (email). Import it in CDK with `sns.Topic.fromTopicArn` and pass it to
  `.addAlarmAction(new cw_actions.SnsAction(topic))` on every alarm; no new topic, no new subscription.

**Cost — honest accounting, not a guarantee.** I read AWS's public us-east-1 pricing, I do not control your bill:

- **Standard-resolution CloudWatch alarms: $0.10/alarm-month.** 6 alarms ≈ **$0.60/mo ≈ R$3.20/mo**
  at ~5.3 BRL/USD (not R$0.60 — the v1 figure under-converted). Flat per-alarm; alarm state flapping does **not** add
  cost.
- **The metrics themselves are free:** `Lambda Errors`, `SQS ApproximateNumberOfMessagesVisible`,
  `DynamoDB *ThrottleEvents` are all AWS-emitted basic metrics, already published at no charge. No custom metrics, no
  detailed monitoring, no high-resolution alarms — those are what cost more, and none are used here.
- **SNS email: first 1,000 notifications/month free**,
  then $2.00/100k. At the alarm volume this produces (a handful of emails on an actual incident) it is effectively $0.
  It would only grow if the topic also fans out to **SMS** ($0.00645/msg in BR) or triggers a **Lambda** — using the
  existing email topic avoids both.
- **What I cannot guarantee:** that nobody later attaches SMS/Lambda subscriptions to that topic, that AWS doesn't
  change pricing, or that a misconfigured flapping alarm on a chatty metric doesn't send a burst of emails (annoying,
  not expensive — still counts against the free 1,000). Realistic worst case for *this* change: **under R$5/month.**

Also: fix the stale "no DLQ" / "no alarm" claims in `docs/README.md` and both `CLAUDE.md`.

**Critérios de aceitação**

- [ ] DLQ-depth alarm on all 3 DLQs
- [ ] `Errors` alarm on all 3 Lambdas
- [ ] Throttle alarm on `poker_table_state`
- [ ] Alarms notify a channel someone reads
- [ ] Stale "no DLQ" / "no alarm" docs corrected
- [ ] CDK test asserting each DLQ + alarm exists

---

### Issue 3 — [BACKEND/TABLEMANAGER]

`GetOrCreateActor` serializes the whole instance behind one mutex + three network calls

**Module:** `api/internal/tablemanager/manager.go`
**Priority:** High · **Effort:** M · **Cost:** $0

**Problema**
`GetOrCreateActor` does `m.mu.Lock(); defer m.mu.Unlock()` and, inside that critical section:

1. `m.store.LoadTable(ctx, tableID)` — a strongly-consistent DynamoDB `BatchGetItem`,
2. `m.leases.Acquire(ctx, tableID)` — a Valkey round trip,
3. `m.roomLoader(tableID)` — another DynamoDB read (`roomstore.Get`).

`m.mu` is process-global. A connection or action to **any** table whose actor isn't already live blocks on the mutex
while an **unrelated** table finishes its cold-start network calls.
`buyin.tableUnavailable` (Issue 29) also calls `GetOrCreateActor` for a *third* unrelated table under the same lock.

**Impacto**
Post-deploy reconnect storm: every client re-resolves its table's actor at once; if DynamoDB is slow or throttling
(Issue 6), the whole instance's actor creation stalls in lockstep. Latent head-of-line-blocking that the 8-player test
could not surface.

**Solução proposta**
Guard the create path per-table: a `singleflight.Group` keyed by `tableID` (or a
`map[string]*sync.Mutex`), so two callers for the *same* table still can't create two actors (T7), but callers for
*different* tables proceed concurrently. Keep a short global lock only for the
`m.actors` map read/write.

**Critérios de aceitação**

- [ ] Concurrent `GetOrCreateActor` for different table IDs do not block each other
- [ ] Concurrent calls for the same table ID still yield exactly one actor (T7 test passes)
- [ ] `roomLoader` / lease acquire happen outside the global map lock
- [ ] Benchmark: N-table cold-start latency under a simulated reconnect storm, before/after

---

### Issue 4 — [BACKEND/RECONCILE]

`cmd/reconcile` swallows per-entry errors — stuck money never reaches the DLQ, no attempt counter

**Module:** `api/cmd/reconcile/main.go` (`run`), `api/internal/reconcile/pending.go`
**Priority:** High · **Effort:** M · **Cost:** $0

**Problema**
`run()` loops over unresolved `poker_pending_cashouts` and, on a per-entry failure, does
`slog.Error("ALARM: ...")` + `continue`, then `return nil`. Consequences:

- The invocation succeeds, so the Scheduler `deadLetterConfig` (which only captures *invocation*
  failures) never fires — **the DLQ provides zero protection for the actual failure mode**.
- No attempt counter on the row, no max-attempts, no terminal "needs-manual-review" state. A permanently-failing entry
  retries every 5 minutes forever, one `ALARM:` log line each time — and `ALARM:` is "something you look for, not
  something that pages you".
- `config.LoadForLambda` does not enforce the `REAL_MONEY_ENABLED`→`LEGAL_SIGNOFF_REF` gate.

**Impacto**
Once real money is on, a stuck cash-out or fee debit is invisible unless someone greps logs, and retries forever with no
escalation.

**Solução proposta**

- Add `Attempts` / `LastAttemptAt` / `LastError` to `PendingCashout`; increment on each failure.
- After N attempts (~10, ~50 min) move the row to `gsi_status = "manual_review"` so it leaves the normal sweep; alarm on
  any row in that state (Issue 2).
- Mirror the real-money legal gate in `config.LoadForLambda`.

**Critérios de aceitação**

- [x] Attempt counter persisted per pending entry — `PendingCashout.Attempts` / `LastAttemptAt` /
  `LastError`, incremented by `PendingStore.RecordFailedAttempt` (#32).
- [x] After N attempts the entry is quarantined and alarmed, not retried — `reconcile.MaxAttempts`
  = 5; `gsi_status` flips to `"manual_review"` (out of `ListUnresolved`), `run` returns an
  aggregated error so the Lambda invocation fails and the message reaches the DLQ. Early-attempt
  failures are counted + `slog.Warn`-logged and retried next run; the whole batch is processed
  before returning so one poison entry never blocks the rest (#32).
- [x] `LoadForLambda` enforces the real-money legal-signoff gate — same
  `RealMoneyEnabled && LegalSignoffRef == ""` fail-closed check as `Load` (#32).
- [x] Test: an entry failing N times ends up quarantined —
  `TestRunEscalatesEntryThatExhaustsRetries` (unit) +
  `TestRecordFailedAttemptQuarantinesAfterMaxAttempts` (integration) (#32).
- [ ] Runbook: inspecting and resolving a quarantined money entry — still open (alarming on the
  `manual_review` state is Issue 2 / CDK work).

---

### Issue 5 — [INFRA/BACKEND] Termination-drain lifecycle hook is best-effort; drain is not idempotent

**Module:** `cdk/lib/api-stack.ts` (`TerminationDrainFunction`), `api/internal/tablemanager` (`DrainAndRelease`)
**Priority:** High · **Effort:** M · **Cost:** $0

**Problema** (the v1 draft's SSM-agent premise was **wrong** — `bin/poker.ts:45` hardcodes
`ENABLE_SSM_AGENT = true`; struck.)
The real, documented gap (`api/CLAUDE.md`, `docs/specs/2026-09-01-duplicate-seat-commit-guard.md`):
during a spot rebalance storm on 2026-09-01 the drain Lambda was invoked for **only 3 of ≥4–5 real terminations**. AWS
does not guarantee the `INSTANCE_TERMINATING` hook fires for every spot reclamation, and the Lambda hard-codes a 55 s
SSM `RunCommand` wait inside a 90 s timeout / 120 s heartbeat — a slow drain is truncated and the `finally` completes
the lifecycle action anyway.
`DrainAndRelease` iterates `m.actors` and calls `m.Release` per table; not obviously safe under concurrent invocation or
partial completion.

**Impacto**
Ungraceful termination strands table leases and in-flight actor state, forcing the conditional-write backstops to do
work documented as *not* something to rely on — the chain behind the triple-seat incident.

**Solução proposta** (all $0)

- The app process itself polls EC2 instance metadata for `spot/instance-action` and
  `autoscaling/target-lifecycle-state` on a short interval and triggers `DrainAndRelease`
  proactively — independent of the lifecycle-hook Lambda.
- Make `DrainAndRelease` idempotent + bounded.
- Widen the Lambda with a `RecordLifecycleActionHeartbeat` loop instead of a fixed 55 s.
- Log "drain completed / skipped"; alarm on "terminated without a confirmed drain" (Issue 2).
- Chaos test: kill an instance mid-hand, assert no stranded lease and no duplicate seat.

**Critérios de aceitação**

- [ ] Drain triggers from in-process spot/lifecycle metadata polling, not only the hook
- [ ] `DrainAndRelease` verified idempotent under concurrent invocation
- [ ] Alarm on "terminated without confirmed drain"
- [ ] Chaos test green
- [ ] `api/CLAUDE.md` / `cdk/CLAUDE.md` Known Issues updated

---

### Issue 6 — [INFRA] Blanket 1000 RCU/WCU on every DynamoDB table + no throttle alarm

**Module:** `cdk/lib/dynamodb-stack.ts` (the `table()` helper)
**Priority:** High · **Effort:** S · **Cost:** ~$0 (an on-demand cap only bills when consumed)

**Problema**
`Billing.onDemand({maxReadRequestUnits: 1000, maxWriteRequestUnits: 1000})` is applied uniformly. For
`poker_table_state`, every committed action is a `TransactWriteItems` of state +
`poker_action_log` entry + idempotency guard (transactional writes bill 2× WCU). With a ~1–2 KB state item a committed
action costs roughly 4–10 WCU across the three tables; `poker_action_log`'s per-hand partition (`tableID#handID`) also
has DynamoDB's hard ~1000 WCU **per-partition** ceiling during a hand. `broadcastAll` can fire a *cascade* of commits (a
fully-preselected street auto-plays as N sequential transactions — Issue 9), plus version-conflict retries. The
`infra.yml`
guard only fails caps `< 100`; a code comment references an "_analysis P0 finding" about exactly this.

**Impacto**
The 8-player test is nowhere near the limit. A dozen active tables during a promo, or the post-deploy reconnect + resync
burst, will throttle `poker_table_state` — surfacing as
`ErrUnavailable`, cascading into more version-conflict retries.

**Solução proposta** (cost-conscious)

- The blanket cap is a legitimate *cost guardrail* — keep it low on genuinely cold tables (purchase history, matchups,
  highlights, hand shares).
- Raise it to a considered ceiling only on hot-path tables (`poker_table_state`,
  `poker_action_log`, `poker_action_guards`, `poker_rooms`, `poker_player_sessions`) — e.g. 2000–4000 — which costs
  nothing until consumed; add a **billing alarm**.
- Add throttle alarms on the hot tables (Issue 2).
- Tighten `infra.yml` to flag a *blanket* low cap on `poker_table_state` specifically.

**Critérios de aceitação**

- [ ] Per-table cap review documented (which tables, ceiling, why)
- [ ] Hot-path tables no longer share the 1000 ceiling
- [ ] Throttle alarms + a billing alarm in place
- [ ] Load test (D6) confirms no throttling at target concurrency

---

### Issue 7 — [INFRA] Single-AZ, spot-only, single-instance-type ASG

**Module:** `cdk/lib/api-stack.ts` (`HaproxyEc2Service`, `spot: {}`, `maxCapacity = min + 1`)
**Priority:** High · **Effort:** S · **Cost:** $0 for type diversification; small for an on-demand base

**Problema**
`minCapacity = 1`, `maxCapacity = 2`, `spot: {}`, one instance type (`t4g.nano`), effectively one AZ. A spot capacity
shortage for that one type in that one AZ zeroes the service with no automatic recovery until capacity returns.

**Impacto**
Correlated, non-self-healing full outage; every connected table drops and relies on the unreliable drain (Issue 5).

**Solução proposta** (cost-conscious)

- **Free:** a mixed-instances policy listing 3–4 equivalent Graviton smalls as spot so a single-type shortage can't zero
  the group; spread the ASG across ≥2 AZs (no cross-AZ data-transfer cost for a stateless tier — DynamoDB and Valkey are
  regional).
- **Optional, a few USD/month:** one on-demand base instance so the floor never hits zero.
- If the decision is "accept the risk while invite-only": write it down, dated, in `cdk/README.md`
  with a "revisit before X" trigger.

**Critérios de aceitação**

- [ ] Mixed-instances policy with ≥3 types (spot)
- [ ] ASG spans ≥2 AZs
- [ ] Decision on an on-demand base recorded (adopt or defer-with-date)
- [ ] CDK test for the mixed-instances / multi-AZ config

---

### Issue 8 — [INFRA/BACKEND] `t4g.nano` (512 MB) vs unbounded in-process actors, timers, and the equity cache

**Module:** `cdk/lib/api-stack.ts`, `api/internal/tablemanager/manager.go`, `api/internal/engine/equity/equity.go`
**Priority:** Medium · **Effort:** M · **Cost:** $0

**Problema**
Two Go app processes + nginx on 512 MB (256 MB swap). On the lease- *holding* instance,
`tablemanager` **never evicts an actor for idleness** — only a lost lease or `Release` drops it
(`evictLeaseLessActorWhenIdle` only runs for lease- *less* actors). So every table ever touched keeps a live goroutine +
an AFK-sweep `time.AfterFunc` firing every 60 s, indefinitely.
`globalEquityCache` is a package-global LRU of 20,000 entries bounded by *count*, not bytes, never cleared when a table
closes.

**Impacto**
OOM-kill of an app process (→ Issue 1 → Issue 5) under a table spike; swap thrashing degrades turn-timer accuracy (the
fuzz suite already needs `-count=15`).

**Solução proposta** (all $0)

- Set `GOMEMLIMIT` from instance memory (÷2 for the two processes).
- Bound `globalEquityCache` by approximate bytes; evict a table's entries on actor teardown.
- Evict lease-holding actors too after a bounded zero-connection idle window (the lease is latency-only; the actor can
  be recreated).
- Memory-pressure log line + alarm (Issue 2).
- Measure RSS-vs-live-tables in D6; only *then* decide whether a `t4g.small` (2 GB, a few USD/mo)
  is warranted.

**Critérios de aceitação**

- [ ] `GOMEMLIMIT` set
- [ ] Equity cache byte-bounded and released on table close
- [ ] Lease-holding actors evicted when idle
- [ ] RSS-vs-tables curve measured; instance-size decision recorded

---

### Issue 9 — [BACKEND/TABLE] `broadcastAll` does heavy per-commit work on the actor goroutine

**Module:** `api/internal/table/actor.go` (`broadcastAll`, `processInlinePreselections`, `processPendingExitAutoFolds`)
**Priority:** Medium · **Effort:** M · **Cost:** $0

**Problema**
`broadcastAll` runs synchronously on the single actor goroutine and, per call:

- `processPendingExitAutoFolds` + `processInlinePreselections` can each loop, calling
  `applyActAndCommit` (a full `TransactWriteItems`) **once per auto-played seat** — a fully-preselected street commits N
  transactions back to back before one broadcast.
- then, per seated viewer: `ViewFor` (rebuilds the whole snapshot), `applyPresence`,
  `applyStreaks`, `applyActivity` (rebuilds the chat slice every time), and — per active player —
  `equity.EstimateWithStats(..., 200)` Monte-Carlo (9× at a full all-in table).

**Impacto**
Latency spikes on the critical path; write amplification feeding Issue 6; CPU/GC pressure on a 512 MB box feeding Issue

8.

**Solução proposta**

- Build the shared snapshot parts (board, pots, chat, reactions, hashes) **once** per broadcast; rebuild only the
  per-viewer parts (hole cards, equity, category).
- `sync.Pool` buffer for the proto marshal.
- Move equity off the synchronous path (a detached goroutine that pushes a delta, or a one-broadcast lag).
- Name the `200` iteration count.
- Consider a small "delta" broadcast for the common single-seat-changed case, full snapshots only on stage transitions /
  connect / `sync_state`.

**Critérios de aceitação**

- [ ] Allocations + wall time per `broadcastAll` measured before/after
- [ ] Shared snapshot parts built once per broadcast
- [ ] Equity off the actor's synchronous path
- [ ] No change to per-viewer visibility (masking tests unaffected)

---

### Issue 10 — [BACKEND/BUYIN] Real-money buy-in skips the poker-terms acceptance check

**Module:** `api/internal/buyin/service.go`, `api/internal/app/app.go`
**Priority:** High · **Effort:** S · **Cost:** $0 · **Blocks real money**

`buyIn()` calls `s.players.RequireAccepted` only `if s.players != nil`. Per `docs/README.md` and
`api/CLAUDE.md` the real-money path does not enforce the poker fair-play addendum the sandbox path enforces. Make it
unconditional on any `currency_mode: "real"` seating. Add a table-level test for accepted / not-accepted /
stale-version × both modes. Remove the `docs/README.md` known-issue entry.

---

### Issue 11 — [BACKEND/WALLETCLIENT] Two `ctech-account` scopes ungranted → wallet verification impossible

**Module:** `api/internal/walletclient` · **Priority:** High · **Effort:** S · **Blocks real money**

From `api/CLAUDE.md` (re-verify against `ctech-account`): `internal:wallet:game-status` has no catalog entry, and
poker's M2M client was never granted `internal:wallet:debit-real`. Both are config changes in `ctech-account`.
`IsGamblingActivated` and `DebitReal` cannot succeed until fixed. Add an integration test / runbook proving both calls
succeed with the poker client token, and cover the grants in deploy reconciliation.

---

### Issue 12 — [BACKEND/MONEY] Real-money correctness gaps: Lambda legal gate, no real-money table cleanup, Claim-race free seat

**Module:** `api/internal/config`, `api/cmd/tablecleanup`, `api/internal/buyin`, `api/internal/entitlement`
**Priority:** High · **Effort:** M · **Cost:** $0 · **Blocks real money**

1. **`config.LoadForLambda` does not enforce `REAL_MONEY_ENABLED`→`LEGAL_SIGNOFF_REF`** the way
   `config.Load` does. `cmd/reconcile` uses `walletclient` (can `DebitReal`/`CashoutGame`). Mirror the gate.
2. **`cmd/tablecleanup` has no real-money cleanup path** — it `continue`s past any non-sandbox room, so a real-money
   table with a stuck seated player's hold is never archived and the hold never released. And "a missing room record is
   treated the same as sandbox" — if sweep ordering ever breaks, a real-money table's players would be `Credit`'d to the
   **sandbox** ledger, violating the load-bearing currency boundary.
3. **Entitlement Claim race**: the loser of `Claim` (`ErrAlreadyClaimed`) returns "covered" and seats the player, while
   the winner's subsequent `DebitReal` may fail (retried async). If reconcile ultimately can't charge, the player got a
   seat for free.

**Critérios de aceitação**

- [ ] `LoadForLambda` enforces the legal gate
- [ ] A real-money stale-table cleanup path exists (release holds, archive), or an explicit documented decision that
  real-money tables are cleaned differently
- [ ] `tablecleanup` never credits a real-money table's stack to any ledger; missing-room →
  "unknown, skip", not "sandbox"
- [ ] Decision recorded on the Claim-race free-seat window (accept as bounded, or gate seating on the fee actually
  clearing)
- [ ] Fold into the D2 deep-dive

---

### Issue 13 — [INFRA/.github] GitHub OIDC roles are over-privileged and not branch-scoped

**Module:** `cdk/lib/oidc-stack.ts`
**Priority:** High · **Effort:** S · **Cost:** $0

**Problema**

- `infraRole` is granted `AdministratorAccess`.
- The trust condition for **every** role is `...:sub` ∈ `[repo:owner/repo:*, repo:owner@*/repo@*:*]`
  — `:*` matches **any branch, any workflow, any environment** (the workflows gate deploy jobs with
  `if: github.event_name != 'pull_request'`, but the IAM trust does not).
- The second pattern `repo:owner@*/repo@*:*` is malformed (the `sub` format is `repo:OWNER/REPO:...`).
- `apiRole` grants `ssm:SendCommand` / `ssm:GetCommandInvocation` on `Resource: '*'` — RCE on any EC2 instance in the
  account — plus `autoscaling:StartInstanceRefresh` / `ec2:*Describe*` on `*`.

**Impacto**
Standard GitHub-OIDC misconfiguration; admin + unscoped `sub` is its highest-severity shape.

**Solução proposta**

- Scope every trust condition to specific refs / environments (`ref:refs/heads/main`, etc.).
- Replace `infraRole`'s `AdministratorAccess` with a scoped policy (CloudFormation on
  `CtechPoker-*` + the specific service actions); at minimum add a permissions boundary + a deny on IAM user/access-key
  creation.
- Scope `apiRole`'s `ssm:SendCommand` to poker's instances by tag.
- Delete the malformed second `sub` pattern.

**Critérios de aceitação**

- [ ] Trust conditions pinned to specific refs/environments, no bare `:*`
- [ ] `infraRole` no longer has unconditional `AdministratorAccess`
- [ ] `apiRole` `ssm:SendCommand` scoped by instance tag
- [ ] Malformed `sub` pattern removed
- [ ] `oidc-stack.ts` gets a CDK test (also closes Issue 21's gap)

---

### Issue 14 — [BACKEND/BUYIN] Refund idempotency key collides across players when `idemKey` is empty

**Module:** `api/internal/buyin/service.go` (`buyIn`, seat-failed refund branch)
**Priority:** High · **Effort:** S · **Cost:** $0

The debit key is `fmt.Sprintf("%s#%s#buyin#%s", roomID, playerID, nonce)` (`nonce` → `playerID`
when `idemKey == ""`). The sandbox seat-race refund uses
`mover.Credit(ctx, playerID, amount, idemKey+":refund", ...)`. If `idemKey` is ever empty that key is the constant
literal `":refund"` for **every** player → `ctech-wallet` dedupes the second such refund → a second player losing the
same seat race is **not refunded**. Fix: derive the refund key from the composite `key` (`key + ":refund"`). Test: two
concurrent `BuyIn` for different players into a 1-seat-left table, both empty `idemKey`, loser fully refunded. Audit
callers for empty-key sites (auto-rebuy sweep, webhooks). Confirm the real-money `ReleaseHold` path (holdID-scoped) is
unaffected.

**Resolved (#42):** the seat-failed refund branch now credits with `key + ":refund"` — `key` already folds in
`roomID`, `playerID` and the nonce (itself `playerID` when `idemKey == ""`), so it is globally unique per refund
while a genuine retry of the same failed buy-in still reproduces it and `ctech-wallet` dedupes. Callers audited: the
only empty-`idemKey` sites are `app.autoRebuySweep` (passes a generated nonce, not empty — but the `key` derivation
is now safe either way) and any future webhook path; the real-money branch uses `ReleaseHold(holdID)` and never
touched `idemKey`. Regression test: `TestBuyInRefundKeyIsPlayerScopedAndCollisionFree`.

---

### Issue 15 — [BACKEND/API] WS rate limiters are per-instance → N× the intended limit fleet-wide

**Module:** `api/internal/api/v1/tablews.go` (`seatLimiter`)
**Priority:** Medium · **Effort:** M · **Cost:** $0

`seatLimiter` (10 actions/sec/seat) and the reaction limiter are in-memory per process. Any instance accepts any table's
connection (ARCHITECTURE §2), so a client spread across instances (or reconnecting) multiplies its allowance. The HTTP
limiters already use Redis (`router.go`); back the WS per-player limiter with the same `RateLimiter.allowRedis` path,
keyed `ws:act:<playerID>`; keep the in-memory path as the dev fallback; fail open on backend error.

---

### Issue 16 — [BACKEND/API] WS upgrade accepts any request with no `Origin` header

**Module:** `api/internal/api/v1/tablews.go` (`wsAllowedOrigin`)
**Priority:** Medium · **Effort:** S · **Cost:** $0

`wsAllowedOrigin` returns `true` when `Origin` is absent. For a game whose threat model is automation, "no Origin =
allow" is the wrong prod default. Where `allowedOrigins` is configured (staging/prod), require a present, listed
`Origin` on the browser-facing sockets; keep "absent allowed" only when `len(allowedOrigins) == 0` (dev). Document that
first-party API automation is not a supported use of the game socket.

---

### Issue 17 — [BACKEND/API] Fixed-window rate limiter can leave a TTL-less key

**Module:** `api/internal/api/v1/ratelimit.go` (`allowRedis`)
**Priority:** Medium · **Effort:** S · **Cost:** $0

`allowRedis` does `INCR`, then `EXPIRE` **only when `n == 1`**. If the process dies or `EXPIRE`
errors between the two, the key persists with no TTL and never resets. Fix: `EXPIRE key <window>
NX` after every `INCR`, or a tiny Lua script. Test: simulate an EXPIRE failure → key still expires on a later hit.

---

### Issue 18 — [FRONTEND/INFRA] `unsafe-inline` in the `script-src` CSP

**Module:** `.github/workflows/frontend.yml` (`csp-overrides`), `frontend-cloudfront.yml`
**Priority:** Medium · **Effort:** M · **Cost:** $0

`csp-overrides` sets `script-src 'self' 'unsafe-inline' https://challenges.cloudflare.com`.
`'unsafe-inline'` defeats CSP's main XSS protection. Next static export emits a bounded set of inline scripts whose
SHA-256 hashes can be enumerated at build time. Move to
`script-src 'self' 'sha256-…' https://challenges.cloudflare.com` generated by the build. Review
`style-src` at the same time.

---

### Issue 19 — [INFRA/.github] Dead/duplicate `frontend-cloudfront.yml` workflow

**Module:** `.github/workflows/frontend-cloudfront.yml`
**Priority:** Medium · **Effort:** S · **Cost:** $0

`frontend.yml` (Cloudflare, wired into `deploy.yml`) and `frontend-cloudfront.yml` (S3 + CloudFront, wired into nothing)
both exist, both named "Frontend". `frontend-cloudfront.yml` still has a `pull_request` trigger on `ui/**`, so **every
UI PR runs a second full test + coverage job**
from the retired pipeline, and its deploy job references a CloudFront distribution being torn down. Delete it; if a step
(route-manifest publish) is still needed transitionally, fold it into
`frontend.yml` with a documented cutover date.

---

### Issue 20 — [INFRA/.github] No SAST, secret-scanning, or dependency-review in CI

**Module:** `.github/workflows/`
**Priority:** High · **Effort:** S · **Cost:** $0 (GitHub-native)

CI runs `go vet`, `staticcheck`, `govulncheck`, `tsc`, `eslint`, tests. Missing: CodeQL (Go + JS/TS),
`actions/dependency-review-action` on PRs, repo secret-scanning + push protection, any frontend dependency-vuln scan
(`npm audit` / `osv-scanner` is not run). Given the money path and OAuth handling, add all four (free on this repo) plus
a `SECURITY.md`.

---

### Issue 21 — [INFRA/.github] Unpinned CI tools; two CDK stacks untested

**Module:** `.github/workflows/api.yml`, `cdk/`
**Priority:** Medium · **Effort:** S · **Cost:** $0

`go run honnef.co/go/tools/cmd/staticcheck@latest` and `.../govulncheck@latest` resolve at CI time — a new release can
break `main` with no code change, plus a small supply-chain surface. Pin both. Add CDK tests for `reconcile-stack.ts`
and `oidc-stack.ts` (`docs/README.md` notes both are untested; Issues 2/13 give natural reasons to add them).

---

### Issue 22 — [FRONTEND/TABLE] `useTableRealtime.ts` (1086 L) — decompose the resilience state machine

**Module:** `ui/src/lib/hooks/useTableRealtime.ts`
**Priority:** Medium · **Effort:** L · **Cost:** $0

Read in full: the complexity is largely inherent (socket lifecycle, snapshot reconciliation, optimistic preview,
`pendingActionRef`/`auxFramesRef` retry+backoff, keyed resync watchdogs, mute/block suppression, ~40 refs). Correct and
well-tested (90% enforced) but a maintenance risk. Behaviour-preserving extraction:

- `useTableSocket` — connect / auth frame / reconnect / liveness.
- `useTableActionQueue` — pending/aux frame tracking, retry/backoff, resync classification.
- `useTableSnapshotReducer` — pure snapshot → view-state.
- suppression as a pure filter at the reducer boundary. Keep `useTableRealtime` as the thin composition; coverage
  stays ≥ 90 without threshold edits.

---

### Issue 23 — [BACKEND/TABLE] Cache-rollback obligation is convention-only across mutating handlers

**Module:** `api/internal/table/actor.go`
**Priority:** High · **Effort:** L · **Cost:** $0

`api/CLAUDE.md`: "Every `a.cached`-mutating handler must snapshot and roll back on any commit failure." **Missing from
`applyReadyAndCommit` until 2026-09-01** → the triple-seat incident.
`applyJoinAndCommit` / `applyLeaveAndCommit` / `handleReady` / `handleTurnTimeout` /
`handleNextHand` / `handleRunoutStep` / `handleRequestWinnerCards` each implement the snapshot-and-restore dance inline,
slightly differently. Introduce one helper —
`a.mutate(ctx, actionID, entry, func(t *hand.Table) error {…})` — that snapshots
`a.cached.ExportState()` + `a.handID`, runs the mutation, commits, and on **any** error restores both. Convert every
`apply*AndCommit`. Test: inject a commit failure per handler, assert `a.cached`
byte-identical to before. Add a lint/vet note against a raw `a.cached` mutation + `a.commit`
outside the helper.

---

### Issue 24 — [BACKEND/TABLE] `actor.go` (2838 L) — decompose

**Module:** `api/internal/table/actor.go`
**Priority:** Medium · **Effort:** XL · **Cost:** $0

One file, ~130 methods, one struct carrying 6 timer subsystems + `pending*Deadline` carry-overs + presence maps + streak
maps + connection maps + fleet-sync state + ~20 `SetXForActor` injectors. Incremental, behaviour-preserving extraction:
a `timers` sub-component (table-driven-tested for the
"reload resumes remaining time" invariants); a `presence` type; a `Hooks` struct passed at construction. Target
`actor.go` ≤ ~900 L. Do Issue 23's `mutate` helper first. Fuzz + integration +
`-count=15` stress must stay green.

---

### Issue 25 — [FRONTEND] No client-side error telemetry

**Module:** `ui/`
**Priority:** Medium · **Effort:** S · **Cost:** $0

`app/error.tsx` / `app/global-error.tsx` exist as boundaries; nothing reports. For a product in active testing, an
uncaught client error or a boundary trip is invisible to the team. Add a minimal beacon: `window.onerror` /
`onunhandledrejection` + the boundaries `POST` a compact
`{message, stack (trimmed), route, release}` to a new rate-limited `POST /v1.0/client-errors`
endpoint (no PII, no snapshot); log it structured to `/ctech-poker/<env>/app`. No SaaS needed.

---

### Issue 26 — [BACKEND+FRONTEND] Dead code and silent error swallows

**Priority:** Low · **Effort:** S · **Cost:** $0

- `actor.go`: `saveHandHistorySnapshot` — `if err := …; err != nil {}` (empty body); the comment says "emits a metric"
  but nothing is emitted — a history-save failure is **completely silent**. Add a `slog.Warn`.
- `actor.go` `handleAct`: several empty `if errors.Is(err, tablestore.ErrVersionConflict) {}` blocks.
- `actor.go` `Dispatch`: `if len(a.cmds) >= (cap(a.cmds)*3)/4 { /* comment only */ }` — dead.
- `deck.go`: `commitHash` ("legacy global commit hash") — confirm every consumer of
  `ShuffleResult.CommitHash` / `Snapshot.ShuffleCommitHash`; remove or document as compat-only + pin with a test.
- `useTableRealtime.ts`: `if (message.type === 'equity' …)` — the backend puts equity in the snapshot seat, never sends
  a standalone `equity` message. Dead branch.
- Protocol version is an inline literal (`ProtocolVersion: 11`, "v6" in comments) — name it, add a one-place version
  history + minimum-supported-client note.

---

### Issue 27 — [BACKEND/ARCHIVER] Audit archive renders numeric attributes as float64

**Module:** `api/cmd/archiver/main.go` (`attributeValueToInterface`, `DataTypeNumber`)
**Priority:** Low · **Effort:** S · **Cost:** $0

`strconv.ParseFloat(v.Number(), 64)` — for the **permanent audit archive** of every action (including chip amounts and
payouts), storing money as float64 is a latent precision bug past 2^53. Emit numbers as JSON strings (or `json.Number`)
so the archive is exact.

---

### Issue 28 — [INFRA] Over-broad IAM on the Lambda roles

**Module:** `cdk/lib/reconcile-stack.ts` (+ overlaps Issue 13 for `oidc-stack`)
**Priority:** Low · **Effort:** S · **Cost:** $0

`reconcile-stack.ts` grants `dynamodb:Scan` on the pending table but the code only `Query`s the
`gsi_status` GSI — drop `Scan`.

---

### Issue 29 — [BACKEND/BUYIN] `tableUnavailable` spins up a full Actor to count seats

**Module:** `api/internal/buyin/service.go` (`tableUnavailable`)
**Priority:** Low · **Effort:** S · **Cost:** $0

The entitlement-rebind path calls `s.manager.GetOrCreateActor` for an **unrelated** bound table just to read its seat
count — creating a goroutine + 6 timers + a lease-acquire attempt (and, under Issue 3, blocking the global manager
mutex), then leaving that actor to linger ~5 min. Read the `poker_rooms.seats_taken` write-through field instead (it
already mirrors occupancy for the lobby).

---

### Issue 30 — [BACKEND/TABLE] `ReconnectCmd` dispatched to the actor on every inbound WS frame

**Module:** `api/internal/api/v1/tablews.go` (read loop, ~line 478)
**Priority:** Medium · **Effort:** S · **Cost:** $0

The table read loop dispatches `table.ReconnectCmd` to the actor **before every message**, including `ping`.
`handleReconnect` runs `ensureLoaded` + only broadcasts if a disconnect mark was cleared — but the dispatch is a channel
send + actor-goroutine turn per frame, per seat, on the same goroutine that runs `broadcastAll`. Only signal reconnect
on an actual active-after-gap transition, or debounce to at most once per N seconds per connection; `ping` must never
trigger it.

---

### Issue 31 — [FRONTEND] `set-state-in-effect` suppressions and `any` casts

**Module:** `ui/src/`
**Priority:** Low · **Effort:** M · **Cost:** $0

~10 `// eslint-disable-next-line react-hooks/set-state-in-effect` across `table/page.tsx`,
`HandOutcome.tsx`, `Chat.tsx`, `TableReactions.tsx`, `RealityCheck.tsx` — a recurring pattern that usually indicates
derived state better expressed as `useMemo` / computed during render. ~93 `any` /
`as any` outside tests (some unavoidable proto/library glue; the generated proto file is
`/* eslint-disable */` and fine). A cleanup pass for the genuine cases.

---

### Issue 32 — [FRONTEND] No a11y gate, no bundle-size budget, no Lighthouse CI

**Module:** `ui/`, `.github/workflows/frontend.yml`
**Priority:** Medium · **Effort:** M · **Cost:** $0

`ui/CLAUDE.md` documents strong a11y intent and the frontend is genuinely polished, but nothing automated enforces it
and "experiência do usuário em conexões lentas" is unmeasured. Add:
`jest-axe` assertions in the render tests for the top routes + recovery components; a `next build`
first-load-JS budget per route (fail CI on regression); Lighthouse CI against the static export on a throttled profile.
Confirm `mockRuntime` / `MockControls` (aliased out in prod via
`next.config.ts`) are actually absent from the prod bundle.

---

### Deep-dive tickets

**D1 — [BACKEND/ENGINE] `hand.go` settlement audit.** `betting.go` / `sidepots.go` were read closely and look correct.
`hand.go`'s `runShowdown` was read but the multi-way interactions need a written checklist against `OVERVIEW.md` §3.3:
HU blind posting across button rotation; BB option pre-flop; short all-in not reopening multi-way at multiple positions;
3+ simultaneous all-ins at distinct stacks **with** a folded partial bet (`ComputeSidePots` integration, not just its
unit); the `runShowdown` `len(winners) == 0` refund branch (splits back to contributors — **contradicts the
`folded-money-is-dead-money` ruling** if reached; confirm unreachable given `sidepots` never puts a folded player in
`Eligible`, or fix); odd chip → seat left of the button across multiple layers; uncalled over-shove returned before
rake; run-it-twice halving + `BoardSplitAt` with side pots; `wasEverAllIn` bookkeeping; `EscalateBlindsForActor` integer
drift. Every scenario needs a named regression test. Re-run `-tags exhaustive`.

**D2 — [BACKEND/MONEY] `reconcile` + `entitlement` concurrency audit.** Targeted tests: caller and sweeper both
processing the same pending row; `Claim` racing `Claim` for `(player, originTable)`;
`Rebind` racing a buy-in at the origin table; `DebitReal` failure + retry exactly once; confirm unresolved
`poker_pending_cashouts` rows never get a TTL on the write path. Blocks real money. Fold in Issue 12.

**D3 — [BACKEND/GAMIFICATION] fairness + abuse audit.** `dailyreward`: CSPRNG (confirmed
`rand.Read` at `service.go:122`) + per-day idempotency across a TZ boundary + concurrent claims + distribution matches
the specced table. `achievements`: no double-count (under the `handhook`
`SET NX` guard?); "blefe"/"comeback" notifications never leak the folded opponent's cards.
`leaderboard`: min-hands floor on rate metrics; no other unsupported metric falls through.

**D4 — [BACKEND] peripheral module + handler sweep.** 28 `internal/*` modules and 29 `api/v1/*.go`
handlers were only grep-reviewed this pass. A focused read of each before relying on "clean".

**D5 — [FRONTEND] component sweep.** `app/table/page.tsx` (824 L), `ActionBar` (516 L), `Seat`,
`TableStage`, lobby + store components read only via the hooks they consume this pass.

**D6 — [BACKEND/INFRA] load / soak test.** Validated at 8 players / 1 table. Define a target concurrency profile with
the product owner (e.g. 20–50 tables, 150–400 sockets), run the
`-tags load` harness against a prod-like stack capturing: DynamoDB throttles per table, RSS + CPU credits per instance,
actor command-queue depth, p50/p95/p99 commit + broadcast latency, Valkey latency; and behaviour during a rolling deploy
and a simulated spot kill mid-test. Feeds Issues 6/7/8. Re-run gate before real money.

---

## 4. Label taxonomy to create (`gh label create`)

**Subsystem:** `backend` `#1D76DB` · `frontend` `#0E8A16` · `infra` `#5319E7`
**Module:** `module:engine` `#B60205` · `module:table` `#D93F0B` · `module:tablemanager` `#D93F0B`
· `module:buyin` `#FBCA04` · `module:reconcile` `#FBCA04` · `module:entitlement` `#FBCA04` ·
`module:api` `#0052CC` · `module:gamification` `#C2E0C6` · `module:botcheck` `#BFDADC`
**Type:** `type:security` `#B60205` · `type:bug` `#D73A4A` · `type:correctness` `#B60205` ·
`type:reliability` `#D93F0B` · `type:performance` `#FBCA04` · `type:tech-debt` `#CFD3D7` ·
`type:testing` `#BFD4F2`
**Priority:** `priority:high` `#B60205` · `priority:medium` `#FBCA04` · `priority:low` `#0E8A16`
**Effort:** `effort:S` `#EDEDED` · `effort:M` `#D4C5F9` · `effort:L` `#C5DEF5` · `effort:XL` `#5319E7`
**Meta:** `needs-deep-dive` `#E99695` · `real-money-blocker` `#B60205` · `playtest-followup` `#0E8A16`
· `zero-cost` `#0E8A16`

---

## 5. Strategic recommendations

1. **This week, all $0:** Issues 1, 14, 17, 19, 20, 26, 35 (win-rate floor is a one-line GSI guard).
2. **Before any audience beyond the invite playtest:** Issues 1, 2, 3, 5, 6, **33** (hand-completion freeze) and the D6
   load test. Issue 33 is the single biggest "works at 8, melts at 800" risk.
3. **Leaderboard is the clearest OK-vs-impeccable gap** (§7.3): Issues 34, 35, 36 together turn a top-50 vanity board
   into something every player has a reason to open. All $0 (Valkey ZSET is already deployed).
4. **Before flipping `REAL_MONEY_ENABLED` for real users** (independent of the §11 legal opinion):
   Issues 10, 11, 12, D1, D2. Tag `real-money-blocker`.
5. **Highest engineering leverage:** Issues 23 + 24.
6. **Security posture, all $0:** Issues 13, 20, 16, 18.
7. **Frontend is in good shape** — its issues (22, 25, 31, 32, 39) are quality/observability investments. Prioritise 25
   and 32 (measurable) over 22, 31 and 39.
8. **Keep the low-cost infra mindset — write the trade-offs down.** "Acceptable for a playtest" is a valid answer; it
   belongs as a dated note with a revisit trigger in `cdk/README.md`.

---

## 6. Proposed filing order

Security/crash → correctness → reliability → performance → tech-debt → testing:

1, 13, 10, 11, 14, 16, 20 · 4, 12, 23, 34, 35, 38, 36, 26, 27, 17 · 2, 5, 3, 7, 6, 8, 33, 25, 30 · 9, 15, 37, 39, 28,
29, 40 · 18, 19, 21, 24, 22, 31 · 32 · D1, D2, D6.

---

## 7. D4 / D5 deep-dive findings (this pass) — gamification, player data, leaderboard, name drift

D4 (peripheral backend) and D5 (frontend components) were run before filing, at your request. Read in full this pass:
`leaderboard/{service,store}.go`, `pokerstats/*`, `achievements/service.go`,
`dailyreward/service.go`, `player/{service,store}.go`, `sessionlog/store.go`, `matchup/store.go`,
`handshare/store.go`, `handreveal/service.go`, `social/{model,event_store}.go`,
`recentplayers/*`, `app/app.go`'s `onHandComplete`, and the frontend `leaderboard/page.tsx`,
`table/page.tsx`, `lib/api/{player,leaderboard}.ts`.

### 7.1 Direct answers to your questions

**"Will the leaderboard work with 100K players?"** — Not as built. Three separate walls:

1. **Write path is a global hotspot.** Every leaderboard GSI (`gsi_hands_won`, `gsi_hands_played`,
   `gsi_win_rate`) has a partition key of `gsi_*_pk = <mode>` — so *every* row for a mode shares **one physical
   partition**, hard-capped by DynamoDB at ~1000 WCU / ~3000 RCU **regardless of table capacity**. At 100K active
   players, every completed hand's `IncrementStats` (one
   `UpdateItem` with `ReturnValues: ALL_NEW`) plus `materializeWinRate` (a conditional `UpdateItem`
   with up to **5 retries, each doing a `GetItem`**) all funnel through that one partition. It throttles,
   `materializeWinRate` spins to its "remained contended" error, and the error propagates up through the synchronous
   `onHandComplete` (Issue 33).

2. **No "my rank".** There is no `GET /leaderboard/me` and no `RankOf(playerID)` in the service.
   `GET /leaderboard` returns only `Top(limit≤100)`. The frontend (`leaderboard/page.tsx`) fetches **one page** (default
   50, no cursor), then:
   ```
   const viewerEntry = data.find(p => p.player_id === viewer);
   {viewerEntry && <div>#{data.findIndex(...) + 1} de {data.length} jogadores</div>}
   ```
   So a player ranked #4,231 sees **nothing about themselves**, and a player ranked #12 sees *"#12 de 50"* — the page
   size, not the 100K total. For 99.95% of players the leaderboard has zero personal relevance.

3. **`win_rate` is meaningless / gameable.** The GSI sorts by the stored `win_rate_score`, which has **no min-hands
   floor** — an account that plays exactly one hand and wins it has
   `win_rate_score = 1.0` and tops the board. The service re-sorts the fetched page in memory but can't fix the fact
   that the GSI already returned the wrong 100 rows. A genuine grinder at 58% over 20K hands is invisible.

**"Will the player be able to check his position on the ranking?"** — No, unless they happen to be in the top ~50. →
**Issue 34.**

**"When the user is placed at the ranking will his username be updated too? … with NoSQL the changes are not
synchronized between tables."** — You are exactly right, and it's worse than just the leaderboard:

- `player.SetName` writes **only** `poker_player_profiles`. There is **no fan-out** anywhere.
- The leaderboard row's `player_name` is a denormalized copy, refreshed *only* by
  `IncrementStats(playerID, names[id], ...)` on the **next completed hand** — and `names[id]` is the name the **table
  actor cached at join time** (`SetIdentityCmd`, dispatched on WS connect / buy-in), never re-read mid-session. So:
    - Rename, then play a hand at a new table → leaderboard catches up.
    - Rename while seated → your opponents keep seeing the old name until you reconnect (no live push), and the
      leaderboard stays stale until your next hand.
    - **Rename and stop playing → leaderboard shows the old name forever.**
- The same frozen-name problem hits **`poker_player_hands`** (`OpponentSummary.Name` +
  `AvatarURL`, permanent, no TTL) and — worse — **`poker_hand_shares`** (`ReplaySeat.Name` in the replay frames,
  **public**, ≤30-day TTL). A player who shares a hand and later renames, or whose *opponent* renames (for privacy),
  leaves the old name publicly visible on the share link. The
  `Opponent.Alias` field is already anonymized ("Player 2"); `ReplaySeat.Name` is not.
- `social` events and `matchup` correctly store **only IDs** and resolve names at read time — that is the pattern the
  rest should follow. `player.Service.GetMany` (batch `BatchGetItem`) **already exists** and is unused by the
  leaderboard.

→ **Issue 36.**

### 7.2 The biggest backend finding this pass — Issue 33

`onHandComplete` (in `app/app.go`, ~lines 523–598) is **one synchronous function** that runs, in sequence, on **the
table's actor goroutine** (called from `notifyHandComplete` → `broadcastAll` →
`handleAct`, no `go`, `context.Background()` so uncancellable):

| Step                                                | Cost                                                                                                                                                                                                                        |
|-----------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `tableCurrencyMode` (room lookup)                   | 1 read                                                                                                                                                                                                                      |
| `LoadActionsSince` (whole hand's action log)        | 1 query, unbounded page                                                                                                                                                                                                     |
| `achv.RecordHand`                                   | **~50–150 sequential `UpdateItem` (atomic ADD)** — per-participant `KeyHandsPlayed`, per-winner ~15 keys, per-all-in, per-showdown-result ~10 keys each, plus 1–2 Valkey round trips/participant for the pocket-pair streak |
| `leaderboardSvc.RecordUnlocks`                      | 1 `AtomicIncrement` **per star**                                                                                                                                                                                            |
| `leaderboardSvc.RecordHand`                         | per participant: 1 `UpdateItem` + `materializeWinRate` (1 conditional `UpdateItem` + up to 5×`GetItem`)                                                                                                                     |
| `pokerStatsStore.RecordHand`                        | writes                                                                                                                                                                                                                      |
| `matchupStore.RecordHand`                           | **one `TransactWriteItems` of C(9,2)×2 = 72 items** (Issue 37)                                                                                                                                                              |
| `persistHandHistory`                                | 1 `PutItem` **per participant** (up to 9)                                                                                                                                                                                   |
| `persistHandReveal`, `highlightsStore`, `recentSvc` | more writes                                                                                                                                                                                                                 |

At a full table that is **hundreds of sequential DynamoDB round trips per hand**, all blocking the actor. During that
window no one at the table can act, chat, `show_cards`, or start the next hand; the winning player's `action_ack` is
withheld until it all finishes. At 8 players / 1 table against DynamoDB Local (no latency, no throttling) it's
invisible; at real scale it makes **every hand end with a multi-second table freeze**, and under the Issue 6 throttle
ceiling it compounds.

`autoRebuySweep` is *already* detached to a worker goroutine (`app.go:812` comment) — the pattern exists, it just isn't
applied to the main hook. The fix: `notifyHandComplete` dispatches the immutable `hookOutcome` to a bounded worker
pool / channel; the `handhook` `SET NX` claim already dedupes it fleet-wide. Inside `RecordHand`, the 50–150 sequential
increments should also be batched (`TransactWriteItems`, ≤100/tx) or run under a bounded `errgroup`.

### 7.3 What "OK" vs "impeccable" looks like — leaderboard (the concrete example you asked for)

| Dimension              | Current ("OK", and barely)                                                     | Impeccable                                                                                                                                                                                                                                                                |
|------------------------|--------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Your position**      | Only if you're in the top ~50; shows "#N de 50"                                | `GET /leaderboard/me` → `{rank, total_qualified, delta_since_last_week, neighbors: [±3 rows]}`, always available, backed by a **Valkey sorted set** (`ZREVRANK` = O(log N), exact, and Valkey is already deployed). The DynamoDB GSI stays as the durable rebuild source. |
| **Sticky "you" band**  | none                                                                           | Top 10/25 list + a pinned card "You're #8,241 of 47,900 — 3 above / 3 below" that scrolls with you                                                                                                                                                                        |
| **win_rate integrity** | 1-hand 100% accounts on top                                                    | Only write `gsi_win_rate_pk` once `hands_played >= N` (e.g. 100); each row shows "(2,104 mãos)" so it reads as legitimate; a "provisional" tag below the threshold                                                                                                        |
| **Name freshness**     | frozen at last completed hand                                                  | Resolved at read time via `player.Service.GetMany` for the visible page (one `BatchGetItem`, ~free) — no denormalized `player_name` at all                                                                                                                                |
| **Ties**               | silently ordered by `player_id`                                                | Shown as "T-4", same score visibly grouped                                                                                                                                                                                                                                |
| **Empty state**        | "Nenhum jogador pontuou ainda" even when 100K exist and you're just not top-50 | "Você está em #8,241. Jogue mais 40 mãos para entrar no top 1%."                                                                                                                                                                                                          |
| **Segmentation**       | sandbox/real tabs                                                              | plus all-time / this-season toggle (the achievement-seasons spec already exists)                                                                                                                                                                                          |
| **Write path**         | synchronous on the actor goroutine, single-partition GSI                       | async worker; Valkey-first with DynamoDB as async durable mirror; per-mode sharding of the GSI PK (`mode#<bucket>` with a scatter-gather Top) if the single partition is ever the bottleneck                                                                              |
| **Movement feedback**  | none                                                                           | "Você subiu 340 posições esta semana" from a weekly snapshot diff                                                                                                                                                                                                         |

### 7.4 Other D4 findings (smaller)

- **dailyreward** is **sound.** CSPRNG tier pick (`rand.Read`), cumulative-weight walk sums to exactly 100, `Claim`
  -before-`Credit` with a stable per-day idempotency key makes concurrent spins safe, first-reward override is
  race-safe. Only nit: `roll % 100` has an immeasurable (~1e-18) modulo bias — not worth fixing. **No issue filed.**
  (One thing to verify in code review:
  that `spinStore.Claim` is a genuine conditional write — the flow is only safe if it is.)
- **achievements double-count (Issue 38).** `claimHandHooks()` **fails open** on a Valkey error (deliberate: "a double
  credit is at least visible and bounded"). But `RecordHand`'s `bump()` →
  `store.Increment` is an atomic `ADD`, **not idempotent**, so a Valkey blip during a hand completion → two instances
  both run it → that hand's achievement counters *and* the leaderboard
  `hands_played`/`hands_won` are permanently inflated for every participant. `poker_player_hands`
  (`PutItem` overwrite) and `matchup` (per-pair create-only guard) are idempotent and safe; the ADD-based counters are
  not. Fix: a `(table_id, hand_id)` conditional guard write at the top of the detached `onHandComplete` worker, or per-
  `bump` idempotency keys.
- **`pokerstats`** — materialised VPIP/PFR/3-bet with a per-hand guard row; looked correct, uses
  `BettingAction` to distinguish all-in call vs raise. Fine.
- **`handreveal`** (non-consensual paid reveal of a mucked winner's cards, in *history*) — debit buyer / credit winner
  half / record, idempotent per `(handID, buyerID)` with stable wallet keys. The privacy model is deliberate (the hand
  is archived, no winner still at the table to ask) and documented. One thought for the product owner, not a defect: the
  *winner* has no way to opt their archived hands out of ever being revealed this way.

### 7.5 D5 findings

- **`table/page.tsx` (Issue 39)** — ~824 lines, ~30 `useState`, ~15 `useEffect`, 8+ TanStack queries (room, seated,
  hands, sessions, me, reaction catalog + purchases, notes, relationships), plus `useTableRealtime`, `useDealerVoice`,
  `useTablePreferences`, `useSocialActions`. A god component. It also drives a **1 Hz `setNow(Date.now())` that
  re-renders the whole tree** while a turn timer is active — the countdown ring should own its own ticker (the `Seat`
  code comment already implies it can be a pure function of `deadlineMs − snapshotAt`). Decompose into
  `useTableSession`, `useTableOutcome`, `useTableSocialOverlay`, and isolate the ticker.
- **`leaderboard/page.tsx`** — see §7.1; the "#N de {data.length}" bug is the headline.
- **`avatar_url` frozen in hand history (Issue 40)** — `sessionlog.OpponentSummary.AvatarURL` is captured at
  hand-complete. After `ClearAvatar` the stored URL 404s → broken image in the opponent list. Store `player_id` only and
  resolve `avatar_url` at read time (same fix as the name), or serve a fallback on 404.

### 7.6 New detailed issues (33–40)

**Issue 33 — [BACKEND/TABLE] `onHandComplete` runs the full gamification pipeline on the actor goroutine**
*High · M · $0.* See §7.2. Detach `notifyHandComplete`'s `onHandComplete` call to a bounded worker pool consuming the
immutable `hookOutcome`; the `handhook` `SET NX` claim already dedupes fleet-wide. Batch the ~50–150 sequential
increments inside `achievements.RecordHand` (`TransactWriteItems` ≤100, or a bounded `errgroup`). Acceptance:
hand-completion `action_ack` latency measured before/after; the actor goroutine is never blocked on gamification I/O; a
slow/throttled leaderboard write cannot delay the next hand.

**Issue 34 — [BACKEND+FRONTEND/LEADERBOARD] No "my rank"; frontend ranks within one fetched page; single-partition GSI**
*High · M · $0.* Add `GET /v1.0/leaderboard/me?mode=&metric=` → `{rank, total_qualified, entry,
neighbors:[±3]}`. Back rank/neighbors with a Valkey sorted set per `(mode, metric)` mirrored from the same event that
writes `IncrementStats` (`ZADD` / `ZREVRANK` / `ZRANGE`); the GSI stays the durable rebuild source. Frontend: pinned
"your position" band + top list; stop computing rank from
`data.findIndex` / `data.length`. Acceptance: a player ranked #10,000 sees their exact rank and neighbours; leaderboard
reads/writes don't all hit one partition.

**Issue 35 — [BACKEND/LEADERBOARD] `win_rate` is gameable — no min-hands floor**
*High · S · $0.* Only set `gsi_win_rate_pk` once `hands_played >= MinHandsLeaderboard` (e.g. 100);
`REMOVE` it below the threshold so sub-threshold rows are sparse-excluded from the ranking query. Return `hands_played`
alongside `win_rate` and render "(N mãos)". Acceptance: a 1-hand 100% account does not appear on the win-rate board; a
20K-hand 58% grinder does.

**Issue 36 — [BACKEND/PLAYER] Display-name changes never propagate to denormalized copies**
*Medium · M · $0 (+ privacy).* `player.SetName` writes only `poker_player_profiles`. Choose one:
(a) **read-time resolution** — drop `player_name` from leaderboard rows and `name`/`avatar_url`
from `poker_player_hands` opponent summaries; resolve via `player.Service.GetMany` on the visible page (one
`BatchGetItem`); or (b) **fan-out on rename** — update the (bounded) leaderboard row + recent hand rows + push
`SetIdentityCmd` to any table where the player is currently seated. For **public hand shares**: anonymize
`handshare.ReplaySeat.Name` for non-owner opponents the same way
`Opponent.Alias` already is — a renamed/opted-out opponent's old name must not stay public. Acceptance: rename then
check leaderboard / a past hand / a share link → new name (or a neutral alias for opponents) everywhere; a seated
player's rename reaches their table without a reconnect.

**Issue 37 — [BACKEND/MATCHUP] 72-item transaction per completed hand**
*Medium · M · $0.* `RecordHand` writes C (9,2)=36 pairs × (guard + update) = 72 items in one
`TransactWriteItems` — 72/100 of DynamoDB's hard limit, ~144 WCU on a 1000-cap table per hand (~7 hands/s ceiling
service-wide for matchup alone), and synchronous in `onHandComplete`. Split across multiple transactions, or make
matchup aggregation fully async/eventual off a queue, or cap which pairs are tracked (e.g. only pairs that reached
showdown together). Acceptance: no single transaction over ~25 items; matchup writes off the hand-completion critical
path (folds into Issue 33).

**Issue 38 — [BACKEND/GAMIFICATION] Counters double-count on a Valkey blip**
*Medium · S · $0.* See §7.4. Add a `(table_id, hand_id)` conditional guard write at the top of the (detached)
`onHandComplete` worker so a fail-open `claimHandHooks` can't let two instances both run the non-idempotent `Increment`/
`IncrementStats` ADDs. Acceptance: simulate a Valkey outage during a hand completion on two instances → each
participant's counters advance exactly once.

**Issue 39 — [FRONTEND/TABLE] `table/page.tsx` god component + 1 Hz full re-render**
*Medium · L · $0.* See §7.5. Extract `useTableSession` / `useTableOutcome` / `useTableSocialOverlay`; move the countdown
ticker into an isolated component that reads `deadlineMs − snapshotAt`. Coverage stays ≥ 90 without threshold edits.

**Issue 40 — [BACKEND/SESSIONLOG] Frozen `avatar_url` in hand-history opponent summaries**
*Low · S · $0.* `OpponentSummary.AvatarURL` is captured at write time and 404s after `ClearAvatar`. Store `player_id`
only; resolve `avatar_url` at read time (same fix as Issue 36), or fall back to the default avatar on a broken image.

---

## 8. Per-module frontend review (4 parallel agents) — findings & Issues 41–92

Four fresh, code-anchored reviews were run in parallel, one per frontend area, each producing an **OK-vs-impeccable
table per screen** plus new issues (frontend *and* backend gaps the frontend exposes). Full tables + acceptance criteria
live in
`docs/plans/2026-09-02-frontend-module-review/{01-table-experience,02-lobby-store-wallet,03-gamification-hands-social,04-auth-shell-content-perf}.md`.
Consolidated below.

### 8.1 Overall verdict

The frontend is **genuinely strong** — reduced-motion discipline, keyboard coverage, optimistic feel, explicit
error/empty/loading branches, careful focus management, a well-architected liveness/outage layer, and a thorough
server-side anonymization path for public hand shares. The gaps are specific, not systemic. The five that matter most:

1. **`table/page.tsx`'s `handOutcome` is set every hand and never cleared** → from hand 2 onward, achievement toasts are
   silently suppressed and the pay-to-see-mucked-winner offer is unreachable. (Issue 50)
2. **Cosmetic (deck/felt) PIX purchases have no webhook confirmation** — the service method exists but is never wired to
   the webhook route; buy a deck via PIX, close the dialog, and it stays *paid but not granted and not announced*.
   (Issue 41)
3. **Achievements progress is computed over one page of counters** → understated mastery, permanently-hidden secret
   achievements, an incomplete showcase picker. (Issues 43 backend / 51 frontend)
4. **Marketing/SEO pages (`/poker-rules`, `/guide/*`) ship the entire authenticated-app bundle**
   (~1 MB JS incl. protobuf + WebSocket client) on a static text page. (Issue 52)
5. **The lobby's "join vs create" decision only sees the first page of `/rooms`** → it spawns a duplicate table while a
   joinable one exists on page 2. (Issues 48 backend / 62 frontend)

### 8.2 Backend issues surfaced by the frontend review

| #  | Title                                                                                                                                                                                                                                                                                                      | Module                 | Type        | Prio       | Effort | Cost |
|----|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------|-------------|------------|--------|------|
| 41 | Cosmetic (deck/felt) PIX purchases have no webhook confirmation or realtime push — `walletwebhook.go:27` never wires `cosmeticpurchase.ConfirmFromWebhook`; no `cosmetic_purchase_update` frame; the dialog polls only while open. Paid-but-ungranted, silent.                                             | cosmeticpurchase / api | bug         | **High**   | S–M    | $0   |
| 42 | Session `buyin_amount` must be **cumulative** across rebuys/auto-rebuys or `RealityCheck` + `SessionRecap` overstate the player's winnings (responsible-gaming). Client can't fix it.                                                                                                                      | sessionlog             | correctness | **Medium** | S      | $0   |
| 43 | `GET /players/me/achievements` returns one page → every derived stat (completion %, next star, secret-unlock gate) is truncated. Add `/achievements/me/summary` (every non-zero key, Valkey hash) or make the list fully cursorable.                                                                       | achievements           | bug         | **High**   | S      | $0   |
| 44 | No `unlocked_at` / `last_tier_at` on achievement progress → the achievements page has no "recently unlocked" moment, no recency sort, no celebration. Stamp it when `RecordHand` crosses a tier (idempotent with Issue 38).                                                                                | achievements           | feature-gap | Medium     | M      | $0   |
| 45 | Social inbox events carry only `actor_id` (`api/social.ts:28–37`) → the activity feed renders "Visitante" for any actor not already in the viewer's friends/requests lists (e.g. a friend request from a stranger). Denormalize `actor_name`+`avatar_url` at write, or a batch `GET /social/players?ids=`. | social                 | bug         | Medium     | S      | $0   |
| 46 | `ended_at` unit inconsistency: `/players/me/hands` returns ms, `ProfileShowcase.best_hand.ended_at` needs a `< 1e12 ? *1000` runtime heuristic (`profile/page.tsx:93`). Pick epoch-ms everywhere; delete the heuristic.                                                                                    | player / sessionlog    | bug         | Low        | S      | $0   |
| 47 | `HandItem` and the hand-share payload carry no blind level → the replayer hardcodes `bigBlind={25}` and misrenders pot/blinds for every other stake. Add `big_blind` (+ escalation level).                                                                                                                 | sessionlog / handshare | bug         | Medium     | S      | $0   |
| 48 | Lobby needs a server-resolved seating primitive: `POST /rooms/join-or-create` (returns the room it actually seated you into) **and** an open-rooms-per-`(blinds, seats, currency)` aggregate, so the client never guesses join-vs-create from a stale page.                                                | rooms / api            | feature-gap | Medium     | M      | $0   |
| 49 | `GET /players/me/hand-shares` (list) — `revokeHandShare` is implemented and callable but there is no endpoint to enumerate a player's active shares, so no revocation UI is possible.                                                                                                                      | handshare / api        | feature-gap | Medium     | S      | $0   |

### 8.3 Frontend issues — High

| #  | Title                                                                                                                                                                                                                                                                   | Module       | Effort |
|----|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------|--------|
| 50 | `table/page.tsx` `handOutcome` never cleared → `AchievementToast blocked` latches, `WinnerCards offerBlocked` latches, both dead after hand 1. (F-T1)                                                                                                                   | table        | S      |
| 51 | Achievements page + showcase picker build progress from one page of counters → wrong stars, completion %, hidden secrets. (F-G1; pairs with Issue 43)                                                                                                                   | achievements | S      |
| 52 | `(marketing)` vs `(app)` route-group split — `/poker-rules` + `/guide/*` currently download the protobuf+WS chunk, react-query, `ProfileMenu`+social, `RealtimeBridge`, keep-alive (~1 MB JS). Root cause: root `QueryProvider` mounts realtime unconditionally. (F-A1) | build        | M      |

### 8.4 Frontend issues — Medium

| #  | Title (module)                                                                                                                                                                               | Slug |
|----|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------|
| 53 | Session recap misreports P/L when `rt.removed` beats the `sessions` query (table)                                                                                                            | F-T2 |
| 54 | BotChallenge non-dismissable dialog stuck on "loading" forever if Turnstile fails to load/callback — no timeout, no reload button (table)                                                    | F-T3 |
| 55 | Voice "all in"/"aumentar" commits an irreversible bet with zero confirmation (table)                                                                                                         | F-T4 |
| 56 | 18k-line `globals.css` → the full table renderer CSS (~308 KB, 86 keyframes) loads on every route incl. the OAuth callback (build)                                                           | F-A2 |
| 57 | RouteAnnouncer announces a possibly-stale `document.title` and double-announces via focus + live region (a11y)                                                                               | F-A3 |
| 58 | OAuth callback collapses every failure into "code expired"; 3 s deadline doesn't abort the fetch; no telemetry (auth)                                                                        | F-A4 |
| 59 | `endExpiredSession` latches `endingExpiredSession=true` permanently → app wedged if the IdP redirect no-ops (auth)                                                                           | F-A5 |
| 60 | Error boundary renders "500" for every thrown error — `ApiError(503)` should route to `/unavailable`, `404` to not-found (shell)                                                             | F-A7 |
| 61 | Actionable error toasts (with retry buttons) auto-dismiss after 6 s, vanishing mid-read (ux)                                                                                                 | F-A8 |
| 62 | Lobby "join vs create" uses only the first page of `/rooms` → spawns a duplicate table (lobby; pairs with Issue 48)                                                                          | F-L1 |
| 63 | Room-full seat race is invisible in the lobby — failure only surfaces later as a buy-in error on the table page (lobby; pairs with Issue 48)                                                 | F-L2 |
| 64 | Real-money wallet-mode `Switch` in `ProfileMenu` is ungated (unlike `CurrencyModeTabs`) and its mutation has no `onError` (lobby)                                                            | F-L3 |
| 65 | A total outage that surfaces as a network `TypeError` (the expected dead-HAProxy shape) never escalates past the thin status strip — the *more common* shape gets the *quieter* UI (network) | F-W1 |
| 66 | Hand replayer hardcodes `bigBlind={25}` → wrong pot/blind rendering for every non-25 table (hand-replayer; pairs with Issue 47)                                                              | F-G3 |
| 67 | Hand-history opponents are inert — no profile link, no add-friend/mute/block/report/note where you review who you just played (hands)                                                        | F-G4 |
| 68 | No UI to list or revoke your shared-hand links — `revokeHandShare` exists and is called nowhere (share; pairs with Issue 49)                                                                 | F-G5 |

### 8.5 Frontend issues — Low (69–92)

Full detail + acceptance criteria in the four module-review files. Grouped here:

**Table:** 69 F-T5 SFX allocated per-play, no preload → `your_turn` cue lags on a slow link · 70 F-T6 reaction FX
positions off a one-shot rect + a `document.querySelector` into `Seat` (stale on reflow/orientation) · 71 F-T8 ~120
lines of outcome-assembly inline in `page.tsx` belongs in
`tableOutcome.ts` as a pure fn · 72 F-T9 4–6 always-on `setInterval` clocks during a turn → one shared ticker.

**Lobby/store/wallet:** 73 F-S1 dead `['wallet','balance']` query key invalidated in 3 places, read nowhere · 74 F-L4
buy-in floor is client-authored and inconsistent (20 BB quick-join vs 40 BB private) · 75 F-L5 seat-size options are
`<Button>`s wrapping `<h3>`; whole grid disables on any one join · 76 F-L6 duplicate/divergent daily-reward +
leaderboard API wrappers (`gamification.ts` vs
`dailyReward.ts`, differ by a trailing slash) · 77 F-W2 `/unavailable` "Verificar novamente" gives no feedback when
still down.

**Auth/shell/perf:** 78 F-A6 logout button has no pending state, double-click fires a 2nd `revoke` · 79 F-A9 no
`loading` variant on `Button`, no `Field` wrapper for `Input` (label/error association hand-rolled per form) · 80 F-A10
8 global Google-font files (Plex Sans + Mono ×4 weights each) · 81 **F-A11 `@bufbuild/protobuf` is a runtime dep but
only present transitively via `ts-proto` (devDep)** — a `ts-proto` major bump breaks the prod bundle; add it to
`dependencies` · 82 F-A12 dead
`scripts/publish-routes.sh` (AWS `cloudfront-keyvaluestore`) + `ui/CLAUDE.md` still documents CloudFront KVS as current
(depth on Issue 19) · plus keep-alive should also refresh on
`visibilitychange`/`online` (folded into F-A4/59).

**Gamification/hands/profile/social:** 83 F-G7 leaderboard "1 jogadorer" plural bug + unvirtualized list + non-standard
error state · 84 F-G9 profile showcase collapses private / your-own / 404 into one wrong message; loading branch has no
`<h1>` · 85 F-G10 matchup view drops `ties` and
`net_change_viewer` (both fetched, neither shown) → arithmetic that doesn't add up · 86 F-G11 replayer has no keyboard
transport and autoplay ignores `prefers-reduced-motion` · 87 F-G12 hand history has no filters/grouping and no lifetime
totals · 88 F-G13 achievement tier stars are
`<button>`s that do nothing on activation (5 dead tab-stops per card) · 89 F-G14
"fairness proof unavailable" for legacy hands is styled with the same red `mismatch` class as a real hash failure · 90
F-G15 shared-hand links have no rich preview card · 91 F-G2-frontend (rail UI for Issue 44) · 92 CSP `unsafe-inline` is
**load-bearing** for the static export's inline hydration scripts — the fix (Issue 18) must be per-build script *hashes*
in the reusable workflow, not a config toggle (depth on Issue 18).

### 8.6 Verified impeccable (no action)

Sandbox PIX purchase flow · liveness/outage architecture · public hand-share anonymization (server-side, thorough) ·
provably-fair verify UX · token model (in-memory singletons, no
`localStorage`) · `USE_MOCK` prod stripping (`mockRuntime.ts` confirmed absent from the prod graph) ·
`CreateRoomDialog` radiogroup keyboard impl · hand-history list empty state + hybrid pagination · retry policy (bounded,
jittered, TanStack retry disabled to avoid amplification) · friend-code discovery (exact match only, non-enumerable) ·
playstyle-leak consent model.

---

## 9. Revised strategic recommendations (post frontend review)

1. **$0, this week:** 1, 14, 17, 19, 20, 26, 35, **41** (cosmetic webhook — real money-ish gap), **50** (handOutcome — a
   live feature is dead), **73**, **81** (`@bufbuild/protobuf`).
2. **Before wider audience:** 1, 2, 3, 5, 6, 33, **43+51** (achievements truncation), **52** (bundle), D6.
3. **Leaderboard + achievements as the OK-vs-impeccable pair:** 34, 35, 36, 43, 44, 51.
4. **Responsible-gaming correctness:** 42 (cumulative buy-in) — small, and it's the one place the product actively
   mis-states a player's results.
5. Real-money blockers: 10, 11, 12, D1, D2 — unchanged.

## 10. Proposed filing order (full)

Security/crash → correctness → reliability → performance → tech-debt → testing, backend then frontend:

**Backend/infra:** 1, 13, 10, 11, 14, 16, 20 · 4, 12, 23, 34, 35, 38, 36, 41, 43, 42, 47, 45, 48, 49, 44, 46, 26, 27,
17 · 2, 5, 3, 7, 6, 8, 33, 25, 30 · 9, 15, 37, 39, 28, 29, 40. **Frontend:** 50, 51, 52 · 53–68 (Medium, in listed
order) · 69–92 (Low) · 18, 21, 22, 24, 31, 32. **Deep-dive / test:** D1, D2, D6.

---

## 11. GitHub issue index (filed 2026-09-02)

All 95 open in `artur-oliveira/ctech-poker`. Doc number = GitHub number − 28.

| GitHub | Title |
|---|---|
| #29 | [BACKEND/TABLE] Table actor Run/handle has no panic recovery — one engine panic crashes the whole process |
| #30 | [INFRA] No alarm on any Lambda DLQ; a message landing in one is invisible |
| #31 | [BACKEND/TABLEMANAGER] GetOrCreateActor serializes the whole instance behind one mutex + three network calls |
| #32 | [BACKEND/RECONCILE] cmd/reconcile swallows per-entry errors — stuck money never reaches the DLQ, no attempt counter |
| #33 | [INFRA/BACKEND] Termination-drain lifecycle hook is best-effort; DrainAndRelease is not idempotent |
| #34 | [INFRA] Blanket 1000 RCU/WCU cap on every DynamoDB table + no throttle alarm |
| #35 | [INFRA] Single-AZ, spot-only, single-instance-type ASG → correlated non-self-healing outage |
| #36 | [INFRA/BACKEND] t4g.nano 512 MB vs unbounded in-process actors, timers, and the equity cache |
| #37 | [BACKEND/TABLE] broadcastAll does heavy per-commit work on the actor goroutine (equity MC + preselection cascades) |
| #38 | [BACKEND/BUYIN] Real-money buy-in skips the poker-terms acceptance check |
| #39 | [BACKEND/WALLETCLIENT] Two ctech-account scopes ungranted → wallet verification impossible |
| #40 | [BACKEND/MONEY] Real-money gaps: Lambda legal gate, no real-money table cleanup, Claim-race free seat |
| #41 | [INFRA/.github] GitHub OIDC infra role = AdministratorAccess, trust not branch-scoped; API role ssm:SendCommand on * |
| #42 | [BACKEND/BUYIN] Refund idempotency key collides across players when idemKey is empty |
| #43 | [BACKEND/API] WebSocket rate limiters are per-instance → N× the intended limit across the fleet |
| #44 | [BACKEND/API] WebSocket upgrade accepts any request with no Origin header |
| #45 | [BACKEND/API] Fixed-window rate limiter can leave a TTL-less key if EXPIRE fails after INCR |
| #46 | [FRONTEND/INFRA] unsafe-inline in the script-src CSP |
| #47 | [INFRA/.github] Dead/duplicate frontend-cloudfront.yml workflow runs on every UI PR |
| #48 | [INFRA/.github] No SAST, secret-scanning, or dependency-review in CI |
| #49 | [INFRA/.github] staticcheck@latest / govulncheck@latest unpinned; reconcile-stack/oidc-stack untested |
| #50 | [FRONTEND/TABLE] useTableRealtime.ts (1086 L) — decompose the resilience state machine |
| #51 | [BACKEND/TABLE] Cache-rollback obligation is convention-only across mutating handlers |
| #52 | [BACKEND/TABLE] actor.go (2838 L) — decompose (timers, presence, hooks) |
| #53 | [FRONTEND] No client-side error telemetry (crashes/JS errors invisible) |
| #54 | [BACKEND+FRONTEND] Dead code and silent error swallows |
| #55 | [BACKEND/ARCHIVER] Audit archive renders numeric attributes as float64 |
| #56 | [INFRA] Over-broad IAM on the Lambda roles (unused dynamodb:Scan) |
| #57 | [BACKEND/BUYIN] tableUnavailable spins up a full Actor just to count seats |
| #58 | [BACKEND/TABLE] ReconnectCmd dispatched to the actor on every inbound WS frame incl. ping |
| #59 | [FRONTEND] set-state-in-effect suppressions and any casts |
| #60 | [FRONTEND] No a11y (axe) gate, no bundle-size budget, no Lighthouse CI |
| #61 | [BACKEND/TABLE] onHandComplete runs the entire gamification pipeline synchronously on the actor goroutine |
| #62 | [BACKEND+FRONTEND/LEADERBOARD] No 'my rank'; frontend ranks within one fetched page; single-partition GSI is a global hotspot |
| #63 | [BACKEND/LEADERBOARD] win_rate leaderboard is gameable — a 1-hand 100% account tops it; no min-hands floor in the GSI |
| #64 | [BACKEND/PLAYER] Display-name changes never propagate to denormalized copies (leaderboard, hand history, public shares, live seats) |
| #65 | [BACKEND/MATCHUP] 72-item transaction per completed hand |
| #66 | [BACKEND/GAMIFICATION] achievements + leaderboard counters double-count on a Valkey blip |
| #67 | [FRONTEND/TABLE] table/page.tsx god component + 1 Hz full re-render during a turn |
| #68 | [BACKEND/SESSIONLOG] Frozen avatar_url in hand-history opponent summaries 404s after ClearAvatar |
| #69 | [BACKEND/COSMETICPURCHASE] Cosmetic (deck/felt) PIX purchases have no webhook confirmation or realtime push |
| #70 | [BACKEND/SESSIONLOG] Session buyin_amount must be cumulative across rebuys (responsible-gaming) |
| #71 | [BACKEND/ACHIEVEMENTS] /players/me/achievements returns one page → truncated progress; add a summary endpoint |
| #72 | [BACKEND/ACHIEVEMENTS] No unlock timestamp → no 'recently unlocked' moment on the achievements page |
| #73 | [BACKEND/SOCIAL] Inbox events carry only actor_id; the activity feed can't name most actors |
| #74 | [BACKEND/PLAYER] ended_at unit inconsistency across hand endpoints |
| #75 | [BACKEND/SESSIONLOG+HANDSHARE] Add blind level to HandItem and the hand-share payload |
| #76 | [BACKEND/API] Lobby needs a server-resolved seating primitive + open-rooms-per-bucket aggregate |
| #77 | [BACKEND/HANDSHARE] GET /players/me/hand-shares list endpoint (for the revocation UI) |
| #78 | [FRONTEND/TABLE] handOutcome is never cleared → achievement toasts and the pay-to-see-winner-cards offer die after hand 1 |
| #79 | [FRONTEND/ACHIEVEMENTS] Progress map is first-page-only → wrong stars, completion %, next-star, secret unlocks |
| #80 | [FRONTEND/BUILD] Marketing/SEO pages ship the full authenticated-app JS bundle |
| #81 | [FRONTEND/TABLE] Session recap misreports P/L when rt.removed arrives before the sessions query resolves |
| #82 | [FRONTEND/TABLE] BotChallenge can permanently lock a player out when Turnstile fails to load / never calls back |
| #83 | [FRONTEND/TABLE] Voice 'all in' / 'aumentar' commits an irreversible bet with no confirmation |
| #84 | [FRONTEND/BUILD] 18k-line global stylesheet loads the full table renderer CSS on every route |
| #85 | [FRONTEND/A11Y] RouteAnnouncer announces a possibly-stale title and double-announces via focus + live region |
| #86 | [FRONTEND/AUTH] OAuth callback collapses every failure into 'code expired', no abort, no telemetry |
| #87 | [FRONTEND/AUTH] endExpiredSession latches permanently and wedges the app if the IdP redirect no-ops |
| #88 | [FRONTEND/SHELL] Error boundary treats every thrown error as a generic 500 |
| #89 | [FRONTEND/UX] Actionable error toasts auto-dismiss after 6 seconds |
| #90 | [FRONTEND/LOBBY] 'join vs create' decision uses only the first page of /rooms |
| #91 | [FRONTEND/LOBBY] Room-full seat race is invisible in the lobby; failure only surfaces on the table page |
| #92 | [FRONTEND/LOBBY] Real-money wallet-mode Switch in ProfileMenu is ungated and has no error handler |
| #93 | [FRONTEND] Total-outage that appears as a network error never escalates past the thin status strip |
| #94 | [FRONTEND/HAND-REPLAYER] Hardcoded bigBlind={25} misrepresents pot and blinds for non-25 tables |
| #95 | [FRONTEND/HANDS] Hand-history opponents are inert — no profile link, no player actions |
| #96 | [FRONTEND/SHARE] No way to list or revoke your shared-hand links |
| #97 | [FRONTEND/TABLE] Turn/chip SFX allocated per-play with no preload — your_turn cue arrives late |
| #98 | [FRONTEND/TABLE] Reaction FX position off a one-shot rect read and an untyped DOM query into Seat |
| #99 | [FRONTEND/TABLE] Outcome-assembly logic (~120 lines) lives in the page, not tableOutcome.ts |
| #100 | [FRONTEND/TABLE] Multiple always-on setInterval clocks during a turn |
| #101 | [FRONTEND/STORE] Dead ['wallet','balance'] query key invalidated in three places, read nowhere |
| #102 | [FRONTEND/LOBBY] Buy-in min/max is client-authored and inconsistent between quick-join and private-room |
| #103 | [FRONTEND/LOBBY] Seat-size options render as buttons wrapping headings; whole grid disables during any one join |
| #104 | [FRONTEND] Duplicate/divergent daily-reward + leaderboard API wrappers |
| #105 | [FRONTEND] /unavailable 'Verificar novamente' gives no feedback when the API is still down |
| #106 | [FRONTEND/AUTH] Logout button has no pending state or failure feedback |
| #107 | [FRONTEND/DESIGN-SYSTEM] No pending/loading affordance in Button; no Field wrapper for Input |
| #108 | [FRONTEND/PERF] Eight Google font files loaded globally; sound clips fetched on first play |
| #109 | [FRONTEND/BUILD] @bufbuild/protobuf is a runtime dependency but only present transitively |
| #110 | [INFRA/.github] Dead route-publish script + stale ui/CLAUDE.md after the Cloudflare cutover |
| #111 | [FRONTEND/LEADERBOARD] Plural copy bug ('1 jogadorer'), no virtualization, non-standard error state |
| #112 | [FRONTEND/PROFILE] Private / own / not-found showcase collapse to one wrong message; loading has no h1 |
| #113 | [FRONTEND/PROFILE] Matchup view drops ties and net result → arithmetic that doesn't add up |
| #114 | [FRONTEND/HAND-REPLAYER] No keyboard transport; autoplay ignores reduced-motion |
| #115 | [FRONTEND/HANDS] History has no filters or grouping and no lifetime totals |
| #116 | [FRONTEND/ACHIEVEMENTS] Tier stars are <button>s that do nothing on activation |
| #117 | [FRONTEND/HAND-HISTORY] 'Fairness proof unavailable' for legacy hands is styled as an error |
| #118 | [FRONTEND/SHARE] Shared-hand links have no rich preview |
| #119 | [FRONTEND/ACHIEVEMENTS] 'Recently unlocked' rail + arrival celebration (uses Issue 44's timestamp) |
| #120 | [INFRA] CSP unsafe-inline is load-bearing for the static export — fix must be per-build script hashes |
| #121 | [BACKEND/ENGINE] Deep-dive: hand.go multi-way all-in + run-it-twice + odd-chip + folded-money settlement audit |
| #122 | [BACKEND/MONEY] Deep-dive: reconcile + entitlement concurrency / lost-update audit |
| #123 | [BACKEND/INFRA] Load / soak test at target concurrency before wider release |
