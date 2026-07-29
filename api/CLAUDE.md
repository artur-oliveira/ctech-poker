# api/ — CLAUDE.md

Go real-time poker game server (Fiber v3 + `fasthttp/websocket` + DynamoDB + Valkey). **Sandbox (play-money) mode is
implemented end-to-end with a 2.5% rake engine. Real-money mode (Phase 5) is fully implemented end-to-end under the
Brazil-legal fixed-fee model (no rake, flat entry fee per tier):** `POST /rooms` accepts `currency_mode: "real"`,
validates blinds against the 10-tier fee catalog, and stores the fixed `EntryFeeCents` on the room. Every real-money
buy-in and rebuy charges that entry fee to the player's real withdrawable wallet via `walletclient.DebitReal`.
Failed fee debits are queued to the same retry table (`poker_pending_cashouts` with `Kind: "fee_debit"`) for Lambda
reconciliation retries. **Still blocking, found 2026-07-25 while verifying cross-repo:**
1. ctech-wallet's scope catalog (`ctech-account/api/internal/scopes/catalog.go`) has no `internal:wallet:game-status` entry, so no M2M client can ever be granted the scope `ctech-wallet`'s `GET /wallet/game/status/:user_id` requires.
2. Poker's M2M client has never been granted the `internal:wallet:debit-real` scope in `ctech-account`'s catalog.
Both are data/config actions in `ctech-account`, not code changes in this repo.
Also unresolved (re-verified 2026-07-28):
- No ASG lifecycle hook exists in either `ctech-cdk`'s `PrivateIpv4Ec2Service` or this repo's `cdk/lib/api-stack.ts` — `tablemanager.DrainAndRelease` relies on the EC2 default shutdown grace period, not a guaranteed drain window.
- The real-money buy-in path skips the poker-terms-acceptance check the sandbox path performs (`internal/app/app.go`).
- No WAF at the CloudFront edge; application rate limits (`internal/api/v1/ratelimit.go`) and Turnstile are the only protection.
- Neither EventBridge Scheduler target (`cmd/reconcile`, `cmd/tablecleanup`) has a DLQ.

## Conventions (follow these)

- **Reuse `gopkg.aoctech.app/api-commons`** for JWT verify (`jwtverify`), WebSocket registry (`ws.Registry`), cache
  backend (`cache.Backend`), and problem responses (`problem`). Do NOT hand-roll these.
- **Named constants / no magic strings.** DynamoDB table/field names, route paths, event type strings, and config keys
  live as named identifiers, not literals at call sites.
- **Correctness = DynamoDB conditional writes.** Every mutated action commits via
  `tablestore.CommitAction` with a `version` equality `ConditionExpression` + per-action idempotency guard. Never
  read-then-write against table state.
- **`tablelease` is latency-only**, not correctness. Never add lease-based correctness logic.
- **Player identity comes from the JWT `sub`** — derive `playerID` from claims, never trust a client-supplied id
  (prevents IDOR).
- **The `currency_mode` boundary is load-bearing.** `buyin` routes to exactly one ledger per room and must never let
  sandbox chips reach the real wallet or vice versa — enforce it in `buyin`, not at the handler. The real path is
  built; what gates it at runtime is `REAL_MONEY_ENABLED` + `LEGAL_SIGNOFF_REF`, checked fail-closed in
  `config.Load`. (Earlier revisions of this file said to reject non-`sandbox` outright — that is no longer the rule.)
- **Money ordering is deliberate**: debit-then-seat on buy-in, remove-then-credit on cash-out. Anything that can
  fail after chips moved goes to `poker_pending_cashouts` for the `cmd/reconcile` sweeper. Keep new money paths in
  that shape rather than inventing a compensating transaction per call site.
- **Hidden information never leaves `ViewFor`.** `Table.ViewFor(viewerID)` is the single place that decides
  per-viewer visibility, masking unseen hole cards as `"back"` before serialisation; fan-out is keyed
  `<tableID>#<viewerID>` so two seats cannot share a snapshot. Add visibility rules there, never in a handler.
- **The fairness reveal is asymmetric on purpose.** The server seed is published only when nothing stayed hidden
  (a real showdown). Every other hand gets the seed-less per-position proof (`fairnessProofFor`). Widening seed
  publication would retroactively expose mucked hole cards — treat it as a security change, not a feature.
  `FairnessProofs` is set only on the copy handed to hooks, never on `Table.lastOutcome`, which is persisted with
  every table-state write.
- **`handeval` is table-driven — never edit `handeval/ref` without regenerating.** `ref` is the reference evaluator
  and the sole definition of the canonical hand ordering; `tables.bin` is compiled from it by
  `go generate ./internal/engine/handeval/...` and embedded. Changing `ref` without regenerating leaves stale tables
  that silently mis-rank every showdown — nothing fails to compile. `handeval/hashq` is shared by the runtime and the
  generator precisely so the perfect hash can't drift between them; keep it that way.
- Tests: `go test ./... -race`. Integration tests use DynamoDB Local (`docker-compose.test.yml`). Engine logic is
  unit-tested; keep it that way. The normal `handeval` suite uses a deterministic 20,000-hand differential sample;
  its exhaustive proof over all C(52,7) = 133,784,560 hands is behind `-tags exhaustive` (~10 min) — run it after
  any change to `ref`, `hashq`, the generator, or `tables.bin`.

## B9 authz — what is enforced (fixed 2026-07)

Player-facing auth requires a **user token**: non-empty `sub` AND non-empty `sid` (an empty `sid` marks an M2M
`client_credentials` token — ecosystem convention, see `jwtverify.Claims`). Enforced in `authMiddleware`
(`internal/api/v1/auth.go`) and in the WS gateway's token check (`tablews.go`), so M2M credentials can never act
as players. `GET /leaderboard` and `GET /tables/:tableId/hands/:handId/history` now sit behind the same auth
middleware (`leaderboard.go` / `handhistory.go` / `router.go`).
`playerID := claims.Sub` is kept everywhere (IDOR safety). There is still **no scope / kyc / role check** on user
routes — none is defined for poker's user surface today; revisit before real-money mode ships if scopes are added
to the catalog.

## Other known issues (documentation only — see api/README.md)

- B10 fixed: archiver stream failures now go to an SQS DLQ with a CloudWatch alarm (`cdk/lib/archiver-stack.ts`).
- B31 fixed by rejection: `leaderboard.Top("achievement_points")` returns an unsupported-metric error instead of
  silently ranking via `gsi_hands_won`; add a `gsi_achievement_points` GSI before re-enabling the metric.
- B32 fixed: `ShuffleCommitHash` and the per-card `RootCommitHash` are published from
  `StartHand` on. Complete hands reveal either the full seed (no hidden private cards) or
  viewer-scoped card+salt proofs with hashes for hidden positions and rabbit runout cards.
- A separate audit (`docs/plans/2026-07-19-api-audit-remediation.md`) covers H1–H4 / M1–M7 / L1–L6 / E1–E3 / S1–S7. Some
  fixes are already in code (actor re-resolve `tablews.go:185-198`, prod Valkey fail-fast, HTTP rate limiters
  `router.go:39-41`); others are not — verify before relying on them.

## Layout

`cmd/{server,archiver,reconcile,tablecleanup,handreplay}` ·
`internal/api/v1` (+ `api/v1/proto`, generated from `../proto/poker.proto`) · `internal/app` (Fx wiring) ·
`internal/engine/{hand,betting,deck,equity,handeval,sidepots}` ·
`internal/{table,tablemanager,tablestore,tablelease,roomstore}` ·
`internal/{buyin,walletclient,reconcile}` (money) ·
`internal/{player,playernotes,pokerstats,sessionlog,handshare}` (player-scoped data) ·
`internal/{leaderboard,achievements,dailyreward}` (gamification) ·
`internal/{botcheck,chatfilter,metrics,config,problem}` · `tests/{integration,load}`.

Transport is **binary protobuf** on both gateways (`GET /v1.0/tables/:id/ws`, `GET /v1.0/ws`), with the access token
sent as the first frame after upgrade and a 32 KiB frame cap.
