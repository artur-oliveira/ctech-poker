# Head-to-Head Player Comparator — Design

## Summary

A two-player comparator: given the viewer and one other player, show how many hands they've played
at the same table together, who's won more of those hands, and (for the subset that's well-defined)
the viewer's chip result specifically against that opponent. This is a new stat, not a new social
graph feature — it doesn't group players (no "club"/persistent group, which is explicitly out per
`ui/CLAUDE.md`'s "Not built" list) and isn't a leaderboard of every opponent, just a query naming one
other player.

No production code changes in this pass — spec only.

## Why not derive this from existing history at query time

`api/internal/sessionlog/store.go`'s `HandItem` (PK=`player_id`, SK=`currency_mode#hand_id`, no TTL —
`store.go:23-35`'s comment confirms `poker_player_hands` is the untrimmed durable history) already
carries an `Opponents []OpponentSummary` list per hand (`store.go:62,86-99`), so in principle a
head-to-head could be computed by querying the viewer's entire hand partition and filtering for hands
where `Opponents` contains the target ID. Rejected: `ListSessions`/`FindOpenSession`'s own docstrings
already flag the failure mode of "page through everything, every time" queries against an
ever-growing per-player partition (see `FindOpenSession`'s comment on why a fixed-`Limit` query used to
strand open sessions, `store.go:145-151`) — a comparator against a specific opponent, called from a
profile page, would otherwise page a heavy player's *entire lifetime hand history* synchronously on
every view. `api/internal/pokerstats` already establishes the alternative for exactly this shape of
problem: a small incrementally-updated aggregate written at the same hand-complete hook that already
writes `sessionlog`/`pokerstats`/`leaderboard`, one DynamoDB item read at query time instead of an
unbounded scan.

## Data model

New package `internal/matchup`, one DynamoDB table `poker_player_matchups`, following `pokerstats`'s
exact shape (`internal/pokerstats/stats.go:39-42`'s `Store{base dynamo.Base}` /
`dynamo.NewBase(db, env, table)`):

- **PK**: `pair#<mode>#<idLow>#<idHigh>` — `idLow`/`idHigh` are the two player IDs in lexicographic
  order, so the same pair always resolves to the same item regardless of who's "viewer." Scoped by
  `mode` (sandbox/real) the same way `pokerstats.Store.Get`'s key is (`"stats#"+mode+"#"+playerID`,
  `stats.go:100`) — sandbox and real chip results must never mix into one number.
- **Item fields**: `HandsTogether int64`, `WinsLow int64`, `WinsHigh int64`, `Ties int64`,
  `HeadsUpHandsTogether int64`, `NetChangeLow int64` (cumulative chip result for `idLow`, heads-up
  hands only — see below), `UpdatedAt`.

```go
type Stats struct {
    HandsTogether        int64 `dynamodbav:"hands_together"`
    WinsLow              int64 `dynamodbav:"wins_low"`
    WinsHigh             int64 `dynamodbav:"wins_high"`
    Ties                 int64 `dynamodbav:"ties"`
    HeadsUpHandsTogether int64 `dynamodbav:"heads_up_hands_together"`
    NetChangeLow         int64 `dynamodbav:"net_change_low"`
}
```

### Why chip P&L is heads-up-only

In a 3+-way hand, "how many chips did I win off *this specific* opponent" has no single correct
answer — a pot both of you contributed to might be won by a third player, or split between you and
someone else. `HandOutcome.Payouts`/`Contributions` (`engine/hand/hand.go:170-171`) are per-player
totals for the whole hand, not per-opponent-pair, and there's no reliable way to attribute a shared
pot's net result to one specific other player once a third contributor is involved. Rather than
publish a number that's quietly wrong in every multi-way pot, `NetChangeLow` only accumulates on hands
where `len(outcome.Participants) == 2` (a genuine heads-up hand between exactly this pair), tracked
separately via `HeadsUpHandsTogether` so the UI can show "chip result over N heads-up hands" instead of
implying it covers every hand they've shared a table for. `HandsTogether`/`WinsLow`/`WinsHigh`/`Ties`
have no such caveat — hand-win/loss/tie is well-defined for any table size using
`slices.Contains(outcome.Winners, id)`, the exact same check `handItemFor` already uses
(`app.go:543-545`).

## Write path

Hook into the same place `pokerstats.Store.RecordHand` is already called —
`newTableManager`'s `onHandComplete` closure, `app.go:419-496`, right after the
`pokerStatsStore.RecordHand` call at `app.go:472`. For every unordered pair within
`outcome.Participants` (max C(9,2)=36 for a 9-max table, computed once per completed hand — the same
place `pokerstats.Analyze` already walks every participant), call a new
`matchupStore.RecordHand(ctx, mode, outcome)`.

`matchup.Store.RecordHand` mirrors `pokerstats.Store.RecordHand`'s guard pattern exactly
(`stats.go:47-56,71-84`): one `TransactWriteItem` per completed hand, gated by a
`BuildPutTxItemIfAbsent` guard keyed `guard#<handID>#<pairKey>` (same 90-day TTL constant reuse,
`guardTTLDays`) so a duplicate `onHandComplete` invocation (retry, reconnect-replay) can never
double-count a pair. Increments via `ADD hands_together :one, wins_low :winLow, ...` — no
read-then-write, consistent with `api/CLAUDE.md`'s "Correctness = DynamoDB conditional writes" rule
(that rule is written for `tablestore.CommitAction`, but the same non-negotiable — never
read-modify-write shared counters — applies here for the identical reason: concurrent hand-completes
across different tables touching the same pair must never race a plain read-then-put).

## Read path

New handler `GET /players/me/matchups/:opponentId?mode=sandbox`, alongside the existing
`playernotes`/`pokerstats`-style read-only per-player endpoints (`internal/api/v1`). Looks up
`pair#<mode>#<idLow>#<idHigh>` (computed from `claims.Sub` + the path param, same IDOR-safe pattern
`api/CLAUDE.md` requires: "player identity comes from the JWT `sub` ... never trust a client-supplied
id" — the path param names the *opponent*, never substitutes for the viewer's own ID). Response maps
`WinsLow`/`WinsHigh`/`NetChangeLow` onto "viewer" / "opponent" based on which of the two IDs is
lexicographically lower — this remapping happens once, in the handler, not duplicated on read. Missing
item (no pair history yet) returns zeroed stats, same as `pokerstats.Store.Get`'s `item == nil` branch
(`stats.go:105-107`).

## How this differs from `playernotes`

`internal/playernotes/store.go` already exists — a manual, one-directional, free-text note one player
writes about another (e.g. "always 3-bets light"). The comparator is the opposite shape: automatic,
symmetric, numeric. Both can coexist on the same opponent-facing surface without conflict — a note is
an opinion the viewer wrote, a matchup is a fact derived from hand history neither player can edit.

## ui/ surface

Add to `ui/src/app/profile/[id]` — the existing public read-only showcase route for another player
(`ui/CLAUDE.md`'s Layout section: "that route is the public read-only showcase of another player").
A small card ("Vocês já jogaram N mãos juntos, você venceu X, [opponent] venceu Y") fetched via
`GET /players/me/matchups/:id`, rendered only when `hands_together > 0` (no empty-state card for pairs
that have never shared a table). No new realtime hook — this is a one-shot fetch like the rest of the
profile showcase, not something `useLobbyRealtime`/`useTableRealtime` needs to push live.

## Testing

- `matchup.Store.RecordHand`: unit test mirroring `pokerstats/stats_test.go`'s
  `TestAnalyzeVPIPPFRAndThreeBet` shape — feed a synthetic `HandOutcome` for a 3-way hand and assert
  `HandsTogether`/`Wins*`/`Ties` increment for every pair but `NetChangeLow`/`HeadsUpHandsTogether`
  stay untouched; feed a 2-participant `HandOutcome` and assert `NetChangeLow` moves by
  `Payouts[idLow] - Contributions[idLow]`.
- Duplicate-guard test: call `RecordHand` twice with the same `handID` and pair, assert counters
  increment only once — same shape as any existing `pokerstats` idempotency test.
- Handler test: `GET /players/me/matchups/:id` with no existing pair item returns zeroed stats, not
  a 404 — a pair with no shared history isn't an error.
- Integration: extend `internal/sessionlog/store_integration_test.go`'s DynamoDB-Local suite (or add
  a sibling `internal/matchup/store_integration_test.go`) with a real `TransactWrite` round-trip.

## Out of scope

- A "biggest pot together" figure — would need a conditional-max DynamoDB update (no native `MAX` in
  a single `ADD` expression the way there is for sums), and it's a nice-to-have, not part of what was
  asked for ("comparador"). Add later as a `read-modify-write-with-condition-expression` on top of this
  same item if requested.
- A ranked list of "your toughest opponents" (all pairs, sorted) — that's a different query shape
  (scan/GSI over every pair a player has, not "look up one named pair") and wasn't asked for; this
  spec is a two-player comparator, not an opponent leaderboard.
- Historical backfill for hands recorded before this ships — `poker_player_matchups` starts empty;
  older hands in `poker_player_hands` are not retroactively aggregated (would require a one-off
  migration script reading the full historical partition this spec explicitly avoids querying live).
