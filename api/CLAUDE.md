# api/ — CLAUDE.md

Go real-time poker game server (Fiber v3 + `fasthttp/websocket` + DynamoDB + Valkey). **Sandbox (play-money) mode is
implemented end-to-end with a 2.5% rake engine. Real-money mode (Phase 5) is fully implemented end-to-end under the
Brazil-legal fixed-fee model (no rake, flat entry fee per tier):** `POST /rooms` accepts `currency_mode: "real"`,
validates blinds against the 10-tier fee catalog, and stores the fixed `EntryFeeCents` and `Tier` on the room. The fee
is a **per-table reservation entitlement (`internal/entitlement`, `poker_table_entitlements`), not a per-buy-in
charge**: the first real-money buy-in at a table claims and pays a 3-hour, absolute (never sliding) entitlement before
the player is seated; every rebuy or re-entry at that same table within the window is free. If the table becomes
unavailable (archived, or full) while the entitlement is still valid, `buyin.Service` rebinds it to another table of the
same tier instead of charging again — never across tiers. `GET /rooms/:id/seated` exposes `fee_due` /
`entitlement_expires_at` so the client never surprises a player with a silent charge at buy-in. Failed fee debits are
queued to the same retry table (`poker_pending_cashouts` with `Kind: "fee_debit"`) for Lambda reconciliation retries;
the entitlement itself is left in place on a debit failure (nobody is ever seated on that path) so the retry — this
request's caller, or `cmd/reconcile` — completes the same idempotent charge at most once. See
`docs/plans/2026-08-21-entry-fee-entitlement.md`. The Rebind half of the entitlement race is closed by #146 (issue
#122) — see `docs/specs/2026-09-02-reconcile-entitlement-concurrency-audit.md`. **The Claim half was closed later
(#40):** #139 dropped its `confirmFeeCharged` in favour of #146, and #146 shipped only the `Rebind` CAS, so the
free-seat window survived both. Coverage is now never inferred from "an entitlement row exists" — every path
(`resolveEntitlement`'s exact-match and rebind branches, the `ErrAlreadyClaimed` loser, and read-only `FeeStatus`)
goes through `buyin.Service.confirmFeeCharged`, which decides from the fee's own **resolved** `poker_pending_cashouts`
row and otherwise completes the same charge before anyone is seated. That works because the fee's idempotency key is
now derived from the reservation itself (`entitlementFeeKey`: origin table + player + `ExpiresAt`), not from the
request nonce — so a retry, a claim-race loser and `cmd/reconcile` all reproduce one key and ctech-wallet charges once,
while a fresh window after expiry keys a new charge.

`cmd/tablecleanup` now handles real-money tables instead of skipping them: it settles every seated player's game-wallet
hold via `CashoutGame`, first recording the obligation to `poker_pending_cashouts` (same
record-then-attempt-then-resolve shape as `buyin.Service.settle`) so `cmd/reconcile`'s sweep retries it — using the
recorded hold ID — if the immediate cash-out fails; the table archives either way once the obligation is durably
recorded. The table-entry entitlement itself is left untouched (it's a paid, non-refundable reservation, not a fund
hold — it just expires on its own TTL). A room record that can't be found is now "unknown, skip" — never treated as
sandbox — so a sweep-ordering bug can no longer credit a real-money table's stack to the sandbox ledger. See
`cmd/tablecleanup/main.go`'s `settleRealMoneyAndArchive`.

**Still blocking, found 2026-07-25 while verifying cross-repo:**

1. ctech-wallet's scope catalog (`ctech-account/api/internal/scopes/catalog.go`) has no `internal:wallet:game-status`
   entry, so no M2M client can ever be granted the scope `ctech-wallet`'s `GET /wallet/game/status/:user_id` requires.
2. Poker's M2M client has never been granted the `internal:wallet:debit-real` scope in `ctech-account`'s catalog. Both
   are data/config actions in `ctech-account`, not code changes in this repo — **this is what needs to change to
   actually unblock real-money mode; nothing in this repo can grant a scope on ctech-account's behalf.** (issue #39)
   Until both are granted, in every environment poker's M2M client runs in:
    - Add `internal:wallet:game-status` to `ctech-account/api/internal/scopes/catalog.go`.
    - Grant poker's M2M client both `internal:wallet:game-status` and `internal:wallet:debit-real` in
      `ctech-account`'s client-grant data/config for every environment (dev/staging/prod), and cover both grants in
      deploy reconciliation so a new environment can't come up missing them silently. What this repo *does* do about it:
      `walletclient.Client.ValidateRequiredScopes` (`internal/walletclient/client.go`)
      runs once at startup, gated on `REAL_MONEY_ENABLED`, wired via `validateWalletScopes` in `internal/app/app.go`
      (registered as an `fx.Lifecycle` `OnStart` hook, before `startServer`). It fetches an M2M token for each of the
      two scopes above and confirms the scope actually made it into the grant — checking the token endpoint's own
      response (an outright rejection surfaces directly) and, for a JWT access token, decoding its `scope` claim
      (unverified — safe here since we already trust the token endpoint's TLS response; this is a diagnostic read, not
      an auth decision) in case ctech-account silently narrows the grant instead of rejecting the request. Either
      failure mode returns an error from the `OnStart` hook, which fails the whole process to start with a message
      naming exactly which scope is missing — so a broken grant is a loud, immediate deploy failure instead of every
      real-money entry fee and gambling-activation check silently failing in production. An opaque (non-JWT) token can't
      be decoded this way and is treated as "can't verify, assume granted" rather than a false positive. Also unresolved
      (re-verified 2026-07-28):

- An ASG lifecycle hook + drain Lambda **do** exist (`cdk/lib/api-stack.ts`'s `TerminationDrainFunction`) and do reach
  `tablemanager.DrainAndRelease` via `OnStop` when they fire — re-verified 2026-09-01 from
  `"shutting down ctech-poker-api, draining table manager leases"` in `/ctech-poker/prod/app`. What is not reliable is
  the hook firing for *every* termination: under a spot rebalance storm the same day, the drain Lambda invoked for only
  3 of at least 4-5 real terminations — see `cdk/CLAUDE.md`'s Known Issues for the details and
  `docs/specs/2026-09-01-duplicate-seat-commit-guard.md` for the resulting incident. **Fixed (#33):** the app no longer
  waits solely on that hook. `startServer` (`internal/app/app.go`) also runs
  `pollSpotTermination` — a background goroutine, only in prod (`cfg.Env == "prod"`), that polls this instance's own EC2
  metadata (`http://169.254.169.254/latest/meta-data/spot/instance-action`)
  every 5s and calls `manager.DrainAndRelease` proactively the instant a spot termination notice appears, instead of
  waiting for the Lambda to reach `OnStop`. `tablemanager.Manager.DrainAndRelease`
  is now idempotent — a `drainMu`/`draining`/`drainDone` guard makes every call after the first (concurrent or
  sequential, from the proactive poller or the OnStop/SIGTERM path) a no-op wait rather than a second walk of
  `m.actors`, so the two triggers can never double-release a lease no matter which fires first or whether both do. This
  still does not cover non-spot terminations (no metadata notice precedes those) — treat the hook itself as best-effort
  for those, with the commit-time duplicate-seat guard below as the remaining backstop.
- No WAF at the CloudFront edge (and the distribution itself is being retired — the app is on Cloudflare Workers);
  application rate limits (`internal/api/v1/ratelimit.go`) and Turnstile are the only protection.
- `cmd/reconcile` now *reaches* its Lambda DLQ: each pending entry carries an `Attempts` counter
  (`reconcile.PendingCashout`), a per-entry failure increments it via `PendingStore.RecordFailedAttempt`, and once it
  hits `reconcile.MaxAttempts` (5) the row's `gsi_status` flips to `"manual_review"` so it drops out of `ListUnresolved`
  and `run` returns an aggregated error — failing the invocation so the message lands in the DLQ. Early-attempt failures
  are still counted + logged (`slog.Warn`) and retried next run without failing the invocation. `run` processes the
  whole batch before returning, so one poison entry never blocks the rest.
- As of #30, the `cmd/reconcile`, `cmd/tablecleanup`, and archiver Lambdas each have a DLQ-depth alarm
  (`ApproximateNumberOfMessagesVisible >= 1`) + a Lambda-`Errors` alarm on the `ctech-prod-alerts` SNS topic
  (`cdk/lib/alarms.ts`, `addLambdaDlqAlarms`).

## Conventions (follow these)

- **Reuse `gopkg.aoctech.app/api-commons`** for JWT verify (`jwtverify`), WebSocket registry (`ws.Registry`), cache
  backend (`cache.Backend`), and problem responses (`problem`). Do NOT hand-roll these.
- **Named constants / no magic strings.** DynamoDB table/field names, route paths, event type strings, and config keys
  live as named identifiers, not literals at call sites.
- **Correctness = DynamoDB conditional writes.** Every mutated action commits via
  `tablestore.CommitAction` with a `version` equality `ConditionExpression` + per-action idempotency guard. Never
  read-then-write against table state.
- **Never read the table item with `TransactGetItems`.** `LoadTable` uses `BatchGetItem` with `ConsistentRead` — the
  same strong consistency, no transaction semantics. A transactional read conflicts with any `CommitAction`
  `TransactWrite` on that same item and fails with `TransactionCanceledException[TransactionConflict]`, which the SDK
  never retries (it is in neither `DefaultRetryableErrorCodes` nor `DefaultThrottleErrorCodes`). Since `ensureLoaded`
  runs before every handler's own validation, a failed load aborts the command outright: that is what made
  `show_cards` / `request_rabbit_hunt` fail on essentially every attempt in the write-heavy post-hand window. The
  isolation was never worth anything either — a one-item read set is already atomic. Handle `UnprocessedKeys`: a
  `BatchGetItem` that read nothing is not the same as an absent table. See
  `docs/plans/2026-08-27-table-load-transaction-conflict.md`.
- **Distinguish "the store failed" from "the action is illegal."** Failures that never reached a verdict about the
  player's command wrap `tablestore.ErrUnavailable`; `tablews.go`'s `actionErrorCode` turns those (and
  `table.ErrActorStopped`) into `unavailable`, everything else into `invalid_action`. Reporting an outage as
  `invalid_action` blames the player for it and makes the client end the command instead of resyncing and resubmitting.
  `ErrVersionConflict` stays `invalid_action` — it *is* a verdict.
- **`tablelease` is latency-only**, not correctness. Never add lease-based correctness logic.
- **Every `a.cached`/`a.handID`/`a.activity`-mutating handler routes its mutation-and-commit body through
  `Actor.mutate(fn func() error) error`** (`internal/table/actor.go`) — never a hand-rolled snapshot/restore. `mutate`
  snapshots all three before calling `fn` and restores them on any error `fn` returns (a validation rejection, an engine
  error partway through, or `a.commit` itself failing), so a handler can no longer forget to undo a partial mutation the
  way `applyReadyAndCommit` did until 2026-09-01 (leaving an uncommitted mutation trusted in the actor's cache under
  `trustCache`, which the next unrelated successful commit then persisted for real — a player ended up seated three
  times at once while another silently vanished during a spot-instance rebalance storm). The snapshot round-trips
  through `attributevalue.MarshalMap`/`UnmarshalMap` (the same encoding `CommitAction` uses for the real write) rather
  than a bare `ExportState()`/`NewTableFromState()` pair — that shallow pair aliases the live
  `*Player` pointers (and, on a removal, the same backing array), so it silently fails to undo an in-place field
  mutation on an already-seated player. `Actor.commit` also refuses outright to persist any state with a duplicate
  player ID (`hand.Table.DuplicateSeatIDForActor`) as a backstop, and `ensureLoaded` forces a reload past
  `trustCache` the moment it sees one — but a missing rollback is still a bug, not something to rely on the backstop
  for. See `docs/specs/2026-09-01-duplicate-seat-commit-guard.md` (2026-09-02 follow-up section, #51).
- **The actor goroutine recovers per-command panics — never let one escape.** `Actor.Run` dispatches every command
  through `handleSafely`, which wraps `handle` in `defer/recover`: an engine panic (out-of-bounds `dealCard`, malformed
  persisted `State` decode, a nil deref in `hand.go`) is logged with `table_id`/`hand_id`/command type + stack, the
  caller gets a `tablestore.ErrUnavailable`-wrapped error (resync, not `invalid_action`), and `a.cached`/`version`
  /`handID`/`activity` are dropped so the next command reloads authoritative state — a panic can land mid-mutation with
  no rollback path, exactly the poisoned-cache shape of the 2026-09-01 duplicate-seat incident. Nothing is persisted:
  the panic unwinds before `commit`. Don't add a bare `recover()` elsewhere in the actor or swallow a panic silently.
- **Player identity comes from the JWT `sub`** — derive `playerID` from claims, never trust a client-supplied id
  (prevents IDOR).
- **The `currency_mode` boundary is load-bearing.** `buyin` routes to exactly one ledger per room and must never let
  sandbox chips reach the real wallet or vice versa — enforce it in `buyin`, not at the handler. The real path is built;
  what gates it at runtime is `REAL_MONEY_ENABLED` + `LEGAL_SIGNOFF_REF`, checked fail-closed in
  `config.Load` and, since the reconcile Lambda also moves real money, in `config.LoadForLambda`. (Earlier revisions of
  this file said to reject non-`sandbox` outright — that is no longer the rule.)
- **Money ordering is deliberate**: debit-then-seat on buy-in, remove-then-credit on cash-out. Anything that can fail
  after chips moved goes to `poker_pending_cashouts` for the `cmd/reconcile` sweeper. Keep new money paths in that shape
  rather than inventing a compensating transaction per call site.
- **Hidden information never leaves `ViewFor`.** `Table.ViewFor(viewerID)` is the single place that decides per-viewer
  visibility, masking unseen hole cards as `"back"` before serialisation; fan-out is keyed
  `<tableID>#<viewerID>` so two seats cannot share a snapshot. Add visibility rules there, never in a handler.
  `SeatView.HandCategory` follows the same rule: an opponent's category is built only from their actually-revealed hole
  cards (0, 1, or 2 — `snapshot.go`'s `partialCategory`/`categoryFromRankCounts`) plus the board, never their
  still-hidden card, and stays unset entirely with nothing revealed — a board-only category would be identical for every
  unrevealed opponent and read as hands being shown before showdown. The viewer's own seat is the one exception —
  `Table.ViewFor` sends their true category unconditionally the same way it does `Equity` — so a client showing it
  before the viewer has locally peeked their own cards leaks the "all-in/won without peeking"
  achievements' exact hidden state (Seat.tsx's `showHandCategory`/`showEquity` gates are what withhold it there).
- **The fairness reveal is asymmetric on purpose.** The server seed is published only when nothing stayed hidden (a real
  showdown). Every other hand gets the seed-less per-position proof (`fairnessProofFor`). Widening seed publication
  would retroactively expose mucked hole cards — treat it as a security change, not a feature.
  `FairnessProofs` is set only on the copy handed to hooks, never on `Table.lastOutcome`, which is persisted with every
  table-state write.
- **Post-hand hooks that need to write back into the same table's live cache must call the Actor directly, never
  `Dispatch`.** `tablemanager.Manager.SetOnHandCompleteForActor`'s wrapper (and every hook chained onto it —
  `autoRebuySweep`, `SetOnTableStreak`, ...) runs synchronously on that table's own actor goroutine; a `Dispatch` back
  into the same actor from there deadlocks (its `Run` loop is the very call stack already blocked processing this hook).
  `Actor.SetStreaksForActor` (backing the hot/cold streak badge, `Seat.CurrentStreak`) is the pattern to copy:
  a plain exported method that mutates actor-owned cache state directly, applied onto `ViewFor`'s output the same way
  `applyPresence` already does for `ConnectionState`.
- **`onHandComplete`'s own gamification pipeline runs off the actor goroutine, not on it (fixed 2026-09, #61).**
  `app.newTableManager`'s `onHandComplete` closure — achievements, leaderboard, pokerstats, matchup, session/hand
  history, highlights, recent players, ~50-150+ sequential DynamoDB round trips at a full table — is detached via
  `app.dispatchGamificationPipeline` (`go` + `recover`), the same pattern `autoRebuySweep` already used. This is safe
  specifically because, by the time `table/actor.go`'s `notifyHandComplete` reaches this hook,
  `broadcastAll` has already sent every player their post-hand `state` snapshot, and the fleet-wide `handhook` SET NX
  claim (`claimHandHooks`) has already been taken synchronously — so detaching can neither delay what a player sees.
  `SetOnTableStreak` and `autoRebuySweep` stay exactly as they were (the former writes back into actor-owned cache via
  `SetStreaksForActor` and must stay synchronous per the rule above; the latter was already detached). **Known gap, not
  closed by this change:** if the process dies while the detached goroutine is mid-flight, the hand's
  `handhook` claim was already taken and is never released, so that hand's gamification writes are permanently lost —
  pre-existing (a synchronous panic/crash mid-`onHandComplete` had the same failure mode) but the detach widens the
  window slightly since the actor can move on to the next hand while the goroutine is still running.
  **Ordering within the pipeline matters (2026-09-03):** the two player-visible writes — `persistHandHistory`
  (backs the last-winners strip) and `highlightsStore.RecordHand` (backs "maior pote de hoje") — run **first**,
  before achievements/leaderboard/pokerstats/matchup. The client invalidates both query keys the instant it sees the
  `complete` snapshot, so anything ahead of these writes in the pipeline was making its refetch race ahead of the row
  and show the finished hand a whole hand late. Keep them at the front; the UI also re-invalidates on a backoff
  (`ui/src/lib/settleRefetch.ts`) as the real safety net. See
  `docs/specs/2026-09-03-post-hand-refresh-latency-and-achievement-toast-replay.md`.
- **`handhook`'s claim does NOT by itself make the pipeline's counters double-run-safe (#66).** `claimHandHooks`
  fails OPEN on a Valkey error ("a double credit is at least visible and bounded" — see `internal/handhook`'s doc
  comment), so a Valkey blip during hand completion can let two instances both pass the claim and both reach
  `onHandComplete` for the same hand. `pokerstats.Store.RecordHand`/`matchup.Store.RecordHand` already guard themselves
  with their own `"guard#table_id#hand_id"` conditional-write idempotency key (safe either way), and
  session/hand-history/highlights/recent-players are plain overwrites (also safe either way) — but
  `achievements.Service.RecordHand`'s `bump`/`bumpBy`/`streak` and `leaderboard.Service`'s
  `IncrementStats`/`IncrementAchievementPoints` are bare DynamoDB `ADD`s with no guard of their own, so a duplicate run
  permanently inflated every participant's achievement progress and `hands_played`/`hands_won`. Fixed by
  `achievements.Service.ClaimHandCounters` / `achievements.Store.ClaimHandCounters` — a second, DynamoDB-backed
  conditional-write claim (reusing `poker_achievement_progress`, no new table, the same reuse `streakKeyPrefix`
  already established) that `app.go`'s pipeline takes once, before calling `achv.RecordHand` and either
  `leaderboardSvc` method, skipping both entirely when the claim is lost. Unlike `handhook`'s claim, an error from
  `ClaimHandCounters` fails **closed** (skips the increments) rather than open: this guard is the actual source of truth
  for "already counted," not an optional latency accelerator, so an ambiguous outcome must never risk a second increment
  landing. If a future writer needs the same non-idempotent-`ADD` protection, gate it behind
  `ClaimHandCounters` too rather than inventing a new guard — don't assume `handhook`'s claim alone is enough.
- **Per-seat display state belongs in Valkey, not in the actor.** Several instances serve one table and all broadcast to
  the same sockets, so a process-local tally shows two different values for one seat, alternating between broadcasts
  (the streak badge read "V2, V4, V2, V4" in production). `internal/tablestreak` holds it now:
  `ensureLoaded` re-reads it, `SetStreaksForActor` merges into it, and `Actor.streaks` is only that read — never the
  source of truth. Removing a player follows the same rule with a harder edge: **only persisted state (`LastActionAt`)
  may justify a removal.** In-memory presence marks (`disconnectedSince`) are instance-local, and the disconnect kick
  built on one was cashing out players who had merely reconnected through another instance.
- **A process flag cannot deduplicate a fleet-wide side effect.** `Table.lastOutcome` is persisted, so any instance that
  broadcasts an already-`Complete` table runs `notifyHandComplete` — chat, reactions and connect/disconnect all call
  `broadcastAll`. Guard non-idempotent hooks with `internal/handhook` (`SET NX` per `(table_id, hand_id)`), not with a
  field. Pick the store by what the state *is*: an atomic claim needs the raw Valkey client (`cache.Backend`'s
  `Get`/`Set` cannot express one); a value born with a state transition belongs in the same conditional write
  (`StoredTable.NextHandDeadlineUnixMs`, resumed via `pendingNextHandDeadline` like the turn clock); merely shared
  display state goes in `cache.Backend` (`tablestreak`, `tableconn`). Everything cross-instance fails **open** — degrade
  the display or accept a bounded duplicate, never stall the actor goroutine or drop a player's progression.
- **A timer that has already fired is the actor's only scheduler — a handler that consumes one must put it back.**
  `handleNextHand` clears `nextHandArmedFor` before anything that can fail, but on a quiet table between hands there is
  no later command to carry a re-arm, so an ordinary failure (a cancelled DynamoDB context on the load or the commit)
  stalled the hand outright (#136). Every non-panic error branch now goes through `retryNextHand`, which re-arms with a
  linear backoff bounded by `MaxNextHandRetries`; past the cap the AFK sweep's `rearmTimersFromCache` stays the
  last-resort watchdog. `Dispatch` is blocking, never lossy — a full mailbox backpressures the caller.
  **The re-arm is also capped the other direction: `armNextHandTimer` refuses to (re-)arm more than
  `MaxNextHandArmsPerHand` times for one `handID`.** `rearmTimersFromCache` fires on every reconnect, keepalive ping
  and sweep, and a persisted next-hand deadline in the past makes the timer fire instantly — so on a wedged table (a
  seat that won't leave) a client reconnect loop became ~8 rejected next-hand `TransactWriteItems` per second, each
  still billed, for 12 minutes (2026-09-02, `docs/specs/2026-09-03-next-hand-rearm-storm.md`). Past the cap the timer
  is left un-armed; `cmd/tablecleanup` or an operator is the recovery for a table that stuck, not a transaction that
  keeps being rejected. `poker_table_state` / `poker_table_state_history` are also TTL'd now (`tablestore.stateTTLDays`,
  refreshed on every commit) and PITR-off — ephemeral, rebuildable, and a runaway write shows up in PITR storage too.
- **A timer-fired handler must force a fresh reload, not `ensureLoaded(ctx, false)`.** `handleTurnTimeout`,
  `handleNextHand` and `handleRunoutStep` are only ever reached from a `time.AfterFunc` armed by *this* actor
  instance — and `internal/tablelease` is latency-only, never an exclusive fleet lock (several instances run
  independent `Actor`s for the same table by design, each `trustCache`-ing its own copy indefinitely between real
  syncs). A quiet instance's timer can fire minutes after another instance has already carried the hand
  forward; `ensureLoaded(ctx, false)`'s trustCache fast path let that stale cache pass the handler's own
  current-player/stage guard, charging time bank and logging turn-timeout activity against a stage/hand another
  instance had already resolved — reproduced live 2026-09-04, see `docs/specs/2026-09-04-cross-instance-stale-turn-timer.md`.
  All three now call `ensureLoaded(ctx, true)` on entry, same reasoning `handleJoin` already used. The
  DynamoDB version-conditioned commit already stopped this from *persisting* wrong state in most cases; the fix
  closes the staleness window itself, which is what fed a stale `next_hand_unix_ms`/`action_deadline_unix_ms`
  into a broadcast. `handleKickTimeout`/`handleAFKSweep`/`handleExpireWinnerCards` share the same
  `ensureLoaded(ctx, false)`-from-a-timer shape and were deliberately left out of this fix (see the spec) —
  revisit if the same class of bug shows up there.
- **A snapshot must never carry a deadline that has already elapsed, no matter why it went stale.** The above
  fix alone did not close the incident — `tablemanager.Manager.evictActorWhenIdle` (renamed from
  `evictLeaseLessActorWhenIdle` in the same `666837c` split) now evicts an idle actor **even if it holds the
  table lease** (previously exempt), and since two Go processes per instance sit behind nginx round-robin
  (`APP_PORT`/`APP_PORT_ALT`), the lease-holding process for a table can easily have zero locally-connected
  sockets while the other process serves both players — so it gets evicted mid-game. `armTurnTimer`/
  `armNextHandTimer` deliberately reuse a persisted deadline "even if already past" on a fresh actor's first
  arm (a legitimate cross-instance resume), and a resume landing mid-turn served that already-expired
  timestamp straight to the client: a countdown ring at 0s (or, per `Seat.tsx`'s `>`-strict gates, no ring at
  all) with no real decision window before the near-immediate server-side timeout resolved it. Fixed at the
  single point both `broadcastAll` and `handleSnapshot` build a snapshot from: `Actor.deadlinesForBroadcast`
  (`actor_timers.go`) withholds `ActionDeadlineUnixMs`/`ActionBaseDeadlineUnixMs`/`NextHandUnixMs` unless the
  underlying deadline is still strictly in the future, logging a `WARN` when it withholds one — the server-side
  timer is untouched (it already fires immediately for an overdue deadline), only what the player is shown
  changes. Temporary `INFO`-level arm-time logging in `armTurnTimer`/`armNextHandTimer` pairs with that `WARN`
  to show exactly when a deadline went stale; remove once confirmed fixed in prod. Do **not** revert
  `evictActorWhenIdle`'s lease-holding case — it fixes a real memory leak (#36); fix staleness at the broadcast
  boundary instead, since that covers every cause of it, not just eviction.
- **Nothing viewer-independent belongs inside `broadcastAll`'s per-seat loop, and nothing expensive belongs on the
  actor goroutine twice.** Chat and reaction views are built once per broadcast (`activityViews`) and shared; the
  `equityIterations` Monte Carlo is memoized by `(hole, board, opponent count)` per hand (`equityFor`), because a
  single street re-broadcasts on every chat message, act and reconnect signal (#37). That signal is itself debounced:
  `tablews.go` used to dispatch `ReconnectCmd` ahead of *every* inbound frame, keepalive pings included; it now fires
  at most once per `tableconn.SyncInterval` per connection (#58). It cannot simply be skipped for pings — it is also
  the paced caller keeping this instance's `tableconn` entry (45s `EntryTTL`) alive on a table whose only traffic is
  pings, and the 1-minute AFK sweep ticks too slowly to cover that.
- **`handeval` is table-driven — never edit `handeval/ref` without regenerating.** `ref` is the reference evaluator and
  the sole definition of the canonical hand ordering; `tables.bin` is compiled from it by
  `go generate ./internal/engine/handeval/...` and embedded. Changing `ref` without regenerating leaves stale tables
  that silently mis-rank every showdown — nothing fails to compile. `handeval/hashq` is shared by the runtime and the
  generator precisely so the perfect hash can't drift between them; keep it that way.
- Tests: `go test ./... -race`. Integration tests use DynamoDB Local (`docker-compose.test.yml`). Engine logic is
  unit-tested; keep it that way. The normal `handeval` suite uses a deterministic 20,000-hand differential sample; its
  exhaustive proof over all C (52,7) = 133,784,560 hands is behind `-tags exhaustive` (~10 min) — run it after any
  change to `ref`, `hashq`, the generator, or `tables.bin`. Multi-server turn-order/timer changes must also run
  `go test -tags integration -race ./tests/integration -run TestMultiServerFuzz`; retain the existing `-count=15`
  stress run for changes to `internal/table/actor.go` timer paths.
- **Changing an exported signature? `go test ./... -race` will not tell you.** `internal/tablestore/dynamo_test.go` and
  `internal/table/actor_test.go` are `//go:build integration` files inside *unit* packages, so the plain gate never
  compiles them: adding a parameter to `tablestore.Store.CommitAction` passes every local check and then fails the CI
  build. Run `go vet -tags integration ./...` (and `-tags exhaustive`, `-tags load`) after any signature change — it
  compiles every tagged file in seconds without needing DynamoDB Local.

## B9 authz — what is enforced (fixed 2026-07)

Player-facing auth requires a **user token**: non-empty `sub` AND non-empty `sid` (an empty `sid` marks an M2M
`client_credentials` token — ecosystem convention, see `jwtverify.Claims`). Enforced in `authMiddleware`
(`internal/api/v1/auth.go`) and in the WS gateway's token check (`tablews.go`), so M2M credentials can never act as
players. `GET /leaderboard` and `GET /tables/:tableId/hands/:handId/history` now sit behind the same auth middleware
(`leaderboard.go` / `handhistory.go` / `router.go`).
`playerID := claims.Sub` is kept everywhere (IDOR safety). There is still **no scope / kyc / role check** on user
routes — none is defined for poker's user surface today; revisit before real-money mode ships if scopes are added to the
catalog.

## Other known issues (documentation only — see api/README.md)

- **Issue #44 fixed:** `wsAllowedOrigin` (`tablews.go`, backs both WS gateways) used to allow a WebSocket upgrade with
  no `Origin` header at all, even once `CORS_ALLOWED_ORIGINS` was configured — a scripted client with a valid
  first-party token could skip the origin check entirely, which is the wrong prod default for a game whose threat model
  is automation, not cross-site browsers. Now a missing Origin is allowed only when no allow-list is configured (dev —
  `config.Load` fails closed if prod's list is empty); once an allow-list exists, a present and listed Origin is
  required, matching (and reusing) the HTTP CORS allow-list. `cmd/loadtest`, the one non-browser caller of the table
  socket that is meant to run against staging/prod, gained an `-origin` flag for this — it is not exempted from the
  check, it just has to send a real, listed Origin like anything else would.
- **(#38) fixed:** real-money `BuyIn` now enforces `player.RequireAccepted` unconditionally. Both
  `app.newBuyinService` constructors (sandbox and real-money) chain `.WithPlayers(players)`, and `buyin.Service.buyIn`
  calls `s.players.RequireAccepted` before any wallet debit or entitlement charge whenever a players store is wired —
  see `internal/buyin/terms_test.go`'s `TestRealMoneyBuyInRequiresPokerTerms`.
- Issue #31 fixed: `tablemanager.Manager.GetOrCreateActor` no longer serializes the whole instance behind one
  process-global mutex spanning `LoadTable`/`leases.Acquire`/`roomLoader`. It now holds a refcounted per-tableID
  `*sync.Mutex` (`Manager.locks`) across the create path — different tables' cold starts never block each other,
  same-table callers still dedupe to exactly one Actor (T7) — with `Manager.mu` held only for the short `actors`/
  `cancels`/`releases`/`locks` map mutations, never across a network call. See
  `internal/tablemanager/manager_concurrency_test.go`.
- B10 fixed: archiver stream failures now go to an SQS DLQ with a DLQ-depth + `Errors` CloudWatch alarm on
  `ctech-prod-alerts` (`cdk/lib/archiver-stack.ts`, `cdk/lib/alarms.ts`; #30).
- Issue #55 fixed: `cmd/archiver`'s `attributeValueToInterface` no longer routes DynamoDB Number attributes through
  `strconv.ParseFloat`/float64 before archiving — a chip total or payout past 2^53 silently lost precision on the
  permanent audit archive. It now carries the attribute through as `json.Number(v.Number())`, so `json.Marshal` emits
  the original digit string verbatim as a bare JSON number token; any consumer must decode with
  `json.Decoder.UseNumber()` to preserve the same fidelity on read. See
  `cmd/archiver/main_test.go`'s `TestNumericAttributesPreserveIntegerFidelity`.
- B31 fixed by rejection: `leaderboard.Top("achievement_points")` returns an unsupported-metric error instead of
  silently ranking via `gsi_hands_won`; add a `gsi_achievement_points` GSI before re-enabling the metric.
- Issue #63 fixed: the `win_rate` board enforces `leaderboard.MinHandsForWinRateRank` (100) hands per currency mode.
  `gsi_win_rate_pk` is a sparse key — `leaderboard.Store.syncWinRateRankKey` writes it once the counters cross the floor
  and `REMOVE`s it below, so a 1-hand 100% row is never returned by `gsi_win_rate`; `Service.Top` filters sub-floor rows
  again before sorting so none occupies a rank slot. Legacy stale keys clean up lazily on the row's next write — no
  migration job. `hands_won` / `hands_played` boards are untouched.
- **Issue #62 partially fixed.** `GET /leaderboard/me` (`leaderboard.Service.MyRank` / `Store.RankOf`) gives a player
  their exact rank + total via `Select: COUNT` queries instead of the frontend computing rank from whatever page of
  `Top` it happened to fetch (the old bug: `#{data.findIndex+1} de {data.length}` showed page size, not the real total).
  **Still open, deliberately deferred:** the underlying `gsi_hands_won`/`gsi_hands_played`/`gsi_win_rate`
  GSIs remain single-partition per mode (`gsi_*_pk = mode`) — every hand's `IncrementStats` write and every
  `RankOf`/`Top` read still funnel through one DynamoDB partition per mode, and `RankOf`'s full-partition COUNT for
  `total` is itself unbounded in the number of ranked players (capped by `maxRankCountPages`, not fixed by it). The
  issue's proposed fix — a Valkey ZSET mirror per `(mode, metric)`, rebuilt from the GSI on cold start — was out of
  scope for the correctness fix and is not implemented.
- B32 fixed: `ShuffleCommitHash` and the per-card `RootCommitHash` are published from
  `StartHand` on. Complete hands reveal either the full seed (no hidden private cards) or viewer-scoped card+salt proofs
  with hashes for hidden positions and rabbit runout cards. Rabbit-hunt runout cards specifically are withheld from a
  viewer's `ViewFor` snapshot until `Table.RequestRabbitHunt` charges them the table's big blind (sandbox tables only —
  `Table.currencyMode`); see `docs/specs/2026-08-21-paid-rabbit-hunt.md`.
- **The live paid winner-cards reveal is consensual.** `Table.RequestWinnerCards` charges the requester and opens a
  single per-hand `pendingWinnerCards` request (`hand.WinnerCardsRequest`, persisted in `State`); nothing is revealed
  and nobody is paid until the winner sends `accept_winner_cards`. `decline_winner_cards`, the
  `hand.WinnerCardsConsentWindow` (8s, deliberately under `table.NextHandDelay`) expiring, or `StartHand` dealing the
  next hand all refund the requester in full — the fee only splits winner/rake on accept. `winnerCardsAsked` caps it at
  one ask per player per hand (answered or not), so a decline can't be re-asked past. The expiry is enforced server-side
  by `Actor.armWinnerCardsTimer`, re-derived from the persisted `ExpiresAt` in `rearmTimersFromCache`, so an actor
  handoff can never strand a fee. The prompt itself is viewer-scoped in `ViewFor`
  (`Snapshot.PendingWinnerCards`): only the winner and the requester learn a request exists. Rabbit hunt stays
  unilateral on purpose — that secret belongs to the deck, not to another player. See
  `docs/specs/2026-08-24-pay-to-see-cards-consent.md`.
- **Exit mid-hand no longer waits for the turn.** `request_exit` (WS) pauses the player immediately and marks them
  `PendingExit`. `Table.RequestExit` folds them via `SitOutForActor`
  only if they're currently the player on the clock (`Round.Act` has no turn-order check of its own, so calling
  `SitOutForActor` unconditionally would force-fold an exiting BB/SB before their turn ever comes back — breaking an
  uncontested win they're still owed). Otherwise they're left untouched: `Actor.processPendingExitAutoFolds` folds them
  the instant their own turn actually arrives, and `Actor.removeEligiblePendingExits` sweeps and cashes out every
  `PendingExit` player no longer `DealtIntoCurrentHandForActor`. Both are hooked into `broadcastAll` (the same
  per-commit point `armTurnTimer`/inline preselections already use) — not gated behind
  `claimHandHooks`, since `RemovePlayerForActor`'s conditional commit already makes a duplicate sweep a safe no-op.
  `dealtIntoCurrentHand`/`handOrder` stays true through the entire post-hand
  `Complete`-stage window, so removal only actually happens once the *next* hand's `StartHand`
  runs, not synchronously with the hand that completes. `cancel_exit` reverses it before either commits. See
  `docs/plans/2026-08-26-exit-mid-hand-design.md`. **Every system removal path (`removeEligiblePendingExits`,
  `removeIdlePlayersBetweenHands`, `handleAFKSweep`, `handleKickTimeout`) generates a fresh
  `Actor.newSettlementNonce` and forwards it verbatim through both `systemSettlementIntent` and `onPlayerRemoved`
  — it is what makes the co-committed `poker_pending_cashouts` key
  (`roomID#playerID#system_leave#reason#nonce`) unique per removal.** Without it, keying on `(room, player,
  reason)` alone meant the SECOND system removal at the same table hit the leftover create-only row's
  `attribute_not_exists` and cancelled the whole seat-removal transaction — the player was wedged
  `pending_exit=true` until an idle sweep (different reason → different key) caught them (fixed 2026-09-03,
  `docs/specs/2026-09-03-system-leave-settlement-key-collision.md`). The two calls must share the same nonce or
  `reconcile` credits the wallet twice under divergent keys. Mirrors the client nonce `buyin.Service.CashOut`
  already appends.
- **Every list endpoint returns the `sendPage` envelope** (`{data, has_next, next_cursor, has_previous,
  previous_cursor}` — `internal/api/v1/helpers.go`), including fixed in-memory catalogs, which simply sit permanently on
  their only page. Purchase history (`sandbox-purchase`, `reaction-purchase`, `cosmetic-purchase/:kind`) pages for real
  off a DynamoDB `ExclusiveStartKey`. `cosmeticpurchase.Store.List` **must** stay filtered by `kind`: deck and felt
  share one purchase table, and the unfiltered query is what made the store report "8 de 6 liberados".
- **Ownership is read from the entitlement tables, never from purchase history.** `EntitlementStore.OwnedIDs` backs the
  `owned` flag on every catalog entry (`cosmeticpurchase`/`reactionpurchase` `ListCatalog`), because a buy/refund/buy
  cycle leaves history rows no client can safely reduce to ownership.
- `internal/wsdrain` tracks this process's live sockets so `OnStop` can send each a 1001 "going away" close before
  `ShutdownWithContext` force-closes them mid-deploy. See `docs/specs/2026-08-24-graceful-ws-shutdown-on-deploy.md`.
- `internal/handreveal` (`poker_hand_reveals` + `poker_hand_reveal_payments`) extends the live paid winner-cards reveal
  (`Table.RequestWinnerCards`) to hand history — non-consensual by design, since the hand is archived and there is no
  winner still at the table to ask: `POST`/`GET /players/me/hands/:handId/reveal-winner`. Sandbox-only, one archive row
  per eligible hand (won without showdown, single winner), written by the same hand-complete/hand-updated hooks that
  already write `sessionlog.HandItem`. Payment moves through `walletclient` (debit buyer, credit winner half), not a
  local DynamoDB transaction — sandbox balance lives in ctech-wallet. See
  `docs/specs/2026-08-21-pay-to-see-winner-cards-history.md`.
- A separate audit (`docs/plans/2026-07-19-api-audit-remediation.md`) covers H1–H4 / M1–M7 / L1–L6 / E1–E3 / S1–S7. Some
  fixes are already in code (actor re-resolve `tablews.go:185-198`, prod Valkey fail-fast, HTTP rate limiters
  `router.go:39-41`); others are not — verify before relying on them.
- Issue #45 fixed: `RateLimiter.allowRedis` (`internal/api/v1/ratelimit.go`) used to `INCR` then `EXPIRE`
  only when the counter had just been created (`n == 1`); if the process died or the `EXPIRE` call failed in between,
  the key kept counting with no TTL and the bucket never reset. Both steps now run as one atomic Lua script
  (`incrAndBoundTTLScript`) that increments the key and, on every hit (not just the first), re-applies the TTL whenever
  `TTL` reports the key has none — so a key that somehow lost its expiry self-heals on its very next hit instead of
  staying stuck. See
  `internal/api/v1/ratelimit_test.go`'s `TestRateLimiterAllowRedisRecoversMissingTTL`.
- **Issue #68 fixed:** `sessionlog.OpponentSummary.AvatarURL` is still captured (denormalized) at hand-complete, but
  `playerHandlers.handHistory`/`handByID` (`internal/api/v1/player.go`) no longer serve that frozen copy as-is —
  `resolveOpponentAvatars` re-resolves every opponent's `AvatarURL` from their *current* profile via
  `player.Service.GetMany` (chunked at `player.MaxBatchProfileIDs` per call) before the response is sent, so a hand
  recorded before a later `ClearAvatar` (or an avatar-report block) never serves a URL pointing at a deleted object.
  `player.AvatarURL` already treats a blocked/missing avatar as absent, so a cleared or since-deleted opponent profile
  now resolves to `""` instead of a stale, 404ing link — same as the player's own profile response. The helper is now
  called `resolveOpponentProfiles` and resolves `Name` the same way — see the #64 entry below.
- **Issue #64 fixed by read-time resolution — no backfill, nothing fans out on rename.** Denormalized display names are
  still written (they are the fallback when a profile no longer resolves) but no consumer serves its own copy any more:
  `player.Service.GetMany` is the single resolution point (it now chunks at `player.MaxBatchProfileIDs` internally, so
  no caller has to), `resolveOpponentProfiles` (`internal/api/v1/player.go`) overwrites `OpponentSummary.Name` +
  `AvatarURL` on every hand-history read, and `leaderboardHandlers.resolveNames` (`internal/api/v1/leaderboard.go`)
  overwrites `Entry.PlayerName` on `GET /leaderboard` and `/leaderboard/me` — one `BatchGetItem` per rendered page.
  Existing rows are therefore correct on their next read with no migration job. Live seats are the one write-side push:
  `POST /players/me` with a new `name` calls `tableIdentityPusher.push` (`internal/api/v1/tableidentity.go`), which
  finds the player's current table from `presence` and dispatches `SetIdentityCmd` so opponents see the rename without
  a reconnect. `identityCmdFor` in that file is now the only place a `SetIdentityCmd` payload is assembled (the WS
  gateway uses it too) — `hand.Table.SetPlayerIdentityForActor` replaces name/avatar/badge as one unit, so pushing a
  single field would blank the other two. Public hand shares already anonymized `ReplaySeat.Name`; that half of the
  issue needed no change.
- **Issue #72 fixed:** `achievements.Store.StampTierUnlock` stamps `unlocked_at` on a progress row when
  `Service.RecordHand` reports a `TierUnlock`, surfaced as `unlocked_at` on `PlayerAchievementProgress` and on
  `AchievementState` (`/players/me/achievements` and the summary endpoint). Legacy rows carry no stamp and report an
  empty string — a still-locked or pre-change row, never an error. A replayed hand hook stamps nothing: it is stopped by
  `ClaimHandCounters`, and past that a counter that crosses no threshold reports no unlock. A stamp failure is logged,
  not returned — the counters are already committed and must not be retried.
- **Issue #65 fixed:** `matchup.Store.RecordHand` no longer writes one 72-item `TransactWriteItems` call per hand. It
  chunks pairs at `maxPairsPerTx` (12 pairs = 24 items), so a 9-max table's C(9,2)=36 pairs commit as 3 bounded
  transactions. The atomicity that actually mattered is per-pair, not per-hand: each pair's create-only guard rides in
  the same transaction as that pair's increment, so no pair is ever double-counted and a partially-applied hand is
  safely completed by a retry (landed chunks fail their guards and are skipped). Moving matchup off the
  hand-completion critical path was #61's detached gamification pipeline, already done.
- **Issue #70 fixed: a session's `buyin_amount` is now updated with an atomic conditional ADD, not a
  read-modify-write.** `buyin.Service.buyIn`'s rebuy path used to `FindOpenSession`, add `amount` to the decoded
  `BuyinAmount` in Go, and `PutItem` the whole item back — two concurrent rebuys for the same player (a client
  double-submit, or the auto-rebuy sweep racing a manual rebuy) could both read the same starting value and one addition
  would be silently lost, undercounting money actually put at risk (a responsible-gaming/spend-tracking correctness
  issue: `RealityCheck`/`SessionRecap` both derive their "session result" from this figure). `AddBuyin`
  (`internal/sessionlog/store.go`) now runs a single `TransactWriteItems` call: an `ADD buyin_amount :amt` update
  conditioned on `attribute_exists(pk) AND ended_at = :zero` (a rebuy losing a race with a concurrent cash-out never
  reopens an already-closed session), paired with a create-only idempotency guard row keyed by the same composite key
  `buyin.Service` already uses for the wallet debit — so a retried buy-in can never double-count the rebuy. The guard
  lives in the same `poker_player_sessions` table under a namespaced partition key (`buyinguard#<player_id>`, never a
  real player id), so it can never surface as a bogus row in `ListSessions`/`FindOpenSession`/
  `FindLatestOpenSession`, all of which query by the literal player id. Every buy-in path (initial seat, manual rebuy,
  re-entry after busting, the post-hand auto-rebuy sweep) funnels through this same `buyIn` call, so all of them get the
  fix. See `internal/sessionlog/store_integration_test.go`'s `TestAddBuyin*` tests.
- Issue #73 fixed: `social.Event` still only stores `actor_id` — no name/avatar denormalization — but `GET
  /social/inbox` now resolves every distinct actor on the page through one `player.Service.GetMany` batch call
  (`socialHandlers.hydrateInboxActors`, `internal/api/v1/social.go`) and adds `actor_name`/`actor_avatar_url` to the
  response only. This closes the "Visitante" bug (a stranger's `friend_request` or a `table_invite` couldn't be named
  because the frontend's `nameResolver` only knew actors already in the friends/requests lists) without reintroducing
  the #64 name-drift failure mode a write-time denormalized copy would have.

## Layout

`cmd/{server,archiver,reconcile,tablecleanup,handreplay}` ·
`internal/api/v1` (+ `api/v1/proto`, generated from `../proto/poker.proto`) · `internal/app` (Fx wiring) ·
`internal/engine/{hand,betting,deck,equity,handeval,sidepots}` ·
`internal/{table,tablemanager,tablestore,tablelease,roomstore}` ·
`internal/{buyin,walletclient,reconcile,entitlement}` (money) ·
`internal/{player,playernotes,pokerstats,matchup,sessionlog,handshare,handreveal,highlights}` (player-scoped data) ·
`internal/{leaderboard,achievements,dailyreward}` (gamification) ·
`internal/{botcheck,chatfilter,config,problem,wsdrain}` · `tests/{integration,load}`.

Transport is **binary protobuf** on both gateways (`GET /v1.0/tables/:id/ws`, `GET /v1.0/ws`), with the access token
sent as the first frame after upgrade and a 32 KiB frame cap.

## Mandatory Documentation Policy

**Every code change MUST be documented.**

There are NO exceptions.

Any modification affecting behavior, architecture, APIs, integrations, configuration, deployment, security, business
rules, or developer workflow MUST include the corresponding documentation update in the same change.
