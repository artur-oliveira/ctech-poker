# Multi-Server Turn-Order Fuzz Harness — Design

## Summary

`tests/integration/multiserver_test.go` (641 lines, 6 tests) proves the three historical multi-server
bugs (join-order-instead-of-seat-order turn resolution, `handOrder`/`players` pointer desync after
reload, uncommitted-mutation-poisoning-the-actor-on-conflict) stay fixed — but each test is a fixed,
hand-written scenario. The bug-3 class was found only via `-race -count=15+` against real timers because
a single clean run has low reproduction odds for that race (see
    [[ctech-poker-turn-order-and-multiserver-bugs]]): *"a clean run is not proof of absence for this class of
bug."* Fixed scenarios can't explore the action-interleaving space that class of bug lives in. This adds
one randomized harness that plays many full hands across N simulated server instances with a seeded RNG
choosing both the next legal action and which "server" it arrives through, checking invariants after
every commit instead of only at hand-end.

This is new test infrastructure only — no production code changes.

## Where

New file: `tests/integration/multiserver_fuzz_test.go`, same `//go:build integration` tag and
DynamoDB-Local dependency as the existing file. Reuses its setup helpers directly (`testDynamoClient`,
`mustCreatePokerTables`, `uniqueTableID`, `actionIDSeq`, `newSnapshotSink`) — no new setup path.

## Harness shape

```go
type fuzzConfig struct {
    seed       int64
    numServers int // 2-3, each its own tablemanager.Manager sharing store+lease backend
    numPlayers int // 3-6
    numHands   int // hands to play before stopping
}

func runMultiServerFuzz(t *testing.T, cfg fuzzConfig)
```

Setup mirrors `TestTwoServersDifferentPlayersConvergeOnSharedTable`: one shared
`tablestore.Store`, one shared `cache.NewMemoryBackend` standing in for Redis, `cfg.numServers`
independent `tablemanager.Manager`s over that same store+backend, one `snapshotSink`. Each of
`cfg.numPlayers` players is assigned a **fixed** server for the whole run (`rng.Intn(cfg.numServers)`) —
this is what makes the harness a multi-server test rather than a single-actor fuzz test: every action
that player takes round-trips through DynamoDB's conditional write against the other servers' commits,
exactly like the existing scenario tests.

Main loop, driven by `math/rand/v2` seeded from `cfg.seed`:

```
for hand := 0; hand < cfg.numHands; hand++ {
    for {
        load current table state fresh (store.LoadTable + hand.NewTableFromState)
        if tbl.Stage() == hand.Complete { break }
        current := tbl.CurrentPlayerIDForActor()
        if current == "" { t.Fatalf("stalled: no current player, stage=%v", tbl.Stage()) }
        legal := tbl.ViewFor(current).LegalActions.Actions
        action, amount := pickRandomLegalAction(rng, legal)  // new helper, see below
        dispatch action through serverFor[current], with a random 1-in-8 chance (rng) of instead
            dispatching a SetTurnTimeoutForActor/disconnect-grace command for a DIFFERENT seated
            player first (reproduces bug 3's interleaving: a timer firing on another server mid-turn)
        check invariants (below) against the freshly loaded post-commit state; t.Fatalf with
            cfg.seed and a running action-log slice on any violation
    }
    check chip-conservation invariant across the completed hand
}
```

`pickRandomLegalAction` (new, same file): given `LegalActions.Actions` (a `[]string` of action names)
and the current call amount, returns one of `check`/`call`/`fold`, or `raise` with a random legal amount
between min-raise and stack (only when `raise` is present) — never fabricates an action outside what the
snapshot already reports as legal, so a harness bug can never manufacture a false invariant violation.

## Invariants checked after every commit

1. **Chip conservation.** Sum of every seated player's `Stack` plus every live pot's total equals the
   sum of everyone's original buy-in. This is exactly the check `TestTwoServersDifferentPlayersConvergeOnSharedTable`
   already does once at hand-end (`total != 2000`) — the fuzz harness runs it after *every* action, which
   is what would have caught bug 2 (`handOrder`/`players` desync silently dropping `Contributed` chips)
   the moment it happened instead of only at showdown.
2. **`handOrder`/`players` identity.** `hand.NewTableFromState`'s re-linking (the bug-2 fix) means every
   player ID present in `t.handOrder` must resolve to a `*Player` also reachable via `t.playerByID` with
   the *same* `Stack`/`Contributed` values — assert this by comparing `tbl.PlayersForActor()` against a
   direct read of both slices via a small test-only accessor, or (simpler, no new exported surface) by
   asserting the chip-conservation check above never regresses between two consecutive loads of the same
   hand, which is the observable symptom bug 2 produced.
3. **No stall.** `tbl.CurrentPlayerIDForActor()` is non-empty whenever `Stage() != Complete` (already
   enforced by the `t.Fatalf` in the main loop above) and the loop must reach `Complete` within a bounded
   number of iterations (`cfg.numPlayers * 20`, generous for multi-way with side pots) — a stuck actor
   (bug 3's crash-free failure mode: a phantom fold timer never firing because the seat vanished) shows up
   as this bound being exceeded instead of the test hanging forever.
4. **No panic / no process crash.** The harness dispatches through the real `table.Actor.Dispatch`
   goroutines exactly like production; a nil-pointer panic in `hand.Act`/`runShowdown` (bug 3's original
   symptom) crashes the test binary the same way it would crash a real server — Go's test runner already
   reports this as a failure with a stack trace, no extra harness code needed, but the seed must be logged
   in a `t.Cleanup` (not just on the failure path) since a panic skips straight past any `t.Fatalf` call
   that would otherwise have printed it.

## Seed handling and reproduction

Every invariant failure message includes `cfg.seed` verbatim. A fixed top-level test drives multiple
seeds in one run rather than relying on `-count`, per the memory note that single clean runs are not
proof of absence:

```go
func TestMultiServerFuzz(t *testing.T) {
    if testing.Short() { t.Skip("fuzz harness: slow, run explicitly") }
    base := time.Now().UnixNano()
    for i := 0; i < 25; i++ {
        seed := base + int64(i)
        t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
            t.Parallel()
            runMultiServerFuzz(t, fuzzConfig{seed: seed, numServers: 2 + i%2, numPlayers: 3 + i%4, numHands: 5})
        })
    }
}
```

25 seeds × varying server/player counts in one `go test -tags integration -race` invocation gives the
same style of coverage the memory note says was needed to catch bug 3 (`-race -count=15` minimum),
without requiring a developer to remember the `-count` flag — it's baked into the loop. `t.Parallel()`
per seed also means the harness naturally exercises more actor-goroutine interleaving than a serial
`-count` rerun would.

A failing seed is reproducible standalone: paste the printed seed into a one-off
`runMultiServerFuzz(t, fuzzConfig{seed: <printed>, ...})` call, or (nice-to-have, only if trivial) accept
an optional `-args -seed=N` override — not required for this spec, since the per-run seed is already
logged and copy-pasteable into the config literal.

## Testing

This spec's own deliverable *is* a test file, so "testing" here means validating the harness itself
before trusting it:

- Run `TestMultiServerFuzz` against the current (already-fixed) `actor.go`/`hand.go` — must pass clean
  across all 25 seeds, confirming the invariants don't false-positive on correct behavior.
- Temporarily revert one of the three historical fixes (e.g. skip the `handOrder` re-linking in
  `NewTableFromState`) locally and confirm the fuzz harness's chip-conservation invariant catches it
  within the 25-seed run — this is the harness's own regression test, proving it would have caught bug 2
  had it existed at the time. Do not commit the reverted code; this is a local verification step only.
- `go vet`/`go build -tags integration ./...` must stay clean (new file only, no production code
  touched).

## Out of scope

- Replacing or removing any of the 6 existing hand-written scenario tests — those pin exact historical
  regressions with clear names; the fuzz harness supplements them, it doesn't supersede them.
- Fuzzing preselections (`check_fold`/`call_any`/fixed `call`) — `internal/table/preselection_test.go`
  already covers that surface with scenario tests; folding it into this harness would conflate two
  different action-selection spaces (this harness never sets a preselection, only ever dispatches an
  immediate legal action) and isn't needed for the multi-server turn-order class this spec targets.
- CI wiring (making this run on every PR vs. nightly/manual only) — a `go test -tags integration -race`
  run of 25 parallel seeded hands is slower than the existing suite; where it runs in CI is a repo-ops
  decision, not part of this design. `api/CLAUDE.md`'s testing section gets one line added pointing at
  this file and repeating the existing `-race -count=15` guidance for any future `actor.go` timer-path
  change, per the Mandatory Documentation Policy.
