# api/ — AGENTS.md (for autonomous agents)

Goal: extend the poker game server. Sandbox is live; the real-money path is built and gated at runtime
by `REAL_MONEY_ENABLED` (off by default) — do not assume it is on, and do not remove the gate.

## Hard rules

1. **Reuse `gopkg.aoctech.app/api-commons`** (`jwtverify`, `ws.Registry`, `cache.Backend`, `problem`,
   `dynamo`, `lock`, `ratelimit`). Do not re-implement shared CTech primitives.
2. **No magic strings** — name table names, field names, route paths, event types, config keys.
3. **DynamoDB conditional writes are the correctness mechanism.** Every mutation goes through
   `tablestore.CommitAction` with a `version` guard + idempotency key. Never read-then-write table state
   outside that path. On a version conflict, discard the uncommitted mutation and re-read — do not
   commit a mutation the store rejected. `tablelease` is latency-only; never add lease-correctness.
4. **Identity = JWT `sub`.** Always derive `playerID` from `claims.Sub`; never trust a client-supplied
   player id. Auth requires `sub` **and** `sid`; an empty `sid` is an M2M token and must stay rejected.
5. **Keep the `currency_mode` boundary in `buyin`.** One ledger per room, no path that converts sandbox
   chips to real balance or back. Money ordering: debit-then-seat on buy-in, remove-then-credit on
   cash-out, with failures queued to `poker_pending_cashouts` for `cmd/reconcile`.
6. **The wire is binary protobuf** (`../proto/poker.proto` → `internal/api/v1/proto`). WS auth is the
   **first frame**, not a header. Private rooms re-check the share code with a constant-time compare.
7. **Fiber hijacks the WS connection** — copy any `c.Params`/locals string **before** the socket
   goroutine touches it, or you get a use-after-reuse bug (this really happened).
8. **Hidden information only leaves through `Table.ViewFor(viewerID)`**, which masks unseen hole cards
   as `"back"`. Fan-out is keyed `<tableID>#<viewerID>`. Put visibility rules there, never in a handler.
9. **Never widen the fairness seed reveal.** The seed is published only for hands where nothing stayed
   hidden; every other hand gets the seed-less per-position proof. Widening it retroactively exposes
   mucked hole cards — that is a security regression, not a feature.
10. **`handeval` is table-driven.** Never edit `handeval/ref` without `go generate
    ./internal/engine/handeval/...` — stale `tables.bin` silently mis-ranks showdowns and still compiles.

## Tests

`go test ./... -race`. Integration: `docker compose -f docker-compose.test.yml up` (DynamoDB Local),
then `tests/integration`. Load/chaos harness is behind `-tags load`. Engine code is heavily
unit-tested — preserve and extend tests for betting/sidepots/eval/shuffle changes. The exhaustive
evaluator proof over all C(52,7) hands is behind `-tags exhaustive` (~10 min); run it after any change
to `ref`, `hashq`, the generator, or `tables.bin`.

## Where things live

- Routes/wiring: `internal/api/v1/*` (`router.go` mounts groups, `auth.go` is the authz gate),
  `internal/app` (Fx DI).
- Real-time: `internal/api/v1/tablews.go` (both gateways), `internal/table/*` (per-table actor),
  `internal/tablemanager/*` (actor registry).
- Storage: `internal/tablestore/*`, `internal/roomstore/*`, `internal/sessionlog/*`.
- Engine (pure): `internal/engine/{hand,betting,sidepots,equity,deck,handeval}`.
- Gamification: `internal/{leaderboard,achievements,dailyreward}`.
- Money: `internal/{buyin,walletclient,reconcile}`.
- Player-scoped: `internal/{player,playernotes,pokerstats,handshare}`.
- Integrity/support: `internal/{botcheck,chatfilter,metrics,config,problem}`.

## Known issues (do not paper over)

No WAF at the CloudFront edge · no ASG lifecycle hook (drain relies on the default shutdown grace
period) · no DLQ on either EventBridge Scheduler target · real-money buy-in skips the poker-terms
check · two ctech-account scopes still missing for wallet verification calls.

B9, B10, B31 and B32 are **fixed** — older notes calling them open are stale. See `README.md` and
`../docs/README.md`.
