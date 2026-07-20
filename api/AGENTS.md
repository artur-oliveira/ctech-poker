# api/ — AGENTS.md (for autonomous agents)

Goal: extend the sandbox poker game server. **Real-money mode is out of scope (Phase 5).**

## Hard rules

1. **Reuse `gopkg.aoctech.app/api-commons`** (`jwtverify`, `ws.Registry`, `cache.Backend`,
   `problem`). Do not re-implement shared CTech primitives.
2. **No magic strings** — name table names, field names, route paths, event types, config keys.
3. **DynamoDB conditional writes are the correctness mechanism.** Every mutation goes through
   `tablestore.CommitAction` with a `version` guard + idempotency key. Never read-then-write
   table state outside that path. `tablelease` is latency-only — do not add lease-correctness.
4. **Identity = JWT `sub`.** Always derive `playerID` from `claims.Sub`; never trust a
   client-supplied player id.
5. **Keep sandbox isolation.** `buyin` must reject non-`sandbox` `CurrencyMode`. Do not add a
   real-money wallet path without hold/capture endpoints in ctech-wallet.
6. **WebSocket auth is a first frame** (`{"token","share_code"}`), not a header
   (`tablews.go:49`). Private rooms re-check the share code.

## MUST-FIX KNOWN RISK (B9)

Authz is `sub`-only (`auth.go:20`); `/leaderboard` is unauthenticated (`leaderboard.go:11`);
M2M tokens are not distinguished from users. Do not treat this as accepted. Any change that
touches auth should move toward scope/client-type checks. Keep `playerID := claims.Sub`.

## Tests

`go test ./... -race -coverprofile=coverage.out`. Integration: `docker compose -f
docker-compose.test.yml up` (DynamoDB Local) then run `tests/integration`. Engine code is
heavily unit-tested — preserve/extend tests for betting/sidepots/eval/shuffle changes.

## Where things live

- Routes/wiring: `internal/api/v1/*` (router.go mounts groups; `auth.go` = the authz gate).
- Real-time: `internal/api/v1/tablews.go` (WS gateway), `internal/table/*` (per-table actor),
  `internal/tablemanager/*` (actor registry).
- Storage: `internal/tablestore/*`, `internal/roomstore/*`.
- Engine (pure): `internal/engine/{hand,betting,sidepots,equity,deck}`.
- Gamification: `internal/leaderboard`, `internal/achievements`, `internal/roulette`.
- Ledger: `internal/walletclient` (sandbox credit/debit only), `internal/buyin`.

## Known issues to be aware of (do not paper over)

B10 (archiver no DLQ), B31 (`Top("achievement_points")` wrong GSI), B32 (no commit-reveal
publish/reveal endpoint). See `api/README.md` and `docs/plans/2026-07-19-api-audit-remediation.md`.
