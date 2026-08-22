# Paid Rabbit Hunt Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:
> executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Charge a player the table's big blind to reveal the Rabbit Hunt runout (the cards that would have come after a
hand ends without showdown), instead of the current free version whose reveal data is already unconditionally on the
wire before any UI gate applies.

**Architecture:** `Table.ViewFor` already withholds hidden information per viewer (hole cards); this feature adds one
more condition to the one place it already gates the post-hand fairness reveal (`snapshot.go:349`), keyed on a new
per-hand `rabbitHuntPaid` map. A new `RequestRabbitHuntCmd`/`RabbitHuntVerifyFailedCmd` pair (mirroring the existing
`ShowCardsCmd`) debits/credits the big blind directly on the player's in-memory stack and flips that map, then
`broadcastAll()` re-derives every viewer's snapshot through the unchanged fan-out mechanism — the payer sees the reveal
fields on the next broadcast, nobody else does. The frontend's `RabbitHunt.tsx` keeps its existing local WebCrypto
verification untouched; it now waits for those fields to appear post-payment instead of finding them always present.

**Tech Stack:** Go (table actor / hand engine, `api/`), Next.js/React/TypeScript (`ui/`), binary protobuf wire
(`ui/src/lib/ws/utils.ts` + `lib/api/proto/poker.ts`, no proto schema change — new command types only).

**Spec:** `docs/specs/2026-08-21-paid-rabbit-hunt.md` — read it first, especially the "Correction" note in Scope (Rabbit
Hunt is not already absent from real-money tables; this plan closes that gap at the engine layer, not by assuming UI
absence).

## Global Constraints

- **Sandbox tables only.** `Table.currencyMode != "sandbox"` must reject `RequestRabbitHunt` before any other check —
  real money is closed at the engine layer (`hand.Table`), not assumed closed by the UI never showing the button.
- **Fixed price = the table's current big blind.** No dynamic pricing, no config surface.
- **No new protobuf fields.** The existing flat `ClientMessage`/`ServerMessage` envelopes already carry `type` (string)
  and `action_id`; only two new `type` string values are added, as comment-list entries in `proto/poker.proto:179` and
  `ui/src/lib/api/proto/poker.ts:243`.
- **No fee-collected counter.** Unlike rake, nothing reads it — cut per YAGNI, not deferred. A collected fee just leaves
  the stack.
- **Reuse the existing generic error path.** New commands rejected server-side surface as
  `{type: "error", code: "invalid_action", message: err.Error()}` — the same code every other handler in `tablews.go`
  already uses (`show_cards`, `keep_seat`, etc.). No new wire error codes, no bespoke per-component error UI in
  `RabbitHunt.tsx` — it flows through `useTableRealtime`'s existing `finishAuxiliaryCommand` → `actionError` →
  `ActionBar`'s error slot, exactly like a rejected `show_cards` today.
- **Genuine showdown reveals are never gated.** Only `wonWithoutShowdown == true` (the actual rabbit-hunt case) requires
  payment; a real showdown's fairness proof is owed to everyone unconditionally, unchanged.
- **Quality gates:** backend — `go build ./... && go test ./...` from `api/` for unit tests,
  `go test -tags integration ./...` for the actor-level tests (needs DynamoDB Local:
  `docker-compose -f docker-compose.test.yml up -d`, per `api/README.md`). Frontend — `npx vitest run`,
  `npx tsc --noEmit`, `npx eslint src --max-warnings 0`, `npm run build` from `ui/`, all zero-error/zero-warning,
  coverage thresholds (90% lines/functions/statements/branches) intact.
- **Documentation:** both `api/CLAUDE.md` and `ui/CLAUDE.md` mandate a doc update alongside any behavior/security change
  in the same commit — this plan's spec (already written) covers that; no additional doc file is created per task.

---

## Task 1: Engine — `currencyMode`, `rabbitHuntPaid`, `RequestRabbitHunt`/`RefundRabbitHunt`, and the reveal gate

**Files:**

- Modify: `api/internal/engine/hand/hand.go:107-156` (`Table` struct), `:285-297` (`ConfigureRake`), `:672-699`
  (`StartHand`'s per-hand reset block), add new methods after `:972-975` (`RevealHoleCards`)
- Modify: `api/internal/engine/hand/state.go:19-45` (`State` struct), `:48-76` (`ExportState`), `:81-144`
  (`NewTableFromState`)
- Modify: `api/internal/engine/hand/snapshot.go:348-354` (the reveal gate inside `ViewFor`)
- Test: `api/internal/engine/hand/hand_test.go`

**Interfaces:**

- Produces: `Table.RequestRabbitHunt(playerID string) (fee int64, err error)` — consumed by Task 2's
  `handleRequestRabbitHunt`.
- Produces: `Table.RefundRabbitHunt(playerID string) error` — consumed by Task 2's `handleRabbitHuntVerifyFailed`.
- Consumes: nothing new from other tasks.

- [x] **Step 1: Write the failing tests**

In `api/internal/engine/hand/hand_test.go`, add after `TestRevealHoleCardsRejectsPlayerNotDealtIntoTheHand` (line 846):

```go
func TestRequestRabbitHuntChargesBigBlindAndGatesViewFor(t *testing.T) {
p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
table := NewTable([]*Player{p1, p2}, 10, 20)
table.ConfigureRake("sandbox")
if err := table.StartHand(); err != nil {
t.Fatalf("StartHand: %v", err)
}
toAct := table.playerToActForTest()
winner, winnerID := p1, "p1"
if toAct == "p1" {
winner, winnerID = p2, "p2"
}
if err := table.Act(toAct, betting.ActionFold, 0); err != nil {
t.Fatalf("%s folds: %v", toAct, err)
}

before := winner.Stack
fee, err := table.RequestRabbitHunt(winnerID)
if err != nil {
t.Fatalf("RequestRabbitHunt: %v", err)
}
if fee != 20 {
t.Fatalf("expected the big blind (20) charged, got %d", fee)
}
if winner.Stack != before-20 {
t.Fatalf("expected stack debited by the fee, got %d want %d", winner.Stack, before-20)
}

paidView := table.ViewFor(winnerID)
if paidView.ShuffleServerSeedHex == "" {
t.Fatal("expected the payer's own view to reveal the shuffle seed")
}
unpaidView := table.ViewFor(toAct)
if unpaidView.ShuffleServerSeedHex != "" {
t.Fatal("expected a non-paying viewer's view to stay masked")
}
}

func TestRequestRabbitHuntRejectsDoublePayment(t *testing.T) {
p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
table := NewTable([]*Player{p1, p2}, 10, 20)
table.ConfigureRake("sandbox")
_ = table.StartHand()
toAct := table.playerToActForTest()
winnerID := "p1"
if toAct == "p1" {
winnerID = "p2"
}
_ = table.Act(toAct, betting.ActionFold, 0)

if _, err := table.RequestRabbitHunt(winnerID); err != nil {
t.Fatalf("first RequestRabbitHunt: %v", err)
}
if _, err := table.RequestRabbitHunt(winnerID); err == nil {
t.Fatal("expected the second RequestRabbitHunt this hand to be rejected")
}
}

func TestRequestRabbitHuntRejectsInsufficientStack(t *testing.T) {
p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
table := NewTable([]*Player{p1, p2}, 10, 20)
table.ConfigureRake("sandbox")
_ = table.StartHand()
toAct := table.playerToActForTest()
winnerID, winner := "p1", p1
if toAct == "p1" {
winnerID, winner = "p2", p2
}
winner.Stack = 10
_ = table.Act(toAct, betting.ActionFold, 0)

if _, err := table.RequestRabbitHunt(winnerID); err == nil {
t.Fatal("expected an insufficient-stack rejection")
}
if winner.Stack != 10 {
t.Fatalf("expected no charge on rejection, stack changed to %d", winner.Stack)
}
}

func TestRequestRabbitHuntRejectsRealMoneyTables(t *testing.T) {
p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
table := NewTable([]*Player{p1, p2}, 10, 20)
table.ConfigureRake("real")
_ = table.StartHand()
toAct := table.playerToActForTest()
winnerID := "p1"
if toAct == "p1" {
winnerID = "p2"
}
_ = table.Act(toAct, betting.ActionFold, 0)

if _, err := table.RequestRabbitHunt(winnerID); err == nil {
t.Fatal("expected a real-money table to reject the rabbit hunt request")
}
}

func TestRefundRabbitHuntCreditsBackAndRemasksView(t *testing.T) {
p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
table := NewTable([]*Player{p1, p2}, 10, 20)
table.ConfigureRake("sandbox")
_ = table.StartHand()
toAct := table.playerToActForTest()
winnerID, winner := "p1", p1
if toAct == "p1" {
winnerID, winner = "p2", p2
}
_ = table.Act(toAct, betting.ActionFold, 0)

before := winner.Stack
if _, err := table.RequestRabbitHunt(winnerID); err != nil {
t.Fatalf("RequestRabbitHunt: %v", err)
}
if err := table.RefundRabbitHunt(winnerID); err != nil {
t.Fatalf("RefundRabbitHunt: %v", err)
}
if winner.Stack != before {
t.Fatalf("expected the fee refunded, stack = %d want %d", winner.Stack, before)
}
if table.ViewFor(winnerID).ShuffleServerSeedHex != "" {
t.Fatal("expected the refunded viewer's view to be masked again")
}
if err := table.RefundRabbitHunt(winnerID); err == nil {
t.Fatal("expected a second refund with nothing paid to be rejected")
}
}

func TestGenuineShowdownRevealIsNeverGatedByRabbitHuntPayment(t *testing.T) {
p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
table := NewTable([]*Player{p1, p2}, 10, 20)
table.ConfigureRake("sandbox")
if err := table.StartHand(); err != nil {
t.Fatalf("StartHand: %v", err)
}
for table.Stage() != Complete {
toAct := table.playerToActForTest()
if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
_ = table.Act(toAct, betting.ActionCheck, 0)
}
}
outcome := table.LastOutcomeForActor()
if outcome == nil || outcome.WonWithoutShowdown {
t.Fatal("expected this hand to reach a genuine showdown")
}
if table.ViewFor("p1").ShuffleServerSeedHex == "" {
t.Fatal("expected the full seed published unconditionally after a genuine showdown, no payment needed")
}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `cd api && go test ./internal/engine/hand/... -run RabbitHunt -v` and
`go test ./internal/engine/hand/... -run TestGenuineShowdownReveal -v`
Expected: compile failure — `table.RequestRabbitHunt`/`table.RefundRabbitHunt`/`table.ConfigureRake("sandbox")`'s
currency gate don't exist yet.

- [x] **Step 3: Add `currencyMode` and `rabbitHuntPaid` to `Table`, wire `ConfigureRake` and `StartHand`**

In `api/internal/engine/hand/hand.go`, extend the `Table` struct (`:107-156`) right after the `rakeCollected` field:

```go
    payouts       map[string]int64
rakeBPS       int64
rakeCollected int64

// currencyMode is set once by ConfigureRake and never changes for the
// table's lifetime. It gates RequestRabbitHunt: a per-hand real-money
// debit for a curiosity feature would move real chips outside any wallet
// transaction, so real-money tables are closed at this layer, not
// assumed closed by the UI never showing the button.
currencyMode string

// rabbitHuntPaid tracks, for the current hand, which players have paid
// the big-blind fee to reveal the rabbit-hunt runout. Reset every hand
// alongside rakeCollected/seenActionIDs (see StartHand).
rabbitHuntPaid map[string]bool
```

Update `ConfigureRake` (`:285-297`) to persist the mode:

```go
// ConfigureRake enables the standard 2.5% sandbox rake. Real-money tables
// are always rake-free — Brazilian law treats a cut of the pot/blind on a
// public real-money game as a bet requiring SPA authorization; poker's
// real-money revenue comes entirely from the fixed table-entry fee charged
// at buy-in instead (buyin.Service.BuyIn), never from the pot. The setting
// is persisted with the table state.
func (t *Table) ConfigureRake(currencyMode string) {
t.currencyMode = currencyMode
if currencyMode == "sandbox" {
t.rakeBPS = 250
return
}
t.rakeBPS = 0
}
```

Add the reset in `StartHand`'s per-hand block (`:692`), right after `t.rakeCollected = 0`:

```go
    t.rakeCollected = 0
t.rabbitHuntPaid = make(map[string]bool)
```

- [x] **Step 4: Add `RequestRabbitHunt`/`RefundRabbitHunt`**

In `api/internal/engine/hand/hand.go`, add after `RevealHoleCards` (`:972-975`):

```go
// RequestRabbitHunt charges playerID the current hand's big blind to reveal
// the runout that would have come after a hand ends without a showdown.
// Returns the fee charged. Fails without charging anything if the table
// isn't sandbox, the hand isn't eligible, the player wasn't dealt in, they
// already paid this hand, or their stack can't cover the fee.
func (t *Table) RequestRabbitHunt(playerID string) (fee int64, err error) {
if t.currencyMode != "sandbox" {
return 0, fmt.Errorf("hand: rabbit hunt is only available on sandbox tables")
}
if t.stage != Complete {
return 0, fmt.Errorf("hand: rabbit hunt is only available after the hand is complete")
}
if t.lastOutcome == nil || !t.lastOutcome.WonWithoutShowdown {
return 0, fmt.Errorf("hand: rabbit hunt is only available when the hand ended without a showdown")
}
if len(t.board) >= 5 {
return 0, fmt.Errorf("hand: rabbit hunt is not available once the full board is dealt")
}
dealtIn := false
for _, hp := range t.handOrder {
if hp.ID == playerID {
dealtIn = true
break
}
}
if !dealtIn {
return 0, fmt.Errorf("hand: player %s was not dealt into this hand", playerID)
}
if t.rabbitHuntPaid[playerID] {
return 0, fmt.Errorf("hand: player %s already paid for rabbit hunt this hand", playerID)
}
p := t.playerByID(playerID)
if p == nil {
return 0, fmt.Errorf("hand: player %s is no longer seated", playerID)
}
if p.Stack < t.bigBlind {
return 0, fmt.Errorf("hand: insufficient stack for the rabbit hunt fee")
}
p.Stack -= t.bigBlind
if t.rabbitHuntPaid == nil {
t.rabbitHuntPaid = make(map[string]bool)
}
t.rabbitHuntPaid[playerID] = true
return t.bigBlind, nil
}

// RefundRabbitHunt reverses a RequestRabbitHunt charge for playerID this
// hand, used when the client reports it couldn't verify the revealed
// runout. Fails if playerID never paid this hand (nothing to refund).
func (t *Table) RefundRabbitHunt(playerID string) error {
if !t.rabbitHuntPaid[playerID] {
return fmt.Errorf("hand: player %s has no rabbit hunt payment to refund this hand", playerID)
}
p := t.playerByID(playerID)
if p == nil {
return fmt.Errorf("hand: player %s is no longer seated", playerID)
}
p.Stack += t.bigBlind
delete(t.rabbitHuntPaid, playerID)
return nil
}
```

- [x] **Step 5: Add the reveal gate in `ViewFor`**

In `api/internal/engine/hand/snapshot.go`, change (`:348-354`):

```go
        if t.stage == Complete {
proof, runout := t.fairnessProofFor(viewerID, wonWithoutShowdown)
out.ShuffleServerSeedHex = proof.ServerSeedHex
out.RevealedCardSalts = proof.RevealedCardSalts
out.UnrevealedCardHashes = proof.UnrevealedCardHashes
out.RunoutCards = runout
}
```

to:

```go
        if t.stage == Complete && (!wonWithoutShowdown || t.rabbitHuntPaid[viewerID]) {
proof, runout := t.fairnessProofFor(viewerID, wonWithoutShowdown)
out.ShuffleServerSeedHex = proof.ServerSeedHex
out.RevealedCardSalts = proof.RevealedCardSalts
out.UnrevealedCardHashes = proof.UnrevealedCardHashes
out.RunoutCards = runout
}
```

- [x] **Step 6: Persist the new fields in `State`**

In `api/internal/engine/hand/state.go`, add to the `State` struct (`:19-45`) after `RakeCollected`:

```go
    RakeBPS       int64
RakeCollected int64
CurrencyMode  string
HandOrder     []*Player
SeenActionIDs map[string]bool
ReadyToPost   map[string]bool
OwesBigBlind  map[string]bool
LastOutcome   *HandOutcome
WasEverAllIn  map[string]bool
RabbitHuntPaid map[string]bool
```

In `ExportState` (`:48-76`), add:

```go
        RakeBPS:       t.rakeBPS,
RakeCollected: t.rakeCollected,
CurrencyMode:  t.currencyMode,
HandOrder:     t.handOrder,
SeenActionIDs: t.seenActionIDs,
ReadyToPost:   t.readyToPost,
OwesBigBlind:  t.owesBigBlind,
LastOutcome:   t.lastOutcome,
WasEverAllIn:  t.wasEverAllIn,
RabbitHuntPaid: t.rabbitHuntPaid,
```

In `NewTableFromState` (`:81-144`), add:

```go
        rakeBPS:       s.RakeBPS,
rakeCollected: s.RakeCollected,
currencyMode:  s.CurrencyMode,
handOrder:     handOrder,
seenActionIDs: s.SeenActionIDs,
readyToPost:   s.ReadyToPost,
owesBigBlind:  s.OwesBigBlind,
lastOutcome:   s.LastOutcome,
wasEverAllIn:  s.WasEverAllIn,
rabbitHuntPaid: s.RabbitHuntPaid,
```

- [x] **Step 7: Run the tests to verify they pass**

Run: `cd api && go build ./... && go test ./internal/engine/hand/... -run 'RabbitHunt|GenuineShowdown' -v`
Expected: all six new tests PASS. Also run the full package (`go test ./internal/engine/hand/...`) to confirm nothing
existing broke — `ConfigureRake` and `StartHand`'s reset block are shared by every other test in the file.

- [x] **Step 8: Commit**

```bash
git add api/internal/engine/hand/hand.go api/internal/engine/hand/state.go api/internal/engine/hand/snapshot.go api/internal/engine/hand/hand_test.go
git commit -m "feat(engine): charge and gate rabbit hunt reveal behind payment"
```

---

## Task 2: Actor + WS wiring — `RequestRabbitHuntCmd`/`RabbitHuntVerifyFailedCmd`

**Files:**

- Modify: `api/internal/table/commands.go:103-118` (add the two new command structs near `ShowCardsCmd`)
- Modify: `api/internal/table/actor.go:242-243` (dispatch switch), add handlers after `handleShowCards` (`:1367-1408`)
- Modify: `api/internal/api/v1/tablews.go:585-594` (add the two new `case` branches near `"show_cards"`)
- Modify: `proto/poker.proto:179` (comment-list update only, no schema change)
- Test: `api/internal/table/actor_test.go` (build tag `integration`)

**Interfaces:**

- Consumes: `Table.RequestRabbitHunt(playerID string) (int64, error)`, `Table.RefundRabbitHunt(playerID string) error`
  (Task 1).
- Produces: `table.RequestRabbitHuntCmd{PlayerID, ActionID, Reply}`,
  `table.RabbitHuntVerifyFailedCmd{PlayerID, ActionID, Reply}` — consumed by Task 3's `useTableRealtime.ts` via the
  `"request_rabbit_hunt"`/`"rabbit_hunt_verify_failed"` wire `type` strings.

- [x] **Step 1: Write the failing integration tests**

In `api/internal/table/actor_test.go`, add after `TestShowCardsCmdRevealsFoldedWinnerToEveryone` (line 272):

```go
func TestRequestRabbitHuntCmdRevealsOnlyToPayer(t *testing.T) {
db := testClient(t)
store := tablestore.NewStore(db, "table_test")
mustCreateTestTables(t, db, "table_test")
a, tableID := newTestActor(t, store)
ctx := context.Background()

_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)})
_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: make(chan error, 1)})

stored, _ := store.LoadTable(ctx, tableID)
toAct := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
winnerID := "p1"
if toAct == "p1" {
winnerID = "p2"
}
if err := a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: make(chan error, 1)}); err != nil {
t.Fatalf("fold: %v", err)
}

if err := a.Dispatch(RequestRabbitHuntCmd{PlayerID: winnerID, ActionID: "a2", Reply: make(chan error, 1)}); err != nil {
t.Fatalf("RequestRabbitHuntCmd: %v", err)
}

stored, _ = store.LoadTable(ctx, tableID)
table := hand.NewTableFromState(stored.State)
if table.ViewFor(winnerID).ShuffleServerSeedHex == "" {
t.Fatal("expected the payer's own view to reveal the shuffle seed")
}
if table.ViewFor(toAct).ShuffleServerSeedHex != "" {
t.Fatal("expected the non-paying viewer's view to stay masked")
}
}

func TestRequestRabbitHuntCmdRejectsDoublePaymentSameHand(t *testing.T) {
db := testClient(t)
store := tablestore.NewStore(db, "table_test")
mustCreateTestTables(t, db, "table_test")
a, tableID := newTestActor(t, store)
ctx := context.Background()

_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)})
_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: make(chan error, 1)})
stored, _ := store.LoadTable(ctx, tableID)
toAct := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
winnerID := "p1"
if toAct == "p1" {
winnerID = "p2"
}
_ = a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: make(chan error, 1)})

if err := a.Dispatch(RequestRabbitHuntCmd{PlayerID: winnerID, ActionID: "a2", Reply: make(chan error, 1)}); err != nil {
t.Fatalf("first RequestRabbitHuntCmd: %v", err)
}
if err := a.Dispatch(RequestRabbitHuntCmd{PlayerID: winnerID, ActionID: "a3", Reply: make(chan error, 1)}); err == nil {
t.Fatal("expected the second, distinctly-actioned request this hand to be rejected")
}
}

func TestRabbitHuntVerifyFailedCmdRefundsAndRemasks(t *testing.T) {
db := testClient(t)
store := tablestore.NewStore(db, "table_test")
mustCreateTestTables(t, db, "table_test")
a, tableID := newTestActor(t, store)
ctx := context.Background()

_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)})
_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: make(chan error, 1)})
stored, _ := store.LoadTable(ctx, tableID)
toAct := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
winnerID := "p1"
if toAct == "p1" {
winnerID = "p2"
}
_ = a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: make(chan error, 1)})
if err := a.Dispatch(RequestRabbitHuntCmd{PlayerID: winnerID, ActionID: "a2", Reply: make(chan error, 1)}); err != nil {
t.Fatalf("RequestRabbitHuntCmd: %v", err)
}

stored, _ = store.LoadTable(ctx, tableID)
var chargedStack int64
for _, s := range hand.NewTableFromState(stored.State).ViewFor(winnerID).Seats {
if s.PlayerID == winnerID {
chargedStack = s.Stack
}
}

if err := a.Dispatch(RabbitHuntVerifyFailedCmd{PlayerID: winnerID, ActionID: "a3", Reply: make(chan error, 1)}); err != nil {
t.Fatalf("RabbitHuntVerifyFailedCmd: %v", err)
}

stored, _ = store.LoadTable(ctx, tableID)
table := hand.NewTableFromState(stored.State)
view := table.ViewFor(winnerID)
for _, s := range view.Seats {
if s.PlayerID == winnerID && s.Stack != chargedStack+20 {
t.Fatalf("expected the fee refunded, stack = %d want %d", s.Stack, chargedStack+20)
}
}
if view.ShuffleServerSeedHex != "" {
t.Fatal("expected the refunded viewer's view to be masked again")
}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Prereq (once per session): `docker-compose -f docker-compose.test.yml up -d` from `api/`. Run:
`cd api && go test -tags integration ./internal/table/... -run RabbitHunt -v`
Expected: compile failure — `RequestRabbitHuntCmd`/`RabbitHuntVerifyFailedCmd` don't exist yet.

- [x] **Step 3: Add the command structs**

In `api/internal/table/commands.go`, add after `ShowCardsCmd` (`:103-108`, before `SetRunItTwiceCmd`):

```go
type RequestRabbitHuntCmd struct {
PlayerID string
ActionID string
Reply    chan error
}

func (c RequestRabbitHuntCmd) reply() chan error { return c.Reply }

type RabbitHuntVerifyFailedCmd struct {
PlayerID string
ActionID string
Reply    chan error
}

func (c RabbitHuntVerifyFailedCmd) reply() chan error { return c.Reply }
```

- [x] **Step 4: Add the actor handlers and dispatch cases**

In `api/internal/table/actor.go`, add two cases to the dispatch switch right after `case ShowCardsCmd:` (`:242-243`):

```go
    case ShowCardsCmd:
return a.handleShowCards(ctx, c)
case RequestRabbitHuntCmd:
return a.handleRequestRabbitHunt(ctx, c)
case RabbitHuntVerifyFailedCmd:
return a.handleRabbitHuntVerifyFailed(ctx, c)
```

Add the two handlers after `handleShowCards` (`:1367-1408`):

```go
func (a *Actor) handleRequestRabbitHunt(ctx context.Context, c RequestRabbitHuntCmd) error {
if err := a.ensureLoaded(ctx, false); err != nil {
return err
}
changed := false
apply := func () error {
if _, err := a.cached.RequestRabbitHunt(c.PlayerID); err != nil {
return err
}
changed = true
return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "request_rabbit_hunt",
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

func (a *Actor) handleRabbitHuntVerifyFailed(ctx context.Context, c RabbitHuntVerifyFailedCmd) error {
if err := a.ensureLoaded(ctx, false); err != nil {
return err
}
changed := false
apply := func () error {
if err := a.cached.RefundRabbitHunt(c.PlayerID); err != nil {
return err
}
changed = true
return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "rabbit_hunt_verify_failed",
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

- [x] **Step 5: Wire the WS gateway cases**

In `api/internal/api/v1/tablews.go`, add two cases right after `case "show_cards":` (`:585-594`):

```go
                case "request_rabbit_hunt":
ensureActionID()
r := make(chan error, 1)
if err := dispatch(table.RequestRabbitHuntCmd{PlayerID: playerID, ActionID: m.ActionId, Reply: r}); err != nil {
send(&pokerproto.ServerMessage{Type: "error", Code: "invalid_action", Message: err.Error(), ActionId: m.ActionId})
} else {
ack()
}
case "rabbit_hunt_verify_failed":
ensureActionID()
r := make(chan error, 1)
if err := dispatch(table.RabbitHuntVerifyFailedCmd{PlayerID: playerID, ActionID: m.ActionId, Reply: r}); err != nil {
send(&pokerproto.ServerMessage{Type: "error", Code: "invalid_action", Message: err.Error(), ActionId: m.ActionId})
} else {
ack()
}
```

- [x] **Step 6: Update the proto comment list**

In `proto/poker.proto:179`, extend the `type` field's doc comment:

```proto
  string type = 1; // "auth" | "ping" | "sync_state" | "ready" | "act" | "preselect_action" | "post_big_blind" | "show_cards" | "keep_seat" | "chat" | "reaction" | "bot_challenge" | "set_run_it_twice" | "peek_cards" | "request_rabbit_hunt" | "rabbit_hunt_verify_failed"
```

No `protoc` regeneration needed — this is a comment-only change, no new field.

- [x] **Step 7: Run the tests to verify they pass**

Run: `cd api && go build ./... && go test -tags integration ./internal/table/... -run RabbitHunt -v`
Expected: all three new tests PASS.

- [x] **Step 8: Commit**

```bash
git add api/internal/table/commands.go api/internal/table/actor.go api/internal/table/actor_test.go api/internal/api/v1/tablews.go proto/poker.proto
git commit -m "feat(table): dispatch paid rabbit hunt request and refund commands"
```

---

## Task 3: Frontend realtime hook — `requestRabbitHunt`/`reportRabbitHuntVerifyFailed`

**Files:**

- Modify: `ui/src/lib/hooks/useTableRealtime.ts` (multiple sites, listed in Step 3)
- Modify: `ui/src/lib/api/proto/poker.ts:243` (comment-list update only)
- Test: `ui/src/lib/hooks/useTableRealtime.test.tsx`

**Interfaces:**

- Produces: `requestRabbitHunt(): boolean`, `requestRabbitHuntPending: boolean`,
  `reportRabbitHuntVerifyFailed(): boolean` on the hook's return object — consumed by Task 5's `page.tsx` wiring.
- Consumes: the `"request_rabbit_hunt"`/`"rabbit_hunt_verify_failed"` wire `type` strings (Task 2).

- [x] **Step 1: Write the failing tests**

In `ui/src/lib/hooks/useTableRealtime.test.tsx`, add after the
`'locks ready and card reveal commands until acknowledgement'` test (ends line 349):

```tsx
  test('locks the rabbit hunt request until acknowledgement and surfaces a rejection', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));

    act(() => {
        expect(result.current.requestRabbitHunt()).toBe(true);
        expect(result.current.requestRabbitHunt()).toBe(false);
    });
    expect(result.current.requestRabbitHuntPending).toBe(true);
    expect(ws.send).toHaveBeenLastCalledWith({type: 'request_rabbit_hunt', action_id: 'action-1'});

    receive({type: 'error', code: 'invalid_action', action_id: 'action-1'});
    expect(result.current.requestRabbitHuntPending).toBe(false);
    expect(result.current.actionError).toMatchObject({code: 'invalid_action'});
});

test('acknowledges a successful rabbit hunt request and unlocks it', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    act(() => {
        expect(result.current.requestRabbitHunt()).toBe(true);
    });
    receive({type: 'action_ack', action_id: 'action-1'});
    expect(result.current.requestRabbitHuntPending).toBe(false);
});

test('reports a rabbit hunt verification failure as a fire-and-forget frame', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    act(() => {
        expect(result.current.reportRabbitHuntVerifyFailed()).toBe(true);
    });
    expect(ws.send).toHaveBeenLastCalledWith({type: 'rabbit_hunt_verify_failed', action_id: 'action-1'});
});
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `cd ui && npx vitest run src/lib/hooks/useTableRealtime.test.tsx`
Expected: FAIL — `result.current.requestRabbitHunt is not a function`.

- [x] **Step 3: Implement the hook additions**

In `ui/src/lib/hooks/useTableRealtime.ts`:

Declare the new refs/state right after the `showCards` ones (`:228-234`):

```ts
  const readyLockRef = useRef(false);
const showCardsLockRef = useRef(false);
const requestRabbitHuntLockRef = useRef(false);
const readyActionRef = useRef<string | null>(null);
const showCardsActionRef = useRef<string | null>(null);
const requestRabbitHuntActionRef = useRef<string | null>(null);
const readyTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
const showCardsTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
const requestRabbitHuntTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
const [readyPending, setReadyPending] = useState(false);
const [showCardsPending, setShowCardsPending] = useState(false);
const [requestRabbitHuntPending, setRequestRabbitHuntPending] = useState(false);
```

Register it in `finishAuxiliaryCommand` (`:306-325`), right after the `showCardsActionRef` block:

```ts
    if (showCardsActionRef.current === actionId) {
    if (showCardsTimerRef.current) clearTimeout(showCardsTimerRef.current);
    showCardsActionRef.current = null;
    showCardsLockRef.current = false;
    setShowCardsPending(false);
}
if (requestRabbitHuntActionRef.current === actionId) {
    if (requestRabbitHuntTimerRef.current) clearTimeout(requestRabbitHuntTimerRef.current);
    requestRabbitHuntActionRef.current = null;
    requestRabbitHuntLockRef.current = false;
    setRequestRabbitHuntPending(false);
}
```

Add it to the legacy-unversioned resync list (`:450`):

```ts
        for (const id of [readyActionRef.current, showCardsActionRef.current, postBigBlindActionRef.current,
    requestRabbitHuntActionRef.current]) {
    if (id) finishAuxiliaryCommand(id);
}
```

Add it to the mock "connecting" reset list (`:651`):

```ts
            for (const actionId of [readyActionRef.current, showCardsActionRef.current, postBigBlindActionRef.current,
    requestRabbitHuntActionRef.current]) {
    if (actionId) finishAuxiliaryCommand(actionId);
}
```

Add it to the `handleOpen` reset (`:597`), right after `setShowCardsPending(false);`:

```ts
      setShowCardsPending(false);
setRequestRabbitHuntPending(false);
```

Add it to the table-switch reset effect (`:695-706`):

```ts
    readyActionRef.current = null;
showCardsActionRef.current = null;
requestRabbitHuntActionRef.current = null;
readyLockRef.current = false;
showCardsLockRef.current = false;
requestRabbitHuntLockRef.current = false;
for (const timer of [pendingTimer.current, readyTimerRef.current, showCardsTimerRef.current,
    postBigBlindTimerRef.current, requestRabbitHuntTimerRef.current]) {
    if (timer) clearTimeout(timer);
}
pendingTimer.current = undefined;
readyTimerRef.current = undefined;
showCardsTimerRef.current = undefined;
postBigBlindTimerRef.current = undefined;
requestRabbitHuntTimerRef.current = undefined;
```

Add it to the unmount cleanup effect (`:715-719`):

```ts
    if (pendingTimer.current) clearTimeout(pendingTimer.current);
if (readyTimerRef.current) clearTimeout(readyTimerRef.current);
if (showCardsTimerRef.current) clearTimeout(showCardsTimerRef.current);
if (postBigBlindTimerRef.current) clearTimeout(postBigBlindTimerRef.current);
if (requestRabbitHuntTimerRef.current) clearTimeout(requestRabbitHuntTimerRef.current);
```

Add the two functions and the pending flag to the hook's returned object, right after `showCards` (`:812-825`):

```ts
    showCardsPending,
    requestRabbitHuntPending,
```

```ts
    showCards: (cardIndex?: number) => {
    if (showCardsLockRef.current) return false;
    const actionId = crypto.randomUUID();
    showCardsLockRef.current = true;
    showCardsActionRef.current = actionId;
    setShowCardsPending(true);
    const ok = emit({type: 'show_cards', action_id: actionId, card_index: cardIndex});
    if (!ok) {
        finishAuxiliaryCommand(actionId);
        return false;
    }
    showCardsTimerRef.current = setTimeout(() => finishAuxiliaryCommand(actionId, 'action_timeout'), ACTION_TIMEOUT_MS);
    return ok;
},
    requestRabbitHunt
:
() => {
    if (requestRabbitHuntLockRef.current) return false;
    const actionId = crypto.randomUUID();
    requestRabbitHuntLockRef.current = true;
    requestRabbitHuntActionRef.current = actionId;
    setRequestRabbitHuntPending(true);
    const ok = emit({type: 'request_rabbit_hunt', action_id: actionId});
    if (!ok) {
        finishAuxiliaryCommand(actionId);
        return false;
    }
    requestRabbitHuntTimerRef.current = setTimeout(() => finishAuxiliaryCommand(actionId, 'action_timeout'), ACTION_TIMEOUT_MS);
    return ok;
},
    reportRabbitHuntVerifyFailed
:
() => emit({type: 'rabbit_hunt_verify_failed', action_id: crypto.randomUUID()}),
```

(`reportRabbitHuntVerifyFailed` is fire-and-forget like `keepSeat` right below it — nothing in the UI waits on its ack,
since `RabbitHunt.tsx` already shows its own "taxa devolvida" message locally the moment verification fails, independent
of the server's response.)

- [x] **Step 4: Update the proto comment list**

In `ui/src/lib/api/proto/poker.ts:243`, mirror the same comment update as Task 2 Step 6:

```ts
  /** "auth" | "ping" | "sync_state" | "ready" | "act" | "preselect_action" | "post_big_blind" | "show_cards" | "keep_seat" | "chat" | "reaction" | "bot_challenge" | "set_run_it_twice" | "peek_cards" | "request_rabbit_hunt" | "rabbit_hunt_verify_failed" */
```

- [x] **Step 5: Run the tests to verify they pass**

Run: `cd ui && npx vitest run src/lib/hooks/useTableRealtime.test.tsx`
Expected: all new tests PASS, and no existing test in the file regressed (the shared lists/effects touched in Step 3 are
exercised by many existing tests in this file).

- [x] **Step 6: Commit**

```bash
git add ui/src/lib/hooks/useTableRealtime.ts ui/src/lib/hooks/useTableRealtime.test.tsx ui/src/lib/api/proto/poker.ts
git commit -m "feat(realtime): add requestRabbitHunt/reportRabbitHuntVerifyFailed to the table hook"
```

---

## Task 4: `RabbitHunt.tsx` — price, request-then-verify flow, refund message

**Files:**

- Modify: `ui/src/components/table/RabbitHunt.tsx`
- Test: `ui/src/components/table/RabbitHunt.test.tsx`

**Interfaces:**

- Consumes: `bigBlind: number`, `pending?: boolean`, `onRequestRabbitHuntAction?: () => void`,
  `onRabbitHuntVerifyFailedAction?: () => void` — new props, wired by Task 5.
- Produces: nothing new consumed elsewhere — this is a leaf component.

- [x] **Step 1: Write the failing tests**

In `ui/src/components/table/RabbitHunt.test.tsx`, replace the two existing button-name matchers (`/Ver o que viria/`,
used at lines 58, 70, 84, 101) with `/Ver por 50 fichas/` (the mock `snapshot()` in this file has no `big_blind` field
today — it's not part of `TableSnapshot`; `bigBlind` will now be a separate required prop on `RabbitHunt`, so every
`render(<RabbitHunt .../>)` call in this file needs `bigBlind={50}` added). Concretely:

Change every `render(<RabbitHunt snapshot={...} viewer="viewer"/>)` call in the file to
`render(<RabbitHunt snapshot={...} viewer="viewer" bigBlind={50}/>)`, and every
`screen.getByRole('button', {name: /Ver o que viria/})` to `screen.getByRole('button', {name: /Ver por 50 fichas/})`.

Then add two new tests at the end of the `describe('RabbitHunt', ...)` block:

```tsx
  test('requests payment on click and reports a verification failure for a refund', async () => {
    const onRequest = vi.fn();
    const onVerifyFailed = vi.fn();
    verifyDeck.mockResolvedValue({deck: ['deck'], matches: false});
    render(<RabbitHunt snapshot={snapshot()} viewer="viewer" bigBlind={50}
                       onRequestRabbitHuntAction={onRequest} onRabbitHuntVerifyFailedAction={onVerifyFailed}/>);
    await userEvent.click(screen.getByRole('button', {name: /Ver por 50 fichas/}));
    expect(onRequest).toHaveBeenCalledOnce();
    expect(await screen.findByText('Não foi possível verificar o runout. Taxa devolvida.')).toBeInTheDocument();
    expect(onVerifyFailed).toHaveBeenCalledOnce();
});

test('disables the button while the payment is pending', () => {
    render(<RabbitHunt snapshot={snapshot()} viewer="viewer" bigBlind={50} pending/>);
    expect(screen.getByRole('button', {name: /Ver por 50 fichas/})).toBeDisabled();
});
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `cd ui && npx vitest run src/components/table/RabbitHunt.test.tsx`
Expected: FAIL — `bigBlind` prop not accepted by the type, button text still says "Ver o que viria", no `pending`/
`onRequestRabbitHuntAction`/`onRabbitHuntVerifyFailedAction` props exist.

- [x] **Step 3: Implement the component changes**

In `ui/src/components/table/RabbitHunt.tsx`, change the export signature:

```tsx
export function RabbitHunt({
                               snapshot,
                               viewer,
                               bigBlind,
                               pending,
                               onRequestRabbitHuntAction,
                               onRabbitHuntVerifyFailedAction
                           }: {
    snapshot: TableSnapshot;
    viewer?: string;
    bigBlind: number;
    pending?: boolean;
    onRequestRabbitHuntAction?: () => void;
    onRabbitHuntVerifyFailedAction?: () => void;
}) {
```

Change the `catch` branch of the verification effect (currently
`void load().catch(() => { if (live) setVerificationFailed(true); });`) to also report the failure:

```tsx
    void load().catch(() => {
    if (live) {
        setVerificationFailed(true);
        onRabbitHuntVerifyFailedAction?.();
    }
});
```

Change the button and its label:

```tsx
    {
    !requested ? <button type="button" disabled={pending} onClick={() => {
        setRequested(true);
        onRequestRabbitHuntAction?.();
    }}>
        <Rabbit aria-hidden="true"/>
        <span><b>{`Ver por ${bigBlind.toLocaleString('pt-BR')} fichas`}</b><small>Rabbit hunting · não altera o resultado</small></span>
    </button> : cards.length ? <>
```

Change the verification-failure message:

```tsx
      :
verificationFailed
    ? <span className="rabbit-hunt-label">Não foi possível verificar o runout. Taxa devolvida.</span>
    : <span className="rabbit-hunt-label">Verificando o baralho…</span>
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `cd ui && npx vitest run src/components/table/RabbitHunt.test.tsx`
Expected: all tests PASS, including every pre-existing test updated in Step 1.

- [x] **Step 5: Commit**

```bash
git add ui/src/components/table/RabbitHunt.tsx ui/src/components/table/RabbitHunt.test.tsx
git commit -m "feat(table): charge for rabbit hunt reveal and report verification failures for a refund"
```

---

## Task 5: Prop threading — `TableStage.tsx` and `page.tsx`

**Files:**

- Modify: `ui/src/components/table/TableStage.tsx:69-100` (`Props` type), `:102-125` (destructure), `:179`, `:195` (both
  `<RabbitHunt>` call sites)
- Modify: `ui/src/app/table/page.tsx:546-554` area (the `<TableStage>` call site)
- Test: `ui/src/app/table/page.test.tsx`

**Interfaces:**

- Consumes: `requestRabbitHunt()`, `requestRabbitHuntPending`, `reportRabbitHuntVerifyFailed()` from `useTableRealtime`
  (Task 3).
- Produces: nothing further downstream.

- [x] **Step 1: Write the failing test**

In `ui/src/app/table/page.test.tsx`, add
`requestRabbitHunt: vi.fn(() => true), requestRabbitHuntPending: false, reportRabbitHuntVerifyFailed: vi.fn(() => true),`
to the `realtime()` helper's default mock object (`:179-186`, alongside the existing `showCards`/`showCardsPending`
entries).

Then add a new test after `'offers card reveal only for a participating seat still holding a hidden card'` (ends line
490):

```tsx
  test('wires the rabbit hunt price and request/refund callbacks into TableStage', async () => {
    realtime({snapshot: snapshot({stage: 'complete', won_without_showdown: true})});
    render(<TablePage/>);
    expect(mocks.stageProps?.bigBlind).toBeGreaterThan(0);
    act(() => (mocks.stageProps?.onRequestRabbitHuntAction as () => void)());
    expect(mocks.realtime.requestRabbitHunt).toHaveBeenCalledOnce();
    act(() => (mocks.stageProps?.onRabbitHuntVerifyFailedAction as () => void)());
    expect(mocks.realtime.reportRabbitHuntVerifyFailed).toHaveBeenCalledOnce();
    expect(mocks.stageProps?.rabbitHuntPending).toBe(false);
});
```

- [x] **Step 2: Run the test to verify it fails**

Run: `cd ui && npx vitest run src/app/table/page.test.tsx -t "rabbit hunt"`
Expected: FAIL — `mocks.stageProps?.onRequestRabbitHuntAction` is `undefined`, not a function.

- [x] **Step 3: Thread the props through `TableStage`**

In `ui/src/components/table/TableStage.tsx`, add to the `Props` type (`:69-100`) right after `onPeekCardsAction`:

```ts
  onPeekCardsAction ? : () => void;
rabbitHuntPending ? : boolean;
onRequestRabbitHuntAction ? : () => void;
onRabbitHuntVerifyFailedAction ? : () => void;
```

Add to the destructure (`:102-125`) right after `onPeekCardsAction`:

```ts
                             onPeekCardsAction,
    rabbitHuntPending,
    onRequestRabbitHuntAction,
    onRabbitHuntVerifyFailedAction,
```

Update both `<RabbitHunt>` call sites (`:179` and `:195`):

```tsx
      <RabbitHunt key={snapshot.hand_id} snapshot={snapshot} viewer={viewer} bigBlind={bigBlind}
                  pending={rabbitHuntPending} onRequestRabbitHuntAction={onRequestRabbitHuntAction}
                  onRabbitHuntVerifyFailedAction={onRabbitHuntVerifyFailedAction}/>
```

- [x] **Step 4: Wire it from `page.tsx`**

In `ui/src/app/table/page.tsx`, add three props to the `<TableStage>` call (`:552-554` area), right after
`onPeekCardsAction={rt.peekCards}`:

```tsx
                  onPeekCardsAction = {rt.peekCards}
rabbitHuntPending = {rt.requestRabbitHuntPending}
onRequestRabbitHuntAction = {rt.requestRabbitHunt}
onRabbitHuntVerifyFailedAction = {rt.reportRabbitHuntVerifyFailed}
```

- [x] **Step 5: Run the test to verify it passes**

Run: `cd ui && npx vitest run src/app/table/page.test.tsx`
Expected: the new test PASSES; no other test in the file regresses (the `realtime()` helper default object change in
Step 1 affects every test that doesn't override those keys, but the new keys are additive — no existing assertion reads
them).

- [x] **Step 6: Full quality gate**

Run from `ui/`: `npx vitest run && npx tsc --noEmit && npx eslint src --max-warnings 0 && npm run build`
Expected: all green, zero errors/warnings, coverage thresholds intact.

Run from `api/`: `go build ./... && go test ./... && go test -tags integration ./internal/table/... -run RabbitHunt`
Expected: all green.

- [x] **Step 7: Commit**

```bash
git add ui/src/components/table/TableStage.tsx ui/src/app/table/page.tsx ui/src/app/table/page.test.tsx
git commit -m "feat(table): wire the paid rabbit hunt request/refund flow into the table page"
```
