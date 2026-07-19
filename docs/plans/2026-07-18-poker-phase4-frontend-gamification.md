# Phase 4 — Frontend Polish & Gamification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The gamified experience the brief asks for, built on top of the already-correct engine/table-server/room
layers from Phases 0-3: hand equity display, achievements, leaderboard, sandbox credit roulette (OVERVIEW.md §9),
and a real-time SPA with card animations using the SVGs already committed under `ui/svgs/` (OVERVIEW.md §6).

**Architecture:** Three small, independent gamification backends (equity is computed inline per-snapshot; achievements
and leaderboard both hook into the same hand-completion event Phase 3's `table.Actor` already reaches every hand;
roulette is a standalone once-per-24h endpoint) sit behind new routes, all still under `/v1.0/`. The frontend is a
Next.js SPA mirroring `ctech-wallet/ui`'s exact stack and deploy shape (static export → S3 → CloudFront), reusing
`@aoctech/auth-client` for OAuth and `@aoctech/ws-client` for the table WebSocket — no new frontend infra pattern,
only a new consumer of patterns that already exist and are proven in production.

**Tech Stack:** Go 1.26 (backend gamification), Next.js 16 + React 19 + TypeScript + Tailwind + ShadCN
(frontend, matching `ctech-wallet/ui`'s `package.json` exactly), `@aoctech/auth-client`, `@aoctech/ws-client`,
`@tanstack/react-query`, AWS CDK (S3 + CloudFront, matching `ctech-wallet/cdk/lib/frontend-stack.ts`).

## Global Constraints

- Every backend route lives under `/v1.0/` (existing convention).
- Hand equity is computed server-side only, sent privately to the owning viewer, never derived or computed
  client-side (ARCHITECTURE.md §6 — client-side equity would require exposing remaining-deck composition, which
  leaks information about opponents' hole cards by elimination).
- Public rooms: equity display always on, no toggle. Private rooms: `equity_display_enabled` from the room's
  config (Phase 3), default `true`.
- No public leaderboard ever exposes a real-money amount won/lost (OVERVIEW.md §9.1) — moot for this plan since
  Phase 3 only ever creates sandbox rooms, but the leaderboard schema itself must have no such field, so a future
  real-money phase can't accidentally wire one in by just "adding a column".
- Achievements are data-driven (`Achievement{Key, Metric, Tiers}`), never hardcoded per-achievement `if` branches
  (OVERVIEW.md §9.2).
- Sandbox roulette never touches the real-money ledger — it only ever calls `walletclient.Client.Credit` against
  the sandbox scope already established in Phase 3 (OVERVIEW.md §9.3).
- The frontend is a static export (`next build` with `output: 'export'` in production, `next dev` with a rewrite
  proxy in development) — matches `ctech-wallet/ui/next.config.ts` exactly, so the same CloudFront + ALB-origin
  pattern applies with zero new infra concepts.
- The UI must read as a game, not a SaaS dashboard (OVERVIEW.md §6) — every table-state-changing frontend update
  goes through a CSS transition, never an instant DOM swap.

---

### Task 1: Hand equity Monte Carlo calculator

**Files:**
- Create: `api/internal/engine/equity/equity.go`
- Test: `api/internal/engine/equity/equity_test.go`

**Interfaces:**
- Consumes: `deck.Card`, `handeval.Best7` (existing engine packages).
- Produces: `func Estimate(hole [2]deck.Card, board []deck.Card, deadCards []deck.Card, numOpponents, iterations
  int) (float64, error)` — consumed by Task 2's snapshot wiring.

`deadCards` excludes cards already known to be out of play (the viewer's own hole cards, the board) from the
sampling pool — opponents' actual hole cards are never known to this function, which is the entire point: it
estimates against random opponent ranges, not real ones.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/engine/equity/equity_test.go
package equity

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func TestPocketAcesHeadsUpPreflopIsStrongFavorite(t *testing.T) {
	hole := [2]deck.Card{{Rank: deck.Ace, Suit: deck.Clubs}, {Rank: deck.Ace, Suit: deck.Diamonds}}
	eq, err := Estimate(hole, nil, nil, 1, 2000)
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	if eq < 0.75 {
		t.Fatalf("expected pocket aces heads-up preflop equity above 0.75, got %f", eq)
	}
}

func TestEquitySumsToAtMostOneAcrossExhaustiveOutcome(t *testing.T) {
	hole := [2]deck.Card{{Rank: deck.King, Suit: deck.Clubs}, {Rank: deck.Queen, Suit: deck.Diamonds}}
	board := []deck.Card{
		{Rank: deck.King, Suit: deck.Hearts}, {Rank: deck.Seven, Suit: deck.Spades}, {Rank: deck.Two, Suit: deck.Clubs},
	}
	eq, err := Estimate(hole, board, nil, 1, 2000)
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	if eq <= 0 || eq > 1 {
		t.Fatalf("expected equity in (0,1], got %f", eq)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/equity/... -v`
Expected: FAIL with "undefined: Estimate".

- [ ] **Step 3: Implement `equity.go`**

```go
// api/internal/engine/equity/equity.go
// Package equity estimates a player's win probability via Monte Carlo
// sampling against random opponent ranges (OVERVIEW.md §9.4) — never against
// real opponent hole cards, which this package has no access to by design.
package equity

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval"
)

// Estimate returns the fraction of iterations in which hole (plus a random
// completion of the board and each opponent's random 2 cards) produces the
// best 7-card hand, ties split fractionally. deadCards are removed from the
// sampling pool in addition to hole and board (reserved for a future caller
// that also wants to exclude folded/known-dead cards; nil is fine today).
func Estimate(hole [2]deck.Card, board []deck.Card, deadCards []deck.Card, numOpponents, iterations int) (float64, error) {
	if numOpponents < 1 {
		return 0, fmt.Errorf("equity: numOpponents must be at least 1")
	}
	pool := remainingDeck(hole, board, deadCards)
	need := (5 - len(board)) + numOpponents*2
	if need > len(pool) {
		return 0, fmt.Errorf("equity: not enough remaining cards to sample %d opponents", numOpponents)
	}

	var wins float64
	for i := 0; i < iterations; i++ {
		shuffled, err := shuffleSubset(pool, need)
		if err != nil {
			return 0, err
		}
		fullBoard := append(append([]deck.Card{}, board...), shuffled[:5-len(board)]...)
		rest := shuffled[5-len(board):]

		myHand := best7(hole, fullBoard)
		best := myHand
		tiesWithMe := 1
		for o := 0; o < numOpponents; o++ {
			oppHole := [2]deck.Card{rest[o*2], rest[o*2+1]}
			oppScore := best7(oppHole, fullBoard)
			switch {
			case oppScore > best:
				best = oppScore
				tiesWithMe = 0
			case oppScore == best && best == myHand:
				tiesWithMe++
			}
		}
		if best == myHand {
			wins += 1.0 / float64(tiesWithMe)
		}
	}
	return wins / float64(iterations), nil
}

func best7(hole [2]deck.Card, board []deck.Card) handeval.Score {
	var full [7]deck.Card
	full[0], full[1] = hole[0], hole[1]
	copy(full[2:], board)
	return handeval.Best7(full)
}

func remainingDeck(hole [2]deck.Card, board, dead []deck.Card) []deck.Card {
	excluded := make(map[deck.Card]bool, 2+len(board)+len(dead))
	excluded[hole[0]] = true
	excluded[hole[1]] = true
	for _, c := range board {
		excluded[c] = true
	}
	for _, c := range dead {
		excluded[c] = true
	}
	out := make([]deck.Card, 0, 52-len(excluded))
	for _, s := range []deck.Suit{deck.Clubs, deck.Diamonds, deck.Hearts, deck.Spades} {
		for r := deck.Two; r <= deck.Ace; r++ {
			c := deck.Card{Rank: r, Suit: s}
			if !excluded[c] {
				out = append(out, c)
			}
		}
	}
	return out
}

// shuffleSubset draws the first n cards of a CSPRNG (not fairness-critical —
// this is an internal estimation tool, not the dealt deck) Fisher-Yates
// partial shuffle of pool, without mutating the caller's slice.
func shuffleSubset(pool []deck.Card, n int) ([]deck.Card, error) {
	cp := append([]deck.Card{}, pool...)
	for i := 0; i < n; i++ {
		j, err := randIntn(len(cp) - i)
		if err != nil {
			return nil, err
		}
		j += i
		cp[i], cp[j] = cp[j], cp[i]
	}
	return cp[:n], nil
}

func randIntn(n int) (int, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	return int(binary.BigEndian.Uint64(b[:]) % uint64(n)), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/engine/equity/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/equity
git commit -m "feat(equity): Monte Carlo hand-equity estimator against random opponent ranges"
```

---

### Task 2: Wire equity into the per-viewer snapshot

**Files:**
- Modify: `api/internal/engine/hand/snapshot.go`
- Modify: `api/internal/table/actor.go`
- Test: `api/internal/engine/hand/snapshot_test.go`

**Interfaces:**
- Extends `hand.Snapshot`/`SeatView` with an `Equity *float64` field, populated only for the requesting viewer's
  own (still-active) seat, and only when `equityEnabled` is true.

Equity computation stays out of `hand.ViewFor` itself — the engine package has no dependency on `equity` today
(and shouldn't gain a Monte Carlo dependency for what is fundamentally table-server orchestration, not hand
rules) — `table.Actor` computes it and attaches it to the snapshot it already builds per viewer.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/engine/hand/snapshot_test.go — add
func TestViewForLeavesEquityNilByDefault(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	view := table.ViewFor("p1")
	for _, s := range view.Seats {
		if s.PlayerID == "p1" && s.Equity != nil {
			t.Fatal("expected ViewFor itself to never populate Equity — that's table.Actor's job (Phase 4 Task 2)")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/hand/... -run TestViewForLeavesEquityNil -v`
Expected: FAIL with "s.Equity undefined" (field doesn't exist yet).

- [ ] **Step 3: Add the field**

```go
// api/internal/engine/hand/snapshot.go — SeatView, add
	Equity *float64 `json:"equity,omitempty"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/engine/hand/... -v`
Expected: PASS.

- [ ] **Step 5: Populate it in `table.Actor.broadcastAll`**

```go
// api/internal/table/actor.go — Actor struct, add
	equityEnabled bool // set once at construction (Phase 3's room config)
```

```go
// api/internal/table/actor.go — New's signature and body
func New(id string, t *hand.Table, store *tablestore.Store, broadcast func(string, hand.Snapshot), equityEnabled bool) *Actor {
	return &Actor{
		id: id, table: t, store: store, broadcast: broadcast, equityEnabled: equityEnabled,
		cmds: make(chan Command, 64), seenIDs: make(map[string]bool),
		disconnectedSince: make(map[string]time.Time), consecutiveDisconnectedHands: make(map[string]int),
		actionDeadline: 30 * time.Second, disconnectGrace: 45 * time.Second,
	}
}
```

Update every existing call site (`tablemanager.Manager.Acquire`, every test in Phases 2-3) to pass a final
`false`/`true` argument — this is a source-breaking signature change, so it must be applied everywhere `table.New`
is called, not just here.

```go
// api/internal/table/actor.go — broadcastAll, replace the body
func (a *Actor) broadcastAll() {
	if a.broadcast == nil {
		return
	}
	street := a.table.Stage()
	for _, id := range a.viewerIDsSnapshot() {
		snap := a.table.ViewFor(id)
		if a.equityEnabled && (street == hand.PreFlop || street == hand.Flop || street == hand.Turn || street == hand.River) {
			a.attachEquity(id, &snap)
		}
		a.broadcast(id, snap)
	}
}

// attachEquity computes viewerID's own equity and sets it on their own
// SeatView entry — never anyone else's, since that's the exact leak
// ARCHITECTURE.md §8/§6 forbids.
func (a *Actor) attachEquity(viewerID string, snap *hand.Snapshot) {
	hole, board, ok := a.table.HoleAndBoardForActor(viewerID)
	if !ok {
		return
	}
	opponents := 0
	for _, s := range snap.Seats {
		if s.PlayerID != viewerID && (s.State == "active" || s.State == "all_in") {
			opponents++
		}
	}
	if opponents == 0 {
		return
	}
	eq, err := equity.Estimate(hole, board, nil, opponents, 500)
	if err != nil {
		return // equity is a nice-to-have display, never worth failing the broadcast over
	}
	for i := range snap.Seats {
		if snap.Seats[i].PlayerID == viewerID {
			snap.Seats[i].Equity = &eq
		}
	}
}
```

`HoleAndBoardForActor` doesn't exist yet:

```go
// api/internal/engine/hand/hand.go — add near PlayersForActor
// HoleAndBoardForActor exposes one player's hole cards plus the current
// board — used by Phase 4's equity calculator, which needs raw cards (not
// the redacted card-code strings ViewFor produces) to feed handeval.Best7's
// existing deck.Card-based API. ok is false if playerID isn't found or isn't
// currently dealt a hand (folded/sitting out/pending entry all still HAVE
// HoleCards set from a prior deal, but returning them for a folded player
// would be meaningless to compute equity for, so ok is false unless Active
// or AllIn).
func (t *Table) HoleAndBoardForActor(playerID string) ([2]deck.Card, []deck.Card, bool) {
	p := t.playerByID(playerID)
	if p == nil || (p.State != Active && p.State != AllIn) {
		return [2]deck.Card{}, nil, false
	}
	return p.HoleCards, t.board, true
}
```

Add `"gopkg.aoctech.app/poker/api/internal/engine/equity"` and `"gopkg.aoctech.app/poker/api/internal/engine/hand"`
imports to `actor.go`.

- [ ] **Step 6: Thread `equityEnabled` from the room config through `tablemanager`/`app.go`**

```go
// api/internal/tablemanager/manager.go — Acquire's call to table.New
	actor := table.New(tableID, ht, m.store, m.broadcastFor(tableID), m.equityEnabledFor(tableID))
```

`Manager` needs a way to know a table's equity setting without importing `roomstore` (same layering rule as Task
9 of the Phase 3 plan) — reuse the same variadic-hook pattern already established there instead of a new field:

```go
// api/internal/tablemanager/manager.go — replace the m.equityEnabledFor(tableID) call with a stored default,
// and let callers override it via the acquire hook (consistent with escalation's own wiring in Phase 3):
	actor := table.New(tableID, ht, m.store, m.broadcastFor(tableID), true) // default on; Phase 4's rooms.go overrides per-room below
```

```go
// api/internal/api/v1/rooms.go — createRoom, extend the escalation Acquire hook (or add a standalone one
// when there's no blind escalation) to also set equity — table.Actor has no exported "set equity after the
// fact" today, so this requires equityEnabled to be an Acquire-time decision, not a post-hoc one:
```

This surfaces a real sequencing gap: `equityEnabled` is baked into `table.New` at Actor-construction time, but
`rooms.go`'s `createRoom` only calls `Acquire` for *private rooms with escalation configured* (Phase 3 Task 9) —
every other room (public, or private without escalation) never calls `Acquire` until the first WebSocket
connection reaches it (Phase 2 Task 7's lazy `manager.Acquire` call), which has no room-config context at all
today. Fix by giving the WS gateway's `seed` closure (Phase 3 Task 5's `roomBackedSeed`) the equity flag too,
since it already loads the `roomstore.Room` row:

```go
// api/internal/app/app.go — replace roomBackedSeed AND how it's threaded into RegisterTableWS
// equityForRoom loads the room's equity_display_enabled flag (public rooms:
// always true, private: whatever was configured at creation — Phase 3
// roomstore.Room already carries this field either way).
func equityForRoom(rooms *roomstore.Store, tableID string) bool {
	room, err := rooms.Get(context.Background(), tableID)
	if err != nil || room == nil {
		return true
	}
	return room.EquityDisplayEnabled
}
```

`RegisterTableWS`/`RegisterTableProxy` both call `manager.Acquire(ctx, tableID, seed(tableID))` today with no
equity argument — extend `Manager.Acquire`'s existing variadic `onCreated ...func(*Actor)` hook (Phase 3 Task 9)
instead of another signature change: `table.Actor` needs a settable equity flag reachable from that hook.

```go
// api/internal/table/actor.go — add
// SetEquityEnabledForActor lets tablemanager's Acquire hook configure equity
// display after construction — New's own equityEnabled parameter (Step 5)
// remains the constructor default (true) for callers (mostly tests) that
// never need per-room configuration.
func (a *Actor) SetEquityEnabledForActor(enabled bool) { a.equityEnabled = enabled }
```

```go
// api/internal/api/v1/tablews.go and tableproxy.go — both Acquire call sites, replace with
			actor, err = manager.Acquire(ctx, tableID, seed(tableID), func(a *table.Actor) {
				a.SetEquityEnabledForActor(equityForRoom(rooms, tableID))
			})
```

Thread a `rooms *roomstore.Store` parameter into `RegisterTableWS`/`RegisterTableProxy` (both currently take
`seed` but not `rooms`) and their call sites in `router.go`.

- [ ] **Step 7: Run the full suite**

Run: `go build ./... && go test ./... -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add api/internal/engine/hand api/internal/table api/internal/tablemanager api/internal/api/v1 api/internal/app/app.go
git commit -m "feat(table): compute and attach per-viewer hand equity, respecting the room's display toggle"
```

---

### Task 3: Achievement catalog and progress tracking

**Files:**
- Create: `api/internal/achievements/catalog.go`
- Create: `api/internal/achievements/store.go`
- Create: `api/internal/achievements/service.go`
- Test: `api/internal/achievements/service_test.go`
- Modify: `api/internal/table/actor.go`

**Interfaces:**
- Consumes: a new `hand.HandOutcome` the engine emits when a hand completes (this task adds it), `dynamo.Base`
  (existing shared package).
- Produces: `type Achievement struct{...}`, `var Catalog []Achievement`, `type Store struct{...}`,
  `func NewStore(db *dynamodb.Client, env string) *Store`, `type Service struct{...}`, `func NewService(store
  *Store) *Service`, `func (s *Service) RecordHand(ctx, tableID string, outcome hand.HandOutcome) error` — the
  last one is consumed by Task 6's wiring into `table.Actor`.

- [ ] **Step 1: Emit a structured hand outcome from the engine**

```go
// api/internal/engine/hand/hand.go — add near ViewFor
// HandOutcome summarizes a just-completed hand for gamification consumers
// (Phase 4's achievements/leaderboard) — deliberately separate from Snapshot
// (the wire-safe per-viewer view): this is server-internal, never sent to a
// client, and carries information ViewFor intentionally withholds (e.g. every
// participant's true state history for the hand, not just the redacted final
// board state).
type HandOutcome struct {
	Winners            []string // player IDs who received any payout
	WinningCategory    string   // handeval category name of the best winning hand ("pair", "flush", ...), "" if WonWithoutShowdown
	WonWithoutShowdown bool     // true if every other player folded (never reached Showdown)
	ComebackWinners    []string // winners who were AllIn at some point this hand — see doc note on this being a simplified proxy
	Participants       []string // every player dealt into this hand (for VPIP/hands-played counters)
}

var categoryNames = map[handeval.Category]string{
	handeval.HighCard: "high_card", handeval.Pair: "pair", handeval.TwoPair: "two_pair",
	handeval.ThreeOfAKind: "three_of_a_kind", handeval.Straight: "straight", handeval.Flush: "flush",
	handeval.FullHouse: "full_house", handeval.FourOfAKind: "four_of_a_kind",
	handeval.StraightFlush: "straight_flush", handeval.RoyalFlush: "royal_flush",
}
```

`handeval.Category`'s ten constant names must match exactly what `handeval.go` actually declares — confirm against
`api/internal/engine/handeval/handeval.go`'s `Category` block before transcribing (Task 6 of the foundations plan
is the source of truth for the exact identifiers; this task must not invent new ones).

```go
// api/internal/engine/hand/hand.go — runShowdown, capture the outcome as the last thing it does, before rotateDealer
	outcome := HandOutcome{Winners: winnersList(payouts), Participants: participantIDs(contributions)}
	if wonAtShowdown {
		outcome.WinningCategory = categoryNames[bestCategory]
	} else {
		outcome.WonWithoutShowdown = true
	}
	for _, id := range outcome.Winners {
		if wasEverAllIn[id] {
			outcome.ComebackWinners = append(outcome.ComebackWinners, id)
		}
	}
	t.lastOutcome = &outcome
```

This requires threading three new bits of bookkeeping through `runShowdown` (`wonAtShowdown`/`bestCategory` — the
category of whichever `bestScore` actually won a layer, tracked across the existing per-layer loop rather than
recomputed — and `wasEverAllIn`, a per-hand set populated wherever `p.State = AllIn` is already assigned, i.e. in
`postBlind`, `betting.Round.Act`'s all-in branch surfaced back through `Act`, and nowhere else). Add:

```go
// api/internal/engine/hand/hand.go — Table struct, add
	lastOutcome  *HandOutcome
	wasEverAllIn map[string]bool
```

```go
// api/internal/engine/hand/hand.go — StartHand, reset it alongside t.payouts = nil
	t.wasEverAllIn = make(map[string]bool)
```

```go
// api/internal/engine/hand/hand.go — Act, right after `if bs.AllIn { p.State = AllIn }`
	if bs.AllIn {
		p.State = AllIn
		t.wasEverAllIn[p.ID] = true
	}
```

```go
// api/internal/engine/hand/hand.go — postBlind, right after `p.State = AllIn`
	if amount >= p.Stack {
		amount = p.Stack
		p.State = AllIn
		if t.wasEverAllIn == nil {
			t.wasEverAllIn = make(map[string]bool)
		}
		t.wasEverAllIn[p.ID] = true
	}
```

`runShowdown`'s existing per-layer loop already computes `bestScore`/`winners` per layer — track the single
overall winning score/category across the whole hand (the highest layer's winner, since side-pot layering means
the main-pot winner's hand is the one meaningfully "the winning hand" for a category achievement) by capturing it
on the LAST layer processed (layers are iterated in ascending contribution order — `sidepots.ComputeSidePots`'s
existing contract from the foundations plan — so the last layer is the highest, main-pot-equivalent one):

```go
// api/internal/engine/hand/hand.go — runShowdown, declare before the `for _, layer := range layers` loop
	var finalWinners []string
	var finalBestScore handeval.Score
	wonAtShowdown := t.stage == Showdown
```

```go
// api/internal/engine/hand/hand.go — inside the layer loop, after the winners/bestScore for this layer are computed
		finalWinners, finalBestScore = winners, bestScore
```

```go
// api/internal/engine/hand/hand.go — after the layer loop, before `t.payouts = payouts`
	outcome := HandOutcome{Winners: dedupeIDs(finalWinners), Participants: participantIDs(contributions)}
	if wonAtShowdown {
		outcome.WinningCategory = categoryNames[finalBestScore.Category()]
	} else {
		outcome.WonWithoutShowdown = true
	}
	for _, id := range outcome.Winners {
		if t.wasEverAllIn[id] {
			outcome.ComebackWinners = append(outcome.ComebackWinners, id)
		}
	}
	t.lastOutcome = &outcome
```

`handeval.Score` needs a `Category()` accessor (it currently only exposes the packed `uint32` via `makeScore`,
unexported) and `dedupeIDs`/`participantIDs`/`winnersList` are small helpers:

```go
// api/internal/engine/handeval/handeval.go — add
// Category extracts the top-nibble category from a packed Score — exported
// for Phase 4's achievements, which need the human-facing category name, not
// just an orderable value.
func (s Score) Category() Category { return Category(s >> 24) }
```

```go
// api/internal/engine/hand/hand.go — add
func dedupeIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func participantIDs(contributions []sidepots.Contribution) []string {
	out := make([]string, len(contributions))
	for i, c := range contributions {
		out[i] = c.PlayerID
	}
	return out
}
```

Add `func (t *Table) LastOutcomeForActor() *HandOutcome { return t.lastOutcome }` next to `PlayersForActor`.

- [ ] **Step 2: Write the failing achievements test**

```go
// api/internal/achievements/service_test.go
package achievements

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type memStore struct {
	progress map[string]map[string]int // playerID -> achievementKey -> counter
}

func newMemStore() *memStore { return &memStore{progress: map[string]map[string]int{}} }

func (m *memStore) Increment(_ context.Context, playerID, key string, by int) (int, error) {
	if m.progress[playerID] == nil {
		m.progress[playerID] = map[string]int{}
	}
	m.progress[playerID][key] += by
	return m.progress[playerID][key], nil
}

func TestRecordHandIncrementsWinCounterForWinners(t *testing.T) {
	store := newMemStore()
	svc := NewServiceWithStore(store)
	outcome := hand.HandOutcome{Winners: []string{"p1"}, WinningCategory: "flush", Participants: []string{"p1", "p2"}}

	if err := svc.RecordHand(context.Background(), "table-1", outcome); err != nil {
		t.Fatalf("record hand: %v", err)
	}
	if store.progress["p1"][KeyWins] != 1 {
		t.Fatalf("expected p1's win counter incremented once, got %d", store.progress["p1"][KeyWins])
	}
	if store.progress["p1"][KeyWinByCategory("flush")] != 1 {
		t.Fatalf("expected p1's flush-win counter incremented once, got %d", store.progress["p1"][KeyWinByCategory("flush")])
	}
	if store.progress["p1"][KeyHandsPlayed] != 1 || store.progress["p2"][KeyHandsPlayed] != 1 {
		t.Fatalf("expected both participants' hands-played counters incremented")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/achievements/... -v`
Expected: FAIL with "undefined: NewServiceWithStore".

- [ ] **Step 4: Implement the catalog**

```go
// api/internal/achievements/catalog.go
// Package achievements implements OVERVIEW.md §9.2's star-tier system: a
// data-driven catalog (never per-achievement if/else branches), with rarer
// metrics given shorter tier ladders than common ones.
package achievements

import "fmt"

type Tier struct {
	Stars     int
	Threshold int
}

type Achievement struct {
	Key    string
	Metric string
	Tiers  []Tier
}

const (
	KeyWins         = "wins"
	KeyHandsPlayed  = "hands_played"
	KeyComeback     = "comeback"
	KeyBluff        = "bluff"
	KeySurvivor     = "survivor"
)

// KeyWinByCategory namespaces the per-hand-category ladder (OVERVIEW.md §9.2
// "Vencer por categoria de mão") — one counter per handeval category name.
func KeyWinByCategory(category string) string { return fmt.Sprintf("win_category_%s", category) }

// Catalog is intentionally short for MVP — exactly the ladders OVERVIEW.md
// §9.2 specifies by name; a new achievement is adding one entry here, never
// new code.
var Catalog = []Achievement{
	{Key: KeyWins, Metric: "hand_won", Tiers: []Tier{{1, 1}, {2, 10}, {3, 100}, {4, 1000}, {5, 10000}}},
	{Key: KeyWinByCategory("royal_flush"), Metric: "hand_won_with_category", Tiers: []Tier{{1, 1}, {2, 5}, {3, 10}, {4, 25}, {5, 50}}},
	{Key: KeyHandsPlayed, Metric: "hand_played", Tiers: []Tier{{1, 100}, {2, 1000}, {3, 10000}, {4, 50000}, {5, 100000}}},
	{Key: KeyComeback, Metric: "won_after_all_in", Tiers: []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}}},
	{Key: KeyBluff, Metric: "won_without_showdown_weaker_hand", Tiers: []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}}},
	{Key: KeySurvivor, Metric: "hands_without_leaving", Tiers: []Tier{{1, 50}, {2, 250}, {3, 1000}, {4, 5000}, {5, 25000}}},
}
```

`KeyBluff` is tracked here as a catalog entry but **never incremented by this task** — real bluff detection needs
each folded opponent's actual hole cards compared against the winner's hand, which `hand.HandOutcome` does not
carry (see this plan's Closing Note: flagged as a deliberate simplification, not silently dropped).

- [ ] **Step 5: Implement the service**

```go
// api/internal/achievements/service.go
package achievements

import (
	"context"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// progressStore is the subset of persistence RecordHand needs — narrowed to
// an interface so tests use an in-memory double instead of DynamoDB Local.
type progressStore interface {
	// Increment adds `by` to playerID's counter for key and returns the new
	// total (needed to check tier crossing without a separate read).
	Increment(ctx context.Context, playerID, key string, by int) (int, error)
}

type Service struct {
	store progressStore
}

func NewServiceWithStore(store progressStore) *Service { return &Service{store: store} }

// RecordHand increments every counter this hand's outcome affects, for every
// affected player, then checks each affected (player, achievement) pair for
// a newly crossed tier. tableID is accepted for future per-table analytics
// but unused today — no per-table achievement scoping exists yet.
func (s *Service) RecordHand(ctx context.Context, tableID string, outcome hand.HandOutcome) error {
	for _, id := range outcome.Participants {
		if _, err := s.store.Increment(ctx, id, KeyHandsPlayed, 1); err != nil {
			return err
		}
	}
	for _, id := range outcome.Winners {
		if _, err := s.store.Increment(ctx, id, KeyWins, 1); err != nil {
			return err
		}
		if outcome.WinningCategory != "" {
			if _, err := s.store.Increment(ctx, id, KeyWinByCategory(outcome.WinningCategory), 1); err != nil {
				return err
			}
		}
	}
	for _, id := range outcome.ComebackWinners {
		if _, err := s.store.Increment(ctx, id, KeyComeback, 1); err != nil {
			return err
		}
	}
	return nil
}

// TierCrossed reports the highest tier whose threshold total meets or
// exceeds newTotal but did not before this increment — callers (Task 6's
// wiring) use it to decide whether to fire an unlock notification. Returns
// (0, false) if no new tier was crossed (newTotal - delta already met a
// higher threshold, or no tier's threshold is met at all).
func TierCrossed(key string, previousTotal, newTotal int) (int, bool) {
	for _, a := range Catalog {
		if a.Key != key {
			continue
		}
		for _, t := range a.Tiers {
			if previousTotal < t.Threshold && newTotal >= t.Threshold {
				return t.Stars, true
			}
		}
	}
	return 0, false
}
```

`Increment`'s interface returns only the new total, not the previous one — `TierCrossed` needs both. Fix by
having `progressStore.Increment` return both:

```go
// api/internal/achievements/service.go — replace the interface and every call site
type progressStore interface {
	Increment(ctx context.Context, playerID, key string, by int) (previous, current int, err error)
}
```

Update `RecordHand`'s four call sites to capture `(prev, cur, err)` and call `TierCrossed(key, prev, cur)`,
returning the crossed-tier info via a growing `[]TierUnlock` return value:

```go
// api/internal/achievements/service.go — final version
type TierUnlock struct {
	PlayerID string
	Key      string
	Stars    int
}

func (s *Service) RecordHand(ctx context.Context, tableID string, outcome hand.HandOutcome) ([]TierUnlock, error) {
	var unlocks []TierUnlock
	bump := func(playerID, key string) error {
		prev, cur, err := s.store.Increment(ctx, playerID, key, 1)
		if err != nil {
			return err
		}
		if stars, ok := TierCrossed(key, prev, cur); ok {
			unlocks = append(unlocks, TierUnlock{PlayerID: playerID, Key: key, Stars: stars})
		}
		return nil
	}
	for _, id := range outcome.Participants {
		if err := bump(id, KeyHandsPlayed); err != nil {
			return nil, err
		}
	}
	for _, id := range outcome.Winners {
		if err := bump(id, KeyWins); err != nil {
			return nil, err
		}
		if outcome.WinningCategory != "" {
			if err := bump(id, KeyWinByCategory(outcome.WinningCategory)); err != nil {
				return nil, err
			}
		}
	}
	for _, id := range outcome.ComebackWinners {
		if err := bump(id, KeyComeback); err != nil {
			return nil, err
		}
	}
	return unlocks, nil
}
```

Update `service_test.go`'s `memStore.Increment` signature to `(previous, current int, err error)` and its
`svc.RecordHand` call to capture the `[]TierUnlock` return.

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/achievements/... ./internal/engine/hand/... ./internal/engine/handeval/... -v`
Expected: PASS.

- [ ] **Step 7: Implement DynamoDB-backed `Store`**

```go
// api/internal/achievements/store.go
package achievements

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const tableProgress = "poker_achievement_progress"

// Store persists per-player achievement counters, pk=player_id, sk=achievement_key.
type Store struct {
	base dynamo.Base
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableProgress)}
}

func (s *Store) Increment(ctx context.Context, playerID, key string, by int) (previous, current int, err error) {
	sk := key
	before, err := s.base.GetItem(ctx, playerID, sk)
	if err != nil {
		return 0, 0, fmt.Errorf("achievements: get: %w", err)
	}
	prev := 0
	if before != nil {
		decoded, decErr := dynamo.Decode[struct {
			Counter int `dynamodbav:"counter"`
		}](before)
		if decErr == nil {
			prev = decoded.Counter
		}
	}
	newTotal, err := s.base.AtomicIncrement(ctx, playerID, &sk, "counter")
	if err != nil {
		return 0, 0, fmt.Errorf("achievements: increment: %w", err)
	}
	return prev, int(newTotal), nil
}
```

`AtomicIncrement` (existing `dynamo.Base` method, confirmed in Phase 2's research) requires the item to already
exist for `ADD` to work predictably from zero — DynamoDB's `ADD` on a missing attribute initializes it to the
operand, so a brand-new `(playerID, key)` pair works correctly with no pre-creation step needed; `before == nil`
on the very first call is expected and handled (`prev` stays `0`).

- [ ] **Step 8: Commit**

```bash
git add api/internal/achievements api/internal/engine/hand/hand.go api/internal/engine/handeval/handeval.go
git commit -m "feat(achievements): data-driven star-tier catalog with DynamoDB-backed progress tracking"
```

---

### Task 4: Leaderboard aggregation

**Files:**
- Create: `api/internal/leaderboard/store.go`
- Create: `api/internal/leaderboard/service.go`
- Test: `api/internal/leaderboard/service_test.go`
- Create: `api/internal/api/v1/leaderboard.go`

**Interfaces:**
- Consumes: `hand.HandOutcome` (Task 3), `dynamo.Base` (existing).
- Produces: `func (s *Service) RecordHand(ctx, outcome hand.HandOutcome) error`, `func (s *Service) Top(ctx,
  metric string, limit int) ([]Entry, error)`, route `GET /v1.0/leaderboard?metric=win_rate|hands_played`.

Non-monetary only (OVERVIEW.md §9.1): `hands_played`, `hands_won`, `win_rate` (derived, not stored), and
`achievement_points` (sum of unlocked stars — wired once Task 3's unlocks are available, Task 6).

- [ ] **Step 1: Write the failing test**

```go
// api/internal/leaderboard/service_test.go
package leaderboard

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type memStore struct {
	rows map[string]*Entry
}

func newMemStore() *memStore { return &memStore{rows: map[string]*Entry{}} }

func (m *memStore) IncrementStats(_ context.Context, playerID string, handsPlayedDelta, handsWonDelta int) error {
	if m.rows[playerID] == nil {
		m.rows[playerID] = &Entry{PlayerID: playerID}
	}
	m.rows[playerID].HandsPlayed += handsPlayedDelta
	m.rows[playerID].HandsWon += handsWonDelta
	return nil
}

func (m *memStore) Top(_ context.Context, metric string, limit int) ([]Entry, error) {
	out := make([]Entry, 0, len(m.rows))
	for _, e := range m.rows {
		out = append(out, *e)
	}
	return out, nil
}

func TestRecordHandUpdatesHandsPlayedAndWon(t *testing.T) {
	store := newMemStore()
	svc := NewServiceWithStore(store)
	outcome := hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1", "p2"}}

	if err := svc.RecordHand(context.Background(), outcome); err != nil {
		t.Fatalf("record hand: %v", err)
	}
	if store.rows["p1"].HandsPlayed != 1 || store.rows["p1"].HandsWon != 1 {
		t.Fatalf("expected p1 hands_played=1 hands_won=1, got %+v", store.rows["p1"])
	}
	if store.rows["p2"].HandsPlayed != 1 || store.rows["p2"].HandsWon != 0 {
		t.Fatalf("expected p2 hands_played=1 hands_won=0, got %+v", store.rows["p2"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/leaderboard/... -v`
Expected: FAIL with "undefined: NewServiceWithStore".

- [ ] **Step 3: Implement `service.go`**

```go
// api/internal/leaderboard/service.go
// Package leaderboard aggregates non-monetary per-player stats (OVERVIEW.md
// §9.1) — no real-money amount is ever a field here, by design, so a future
// real-money phase can't accidentally expose one by adding a column.
package leaderboard

import (
	"context"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type Entry struct {
	PlayerID    string  `json:"player_id"`
	HandsPlayed int     `json:"hands_played"`
	HandsWon    int     `json:"hands_won"`
	WinRate     float64 `json:"win_rate"` // derived: HandsWon/HandsPlayed, computed at read time
}

type statsStore interface {
	IncrementStats(ctx context.Context, playerID string, handsPlayedDelta, handsWonDelta int) error
	Top(ctx context.Context, metric string, limit int) ([]Entry, error)
}

type Service struct {
	store statsStore
}

func NewServiceWithStore(store statsStore) *Service { return &Service{store: store} }

func (s *Service) RecordHand(ctx context.Context, outcome hand.HandOutcome) error {
	won := make(map[string]bool, len(outcome.Winners))
	for _, id := range outcome.Winners {
		won[id] = true
	}
	for _, id := range outcome.Participants {
		wonDelta := 0
		if won[id] {
			wonDelta = 1
		}
		if err := s.store.IncrementStats(ctx, id, 1, wonDelta); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Top(ctx context.Context, metric string, limit int) ([]Entry, error) {
	entries, err := s.store.Top(ctx, metric, limit)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].HandsPlayed > 0 {
			entries[i].WinRate = float64(entries[i].HandsWon) / float64(entries[i].HandsPlayed)
		}
	}
	return entries, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/leaderboard/... -v`
Expected: PASS.

- [ ] **Step 5: Implement the DynamoDB store and HTTP route**

```go
// api/internal/leaderboard/store.go
package leaderboard

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableStats = "poker_leaderboard_stats"
	statsSK    = "stats"
	gsiHandsWon = "gsi_hands_won"
)

type Store struct {
	base dynamo.Base
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableStats)}
}

func (s *Store) IncrementStats(ctx context.Context, playerID string, handsPlayedDelta, handsWonDelta int) error {
	sk := statsSK
	if handsPlayedDelta != 0 {
		if _, err := s.base.AtomicIncrement(ctx, playerID, &sk, "hands_played"); err != nil {
			return fmt.Errorf("leaderboard: increment hands_played: %w", err)
		}
	}
	if handsWonDelta != 0 {
		if _, err := s.base.AtomicIncrement(ctx, playerID, &sk, "hands_won"); err != nil {
			return fmt.Errorf("leaderboard: increment hands_won: %w", err)
		}
	}
	return nil
}

// Top returns the top `limit` players by hands_won (the only ranking wired
// for MVP — a GSI keyed purely on a numeric attribute doesn't exist in
// DynamoDB without a constant partition key, which is fine at poker's launch
// scale; a hot-partition concern to revisit only if leaderboard size grows
// far beyond what a single query page can return).
func (s *Store) Top(ctx context.Context, metric string, limit int) ([]Entry, error) {
	result, err := s.base.QueryGSI(ctx, gsiHandsWon, "gsi_hands_won_pk", "all", limit, nil)
	if err != nil {
		return nil, fmt.Errorf("leaderboard: query top: %w", err)
	}
	out := make([]Entry, 0, len(result.Items))
	for _, item := range result.Items {
		e, err := dynamo.Decode[Entry](item)
		if err != nil {
			return nil, fmt.Errorf("leaderboard: decode: %w", err)
		}
		out = append(out, *e)
	}
	return out, nil
}
```

- [ ] **Step 6: Add the HTTP route**

```go
// api/internal/api/v1/leaderboard.go
package v1

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
)

func RegisterLeaderboard(router fiber.Router, svc *leaderboard.Service) {
	router.Get("/leaderboard", func(c fiber.Ctx) error {
		limit := 50
		if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
			limit = l
		}
		entries, err := svc.Top(c.Context(), c.Query("metric", "hands_won"), limit)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load leaderboard"})
		}
		return c.JSON(entries)
	})
}
```

Mount `RegisterLeaderboard(router, leaderboardSvc)` in `router.go`'s `Register`, add the `leaderboard.Service`
Fx provider in `app.go`.

- [ ] **Step 7: Commit**

```bash
git add api/internal/leaderboard api/internal/api/v1/leaderboard.go
git commit -m "feat(leaderboard): non-monetary hands-played/hands-won aggregation and top-N endpoint"
```

---

### Task 5: Sandbox credit roulette

**Files:**
- Create: `api/internal/roulette/service.go`
- Create: `api/internal/roulette/store.go`
- Test: `api/internal/roulette/service_test.go`
- Create: `api/internal/api/v1/roulette.go`

**Interfaces:**
- Consumes: `walletclient.Client` (Phase 3).
- Produces: `func (s *Service) Spin(ctx, playerID string) (awardedAmount int64, err error)`, route
  `POST /v1.0/roulette/spin`.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/roulette/service_test.go
package roulette

import (
	"context"
	"errors"
	"testing"
)

type fakeCooldownStore struct {
	spun map[string]bool
}

func (f *fakeCooldownStore) TryClaimSpin(_ context.Context, playerID string) (bool, error) {
	if f.spun == nil {
		f.spun = map[string]bool{}
	}
	if f.spun[playerID] {
		return false, nil
	}
	f.spun[playerID] = true
	return true, nil
}

type fakeCredit struct{ credited int64 }

func (f *fakeCredit) Credit(_ context.Context, _ string, amount int64, _, _ string) error {
	f.credited = amount
	return nil
}

func TestSpinCreditsOneOfTheFixedTiers(t *testing.T) {
	cooldown := &fakeCooldownStore{}
	credit := &fakeCredit{}
	svc := NewService(credit, cooldown)

	amount, err := svc.Spin(context.Background(), "p1")
	if err != nil {
		t.Fatalf("spin: %v", err)
	}
	valid := map[int64]bool{100: true, 200: true, 500: true, 1000: true}
	if !valid[amount] {
		t.Fatalf("expected amount to be one of the fixed tiers, got %d", amount)
	}
	if credit.credited != amount {
		t.Fatalf("expected wallet credited exactly the awarded amount, got %d vs awarded %d", credit.credited, amount)
	}
}

func TestSpinRejectsSecondAttemptWithinCooldown(t *testing.T) {
	cooldown := &fakeCooldownStore{}
	credit := &fakeCredit{}
	svc := NewService(credit, cooldown)

	if _, err := svc.Spin(context.Background(), "p1"); err != nil {
		t.Fatalf("first spin: %v", err)
	}
	if _, err := svc.Spin(context.Background(), "p1"); !errors.Is(err, ErrAlreadySpunToday) {
		t.Fatalf("expected ErrAlreadySpunToday on second spin, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/roulette/... -v`
Expected: FAIL with "undefined: NewService".

- [ ] **Step 3: Implement `service.go`**

```go
// api/internal/roulette/service.go
// Package roulette implements OVERVIEW.md §9.3's free sandbox credit
// roulette: a fixed set of prize tiers, probability inversely proportional
// to value, CSPRNG selection, once per player per 24h. Sandbox-only — it
// only ever calls the sandbox credit path, never the real-money ledger.
package roulette

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
)

var ErrAlreadySpunToday = errors.New("roulette: already spun within the last 24h")

// tier pairs a prize amount with its selection weight — weights are
// inversely proportional to value (smaller prize, higher chance), matching
// OVERVIEW.md §9.3 exactly: 1000/500/200/100 in ascending prize order get
// descending weight.
type tier struct {
	amount int64
	weight int
}

var tiers = []tier{
	{amount: 100, weight: 50},
	{amount: 200, weight: 30},
	{amount: 500, weight: 15},
	{amount: 1000, weight: 5},
}

type credit interface {
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

type cooldownStore interface {
	// TryClaimSpin atomically checks-and-marks today's spin for playerID.
	// Returns false (no error) if a spin was already claimed in the current
	// cooldown window — this is the only correctness-critical operation in
	// this package, so it lives behind one atomic call rather than a
	// read-then-write the caller could race.
	TryClaimSpin(ctx context.Context, playerID string) (bool, error)
}

type Service struct {
	wallet   credit
	cooldown cooldownStore
}

func NewService(wallet credit, cooldown cooldownStore) *Service {
	return &Service{wallet: wallet, cooldown: cooldown}
}

func (s *Service) Spin(ctx context.Context, playerID string) (int64, error) {
	ok, err := s.cooldown.TryClaimSpin(ctx, playerID)
	if err != nil {
		return 0, fmt.Errorf("roulette: claim spin: %w", err)
	}
	if !ok {
		return 0, ErrAlreadySpunToday
	}

	amount, err := pickTier()
	if err != nil {
		return 0, fmt.Errorf("roulette: pick tier: %w", err)
	}
	idemKey := fmt.Sprintf("%s#roulette#%d", playerID, amount) // best-effort; see closing note on idempotency granularity
	if err := s.wallet.Credit(ctx, playerID, amount, idemKey, "sandbox_roulette"); err != nil {
		return 0, fmt.Errorf("roulette: credit: %w", err)
	}
	return amount, nil
}

// pickTier draws a CSPRNG-weighted selection — same fairness discipline as
// the deck shuffle (OVERVIEW.md §3.5/§9.3): not real money, but still
// auditable and non-manipulable.
func pickTier() (int64, error) {
	total := 0
	for _, t := range tiers {
		total += t.weight
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	roll := int(binary.BigEndian.Uint64(b[:]) % uint64(total))
	for _, t := range tiers {
		if roll < t.weight {
			return t.amount, nil
		}
		roll -= t.weight
	}
	return tiers[0].amount, nil // unreachable given the loop above sums to total
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/roulette/... -v`
Expected: PASS.

- [ ] **Step 5: Implement the DynamoDB cooldown store and HTTP route**

```go
// api/internal/roulette/store.go
package roulette

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const tableSpins = "poker_roulette_spins"

// cooldownKey resets at a fixed time daily (OVERVIEW.md §9.3: "e.g. midnight
// BRT") by using the UTC calendar date as part of the sort key — a player
// can claim exactly one spin per that key's lifetime.
func cooldownKey(now time.Time) string {
	return now.UTC().Format("2006-01-02")
}

type Store struct {
	base dynamo.Base
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableSpins)}
}

// TryClaimSpin uses a create-only conditional put (attribute_not_exists via
// UpdateItem's absence of the exists guard is the wrong primitive here —
// PutItem has no conditional variant on dynamo.Base directly, so this uses
// UpsertAttrs's underlying UpdateItem is also wrong for "only if absent".
// The correct primitive is a raw conditional PutItem, added as a one-off
// here since dynamo.Base has no exported conditional-create-only PutItem
// today (only the TransactWriteItem-based BuildPutTxItemIfAbsent, meant for
// composing multi-item transactions, not a single-item call) — call
// TransactWrite with exactly one item, which correctly enforces
// attribute_not_exists(pk) atomically.
func (s *Store) TryClaimSpin(ctx context.Context, playerID string) (bool, error) {
	item, err := dynamo.Encode(struct {
		PK       string `dynamodbav:"pk"`
		SK       string `dynamodbav:"sk"`
		SpunAt   string `dynamodbav:"spun_at"`
	}{PK: playerID, SK: cooldownKey(time.Now()), SpunAt: dynamo.NowStr()})
	if err != nil {
		return false, fmt.Errorf("roulette: encode: %w", err)
	}
	txItem := s.base.BuildPutTxItemIfAbsent(item)
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{txItem}); err != nil {
		if dynamo.IsConditionFailed(err) {
			return false, nil
		}
		return false, fmt.Errorf("roulette: claim: %w", err)
	}
	return true, nil
}
```

Add `"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"` import for `types.TransactWriteItem`.

- [ ] **Step 6: Add the HTTP route**

```go
// api/internal/api/v1/roulette.go
package v1

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/roulette"
)

func RegisterRoulette(router fiber.Router, auth fiber.Handler, svc *roulette.Service) {
	router.Post("/roulette/spin", auth, func(c fiber.Ctx) error {
		userID := c.Locals(localsUserID).(string)
		amount, err := svc.Spin(c.Context(), userID)
		if err != nil {
			if errors.Is(err, roulette.ErrAlreadySpunToday) {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "already spun today"})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "spin failed"})
		}
		return c.JSON(fiber.Map{"amount": amount})
	})
}
```

Mount `RegisterRoulette(router, authMiddleware(verifier), rouletteSvc)` in `router.go`, add the Fx provider.

- [ ] **Step 7: Commit**

```bash
git add api/internal/roulette api/internal/api/v1/roulette.go
git commit -m "feat(roulette): sandbox credit roulette with CSPRNG weighted tiers and a 24h cooldown"
```

---

### Task 6: Wire achievements/leaderboard into hand completion

**Files:**
- Modify: `api/internal/table/actor.go`
- Modify: `api/internal/tablemanager/manager.go`
- Test: `api/internal/table/gamification_test.go`

**Interfaces:**
- `table.Actor` gains an `onHandComplete func(hand.HandOutcome)` callback, invoked once per completed hand.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/table/gamification_test.go
package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestActorInvokesOnHandCompleteExactlyOncePerHand(t *testing.T) {
	p1 := &hand.Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &hand.Player{ID: "p2", Stack: 1000, Ready: true}
	ht := hand.NewTable([]*hand.Player{p1, p2}, 10, 20)
	calls := 0
	a := New("table-1", ht, nil, func(string, hand.Snapshot) {}, true)
	a.onHandComplete = func(hand.HandOutcome) { calls++ }
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)

	reply := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})

	for ht.Stage() != hand.Complete {
		var toAct string
		for _, s := range ht.ViewFor("p1").Seats {
			if s.State == "active" {
				toAct = s.PlayerID
				break
			}
		}
		r := make(chan error, 1)
		if err := a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "a" + toAct + string(rune(calls)), Action: betting.ActionCall, Reply: r}); err != nil {
			_ = a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "b" + toAct + string(rune(calls)), Action: betting.ActionCheck, Reply: r})
		}
	}
	time.Sleep(10 * time.Millisecond)
	if calls != 1 {
		t.Fatalf("expected onHandComplete invoked exactly once, got %d", calls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/table/... -run TestActorInvokesOnHandComplete -v`
Expected: FAIL — `a.onHandComplete` field doesn't exist.

- [ ] **Step 3: Wire the callback**

```go
// api/internal/table/actor.go — Actor struct, add
	onHandComplete func(hand.HandOutcome)
```

```go
// api/internal/table/actor.go — handleAct, right after `if a.table.Stage() == hand.Complete { a.persistSnapshot() }`
	if a.table.Stage() == hand.Complete {
		a.persistSnapshot()
		if a.onHandComplete != nil {
			if outcome := a.table.LastOutcomeForActor(); outcome != nil {
				a.onHandComplete(*outcome)
			}
		}
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/table/... -v`
Expected: PASS.

- [ ] **Step 5: Wire real services into the callback via `tablemanager`**

```go
// api/internal/tablemanager/manager.go — NewManager, add a parameter
func NewManager(leases *tablelease.Service, owners *tableowner.Registry, store *tablestore.Store, instanceAddr string, broadcast func(string, string, hand.Snapshot), onHandComplete func(tableID string, outcome hand.HandOutcome)) *Manager {
	return &Manager{leases: leases, owners: owners, store: store, instanceAddr: instanceAddr, broadcast: broadcast, onHandComplete: onHandComplete, actors: make(map[string]*Actor)}
}
```

```go
// api/internal/tablemanager/manager.go — Manager struct, add
	onHandComplete func(string, hand.HandOutcome)
```

```go
// api/internal/tablemanager/manager.go — Acquire, table.New call site
	actor := table.New(tableID, ht, m.store, m.broadcastFor(tableID), true)
	actor.SetOnHandCompleteForActor(func(outcome hand.HandOutcome) {
		if m.onHandComplete != nil {
			m.onHandComplete(tableID, outcome)
		}
	})
```

```go
// api/internal/table/actor.go — add
func (a *Actor) SetOnHandCompleteForActor(fn func(hand.HandOutcome)) { a.onHandComplete = fn }
```

Update every existing `NewManager(...)` call site (Phase 2/3 tests, `app.go`) to pass a trailing `nil` (or a real
callback in `app.go`'s production wiring below) — another source-breaking signature change applied everywhere.

```go
// api/internal/app/app.go — newTableManager, add achievements/leaderboard services and wire the callback
func newTableManager(leases *tablelease.Service, owners *tableowner.Registry, reg ws.Registry, cfg *config.Config, achv *achievements.Service, lb *leaderboard.Service) *tablemanager.Manager {
	broadcast := func(tableID, viewerID string, snap hand.Snapshot) {
		data, _ := json.Marshal(map[string]any{"type": "state", "snapshot": snap})
		reg.Broadcast(context.Background(), tableID+"#"+viewerID, data)
	}
	onHandComplete := func(tableID string, outcome hand.HandOutcome) {
		ctx := context.Background()
		unlocks, err := achv.RecordHand(ctx, tableID, outcome)
		if err != nil {
			slog.Error("achievements record hand failed", "table", tableID, "err", err)
		}
		for _, u := range unlocks {
			data, _ := json.Marshal(map[string]any{"type": "achievement_unlocked", "key": u.Key, "stars": u.Stars})
			reg.Broadcast(ctx, tableID+"#"+u.PlayerID, data)
		}
		if err := lb.RecordHand(ctx, outcome); err != nil {
			slog.Error("leaderboard record hand failed", "table", tableID, "err", err)
		}
	}
	return tablemanager.NewManager(leases, owners, nil, cfg.InstancePrivateIP+":"+strconv.Itoa(cfg.Port), broadcast, onHandComplete)
}
```

- [ ] **Step 6: Run the full suite**

Run: `go build ./... && go test ./... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add api/internal/table api/internal/tablemanager api/internal/app/app.go
git commit -m "feat(table): fire achievements and leaderboard updates on every completed hand"
```

---

### Task 7: CDK — gamification tables

**Files:**
- Modify: `cdk/lib/dynamodb-stack.ts`
- Modify: `cdk/lib/api-stack.ts`
- Test: `cdk/test/dynamodb-stack.test.ts`

**Interfaces:**
- Extends `DynamoDBStack` with `poker_achievement_progress`, `poker_leaderboard_stats` (+ `gsi_hands_won`),
  `poker_roulette_spins`.

- [ ] **Step 1: Extend the test**

```typescript
// cdk/test/dynamodb-stack.test.ts — add
test('creates gamification tables', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestDynamoDBStack3', {environment: 'dev'});
  const template = Template.fromStack(stack);
  template.resourceCountIs('AWS::DynamoDB::Table', 6);
  for (const name of ['poker_achievement_progress', 'poker_leaderboard_stats', 'poker_roulette_spins']) {
    template.hasResourceProperties('AWS::DynamoDB::Table', {TableName: `dev_${name}`});
  }
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd cdk && npx jest dynamodb-stack.test.ts`
Expected: FAIL — resource count is 3, not 6.

- [ ] **Step 3: Add the three tables**

```typescript
// cdk/lib/dynamodb-stack.ts — TableName, extend
export type TableName =
  | 'poker_hand_snapshots' | 'poker_action_log' | 'poker_rooms'
  | 'poker_achievement_progress' | 'poker_leaderboard_stats' | 'poker_roulette_spins';
```

```typescript
// cdk/lib/dynamodb-stack.ts — constructor, add
    table('poker_achievement_progress');
    const stats = table('poker_leaderboard_stats');
    stats.addGlobalSecondaryIndex({
      indexName: 'gsi_hands_won',
      partitionKey: {name: 'gsi_hands_won_pk', type: dynamodb.AttributeType.STRING},
      sortKey: {name: 'hands_won', type: dynamodb.AttributeType.NUMBER},
      projectionType: dynamodb.ProjectionType.ALL,
    });
    table('poker_roulette_spins');
```

`Store.IncrementStats` (Task 4) never writes a `gsi_hands_won_pk` attribute today — add it there so the GSI this
task provisions is actually populated:

```go
// api/internal/leaderboard/store.go — IncrementStats, after both AtomicIncrement calls
	if handsWonDelta != 0 || handsPlayedDelta != 0 {
		_ = s.base.UpsertAttrs(ctx, playerID, &sk, map[string]any{"gsi_hands_won_pk": "all"})
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd cdk && npx jest dynamodb-stack.test.ts`
Expected: PASS.

- [ ] **Step 5: Grant table access on the API instance role**

```typescript
// cdk/lib/api-stack.ts — extend the existing dynamodb PolicyStatement's resources array
      resources: [
        handSnapshotsTableArn, actionLogTableArn, roomsTableArn,
        achievementProgressTableArn, leaderboardStatsTableArn, rouletteSpinsTableArn,
      ],
```

Add the three corresponding `ApiStackProps` fields and thread them from `bin/poker.ts`, matching Phase 2 Task 11
and Phase 3 Task 10's exact pattern.

- [ ] **Step 6: Commit**

```bash
git add cdk/lib/dynamodb-stack.ts cdk/lib/api-stack.ts cdk/bin/poker.ts api/internal/leaderboard/store.go
git commit -m "feat(cdk): provision achievement/leaderboard/roulette tables"
```

---

### Task 8: Frontend scaffold

**Files:**
- Create: `ui/package.json`, `ui/next.config.ts`, `ui/tsconfig.json`, `ui/tailwind.config` (via `postcss.config.mjs`
  + ShadCN defaults, matching `ctech-wallet/ui`), `ui/src/app/layout.tsx`, `ui/src/lib/auth/oauth.ts`,
  `ui/src/lib/api/client.ts`, `ui/src/app/callback/page.tsx`

**Interfaces:**
- Produces the base app shell every later frontend task builds on: authenticated API client, OAuth callback
  handling, root layout.

This task is a direct structural mirror of `ctech-wallet/ui`'s equivalent files — copied and re-scoped (poker's
own client ID, poker's own API base path), not redesigned, since the pattern is already proven in production.

- [ ] **Step 1: `package.json`**

```json
{
  "name": "ctech-poker-ui",
  "version": "0.1.0",
  "private": true,
  "scripts": {
    "dev": "next dev --port 3010",
    "build": "next build",
    "start": "next start",
    "lint": "eslint"
  },
  "dependencies": {
    "@aoctech/auth-client": "^1.1.0",
    "@aoctech/ws-client": "^1.0.0",
    "@tanstack/react-query": "^5.100.14",
    "axios": "^1.16.1",
    "clsx": "^2.1.1",
    "lucide-react": "^1.17.0",
    "next": "16.2.6",
    "react": "19.2.4",
    "react-dom": "19.2.4",
    "tailwind-merge": "^3.6.0",
    "tw-animate-css": "^1.4.0"
  },
  "devDependencies": {
    "@tailwindcss/postcss": "^4",
    "@types/node": "^20",
    "@types/react": "^19",
    "@types/react-dom": "^19",
    "eslint": "^9",
    "eslint-config-next": "16.2.6",
    "tailwindcss": "^4",
    "typescript": "^5"
  }
}
```

- [ ] **Step 2: `next.config.ts`** (identical structure to `ctech-wallet/ui/next.config.ts`, poker's own dev port)

```typescript
import type {NextConfig} from "next";
import path from "path";

const isProduction = process.env.NODE_ENV === 'production';
const DEV_API_ORIGIN = process.env.DEV_API_ORIGIN || 'http://localhost:8003';

const nextConfig: NextConfig = {
  turbopack: {root: path.join(__dirname)},
  allowedDevOrigins: ['127.0.0.1'],
  ...(isProduction
    ? {output: 'export' as const}
    : {
      async rewrites() {
        return [{source: '/v1.0/:path*', destination: `${DEV_API_ORIGIN}/v1.0/:path*`}];
      },
    }),
};

export default nextConfig;
```

- [ ] **Step 3: Auth wrapper**

```typescript
// ui/src/lib/auth/oauth.ts
import {OAuthClient, decodeIdToken as sdkDecodeIdToken} from '@aoctech/auth-client'
import type {UnverifiedIdTokenClaims} from '@aoctech/auth-client'

const CTECH_URL = process.env.NEXT_PUBLIC_CTECH_URL!
const CLIENT_ID = process.env.NEXT_PUBLIC_CTECH_CLIENT_ID!

const client = new OAuthClient({
  baseUrl: CTECH_URL,
  clientId: CLIENT_ID,
  redirectUri: typeof window !== 'undefined' ? `${window.location.origin}/callback` : '',
  scope: 'openid profile',
})

export type {UnverifiedIdTokenClaims}
export const decodeIdToken = sdkDecodeIdToken

export async function startOAuthFlow(returnTo = '/'): Promise<void> {
  await client.startOAuthFlow(returnTo)
}

export async function exchangeCode(code: string, state: string) {
  const result = await client.exchangeCode(code, state)
  return {accessToken: result.accessToken, idToken: result.idToken ?? null, returnTo: result.returnTo}
}

export async function doRefresh(): Promise<{accessToken: string} | null> {
  const result = await client.refresh()
  return result ? {accessToken: result.accessToken} : null
}
```

- [ ] **Step 4: API client**

```typescript
// ui/src/lib/api/client.ts
import axios from 'axios'
import {doRefresh} from '@/lib/auth/oauth'

let accessToken: string | null = null
const listeners = new Set<(token: string | null) => void>()

export function setAccessToken(token: string | null) {
  accessToken = token
  listeners.forEach((fn) => fn(token))
}
export function getAccessToken() { return accessToken }
export function subscribeAccessToken(fn: (token: string | null) => void) {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

export const apiClient = axios.create({baseURL: process.env.NEXT_PUBLIC_API_URL || ''})

apiClient.interceptors.request.use((config) => {
  if (accessToken) config.headers.Authorization = `Bearer ${accessToken}`
  return config
})

apiClient.interceptors.response.use(
  (res) => res,
  async (error) => {
    if (error.response?.status === 401 && !error.config._retried) {
      error.config._retried = true
      const refreshed = await doRefresh()
      if (refreshed) {
        setAccessToken(refreshed.accessToken)
        error.config.headers.Authorization = `Bearer ${refreshed.accessToken}`
        return apiClient.request(error.config)
      }
    }
    return Promise.reject(error)
  },
)
```

- [ ] **Step 5: Root layout + OAuth callback page**

```tsx
// ui/src/app/layout.tsx
import type {Metadata} from 'next'
import './globals.css'

export const metadata: Metadata = {title: 'CTech Poker', description: 'Texas Hold\'em sandbox poker'}

export default function RootLayout({children}: {children: React.ReactNode}) {
  return (
    <html lang="pt-BR">
      <body>{children}</body>
    </html>
  )
}
```

```tsx
// ui/src/app/callback/page.tsx
'use client'
import {useEffect} from 'react'
import {useRouter, useSearchParams} from 'next/navigation'
import {exchangeCode} from '@/lib/auth/oauth'
import {setAccessToken} from '@/lib/api/client'

export default function CallbackPage() {
  const router = useRouter()
  const params = useSearchParams()

  useEffect(() => {
    const code = params.get('code')
    const state = params.get('state')
    if (!code || !state) {
      router.replace('/')
      return
    }
    exchangeCode(code, state).then(({accessToken, returnTo}) => {
      setAccessToken(accessToken)
      router.replace(returnTo || '/lobby')
    })
  }, [params, router])

  return <p>Autenticando…</p>
}
```

Add a minimal `ui/src/app/globals.css` importing Tailwind (`@import "tailwindcss";`), matching
`ctech-wallet/ui`'s own setup.

- [ ] **Step 6: Manual verification**

Run: `cd ui && npm install && npm run build`
Expected: builds without error (no automated test for scaffold wiring — there's no behavior yet to assert beyond
"it compiles", covered by the build itself).

- [ ] **Step 7: Commit**

```bash
git add ui/package.json ui/next.config.ts ui/src/app/layout.tsx ui/src/app/callback/page.tsx ui/src/app/globals.css ui/src/lib
git commit -m "feat(ui): Next.js SPA scaffold with OAuth callback and authenticated API client"
```

---

### Task 9: Lobby page

**Files:**
- Create: `ui/src/app/lobby/page.tsx`
- Create: `ui/src/components/lobby/RoomList.tsx`
- Create: `ui/src/components/lobby/CreateRoomDialog.tsx`
- Create: `ui/src/lib/api/rooms.ts`

**Interfaces:**
- Consumes: `GET /v1.0/rooms`, `POST /v1.0/rooms` (Phase 3 Task 4).

- [ ] **Step 1: API bindings**

```typescript
// ui/src/lib/api/rooms.ts
import {apiClient} from './client'

export interface Room {
  room_id: string
  visibility: 'public' | 'private'
  small_blind: number
  big_blind: number
  max_seats: number
  buy_in_min: number
  buy_in_max: number
  equity_display_enabled: boolean
  status: 'waiting' | 'active'
}

export interface CreateRoomInput {
  visibility: 'public' | 'private'
  small_blind: number
  big_blind: number
  max_seats: number
  buy_in_min: number
  buy_in_max: number
  equity_display_enabled?: boolean
  blind_escalation?: {interval_minutes: number; multiplier: number; max: number}
}

export async function listPublicRooms(): Promise<Room[]> {
  const {data} = await apiClient.get<Room[]>('/v1.0/rooms')
  return data
}

export async function createRoom(input: CreateRoomInput): Promise<Room> {
  const {data} = await apiClient.post<Room>('/v1.0/rooms', input)
  return data
}
```

- [ ] **Step 2: Room list component**

```tsx
// ui/src/components/lobby/RoomList.tsx
'use client'
import {useQuery} from '@tanstack/react-query'
import {useRouter} from 'next/navigation'
import {listPublicRooms} from '@/lib/api/rooms'

export function RoomList() {
  const router = useRouter()
  const {data: rooms, isLoading} = useQuery({queryKey: ['rooms'], queryFn: listPublicRooms, refetchInterval: 5000})

  if (isLoading) return <p>Carregando mesas…</p>
  if (!rooms?.length) return <p>Nenhuma mesa pública aberta agora.</p>

  return (
    <table className="w-full text-left">
      <thead>
        <tr><th>Blinds</th><th>Jogadores</th><th>Buy-in</th><th /></tr>
      </thead>
      <tbody>
        {rooms.map((r) => (
          <tr key={r.room_id} className="border-t border-white/10">
            <td>{r.small_blind}/{r.big_blind}</td>
            <td>{r.max_seats} lugares</td>
            <td>{r.buy_in_min}–{r.buy_in_max}</td>
            <td>
              <button
                className="rounded bg-emerald-600 px-3 py-1 text-white"
                onClick={() => router.push(`/table/${r.room_id}`)}
              >
                Entrar
              </button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
```

- [ ] **Step 3: Create-room dialog**

```tsx
// ui/src/components/lobby/CreateRoomDialog.tsx
'use client'
import {useState} from 'react'
import {useRouter} from 'next/navigation'
import {createRoom, type CreateRoomInput} from '@/lib/api/rooms'

const PUBLIC_STAKES = [
  {small: 10, big: 20}, {small: 25, big: 50}, {small: 50, big: 100}, {small: 100, big: 200},
]

export function CreateRoomDialog() {
  const [open, setOpen] = useState(false)
  const [visibility, setVisibility] = useState<'public' | 'private'>('public')
  const [stakeIdx, setStakeIdx] = useState(0)
  const [maxSeats, setMaxSeats] = useState(9)
  const [escalation, setEscalation] = useState(false)
  const router = useRouter()

  async function handleCreate() {
    const stake = PUBLIC_STAKES[stakeIdx]
    const input: CreateRoomInput = {
      visibility, small_blind: stake.small, big_blind: stake.big, max_seats: maxSeats,
      buy_in_min: stake.big * 20, buy_in_max: stake.big * 100,
    }
    if (visibility === 'private' && escalation) {
      input.blind_escalation = {interval_minutes: 15, multiplier: 150, max: stake.big * 20}
    }
    const room = await createRoom(input)
    router.push(`/table/${room.room_id}`)
  }

  if (!open) {
    return <button className="rounded bg-emerald-600 px-4 py-2 text-white" onClick={() => setOpen(true)}>Criar mesa</button>
  }

  return (
    <div className="rounded border border-white/10 p-4">
      <label className="block">
        Visibilidade
        <select value={visibility} onChange={(e) => setVisibility(e.target.value as 'public' | 'private')}>
          <option value="public">Pública</option>
          <option value="private">Privada</option>
        </select>
      </label>
      <label className="block">
        Stakes
        <select value={stakeIdx} onChange={(e) => setStakeIdx(Number(e.target.value))}>
          {PUBLIC_STAKES.map((s, i) => <option key={i} value={i}>{s.small}/{s.big}</option>)}
        </select>
      </label>
      <label className="block">
        Lugares (2-9)
        <input type="number" min={2} max={9} value={maxSeats} onChange={(e) => setMaxSeats(Number(e.target.value))} />
      </label>
      {visibility === 'private' && (
        <label className="block">
          <input type="checkbox" checked={escalation} onChange={(e) => setEscalation(e.target.checked)} />
          Escalonar blinds automaticamente
        </label>
      )}
      <button className="mt-2 rounded bg-emerald-600 px-4 py-2 text-white" onClick={handleCreate}>Confirmar</button>
    </div>
  )
}
```

- [ ] **Step 4: Lobby page**

```tsx
// ui/src/app/lobby/page.tsx
import {RoomList} from '@/components/lobby/RoomList'
import {CreateRoomDialog} from '@/components/lobby/CreateRoomDialog'

export default function LobbyPage() {
  return (
    <main className="mx-auto max-w-3xl p-6">
      <h1 className="mb-4 text-2xl font-bold">Mesas</h1>
      <CreateRoomDialog />
      <div className="mt-6"><RoomList /></div>
    </main>
  )
}
```

- [ ] **Step 5: Manual verification**

Run: `cd ui && npm run dev` (with the Go API running locally on :8003), open `/lobby`, confirm the room list
loads and a created room navigates to `/table/:id`.

- [ ] **Step 6: Commit**

```bash
git add ui/src/app/lobby ui/src/components/lobby ui/src/lib/api/rooms.ts
git commit -m "feat(ui): lobby page with public room listing and room creation"
```

---

### Task 10: Real-time table state hook

**Files:**
- Create: `ui/src/lib/hooks/useTableRealtime.ts`
- Create: `ui/src/lib/api/table.ts`

**Interfaces:**
- Consumes: `@aoctech/ws-client`'s `useWebSocket` (existing shared package, same one `ctech-wallet/ui` already
  uses), `GET /v1.0/tables/:id/ws` (Phase 2 Task 7).

- [ ] **Step 1: Wire types + the hook**

```typescript
// ui/src/lib/api/table.ts
export interface SeatView {
  player_id: string
  stack: number
  state: string
  contributed: number
  hole_cards?: string[]
  equity?: number
}

export interface TableSnapshot {
  stage: string
  board: string[]
  seats: SeatView[]
  payouts?: Record<string, number>
}

export type ServerMessage =
  | {type: 'connected'; conn_id: string}
  | {type: 'state'; snapshot: TableSnapshot}
  | {type: 'error'; code: string; message?: string}
  | {type: 'achievement_unlocked'; key: string; stars: number}
  | {type: 'pong'}
```

```typescript
// ui/src/lib/hooks/useTableRealtime.ts
'use client'
import {useCallback, useState} from 'react'
import {useWebSocket} from '@aoctech/ws-client'
import {getAccessToken, subscribeAccessToken} from '@/lib/api/client'
import type {ServerMessage, TableSnapshot} from '@/lib/api/table'

const WS_BASE_URL = process.env.NEXT_PUBLIC_API_URL || ''

function buildWsUrl(tableId: string): string {
  const origin = WS_BASE_URL || window.location.origin
  const base = origin.replace(/^http/, 'ws')
  return `${base}/v1.0/tables/${tableId}/ws`
}

export function useTableRealtime(tableId: string) {
  const token = getAccessToken()
  const wsUrl = token ? buildWsUrl(tableId) : null
  const [snapshot, setSnapshot] = useState<TableSnapshot | null>(null)
  const [unlocked, setUnlocked] = useState<{key: string; stars: number} | null>(null)

  const handleMessage = useCallback((data: unknown) => {
    const msg = data as ServerMessage
    if (msg.type === 'state') setSnapshot(msg.snapshot)
    if (msg.type === 'achievement_unlocked') setUnlocked({key: msg.key, stars: msg.stars})
  }, [])

  const {status, send} = useWebSocket({
    url: wsUrl,
    onMessage: handleMessage,
    enabled: !!wsUrl,
    authToken: token ?? undefined,
    subscribeToken: subscribeAccessToken,
  })

  const sendReady = useCallback((ready: boolean) => send(JSON.stringify({type: 'ready', ready})), [send])
  const sendAct = useCallback(
    (action: string, amount: number, actionId: string) =>
      send(JSON.stringify({type: 'act', action, amount, action_id: actionId})),
    [send],
  )
  const sendPostBigBlind = useCallback(() => send(JSON.stringify({type: 'post_big_blind'})), [send])

  return {status, snapshot, unlocked, sendReady, sendAct, sendPostBigBlind}
}
```

- [ ] **Step 2: Manual verification**

Run: `cd ui && npm run build` (compiles — no server-dependent test at this layer; end-to-end behavior is verified
once Task 11 renders against it).

- [ ] **Step 3: Commit**

```bash
git add ui/src/lib/hooks/useTableRealtime.ts ui/src/lib/api/table.ts
git commit -m "feat(ui): real-time table state hook over the table WebSocket gateway"
```

---

### Task 11: Table page — seats, cards, action bar

**Files:**
- Create: `ui/src/app/table/[id]/page.tsx`
- Create: `ui/src/components/table/Seat.tsx`
- Create: `ui/src/components/table/Board.tsx`
- Create: `ui/src/components/table/ActionBar.tsx`
- Create: `ui/src/lib/cards.ts`

**Interfaces:**
- Consumes: `useTableRealtime` (Task 10), the SVGs already committed at `ui/svgs/`.

- [ ] **Step 1: Card-code-to-SVG mapping**

```typescript
// ui/src/lib/cards.ts
const RANK_NAMES: Record<string, string> = {
  '2': '2', '3': '3', '4': '4', '5': '5', '6': '6', '7': '7', '8': '8', '9': '9', T: '10',
  J: 'jack', Q: 'queen', K: 'king', A: 'ace',
}
const SUIT_NAMES: Record<string, string> = {c: 'club', d: 'diamond', h: 'heart', s: 'spade'}

// cardSvgPath converts a wire card code ("As", "Td") to its SVG path under
// /svgs — the naming convention already committed there (e.g. club-ace.svg,
// club-10.svg) drives this mapping directly, no lookup table duplication.
export function cardSvgPath(code: string): string {
  const rank = RANK_NAMES[code[0]]
  const suit = SUIT_NAMES[code[1]]
  return `/svgs/${suit}-${rank}.svg`
}

export const cardBackPath = '/svgs/card-back-red.svg'
```

- [ ] **Step 2: Seat component**

```tsx
// ui/src/components/table/Seat.tsx
import Image from 'next/image'
import {cardSvgPath, cardBackPath} from '@/lib/cards'
import type {SeatView} from '@/lib/api/table'

export function Seat({seat, isViewer}: {seat: SeatView; isViewer: boolean}) {
  const showFaceUp = isViewer || seat.hole_cards?.length === 2
  return (
    <div className="flex flex-col items-center gap-1 rounded bg-black/30 p-2 text-white">
      <div className="flex gap-1">
        {[0, 1].map((i) => (
          <Image
            key={i}
            src={showFaceUp && seat.hole_cards ? cardSvgPath(seat.hole_cards[i]) : cardBackPath}
            alt="card"
            width={40}
            height={56}
            className="transition-transform duration-300 ease-out"
          />
        ))}
      </div>
      <span className="text-xs">{seat.player_id}</span>
      <span className="text-sm font-bold">{seat.stack}</span>
      {seat.equity != null && <span className="text-xs text-emerald-400">{Math.round(seat.equity * 100)}%</span>}
    </div>
  )
}
```

- [ ] **Step 3: Board component**

```tsx
// ui/src/components/table/Board.tsx
import Image from 'next/image'
import {cardSvgPath} from '@/lib/cards'

export function Board({cards}: {cards: string[]}) {
  return (
    <div className="flex gap-2">
      {cards.map((code, i) => (
        <Image
          key={i}
          src={cardSvgPath(code)}
          alt="board card"
          width={52}
          height={72}
          className="animate-[flip_0.4s_ease-out]"
        />
      ))}
    </div>
  )
}
```

Add the `flip` keyframe to `globals.css`:

```css
@keyframes flip {
  from { transform: rotateY(90deg); opacity: 0; }
  to { transform: rotateY(0deg); opacity: 1; }
}
```

- [ ] **Step 4: Action bar**

```tsx
// ui/src/components/table/ActionBar.tsx
'use client'
import {useState} from 'react'

export function ActionBar({
  bigBlind,
  onAct,
}: {
  bigBlind: number
  onAct: (action: string, amount: number) => void
}) {
  const [raiseTo, setRaiseTo] = useState(bigBlind * 2)
  return (
    <div className="flex items-center gap-2">
      <button className="rounded bg-red-600 px-3 py-2 text-white" onClick={() => onAct('fold', 0)}>Fold</button>
      <button className="rounded bg-gray-600 px-3 py-2 text-white" onClick={() => onAct('check', 0)}>Check</button>
      <button className="rounded bg-blue-600 px-3 py-2 text-white" onClick={() => onAct('call', 0)}>Call</button>
      <input
        type="range"
        min={bigBlind}
        step={bigBlind}
        value={raiseTo}
        onChange={(e) => setRaiseTo(Number(e.target.value))}
      />
      <span className="text-white">{raiseTo}</span>
      <button className="rounded bg-emerald-600 px-3 py-2 text-white" onClick={() => onAct('raise', raiseTo)}>Raise</button>
    </div>
  )
}
```

- [ ] **Step 5: Table page**

```tsx
// ui/src/app/table/[id]/page.tsx
'use client'
import {useParams} from 'next/navigation'
import {useTableRealtime} from '@/lib/hooks/useTableRealtime'
import {Seat} from '@/components/table/Seat'
import {Board} from '@/components/table/Board'
import {ActionBar} from '@/components/table/ActionBar'
import {decodeIdToken} from '@/lib/auth/oauth'
import {getAccessToken} from '@/lib/api/client'

export default function TablePage() {
  const {id} = useParams<{id: string}>()
  const {snapshot, sendReady, sendAct} = useTableRealtime(id)

  const token = getAccessToken()
  const viewerId = token ? decodeIdToken(token)?.sub : undefined

  if (!snapshot) return <p className="p-6 text-white">Conectando à mesa…</p>

  let actionId = 0
  const act = (action: string, amount: number) => sendAct(action, amount, `${viewerId}-${Date.now()}-${actionId++}`)

  return (
    <main className="flex min-h-screen flex-col items-center gap-6 bg-green-900 p-6">
      <Board cards={snapshot.board} />
      <div className="flex flex-wrap justify-center gap-4">
        {snapshot.seats.map((seat) => (
          <Seat key={seat.player_id} seat={seat} isViewer={seat.player_id === viewerId} />
        ))}
      </div>
      <button className="rounded bg-yellow-600 px-4 py-2" onClick={() => sendReady(true)}>Pronto</button>
      <ActionBar bigBlind={20} onAct={act} />
    </main>
  )
}
```

`ActionBar`'s hardcoded `bigBlind={20}` is a known gap — the snapshot doesn't currently carry the room's blind
amounts (only `hand.Snapshot`'s existing fields do, and blinds aren't one of them). Flagged rather than silently
wrong: a follow-up would add `SmallBlind`/`BigBlind` to `hand.Snapshot` (Phase 2's `snapshot.go`) the same way
`Stage`/`Board` are already there — small, but out of this task's scope since it touches Phase 2 code again.

- [ ] **Step 6: Manual verification**

Run: `cd ui && npm run dev`, open two browser tabs against `/table/:id` for a room both accounts joined via the
lobby, confirm cards render from the SVGs, actions transmit, and the board animates in on each new street.

- [ ] **Step 7: Commit**

```bash
git add ui/src/app/table ui/src/components/table ui/src/lib/cards.ts ui/src/app/globals.css
git commit -m "feat(ui): table page with seats, board, and action bar wired to the WebSocket gateway"
```

---

### Task 12: Achievements toast, leaderboard screen, roulette wheel

**Files:**
- Create: `ui/src/components/AchievementToast.tsx`
- Create: `ui/src/app/leaderboard/page.tsx`
- Create: `ui/src/app/roulette/page.tsx`
- Create: `ui/src/lib/api/gamification.ts`

- [ ] **Step 1: API bindings**

```typescript
// ui/src/lib/api/gamification.ts
import {apiClient} from './client'

export interface LeaderboardEntry {
  player_id: string
  hands_played: number
  hands_won: number
  win_rate: number
}

export async function fetchLeaderboard(metric = 'hands_won'): Promise<LeaderboardEntry[]> {
  const {data} = await apiClient.get<LeaderboardEntry[]>('/v1.0/leaderboard', {params: {metric}})
  return data
}

export async function spinRoulette(): Promise<{amount: number}> {
  const {data} = await apiClient.post<{amount: number}>('/v1.0/roulette/spin')
  return data
}
```

- [ ] **Step 2: Achievement toast**

```tsx
// ui/src/components/AchievementToast.tsx
'use client'
import {useEffect, useState} from 'react'

export function AchievementToast({unlock}: {unlock: {key: string; stars: number} | null}) {
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    if (!unlock) return
    setVisible(true)
    const t = setTimeout(() => setVisible(false), 4000)
    return () => clearTimeout(t)
  }, [unlock])

  if (!unlock || !visible) return null
  return (
    <div className="fixed bottom-4 right-4 animate-[slide-in_0.3s_ease-out] rounded bg-yellow-500 px-4 py-2 text-black shadow-lg">
      Conquista desbloqueada: {unlock.key} — {'★'.repeat(unlock.stars)}
    </div>
  )
}
```

```css
/* ui/src/app/globals.css — add */
@keyframes slide-in {
  from { transform: translateY(20px); opacity: 0; }
  to { transform: translateY(0); opacity: 1; }
}
```

Wire it into `table/[id]/page.tsx`: `<AchievementToast unlock={unlocked} />` alongside the existing `<Board>`.

- [ ] **Step 3: Leaderboard screen**

```tsx
// ui/src/app/leaderboard/page.tsx
'use client'
import {useQuery} from '@tanstack/react-query'
import {fetchLeaderboard} from '@/lib/api/gamification'

export default function LeaderboardPage() {
  const {data} = useQuery({queryKey: ['leaderboard'], queryFn: () => fetchLeaderboard()})
  return (
    <main className="mx-auto max-w-2xl p-6">
      <h1 className="mb-4 text-2xl font-bold">Ranking</h1>
      <table className="w-full text-left">
        <thead><tr><th>Jogador</th><th>Mãos</th><th>Vitórias</th><th>% Vitória</th></tr></thead>
        <tbody>
          {data?.map((e) => (
            <tr key={e.player_id} className="border-t border-white/10">
              <td>{e.player_id}</td>
              <td>{e.hands_played}</td>
              <td>{e.hands_won}</td>
              <td>{(e.win_rate * 100).toFixed(1)}%</td>
            </tr>
          ))}
        </tbody>
      </table>
    </main>
  )
}
```

- [ ] **Step 4: Roulette wheel**

```tsx
// ui/src/app/roulette/page.tsx
'use client'
import {useState} from 'react'
import {spinRoulette} from '@/lib/api/gamification'

export default function RoulettePage() {
  const [result, setResult] = useState<number | null>(null)
  const [spinning, setSpinning] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleSpin() {
    setSpinning(true)
    setError(null)
    try {
      const {amount} = await spinRoulette()
      setTimeout(() => {
        setResult(amount)
        setSpinning(false)
      }, 1200) // matches the CSS spin duration below, so the number reveals as the wheel settles
    } catch {
      setError('Você já girou hoje. Volte amanhã!')
      setSpinning(false)
    }
  }

  return (
    <main className="mx-auto flex max-w-md flex-col items-center gap-4 p-6">
      <h1 className="text-2xl font-bold">Roleta Sandbox</h1>
      <div className={spinning ? 'animate-[spin_1.2s_ease-out]' : ''}>🎡</div>
      {result != null && <p className="text-xl">Você ganhou {result} fichas sandbox!</p>}
      {error && <p className="text-red-400">{error}</p>}
      <button className="rounded bg-emerald-600 px-4 py-2 text-white" disabled={spinning} onClick={handleSpin}>
        Girar
      </button>
    </main>
  )
}
```

- [ ] **Step 5: Manual verification**

Run: `cd ui && npm run dev`, visit `/leaderboard` and `/roulette`, confirm data loads and a second spin same-day
shows the cooldown error.

- [ ] **Step 6: Commit**

```bash
git add ui/src/components/AchievementToast.tsx ui/src/app/leaderboard ui/src/app/roulette ui/src/lib/api/gamification.ts ui/src/app/globals.css
git commit -m "feat(ui): achievement toast, leaderboard screen, sandbox roulette wheel"
```

---

### Task 13: Basic chat with moderation

**Files:**
- Modify: `api/internal/api/v1/tablews.go`
- Modify: `api/internal/api/v1/tableproxy.go`
- Create: `api/internal/chatfilter/filter.go`
- Test: `api/internal/chatfilter/filter_test.go`
- Create: `ui/src/components/table/Chat.tsx`

**Interfaces:**
- Extends `clientMessage` with a `"chat"` type; server relays via `ws.Registry.Broadcast` on the table's key,
  after a basic profanity filter (OVERVIEW.md §8.4).

- [ ] **Step 1: Write the failing filter test**

```go
// api/internal/chatfilter/filter_test.go
package chatfilter

import "testing"

func TestFilterMasksKnownWords(t *testing.T) {
	f := New([]string{"idiota"})
	got := f.Clean("você é um idiota mesmo")
	if got == "você é um idiota mesmo" {
		t.Fatal("expected the flagged word to be masked")
	}
}

func TestFilterLeavesCleanMessagesUntouched(t *testing.T) {
	f := New([]string{"idiota"})
	got := f.Clean("boa mão!")
	if got != "boa mão!" {
		t.Fatalf("expected unchanged, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/chatfilter/... -v`
Expected: FAIL with "undefined: New".

- [ ] **Step 3: Implement the filter**

```go
// api/internal/chatfilter/filter.go
// Package chatfilter implements a basic profanity mask (OVERVIEW.md §8.4) —
// a fixed word list, case-insensitive substring match. Not a moderation
// system (no ML, no context awareness) — deliberately simple, matching the
// brief's own framing ("basic profanity filter + report/mute").
package chatfilter

import "strings"

type Filter struct {
	words []string
}

func New(bannedWords []string) *Filter {
	lower := make([]string, len(bannedWords))
	for i, w := range bannedWords {
		lower[i] = strings.ToLower(w)
	}
	return &Filter{words: lower}
}

// Clean masks every occurrence of a banned word with asterisks of the same
// length, preserving message length so client-side rendering never jumps.
func (f *Filter) Clean(msg string) string {
	lower := strings.ToLower(msg)
	out := msg
	for _, w := range f.words {
		idx := strings.Index(lower, w)
		for idx != -1 {
			out = out[:idx] + strings.Repeat("*", len(w)) + out[idx+len(w):]
			lower = lower[:idx] + strings.Repeat("*", len(w)) + lower[idx+len(w):]
			idx = strings.Index(lower, w)
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/chatfilter/... -v`
Expected: PASS.

- [ ] **Step 5: Wire chat into the WS gateway**

```go
// api/internal/api/v1/tablews.go — clientMessage, add
	Message string `json:"message,omitempty"`
```

```go
// api/internal/api/v1/tablews.go — RegisterTableWS, add a package-level filter and a case in the message switch
var chatWords = []string{"idiota", "burro"} // placeholder list — a human-curated list is an ops task, not an engineering one
var chat = chatfilter.New(chatWords)
```

```go
// api/internal/api/v1/tablews.go — inside the message-type switch
				case "chat":
					cleaned := chat.Clean(m.Message)
					data, _ := json.Marshal(map[string]any{"type": "chat", "player_id": playerID, "message": cleaned})
					reg.Broadcast(ctx, tableID+"#chat", data)
```

Chat broadcasts to a distinct key (`tableID+"#chat"`) so it doesn't collide with the per-viewer state channel
(`tableID+"#"+viewerID`) — every connected client must additionally register on the chat key:

```go
// api/internal/api/v1/tablews.go — RegisterTableWS, right after the existing reg.Register call
			reg.Register(tableID+"#chat", connID+"-chat", &wsConnAdapter{conn: conn})
			defer reg.Unregister(tableID+"#chat", connID+"-chat")
```

Report/mute (the other half of OVERVIEW.md §8.4) is out of this task's scope — flagged in the closing note, not
silently dropped.

- [ ] **Step 6: Frontend chat component**

```tsx
// ui/src/components/table/Chat.tsx
'use client'
import {useState} from 'react'

export function Chat({onSend, messages}: {onSend: (msg: string) => void; messages: {player_id: string; message: string}[]}) {
  const [draft, setDraft] = useState('')
  return (
    <div className="w-64 rounded bg-black/40 p-2 text-white">
      <div className="mb-2 h-40 overflow-y-auto text-sm">
        {messages.map((m, i) => <div key={i}><b>{m.player_id}:</b> {m.message}</div>)}
      </div>
      <input
        className="w-full rounded bg-black/60 px-2 py-1"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && draft.trim()) {
            onSend(draft)
            setDraft('')
          }
        }}
        placeholder="Diga algo…"
      />
    </div>
  )
}
```

Wiring `Chat` into `useTableRealtime`/`TablePage` requires accumulating `{type:'chat', player_id, message}`
messages client-side into a list and a `sendChat` function alongside `sendReady`/`sendAct` — add both following
the exact same pattern already established for `sendReady` in Task 10's hook.

- [ ] **Step 7: Commit**

```bash
git add api/internal/chatfilter api/internal/api/v1/tablews.go ui/src/components/table/Chat.tsx
git commit -m "feat(chat): basic table chat with a profanity filter"
```

---

### Task 14: CDK/CI — frontend hosting and deploy pipeline

**Files:**
- Create: `cdk/lib/frontend-stack.ts`
- Modify: `cdk/bin/poker.ts`
- Create: `.github/workflows/frontend.yml`
- Test: `cdk/test/frontend-stack.test.ts`

**Interfaces:**
- Mirrors `ctech-wallet/cdk/lib/frontend-stack.ts` exactly (S3 + CloudFront + OAC, ALB-origin API proxying) —
  same pattern, poker's own bucket/distribution names.

- [ ] **Step 1: Write the failing test**

```typescript
// cdk/test/frontend-stack.test.ts
import {App} from 'aws-cdk-lib';
import {Template} from 'aws-cdk-lib/assertions';
import {FrontendStack} from '../lib/frontend-stack';

test('creates an S3 bucket and CloudFront distribution', () => {
  const app = new App();
  const stack = new FrontendStack(app, 'TestFrontendStack', {
    environment: 'dev',
    certificateArn: 'arn:aws:acm:us-east-1:868899309401:certificate/test',
    apiDomainName: 'poker-api-dev.aoctech.app',
    authDomainName: 'accounts.aoctech.app',
  });
  const template = Template.fromStack(stack);
  template.resourceCountIs('AWS::S3::Bucket', 1);
  template.resourceCountIs('AWS::CloudFront::Distribution', 1);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd cdk && npx jest frontend-stack.test.ts`
Expected: FAIL — `../lib/frontend-stack` does not exist.

- [ ] **Step 3: Implement `frontend-stack.ts`**

Copy `ctech-wallet/cdk/lib/frontend-stack.ts` verbatim into `cdk/lib/frontend-stack.ts`, changing only:
`frontendBucketName`/`routeStoreName`/`SERVICE`/`API_PATH_PATTERNS` imports to poker's own `constants.ts`
equivalents (add `frontendBucketName = (env: Environment) => \`${env}-${SERVICE}-frontend\`` and
`API_PATH_PATTERNS = ['/v1.0/*']` to `cdk/lib/constants.ts` if not already present from an earlier task), and the
distribution's default root object / SPA fallback behavior (identical — Next.js static export needs the same
404→200 rewrite CloudFront function wallet's stack already implements, since poker's routing is the same
client-side-routed SPA shape).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd cdk && npx jest frontend-stack.test.ts`
Expected: PASS.

- [ ] **Step 5: Wire `bin/poker.ts`**

```typescript
// cdk/bin/poker.ts — add
new FrontendStack(app, `${environment}-ctech-poker-frontend`, {
  environment, env: awsEnv,
  certificateArn: CERT_ARN,
  apiDomainName: domainForEnv(environment, API_DOMAIN_PREFIX),
  authDomainName: domainForEnv(environment, 'accounts'),
});
```

- [ ] **Step 6: Add the CI workflow**

Copy `ctech-wallet/.github/workflows/frontend.yml` into `.github/workflows/frontend.yml`, substituting poker's
own S3 bucket name / CloudFront distribution ID (read from CDK outputs the same way the existing `api.yml`
workflow already reads `AsgName` — mirror that `aws cloudformation describe-stacks --query` pattern for the
frontend stack's bucket/distribution outputs instead) and poker's own `NEXT_PUBLIC_*` build-time env vars
(`NEXT_PUBLIC_API_URL`, `NEXT_PUBLIC_CTECH_URL`, `NEXT_PUBLIC_CTECH_CLIENT_ID`).

- [ ] **Step 7: Synth to verify no CDK errors**

Run: `cd cdk && npx tsc --noEmit`
Expected: compiles without error.

- [ ] **Step 8: Commit**

```bash
git add cdk/lib/frontend-stack.ts cdk/lib/constants.ts cdk/bin/poker.ts cdk/test/frontend-stack.test.ts .github/workflows/frontend.yml
git commit -m "feat(cdk): S3+CloudFront frontend hosting and its CI deploy pipeline"
```

---

## Closing note — flagged, not built now

- **Bluff detection** (`achievements.KeyBluff`) is cataloged but never incremented — real detection needs a
  folded opponent's hole cards compared against the winner's hand at a hand that ended without showdown, which
  `hand.HandOutcome` (Task 3) doesn't currently carry. Adding a `FoldedHands map[string]handeval.Score` field to
  `HandOutcome` and a comparison in `achievements.Service.RecordHand` is the natural follow-up — scoped out here
  to keep Task 3 to one clearly-testable unit of work.
- **Report/mute** (the other half of OVERVIEW.md §8.4's chat moderation) is not built — Task 13 only ships the
  profanity mask. A `POST /v1.0/tables/:id/report` endpoint plus a per-player mute list checked before
  broadcasting chat is the natural next slice.
- **`ActionBar`'s hardcoded big blind** (Task 11) is a real, named gap — `hand.Snapshot` needs a `SmallBlind`/
  `BigBlind` field (touches Phase 2 code) before the frontend can show the correct raise sizing without guessing.
- **Cash-out reconciliation** was already flagged in Phase 3's closing note and remains open here — gamification
  doesn't make it worse, but it's still the single biggest correctness gap left in the sandbox money path.
