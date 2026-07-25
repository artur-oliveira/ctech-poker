# api/ — CLAUDE.md

Go real-time poker game server (Fiber v3 + `fasthttp/websocket` + DynamoDB + Valkey). **Sandbox (play-money) mode is
implemented end-to-end. Real-money mode (Phase 5) is now reachable in-repo (fixed 2026-07-25):** `POST /rooms`
accepts `currency_mode: "real"` (`internal/api/v1/roomdto.go`, `internal/api/v1/rooms.go:createRoom`), gated on
`cfg.RealMoneyEnabled`; `GET /rooms/stakes?currency_mode=real` serves the BRL-cent catalog under the same gate.
`newBuyinService`'s real-money branch now chains `.WithPlayers(players)` so real-money buy-ins get the same
terms-of-service gate sandbox does. CDK (`cdk/lib/api-stack.ts`) fetches `REAL_MONEY_ENABLED`/`LEGAL_SIGNOFF_REF`
from SSM (`/ctech/{env}/poker/real-money-enabled`, `.../legal-signoff-ref`) on every boot. **Still blocking, found
2026-07-25 while verifying cross-repo:** ctech-wallet's scope catalog (`ctech-account/api/internal/scopes/catalog.go`)
has no `internal:wallet:game-status` entry, so no M2M client can ever be granted the scope
`ctech-wallet`'s `GET /wallet/game/status/:user_id` requires — poker's `IsGamblingActivated` check can never
succeed until ctech-account adds that scope. Also unresolved: no ASG lifecycle hook exists in either
`ctech-cdk`'s `PrivateIpv4Ec2Service` or this repo's `cdk/lib/api-stack.ts` — `tablemanager.DrainAndRelease`
relies on the EC2 default shutdown grace period, not a guaranteed drain window. See
`docs/plans/2026-07-19-poker-phase5-realmoney-and-hardening.md`'s Status section for the full task-by-task
verdict (predates this fix; treat the blockers above as current, the rest as historical).

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
- **Sandbox isolation is load-bearing:** `buyin` must keep rejecting non-`sandbox`
  `CurrencyMode` (`ErrUnsupportedCurrencyMode`). Do not add a real-money wallet path here without ctech-wallet's
  hold/capture endpoints first.
- Tests: `go test ./... -race`. Integration tests use DynamoDB Local (`docker-compose.test.yml`). Engine logic is
  unit-tested; keep it that way.

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
- B32 fixed: `ShuffleCommitHash` published in every WS snapshot from `StartHand` on, `ShuffleServerSeedHex`
  revealed at `Stage == Complete` (`snapshot.go:160-165`) — commit-reveal is now client-verifiable.
- A separate audit (`docs/plans/2026-07-19-api-audit-remediation.md`) covers H1–H4 / M1–M7 / L1–L6 / E1–E3 / S1–S7. Some
  fixes are already in code (actor re-resolve `tablews.go:185-198`, prod Valkey fail-fast, HTTP rate limiters
  `router.go:39-41`); others are not — verify before relying on them.

## Layout

`cmd/{server,archiver,handreplay}` · `internal/{api,app,engine,table,tablemanager,
tablestore,roomstore,buyin,player,leaderboard,achievements,roulette,walletclient,
tablelease,chatfilter,config,problem}` · `tests/integration`.
