# Hand settlement audit — multi-way all-in / run-it-twice / odd-chip / folded-money (#121)

Date: 2026-09-02
Scope: `api/internal/engine/hand/hand.go` `runShowdown` and its integration with
`sidepots.ComputeSidePots`, `equity`, `betting`, `deck`.

Follow-up to the 2026-09-02 systematic review (§3 D1). The prior pass found the
engine math correct overall but flagged this area for a focused checklist run
and specifically called out the `runShowdown` `len(winners) == 0` refund
branch as contradicting the `folded-money-is-dead-money` ruling.

## Result summary

| # | Checklist item | Verdict |
|---|---|---|
| a | Multi-way all-in, 3+ distinct all-in amounts + folded partial bet builds side pots | **Verified correct** |
| b | Run-it-twice halves each board's pot (main + side) and recombines | **Verified correct** |
| c | Odd chip on a split pot goes to the first live seat left of the button | **Verified correct** |
| d | `runShowdown` `len(winners) == 0` branch | **Bug found and fixed** |

## a. Multi-way all-in + folded partial bet — verified correct

`runShowdown` builds `[]sidepots.Contribution` straight from `p.Contributed` /
`p.State` for every player in `t.handOrder`, then delegates layer construction
entirely to `sidepots.ComputeSidePots`. Layer boundaries are drawn only at
distinct **live** contribution levels; a folded player's partial contribution
never adds a boundary and is absorbed as dead money into the layers the live
players contest (rolling down into the main pot when it sits above every live
contribution). Rake is applied per layer against the called portion only;
genuinely uncalled bands (`PotLayer.Uncalled`) are returned to their sole
contributor and never raked.

New regression test:
`TestMultiWayAllInThreeDistinctAmountsWithFoldedPartialBuildsSidePots` — a
5-handed hand with all-ins at 100 / 250 / 500, a fourth player driving the pot
to 2800, and a fifth putting in 1300 and folding on the turn. Asserts exactly
four contested layers (500 / 600 / 750 / 3100), the folded 1300 absorbed as
dead money (never a refund layer), the folder paid 0, and full chip
conservation (4950 in, 4950 out).

## b. Run-it-twice with side pots — verified correct

Each non-uncalled layer's post-rake amount is split
`firstAmount = net/2 + net%2` (board one) and `net/2` (board two); rake is
charged once against the full layer before the split. Each board is evaluated
independently via `evaluateLayer(layer, secondBoard)` and awarded separately,
so a side pot is halved and recombined exactly like the main pot. The halving
odd chip deterministically belongs to board one.

New regression test:
`TestRunItTwiceHalvesMainAndSidePotPerBoardOddChipToBoardOne` — pre-flop
three-way all-in with a 603-chip (odd) main pot and an 804-chip side pot, run
twice, board one rigged for B and board two for C. Asserts main pot halves
302 / 301 (odd chip on board one), side pot halves 402 / 402, four pot results
(2 layers x 2 runouts), and conservation across both boards.

## c. Odd chip to seat left of the button — verified correct

`oddChipWinner` walks `t.handOrder` starting at `dealerIndexWithin+1` and
returns the first winner it meets — i.e. the first live seat clockwise from the
button, matching OVERVIEW.md § 3.3. Used both for the showdown split remainder
(`award`) and for the run-it-twice / uncalled-layer paths.

New regression test: `TestOddChipOnChoppedPotGoesToFirstSeatLeftOfButton` — a
genuine three-way showdown where B and C chop a 45-chip pot and A loses;
asserts the odd chip lands on B (button is A). The existing
`TestOddChipStartsLeftOfDealer` already unit-tests `oddChipWinner` traversal
directly across a button move.

## d. `runShowdown` `len(winners) == 0` — bug found and fixed

### What was wrong

The branch split `layer.Amount` **evenly back to `layer.Eligible`** and
recorded the layer as `Refund: true`. Its comment claimed this fires when
"every player who reached this layer's contribution level has since folded" and
that refunding the contributors is therefore correct.

That reasoning is stale. Since the 2026-08-31 `folded-money-is-dead-money`
change, `sidepots.ComputeSidePots` **never puts a folded player in a contested
layer's `Eligible`** — a fully-folded band is rolled into dead money inside
`ComputeSidePots` itself and is never emitted as a layer for `runShowdown` to
evaluate. So the branch cannot be reached by folding at all. The only way
`evaluateLayer` can now return zero winners is if **every** id in
`layer.Eligible` resolves to a `nil` `*Player` (removed from `t.players`) — and
even that is prevented today, because `RemovePlayerForActor` refuses to remove
anyone still in `t.handOrder` for the entire hand (`stage` stays
`!= WaitingForPlayers/Complete` until the very last line of `runShowdown`).

Worse, in the one state that *could* trigger it (all eligible players removed),
the even-split "refund" is computed into `payouts` for now-`nil` player IDs and
then silently dropped by the credit loop (`if p := t.playerByID(id); p != nil`)
— the chips vanish from the pot entirely.

Either way the behavior contradicts the ruling: folded/dead money must roll to
the live winner, never be refunded to (or dropped on behalf of) its
contributors.

### The fix (`hand.go`, `runShowdown`)

Replaced the even-split refund with a `deadWinnerlessCarry` accumulator: a
winnerless layer's chips roll **forward** into the next contested layer that
actually has a winner (that layer's normal per-layer rake then applies to the
called portion), mirroring how `ComputeSidePots` itself rolls dead money into
the nearest contested layer. If no later layer has a winner either, an
after-loop fallback hands the carry to the hand's actual winner
(`winningIDs[len-1]`). Nothing is ever refunded to the contributors and no
chips are dropped. The branch is still documented as provably unreachable
through the public API; the change is defense-in-depth that keeps the invariant
correct if a future change to `sidepots` or `RemovePlayerForActor` ever
reintroduces reachability.

New regression test: `TestWinnerlessLayerRollsToLiveWinnerNeverRefunded` —
whitebox, manufactures the impossible state by splicing a side pot's only two
eligible players out of `t.players` (leaving them in `t.handOrder`) and calls
`runShowdown` directly. Asserts the removed contributors are paid 0 and the
400-chip winnerless side pot rolls to the live main-pot winner (final stack
700, not 300).

## Verification

```
cd api
go build ./...                                  # clean
go vet ./... && go vet -tags integration ./...   # clean
go test ./internal/engine/... -race -count=3     # ok
```

`-tags exhaustive` (the ~10-min `handeval` proof) was **not** re-run: this
change touches only `hand.go` settlement logic and adds tests — none of `ref`,
`hashq`, the table generator, or `tables.bin` were modified, which is the
documented trigger for the exhaustive suite.

## Not in scope / unchanged

`sidepots.ComputeSidePots`, `equity`, `betting`, `deck` were read and left
unchanged — their existing unit coverage is strong and no discrepancies were
found. `EscalateBlindsForActor` integer-division drift and `wasEverAllIn`
bookkeeping (also listed on the issue) were not part of this change.
