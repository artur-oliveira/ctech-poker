# Hand evaluation and equity optimization

## Behavior

`handeval.Best7` preserves the dense scores and categories for valid seven-card
hands. It rejects every duplicate card, as well as out-of-range ranks and suits.
Equity validates hole, board, and dead cards before compact encoding or cache
lookup: malformed cards can otherwise alias valid IDs and receive cached
results. Impossible opponent counts are rejected before multiplication, avoiding
integer overflow.

Equity is expected pot share against uniform random opponent hands, including
fractional shares of ties. It does not infer ranges from betting actions.

- Preflop, without dead cards, against one through eight opponents: an offline
  table supplies the estimate if its four-million-sample budget meets the
  requested iteration count. It normalizes suits into 169 starting-hand classes.
- Heads-up river: every legal opponent pair is enumerated (at most 990), giving
  exact equity. Dead cards reduce the enumerated pool. The result is independent
  of the requested positive iteration count.
- Other inputs, including larger preflop budgets, dead cards, or more opponents:
  deterministic Monte Carlo uses the requested iteration count.

The actor still requests 200 samples for simulated spots. Near 50% equity their
approximate worst-case 95% sampling margin is ±6.9 percentage points. The
precomputed estimates use four million samples per entry, reducing that margin
to about ±0.049 percentage points. This is a per-entry approximation, not a
simultaneous error guarantee for the entire table. Precomputed values are
estimates, not exact probabilities; float32 storage adds negligible rounding
relative to sampling uncertainty.

## Evaluator architecture

The generated flush and non-flush score tables are unchanged. Non-flush scores
still use the same minimal perfect hash over 49,205 rank multisets. Runtime
hashing replaces the thirteen-rank scan with an additive key and two tables:

1. Split the rank counts into groups of six and seven.
2. Encode each group in base five, with the second group starting at bit 16.
3. Add each card's rank weight to accumulate the key.
4. Look up the two groups' contributions to the original lexicographic rank,
   add them, and read the existing non-flush score table.

The contribution tables occupy 187,500 bytes, plus 104 bytes of rank weights.
They are built at startup from rank-count combinatorics. Every valid combined
key is tested against the original `hashq.Hash`; no new score binary is needed.

A packed card also carries a suit-counter increment above bit 36 and a rank bit
in one of four 16-bit suit lanes. Four suit counts are accumulated in the same
addition as the rank key. Adding a bias of three to each count exposes a high
bit exactly when that suit has at least five cards. With seven valid cards,
no suit counter carries into its neighbor, and at most one suit can flush.
A bit scan then locates its rank-mask lane. The 52 packed cards occupy 832 bytes.

BoardState holds only two uint64 values (16 bytes). Adding hole-card keys and
masks yields the complete hand directly. `Finalize` remains as a compatibility
no-op. BoardState is a trusted fast path: its five board cards and two hole
cards must all be valid and distinct. `Best7` and the equity entry point perform
validation for their callers.

## Equity implementation

Preflop data lives in `equity/preflop/table_generated.go`: 169 × 8 float32 values,
5,408 bytes of numeric data. `go generate` builds it using a fixed PCG seed and
four million trials per starting-hand class. Each trial supplies estimates for
all eight opponent counts, sharing a board and opponent prefixes. Split shares
are accumulated in integer units of 1/2520, which is divisible by every winner
count from one to nine. Class seeds and integer totals make results independent
of worker scheduling. The generator uses PCG independently of the runtime
sampler's XorShift implementation.

The preflop path runs after input validation but before the global LRU. It
allocates no per-table cache entry and reports `EstimateStats.Precomputed=true`;
`CacheHit` continues to mean an actual LRU hit. Suit-equivalent hands return the
same table value. The actor's per-hand memoization remains unchanged in scope.

Monte Carlo now draws the missing board first, evaluates hero, and then draws
and evaluates one opponent at a time. Once hero loses, remaining draws are
unnecessary. Each trial starts partial Fisher-Yates at index zero over the full
remaining-card pool, which gives uniform samples from any starting permutation;
the previous trial's early stopping does not remove or duplicate cards.
On multiway rivers the completed board and hero score are computed once.

Lazy drawing changes the sequence relative to older binaries. The seed remains
input-derived, so identical inputs produce identical results on instances of
the same version; mixed-version instances can temporarily return different
sampled estimates during rollout.

Normalization uses stack storage and typed sorting. The actor cache uses full
cards in a fixed-size struct instead of formatted strings. Full cards prevent
invalid compact-ID aliases there. Hand changes clear the actor cache, and global
LRU eviction remains table-scoped. Cache hits, preflop lookups, and calculations
with cache storage disabled allocate zero bytes. New LRU insertions still
allocate storage. Exact river LRU keys retain the iteration-count dimension.

## Measurements

Local Linux/amd64, Intel i7-13620H, Go 1.27; medians of three 300 ms runs.
Evaluator inputs are 4,096 deterministic random hands. Equity uses fixed spots
and 200 requested samples; uncached benchmarks disable LRU storage. The final
benchmarks run packages sequentially (`-p 1`). These are microbenchmarks, not
production latency predictions.

| Operation | Original implementation | Final implementation |
|---|---:|---:|
| Best7 | 64.94 ns | 18.14 ns |
| Prepared-board Eval2 | 44.98 ns | 3.11 ns |
| Actor equity cache hit | 314.90 ns | 21.71 ns |
| Preflop, eight opponents | 42.60 µs (200 samples) | 11.06 ns (offline lookup) |
| Flop, eight opponents | 23.94 µs | 6.28 µs |
| Turn, eight opponents | 16.69 µs | 5.07 µs |
| River, one opponent | 6.26 µs (sampled) | 2.75 µs (exact) |
| River, eight opponents | 13.75 µs | 3.00 µs |

The intermediate additive-only implementation measured 22.77 ns for Best7 and
2.69 ns for prepared-board Eval2 in the follow-up baseline. Packed suits improve
full-hand and board-building work, but add a small cost to isolated Eval2
(3.11 ns). Lazy drawing and preflop lookup yield larger end-to-end equity gains.
Exact river enumeration is bounded by the remaining deck, but is not guaranteed
faster than Monte Carlo for every board or requested sample count.

## Generation and validation

The full API race suite and the exhaustive packed-versus-quinary comparison
passed. Regenerating both assets produced byte-identical output.

Run from `api/`:

```sh
go generate ./internal/engine/handeval/... ./internal/engine/equity/preflop
go test ./... -race
go test -tags exhaustive -run TestPackedMatchesQuinaryExhaustively -timeout 10m ./internal/engine/handeval
go test -p 1 ./internal/engine/handeval ./internal/engine/equity ./internal/table -run '^$' -bench 'Benchmark(Best7$|BoardStateEval2|Estimate|ActorEquityCached)' -benchmem -benchtime=300ms -count=3
```

Both generated assets are reproducible. The 122,273-byte score table is identical
to the original (`ad76afbdd80952bff37892b3d99259ab95a8a64248ab2c0975bd7f864d478ba0`).
The preflop source checksum is
`874398c100c4ff5c4e7fa64bd9ba65acf013862f24bf5d170cec6c52da9df582`.

Tests cover all 49,205 rank multisets, all 1,326 starting hands and their class
sizes, generator scheduling independence, preflop lookup/fallback selection,
malformed-card aliases, oversized opponent counts, reference-based exact river
results, and lazy sampling against complete small-deck enumeration. Separate
reference simulation checks precomputed heads-up equities; 7–2 offsuit is about
34.6%, correcting the older loose 35.4% fixture.

The packed evaluator and prepared-board path match the original quinary/suit-array
evaluator's exact score on all 133,784,560 valid hands. The initial additive-key
change also passed the slower independent-reference ordering sweep; that test
remains available with `-run TestMatchesReferenceOrderingExhaustively -timeout 60m`.
The original reference evaluator, hash, and score generator were not changed.
