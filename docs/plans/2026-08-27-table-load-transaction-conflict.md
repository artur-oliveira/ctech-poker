# Post-hand commands rejected as "invalid_action" — root cause and fix

## Status

Fixed and merged into the working tree on 2026-08-27. Backend (`tablestore`, `table`, `api/v1`)
and frontend (`useTableRealtime.ts`) all changed; full Go suite (`-race`, plus `-tags integration`
against DynamoDB Local) and the 1154-test UI suite are green.

## Symptom

Every rabbit hunt request failed, essentially immediately and essentially every time, with the
client toast *"Essa ação não é mais válida. Confira o estado atual da mesa."* (`invalid_action`).
`show_cards` failed the same way. `act`, chat and reactions were unaffected.

## Root cause

Found in CloudWatch (`/ctech-poker/prod/app`), on the timer path that shares the same load call:

```
"msg":"table next hand dispatch failed",
"err":"tablestore: get table: dynamodb: operation error DynamoDB: TransactGetItems,
       ... TransactionCanceledException: Transaction cancelled ... [TransactionConflict]"
```

`Store.LoadTable` read the authoritative table item through **`TransactGetItems`**. A
transactional read conflicts with any `TransactWriteItems` touching the same item and fails the
whole read with `TransactionCanceledException[TransactionConflict]`.

Three facts turn that into a user-visible, deterministic failure:

1. **Every handler loads before it validates.** `Actor.ensureLoaded` runs first in every
   `handle*`; a load error returns immediately, before the handler's own precondition,
   idempotency guard or `retryOnConflict` ever runs.
2. **The post-hand window is the write-heaviest moment of a table's life.** `commitOutcomeLogEntries`
   writes one row per winner/showdown result, `armNextHandTimer`'s commit persists the countdown
   deadline, and the hand hooks fire — all against the one `poker_table_state` item.
3. **`show_cards` and `request_rabbit_hunt` exist only in that window.** They are gated on
   `stage == Complete` on both sides (`page.tsx`'s `canRevealCards`, `RabbitHunt.tsx`'s
   `available`, and `RevealHoleCard`/`RequestRabbitHunt` in the engine). So they are precisely the
   commands issued while those writes are in flight.

`act` escaped because a rejection there carries `expected_snapshot_version`, comes back as
`stale_state`, and the client already auto-retries it.

The AWS SDK never masked this: `TransactionCanceledException` is in neither
`DefaultRetryableErrorCodes` nor `DefaultThrottleErrorCodes`
(`aws-sdk-go-v2@v1.43.7/aws/retry/standard.go`), and the repo installs no custom retryer.

The transactional read was also buying nothing. It read a **single** item (`pk = tableID`), and a
single-item write is already atomic in DynamoDB — so serializable isolation over a one-item read
set is a no-op that cost 2x RCU and added a failure mode. (It never gave cross-item isolation with
the action log or the guard either, since it never read them.)

## Fix

### 1. `LoadTable` — `BatchGetItem` with `ConsistentRead` (`api/internal/tablestore/dynamo.go`)

Same strong consistency (all writes acknowledged before the read are visible), no transaction
semantics to conflict with, half the read capacity. The prior comment claimed `TransactGetItems`
was "the only way api-commons' `dynamo.Base` exposes" a consistent single-item read — it is not;
`BatchGetItemRaw` takes `ConsistentRead`.

`BatchGetItem` is the one read path that can answer "fine, but I did not read it"
(`UnprocessedKeys`, under throttling). Treating that as an empty result would report a live table
as unseeded, so it retries three times with a 20/40/80ms backoff before failing loudly. Throttling
and 5xx are still handled by the SDK's own standard retryer, as before.

Consistency notes that do **not** apply here but are worth recording: a strongly consistent read is
unavailable on a GSI (this reads the base table by `pk`), and a non-transactional read can observe
one `TransactWriteItems` partially across *different* items. `LoadTable` reads only the state item,
exactly as the transactional version did, so neither is a regression. Reading state + action log as
one atomic set would need this revisited.

### 2. Outages no longer masquerade as illegal moves (`tablestore`, `table`, `api/v1`)

`tablews.go` mapped every handler error to `invalid_action`. For a store failure that is wrong
twice over: it tells the player their move is no longer legal (blaming them for an outage), and it
makes the client end the command instead of resyncing it.

- New sentinel `tablestore.ErrUnavailable`, wrapped around the failures that never reached a
  verdict about the action: `LoadTable` retrieval, `SeedTable`, `CommitAction`'s non-conflict /
  non-duplicate write failure and its guard re-read, plus `ensureLoaded`'s "no state seeded".
- New `actionErrorCode(err)` in `tablews.go` answers `unavailable` for `ErrUnavailable` and
  `table.ErrActorStopped`, `invalid_action` otherwise. Applied at all 16 rejection sites (14 plain,
  2 that already branched to `stale_state`).
- `ErrVersionConflict` deliberately stays `invalid_action`: it *is* a verdict about the action.

### 3. Auxiliary commands are retried client-side (`ui/src/lib/hooks/useTableRealtime.ts`)

`act()` had a stale-state retry; the auxiliary commands had none, because they carry no
`expected_snapshot_version` for the server to compare against — they could only ever receive a flat
rejection, and gave up on the first one.

`auxFramesRef` now keeps the exact frame under its `action_id` while in flight (`emitAux`), and a
resync-class rejection (`stale_state`, `invalid_action`, `rate_limited`, `unavailable`) resubmits
that identical frame under the **same** `action_id` rather than surfacing it. Reusing the id is
safe for exactly the reason `act`'s retry reuses it: these commands are rejected before commit, so
no idempotency guard row was ever written for them.

Backoff is 700/1400/2800ms — deliberately longer than the resync scheduled for that same
`action_id` (≤450ms on a first rejection), so the resubmit is judged against the state the resync
pulled and not the one that just rejected it. Capped at `MAX_ACTION_RETRIES` (3, shared with
`act`), with the original 8s `ACTION_TIMEOUT_MS` timer left armed as the backstop. Covers
`show_cards`, `request_rabbit_hunt`, `request_winner_cards`, `accept`/`decline_winner_cards` and
`request_exit`.

This is defence in depth, not the fix: with (1) in place the rejection should not happen at all.

## Tests

- `TestLoadTableSurvivesConcurrentCommits` (`-tags integration`) reads the table item repeatedly
  while a goroutine commits 25 versions to it. **Honest limitation:** DynamoDB Local does not
  emulate `TransactionConflict`, so this test also passes against the old implementation. It pins
  the invariant; the proof of the bug is the production log above.
- `TestActionErrorCodeSeparatesOutagesFromIllegalMoves` covers the classifier, including wrapped
  sentinels — every real failure arrives wrapped in context by the time it reaches the gateway.
- `useTableRealtime.test.tsx`: new test asserting three resubmits under one `action_id` and the
  error surfacing only once the budget is exhausted. Two existing tests (rabbit hunt, winner cards)
  asserted failure on the first rejection and were updated to the new contract.

## Also changed

`Actor.retryOnConflict` had a dead empty `if err != nil {} else {}` — removed. During the
investigation it was briefly extended to reload-and-retry on *any* engine rejection, on a
stale-lease-cache theory the CloudWatch evidence then disproved; that change was reverted, since a
load error returns before `retryOnConflict` is ever reached.

## Not done

`resolveCommitErr` classifies any `TransactionCanceledException` on the **write** side as
`ErrVersionConflict` when no guard row is present, so a write-side `TransactionConflict` is
reported as a version conflict. Behaviour is benign — `retryOnConflict` reloads and retries, which
is the right response to both — but the two are not actually the same condition. Left alone.

No metric or alarm was added for store-unavailable rejections. Worth adding if `unavailable` shows
up in client telemetry at any real rate.
