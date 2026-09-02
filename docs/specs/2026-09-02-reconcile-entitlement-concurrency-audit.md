# Reconcile + entitlement concurrency / lost-update audit (#122)

Date: 2026-09-02
Scope: `api/internal/reconcile/`, `api/internal/entitlement/`, and their interaction
with `api/internal/buyin/`, `api/internal/walletclient/`, `api/cmd/reconcile/`,
`api/cmd/tablecleanup/`.

Two related PRs were already in flight and were read first so their work is not
duplicated here:

- **PR #130** (`#32`) — `cmd/reconcile` per-entry `Attempts` counter, `manual_review`
  quarantine, aggregated error so the Lambda invocation fails and reaches the DLQ,
  `config.LoadForLambda` legal gate.
- **PR #139** (`#40`) — `entitlement.Store.Claim` atomic winner read-back
  (`ReturnValuesOnConditionCheckFailure: ALL_OLD`), `buyin.Service.confirmFeeCharged`
  as the single "fee is covered" decision (keyed on the entitlement's own
  `poker_pending_cashouts` recovery row, not on "a row exists"), and
  `cmd/tablecleanup` real-money settle-and-archive.

This audit covers the **broader** lost-update / non-idempotency surface across
those packages and their callers.

---

## Finding 1 (NEW — FIXED here): `entitlement.Store.Rebind` was not compare-and-swap on the field it mutates

### The bug

`Rebind` moved `bound_table_id` with

```
UpdateExpression:    SET bound_table_id = :new
ConditionExpression: attribute_exists(pk) AND expires_at > :now
```

The condition guarded existence and expiry but **not** the current value of the
attribute being changed. This is a read-then-write against mutable state without a
version/value check — exactly the pattern `tablestore` (see its package doc and
`ErrVersionConflict`) exists to prevent everywhere else in the codebase.

### Why it is a lost update / double-admission

`buyin.Service.resolveEntitlement` picks a rebind candidate from a snapshot read
(`ActiveFor`), then calls `Rebind(playerID, e.OriginTableID, room.ID)`. The row is
keyed by `(playerID, OriginTableID)`; `OriginTableID` is immutable, so **every**
concurrent buy-in for the same player + same origin entitlement targets the same
row regardless of where `bound_table_id` currently points.

Race: the player has one paid entitlement (origin table A, currently bound to A).
A is (or briefly appears) unavailable. Two concurrent buy-ins arrive, one at table
B and one at table C (same tier). Both read `bound_table_id = A`, both treat the
entitlement as a rebind candidate, both call `Rebind`:

1. buy-in @B: `Rebind(player, A, B)` → condition `exists AND not expired` holds →
   `bound_table_id := B`, returns `nil`.
2. buy-in @C: `Rebind(player, A, C)` → condition **still** holds (row exists, not
   expired — it does not care that `bound_table_id` is now B) → `bound_table_id := C`,
   returns `nil`.

Both `Rebind` calls succeed. Both callers then run `confirmFeeCharged` against the
**same** entitlement (`e.OriginTableID` + `e.CreatedAt` unchanged by rebind), find
the same one fee-recovery row resolved, and seat the player **for free at both
tables B and C off a single paid entry fee**. `bound_table_id` ends up pointing at
whichever `Rebind` raced last; the other free seat is already committed and no
longer gated by the entitlement (once seated, table state is authoritative).

This is orthogonal to PR #139: `confirmFeeCharged` correctly confirms the fee
*cleared*, but the fee only ever pays for **one** table, and the rebind race lets
one cleared fee authorize simultaneous entry to two.

### The fix

`Rebind` now takes `expectedBoundTableID` (the caller's just-read
`e.BoundTableID`) and the condition is a full CAS:

```
attribute_exists(pk) AND expires_at > :now AND bound_table_id = :old
```

Exactly one racing `Rebind` wins; every loser gets `ErrNotFound` (already the
"lost the race — try the next candidate or charge a fresh fee" path in
`resolveEntitlement`), so the losing table correctly charges its own fee.

Files:
- `api/internal/entitlement/store.go` — `Rebind` signature + condition.
- `api/internal/buyin/service.go` — `entitlementStore` interface + call site pass
  `e.BoundTableID`.
- `api/internal/entitlement/store_integration_test.go` — existing `Rebind` tests
  updated for the new arg; new `TestRebindIsCompareAndSwapOnBoundTableID`
  (8 goroutines racing to rebind one entitlement to 8 different tables → exactly
  one success, one persisted row).

---

## Finding 2 (reviewed — NO new bug): sweeper vs. live caller on the same `poker_pending_cashouts` row

`cmd/reconcile.run` and the request-path (`buyin.Service.settle` /
`confirmFeeCharged` / `cmd/tablecleanup`) can both process the same recovery row.
This is safe as built:

- **Grace period.** `ListUnresolved(gracePeriod)` filters out any row whose
  `RecordedAt` is newer than `now - 2m`, so the sweeper does not touch a row while
  its originating request is still plausibly in flight. This is a heuristic, not an
  enforced ordering, but the layers below make the overlap harmless anyway:
- **Wallet idempotency key.** Every money call the sweeper makes
  (`CashoutGame` / `Credit` / `DebitReal`) reuses the row's stored
  `IdempotencyKey`, identical to the one the request path used. ctech-wallet
  collapses duplicates. Re-running `cmd/reconcile` twice for the same stuck entry
  therefore cannot double-credit or double-debit — the second call is a wallet-side
  no-op.
- **`MarkResolved` is monotonic.** It unconditionally sets `resolved = true`,
  `resolved_at`, a 30-day `ttl`, and clears `gsi_status`. Concurrent or repeated
  calls converge; no counter is incremented, nothing is lost. No CAS needed.
- **GSI eventual consistency.** `ListUnresolved` queries `gsi_status = "open"`
  (eventually consistent) and re-filters `p.Resolved` in memory from the same
  possibly-stale projection, so a just-resolved row can still surface for one more
  pass. The redundant retry is absorbed by the idempotency key above.

Conclusion: relies on ctech-wallet idempotency (per `api/CLAUDE.md`'s "money
ordering" convention) rather than being fully self-contained, but there is no
double-movement or lost-update bug. No change.

---

## Finding 3 (reviewed — NO new bug): `Record` is immutable-first-write; unresolved rows never get a TTL

- `PendingStore.Record` / `BuildRecordTx` use `BuildPutTxItemIfAbsent`
  (`attribute_not_exists` condition); a condition failure is swallowed as success.
  A replay — same request retried, or `cmd/tablecleanup` / `buyin` recording the
  same key twice — can never overwrite `amount`, `currency_mode`, `hold_ids`, or
  `idempotency_key`.
- The `PendingCashout` struct and the `Record` encode path set **no** `ttl`
  attribute. `ttl` is written **only** by `MarkResolved`, 30 days out, after the
  obligation is settled. Confirmed: an unresolved recovery row can never be reaped
  by DynamoDB TTL on the write path. No change.

---

## Finding 4 (reviewed — NO new bug): system-removal settlement is recorded transactionally

`SettleSystemRemoval` (AFK sweep / disconnect kick / exit-requested sweep) looked
like a two-step "remove seat, then separately `Record` the obligation" sequence
with a crash window in between. It is not: `tablemanager` wires
`buyin.Service.BuildSystemSettlementIntent` as the actor's
`systemSettlementIntent` pre-commit builder (`internal/table/actor.go:2122`), so
the `poker_pending_cashouts` row is written **in the same `TransactWriteItems`
batch** as the version-conditioned seat-removal state commit. The later
`SettleSystemRemoval` → `settle` → `Record` call is idempotent redundancy
(swallowed `attribute_not_exists` failure), not the durability mechanism.
`CashOut` uses the same pattern via `LeaveCmd.SettlementIntent`. No change.

---

## Finding 5 (reviewed — NO new bug): `entitlement.Store.Claim` race

Covered by PR #139 (atomic winner read-back). Verified the fix closes the
"two Claims in flight" and the more common "row exists but its `DebitReal` never
cleared" manifestations. Not touched here beyond the `Rebind` signature change in
the same file (see Coordination note).

---

## Finding 6 (reviewed — NO new bug): `cmd/reconcile` non-idempotent-retry check

Walked every branch of `run`:

| Kind / mode | op | idempotent on re-run? |
|---|---|---|
| `fee_debit` | `DebitReal(…, e.IdempotencyKey, …)` | yes — wallet key dedup |
| `cashout` + `real` | `CashoutGame(…, e.HoldIDs, e.IdempotencyKey, …)` | yes — wallet key dedup |
| `cashout` + `sandbox` | `Credit(…, e.IdempotencyKey, …)` | yes — wallet key dedup |

then `MarkResolved` (monotonic, Finding 2). Running the sweep twice over the same
entry produces at most one real money movement. No change.

---

## Consistency check against `tablestore`'s version + `ConditionExpression` discipline

| Store | Mutation | Guard | Verdict |
|---|---|---|---|
| `tablestore` | `CommitAction` | `version = :expected` CAS | reference pattern |
| `reconcile.PendingStore` | `Record` | `attribute_not_exists` (immutable) | consistent |
| `reconcile.PendingStore` | `MarkResolved` | none (monotonic set) | acceptable — no lost update possible |
| `entitlement.Store` | `Claim` | `attribute_not_exists(pk) OR expires_at <= :now` | consistent |
| `entitlement.Store` | `Rebind` | ~~`exists AND not expired`~~ → **`… AND bound_table_id = :old`** | **fixed by this audit** to match the discipline |

---

## Verification

```
cd api
go build ./...                       # clean
go vet ./...                         # clean
go vet -tags integration ./...       # clean
go test ./internal/reconcile/... ./internal/entitlement/... ./internal/buyin/... -race
go test -tags integration ./internal/reconcile/... ./internal/entitlement/... \
        ./internal/buyin/... ./cmd/reconcile/... ./cmd/tablecleanup/... \
        ./internal/config/... -race   # against a throwaway podman DynamoDB Local, torn down after
```

All green, including the new `TestRebindIsCompareAndSwapOnBoundTableID`.

---

## Coordination with PR #139

PR #139 also edits `api/internal/entitlement/store.go` and
`api/internal/buyin/service.go`. This audit's changes are in **different regions**:

- `store.go`: #139 rewrites `Claim` (and adds `decodeConditionFailureItem`); this
  audit only touches `Rebind`. No line overlap.
- `service.go`: #139 rewrites `chargeEntryFee` / adds `confirmFeeCharged` / drops
  the `nonce` param from `resolveEntitlement`; this audit only changes the
  `entitlementStore.Rebind` interface signature and the one `Rebind` call site
  inside `resolveEntitlement`'s rebind branch. When #139 merges first, the
  `Rebind` call site there (`s.confirmFeeCharged(ctx, room, playerID, e)` on the
  next line) is unaffected — only add `e.BoundTableID` as the third arg.
- `store_integration_test.go`: #139 changes `Claim` call sites to two-value form;
  this audit changes `Rebind` call sites to add an arg and adds one new test.
  Mechanical merge.
