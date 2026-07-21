# api/ — CLAUDE.md

Go real-time poker game server (Fiber v3 + `fasthttp/websocket` + DynamoDB + Valkey).
**Sandbox (play-money) mode is implemented end-to-end. Real-money mode & Hardening (Phase 5 Tasks 1–12) is FULLY IMPLEMENTED**
(gated on `REAL_MONEY_ENABLED=true` + `LEGAL_SIGNOFF_REF` config, see `internal/config/config.go:44-51`).

## Conventions (follow these)

- **Reuse `gopkg.aoctech.app/api-commons`** for JWT verify (`jwtverify`), WebSocket
  registry (`ws.Registry`), cache backend (`cache.Backend`), and problem responses
  (`problem`). Do NOT hand-roll these.
- **Named constants / no magic strings.** DynamoDB table/field names, route paths, event
  type strings, and config keys live as named identifiers, not literals at call sites.
- **Correctness = DynamoDB conditional writes.** Every mutated action commits via
  `tablestore.CommitAction` with a `version` equality `ConditionExpression` + per-action
  idempotency guard. Never read-then-write against table state.
- **`tablelease` is latency-only**, not correctness. Never add lease-based correctness logic.
- **Player identity comes from the JWT `sub`** — derive `playerID` from claims, never trust a
  client-supplied id (prevents IDOR).
- **Sandbox isolation is load-bearing:** `buyin` must keep rejecting non-`sandbox`
  `CurrencyMode` (`ErrUnsupportedCurrencyMode`). Do not add a real-money wallet path here
  without ctech-wallet's hold/capture endpoints first.
- Tests: `go test ./... -race`. Integration tests use DynamoDB Local (`docker-compose.test.yml`).
  Engine logic is unit-tested; keep it that way.

## ⚠️ KNOWN RISK — B9 authz gap (fix it, do not accept it)

`internal/api/v1/auth.go:20` is the **only** authz check: `claims.Sub == "" → reject`.
There is **no scope / kyc / role check**, `GET /leaderboard` is unauthenticated
(`leaderboard.go:11`), and **M2M credentials are not distinguished from user credentials**.
A token with any non-empty `sub` (including an M2M client credential with no session `sid`)
satisfies the guard. Not exploitable for real funds today (sandbox-only), but it **must** be
closed before real-money mode ships. When fixing: keep `playerID := claims.Sub` for IDOR
safety, add a scope/client-type distinction, and decide whether `/leaderboard` needs auth.

## Other known issues (documentation only — see api/README.md)

- B10 archiver Lambda has no DLQ; B31 `leaderboard.Top("achievement_points")` hits the wrong
  GSI; B32 commit-reveal is unverifiable (no publish/reveal endpoint).
- A separate audit (`docs/plans/2026-07-19-api-audit-remediation.md`) covers H1–H4 / M1–M7 /
  L1–L6 / E1–E3 / S1–S7. Some fixes are already in code (actor re-resolve `tablews.go:185-198`,
  prod Valkey fail-fast, HTTP rate limiters `router.go:39-41`); others are not — verify
  before relying on them.

## Layout

`cmd/{server,archiver,handreplay}` · `internal/{api,app,engine,table,tablemanager,
tablestore,roomstore,buyin,player,leaderboard,achievements,roulette,walletclient,
tablelease,chatfilter,config,problem}` · `tests/integration`.
