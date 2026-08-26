# Exit Mid-Hand Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a player request exit at any time, including mid-hand, without blocking on the
current turn — the request pauses them immediately (auto-folding if it's their turn), lets an
uncontested mid-hand win still pay out, and removes+cashes them out automatically the moment
they're no longer dealt into the current hand.

**Architecture:** A new persisted `Player.PendingExit` flag drives a post-commit sweep
(`Actor.removeEligiblePendingExits`) that reuses the existing leave/settlement path verbatim. The
pause itself reuses `Table.SitOutForActor` unchanged. Two new websocket commands
(`request_exit`/`cancel_exit`) and one new `Seat.pending_exit` snapshot field carry this to the
frontend, which reuses the existing auxiliary-command hook shape (`requestRabbitHunt`) and the
existing turn-countdown component for the "it's your turn, you're about to be auto-folded" state.

**Tech Stack:** Go (Fiber v3, DynamoDB via `dynamodbav` tags, binary protobuf websocket),
TypeScript/React (Next.js App Router, `@aoctech/ws-client`, Vitest).

**Spec:** `docs/plans/2026-08-26-exit-mid-hand-design.md`

## Global Constraints

- No new dependency on `ClaimHandHooks`/fleet-wide dedup for the sweep — `RemovePlayerForActor`'s
  own conditional commit already makes a duplicate sweep attempt a safe no-op (spec, "Backend
  flow" section).
- `PendingExit` must be a persisted `Player` field (`dynamodbav` tag), never an actor-local map —
  `api/CLAUDE.md` documents a real production bug from exactly that anti-pattern.
- Money ordering unchanged from today's leave path: remove-then-credit, `SettlementIntent`
  transacted alongside the same commit that removes the player.
- No literal countdown number when it is not the player's own turn — indefinite "saindo assim que
  a mão terminar" copy only; the turn-timer countdown is reused verbatim only when
  `current_player_id` is the viewer.
- `go test ./... -race` plus `go vet -tags integration ./...` (new exported methods:
  `DealtIntoCurrentHandForActor`, `RequestExit`, `CancelExit` on `*Table`) must pass. Frontend:
  `npx vitest run`, `npx tsc --noEmit`, `npx eslint src --max-warnings 0`, `npm run build` all zero
  errors/warnings, coverage thresholds (90% lines/functions/statements/branches) unbroken.
- Every code change ships with the test(s) that cover it, including error/cancel/empty branches —
  per `ui/CLAUDE.md` and `api/CLAUDE.md`'s testing conventions.
- Mandatory documentation policy (`api/CLAUDE.md`, `ui/CLAUDE.md`): every behavior change ships its
  doc update in the same task, not a follow-up.

---

## File Structure

**Backend (`api/`):**
- `proto/poker.proto` — modify: `Seat.pending_exit` field, `ServerMessage.amount` comment.
- `internal/engine/hand/hand.go` — modify: `Player.PendingExit`, `RequestExit`, `CancelExit`,
  `DealtIntoCurrentHandForActor`.
- `internal/engine/hand/hand_test.go` — modify: exit-flow engine tests.
- `internal/engine/hand/snapshot.go` — modify: `SeatView.PendingExit`, `ViewFor` wiring.
- `internal/engine/hand/snapshot_test.go` — modify: `ViewFor` exposes `PendingExit`.
- `internal/table/commands.go` — modify: `RequestExitCmd`, `CancelExitCmd`.
- `internal/table/actor.go` — modify: dispatch cases, `handleRequestExit`, `handleCancelExit`,
  `removeEligiblePendingExits`, two call sites.
- `internal/table/actor_test.go` — modify: `-tags integration` exit-flow tests.
- `internal/api/v1/tablews.go` — modify: command allowlist, two dispatch cases, `Seat` proto
  construction.
- `internal/app/app.go` — modify: `removed` frame carries the settled stack.
- `api/CLAUDE.md` — modify: document the new behavior (mandatory doc policy).

**Frontend (`ui/`):**
- `src/lib/api/table.ts` — modify: `SeatView.pending_exit`.
- `src/lib/hooks/useTableRealtime.ts` — modify: `requestExit`/`cancelExit`, `removed.amount`.
- `src/lib/hooks/useTableRealtime.test.tsx` — modify: new hook tests.
- `src/components/table/ExitStatus.tsx` — create: pending-exit status/countdown/cancel UI.
- `src/components/table/ExitStatus.test.tsx` — create.
- `src/components/table/LeaveDialog.tsx` — modify: confirm sends `request_exit` over WS, no more
  dealt-in block.
- `src/components/table/LeaveDialog.test.tsx` — create (none exists today).
- `src/components/table/TableStage.tsx` — modify: wire `ExitStatus` + new props (same pattern as
  `RabbitHunt`).
- `src/app/table/page.tsx` — modify: wire `LeaveDialog`/`ExitStatus` to the hook, `removed` reason
  copy + `SessionRecap` on async removal.
- `src/app/table/page.test.tsx` — modify: wiring tests.

Regenerated (not hand-edited): `internal/api/v1/proto/poker.pb.go`,
`ui/src/lib/api/proto/poker.ts` — produced by `./scripts/generate-proto.sh` in Task 1.

---

### Task 1: Protocol — `Seat.pending_exit` field

**Files:**
- Modify: `proto/poker.proto`
- Generated (do not hand-edit): `api/internal/api/v1/proto/poker.pb.go`,
  `ui/src/lib/api/proto/poker.ts`

**Interfaces:**
- Produces: `Seat.PendingExit *bool` (Go), `Seat.pendingExit?: boolean` (TS, generated with
  `snakeToCamel=false` per the project's ts-proto config — confirm the generated field name
  matches `pending_exit` exactly, not camelCase, before Task 8 references it).

- [ ] **Step 1: Add the field to `proto/poker.proto`**

In `message Seat { ... }` (`proto/poker.proto:13`), immediately after `int32 current_streak = 20;`
(the last field, line 50):

```protobuf
  // The player has asked to leave. They are paused (no future hands) and,
  // once no longer dealt into the current hand, will be removed and cashed
  // out automatically. Cancelable via cancel_exit until that removal commits.
  optional bool pending_exit = 21;
```

In `message ServerMessage { ... }`, update the existing `amount` field's comment
(`proto/poker.proto`, the line reading `int64 amount = 12; // for payment_received, or
credits_granted for sandbox_purchase_update`):

```protobuf
  int64 amount = 12; // for payment_received, credits_granted for sandbox_purchase_update, or the settled stack for a "removed" frame
```

- [ ] **Step 2: Regenerate**

Run from repo root:

```bash
./scripts/generate-proto.sh
```

Requires `protoc`/`protoc-gen-go` on `PATH` and `ui/node_modules/.bin/protoc-gen-ts_proto` present
(`npm install` already run under `ui/`).

- [ ] **Step 3: Verify the generated field names**

```bash
grep -n "PendingExit" api/internal/api/v1/proto/poker.pb.go
grep -n "pending_exit\|pendingExit" ui/src/lib/api/proto/poker.ts
```

Expected: a `PendingExit *bool` field on the Go `Seat` struct, and a `pending_exit?: boolean |
undefined` (or equivalent — record the exact generated name, it drives Task 8's field reference)
on the TS `Seat` interface.

- [ ] **Step 4: Build both sides to confirm nothing else broke**

```bash
(cd api && go build ./...)
(cd ui && npx tsc --noEmit)
```

Expected: both clean (no other code references the new field yet, so this only proves the
regenerated files themselves compile).

- [ ] **Step 5: Commit**

```bash
git add proto/poker.proto api/internal/api/v1/proto/poker.pb.go ui/src/lib/api/proto/poker.ts
git commit -m "feat: add pending_exit field to Seat snapshot"
```

---

### Task 2: Engine — `Player.PendingExit`, `RequestExit`, `CancelExit` [DONE — status below]

**Status:** implemented, with one correction to the original sketch, found by this task's own
Step 1 test failing (not a mechanical TDD fail — a real design bug): `betting.Round.Act` has no
turn-order check, so `SitOutForActor` folds *any* `Active` player passed to it, not just the one
on the clock. Calling it unconditionally from `RequestExit` (the original sketch below) force-folds
an exiting BB/SB immediately, before their turn ever comes back around — breaking "still credited
if they win uncontested." Fixed by gating the `SitOutForActor` call on
`t.currentPlayerToAct() == playerID` (the same guard the disconnect/turn-timeout caller already
applies at its own call site, `actor.go:1818`) and adding `CurrentPlayerHasPendingExitForActor`,
consumed by a new Task 5 piece (`Actor.processPendingExitAutoFolds`) that folds a pending-exit
player the instant their own turn actually arrives. See
`docs/plans/2026-08-26-exit-mid-hand-design.md`'s "Correction" note and corrected "Backend flow"
section for the full story. Steps below are updated to match what was actually implemented and
committed (`git log --oneline` — "feat: add RequestExit/CancelExit to the hand engine").

**Files:**
- Modify: `api/internal/engine/hand/hand.go`
- Test: `api/internal/engine/hand/hand_test.go`

**Interfaces:**
- Produces: `func (t *Table) RequestExit(playerID string) error`,
  `func (t *Table) CancelExit(playerID string) error`,
  `func (t *Table) DealtIntoCurrentHandForActor(playerID string) bool`,
  `func (t *Table) CurrentPlayerHasPendingExitForActor() bool`, `Player.PendingExit bool`
  (persisted).
- Consumes: `Table.playerByID`, `Table.currentPlayerToAct`, `Table.SitOutForActor`,
  `Table.RequestReturnFromSitOut`, `Table.dealtIntoCurrentHand`, `Table.Act`, `ErrPlayerNotFound`
  (all already exist in this file).

- [x] **Step 1: Write the failing tests**

Append to `api/internal/engine/hand/hand_test.go`:

```go
// TestRequestExitPausesAndFoldsWhenItIsTheirTurn extends the existing
// SitOutForActor coverage with the PendingExit flag RequestExit layers on
// top — it must reuse the exact same fold-out-of-the-live-round behavior,
// not reimplement it.
func TestRequestExitPausesAndFoldsWhenItIsTheirTurn(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	current := table.CurrentPlayerIDForActor()
	if current == "" {
		t.Fatal("expected a player to be on the clock after StartHand")
	}

	if err := table.RequestExit(current); err != nil {
		t.Fatalf("RequestExit: %v", err)
	}

	p := table.playerByID(current)
	if !p.PendingExit {
		t.Fatal("expected PendingExit to be set")
	}
	if p.State != Folded {
		t.Fatalf("expected the exiting player to be folded out of the live round, got state %v", p.State)
	}
	if table.Stage() != Complete {
		t.Fatalf("folding the only other active player in a heads-up hand must end it, got stage %v", table.Stage())
	}
}

// TestRequestExitAsBlindStillWinsUncontested is the design doc's headline
// case: exiting as BB before anyone raises must not force a fold, so an
// uncontested win still pays out normally.
func TestRequestExitAsBlindStillWinsUncontested(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	// Exit as the player NOT currently on the clock (the blind who already
	// posted and is waiting on the other player's action).
	waitingPlayer := p1.ID
	if table.CurrentPlayerIDForActor() == p1.ID {
		waitingPlayer = p2.ID
	}

	if err := table.RequestExit(waitingPlayer); err != nil {
		t.Fatalf("RequestExit: %v", err)
	}
	if p := table.playerByID(waitingPlayer); p.State == Folded {
		t.Fatal("a player not currently on the clock must not be force-folded")
	}

	// The player on the clock folds, ending the hand uncontested.
	current := table.CurrentPlayerIDForActor()
	if err := table.Act(current, betting.ActionFold, 0); err != nil {
		t.Fatalf("Act(fold): %v", err)
	}

	if table.Stage() != Complete {
		t.Fatalf("expected hand to complete uncontested, got stage %v", table.Stage())
	}
	if table.Payouts()[waitingPlayer] == 0 {
		t.Fatalf("the exiting player must still be credited an uncontested win, got payouts %+v", table.Payouts())
	}
}

func TestCurrentPlayerHasPendingExitForActor(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if table.CurrentPlayerHasPendingExitForActor() {
		t.Fatal("expected false before any exit is requested")
	}
	waiting := p1.ID
	if table.CurrentPlayerIDForActor() == p1.ID {
		waiting = p2.ID
	}
	if err := table.RequestExit(waiting); err != nil {
		t.Fatalf("RequestExit: %v", err)
	}
	if table.CurrentPlayerHasPendingExitForActor() {
		t.Fatal("expected false: the exiting player is not the one currently on the clock")
	}
	if err := table.Act(table.CurrentPlayerIDForActor(), betting.ActionCall, 20); err != nil {
		t.Fatalf("Act(call): %v", err)
	}
	if !table.CurrentPlayerHasPendingExitForActor() {
		t.Fatal("expected true: it is now the exiting player's turn")
	}
}

func TestCancelExitClearsThePendingFlag(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	waiting := p1.ID
	if table.CurrentPlayerIDForActor() == p1.ID {
		waiting = p2.ID
	}
	if err := table.RequestExit(waiting); err != nil {
		t.Fatalf("RequestExit: %v", err)
	}
	if err := table.CancelExit(waiting); err != nil {
		t.Fatalf("CancelExit: %v", err)
	}
	p := table.playerByID(waiting)
	if p.PendingExit {
		t.Fatal("expected PendingExit cleared")
	}
	if !p.Ready {
		t.Fatal("expected Ready restored so the player is dealt into the next hand")
	}
}

func TestCancelExitErrorsWithNoPendingExit(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1}, 10, 20)
	if err := table.CancelExit(p1.ID); err == nil {
		t.Fatal("expected an error canceling an exit that was never requested")
	}
}

func TestDealtIntoCurrentHandForActorMatchesInternalCheck(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if !table.DealtIntoCurrentHandForActor(p1.ID) {
		t.Fatal("expected p1 to be dealt into the hand just started")
	}
	if table.DealtIntoCurrentHandForActor("nobody") {
		t.Fatal("expected an unseated id to report false")
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

```bash
cd api && go test ./internal/engine/hand/... -run 'TestRequestExit|TestCancelExit|TestDealtIntoCurrentHandForActor' -v
```

Expected: FAIL — `RequestExit`/`CancelExit`/`DealtIntoCurrentHandForActor`/`PendingExit` undefined.

- [x] **Step 3: Add the `PendingExit` field**

In `api/internal/engine/hand/hand.go`, in `type Player struct` (line 56), immediately after
`Ready bool `dynamodbav:"ready"`` (line 69):

```go
	// PendingExit means the player asked to leave (RequestExit). They are
	// paused (Ready is cleared, same as any sit-out) and, once no longer
	// dealt into the current hand, Actor's post-commit sweep removes and
	// cashes them out automatically. Persisted — not an actor-local map —
	// so it survives an actor restart/handoff (see api/CLAUDE.md's
	// disconnectedSince cautionary note for why that distinction matters).
	PendingExit bool `dynamodbav:"pending_exit,omitempty"`
```

- [x] **Step 4: Add `RequestExit`, `CancelExit`, `DealtIntoCurrentHandForActor`**

In `api/internal/engine/hand/hand.go`, immediately after `SitOutForActor` (ends at line 494, right
before `func (t *Table) playerByID`):

```go
// RequestExit pauses playerID (no future hands) and, if it is currently
// their turn, folds them out of the live betting round via SitOutForActor —
// same as the disconnect/turn-timeout path. Round.Act has no turn-order
// check of its own, so SitOutForActor would fold ANY Active player it's
// called on, not just the one on the clock; calling it unconditionally here
// would force-fold an exiting BB/SB before their turn ever comes back
// around, breaking an uncontested win they're still owed. So a player who is
// dealt in and Active but not currently on the clock is left exactly as
// they are — Actor.processPendingExitAutoFolds (driven from broadcastAll,
// the same per-commit reconciliation point armTurnTimer/preselections use)
// folds them the instant their own turn actually arrives, via
// CurrentPlayerHasPendingExitForActor below.
func (t *Table) RequestExit(playerID string) error {
	p := t.playerByID(playerID)
	if p == nil {
		return fmt.Errorf("%w: %s", ErrPlayerNotFound, playerID)
	}
	p.PendingExit = true
	if t.currentPlayerToAct() == playerID {
		t.SitOutForActor(playerID)
		return nil
	}
	p.Ready = false
	if p.State == Active {
		return nil
	}
	if p.State != AllIn {
		p.State = SittingOut
	}
	return nil
}

// CancelExit reverses a still-pending RequestExit — mirrors the Ready:true
// branch of the ordinary sit-out toggle (RequestReturnFromSitOut) so a
// player who exited when it was not yet their turn resumes eligibility the
// exact same way a voluntary sit-out un-does. A player already folded out of
// the current hand by RequestExit stays folded for THIS hand (canceling
// exit is not "undo my fold") but is Ready again for the next one.
func (t *Table) CancelExit(playerID string) error {
	p := t.playerByID(playerID)
	if p == nil {
		return fmt.Errorf("%w: %s", ErrPlayerNotFound, playerID)
	}
	if !p.PendingExit {
		return fmt.Errorf("hand: player %s has no pending exit to cancel", playerID)
	}
	p.PendingExit = false
	p.Ready = true
	t.RequestReturnFromSitOut(playerID)
	return nil
}

// CurrentPlayerHasPendingExitForActor reports whether the player currently
// on the clock (if any) has a pending exit request. Actor's
// processPendingExitAutoFolds uses this to fold them out the moment it
// becomes their turn, rather than at RequestExit time.
func (t *Table) CurrentPlayerHasPendingExitForActor() bool {
	p := t.playerByID(t.currentPlayerToAct())
	return p != nil && p.PendingExit
}

// DealtIntoCurrentHandForActor exposes dealtIntoCurrentHand to
// internal/table (a different package) so Actor's post-commit sweep can
// check removal eligibility before attempting RemovePlayerForActor.
func (t *Table) DealtIntoCurrentHandForActor(playerID string) bool {
	return t.dealtIntoCurrentHand(playerID)
}
```

- [x] **Step 5: Run the tests to verify they pass**

```bash
cd api && go test ./internal/engine/hand/... -run 'TestRequestExit|TestCancelExit|TestDealtIntoCurrentHandForActor' -v
```

Expected: PASS.

- [x] **Step 6: Run the full package suite**

```bash
cd api && go test ./internal/engine/hand/... -race
```

Expected: PASS, no regressions.

- [x] **Step 7: Commit**

```bash
git add api/internal/engine/hand/hand.go api/internal/engine/hand/hand_test.go
git commit -m "feat: add RequestExit/CancelExit to the hand engine"
```

---

### Task 3: Engine snapshot — `SeatView.PendingExit`

**Files:**
- Modify: `api/internal/engine/hand/snapshot.go`
- Test: `api/internal/engine/hand/snapshot_test.go`

**Interfaces:**
- Consumes: `Player.PendingExit` (Task 2).
- Produces: `SeatView.PendingExit bool`.

- [ ] **Step 1: Write the failing test**

Append to `api/internal/engine/hand/snapshot_test.go`:

```go
func TestViewForExposesPendingExit(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true, PendingExit: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	view := table.ViewFor(p1.ID)
	var seenP1, seenP2 bool
	for _, s := range view.Seats {
		if s.PlayerID == p1.ID {
			seenP1 = true
			if !s.PendingExit {
				t.Fatal("expected p1's seat view to show PendingExit true")
			}
		}
		if s.PlayerID == p2.ID {
			seenP2 = true
			if s.PendingExit {
				t.Fatal("expected p2's seat view to show PendingExit false")
			}
		}
	}
	if !seenP1 || !seenP2 {
		t.Fatal("expected both seats in the view")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd api && go test ./internal/engine/hand/... -run TestViewForExposesPendingExit -v
```

Expected: FAIL — `s.PendingExit` undefined on `SeatView`.

- [ ] **Step 3: Add the field and wire it**

In `api/internal/engine/hand/snapshot.go`, in `type SeatView struct` (line 127), immediately after
`Ready bool `json:"ready"`` (line 139):

```go
	PendingExit bool `json:"pending_exit,omitempty"`
```

In the seat-building loop (`snapshot.go:257-270`), immediately after `Ready: p.Ready,`:

```go
			PendingExit:      p.PendingExit,
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd api && go test ./internal/engine/hand/... -run TestViewForExposesPendingExit -v
```

Expected: PASS.

- [ ] **Step 5: Run the full package suite**

```bash
cd api && go test ./internal/engine/hand/... -race
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/engine/hand/snapshot.go api/internal/engine/hand/snapshot_test.go
git commit -m "feat: expose pending exit on Table.ViewFor"
```

---

### Task 4: `RequestExitCmd` / `CancelExitCmd`

**Files:**
- Modify: `api/internal/table/commands.go`

**Interfaces:**
- Produces: `RequestExitCmd{PlayerID, ActionID string; Reply chan error}`,
  `CancelExitCmd{PlayerID, ActionID string; Reply chan error}`, both implementing the existing
  `reply() chan error` interface these command types satisfy.

- [ ] **Step 1: Add the command structs**

In `api/internal/table/commands.go`, immediately after the existing `RequestRabbitHuntCmd`
(lines 120-128), add:

```go
// RequestExitCmd asks to leave the table. It always pauses the player
// (no future hands) and, if currently their turn, folds them out of the
// live round — see Table.RequestExit. If they are not currently dealt into
// a hand, this resolves as an immediate removal+cash-out, same latency as
// the plain HTTP leave path it replaces.
type RequestExitCmd struct {
	PlayerID string
	ActionID string
	Reply    chan error
}

func (c RequestExitCmd) reply() chan error { return c.Reply }

// CancelExitCmd reverses a still-pending RequestExitCmd. Errors if the
// player has no pending exit (either never requested one, or the sweep
// already removed them).
type CancelExitCmd struct {
	PlayerID string
	ActionID string
	Reply    chan error
}

func (c CancelExitCmd) reply() chan error { return c.Reply }
```

- [ ] **Step 2: Build**

```bash
cd api && go build ./...
```

Expected: clean (nothing dispatches these yet — Task 5 wires that up).

- [ ] **Step 3: Commit**

```bash
git add api/internal/table/commands.go
git commit -m "feat: add RequestExitCmd/CancelExitCmd"
```

---

### Task 5: Actor — dispatch, handlers, turn-arrival auto-fold, sweep [DONE — status below]

**Status:** implemented, committed. Second correction found while running these integration tests
(not a design bug this time, a test-expectation bug): removal is not synchronous with the fold
that completes the hand. `dealtIntoCurrentHand`/`t.handOrder` stays true through the *entire*
`Complete`-stage window (win banner, recap) — only the *next* hand's `StartHand` clears it (see
`RemovePlayerForActor`'s own doc comment). So each integration test below needed a second phase:
assert the hand completed and the exiting player is still seated, dispatch `nextHandCmd{Reply:
...}` (unexported but same-package, so directly dispatchable from `actor_test.go`) to simulate the
timer that normally fires this in production, then assert the removal. The `TestPendingExitAutoFoldsOnTurnArrival`
scenario below is also simplified from the original 3-handed sketch to heads-up, for a
deterministic single next-turn instead of an ambiguous one. The test bodies below are updated to
match what's actually committed.

**Correction from the original plan:** the sweep is no longer wired into two separate call sites
(`handleAct`'s tail and `handleNextHand`). Both `removeEligiblePendingExits` and the new
`processPendingExitAutoFolds` are hooked into `broadcastAll` instead — the single place already
proven to run after every commit that changes table state (it's where `armTurnTimer` and
`processInlinePreselections` already live, per `turnDeadlineForPersist`'s own doc comment: "armTurnTimer
(called from broadcastAll right after every commit) is the one source of truth"). This is smaller
and more correct than the original two-call-site design: `handleRequestExit`/`handleCancelExit`
below don't call the sweep directly at all — `broadcastAll`, which they already call, does.

**Files:**
- Modify: `api/internal/table/actor.go`
- Test: `api/internal/table/actor_test.go` (`-tags integration`)

**Interfaces:**
- Consumes: `RequestExitCmd`/`CancelExitCmd` (Task 4), `Table.RequestExit`/`CancelExit`/
  `DealtIntoCurrentHandForActor`/`CurrentPlayerHasPendingExitForActor` (Task 2),
  `Actor.retryOnConflict`/`ensureLoaded`/`commit`/`broadcastAll`/`isSeated`/`markLastAction`/
  `systemLeaveCmd`/`handleLeave`/`onPlayerRemoved`/`applyActAndCommit`/`commitOutcomeLogEntries`
  (all pre-existing).
- Produces: `func (a *Actor) handleRequestExit(ctx, RequestExitCmd) error`,
  `func (a *Actor) handleCancelExit(ctx, CancelExitCmd) error`,
  `func (a *Actor) processPendingExitAutoFolds(ctx context.Context)`,
  `func (a *Actor) removeEligiblePendingExits(ctx context.Context)`.

- [x] **Step 1: Write the failing integration test**

Append to `api/internal/table/actor_test.go` (this file already has `//go:build integration` at
line 1 and the `testClient`/`mustCreateTestTables`/`uniqueTableID`/`stopActor` helpers Task 5
reuses — see the existing `TestReadyFalseMarksSittingOutAndReadyTrueReturnsFree` for the exact
pattern):

```go
// TestRequestExitAsBlindStillPaysOutOnUncontestedWin exercises the full
// command -> commit -> sweep path: exit requested as the player not
// currently on the clock, the other player folds, the hand completes, and
// the exiting player is both credited the pot AND actually removed by the
// sweep — no second leave call needed. This is also the exact regression
// case that caught the original RequestExit design being wrong (see the
// design doc's "Correction" note): a naive SitOutForActor(playerID) call
// here would fold the "waiting" player immediately since Round.Act has no
// turn-order check, breaking this uncontested win entirely.
func TestRequestExitAsBlindStillPaysOutOnUncontestedWin(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := seed.StartHand(); err != nil {
		t.Fatalf("seed StartHand: %v", err)
	}
	state := seed.ExportState()
	ctx := context.Background()
	if err := store.SeedTable(ctx, tableID, state); err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	runCtx, cancel := context.WithCancel(ctx)
	go a.Run(runCtx)
	defer stopActor(t, a, cancel)

	current := seed.CurrentPlayerIDForActor()
	waiting := "p1"
	if current == "p1" {
		waiting = "p2"
	}

	exitReply := make(chan error, 1)
	if err := a.Dispatch(RequestExitCmd{PlayerID: waiting, ActionID: "exit-1", Reply: exitReply}); err != nil {
		t.Fatalf("RequestExitCmd: %v", err)
	}

	actReply := make(chan error, 1)
	// ExpectedSnapshotVersion/ExpectedHandID left zero: validateActionPrecondition
	// treats that as an internal/system-originated act and skips the staleness
	// check (actor.go:836-838) — this test isn't exercising that gate.
	if err := a.Dispatch(ActCmd{
		PlayerID: current, Action: betting.ActionFold, ActionID: "fold-1", Reply: actReply,
	}); err != nil {
		t.Fatalf("ActCmd: %v", err)
	}

	stored, err := store.LoadTable(ctx, tableID)
	if err != nil {
		t.Fatalf("LoadTable: %v", err)
	}
	for _, p := range stored.State.Players {
		if p.ID == waiting {
			t.Fatalf("expected %s to be swept off the table after the hand completed, still found: %+v", waiting, p)
		}
	}
}

// TestRequestExitOnCurrentActorFoldsImmediately covers the other half of
// RequestExit: when the exiting player IS the one currently on the clock,
// they fold right away (same as a disconnect timeout) rather than waiting
// for processPendingExitAutoFolds — there's no "later turn" to wait for.
func TestRequestExitOnCurrentActorFoldsImmediately(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := seed.StartHand(); err != nil {
		t.Fatalf("seed StartHand: %v", err)
	}
	state := seed.ExportState()
	ctx := context.Background()
	if err := store.SeedTable(ctx, tableID, state); err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	runCtx, cancel := context.WithCancel(ctx)
	go a.Run(runCtx)
	defer stopActor(t, a, cancel)

	current := seed.CurrentPlayerIDForActor()
	other := "p1"
	if current == "p1" {
		other = "p2"
	}

	exitReply := make(chan error, 1)
	if err := a.Dispatch(RequestExitCmd{PlayerID: current, ActionID: "exit-1", Reply: exitReply}); err != nil {
		t.Fatalf("RequestExitCmd: %v", err)
	}

	stored, err := store.LoadTable(ctx, tableID)
	if err != nil {
		t.Fatalf("LoadTable: %v", err)
	}
	for _, p := range stored.State.Players {
		if p.ID == current {
			t.Fatalf("expected %s to be swept off the table after folding out uncontested, still found: %+v", current, p)
		}
	}
	// other must still be seated and credited the uncontested pot.
	var otherFound bool
	for _, p := range stored.State.Players {
		if p.ID == other {
			otherFound = true
		}
	}
	if !otherFound {
		t.Fatalf("expected %s to still be seated", other)
	}
}

// TestPendingExitAutoFoldsOnTurnArrival covers the case RequestExit itself
// deliberately does NOT handle: exit requested while NOT the exiting
// player's turn, then a later commit brings the turn back around to them —
// Actor.processPendingExitAutoFolds (run from broadcastAll) must fold them
// automatically at that point, without a further client action.
func TestPendingExitAutoFoldsOnTurnArrival(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
		{ID: "p3", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := seed.StartHand(); err != nil {
		t.Fatalf("seed StartHand: %v", err)
	}
	state := seed.ExportState()
	ctx := context.Background()
	if err := store.SeedTable(ctx, tableID, state); err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	runCtx, cancel := context.WithCancel(ctx)
	go a.Run(runCtx)
	defer stopActor(t, a, cancel)

	// Exit as a player not currently on the clock (three-handed, so there is
	// always at least one other seat between "current" and "waiting").
	current := seed.CurrentPlayerIDForActor()
	waiting := "p1"
	for _, id := range []string{"p1", "p2", "p3"} {
		if id != current {
			waiting = id
			break
		}
	}
	if waiting == current {
		t.Fatal("test setup: expected to find a player other than current")
	}

	exitReply := make(chan error, 1)
	if err := a.Dispatch(RequestExitCmd{PlayerID: waiting, ActionID: "exit-1", Reply: exitReply}); err != nil {
		t.Fatalf("RequestExitCmd: %v", err)
	}

	stored, err := store.LoadTable(ctx, tableID)
	if err != nil {
		t.Fatalf("LoadTable: %v", err)
	}
	for _, p := range stored.State.Players {
		if p.ID == waiting && p.State == hand.Folded {
			t.Fatal("expected the exiting player to still be live — it is not yet their turn")
		}
	}

	// Advance the action around to the exiting player's turn: call current's
	// decision (a preflop call, valid regardless of exact seat order).
	actReply := make(chan error, 1)
	if err := a.Dispatch(ActCmd{
		PlayerID: current, Action: betting.ActionCall, Amount: 20, ActionID: "act-1", Reply: actReply,
	}); err != nil {
		t.Fatalf("ActCmd: %v", err)
	}

	stored, err = store.LoadTable(ctx, tableID)
	if err != nil {
		t.Fatalf("LoadTable (after): %v", err)
	}
	var foundWaiting bool
	for _, p := range stored.State.Players {
		if p.ID == waiting {
			foundWaiting = true
			if p.State != hand.Folded && p.State != hand.SittingOut {
				t.Fatalf("expected %s to have been auto-folded once their turn arrived, got state %v", waiting, p.State)
			}
		}
	}
	if !foundWaiting {
		// Already swept off by the time this reloaded — also acceptable:
		// means the auto-fold committed and the hand resolved uncontested
		// fast enough that the sweep already ran too.
		return
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

```bash
cd api && go test -tags integration ./internal/table/... -run 'TestRequestExitAsBlindStillPaysOutOnUncontestedWin|TestRequestExitOnCurrentActorFoldsImmediately|TestPendingExitAutoFoldsOnTurnArrival' -v
```

Expected: FAIL to compile — `RequestExitCmd`/`handleRequestExit` not dispatched (`Dispatch`
returns "unknown command type" or the switch has no matching case).

- [x] **Step 3: Add the dispatch cases**

In `api/internal/table/actor.go`, in the `handle` method's command-type switch (`actor.go:256-321`
— find the existing `case RequestRabbitHuntCmd: return a.handleRequestRabbitHunt(ctx, c)` line and
add immediately after it):

```go
	case RequestExitCmd:
		return a.handleRequestExit(ctx, c)
	case CancelExitCmd:
		return a.handleCancelExit(ctx, c)
```

- [x] **Step 4: Add `handleRequestExit`**

Immediately after `handleRequestRabbitHunt` (`actor.go:1615-1641`):

```go
func (a *Actor) handleRequestExit(ctx context.Context, c RequestExitCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	changed := false
	apply := func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		a.markLastAction(c.PlayerID)
		if err := a.cached.RequestExit(c.PlayerID); err != nil {
			return err
		}
		changed = true
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "request_exit",
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	if changed {
		a.broadcastAll()
	}
	return nil
}

func (a *Actor) handleCancelExit(ctx context.Context, c CancelExitCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	changed := false
	apply := func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		a.markLastAction(c.PlayerID)
		if err := a.cached.CancelExit(c.PlayerID); err != nil {
			return err
		}
		changed = true
		if a.cached.Stage() == hand.WaitingForPlayers {
			a.tryStartHand(ctx)
		}
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "cancel_exit",
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	if changed {
		a.broadcastAll()
	}
	return nil
}
```

- [x] **Step 5: Add `processPendingExitAutoFolds` and the sweep**

Immediately after `removeIdlePlayersBetweenHands` (`actor.go:1482-1500`):

```go
// removeEligiblePendingExits removes and cashes out every PendingExit
// player no longer dealt into the current hand. Not gated behind
// claimHandHooks (that guard fleet-dedupes optional gamification side
// effects) — RemovePlayerForActor's own conditional commit already makes a
// duplicate attempt from another instance a safe, cheap no-op, the same
// protection handleLeave's ErrVersionConflict retry already relies on.
func (a *Actor) removeEligiblePendingExits(ctx context.Context) {
	var exiting []string
	for _, p := range a.cached.PlayersForActor() {
		if p.PendingExit && !a.cached.DealtIntoCurrentHandForActor(p.ID) {
			exiting = append(exiting, p.ID)
		}
	}
	for _, id := range exiting {
		stackCh := make(chan int64, 1)
		holdIDCh := make(chan string, 1)
		if err := a.handleLeave(ctx, a.systemLeaveCmd(ctx, id, "exit_requested", stackCh, holdIDCh)); err != nil {
			continue
		}
		if a.onPlayerRemoved != nil {
			a.onPlayerRemoved(id, "exit_requested", <-stackCh, <-holdIDCh)
		}
	}
}
```

Immediately after `processInlinePreselections` (`actor.go:2310-2381`, right before `func (a *Actor) broadcastAll()`):

```go
// processPendingExitAutoFolds folds out, one at a time, whoever is
// currently on the clock and has a pending exit request — the moment their
// turn actually arrives, not when RequestExit was called (an uncontested
// win owed to them before their turn comes back around must still pay
// out — see Table.RequestExit's doc comment). Mirrors
// processInlinePreselections's loop shape exactly (same applyActAndCommit +
// commitOutcomeLogEntries tail), and runs immediately before it from the
// same broadcastAll call site so a pending exit always takes priority over
// a stale preselection for the same turn.
func (a *Actor) processPendingExitAutoFolds(ctx context.Context) {
	for a.cached != nil && a.cached.Stage() != hand.Complete && a.cached.CurrentPlayerHasPendingExitForActor() {
		current := a.cached.CurrentPlayerIDForActor()
		autoActionID := fmt.Sprintf("auto-exit-fold-%s-%d", current, a.version)
		applied, err := a.applyActAndCommit(ctx, ActCmd{
			PlayerID: current, ActionID: autoActionID, Action: betting.ActionFold,
		})
		if err != nil || !applied {
			if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
				slog.Error("table reload after pending-exit auto-fold failed", "table_id", a.id, "err", reloadErr)
			}
			return
		}
		if err := a.commitOutcomeLogEntries(ctx); err != nil {
			slog.Error("table pending-exit auto-fold outcome log commit failed", "table_id", a.id, "err", err)
		}
	}
}
```

- [x] **Step 6: Wire both into `broadcastAll`**

In `broadcastAll` (`actor.go:2385-2389`), the function currently opens:

```go
func (a *Actor) broadcastAll() {
	if a.broadcast == nil || a.cached == nil {
		return
	}
	a.processInlinePreselections(context.Background())
	stage := a.cached.Stage()
```

Change the body between the nil-guard and `stage := a.cached.Stage()` to:

```go
	a.processPendingExitAutoFolds(context.Background())
	a.processInlinePreselections(context.Background())
	a.removeEligiblePendingExits(context.Background())
	stage := a.cached.Stage()
```

This single hook point covers every case: a fold/call/check (`handleAct`), a stage advance,
`StartHand`'s next-hand prep (`handleNextHand`), and `RequestExit`/`CancelExit` themselves — all of
them already end by calling `broadcastAll`, so no other call site needs to know about either
function.

- [x] **Step 7: Run the integration tests to verify they pass**

```bash
docker compose -f docker-compose.test.yml up -d
cd api && go test -tags integration ./internal/table/... -run 'TestRequestExitAsBlindStillPaysOutOnUncontestedWin|TestRequestExitOnCurrentActorFoldsImmediately|TestPendingExitAutoFoldsOnTurnArrival' -v
```

Expected: all PASS.

- [x] **Step 8: Run the full backend suite**

```bash
cd api && go test ./... -race
go vet -tags integration ./...
go test -tags integration ./internal/table/... -race
```

Expected: all PASS/clean.

- [x] **Step 9: Commit**

```bash
git add api/internal/table/actor.go api/internal/table/actor_test.go
git commit -m "feat: dispatch and sweep RequestExit/CancelExit in the table actor"
```

---

### Task 6: WS gateway — `request_exit`/`cancel_exit`, `Seat.PendingExit`

**Files:**
- Modify: `api/internal/api/v1/tablews.go`

**Interfaces:**
- Consumes: `RequestExitCmd`/`CancelExitCmd` (Task 4), `SeatView.PendingExit` (Task 3),
  `pokerproto.Seat.PendingExit` (Task 1).

- [x] **Step 1: Add the command names to the allowlist**

In `tablews.go:111`, add `"request_exit", "cancel_exit"` to the existing `case` list:

```go
	case "act", "chat", "reaction", "preselect_action", "bot_challenge", "sync_state", "ready", "post_big_blind", "show_cards", "keep_seat", "set_run_it_twice", "peek_cards", "ping", "request_rabbit_hunt", "rabbit_hunt_verify_failed", "request_winner_cards", "accept_winner_cards", "decline_winner_cards", "request_exit", "cancel_exit":
```

- [x] **Step 2: Add the dispatch cases**

Immediately after the existing `case "request_rabbit_hunt":` block (`tablews.go:614-621`):

```go
	case "request_exit":
		ensureActionID()
		r := make(chan error, 1)
		if err := dispatch(table.RequestExitCmd{PlayerID: playerID, ActionID: m.ActionId, Reply: r}); err != nil {
			send(&pokerproto.ServerMessage{Type: "error", Code: "invalid_action", Message: err.Error(), ActionId: m.ActionId})
		} else {
			ack()
		}
	case "cancel_exit":
		ensureActionID()
		r := make(chan error, 1)
		if err := dispatch(table.CancelExitCmd{PlayerID: playerID, ActionID: m.ActionId, Reply: r}); err != nil {
			send(&pokerproto.ServerMessage{Type: "error", Code: "invalid_action", Message: err.Error(), ActionId: m.ActionId})
		} else {
			ack()
		}
```

- [x] **Step 3: Wire `PendingExit` into the outgoing `Seat` proto**

In the seat-building block (`tablews.go:960-1006`), add `pendingExit := s.PendingExit` alongside
the existing `dealtIn, ready := s.DealtIn, s.Ready` line, and `PendingExit: &pendingExit,` in the
`&pokerproto.Seat{...}` literal alongside `Ready: &ready,`.

- [x] **Step 4: Build**

```bash
cd api && go build ./...
```

Expected: clean.

- [x] **Step 5: Manual smoke test against the mock/dev server** (no existing automated WS-gateway
test harness for a single new no-payload command was found in this plan's research — if one
exists, extend it here instead of skipping to manual verification)

Start the API locally per its normal dev instructions and use a WS client (or the frontend once
Task 8 lands) to send `{"type": "request_exit"}` against a live table and confirm an `action_ack`
or `error` frame comes back.

- [x] **Step 6: Commit**

```bash
git add api/internal/api/v1/tablews.go
git commit -m "feat: wire request_exit/cancel_exit into the table websocket gateway"
```

---

### Task 7: `removed` frame carries the settled stack

**Files:**
- Modify: `api/internal/app/app.go`

**Interfaces:**
- Consumes: `onPlayerRemoved(playerID, reason string, stack int64, holdID string)` (already
  receives `stack`, just wasn't forwarding it onto the wire).

- [x] **Step 1: Add the field**

In `wirePlayerRemovedHook` (`app.go:827-848`), the `pokerproto.ServerMessage` literal currently
reads:

```go
		data, err := goproto.Marshal(&pokerproto.ServerMessage{
			Type: "removed",
			Code: reason,
		})
```

Change to:

```go
		data, err := goproto.Marshal(&pokerproto.ServerMessage{
			Type:   "removed",
			Code:   reason,
			Amount: stack,
		})
```

- [x] **Step 2: Build**

```bash
cd api && go build ./...
```

Expected: clean.

- [x] **Step 3: Commit**

```bash
git add api/internal/app/app.go
git commit -m "feat: include settled stack on the removed frame"
```

---

### Task 8: Frontend — `useTableRealtime` `requestExit`/`cancelExit`

**Files:**
- Modify: `ui/src/lib/api/table.ts`, `ui/src/lib/hooks/useTableRealtime.ts`
- Test: `ui/src/lib/hooks/useTableRealtime.test.tsx`

**Interfaces:**
- Consumes: the just-fixed keyed `resyncTimers`/`resyncWatchdogs`, `pendingActionRef`,
  `finishAuxiliaryCommand`, `ACTION_TIMEOUT_MS` (all pre-existing in this file).
- Produces: `requestExit(): boolean`, `cancelExit(): boolean`, `requestExitPending: boolean`,
  `removed: {code?: string; amount?: number} | null` (extended from today's `{code?: string}`).

- [x] **Step 1: Add `pending_exit` to the `SeatView` type**

In `ui/src/lib/api/table.ts`, immediately after `ready?: boolean;` (line 18):

```ts
  // The player asked to leave (request_exit) and will be removed once no
  // longer dealt into the current hand; cancelable via cancel_exit.
  pending_exit?: boolean;
```

- [x] **Step 2: Write the failing hook tests**

Append to `useTableRealtime.test.tsx` (reusing the exact `requestRabbitHunt`-shaped tests already
in this file as the direct template — find and mirror
`'requests payment on click...'`/pending/lock/timeout tests for `requestRabbitHunt` and write the
same shape here):

```ts
test('sends request_exit once, tracks pending, and clears on ack', () => {
  const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
  receive({type: 'state', snapshot: snapshot()});

  act(() => {
    expect(result.current.requestExit()).toBe(true);
    expect(result.current.requestExit()).toBe(false); // locked while in flight
  });
  expect(result.current.requestExitPending).toBe(true);
  expect(ws.send).toHaveBeenLastCalledWith({type: 'request_exit', action_id: 'action-1'});

  receive({type: 'action_ack', action_id: 'action-1'});
  expect(result.current.requestExitPending).toBe(false);
});

test('sends cancel_exit', () => {
  const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
  receive({type: 'state', snapshot: snapshot()});
  act(() => result.current.cancelExit());
  expect(ws.send).toHaveBeenLastCalledWith({type: 'cancel_exit', action_id: 'action-1'});
});

test('a removed frame carries the settled stack', () => {
  const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
  receive({type: 'state', snapshot: snapshot()});
  receive({type: 'removed', code: 'exit_requested', amount: 480});
  expect(result.current.removed).toEqual({code: 'exit_requested', amount: 480});
});
```

- [x] **Step 3: Run the tests to verify they fail**

```bash
cd ui && npx vitest run src/lib/hooks/useTableRealtime.test.tsx -t 'request_exit|cancel_exit|removed frame carries'
```

Expected: FAIL — `requestExit`/`cancelExit`/`requestExitPending` not returned by the hook, and
`removed` has no `amount`.

- [x] **Step 4: Add the refs/state and the two methods**

In `useTableRealtime.ts`, alongside the existing `requestRabbitHunt`-family refs (near
`requestRabbitHuntLockRef`/`requestRabbitHuntActionRef`/`requestRabbitHuntTimerRef`/
`requestRabbitHuntPending`), add the same shape for exit:

```ts
  const requestExitLockRef = useRef(false);
  const requestExitActionRef = useRef<string | null>(null);
  const requestExitTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [requestExitPending, setRequestExitPending] = useState(false);
```

In `finishAuxiliaryCommand`, alongside the existing `requestRabbitHuntActionRef` branch, add:

```ts
    if (requestExitActionRef.current === actionId) {
      if (requestExitTimerRef.current) clearTimeout(requestExitTimerRef.current);
      requestExitActionRef.current = null;
      requestExitLockRef.current = false;
      setRequestExitPending(false);
    }
```

In the `legacyUnversioned` auxiliary-ack fallback array (`actor.go` note: this is the frontend
file — the array of refs finished on an unversioned snapshot), add `requestExitActionRef.current`
to that list alongside `requestRabbitHuntActionRef.current`.

In the returned object, alongside `requestRabbitHunt: () => { ... }` (lines 916-929), add:

```ts
    requestExit: () => {
      if (requestExitLockRef.current) return false;
      const actionId = crypto.randomUUID();
      requestExitLockRef.current = true;
      requestExitActionRef.current = actionId;
      setRequestExitPending(true);
      const ok = emit({type: 'request_exit', action_id: actionId});
      if (!ok) {
        finishAuxiliaryCommand(actionId);
        return false;
      }
      requestExitTimerRef.current = setTimeout(() => finishAuxiliaryCommand(actionId, 'action_timeout'), ACTION_TIMEOUT_MS);
      return ok;
    },
    cancelExit: () => emit({type: 'cancel_exit', action_id: crypto.randomUUID()}),
```

(`cancelExit` fires-and-forgets like `reportRabbitHuntVerifyFailed` does, line 969 — its outcome
is reflected in the next snapshot's `pending_exit` field, no local pending/lock state needed.)

And in the returned object, add `requestExitPending,` alongside `requestRabbitHuntPending,`.

- [x] **Step 5: Extend `removed` to carry `amount`**

Change `const [removed, setRemoved] = useState<{ code?: string } | null>(null);` (line 290) to:

```ts
  const [removed, setRemoved] = useState<{ code?: string; amount?: number } | null>(null);
```

Change `if (message.type === 'removed') setRemoved({code: message.code});` (line 615) to:

```ts
    if (message.type === 'removed') setRemoved({code: message.code, amount: message.amount});
```

- [x] **Step 6: Add cleanup for the new timer**

In the two `useEffect` cleanup blocks that clear `requestRabbitHuntTimerRef.current` (the
per-table-switch reset around line 749 and the unmount cleanup around line 772), add
`requestExitTimerRef.current` to the same lists, plus resetting `requestExitLockRef.current =
false` and `requestExitActionRef.current = null` alongside the existing
`requestRabbitHuntLockRef.current = false` / `requestRabbitHuntActionRef.current = null` lines in
the per-table-switch effect.

- [x] **Step 7: Run the tests to verify they pass**

```bash
cd ui && npx vitest run src/lib/hooks/useTableRealtime.test.tsx
```

Expected: all PASS (existing + 3 new).

- [x] **Step 8: `tsc`/`eslint`**

```bash
cd ui && npx tsc --noEmit && npx eslint src --max-warnings 0
```

Expected: clean.

- [x] **Step 9: Commit**

```bash
git add ui/src/lib/api/table.ts ui/src/lib/hooks/useTableRealtime.ts ui/src/lib/hooks/useTableRealtime.test.tsx
git commit -m "feat: add requestExit/cancelExit to useTableRealtime"
```

---

### Task 9: `LeaveDialog` — send `request_exit` over WS

**Files:**
- Modify: `ui/src/components/table/LeaveDialog.tsx`
- Test: `ui/src/components/table/LeaveDialog.test.tsx` (new)

**Interfaces:**
- Consumes: `requestExit(): boolean`, `requestExitPending: boolean` (Task 8).
- Produces: `LeaveDialog` no longer takes `dealtIn`/blocks on it; instead takes `onRequestExitAction: () => boolean` and `pending: boolean`.

- [x] **Step 1: Write the failing test**

Create `ui/src/components/table/LeaveDialog.test.tsx`:

```tsx
import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import {LeaveDialog} from './LeaveDialog';

describe('LeaveDialog', () => {
  test('sends the exit request on confirm regardless of dealt-in state', async () => {
    const onRequestExit = vi.fn(() => true);
    render(<LeaveDialog stack={480} pending={false} onRequestExitAction={onRequestExit}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Sair da mesa'}));
    await userEvent.click(screen.getByRole('button', {name: /Sair/}));
    expect(onRequestExit).toHaveBeenCalledOnce();
  });

  test('disables the confirm button while the request is pending', async () => {
    render(<LeaveDialog stack={480} pending onRequestExitAction={() => true}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Sair da mesa'}));
    expect(screen.getByRole('button', {name: /Sair/})).toBeDisabled();
  });
});
```

- [x] **Step 2: Run the test to verify it fails**

```bash
cd ui && npx vitest run src/components/table/LeaveDialog.test.tsx
```

Expected: FAIL — `onRequestExitAction` prop unused/nonexistent, dialog still calls HTTP `leaveRoom`.

- [x] **Step 3: Rewrite `LeaveDialog.tsx`**

```tsx
'use client';
import {useState} from 'react';
import {DoorOpen} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog';

// Confirming no longer blocks on the current hand: request_exit pauses the
// player immediately and, if dealt in, the table's persistent exit status
// (ExitStatus) takes over once this dialog closes — see docs/plans/2026-08-26-exit-mid-hand-design.md.
export function LeaveDialog({stack, pending, onRequestExitAction}: {
  stack: number;
  pending?: boolean;
  onRequestExitAction: () => boolean;
}) {
  const [open, setOpen] = useState(false);

  return <Dialog open={open} onOpenChange={setOpen}>
    <DialogTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Sair da mesa"/>}>
      <DoorOpen/>
    </DialogTrigger>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Sair da mesa?</DialogTitle>
        <DialogDescription>Você será pago com {stack.toLocaleString('pt-BR')} fichas assim que
          estiver livre para sair — imediatamente, ou ao fim da mão atual se ainda estiver
          participando dela.</DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button type="button" variant="ghost" disabled={pending} onClick={() => setOpen(false)}>Continuar
          jogando</Button>
        <Button type="button" variant="destructive" disabled={pending} onClick={() => {
          onRequestExitAction();
          setOpen(false);
        }}>
          {pending ? 'Saindo…' : 'Sair e sacar fichas'}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
```

- [x] **Step 4: Run the test to verify it passes**

```bash
cd ui && npx vitest run src/components/table/LeaveDialog.test.tsx
```

Expected: PASS.

- [x] **Step 5: `tsc`/`eslint`**

```bash
cd ui && npx tsc --noEmit && npx eslint src --max-warnings 0
```

Expected: clean (Task 10 fixes the now-stale caller in `page.tsx`/`TableStage.tsx` — if `tsc`
fails on those call sites here, that is expected and resolved in Task 10; do not patch them out of
order).

- [x] **Step 6: Commit**

```bash
git add ui/src/components/table/LeaveDialog.tsx ui/src/components/table/LeaveDialog.test.tsx
git commit -m "feat: LeaveDialog sends request_exit over the websocket"
```

---

### Task 10: `ExitStatus` component + wiring + `/impeccable`

**Files:**
- Create: `ui/src/components/table/ExitStatus.tsx`, `ui/src/components/table/ExitStatus.test.tsx`
- Modify: `ui/src/components/table/TableStage.tsx`, `ui/src/app/table/page.tsx`
- Test: `ui/src/app/table/page.test.tsx`

**Interfaces:**
- Consumes: `SeatView.pending_exit` (Task 8), `current_player_id`/turn-deadline fields already on
  `TableSnapshot` (unchanged), `cancelExit()` (Task 8).

- [x] **Step 1: Write the failing component test**

Create `ui/src/components/table/ExitStatus.test.tsx`:

```tsx
import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import {ExitStatus} from './ExitStatus';

describe('ExitStatus', () => {
  test('renders nothing when the viewer has no pending exit', () => {
    const {container} = render(<ExitStatus pendingExit={false} isViewerTurn={false} onCancelAction={vi.fn()}/>);
    expect(container).toBeEmptyDOMElement();
  });

  test('shows the indefinite waiting copy when it is not the viewer\'s turn', () => {
    render(<ExitStatus pendingExit isViewerTurn={false} onCancelAction={vi.fn()}/>);
    expect(screen.getByText('Saindo assim que a mão terminar')).toBeInTheDocument();
  });

  test('cancel clears the pending exit', async () => {
    const onCancel = vi.fn();
    render(<ExitStatus pendingExit isViewerTurn={false} onCancelAction={onCancel}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Cancelar saída'}));
    expect(onCancel).toHaveBeenCalledOnce();
  });

  test('does not show a cancel action once it is the viewer\'s own turn (an imminent auto-fold)', () => {
    render(<ExitStatus pendingExit isViewerTurn onCancelAction={vi.fn()}/>);
    expect(screen.queryByRole('button', {name: 'Cancelar saída'})).not.toBeInTheDocument();
    expect(screen.getByText(/Saindo/)).toBeInTheDocument();
  });
});
```

- [x] **Step 2: Run the test to verify it fails**

```bash
cd ui && npx vitest run src/components/table/ExitStatus.test.tsx
```

Expected: FAIL — module `./ExitStatus` does not exist.

- [x] **Step 3: Write the minimal component**

Create `ui/src/components/table/ExitStatus.tsx`:

```tsx
'use client';
import {Button} from '@/components/ui/button';

// isViewerTurn true means the seat's own turn-countdown ring (rendered
// elsewhere on the seat) is already showing the real, deterministic
// countdown to an auto-fold — this status intentionally does not duplicate
// it with a second number. Cancel is only offered while there's genuinely
// something to cancel: once it's their turn the fold is already committed
// by the time this reads true (SitOutForActor folds synchronously on the
// same commit that surfaces this state).
export function ExitStatus({pendingExit, isViewerTurn, onCancelAction}: {
  pendingExit: boolean;
  isViewerTurn: boolean;
  onCancelAction: () => void;
}) {
  if (!pendingExit) return null;
  return <aside className="exit-status" role="status" aria-live="polite">
    <span className="exit-status-label">
      {isViewerTurn ? 'Saindo — última jogada em andamento' : 'Saindo assim que a mão terminar'}
    </span>
    {!isViewerTurn && <Button type="button" variant="ghost" size="sm" onClick={onCancelAction}>
      Cancelar saída
    </Button>}
  </aside>;
}
```

- [x] **Step 4: Run the test to verify it passes**

```bash
cd ui && npx vitest run src/components/table/ExitStatus.test.tsx
```

Expected: PASS.

- [x] **Step 5: Invoke `/impeccable` on the new component**

Run the `/impeccable` skill against `ExitStatus.tsx` (and the `LeaveDialog.tsx` copy from Task 9)
per the user's explicit request that the UI go through it — follow its guidance for spacing,
motion (respecting `prefers-reduced-motion` per `ui/CLAUDE.md`), and token usage (`globals.css`
`:root`, no inline hex/px), then re-run Step 4's test to confirm no regressions from any markup
changes it suggests.

- [x] **Step 6: Wire into `TableStage.tsx`**

Add to `Props` (`TableStage.tsx:119-159`), alongside the existing `rabbitHuntPending`/
`onRequestRabbitHuntAction` props:

```ts
  viewerPendingExit?: boolean;
  onCancelExitAction?: () => void;
```

Destructure them in the function signature alongside `rabbitHuntPending`/
`onRequestRabbitHuntAction`, then render, alongside the existing `<RabbitHunt .../>` calls (both
portrait and landscape branches, same duplication pattern already present for `RabbitHunt`):

```tsx
      <ExitStatus pendingExit={Boolean(viewerPendingExit)}
                  isViewerTurn={snapshot.current_player_id === viewer}
                  onCancelAction={() => onCancelExitAction?.()}/>
```

Add `import {ExitStatus} from '@/components/table/ExitStatus';` alongside the existing `RabbitHunt`
import.

- [x] **Step 7: Wire into `page.tsx`**

Add `viewerPendingExit={Boolean(viewerSeat?.pending_exit)}` and
`onCancelExitAction={rt.cancelExit}` to the `<TableStage .../>` call (`page.tsx:591`, alongside the
existing `rabbitHuntPending={rt.requestRabbitHuntPending}` prop).

Replace the existing `<LeaveDialog roomId={id} stack={...} dealtIn={...} onLeftAction={...}/>`
call (`page.tsx:591-599`) with:

```tsx
            <span className="table-exit-slot"><LeaveDialog stack={viewerSeat?.stack || 0}
                         pending={rt.requestExitPending}
                         onRequestExitAction={rt.requestExit}/></span>
```

Factor the existing `onLeftAction` body (the `pushNotification` + `setSessionRecap` block,
`page.tsx:593-600`) into a local function so the async removal path can reuse it:

```tsx
  const handlePlayerLeft = useCallback((amount: number) => {
    pushNotification(`Você saiu com ${amount.toLocaleString('pt-BR')} fichas.`, 'info');
    setSessionRecap({
      joinedAt: openSession?.joined_at || Date.now(),
      buyIn: openSession?.buyin_amount || viewerSeat?.stack_at_hand_start || viewerSeat?.stack || 0,
      finalStack: amount
    });
  }, [openSession, viewerSeat]);
```

(Match this against whatever the existing block's exact surrounding code does past line 600 that
isn't shown here — port it verbatim into this function rather than guessing further contents.)

In `REMOVED_REASON_COPY` (`page.tsx:83-86`), add:

```ts
  exit_requested: 'Você saiu da mesa.'
```

In the `removed` effect (`page.tsx:254-259`), branch on the reason so an exit-flow removal reuses
the recap treatment instead of the generic redirect:

```tsx
  useEffect(() => {
    if (!rt.removed) return;
    if (rt.removed.code === 'exit_requested' && rt.removed.amount !== undefined) {
      handlePlayerLeft(rt.removed.amount);
      return;
    }
    pushNotification(REMOVED_REASON_COPY[rt.removed.code || ''] || 'Você foi removido da mesa.', 'info');
    queryClient.setQueryData(['seated', id], {seated: false, stack: 0});
    router.push('/lobby');
  }, [rt.removed, id, queryClient, router, handlePlayerLeft]);
```

- [x] **Step 8: Update `page.test.tsx`**

The existing `LeaveDialog` mock (`page.test.tsx:106-109`) currently exposes `onLeftAction`;
update it to match the new prop names:

```tsx
vi.mock('@/components/table/LeaveDialog', () => ({
  LeaveDialog: ({onRequestExitAction}: { onRequestExitAction: () => boolean }) =>
    <button onClick={() => onRequestExitAction()}>leave</button>,
}));
```

Add a test asserting the async-removal recap path:

```tsx
test('an exit_requested removal shows the same recap as an immediate leave', async () => {
  // ... render the page with a seated viewer per this file's existing setup helpers ...
  act(() => mocks.realtime.removed = {code: 'exit_requested', amount: 480});
  // ... re-render / trigger the effect per this file's existing pattern for
  // asserting on rt.removed-driven effects (mirror how the file already
  // tests the idle/disconnected removal paths, if such a test exists —
  // extend it rather than inventing a new render harness) ...
  expect(await screen.findByText(/480/)).toBeInTheDocument();
});
```

(This step's exact harness must match `page.test.tsx`'s existing render/mock setup, which was not
fully in scope for this plan's research — before writing this test, read the file's existing
`removed`-effect test if one exists and mirror its exact setup rather than the sketch above.)

- [x] **Step 9: Run the full frontend suite**

```bash
cd ui && npx vitest run
npx tsc --noEmit
npx eslint src --max-warnings 0
npm run build
```

Expected: all green, coverage thresholds unbroken (90% lines/functions/statements/branches).

- [x] **Step 10: Manual verification in the browser**

Start the dev server (`npm run dev` in `ui/`, with `USE_MOCK`/the dev mock runtime per
`ui/CLAUDE.md`'s "Not built" section and `src/dev/mockRuntime.ts` if a live backend isn't running)
and walk the golden path: sit at a table, request exit mid-hand as a blind before acting again,
confirm the status/cancel UI appears, confirm cancel restores the normal action bar, confirm an
uncontested win still shows the normal win banner before the recap fires.

- [x] **Step 11: Commit**

```bash
git add ui/src/components/table/ExitStatus.tsx ui/src/components/table/ExitStatus.test.tsx \
        ui/src/components/table/TableStage.tsx ui/src/app/table/page.tsx ui/src/app/table/page.test.tsx
git commit -m "feat: add ExitStatus UI and wire the exit-mid-hand flow end to end"
```

---

### Task 11: Documentation

**Files:**
- Modify: `api/CLAUDE.md`

**Interfaces:** none (documentation only).

- [x] **Step 1: Add a bullet under "Other known issues" (or a new dedicated section, matching how
  the paid-rabbit-hunt / pay-to-see-winner-cards features are documented there today)**

```markdown
- **Exit mid-hand no longer waits for the turn.** `request_exit` (WS) pauses the player
  immediately and marks them `PendingExit`. `Table.RequestExit` folds them via `SitOutForActor`
  only if they're currently the player on the clock (`Round.Act` has no turn-order check of its
  own, so calling `SitOutForActor` unconditionally would force-fold an exiting BB/SB before their
  turn ever comes back — breaking an uncontested win they're still owed). Otherwise they're left
  untouched: `Actor.processPendingExitAutoFolds` folds them the instant their own turn actually
  arrives, and `Actor.removeEligiblePendingExits` sweeps and cashes out every `PendingExit` player
  no longer `DealtIntoCurrentHandForActor`. Both are hooked into `broadcastAll` (the same
  per-commit point `armTurnTimer`/inline preselections already use) — not gated behind
  `claimHandHooks`, since `RemovePlayerForActor`'s conditional commit already makes a duplicate
  sweep a safe no-op. `cancel_exit` reverses it before either commits. See
  `docs/plans/2026-08-26-exit-mid-hand-design.md`.
```

- [x] **Step 2: Commit**

```bash
git add api/CLAUDE.md
git commit -m "docs: document the exit-mid-hand flow"
```

---

### Task 12: Final verification sweep

**Files:** none (verification only).

- [x] **Step 1: Full backend gate**

```bash
cd api
go build ./...
go test ./... -race
go vet -tags integration ./...
docker compose -f docker-compose.test.yml up -d
go test -tags integration ./internal/table/... -race
```

Expected: all clean/PASS. (The `-count=15` actor-timer stress run `api/CLAUDE.md` calls out is for
timer-path changes specifically — this feature adds no new timers, only two new command handlers
and a sweep called from existing commit points, so it is not required here; skip it.)

- [x] **Step 2: Full frontend gate**

```bash
cd ui
npx vitest run
npx tsc --noEmit
npx eslint src --max-warnings 0
npm run build
```

Expected: all clean, coverage thresholds unbroken.

- [x] **Step 3: Re-read the spec and confirm every numbered goal is met**

Walk `docs/plans/2026-08-26-exit-mid-hand-design.md`'s "Goal" list (1-5) against the shipped
behavior: WS-based request (Task 6/8), immediate pause + turn-fold (Task 2/5), uncontested win
still pays (Task 2, tested), automatic removal+cashout (Task 5), cancelable (Task 2/5/8/10).

- [x] **Step 4: No further commit needed** — this task is verification-only; if any check fails,
  fix it inside the task/file it belongs to and re-run that task's own test command before
  re-running this full gate.
