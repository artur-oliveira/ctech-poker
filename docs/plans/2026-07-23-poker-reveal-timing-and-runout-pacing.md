# Reveal Timing Grace + All-In Runout Pacing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** give the next-to-act player a grace period on top of their action timer after every board reveal, and pace an all-in runout's remaining streets one at a time (2s apart) from the backend, instead of dealing everything in one broadcast.

**Architecture:** two independent, backend-owned timing changes layered on the existing `table.Actor` timer-arming pattern (`time.AfterFunc` + a dispatched command), with zero new wire fields — the frontend already renders a growing board array correctly. Engine (`api/internal/engine/hand`) stays pure (no timers); all pacing lives in `api/internal/table/actor.go`.

**Tech Stack:** Go (engine + table actor), Next.js/React + CSS (reveal animation).

**Spec:** `docs/specs/2026-07-23-poker-reveal-timing-and-runout-pacing.md`

## Global Constraints

- Backend owns all state/timing; the frontend only renders state it receives, never fakes or independently paces timing.
- No new wire/snapshot field — reuse the existing per-`Act` `broadcastAll()` and the frontend's existing growing-board-array handling.
- Reveal grace is a fixed **+1.5s** (`RevealGrace`), added only to the first action-timer arm immediately after a stage transition into Flop, Turn, or River — not to PreFlop, and not to any same-street follow-up action.
- All-in runout pacing is a fixed **2s** per remaining street (`RunoutStreetDelay`), using the same `time.AfterFunc` + dispatched-command pattern as `armTurnTimer`/`armNextHandTimer`.
- The immediate next missing street is always dealt synchronously (same as today); only *further* streets (2 or more missing) get the paced timer treatment. A single missing street (e.g. all-in on the turn, only the river left) reveals immediately with no pacing at all, exactly like normal play.
- `NextHandDelay` (5s) must only ever arm after the runout's final showdown broadcasts — never race it.
- The river's own flip animation is subtly slower than the turn's, **always** (normal hands and runouts alike) — the frontend needs no runout-specific state to know this, only "this is the river card."
- No code change needed for the achievements/made-hand-category gating item — already confirmed correct (see spec's "Confirmed: no change needed" section). No task below touches it.

---

### Task 1: Reveal grace period

**Files:**
- Modify: `api/internal/table/turntimeout.go`
- Modify: `api/internal/table/actor.go:39-74` (struct), `:656-680` (`armTurnTimer`), `:731-768` (`broadcastAll`)
- Modify: `api/internal/table/turntimeout_test.go:11,31,42` (3 call sites)
- Create: `api/internal/table/revealgrace_test.go`

**Interfaces:**
- Produces: `RevealGrace` (const, `table` package), `Actor.armTurnTimer(current string, grace time.Duration)` (signature change), `Actor.lastBroadcastStage hand.Stage` (field), `isRevealStreet(stage hand.Stage) bool` (package-level helper).
- Consumes: `hand.Stage`, `hand.Flop`/`hand.Turn`/`hand.River`/`hand.Complete` (already imported in `actor.go`).

- [ ] **Step 1: Write the failing test**

Create `api/internal/table/revealgrace_test.go`:

```go
package table

import (
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// TestBroadcastAddsRevealGraceOnlyOnFirstArmAfterStageTransition drives a
// heads-up hand from PreFlop into Flop and asserts the first action deadline
// on the new street includes the +1.5s grace, while a same-street follow-up
// action (still on Flop) does not.
func TestBroadcastAddsRevealGraceOnlyOnFirstArmAfterStageTransition(t *testing.T) {
	table := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}

	seen := map[string]hand.Snapshot{}
	actor := New("table-1", nil, true, func(id string, snapshot hand.Snapshot) {
		seen[id] = snapshot
	})
	actor.cached = table
	actor.broadcastAll() // settles lastBroadcastStage at PreFlop, no grace expected there

	// Drive PreFlop to completion: dealer/SB calls, BB checks -> advanceStage
	// deals the flop and starts a fresh betting round.
	sb := table.CurrentPlayerIDForActor()
	if err := table.Act(sb, betting.ActionCall, 0); err != nil {
		t.Fatalf("sb call: %v", err)
	}
	bb := table.CurrentPlayerIDForActor()
	if err := table.Act(bb, betting.ActionCheck, 0); err != nil {
		t.Fatalf("bb check: %v", err)
	}
	if table.Stage() != hand.Flop {
		t.Fatalf("expected Flop after preflop completes, got %v", table.Stage())
	}

	before := time.Now()
	actor.broadcastAll()
	firstToAct := table.CurrentPlayerIDForActor()
	deadline := time.UnixMilli(seen[firstToAct].ActionDeadlineUnixMs)
	wantMin := before.Add(actor.turnTimeout + RevealGrace - 300*time.Millisecond)
	wantMax := before.Add(actor.turnTimeout + RevealGrace + 300*time.Millisecond)
	if deadline.Before(wantMin) || deadline.After(wantMax) {
		t.Fatalf("expected deadline ~turnTimeout+%v after the flop reveal, got %v (turnTimeout=%v)", RevealGrace, deadline, actor.turnTimeout)
	}

	// Same-street follow-up: firstToAct checks, the second Flop actor's
	// deadline must NOT carry the grace again.
	if err := table.Act(firstToAct, betting.ActionCheck, 0); err != nil {
		t.Fatalf("first flop action: %v", err)
	}
	if table.Stage() != hand.Flop {
		t.Fatalf("expected to still be on Flop awaiting the second action, got %v", table.Stage())
	}
	before2 := time.Now()
	actor.broadcastAll()
	secondToAct := table.CurrentPlayerIDForActor()
	deadline2 := time.UnixMilli(seen[secondToAct].ActionDeadlineUnixMs)
	wantMin2 := before2.Add(actor.turnTimeout - 300*time.Millisecond)
	wantMax2 := before2.Add(actor.turnTimeout + 300*time.Millisecond)
	if deadline2.Before(wantMin2) || deadline2.After(wantMax2) {
		t.Fatalf("expected no grace on the same-street follow-up, deadline %v not within the turnTimeout-only window", deadline2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/table/... -run TestBroadcastAddsRevealGraceOnlyOnFirstArmAfterStageTransition -v`
Expected: FAIL — compile error, `armTurnTimer` (called internally by `broadcastAll`) doesn't yet apply any grace, or (once Step 3's signature isn't in place yet) a build error is fine here since this is a pure test-first commit boundary. If Go reports a build failure because `broadcastAll`'s current single-arg call still compiles fine, the test will instead fail on the deadline assertion (`deadline` inside `turnTimeout` window with no grace, i.e. `wantMin`/`wantMax` for the graced case not met). Either failure mode is acceptable for this step.

- [ ] **Step 3: Implement**

In `api/internal/table/turntimeout.go`, add below `NextHandDelay`:

```go

// RevealGrace is added on top of the normal per-turn deadline the first time
// a new street (Flop/Turn/River) is dealt, so the board-card reveal
// animation has time to finish before the countdown visibly starts
// pressuring the next player to act. Only the first arm after a stage
// transition gets it — see broadcastAll's stage-change check in actor.go.
const RevealGrace = 1500 * time.Millisecond
```

In `api/internal/table/actor.go`, add a field to the `Actor` struct (right after `nextHandArmedFor`):

```go
	nextHandArmedFor             string
	lastBroadcastStage           hand.Stage
```

Change `armTurnTimer` (currently lines 656-680) to accept a `grace time.Duration` and add it to the deadline:

```go
// armTurnTimer (re-)arms the universal per-turn timer for current — the
// player who must act right now, connected or not (empty string when no
// decision is pending). Idempotent: re-arming for the SAME current player is
// a no-op (does not restart their clock), matching "the timer counts down
// from when the turn actually began," not from every subsequent broadcast.
// grace is added on top of the normal turnTimeout — broadcastAll passes
// RevealGrace for the first arm after a stage transition into Flop/Turn/
// River, and 0 otherwise.
func (a *Actor) armTurnTimer(current string, grace time.Duration) {
	if current == a.turnDeadlineFor {
		return
	}
	if a.turnTimer != nil {
		a.turnTimer.Stop()
	}
	a.turnDeadlineFor = current
	if current == "" {
		return
	}
	duration := a.turnTimeout + grace
	a.turnDeadline = timeNowFunc().Add(duration)
	// The timer only dispatches a command; all map/state mutations happen
	// inside Run (handleTurnTimeout), so there is no data race with the Run
	// goroutine.
	a.turnTimer = time.AfterFunc(duration, func() {
		reply := make(chan error, 1)
		_ = a.Dispatch(turnTimeoutCmd{PlayerID: current, Reply: reply})
	})
}
```

Add a package-level helper right after `armTurnTimer` (or near `equityStage`, whichever reads better — place it next to `equityStage` at the bottom of the file):

```go
// isRevealStreet reports whether stage is one of the three streets whose
// arrival deals new board cards and therefore plays a reveal animation —
// PreFlop's hole cards use a different (faster) animation and are excluded.
func isRevealStreet(stage hand.Stage) bool {
	return stage == hand.Flop || stage == hand.Turn || stage == hand.River
}
```

Update `broadcastAll` (currently lines 731-768) to compute the grace and track the last-seen stage:

```go
func (a *Actor) broadcastAll() {
	if a.broadcast == nil || a.cached == nil {
		return
	}
	stage := a.cached.Stage()
	current := a.cached.CurrentPlayerIDForActor()
	grace := time.Duration(0)
	if stage != a.lastBroadcastStage && isRevealStreet(stage) {
		grace = RevealGrace
	}
	a.armTurnTimer(current, grace)
	a.armNextHandTimer(stage == hand.Complete)
	a.lastBroadcastStage = stage
	doEquity := a.equityEnabled.Load() && equityStage(stage)
	for _, p := range a.cached.PlayersForActor() {
```

(Leave the rest of `broadcastAll` — everything from `snapshot := a.cached.ViewFor(p.ID)` onward — unchanged.)

In `api/internal/table/turntimeout_test.go`, update the 3 existing `armTurnTimer` call sites to pass the new second argument:

- Line 11: `a.armTurnTimer("p1")` → `a.armTurnTimer("p1", 0)`
- Line 31: `a.armTurnTimer("p1")` → `a.armTurnTimer("p1", 0)` (both occurrences in `TestArmTurnTimerIsIdempotentForTheSameCurrentPlayer`)
- Line 33: `a.armTurnTimer("p1")` → `a.armTurnTimer("p1", 0)`
- Line 42: `a.armTurnTimer("p1")` → `a.armTurnTimer("p1", 0)`
- Line 43: `a.armTurnTimer("")` → `a.armTurnTimer("", 0)`

(Every call in that file passes `0` — none of these tests exercise the grace path.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/table/... -run 'TestBroadcastAddsRevealGraceOnlyOnFirstArmAfterStageTransition|TestArmTurnTimer' -v`
Expected: PASS for all matched tests.

- [ ] **Step 5: Run the full table package test suite**

Run: `go test ./api/internal/table/... -race`
Expected: PASS (this excludes `//go:build integration` files by default, which is fine for this task).

- [ ] **Step 6: Commit**

```bash
git add api/internal/table/turntimeout.go api/internal/table/actor.go api/internal/table/turntimeout_test.go api/internal/table/revealgrace_test.go
git commit -m "feat(api): add +1.5s reveal grace to the first action timer after a street transition"
```

---

### Task 2: Engine — paced all-in runout primitives

**Files:**
- Modify: `api/internal/engine/hand/hand.go:723-788` (`advanceStage`, remove `runoutBoard`, add new methods)
- Create: `api/internal/engine/hand/runout_test.go`

**Interfaces:**
- Produces: `Table.AdvanceRunoutStreetForActor()` (exported, no args, no return), `Table.IsAwaitingRunoutForActor() bool` (exported), `Table.countRemainingAndActable() (remaining, canStillAct int)` (unexported, shared by both).
- Consumes: existing `Table.runShowdown()`, `Table.dealCard()`, `Table.stage`, `Table.board`, `Table.players`, `Stage` constants (`PreFlop`, `Flop`, `Turn`, `River`, `Complete`).

- [ ] **Step 1: Write the failing tests**

Create `api/internal/engine/hand/runout_test.go`:

```go
package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
)

// TestAllInPreflopDealsFlopImmediatelyThenAwaitsPacedRunout covers the
// 3-missing-streets case: an all-in accepted at PreFlop deals the flop
// synchronously (same as advanceStage always has), then stops — pacing the
// turn and river is table.Actor's job (see IsAwaitingRunoutForActor), one
// street at a time via AdvanceRunoutStreetForActor.
func TestAllInPreflopDealsFlopImmediatelyThenAwaitsPacedRunout(t *testing.T) {
	table := NewTable([]*Player{{ID: "p1", Stack: 30, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	table.dealerSeat = 0
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}

	// Heads-up: p1 (dealer) posts the small blind and acts first preflop.
	// Shove the rest of their stack; p2 calls and remains Active with plenty
	// of chips left -- nobody else can bet against them, so this triggers
	// the runout path.
	first := table.currentPlayerToAct()
	if err := table.Act(first, betting.ActionRaise, 30); err != nil {
		t.Fatalf("p1 shove: %v", err)
	}
	second := table.currentPlayerToAct()
	if err := table.Act(second, betting.ActionCall, 0); err != nil {
		t.Fatalf("p2 call: %v", err)
	}

	if table.Stage() != Flop {
		t.Fatalf("expected the flop dealt immediately, got stage %v", table.Stage())
	}
	if len(table.board) != 3 {
		t.Fatalf("expected 3 board cards after the immediate flop deal, got %d", len(table.board))
	}
	if !table.IsAwaitingRunoutForActor() {
		t.Fatal("expected a paced runout to still be pending (turn + river missing)")
	}

	table.AdvanceRunoutStreetForActor()
	if table.Stage() != Turn || len(table.board) != 4 {
		t.Fatalf("expected the turn dealt next, got stage %v with %d board cards", table.Stage(), len(table.board))
	}
	if !table.IsAwaitingRunoutForActor() {
		t.Fatal("expected the river still pending")
	}

	table.AdvanceRunoutStreetForActor()
	if table.Stage() != Complete || len(table.board) != 5 {
		t.Fatalf("expected the river dealt and showdown run, got stage %v with %d board cards", table.Stage(), len(table.board))
	}
	if table.IsAwaitingRunoutForActor() {
		t.Fatal("expected no runout pending once showdown has run")
	}
	if table.LastOutcomeForActor() == nil {
		t.Fatal("expected showdown to have recorded an outcome")
	}
}

// TestAllInWithOnlyRiverMissingSkipsPacing covers the single-missing-street
// case: nothing to pace, the last street reveals and showdown runs in the
// same call, same as normal (non-all-in) play.
func TestAllInWithOnlyRiverMissingSkipsPacing(t *testing.T) {
	table := NewTable([]*Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	table.dealerSeat = 0
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}

	// Preflop: dealer (SB) calls up to the big blind, BB checks behind.
	preflopFirst := table.currentPlayerToAct()
	if err := table.Act(preflopFirst, betting.ActionCall, 0); err != nil {
		t.Fatalf("preflop call: %v", err)
	}
	preflopSecond := table.currentPlayerToAct()
	if err := table.Act(preflopSecond, betting.ActionCheck, 0); err != nil {
		t.Fatalf("preflop check: %v", err)
	}
	if table.Stage() != Flop {
		t.Fatalf("expected Flop after preflop, got %v", table.Stage())
	}

	// Flop: both check through, no chips committed.
	flopFirst := table.currentPlayerToAct()
	if err := table.Act(flopFirst, betting.ActionCheck, 0); err != nil {
		t.Fatalf("flop first check: %v", err)
	}
	flopSecond := table.currentPlayerToAct()
	if err := table.Act(flopSecond, betting.ActionCheck, 0); err != nil {
		t.Fatalf("flop second check: %v", err)
	}
	if table.Stage() != Turn {
		t.Fatalf("expected Turn after flop, got %v", table.Stage())
	}

	// Turn: first-to-act shoves their entire remaining stack, the other
	// calls -- only the river is left to deal.
	turnFirst := table.currentPlayerToAct()
	shoveAmount := table.playerByID(turnFirst).Stack
	if err := table.Act(turnFirst, betting.ActionBet, shoveAmount); err != nil {
		t.Fatalf("turn shove: %v", err)
	}
	turnSecond := table.currentPlayerToAct()
	if err := table.Act(turnSecond, betting.ActionCall, 0); err != nil {
		t.Fatalf("turn call: %v", err)
	}

	if table.Stage() != Complete {
		t.Fatalf("expected the river to reveal and showdown to run immediately (only one street was missing), got stage %v", table.Stage())
	}
	if len(table.board) != 5 {
		t.Fatalf("expected all 5 board cards dealt, got %d", len(table.board))
	}
	if table.LastOutcomeForActor() == nil {
		t.Fatal("expected showdown to have recorded an outcome")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./api/internal/engine/hand/... -run TestAllIn -v`
Expected: FAIL — `table.IsAwaitingRunoutForActor` / `table.AdvanceRunoutStreetForActor` undefined (build error).

- [ ] **Step 3: Implement**

In `api/internal/engine/hand/hand.go`, replace the whole block from `func (t *Table) advanceStage()` through the end of `func (t *Table) runoutBoard()` (currently lines 723-788) with:

```go
func (t *Table) advanceStage() {
	remaining, canStillAct := t.countRemainingAndActable()
	if remaining <= 1 {
		t.runShowdown()
		return
	}
	// Two or more players are still in the hand, but at most one of them is
	// NOT all-in — e.g. two players shoved pre-flop and everyone else folded
	// or called all-in too. There's nobody left who could call Act to ever
	// complete another betting round (a lone non-all-in player has no one to
	// bet against), so dealing the next street and calling startBettingRound
	// would hang the hand forever. Deal the immediate next street now (same
	// as a normal transition below) and let the caller (table.Actor) pace
	// any further streets one at a time via AdvanceRunoutStreetForActor —
	// see IsAwaitingRunoutForActor.
	if canStillAct <= 1 {
		t.AdvanceRunoutStreetForActor()
		return
	}

	switch t.stage {
	case PreFlop:
		t.board = append(t.board, t.dealCard(), t.dealCard(), t.dealCard())
		t.stage = Flop
	case Flop:
		t.board = append(t.board, t.dealCard())
		t.stage = Turn
	case Turn:
		t.board = append(t.board, t.dealCard())
		t.stage = River
	case River:
		t.runShowdown()
		return
	}
	t.startBettingRound(t.activePlayers(), 0, t.bigBlind)
}

// countRemainingAndActable reports how many players are still in the hand
// (Active or AllIn) and how many of those can still make a betting decision
// (Active only) — shared by advanceStage and IsAwaitingRunoutForActor so both
// agree on exactly the same definition of "nobody left to bet against".
func (t *Table) countRemainingAndActable() (remaining, canStillAct int) {
	for _, p := range t.players {
		if p.State == Active || p.State == AllIn {
			remaining++
			if p.State == Active {
				canStillAct++
			}
		}
	}
	return remaining, canStillAct
}

// AdvanceRunoutStreetForActor deals exactly the next missing community-card
// street (no betting round — at most one player can still act) and, once
// that street is the river, runs showdown immediately. Phase 2's table.Actor
// calls this once synchronously from within Act (via advanceStage, to reveal
// the first missing street right away) and again from a paced timer for
// every further street, checking IsAwaitingRunoutForActor between calls to
// know whether another call is still needed.
func (t *Table) AdvanceRunoutStreetForActor() {
	switch t.stage {
	case PreFlop:
		t.board = append(t.board, t.dealCard(), t.dealCard(), t.dealCard())
		t.stage = Flop
	case Flop:
		t.board = append(t.board, t.dealCard())
		t.stage = Turn
	case Turn:
		t.board = append(t.board, t.dealCard())
		t.stage = River
	}
	if t.stage == River {
		t.runShowdown()
	}
}

// IsAwaitingRunoutForActor reports whether the table is mid all-in runout —
// the board still has a street left to deal and no betting round can ever
// complete again (at most one player can still act). Excluding PreFlop keeps
// this from ever firing before the single remaining actor has had their own
// pre-flop turn: advanceStage always deals the immediate next missing street
// synchronously inside the same Act call, so by the time anyone observes
// this from outside a hand, PreFlop can never still be the case here.
// Recomputed from player state on every call — no persisted flag needed,
// since dealing a street is the only thing that can change the answer.
func (t *Table) IsAwaitingRunoutForActor() bool {
	if t.stage != Flop && t.stage != Turn {
		return false
	}
	remaining, canStillAct := t.countRemainingAndActable()
	return remaining > 1 && canStillAct <= 1
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./api/internal/engine/hand/... -run TestAllIn -v`
Expected: PASS for both `TestAllInPreflopDealsFlopImmediatelyThenAwaitsPacedRunout` and `TestAllInWithOnlyRiverMissingSkipsPacing`.

- [ ] **Step 5: Run the full hand package test suite**

Run: `go test ./api/internal/engine/hand/... -race`
Expected: PASS — confirms removing `runoutBoard` didn't break any other test (none reference it directly; verified during planning).

- [ ] **Step 6: Commit**

```bash
git add api/internal/engine/hand/hand.go api/internal/engine/hand/runout_test.go
git commit -m "feat(engine): deal all-in runout streets one at a time instead of all at once"
```

---

### Task 3: Actor — paced all-in runout timer wiring

**Files:**
- Modify: `api/internal/table/commands.go` (add `runoutStepCmd`)
- Modify: `api/internal/table/turntimeout.go` (add `RunoutStreetDelay`)
- Modify: `api/internal/table/actor.go` (struct fields, `New`, `handle`, `broadcastAll`, new `armRunoutTimer`/`handleRunoutStep`)
- Create: `api/internal/table/runoutpacing_integration_test.go`

**Interfaces:**
- Produces: `RunoutStreetDelay` (const), `runoutStepCmd` (unexported `Command`), `Actor.armRunoutTimer(awaiting bool, stage hand.Stage)`, `Actor.handleRunoutStep(ctx, runoutStepCmd) error`.
- Consumes: Task 2's `hand.Table.IsAwaitingRunoutForActor() bool` and `hand.Table.AdvanceRunoutStreetForActor()`, Task 1's `broadcastAll` structure.

- [ ] **Step 1: Write the failing integration test**

Create `api/internal/table/runoutpacing_integration_test.go`:

```go
//go:build integration

package table

import (
	"context"
	"sync"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// TestAllInRunoutPacesEachStreetBehindATimer drives a heads-up preflop
// all-in (p1 short-stacked) through the real Actor/store commit path and
// confirms the flop is dealt immediately (same broadcast as the call), then
// the turn and river each arrive roughly runoutStreetDelay apart -- not all
// five cards in a single broadcast.
func TestAllInRunoutPacesEachStreetBehindATimer(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")

	seed := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 30, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	state := seed.ExportState()
	state.DealerSeat = 0
	state.DealerDrawn = true
	ctx := context.Background()
	if err := store.SeedTable(ctx, "table-1", state); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var mu sync.Mutex
	firstSeenAt := map[int]time.Time{}
	a := New("table-1", store, true, func(_ string, snapshot hand.Snapshot) {
		mu.Lock()
		if _, ok := firstSeenAt[len(snapshot.Board)]; !ok {
			firstSeenAt[len(snapshot.Board)] = time.Now()
		}
		mu.Unlock()
	})
	a.runoutStreetDelay = 200 * time.Millisecond
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go a.Run(runCtx)

	mustDispatch(t, a, ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)})
	mustDispatch(t, a, ReadyCmd{PlayerID: "p2", Ready: true, Reply: make(chan error, 1)})

	stored, err := store.LoadTable(ctx, "table-1")
	if err != nil || stored == nil {
		t.Fatalf("expected hand to have started, got %+v err=%v", stored, err)
	}
	first := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	mustDispatch(t, a, ActCmd{PlayerID: first, ActionID: "shove", Action: betting.ActionRaise, Amount: 30, Reply: make(chan error, 1)})

	stored, _ = store.LoadTable(ctx, "table-1")
	second := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	callStart := time.Now()
	mustDispatch(t, a, ActCmd{PlayerID: second, ActionID: "call", Action: betting.ActionCall, Reply: make(chan error, 1)})

	// The flop (3 cards) must already be visible in the broadcast triggered
	// by this very Act call -- no timer needed for the immediate street.
	mu.Lock()
	_, gotFlop := firstSeenAt[3]
	mu.Unlock()
	if !gotFlop {
		t.Fatal("expected the flop dealt immediately in the same broadcast as the call")
	}

	waitForBoardLen(t, &mu, firstSeenAt, 5, 3*time.Second)

	mu.Lock()
	turnAt, turnOK := firstSeenAt[4]
	riverAt, riverOK := firstSeenAt[5]
	mu.Unlock()
	if !turnOK || !riverOK {
		t.Fatalf("expected both a 4-card and 5-card broadcast, got %+v", firstSeenAt)
	}
	if elapsed := riverAt.Sub(turnAt); elapsed < 150*time.Millisecond {
		t.Fatalf("expected the river roughly %v after the turn, got %v", a.runoutStreetDelay, elapsed)
	}
	if elapsed := turnAt.Sub(callStart); elapsed < 150*time.Millisecond {
		t.Fatalf("expected the turn roughly %v after the all-in call, got %v", a.runoutStreetDelay, elapsed)
	}

	stored, _ = store.LoadTable(ctx, "table-1")
	if stored.State.Stage != hand.Complete {
		t.Fatalf("expected the hand Complete once the runout finishes, got %v", stored.State.Stage)
	}
}

func mustDispatch(t *testing.T, a *Actor, cmd Command) {
	t.Helper()
	if err := a.Dispatch(cmd); err != nil {
		t.Fatalf("dispatch %T: %v", cmd, err)
	}
}

func waitForBoardLen(t *testing.T, mu *sync.Mutex, seen map[int]time.Time, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		_, ok := seen[n]
		mu.Unlock()
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for a %d-card board broadcast", n)
}
```

- [ ] **Step 2: Start DynamoDB Local and run the test to verify it fails**

Run: `docker compose -f api/tests/integration/docker-compose.test.yml up -d` (adjust path if the compose file lives elsewhere — check with `find api -iname docker-compose.test.yml`)
Run: `go test ./api/internal/table/... -tags integration -run TestAllInRunoutPacesEachStreetBehindATimer -v`
Expected: FAIL — `a.runoutStreetDelay` undefined (build error).

- [ ] **Step 3: Implement**

In `api/internal/table/turntimeout.go`, add below `RevealGrace`:

```go

// RunoutStreetDelay paces an all-in runout: how long the engine waits after
// dealing one community-card street before dealing the next, once two or
// more streets remain to be revealed after an all-in is accepted.
const RunoutStreetDelay = 2 * time.Second
```

In `api/internal/table/commands.go`, add near `nextHandCmd`/`turnTimeoutCmd`:

```go

// runoutStepCmd is dispatched by the paced all-in-runout timer (a
// time.AfterFunc armed in armRunoutTimer) — runoutStreetDelay after the
// previous street was dealt, dealing exactly the next one.
type runoutStepCmd struct{ Reply chan error }

func (c runoutStepCmd) reply() chan error { return c.Reply }
```

In `api/internal/table/actor.go`, add fields to the `Actor` struct (right after `nextHandDelay`):

```go
	nextHandDelay                time.Duration
	runoutTimer                  *time.Timer
	runoutTimerHandID            string
	runoutTimerStage             hand.Stage
	runoutStreetDelay            time.Duration
```

In `New`, add the default next to `nextHandDelay: NextHandDelay,`:

```go
		nextHandDelay:                NextHandDelay,
		runoutStreetDelay:            RunoutStreetDelay,
```

In `handle`'s switch, add a case next to `nextHandCmd`:

```go
	case nextHandCmd:
		return a.handleNextHand(ctx, c)
	case runoutStepCmd:
		return a.handleRunoutStep(ctx, c)
```

Add the two new methods right after `armNextHandTimer`/`handleNextHand` (after line 729, before `broadcastAll`):

```go
// armRunoutTimer (re-)arms the paced all-in-runout timer while
// IsAwaitingRunoutForActor is true. Idempotent per (handID, stage) pair —
// re-arming for the same point in the runout does not restart the delay,
// matching armTurnTimer/armNextHandTimer's convention. stage is passed in by
// broadcastAll (already knows the current stage) so this stays a plain
// comparison, no extra engine call.
func (a *Actor) armRunoutTimer(awaiting bool, stage hand.Stage) {
	if !awaiting {
		if a.runoutTimer != nil {
			a.runoutTimer.Stop()
		}
		a.runoutTimerHandID = ""
		return
	}
	if a.handID == a.runoutTimerHandID && stage == a.runoutTimerStage {
		return
	}
	if a.runoutTimer != nil {
		a.runoutTimer.Stop()
	}
	a.runoutTimerHandID = a.handID
	a.runoutTimerStage = stage
	a.runoutTimer = time.AfterFunc(a.runoutStreetDelay, func() {
		reply := make(chan error, 1)
		_ = a.Dispatch(runoutStepCmd{Reply: reply})
	})
}

// handleRunoutStep fires runoutStreetDelay after the previous runout street
// was dealt and deals exactly the next one, pacing an all-in board reveal
// instead of showing the whole runout in a single broadcast. A stale fire
// (the awaited state no longer holds, e.g. this table already finished the
// runout through another path) is a silent no-op.
func (a *Actor) handleRunoutStep(ctx context.Context, c runoutStepCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	if !a.cached.IsAwaitingRunoutForActor() {
		return nil
	}
	a.cached.AdvanceRunoutStreetForActor()
	if err := a.commit(ctx, "", nil); err != nil && !errors.Is(err, tablestore.ErrVersionConflict) {
		return err
	}
	a.broadcastAll()
	return nil
}
```

Update `broadcastAll` to arm the runout timer alongside the other two:

```go
	a.armTurnTimer(current, grace)
	a.armRunoutTimer(a.cached.IsAwaitingRunoutForActor(), stage)
	a.armNextHandTimer(stage == hand.Complete)
```

- [ ] **Step 4: Run the integration test to verify it passes**

Run: `go test ./api/internal/table/... -tags integration -run TestAllInRunoutPacesEachStreetBehindATimer -v`
Expected: PASS.

- [ ] **Step 5: Run the full test suite (unit + integration)**

Run: `go test ./api/... -race`
Run: `go test ./api/... -tags integration -race`
Expected: PASS for both.

- [ ] **Step 6: Commit**

```bash
git add api/internal/table/commands.go api/internal/table/turntimeout.go api/internal/table/actor.go api/internal/table/runoutpacing_integration_test.go
git commit -m "feat(api): pace all-in runout streets 2s apart via a backend timer"
```

---

### Task 4: Frontend — slower river flip

**Files:**
- Modify: `ui/src/components/table/PlayingCard.tsx`
- Modify: `ui/src/components/table/Board.tsx`
- Modify: `ui/src/app/globals.css`

**Interfaces:**
- Produces: `PlayingCard`'s new optional `slow?: boolean` prop, `.card-flip-slow` CSS class.
- Consumes: nothing new — `Board.tsx` already knows each card's true array index.

- [ ] **Step 1: Add the `slow` prop to `PlayingCard`**

In `ui/src/components/table/PlayingCard.tsx`, change the component signature and the revealed-card `<span>`'s className:

```tsx
export function PlayingCard({card, index, size, owner, slow}: {
  card?: string;
  index: number;
  size: 'board' | 'hole';
  owner?: 'viewer' | 'opponent';
  slow?: boolean;
}) {
  const revealed = Boolean(card && card.toLowerCase() !== 'back' && cardPath(card) !== back);
  const dimensions = size === 'board' ? {width: 68, height: 95} : {width: 46, height: 64};
  const style = {'--deal-index': index} as CSSProperties;
  if (!revealed) return <Image className={`playing-card ${size}-card`} src={back} alt="Carta fechada" {...dimensions}
    style={style}/>;
  
  const label = size === 'board'
    ? `Carta comunitária: ${cardLabel(card!)}`
    : owner === 'viewer'
      ? `Sua carta: ${cardLabel(card!)}`
      : `Carta: ${cardLabel(card!)}`;
  return (
    <span className={`playing-card ${size}-card card-reveal${slow ? ' card-flip-slow' : ''}`} role="img" aria-label={label} style={style}>
      <span className="card-reveal-inner">
        <Image className="card-back" src={back} alt="" aria-hidden="true" {...dimensions}/>
        <Image className="card-front" src={cardPath(card!)} alt="" aria-hidden="true" {...dimensions}/>
      </span>
    </span>
  );
}
```

- [ ] **Step 2: Pass `slow` for the river card in `Board.tsx`**

In `ui/src/components/table/Board.tsx`, the river is always the 5th card in `cards` (array index `4` — flop is indices 0-2, turn is index 3), regardless of the separate `--deal-index` stagger prop:

```tsx
import {PlayingCard} from '@/components/table/PlayingCard';

export function Board({cards, pot, rake}: { cards: string[]; pot: number; rake?: number }) {
  return <div className="board"><span className="game-pot">POTE <b key={pot}
    className="pot-value">{pot.toLocaleString('pt-BR')}</b>{rake ?
    <small title="Comissão da casa cobrada sobre o pote (rake)"
      aria-label={`Comissão da casa: ${rake.toLocaleString('pt-BR')} fichas`}>rake {rake.toLocaleString('pt-BR')}</small> : null}</span>
  <div>{cards.map((card, index) => <PlayingCard key={`${index}-${card}`} card={card}
    index={index < 3 ? index : 0} size="board" slow={index === 4}/>)}{Array.from({length: 5 - cards.length}, (_, i) =>
    <span key={i}/>)}</div>
  </div>;
}
```

- [ ] **Step 3: Slow the flip animation for `.card-flip-slow` in `globals.css`**

In `ui/src/app/globals.css`, right after the existing block:

```css
.card-reveal .card-front {
    z-index: 1;
    animation-name: card-front-turn
}
```

add:

```css

.card-flip-slow .card-back,
.card-flip-slow .card-front {
    animation-duration: 760ms
}
```

(760ms vs. the base 560ms — subtly slower, same easing/delay inherited from the base `.card-reveal .card-back`/`.card-front` rule since only `animation-duration` is overridden.)

- [ ] **Step 4: Lint and build**

Run: `cd ui && npm run lint`
Expected: zero errors, zero warnings.

Run: `cd ui && npm run build`
Expected: build succeeds with zero errors and zero warnings (per `ui/CLAUDE.md`'s quality gate).

- [ ] **Step 5: Manually verify in the browser**

Run: `cd ui && npm run dev:mock`
Open the table page with the mock controls, step through a hand to the river, and confirm the river card visibly takes a beat longer to flip than the turn card did. Also confirm normal (non-all-in) hands show the same slower river flip — this is not runout-specific.

- [ ] **Step 6: Commit**

```bash
git add ui/src/components/table/PlayingCard.tsx ui/src/components/table/Board.tsx ui/src/app/globals.css
git commit -m "feat(ui): flip the river card subtly slower than the turn, always"
```

---

## Self-Review Notes

- **Spec coverage:** Section A → Task 1. Section B (backend pacing) → Tasks 2-3. Section B's frontend river-flip detail → Task 4. Achievements item → explicitly no task (Global Constraints calls this out so it isn't mistaken for a gap).
- **No new wire field:** confirmed no task adds anything to `Snapshot` — Task 3 only paces *when* `broadcastAll` fires, reusing the existing `Board []string` field the frontend already renders.
- **Ordering:** Task 3 depends on Task 2's exported engine methods; Task 1 and Task 4 are independent of everything else. Recommended execution order is as numbered, but 1 and 4 could run in parallel with 2→3 if using subagent-driven-development with multiple workers.
- **Type/signature consistency checked:** `armTurnTimer(current string, grace time.Duration)` (Task 1) is called with `(current, grace)` in `broadcastAll` and with `(id, 0)` everywhere in tests; `IsAwaitingRunoutForActor()`/`AdvanceRunoutStreetForActor()` (Task 2) are called with identical names/signatures in Task 3's `armRunoutTimer`/`handleRunoutStep`.
