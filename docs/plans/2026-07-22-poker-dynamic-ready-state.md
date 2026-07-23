# Ready dinâmico + sitting-out com volta pagando BB + timer de ação universal

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** (1) A player who joins a table is ready to play immediately — no manual "Estou pronto"
click — but can mark themselves sitting-out. (2) Returning from sitting-out is free unless the player's seat projects to
SB or BB of the very next hand, in which case returning costs posting that big blind out of position (same mechanic
mid-hand joiners already pay). (3) Every player (not just disconnected ones) gets a visible per-turn countdown — default
15s, adjustable per room — and is auto-folded when it expires; a disconnected player who times out still falls through
to the existing grace/consecutive-hands sit-out escalation, unchanged.

**Architecture:** `hand.Table` (pure engine, no time/networking) gains: auto-ready on
`AddWaitingPlayer`/`AddMidHandJoiner`, a `RequestReturnFromSitOut` method reusing the exact
`PendingEntry`/`newEntrants` BB-out-of-position template `StartHand` already has for mid-hand joiners, and a
`wouldBeNextBlind` projection helper. `table.Actor` gains a single unified per-turn timer (`turnTimer`/`turnTimeoutCmd`)
that **replaces** the disconnect-only `actionDeadline`
(30s)/`deadlineTimer` mechanism: it arms for whichever player currently must act, regardless of connection state, and
folds them on expiry — but when that player is *also* in `disconnectedSince`, it first runs the existing
grace/consecutive-hands check (unchanged thresholds: 45s, 3 hands) before deciding fold vs. sit-out. This removes a real
bug the naive "add a second timer" approach would have introduced: two independent per-turn timers (a new 15s one and
the old 30s one) racing on the same disconnected player's turn would make the old one's `consecutiveDisconnectedHands`
counter never increment (the new timer folds first every time), silently breaking the "3 disconnected hands → auto
sit-out" escalation. Unifying into one timer avoids that.

**Tech Stack:** Go (existing `internal/engine/hand`, `internal/table`, `internal/roomstore`,
`internal/api/v1`), Next.js 16 (existing `useTableRealtime.ts`, CSS-driven animations, no new dependency).

## Global Constraints

- Engine package (`internal/engine/hand`) stays pure logic — no `time`/networking import. Wall-clock deadlines live in
  `table.Actor` only, which sets them onto a `hand.Snapshot` value it already builds (no new engine dependency).
- `go test ./... -race`. Anything under `//go:build integration` needs DynamoDB Local (`docker-compose.test.yml`).
- UI gate: `eslint src --max-warnings 0` && `next build` with zero errors/warnings. Animations are CSS only
  (`globals.css` keyframes) — reuse the existing `key={value}`-remount-to-restart-animation convention already used by
  `.seat-win` (`Seat.tsx:40`), not `setInterval`/effect-driven state.
- Player identity is always `claims.Sub` — never client-supplied (unaffected by this plan; no new identity-bearing input
  is introduced).
- Reuse `problem.*` HTTP error constructors, existing `roomstore.Store`, existing
  `tablemanager.Manager` roomLoader hook (already re-invoked on every actor creation — this plan extends what it loads,
  not the mechanism itself).

---

### Task 1: Auto-ready on join

**Files:**

- Modify: `../../api/internal/engine/hand/hand.go`
- Modify: `../../api/internal/table/actor.go`
- Test: `../../api/internal/engine/hand/hand_test.go`

**Interfaces:**

- `AddWaitingPlayer`/`AddMidHandJoiner` (existing signatures unchanged) now set `Ready: true` on the
  `*Player` they seat.
- New shared helper `func (a *Actor) tryStartHand()` (unexported) — both `applyReadyAndCommit` and
  `applyJoinAndCommit` call it instead of `applyReadyAndCommit` duplicating the
  `stage in {WaitingForPlayers, Complete} → StartHand()` check inline.

- [ ] **Step 1: Write the failing test**

Add to `../../api/internal/engine/hand/hand_test.go`:

```go
func TestAddWaitingPlayerIsReadyImmediately(t *testing.T) {
	table := NewTable(nil, 10, 20)
	p := &Player{ID: "p1", Stack: 1000}
	if err := table.AddWaitingPlayer(p); err != nil {
		t.Fatalf("AddWaitingPlayer: %v", err)
	}
	if !p.Ready {
		t.Fatal("a player added via AddWaitingPlayer must be Ready immediately (no manual ready click to enter play)")
	}
}

func TestAddMidHandJoinerIsReadyImmediately(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()

	p3 := &Player{ID: "p3", Stack: 1000}
	if err := table.AddMidHandJoiner(p3); err != nil {
		t.Fatalf("AddMidHandJoiner: %v", err)
	}
	if !p3.Ready {
		t.Fatal("a mid-hand joiner must be Ready immediately (still gated by readyToPost/BB, see PostBigBlindCmd)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd api && go test ./internal/engine/hand/... -run TestAdd.*IsReadyImmediately -v
```

Expected: FAIL — both assert `p.Ready` is `true`, but the zero value is `false`.

- [ ] **Step 3: Write minimal implementation**

In `hand.go`, `AddWaitingPlayer` (existing, ~line 217): after constructing nothing new is constructed here (the
`*Player` is caller-supplied) — set `p.Ready = true` as the first line inside the function, before the existing
`t.playerByID` duplicate check:

```go
func (t *Table) AddWaitingPlayer(p *Player) error {
	if t.stage != WaitingForPlayers && t.stage != Complete {
		return fmt.Errorf("hand: cannot add a waiting player while a hand is in progress, use AddMidHandJoiner")
	}
	if t.playerByID(p.ID) != nil {
		return fmt.Errorf("%w: player %s", ErrAlreadySeated, p.ID)
	}
	p.Ready = true
	t.players = append(t.players, p)
	return nil
}
```

In `AddMidHandJoiner` (~line 189), same one-line addition before `t.players = append(...)`:

```go
func (t *Table) AddMidHandJoiner(p *Player) error {
	if t.playerByID(p.ID) != nil {
		return fmt.Errorf("%w: player %s", ErrAlreadySeated, p.ID)
	}
	p.Ready = true
	p.State = PendingEntry
	t.players = append(t.players, p)
	return nil
}
```

Mid-hand joiners stay excluded from the *current* hand regardless of `Ready` (`StartHand`'s
`PendingEntry && !readyToPost` check is untouched) — `Ready: true` here only means "don't require a second manual ready
click on top of `PostBigBlindCmd`" once they're eligible.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd api && go test ./internal/engine/hand/... -run TestAdd.*IsReadyImmediately -v
```

- [ ] **Step 5: Extract the shared start-hand helper in actor.go**

In `../../api/internal/table/actor.go`, replace `applyReadyAndCommit`'s inline start-hand check (current lines 244-258)
with a call to a new shared helper, and make `applyJoinAndCommit` use the same helper (so a join that brings the table
to 2+ ready players starts the hand immediately instead of waiting for a `ReadyCmd` that no longer needs to be sent):

```go
func (a *Actor) applyReadyAndCommit(ctx context.Context, c ReadyCmd) error {
	for _, p := range a.cached.PlayersForActor() {
		if p.ID != c.PlayerID {
			continue
		}
		p.Ready = c.Ready
	}
	a.tryStartHand()
	return a.commit(ctx, "", nil)
}

// tryStartHand attempts to start a new hand if the table is between hands.
// "need at least 2 ready players" is not a caller error — the table just
// keeps waiting; StartHand's own error is swallowed here on purpose. Called
// from both a Ready toggle and a fresh Join, since a join alone can now bring
// the table to 2+ ready players (auto-ready on join, Task 1).
func (a *Actor) tryStartHand() {
	if a.cached.Stage() == hand.WaitingForPlayers || a.cached.Stage() == hand.Complete {
		if err := a.cached.StartHand(); err == nil {
			a.handID = newHandID()
		}
	}
}
```

In `applyJoinAndCommit` (~line 462), add the call right before `return a.commit(ctx, "", nil)`:

```go
func (a *Actor) applyJoinAndCommit(ctx context.Context, c JoinCmd) error {
	if c.MaxSeats > 0 && len(a.cached.PlayersForActor()) >= c.MaxSeats {
		return errors.New("table: no seats available")
	}
	p := &hand.Player{ID: c.PlayerID, Stack: c.Stack, HoldID: c.HoldID}
	stage := a.cached.Stage()
	if stage != hand.WaitingForPlayers && stage != hand.Complete {
		if err := a.cached.AddMidHandJoiner(p); err != nil {
			return err
		}
	} else if err := a.cached.AddWaitingPlayer(p); err != nil {
		return err
	}
	a.tryStartHand()
	return a.commit(ctx, "", nil)
}
```

- [ ] **Step 6: Build and run the full table package**

```bash
cd api && go build ./... && go test ./internal/table/... ./internal/engine/hand/... -race
```

Expected: all pass (no `//go:build integration` tests run without DynamoDB Local — that's fine, this task touches no
integration-only path).

- [ ] **Step 7: Commit**

```bash
git add api/internal/engine/hand/hand.go api/internal/engine/hand/hand_test.go api/internal/table/actor.go
git commit -m "feat(api): auto-ready players on join, no manual ready click to enter play"
```

---

### Task 2: Sitting-out with a free-or-pay-BB return path

**Files:**

- Modify: `../../api/internal/engine/hand/hand.go`
- Modify: `../../api/internal/engine/hand/state.go`
- Test: `../../api/internal/engine/hand/hand_test.go`

**Interfaces:**

- New: `func (t *Table) RequestReturnFromSitOut(playerID string)` — no-op if `playerID` is not currently `SittingOut`.
  Clears `SittingOut` immediately (free return) unless the player projects to SB/BB of the next hand to start, in which
  case it marks `owesBigBlind[playerID] = true` and leaves `SittingOut` set; `StartHand` clears it once that hand
  actually charges the BB (mirrors
  `readyToPost`/`newEntrants` exactly).
- `StartHand`'s active-player loop (hand.go ~257-298) gains the "owing return" branch.
- `State`/`ExportState`/`NewTableFromState` gain `OwesBigBlind map[string]bool` /
  `t.owesBigBlind`, persisted the same way `ReadyToPost` already is.

- [ ] **Step 1: Write the failing tests**

Add to `hand_test.go`:

```go
// TestReturnFromSitOutIsFreeWhenNotNearOwnBlind: a 4-handed table where the
// returning player's seat would NOT be SB/BB of the next hand returns
// immediately, no BB owed.
func TestReturnFromSitOutIsFreeWhenNotNearOwnBlind(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	p3 := &Player{ID: "p3", Stack: 1000, Ready: true}
	p4 := &Player{ID: "p4", Stack: 1000, Ready: true, State: SittingOut}
	table := NewTable([]*Player{p1, p2, p3, p4}, 10, 20)
	table.dealerDrawn = true // dealerSeat 0 (p1); blinds for the next hand land on p2 (SB), p3 (BB)

	table.RequestReturnFromSitOut("p4")
	if p4.State == SittingOut {
		t.Fatal("p4's seat is not SB/BB of the next hand — return must be free and immediate")
	}
}

// TestReturnFromSitOutOwesBigBlindWhenNearOwnBlind: the returning player's
// seat IS the projected BB of the next hand — return must stay SittingOut
// until StartHand charges the out-of-position BB.
func TestReturnFromSitOutOwesBigBlindWhenNearOwnBlind(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true, State: SittingOut}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	table.dealerDrawn = true // heads-up: dealer (p1) posts SB, p2 posts BB — p2 IS the projected BB

	table.RequestReturnFromSitOut("p2")
	if p2.State != SittingOut {
		t.Fatal("p2 projects to BB of the next hand — must stay SittingOut until the BB is actually charged")
	}

	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if p2.State != Active {
		t.Fatalf("expected p2 to be dealt in after paying the owed BB, got state %v", p2.State)
	}
	if p2.Contributed < 20 {
		t.Fatalf("expected p2 to have posted at least the big blind (20), got %d", p2.Contributed)
	}
}

func TestRequestReturnFromSitOutIsNoOpForNonSittingOutPlayer(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1}, 10, 20)
	table.RequestReturnFromSitOut("p1") // must not panic or change anything
	if p1.State != Active {
		t.Fatalf("no-op expected, got state %v", p1.State)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd api && go test ./internal/engine/hand/... -run TestReturnFromSitOut -v
```

Expected: FAIL — `table.RequestReturnFromSitOut undefined`.

- [ ] **Step 3: Implement `wouldBeNextBlind` and `RequestReturnFromSitOut`**

Add to `hand.go`, directly after `blindSeats` (~line 393):

```go
// wouldBeNextBlind reports whether playerID would post the small or big
// blind if StartHand ran right now with playerID included among the active
// players — used by RequestReturnFromSitOut to decide whether returning from
// sitting-out is free or costs a big blind (the same rule as a brand-new
// mid-hand joiner: "perto do próprio blind" = SB or BB of the very next hand,
// no window).
func (t *Table) wouldBeNextBlind(playerID string) bool {
	active := make([]*Player, 0, len(t.players))
	for _, p := range t.players {
		if p.ID == playerID {
			active = append(active, p) // the returning candidate is always projected as playing
			continue
		}
		if !p.Ready || p.State == SittingOut {
			continue
		}
		if p.State == PendingEntry && !t.readyToPost[p.ID] {
			continue
		}
		active = append(active, p)
	}
	if len(active) < 2 {
		return false
	}
	sb, bb := t.blindSeats(active)
	for i, p := range active {
		if p.ID == playerID {
			return i == sb || i == bb
		}
	}
	return false
}

// RequestReturnFromSitOut lets a sitting-out player rejoin. A no-op if the
// player is not currently SittingOut. Reuses the exact BB-out-of-position
// template StartHand already applies to mid-hand joiners (readyToPost):
// projects whether this player would be SB/BB of the next hand and, if so,
// defers the actual return until StartHand charges that big blind instead of
// clearing SittingOut immediately.
func (t *Table) RequestReturnFromSitOut(playerID string) {
	p := t.playerByID(playerID)
	if p == nil || p.State != SittingOut {
		return
	}
	if t.wouldBeNextBlind(playerID) {
		if t.owesBigBlind == nil {
			t.owesBigBlind = make(map[string]bool)
		}
		t.owesBigBlind[playerID] = true
		return
	}
	p.State = Active
}
```

- [ ] **Step 4: Wire `owesBigBlind` into StartHand and persistence**

In the `Table` struct (~line 98, next to `readyToPost`):

```go
	readyToPost   map[string]bool
	owesBigBlind  map[string]bool
```

In `StartHand`'s active-player loop (~lines 281-298), replace:

```go
	active := make([]*Player, 0, len(t.players))
	newEntrants := make(map[string]bool)
	for _, p := range t.players {
		if !p.Ready || p.State == SittingOut {
			continue
		}
		if p.State == PendingEntry && !t.readyToPost[p.ID] {
			continue
		}
		if p.State == PendingEntry {
			newEntrants[p.ID] = true
			delete(t.readyToPost, p.ID)
		}
		p.State = Active
		p.Contributed = 0
		p.HoleCards = [2]deck.Card{t.dealCard(), t.dealCard()}
		active = append(active, p)
	}
```

with:

```go
	active := make([]*Player, 0, len(t.players))
	newEntrants := make(map[string]bool)
	for _, p := range t.players {
		owingReturn := p.State == SittingOut && p.Ready && t.owesBigBlind[p.ID]
		if !p.Ready || (p.State == SittingOut && !owingReturn) {
			if p.State != PendingEntry {
				p.State = SittingOut
			}
			continue
		}
		if p.State == PendingEntry && !t.readyToPost[p.ID] {
			continue
		}
		if p.State == PendingEntry {
			newEntrants[p.ID] = true
			delete(t.readyToPost, p.ID)
		}
		if owingReturn {
			newEntrants[p.ID] = true
			delete(t.owesBigBlind, p.ID)
		}
		p.State = Active
		p.Contributed = 0
		p.HoleCards = [2]deck.Card{t.dealCard(), t.dealCard()}
		active = append(active, p)
	}
```

The added `if p.State != PendingEntry { p.State = SittingOut }` branch also closes a pre-existing cosmetic gap: a player
who merely has `Ready == false` (never called `SitOutForActor`) previously kept whatever stale `State` they had from the
last hand between hands — now `StartHand` labels them
`SittingOut` the moment it actually skips them, so the UI's "Ausente" badge is accurate the instant it matters. This
does not change who gets dealt in (that was already gated by `!p.Ready` alone).

- [ ] **Step 5: Persist `owesBigBlind`**

In `state.go`, add to `State` struct, `ExportState`, and `NewTableFromState`:

```go
// State struct: add field
	OwesBigBlind  map[string]bool
```

```go
// ExportState: add
		OwesBigBlind:  t.owesBigBlind,
```

```go
// NewTableFromState: add
		owesBigBlind:  s.OwesBigBlind,
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
cd api && go test ./internal/engine/hand/... -race -v -run "TestReturnFromSitOut|TestRequestReturnFromSitOutIsNoOp"
cd api && go test ./... -race
```

Expected: all green, including the pre-existing suite (no regression).

- [ ] **Step 7: Commit**

```bash
git add api/internal/engine/hand/hand.go api/internal/engine/hand/state.go api/internal/engine/hand/hand_test.go
git commit -m "feat(api): sitting-out return path, free unless near own blind (pays BB out of position)"
```

---

### Task 3: Wire `ReadyCmd` to sitting-out / return

**Files:**

- Modify: `../../api/internal/table/actor.go`
- Test: `../../api/internal/table/actor_test.go` (integration)

**Interfaces:**

- `applyReadyAndCommit` (Task 1's version) now branches on `c.Ready`: `false` → `SitOutForActor`
  (existing method, unchanged); `true` → `RequestReturnFromSitOut` (Task 2) before the existing Ready-flag assignment
  stands. Per confirmed decision: sitting-out reuses `ReadyCmd`, no new dedicated command.

- [ ] **Step 1: Write the failing test**

Check `actor_test.go`'s existing setup helpers (`newTestActor`, `testClient`,
`mustCreateTestTables` — same as `disconnect_test.go`) and add:

```go
func TestReadyFalseMarksSittingOutAndReadyTrueReturnsFree(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a := newTestActor(t, store)

	ctx := context.Background()
	for _, id := range []string{"p1", "p2", "p3", "p4"} {
		reply := make(chan error, 1)
		_ = a.Dispatch(JoinCmd{PlayerID: id, Stack: 1000, Reply: reply})
	}

	reply := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p4", Ready: false, Reply: reply}); err != nil {
		t.Fatalf("ReadyCmd(false): %v", err)
	}
	stored, _ := store.LoadTable(ctx, "table-1")
	for _, s := range stored.State.Players {
		if s.ID == "p4" && s.State != hand.SittingOut {
			t.Fatalf("expected p4 to be SittingOut after ready:false, got %v", s.State)
		}
	}

	reply2 := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p4", Ready: true, Reply: reply2}); err != nil {
		t.Fatalf("ReadyCmd(true): %v", err)
	}
	stored, _ = store.LoadTable(ctx, "table-1")
	for _, s := range stored.State.Players {
		if s.ID == "p4" && s.State == hand.SittingOut {
			t.Fatal("expected p4's free return (not projected SB/BB) to clear SittingOut immediately")
		}
	}
}
```

> Note for the implementer: adjust the join count/seats so p4 is NOT the projected SB/BB (4-handed,
> dealer seat 0 by construction order) — if `newTestActor`'s seeded table draws a random initial
> dealer, either force `dealerDrawn`/`dealerSeat` via a test-only seam already used elsewhere in this
> file, or assert on whichever player the projection actually excludes rather than hardcoding `p4`.

- [ ] **Step 2: Run to verify failure**

```bash
docker compose -f docker-compose.test.yml up -d
cd api && go test -tags integration ./internal/table/... -run TestReadyFalseMarksSittingOut -v
```

Expected: FAIL — today `ReadyCmd{Ready:false}` only flips the `Ready` bool, `State` stays whatever it was (not
`SittingOut`).

- [ ] **Step 3: Implement**

In `actor.go`, update `applyReadyAndCommit` (already touched in Task 1):

```go
func (a *Actor) applyReadyAndCommit(ctx context.Context, c ReadyCmd) error {
	for _, p := range a.cached.PlayersForActor() {
		if p.ID != c.PlayerID {
			continue
		}
		p.Ready = c.Ready
	}
	if c.Ready {
		a.cached.RequestReturnFromSitOut(c.PlayerID)
	} else {
		a.cached.SitOutForActor(c.PlayerID)
	}
	a.tryStartHand()
	return a.commit(ctx, "", nil)
}
```

`SitOutForActor` already exists and is safe to call unconditionally (idempotent: it just sets
`State = SittingOut`, no guard needed — mutating `Player.State` for a seat still mid-hand does not affect that hand's
already-frozen `betting.Round.Players` copy, so this is safe even if the player is currently `Active`/`AllIn` in a hand
still in progress; the label becomes accurate immediately, the live round is unaffected).

- [ ] **Step 4: Run to verify it passes**

```bash
cd api && go test -tags integration ./internal/table/... -run TestReadyFalseMarksSittingOut -v
cd api && go test -tags integration ./... -race
```

- [ ] **Step 5: Commit**

```bash
git add api/internal/table/actor.go api/internal/table/actor_test.go
git commit -m "feat(api): wire ReadyCmd to sitting-out (ready:false) and return (ready:true)"
```

---

### Task 4: Per-room turn-timeout config

**Files:**

- Modify: `../../api/internal/roomstore/room.go`
- Modify: `../../api/internal/api/v1/roomdto.go`
- Modify: `../../api/internal/api/v1/rooms.go`
- Modify: `../../api/internal/tablemanager/manager.go`
- Modify: `../../api/internal/app/app.go`
- Test: `../../api/internal/api/v1/rooms_test.go` (extend existing), `../../api/internal/tablemanager/manager_test.go`

**Interfaces:**

- `roomstore.Room.TurnTimeoutSeconds int` (new field, `omitempty`-style default: `0` means "use the 15s default",
  mirrors how `BlindEscalation` is `nil`-checked, not how it's mandatory).
- `CreateRoomRequest.TurnTimeoutSeconds *int` — public rooms may not set it (always 15s, same rule as `BlindEscalation`
  being private-only); private rooms may set 5–60s.
- `tablemanager.NewManager`'s `roomLoader` param changes from
  `func(tableID string) (*roomstore.BlindEscalation, bool, error)` to
  `func(tableID string) (*roomstore.Room, bool, error)` — every existing test call site passes a literal `nil` for this
  param, which compiles unchanged under the new function type (verified: no test constructs a non-nil roomLoader today).
- New: `table.DefaultTurnTimeout = 15 * time.Second`, `table.TurnTimeoutFor(seconds int) time.Duration`
  (0 → default).

- [ ] **Step 1: Add the room field**

In `roomstore/room.go`, add to `Room` (next to `BlindEscalation`):

```go
	TurnTimeoutSeconds   int              `dynamodbav:"turn_timeout_seconds,omitempty" json:"turn_timeout_seconds,omitempty"`
```

- [ ] **Step 2: Add the DTO field and validation**

In `roomdto.go`, add to `CreateRoomRequest`:

```go
	TurnTimeoutSeconds   *int                       `json:"turn_timeout_seconds,omitempty"`
```

In `rooms.go`'s `createRoom` (~line 58, alongside the existing `BlindEscalation` public/private check), add:

```go
	if req.Visibility == "public" && req.TurnTimeoutSeconds != nil {
		return problem.BadRequest("turn timeout is only configurable on private rooms").Send(c)
	}
	if req.TurnTimeoutSeconds != nil && (*req.TurnTimeoutSeconds < 5 || *req.TurnTimeoutSeconds > 60) {
		return problem.BadRequest("turn_timeout_seconds must be between 5 and 60").Send(c)
	}
```

And when building `room` (~line 78-87), set it for private rooms only:

```go
	if req.Visibility == "private" {
		room.ShareCode = newShareCode()
		room.BlindEscalation = req.BlindEscalation
		if req.TurnTimeoutSeconds != nil {
			room.TurnTimeoutSeconds = *req.TurnTimeoutSeconds
		}
	}
```

- [ ] **Step 3: Add the default-resolving helper**

Add to `../../api/internal/table/escalation.go` (same file already owns the analogous
`StartEscalation` per-room wiring) or a new small `../../api/internal/table/turntimeout.go`:

```go
package table

import "time"

// DefaultTurnTimeout is used for every public room and any private room that
// never configured its own turn_timeout_seconds.
const DefaultTurnTimeout = 15 * time.Second

// TurnTimeoutFor resolves a room's configured turn_timeout_seconds (0 means
// "not configured") to a duration.
func TurnTimeoutFor(seconds int) time.Duration {
	if seconds <= 0 {
		return DefaultTurnTimeout
	}
	return time.Duration(seconds) * time.Second
}
```

- [ ] **Step 4: Change `roomLoader`'s signature and usage in tablemanager**

In `manager.go`, change the field and param type (~line 38, 45):

```go
	roomLoader     func(tableID string) (*roomstore.Room, bool, error)
```

```go
func NewManager(leases *tablelease.Service, store *tablestore.Store, broadcast func(string, string, hand.Snapshot), roomLoader func(string) (*roomstore.Room, bool, error), completion ...func(string, hand.HandOutcome)) *Manager {
```

Replace the usage block (~lines 146-153):

```go
	// Re-arm blind escalation and the per-turn action timeout from the room's
	// authoritative config so both survive instance/lease moves (T6). Any
	// instance creating the actor loads the room once and applies both.
	if m.roomLoader != nil {
		if room, ok, err := m.roomLoader(tableID); err == nil && ok && room != nil {
			if room.BlindEscalation != nil {
				actor.StartEscalation(*room.BlindEscalation)
			}
			actor.SetTurnTimeoutForActor(table.TurnTimeoutFor(room.TurnTimeoutSeconds))
		}
	}
```

- [ ] **Step 5: Update the real roomLoader closure in app.go**

In `app.go`'s `newTableManager` (~lines 226-237), replace:

```go
	roomLoader := func(tableID string) (*roomstore.BlindEscalation, bool, error) {
		r, err := rooms.Get(context.Background(), tableID)
		if err != nil {
			return nil, false, err
		}
		if r == nil || r.BlindEscalation == nil {
			return nil, false, nil
		}
		return r.BlindEscalation, true, nil
	}
```

with:

```go
	// roomLoader re-arms blind escalation and the per-turn action timeout from
	// the room's authoritative config on every actor creation (T6), so both
	// survive instance/lease moves.
	roomLoader := func(tableID string) (*roomstore.Room, bool, error) {
		r, err := rooms.Get(context.Background(), tableID)
		if err != nil {
			return nil, false, err
		}
		if r == nil {
			return nil, false, nil
		}
		return r, true, nil
	}
```

- [ ] **Step 6: Build**

```bash
cd api && go build ./...
```

Expected: no errors (Task 5 below adds `SetTurnTimeoutForActor` — build this task together with Task 5's Step 1 if your
toolchain rejects the intermediate state; they're one logical unit split only for review size).

- [ ] **Step 7: Extend `rooms_test.go` for the new validation**

Add table-driven cases to whatever existing `createRoom` validation test exists in
`../../api/internal/api/v1/rooms_test.go` (check its current structure first —
`grep -n "func Test.*CreateRoom" rooms_test.go`) covering: public room + `TurnTimeoutSeconds` set → 400; private room +
`TurnTimeoutSeconds: 3` (below 5) → 400; private room + `TurnTimeoutSeconds: 20`
→ 201 with `room.TurnTimeoutSeconds == 20`.

- [ ] **Step 8: Commit**

```bash
git add api/internal/roomstore/room.go api/internal/api/v1/roomdto.go api/internal/api/v1/rooms.go \
        api/internal/tablemanager/manager.go api/internal/app/app.go api/internal/table/turntimeout.go \
        api/internal/api/v1/rooms_test.go
git commit -m "feat(api): per-room configurable turn timeout (default 15s, private rooms may adjust)"
```

---

### Task 5: Universal per-turn timer (replaces disconnect-only `actionDeadline`)

**Files:**

- Modify: `../../api/internal/table/actor.go`
- Modify: `../../api/internal/table/commands.go`
- Modify: `../../api/internal/engine/hand/snapshot.go`
- Modify: `../../api/internal/table/disconnect_test.go`
- Test: new pure-unit tests in `../../api/internal/table/turntimeout_test.go`

**Interfaces:**

- `hand.Snapshot` gains `ActionDeadlineUnixMs int64 \`json:"action_deadline_unix_ms,omitempty"\`` —
  populated by `Actor`, not by the engine (engine stays time-free).
- `hand.Table` gains `func (t *Table) CurrentPlayerIDForActor() string` (thin exported wrapper around the
  already-private `currentPlayerToAct`, same naming convention as
  `CurrentPlayerCanActForActor`).
- `Actor.actionDeadline`/`Actor.deadlineTimer`/`autoFoldCheckCmd`/`handleAutoFoldCheck`/
  `armActionDeadlineIfTheirTurn`/`armActionDeadlineForCurrentTurn` are **removed**, replaced by
  `Actor.turnTimeout` (settable via `SetTurnTimeoutForActor`, default `table.DefaultTurnTimeout`),
  `Actor.turnTimer`/`turnDeadline`/`turnDeadlineFor`, `turnTimeoutCmd`, `handleTurnTimeout`,
  `armTurnTimer`.

- [ ] **Step 1: Write the failing pure-unit tests (no DynamoDB needed)**

Add `../../api/internal/table/turntimeout_test.go` (no build tag, mirrors `escalation_test.go`'s bare-`&Actor{}`
pattern):

```go
package table

import (
	"testing"
	"time"
)

func TestArmTurnTimerEnqueuesTurnTimeoutCmdOnExpiry(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), turnTimeout: time.Millisecond}
	t.Cleanup(func() { close(a.done) })
	a.armTurnTimer("p1")

	select {
	case cmd := <-a.cmds:
		c, ok := cmd.(turnTimeoutCmd)
		if !ok {
			t.Fatalf("got command %T, want turnTimeoutCmd", cmd)
		}
		if c.PlayerID != "p1" {
			t.Fatalf("expected PlayerID p1, got %s", c.PlayerID)
		}
		cmd.reply() <- nil
	case <-time.After(200 * time.Millisecond):
		t.Fatal("turn timer did not enqueue turnTimeoutCmd")
	}
}

func TestArmTurnTimerIsIdempotentForTheSameCurrentPlayer(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), turnTimeout: time.Hour}
	t.Cleanup(func() { close(a.done) })
	a.armTurnTimer("p1")
	firstDeadline := a.turnDeadline
	a.armTurnTimer("p1") // same current player again — must not reset the deadline
	if !a.turnDeadline.Equal(firstDeadline) {
		t.Fatal("re-arming for the same current player must not restart its deadline")
	}
}

func TestArmTurnTimerClearsWhenNoCurrentPlayer(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), turnTimeout: time.Hour}
	t.Cleanup(func() { close(a.done) })
	a.armTurnTimer("p1")
	a.armTurnTimer("")
	if a.turnDeadlineFor != "" {
		t.Fatal("expected turnDeadlineFor cleared when there is no current player")
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd api && go test ./internal/table/... -run TestArmTurnTimer -v
```

Expected: FAIL — `Actor` has no `turnTimeout`/`turnTimer`/`turnDeadline`/`turnDeadlineFor` fields yet,
`turnTimeoutCmd`/`armTurnTimer` don't exist.

- [ ] **Step 3: Add the command type**

In `commands.go`, replace `autoFoldCheckCmd` (~lines 109-117) with:

```go
// turnTimeoutCmd is dispatched by the universal per-turn timer (a
// time.AfterFunc goroutine) so that all actor-map/state mutations happen
// inside Run, never from the timer goroutine (see armTurnTimer). Fires for
// WHOEVER currently must act, connected or not — a disconnected player who
// times out here still falls through to the existing grace/consecutive-hands
// check inside handleTurnTimeout before deciding fold vs. sit-out.
type turnTimeoutCmd struct {
	PlayerID string
	Reply    chan error
}

func (c turnTimeoutCmd) reply() chan error { return c.Reply }
```

- [ ] **Step 4: Replace the Actor fields, `New`, and the dispatch switch**

In `actor.go`'s `Actor` struct, replace:

```go
	actionDeadline               time.Duration
	disconnectGrace              time.Duration
	disconnectedSince            map[string]time.Time
	consecutiveDisconnectedHands map[string]int
	playerNames                  map[string]string
	deadlineTimer                *time.Timer
```

with:

```go
	turnTimeout                  time.Duration
	disconnectGrace              time.Duration
	disconnectedSince            map[string]time.Time
	consecutiveDisconnectedHands map[string]int
	playerNames                  map[string]string
	turnTimer                    *time.Timer
	turnDeadline                 time.Time
	turnDeadlineFor              string
```

In `New` (~line 59-71), replace `actionDeadline: 30 * time.Second,` with
`turnTimeout: DefaultTurnTimeout,` (import the `table` package's own constant — same package, no import needed, just
reference `DefaultTurnTimeout` directly from Task 4's `turntimeout.go`).

Add the setter (near `SetEquityEnabledForActor`):

```go
func (a *Actor) SetTurnTimeoutForActor(d time.Duration) {
	if d > 0 {
		a.turnTimeout = d
	}
}
```

In `handle`'s switch (~line 122-151), replace the `autoFoldCheckCmd` case with:

```go
	case turnTimeoutCmd:
		return a.handleTurnTimeout(ctx, c)
```

- [ ] **Step 5: Replace `handleAutoFoldCheck` with `handleTurnTimeout`**

Replace the entire `handleAutoFoldCheck` function (~lines 409-447) with:

```go
// handleTurnTimeout runs inside Run (dispatched by the universal per-turn
// timer) so it can safely read/write the actor's disconnect bookkeeping maps.
// It fires for whoever currently must act, regardless of connection state. A
// stale timer (the player already acted through the normal path before this
// fired) is a silent no-op — CurrentPlayerCanActForActor is false by then.
func (a *Actor) handleTurnTimeout(ctx context.Context, c turnTimeoutCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	if !a.cached.CurrentPlayerCanActForActor(c.PlayerID) {
		return nil
	}
	if since, disconnected := a.disconnectedSince[c.PlayerID]; disconnected {
		a.consecutiveDisconnectedHands[c.PlayerID]++ // safe: runs in Run goroutine
		if timeNowFunc().Sub(since) >= a.disconnectGrace || a.consecutiveDisconnectedHands[c.PlayerID] >= 3 {
			a.cached.SitOutForActor(c.PlayerID)
			if err := a.commit(ctx, "", nil); err != nil && !errors.Is(err, tablestore.ErrVersionConflict) {
				return err
			}
			a.broadcastAll()
			return nil
		}
	}
	completed, err := a.applyActAndCommit(ctx, ActCmd{
		PlayerID: c.PlayerID, ActionID: "turn-timeout-" + c.PlayerID, Action: betting.ActionFold, Amount: 0, Reply: c.Reply,
	})
	if errors.Is(err, tablestore.ErrVersionConflict) {
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
		completed, err = a.applyActAndCommit(ctx, ActCmd{
			PlayerID: c.PlayerID, ActionID: "turn-timeout-" + c.PlayerID, Action: betting.ActionFold, Amount: 0, Reply: c.Reply,
		})
	}
	if err != nil && !errors.Is(err, tablestore.ErrDuplicateAction) {
		return err
	}
	a.broadcastAll()
	if completed {
		a.notifyHandComplete()
	}
	return nil
}
```

This needs `"gopkg.aoctech.app/poker/api/internal/engine/betting"` imported in `actor.go` if not already (check —
`ActCmd.Action` is already `betting.Action` typed there, so it should already be imported for `handleAct`'s signature;
verify with `grep -n '"gopkg.aoctech.app/poker/api/internal/engine/betting"' actor.go`).

- [ ] **Step 6: Replace the arming functions with the unified `armTurnTimer`**

Replace `armActionDeadlineIfTheirTurn` and `armActionDeadlineForCurrentTurn` (~lines 520-553) with:

```go
// armTurnTimer (re-)arms the universal per-turn timer for current — the
// player who must act right now, connected or not (empty string when no
// decision is pending). Idempotent: re-arming for the SAME current player is
// a no-op (does not restart their clock), matching "the timer counts down
// from when the turn actually began," not from every subsequent broadcast.
func (a *Actor) armTurnTimer(current string) {
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
	a.turnDeadline = timeNowFunc().Add(a.turnTimeout)
	a.turnTimer = time.AfterFunc(a.turnTimeout, func() {
		reply := make(chan error, 1)
		_ = a.Dispatch(turnTimeoutCmd{PlayerID: current, Reply: reply})
	})
}
```

- [ ] **Step 7: Call `armTurnTimer` from `broadcastAll`, drop the old call sites**

In `broadcastAll` (~lines 555-582), add the arm call at the top and stamp the deadline onto each outgoing snapshot:

```go
func (a *Actor) broadcastAll() {
	if a.broadcast == nil || a.cached == nil {
		return
	}
	stage := a.cached.Stage()
	current := a.cached.CurrentPlayerIDForActor()
	a.armTurnTimer(current)
	doEquity := a.equityEnabled.Load() && equityStage(stage)
	for _, p := range a.cached.PlayersForActor() {
		snapshot := a.cached.ViewFor(p.ID)
		if current != "" && current == a.turnDeadlineFor {
			snapshot.ActionDeadlineUnixMs = a.turnDeadline.UnixMilli()
		}
		a.applyPlayerNames(snapshot.Seats)
		if doEquity {
			// ... unchanged
		}
		a.broadcast(p.ID, snapshot)
	}
}
```

(Keep the existing equity block body exactly as-is — only the two new lines above `doEquity` and the `if current != ""`
stamp are added; do not reformat the rest of the function.)

Remove the now-dead calls: `handleDisconnect` no longer calls `a.armActionDeadlineIfTheirTurn(c.PlayerID)`
(the universal timer is already running for whoever's turn it is, independent of connection state — disconnecting
mid-someone-else's-turn arms nothing either way, exactly as before); `handleAct` and
`handleTurnTimeout` no longer call `a.armActionDeadlineForCurrentTurn()` (folded into `broadcastAll`, which every one of
them already calls). `handleReconnect` drops its
`if a.deadlineTimer != nil { a.deadlineTimer.Stop() }` line entirely (the universal timer isn't disconnect-scoped, so
reconnecting must not stop or reset it — the clock keeps running for whoever's turn it already was).

- [ ] **Step 8: Add `CurrentPlayerIDForActor` to hand.Table and `ActionDeadlineUnixMs` to Snapshot**

In `hand.go`, add next to `CurrentPlayerCanActForActor` (~line 164):

```go
// CurrentPlayerIDForActor exposes currentPlayerToAct to Phase 2's table.Actor
// (the universal per-turn timer needs to know who must act now, and whether
// that has changed since the last broadcast, without duplicating round-state
// logic outside this package).
func (t *Table) CurrentPlayerIDForActor() string {
	return t.currentPlayerToAct()
}
```

In `snapshot.go`, add to the `Snapshot` struct (~line 9-17):

```go
	ActionDeadlineUnixMs int64 `json:"action_deadline_unix_ms,omitempty"`
```

- [ ] **Step 9: Update `disconnect_test.go` for the renamed field**

```go
	a.turnTimeout = 20 * time.Millisecond // was: a.actionDeadline = 20 * time.Millisecond
```

The rest of `TestDisconnectAutoFoldsAtActionDeadline` is unchanged — it still disconnects the player-to-act, sleeps past
the deadline, and asserts they were auto-folded; the mechanism underneath is now the unified `turnTimer`/
`handleTurnTimeout` instead of the old disconnect-only path, but the externally observable behavior (disconnected player
on the clock times out → folds) is identical.

- [ ] **Step 10: Run every affected test**

```bash
docker compose -f docker-compose.test.yml up -d
cd api && go test ./internal/table/... -run TestArmTurnTimer -v
cd api && go test -tags integration ./internal/table/... -run TestDisconnectAutoFoldsAtActionDeadline -v
cd api && go test ./... -race
cd api && go test -tags integration ./... -race
```

Expected: all green. Pay special attention to
`TestAllInRunoutDoesNotStallTheHand`/`TestBustedAllInPlayerSitsOutInsteadOfBeingRedealt`
(`hand_test.go`) and any other integration test that drives a full hand via repeated `Act` calls — none of them should
now unexpectedly time out mid-test, since the default `turnTimeout` (15s) is far longer than any test's execution time;
only tests that explicitly set a short `turnTimeout` (this task's new ones, plus `disconnect_test.go`) exercise the
timeout path at all.

- [ ] **Step 11: Commit**

```bash
git add api/internal/table/actor.go api/internal/table/commands.go api/internal/table/disconnect_test.go \
        api/internal/table/turntimeout_test.go api/internal/engine/hand/hand.go api/internal/engine/hand/snapshot.go
git commit -m "feat(api): universal per-turn action timer, unified with disconnect grace/escalation"
```

---

### Task 6: UI — sitting-out toggle, drop the manual first ready click, countdown ring

**Files:**

- Modify: `../../ui/src/lib/api/table.ts`
- Modify: `../../ui/src/app/table/page.tsx`
- Modify: `../../ui/src/components/table/Seat.tsx`
- Modify: `../../ui/src/app/globals.css`
- Modify: `../../ui/src/lib/mock.ts`

**Interfaces:**

- `TableSnapshot.action_deadline_unix_ms?: number` (Task 5's wire field).
- `Seat` gains an optional `deadlineMs?: number` prop, rendered as a CSS countdown ring only when
  `isTurn` and `deadlineMs` is present.

- [ ] **Step 1: Add the TS field**

In `table.ts`, add to `TableSnapshot`:

```ts
  action_deadline_unix_ms?: number
```

- [ ] **Step 2: Remove the manual "first ready" affordance, keep/repurpose the sitting-out toggle**

In `page.tsx`:

- The button at line 100 (`{rt.status === 'connected' ? <Button onClick={() => rt.ready()}>Estou
  pronto</Button> : ...}`) is removed entirely — auto-ready-on-join (Task 1) means there is nothing left for a
  freshly-connected player to confirm. Keep the surrounding loading `<main>` block otherwise unchanged (it now just
  shows the loader/sync copy without a button while
  `!rt.snapshot`).
- The reconnect-notice band at lines 145-148 (shown when `stage === 'waiting_for_players' ||
  stage === 'complete'`) changes: only show a button when the viewer's own seat is currently
  `sitting_out`, and its label/action becomes "Voltar a jogar" (still `rt.ready(true)` — same wire message, now
  meaningfully a "return" rather than a first-time confirmation):

```tsx
      {!connectionMessage && (s.stage === 'waiting_for_players' || s.stage === 'complete') && <div className="reconnect-notice">
          <p>{s.stage === 'complete' ? 'Mão encerrada.' : 'Aguardando jogadores.'}</p>
          {viewerSeat?.state === 'sitting_out' &&
            <Button type="button" variant="ghost" onClick={() => rt.ready(true)}>Voltar a jogar</Button>}
      </div>}
```

- Add a persistent sitting-out toggle to the header (alongside `LeaveDialog`, only relevant once actually seated and not
  already sitting out) — reuses `rt.ready(false)`:

```tsx
          {viewerSeat && viewerSeat.state !== 'sitting_out' &&
            <Button type="button" variant="ghost" onClick={() => rt.ready(false)}>Sentar fora</Button>}
```

- [ ] **Step 3: Add the countdown ring to Seat.tsx**

In `Seat.tsx`, add a `deadlineMs?: number` prop and render a ring only for the seat currently on the clock, following
the exact `key={value}`-remount-to-restart-CSS-animation convention `.seat-win`
already uses (line 40) — no `setInterval`, no effect:

```tsx
export function Seat({seat, isViewer, isTurn, index, payout = 0, deadlineMs}: {
  seat: SeatView;
  isViewer: boolean;
  isTurn: boolean;
  index: number;
  payout?: number;
  deadlineMs?: number
}) {
  const cards = seat.hole_cards;
  const chance = seat.equity == null ? null : Math.round(seat.equity * 100);
  const pendingName = !isViewer && !seat.name;
  const remainingMs = isTurn && deadlineMs ? Math.max(0, deadlineMs - Date.now()) : null;
  return <div data-state={seat.state} aria-current={isTurn ? 'true' : undefined}
    className={`game-seat seat-${index} ${seat.state} ${isViewer ? 'viewer' : ''} ${isTurn ? 'is-turn' : ''} ${payout > 0 ? 'is-winner' : ''} ${pendingName ? 'is-pending-name' : ''} ${TOP_SEAT_INDICES.includes(index) ? 'top-seat' : ''}`}>
    {remainingMs != null &&
      <span key={deadlineMs} className="seat-turn-ring" style={{animationDuration: `${remainingMs}ms`}} aria-hidden="true"/>}
    <div className="seat-cards">{/* unchanged */}</div>
    {/* unchanged */}
  </div>;
}
```

`Date.now()` here runs once per render at mount/prop-change time (same as any plain React render), not inside an effect
or interval — this is the same "compute once, let CSS own the animation clock"
pattern the codebase already uses, not the anti-pattern flagged earlier this session (`react-hooks/set-state-in-effect`
was about calling `setState` from inside a `useEffect` tick loop; this reads `Date.now()` during render and hands the
result straight to a CSS custom duration, no state involved).

- [ ] **Step 4: Pass `deadlineMs` from page.tsx**

Where `<Seat .../>` is rendered (~line 152):

```tsx
        {rotateSeats(s.seats, viewer).map((seat, i) => <Seat key={seat.player_id} seat={seat} index={i}
                                                             isTurn={s.current_player_id === seat.player_id}
                                                             payout={s.payouts?.[seat.player_id] || 0}
                                                             deadlineMs={s.action_deadline_unix_ms}
                                                             isViewer={seat.player_id === viewer}/>)}
```

- [ ] **Step 5: Add the CSS**

In `globals.css`, add near `.game-seat.is-turn:after` (~line 995/1601 — there are two rules, a base and a media-query
override; add the new class near the base one, honoring
`prefers-reduced-motion` the same way surrounding keyframes do — check the existing
`@media (prefers-reduced-motion: reduce)` block in this file and add `.seat-turn-ring` to its animation-disabling
selector list):

```css
.seat-turn-ring {
    position: absolute;
    inset: -4px;
    border-radius: inherit;
    border: 2px solid transparent;
    border-top-color: var(--accent, #fbbf24);
    animation: seat-turn-countdown linear forwards;
    pointer-events: none;
}

@keyframes seat-turn-countdown {
    from { transform: rotate(0deg); opacity: 1; }
    to { transform: rotate(360deg); opacity: .3; }
}
```

> Note for the implementer: this is a minimal placeholder ring (rotating border, not a true
> conic-gradient drain) — check `globals.css`'s existing color tokens (`--accent` may not be the
> right variable name; `grep -n "^\s*--" globals.css` to find the real token) and adjust to match
> the game's actual palette. If the design wants a true drain-to-empty conic-gradient ring instead
> of a rotating border, that is a design refinement on top of this same `key`-remount +
> `animationDuration` mechanism, not a different data flow.

- [ ] **Step 6: Mock adapter — synthesize a deadline for the `timeout` scenario**

`mock.ts` already has a `timeout` scenario (`MOCK_SCENARIOS` in `page.tsx` includes it). Check
`mock.ts` for where that scenario's snapshot is built and add
`action_deadline_unix_ms: Date.now() + 15000` to it so the ring is visually testable in
`USE_MOCK` mode without a live backend.

- [ ] **Step 7: Lint, build, manual verification**

```bash
cd ui && npx eslint src --max-warnings 0 && npx next build
```

```bash
cd ui && npm run dev
```

Open `?scenario=timeout` and confirm: the seat on the clock shows a shrinking/rotating ring that reaches empty at the
configured deadline; a real two-browser session confirms sitting-out (`Sentar fora`) marks the seat `Ausente` and
excludes it from the next hand, and `Voltar a jogar`
either returns immediately or (if that seat is about to be dealt SB/BB) the player is charged the big blind on the next
hand they're dealt into.

- [ ] **Step 8: Commit**

```bash
git add ui/src/lib/api/table.ts ui/src/app/table/page.tsx ui/src/components/table/Seat.tsx \
        ui/src/app/globals.css ui/src/lib/mock.ts
git commit -m "feat(ui): sitting-out toggle, per-turn countdown ring, drop manual first-ready click"
```

## Self-Review Notes

- **Spec coverage:** "entra pronto, pode sentar fora" → Task 1 + Task 6. "voltar custa BB se perto do próprio blind" →
  Task 2 (engine) + Task 3 (wiring). "timer 15s ajustável por sala, vale pra todo mundo" → Task 4 (config) + Task 5
  (engine/actor) + Task 6 (UI countdown).
- **Real bug found and fixed during design, not just requested feature:** running a brand-new 15s-for-everyone timer
  *alongside* the existing 30s disconnect-only timer would have silently broken the "3 consecutive disconnected hands →
  auto sit-out" escalation (the new timer would always fold first, so the old timer — and its
  `consecutiveDisconnectedHands` increment — would never fire again once a player is disconnected on their own turn).
  Task 5 unifies both into one timer specifically to avoid this; this is called out explicitly in Task 5's Architecture
  summary and Step 5's handler so a future reader doesn't "fix" it back apart.
- **Type/wire consistency:** `hand.Snapshot.ActionDeadlineUnixMs int64` (Task 5) →
  `TableSnapshot.action_deadline_unix_ms?: number` (Task 6) → `Seat`'s `deadlineMs` prop — same value, no
  transformation, matches the existing `current_player_id`/`current_player_id` pass-through pattern already in
  `page.tsx`.
- **No placeholders except one, explicitly flagged:** Task 6 Step 5's CSS ring is intentionally a minimal
  rotating-border placeholder, not a full conic-gradient drain — flagged inline as a design refinement point, not
  silently shipped as "done."
- **Out of scope, confirmed by the user, not touched by this plan:** Feature B (5s countdown before next hand), Feature
  C (voluntary card reveal + hand-type text + sounds), Feature D (hand-history persistence for audit). Bug "winner's
  cards shown on fold-to-one" was fixed separately, before this plan, per the user's confirmed ordering (bug3 → A → B →
  C → D).
