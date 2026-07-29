# Achievement Catalog Expansion — Design

**Date:** 2026-07-29
**Status:** Approved design, pending implementation plan.

## Problem

The user brainstormed 40+ new achievement ideas across many categories (session/regularity, strategy,
money-tier, specific-hand, position/action, risk/psychology, social/tournament, rarity tiers, secret
achievements, mixed/milestone). Most of the list needs subsystems this codebase does not have yet: a
"session" concept with P&L, seat/position tracking, street-level action sequencing, a tournament feature,
and a population-percentile rarity engine. Building all of that in one shot is out of scope. This design
covers the slice that is buildable today, purely from data `achievements.Service.RecordHand` and
`pokerstats.Analyze` already have access to, plus one small deterministic card-combinatorics addition
(reusing the existing reference hand evaluator).

## Non-goals (explicitly deferred, tracked separately if ever picked up)

- Session-based achievements (marathon session, daily streak, profitable sessions, bankroll builder,
  stop-loss, recovery, tilt control, patience-as-time, mixed-style/versatility) — need a session/P&L concept.
- Position/action achievements (blind steal, blind defense, button player, value bet, check-raise, pot
  control, hand-reading folds) — need seat/position and per-street action-sequence tracking in the engine.
- Tournament achievements (bounty hunter, final table, ITM, bubble boy) — no tournament feature exists;
  the user's own brainstorm marked these "se aplicável."
- Equity-based achievements (Outs, Milagre, One Outer — win/loss by all-in equity percentage) — need (a) an
  "all-in street" marker threaded from the action log into `HandOutcome`, and (b) a new exact-equity
  calculator against a *known* revealed opponent hand (the existing `equity.Estimate` is a Monte Carlo
  estimate against an unknown random opponent, built for live in-hand display — wrong tool for retroactive
  analysis of a hand where both hands are already known).
- Account-age achievements ("Aniversário") — needs a scheduled job against player creation timestamp, not a
  per-hand hook.
- Rarity tiers (Bronze/Prata/Ouro/Platina/Diamante) — a population-percentile engine is a separate
  cross-cutting feature, applicable to the whole catalog, not one achievement.
- Depends on [[2026-07-29-sandbox-real-segmentation-design]] for the `mode` parameter threaded through
  `RecordHand` — this plan assumes that segmentation lands first (or in the same implementation pass).

## Data model changes

`Achievement` (`catalog.go`) gains one field:

```go
type Achievement struct {
    Key    string
    Metric string
    Tiers  []Tier
    Secret bool // hide progress in the API/UI until the first tier unlocks
}
```

`Store` gains one new primitive alongside the existing monotonic `Increment` (DynamoDB `ADD`, never
decreases): a **streak counter**, needed only by the two achievements that must reset on a losing/qualifying
event instead of always growing.

```go
// IncrementStreak either resets the counter to resetTo (when reset is true) or adds 1 to it.
// Same item, same SK convention as Increment — this is a different update expression (SET vs ADD),
// not a new key or table.
func (s *Store) IncrementStreak(ctx context.Context, playerID, key string, reset bool, resetTo int) (current int, err error)
```

No new DynamoDB table, GSI, or CDK change. Both additions are pure Go/library-level.

## Achievement catalog additions

All logic below runs inside (or alongside) the existing `achievements.Service.RecordHand`, using only:
`HandOutcome` fields already captured (`Winners`, `Participants`, `ShowdownResults`, `PlayerHands`, `Board`,
`Payouts`, `Contributions`), plus the `[]pokerstats.HandMetric` slice `onHandComplete` already computes via
`pokerstats.Analyze` for `pokerstats.Store.RecordHand` (see Propagation below — reused, not recomputed).

### Group A — direct reuse of existing `HandOutcome` fields, no new evaluation logic

| Key | Metric | Tiers | Logic |
|---|---|---|---|
| `real_money_earned` | `real_money_won` | 1 000 / 10 000 / 100 000 / 1 000 000 / 10 000 000 (cents: R$10/100/1k/10k/100k) | Sum of `Payouts[winner]` on real-mode hands |
| `sandbox_chips_earned` | `sandbox_chips_won` | 1k / 10k / 100k / 1M / 10M | Sum of `Payouts[winner]` on sandbox-mode hands |
| `won_with_pocket_pair` | `hand_won_with_pocket_pair` | 1 / 10 / 50 / 100 / 500 | Generalizes `isPocketAces`/`isPocketKings` to any rank pair |
| `won_full_table` | `hand_won_full_table` | 1 / 5 / 10 / 25 / 50 | Winner, `len(Participants) == 9` |
| `won_heads_up` | `hand_won_heads_up` | 10 / 50 / 100 / 500 | Winner, `len(Participants) == 2` |
| `lost_straight_flush_to_royal` (`Secret: true`) | `hand_lost_straight_flush_to_royal` | 1 / 2 / 5 | Loser's `ShowdownResults` category is straight flush, winner's is royal flush |
| `first_hand_allin_win` (`Secret: true`) | `first_hand_won_allin` | 1 (single tier) | Winner is in `AllInPlayers`, and `hands_played` counter reads exactly 1 right after this hand's increment |
| `beat_pocket_aces` ("Verdugo de Ases") | `beat_opponent_pocket_aces` | 1 / 5 / 25 / 100 / 500 | Winner beat an opponent whose `PlayerHands` shows pocket aces (mirror of existing `cracked_aces`, winner's side) |
| `beat_trips_or_better` ("Carrasco") | `beat_opponent_trips_or_better` | 1 / 5 / 25 / 100 / 500 | Winner beat an opponent whose `ShowdownResults` category is `>= three_of_a_kind` (mirror of existing `bad_beat`, winner's side) |
| `hands_played` (existing key) | — | tiers gain a `5000` step | Pure tier-list edit, no new key ("1000ª Hand" milestone framing) |

`won_with_nuts` ("As Nuts") also belongs conceptually here but needs one deterministic enumeration — see
Group C, since it shares the same reasoning as the near-miss checks (comparing against the theoretical best
hand obtainable from the board + remaining deck, not just the two known hands at the table).

### Group B — reuses the `pokerstats.Analyze` output already computed in `onHandComplete`, zero new action-log parsing

| Key | Metric | Tiers | Logic |
|---|---|---|---|
| `three_bet_won_no_showdown` | `three_bet_win_no_showdown` | 5 / 25 / 100 / 500 | Winner's `HandMetric.ThreeBet == true` and `outcome.WonWithoutShowdown == true` |
| `folded_streak` ("Paciência") | `consecutive_hands_no_vpip` | 100 / 500 / 1000 | Uses `Store.IncrementStreak`: `HandMetric.VPIP == false` → +1; `VPIP == true` → reset to 0 |

### Group C — deterministic card-combinatorics, reusing `handeval/ref.BestN` (the existing reference evaluator; called, never edited — CLAUDE.md's "never edit `ref`" rule is about the generator/table source of truth, not about calling it) or plain pattern matching, all off the hot path (computed once per hand, at completion)

| Key | Metric | Tiers | Logic |
|---|---|---|---|
| `four_to_royal_missed` | `near_miss_royal_flush` | 5 / 10 / 25 / 50 | Player's 7 usable cards (hole + full `Board`) hold 4 of the 5 royal-flush ranks in one suit, and the made hand isn't itself a royal flush. Pure suit/rank pattern match, no `BestN` needed |
| `four_to_straight_flush_missed` | `near_miss_straight_flush` | 10 / 50 / 100 / 500 | Same pattern match, generalized to any 5-consecutive-rank same-suit window |
| `paid_river_draw_missed` ("Draw Morto") | `river_draw_missed` | 10 / 50 / 100 | For players in `ShowdownResults` (== reached river): had a 4-flush or open-ended straight draw in hole + `Board[:4]` that `Board[4]` didn't complete |
| `lost_river_after_leading_turn` ("River Cruel") | `lost_river_after_leading_turn` | 5 / 25 / 100 / 500 | For a `ShowdownResults` loser: `ref.BestN(hole + Board[:4])` was the best among all showdown participants' turn-stage hands, but they didn't win at the river |
| `won_runner_runner` | `won_runner_runner` | 1 / 5 / 10 / 25 | Winner was behind on `ref.BestN` at both `Board[:3]` and `Board[:4]`, but ahead at full `Board` |
| `won_with_nuts` ("As Nuts") | `hand_won_with_nuts` | 1 / 5 / 25 / 100 / 500 | Winner's `Best7(hole+Board)` equals the maximum `Best7` achievable by any 2 cards from the undealt deck + `Board` — the mathematically unbeatable hand for that board |

`same_pocket_pair_streak` ("Três Vezes", `Secret: true`, single tier at 3) also uses `Store.IncrementStreak`:
resets on any loss or on winning with a different pocket-pair rank; increments when winning again with the
same rank as the previous win.

## Propagation

`onHandComplete` (`app.go`) already builds `metrics := pokerstats.Analyze(...)` before calling
`pokerStatsStore.RecordHand`. Pass that same `metrics` slice as a new parameter to
`achievements.Service.RecordHand(ctx, tableID, mode, outcome, metrics)` — no duplicate action-log parsing,
no new dependency from `achievements` on `pokerstats` types beyond taking the slice by value (kept as a
small local type alias in `achievements` to avoid a hard package import, since only `PlayerID`/`VPIP`/
`ThreeBet` are needed).

`ref.BestN` is called from `achievements` (a new import of `internal/engine/handeval/ref`) — acceptable
because it only ever runs once per completed hand as part of already-asynchronous gamification bookkeeping,
never on the betting hot path.

## Testing

- One table-driven test per new achievement in `achievements`: synthetic `HandOutcome` (+ `metrics` where
  relevant) in, assert the right tier unlocks — same shape as existing `TestRecordHand`-style tests already
  in the package.
- `IncrementStreak`: a reset-vs-increment test and a race test (two fast hands for the same player,
  assert no lost update) — mirrors how the existing `Increment` is tested for atomicity.
- `ref.BestN` reuse: a sanity test comparing `ref.BestN` against `handeval.Best7` on 7-card hands (they must
  agree when N=7), so a future edit to `ref` that breaks agreement fails loudly here, not in production.
- `won_with_nuts`: a fixed-board unit test with a known nut hand (e.g. board making quads impossible to beat
  except by a specific holding) to pin the "unbeatable" enumeration logic before trusting it against random
  hands.
