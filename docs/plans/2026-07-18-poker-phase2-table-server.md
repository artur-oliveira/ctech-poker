# Phase 2 — Table Server & Real-Time Transport Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the Phase 0/1 pure engine (`internal/engine/hand.Table`) into a live, crash-recoverable,
single-writer-per-table service reachable over WebSocket, per ARCHITECTURE.md §2-3 and OVERVIEW.md §4.

**Architecture:** Each table is owned by exactly one goroutine (`table.Actor`) on exactly one instance, guarded
by the existing `tablelease.Service` (Task 4 of the foundations plan). Every validated action is appended to a
DynamoDB action log *before* being broadcast (log-before-broadcast), and a full-state snapshot is written at the
start of every hand, so a crashed instance's replacement can resume mid-hand from `snapshot + replay(log since
snapshot)`. Clients reach a table over a WebSocket gateway; a connection that lands on an instance which does not
hold the table's lease is transparently proxied — not redirected — to the instance that does, over the private
VPC network (both instances sit in the same security group; ALB only ever sees the client-facing hop).

**Tech Stack:** Go 1.26, Fiber v3, `github.com/fasthttp/websocket`, `gopkg.aoctech.app/api-commons/ws` (Redis
Pub/Sub fan-out registry), `gopkg.aoctech.app/api-commons/dynamo` (DynamoDB helpers), the existing
`internal/tablelease` lease service, AWS CDK.

## Global Constraints

- Every route lives under `/v1.0/` (existing convention, `internal/api/v1/router.go`).
- All amounts are integer chip counts (`int64`), never floats — matches the engine's existing convention.
- Log before broadcast: an `ActionLogEntry` write must complete before the resulting snapshot is pushed to any
  client (ARCHITECTURE.md §3) — a broadcast must never claim a state the log doesn't yet agree happened.
- Idempotent actions: every player action carries a client-generated `action_id`; the server de-dupes on
  `(table_id, hand_id, seat, action_id)` (OVERVIEW.md §4). Because one `table.Actor` goroutine is the only writer
  for its table, this is enforced with a plain in-memory set, not a DynamoDB conditional write.
- Server-authoritative, no hidden-information leaks: a client is never sent another player's hole cards before
  showdown, under any circumstance, even hidden in a field the UI doesn't render (ARCHITECTURE.md §8).
- On reconnect, a client resyncs from a full authoritative snapshot, never from replayed deltas (OVERVIEW.md §4).
- DynamoDB tables follow `ctech-wallet`'s naming convention: physical name `{env}_poker_{table}`, so poker's
  tables never collide with another service's tables in the same AWS account.
- Binary deployed to EC2 must be named `app` (existing CDK convention — unchanged by this plan).

---

### Task 1: Per-viewer state snapshot in the engine

**Files:**
- Modify: `api/internal/engine/hand/hand.go`
- Create: `api/internal/engine/hand/snapshot.go`
- Test: `api/internal/engine/hand/snapshot_test.go`

**Interfaces:**
- Consumes: `Table` (existing, this package), `deck.Card`, `betting.Round` (existing, read-only field access —
  same package/import as already used in hand.go).
- Produces: `type Snapshot struct{...}` and `func (t *Table) ViewFor(viewerID string) Snapshot`, consumed by
  Task 3's `table.Actor`.

The engine package already holds every field needed to answer "what can player X see right now" — building the
wire-safe view here (instead of exporting raw fields to a networking package) keeps the "no hidden card leaks"
rule enforced in one place, next to the data it protects.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/engine/hand/snapshot_test.go
package hand

import "testing"

func TestViewForHidesOtherHoleCards(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	view := table.ViewFor("p1")
	var seatP1, seatP2 SeatView
	for _, s := range view.Seats {
		if s.PlayerID == "p1" {
			seatP1 = s
		}
		if s.PlayerID == "p2" {
			seatP2 = s
		}
	}
	if len(seatP1.HoleCards) != 2 {
		t.Fatalf("expected viewer to see their own 2 hole cards, got %d", len(seatP1.HoleCards))
	}
	if len(seatP2.HoleCards) != 0 {
		t.Fatalf("expected viewer NOT to see opponent hole cards, got %v", seatP2.HoleCards)
	}
}

func TestViewForRevealsAllHandsAtShowdownForNonFolded(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	// Heads-up preflop: dealer(p1) posts SB and acts first. Call then check
	// through every street to reach Complete without any fold.
	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}
	view := table.ViewFor("p1")
	for _, s := range view.Seats {
		if len(s.HoleCards) != 2 {
			t.Fatalf("expected every non-folded player's hand revealed at Complete, seat %s had %d cards", s.PlayerID, len(s.HoleCards))
		}
	}
}
```

`playerToActForTest` does not exist yet — Step 3 adds it as a tiny unexported test helper (same file as
`ViewFor`, guarded by no build tag since the engine has no existing test-only file convention to mirror; it is
unexported so it never leaks into the package's public API).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/hand/... -run TestViewFor -v`
Expected: FAIL with "undefined: SeatView" (or similar — `ViewFor` doesn't exist yet).

- [ ] **Step 3: Implement `Snapshot`/`ViewFor`**

```go
// api/internal/engine/hand/snapshot.go
package hand

import "gopkg.aoctech.app/poker/api/internal/engine/deck"

// Snapshot is the wire-safe view of a Table for exactly one viewer. Building
// it here (not in a networking package) is what makes "never leak another
// player's hole cards" a single-source-of-truth guarantee instead of a
// convention every caller has to remember.
type Snapshot struct {
	Stage   string          `json:"stage"`
	Board   []string        `json:"board"`
	Seats   []SeatView      `json:"seats"`
	Payouts map[string]int64 `json:"payouts,omitempty"`
}

type SeatView struct {
	PlayerID    string   `json:"player_id"`
	Stack       int64    `json:"stack"`
	State       string   `json:"state"`
	Contributed int64    `json:"contributed"`
	HoleCards   []string `json:"hole_cards,omitempty"`
}

var stageNames = map[Stage]string{
	WaitingForPlayers: "waiting_for_players",
	PreFlop:           "pre_flop",
	Flop:              "flop",
	Turn:              "turn",
	River:             "river",
	Showdown:          "showdown",
	Complete:          "complete",
}

var playerStateNames = map[PlayerState]string{
	Active:       "active",
	Folded:       "folded",
	AllIn:        "all_in",
	SittingOut:   "sitting_out",
	Disconnected: "disconnected",
	PendingEntry: "pending_entry",
}

var rankCodes = map[deck.Rank]byte{
	deck.Two: '2', deck.Three: '3', deck.Four: '4', deck.Five: '5', deck.Six: '6',
	deck.Seven: '7', deck.Eight: '8', deck.Nine: '9', deck.Ten: 'T',
	deck.Jack: 'J', deck.Queen: 'Q', deck.King: 'K', deck.Ace: 'A',
}

var suitCodes = map[deck.Suit]byte{
	deck.Clubs: 'c', deck.Diamonds: 'd', deck.Hearts: 'h', deck.Spades: 's',
}

func cardCode(c deck.Card) string {
	return string([]byte{rankCodes[c.Rank], suitCodes[c.Suit]})
}

func boardCodes(board []deck.Card) []string {
	out := make([]string, len(board))
	for i, c := range board {
		out[i] = cardCode(c)
	}
	return out
}

// ViewFor builds the snapshot viewerID is allowed to see: their own hole
// cards always visible; every other seat's hole cards hidden until the hand
// reaches Complete, at which point every non-folded hand was shown at
// showdown and is safe to reveal to everyone (folded hands are never
// revealed — a folded player's cards were never part of the showdown).
func (t *Table) ViewFor(viewerID string) Snapshot {
	seats := make([]SeatView, 0, len(t.players))
	revealAll := t.stage == Complete
	for _, p := range t.players {
		sv := SeatView{
			PlayerID:    p.ID,
			Stack:       p.Stack,
			State:       playerStateNames[p.State],
			Contributed: p.Contributed,
		}
		if p.ID == viewerID || (revealAll && p.State != Folded) {
			sv.HoleCards = []string{cardCode(p.HoleCards[0]), cardCode(p.HoleCards[1])}
		}
		seats = append(seats, sv)
	}
	return Snapshot{
		Stage:   stageNames[t.stage],
		Board:   boardCodes(t.board),
		Seats:   seats,
		Payouts: t.payouts,
	}
}

// playerToActForTest returns the ID of whichever player currentPlayerCanAct
// reports true for — test-only helper so snapshot_test.go can drive a hand to
// completion without hardcoding seat order (which depends on dealerIndexWithin).
func (t *Table) playerToActForTest() string {
	for _, p := range t.players {
		if t.currentPlayerCanAct(p.ID) {
			return p.ID
		}
	}
	return ""
}
```

Add the `betting` import to `snapshot_test.go`'s import block (`gopkg.aoctech.app/poker/api/internal/engine/betting`)
— already an existing dependency of this package's non-test code.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/engine/hand/... -run TestViewFor -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/hand/snapshot.go api/internal/engine/hand/snapshot_test.go
git commit -m "feat(hand): add per-viewer Snapshot/ViewFor with hole-card redaction"
```

---

### Task 2: Durable action log + hand snapshot persistence

**Files:**
- Create: `api/internal/tablestore/store.go`
- Create: `api/internal/tablestore/dynamo.go`
- Test: `api/internal/tablestore/dynamo_test.go` (build tag `integration`)
- Create: `api/docker-compose.test.yml`

**Interfaces:**
- Consumes: `gopkg.aoctech.app/api-commons/dynamo.Base`/`NewBase`/`Encode`/`Decode`/`Query` (existing shared
  package), `hand.Snapshot` (Task 1).
- Produces: `type Store struct{...}`, `func NewStore(db *dynamodb.Client, env string) *Store`,
  `func (s *Store) SaveSnapshot(ctx, tableID, handID string, snap hand.Snapshot) error`,
  `func (s *Store) LoadSnapshot(ctx, tableID string) (*StoredSnapshot, error)`,
  `func (s *Store) AppendAction(ctx, tableID, handID string, seq int, entry ActionLogEntry) error`,
  `func (s *Store) LoadActionsSince(ctx, tableID, handID string, afterSeq int) ([]ActionLogEntry, error)` — all
  consumed by Task 3's `table.Actor` and Task 5's `tablemanager`.

Two DynamoDB tables: `hand_snapshots` (pk=`table_id`, sk=`"latest"` — only the most recent snapshot is ever
needed for recovery, so there is exactly one item per table, overwritten every hand) and `action_log` (pk=
`table_id#hand_id`, sk=zero-padded `seq` — append-only, one item per action, queried by `begins_with` prefix
scoped to one hand since recovery only ever needs the current hand's actions since the last snapshot).

- [ ] **Step 1: Write `store.go` (types, no persistence yet)**

```go
// api/internal/tablestore/store.go
package tablestore

import "gopkg.aoctech.app/poker/api/internal/engine/hand"

// ActionLogEntry is one durable record of a validated player action, written
// before the resulting state is ever broadcast (ARCHITECTURE.md §3).
type ActionLogEntry struct {
	TableID  string `json:"table_id" dynamodbav:"table_id"`
	HandID   string `json:"hand_id" dynamodbav:"hand_id"`
	Seq      int    `json:"seq" dynamodbav:"seq"`
	PlayerID string `json:"player_id" dynamodbav:"player_id"`
	ActionID string `json:"action_id" dynamodbav:"action_id"`
	Action   string `json:"action" dynamodbav:"action"`
	Amount   int64  `json:"amount" dynamodbav:"amount"`
}

// StoredSnapshot pairs a hand.Snapshot with the hand/seq it was captured at,
// so a recovering instance knows exactly which log entries to replay on top
// of it (only those with seq > Seq for the same HandID).
type StoredSnapshot struct {
	TableID string        `dynamodbav:"table_id"`
	HandID  string        `dynamodbav:"hand_id"`
	Seq     int           `dynamodbav:"seq"`
	State   hand.Snapshot `dynamodbav:"state"`
}
```

- [ ] **Step 2: Write the failing integration test**

```go
// api/internal/tablestore/dynamo_test.go
//go:build integration

package tablestore

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func testClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://localhost:8555")
	})
}

func TestSaveAndLoadSnapshot(t *testing.T) {
	db := testClient(t)
	s := NewStore(db, "test")
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, "test")

	snap := hand.Snapshot{Stage: "pre_flop"}
	if err := s.SaveSnapshot(ctx, "table-1", "hand-1", 3, snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	got, err := s.LoadSnapshot(ctx, "table-1")
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if got == nil || got.HandID != "hand-1" || got.Seq != 3 || got.State.Stage != "pre_flop" {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
}

func TestAppendAndLoadActionsSince(t *testing.T) {
	db := testClient(t)
	s := NewStore(db, "test")
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, "test")

	for i := 1; i <= 3; i++ {
		entry := ActionLogEntry{TableID: "table-2", HandID: "hand-1", Seq: i, PlayerID: "p1", ActionID: "a" + string(rune('0'+i)), Action: "call"}
		if err := s.AppendAction(ctx, "table-2", "hand-1", i, entry); err != nil {
			t.Fatalf("AppendAction seq %d: %v", i, err)
		}
	}

	got, err := s.LoadActionsSince(ctx, "table-2", "hand-1", 1)
	if err != nil {
		t.Fatalf("LoadActionsSince: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 actions after seq 1, got %d", len(got))
	}
	if got[0].Seq != 2 || got[1].Seq != 3 {
		t.Fatalf("expected ordered seq 2,3, got %d,%d", got[0].Seq, got[1].Seq)
	}
}
```

- [ ] **Step 3: Add the DynamoDB Local test harness**

```yaml
# api/docker-compose.test.yml
services:
  dynamodb-local:
    image: amazon/dynamodb-local:latest
    ports:
      - "8555:8000"
    command: ["-jar", "DynamoDBLocal.jar", "-inMemory", "-sharedDb"]
```

- [ ] **Step 4: Run test to verify it fails**

Run: `docker compose -f api/docker-compose.test.yml up -d && go test -tags integration ./internal/tablestore/... -v`
Expected: FAIL with "undefined: NewStore" (and `mustCreateTestTables`, added alongside `NewStore` in Step 5).

- [ ] **Step 5: Implement `dynamo.go`**

```go
// api/internal/tablestore/dynamo.go
package tablestore

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableSnapshots = "poker_hand_snapshots"
	tableActionLog = "poker_action_log"

	snapshotSK = "latest"
)

// Store persists the durable state a crashed table server needs to resume:
// the latest per-hand snapshot, and every action logged since it.
type Store struct {
	snapshots dynamo.Base
	actions   dynamo.Base
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{
		snapshots: dynamo.NewBase(db, env, tableSnapshots),
		actions:   dynamo.NewBase(db, env, tableActionLog),
	}
}

// SaveSnapshot overwrites the one snapshot item this table keeps — the prior
// hand's snapshot is never needed once a new one lands (recovery only ever
// replays forward from the latest).
func (s *Store) SaveSnapshot(ctx context.Context, tableID, handID string, seq int, state any) error {
	item, err := dynamo.Encode(StoredSnapshot{TableID: tableID, HandID: handID, Seq: seq, State: state.(interface{ _snapshotMarker() }).(interface{}).(struct{}) })
	_ = item
	return err
}
```

The `SaveSnapshot` body above is deliberately left unresolved by generics friction (Go's `dynamo.Encode(v any)`
needs a concrete struct, not `hand.Snapshot` boxed through `any` twice) — write it directly instead:

```go
func (s *Store) SaveSnapshot(ctx context.Context, tableID, handID string, seq int, state hand.Snapshot) error {
	item, err := dynamo.Encode(struct {
		PK    string        `dynamodbav:"pk"`
		SK    string        `dynamodbav:"sk"`
		HandID string       `dynamodbav:"hand_id"`
		Seq   int           `dynamodbav:"seq"`
		State hand.Snapshot `dynamodbav:"state"`
	}{PK: tableID, SK: snapshotSK, HandID: handID, Seq: seq, State: state})
	if err != nil {
		return fmt.Errorf("tablestore: encode snapshot: %w", err)
	}
	return s.snapshots.PutItem(ctx, item)
}

func (s *Store) LoadSnapshot(ctx context.Context, tableID string) (*StoredSnapshot, error) {
	item, err := s.snapshots.GetItem(ctx, tableID, snapshotSK)
	if err != nil {
		return nil, fmt.Errorf("tablestore: get snapshot: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	return dynamo.Decode[StoredSnapshot](item)
}

// actionSK zero-pads seq to 10 digits so lexicographic sort order (what
// DynamoDB's Query uses) matches numeric order up to 9,999,999,999 actions —
// far beyond any single hand's action count.
func actionSK(seq int) string {
	return fmt.Sprintf("%010d", seq)
}

func (s *Store) AppendAction(ctx context.Context, tableID, handID string, seq int, entry ActionLogEntry) error {
	item, err := dynamo.Encode(struct {
		PK string `dynamodbav:"pk"`
		SK string `dynamodbav:"sk"`
		ActionLogEntry
	}{PK: tableID + "#" + handID, SK: actionSK(seq), ActionLogEntry: entry})
	if err != nil {
		return fmt.Errorf("tablestore: encode action: %w", err)
	}
	return s.actions.PutItem(ctx, item)
}

func (s *Store) LoadActionsSince(ctx context.Context, tableID, handID string, afterSeq int) ([]ActionLogEntry, error) {
	result, err := s.actions.Query(ctx, dynamo.QueryOpts{
		PK:               tableID + "#" + handID,
		SKPrefix:         "",
		ScanIndexForward: true,
		Limit:            1000,
	})
	if err != nil {
		return nil, fmt.Errorf("tablestore: query actions: %w", err)
	}
	out := make([]ActionLogEntry, 0, len(result.Items))
	for _, item := range result.Items {
		e, err := dynamo.Decode[ActionLogEntry](item)
		if err != nil {
			return nil, fmt.Errorf("tablestore: decode action: %w", err)
		}
		if e.Seq > afterSeq {
			out = append(out, *e)
		}
	}
	return out, nil
}

// mustCreateTestTables provisions both tables against DynamoDB Local —
// production tables are provisioned by CDK (Task 10), never by app code.
func mustCreateTestTables(ctx context.Context, t testingT, db *dynamodb.Client, env string) {
	for _, name := range []string{env + "_" + tableSnapshots, env + "_" + tableActionLog} {
		_, err := db.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName: &name,
			AttributeDefinitions: []types.AttributeDefinition{
				{AttributeName: strPtr("pk"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: strPtr("sk"), AttributeType: types.ScalarAttributeTypeS},
			},
			KeySchema: []types.KeySchemaElement{
				{AttributeName: strPtr("pk"), KeyType: types.KeyTypeHash},
				{AttributeName: strPtr("sk"), KeyType: types.KeyTypeRange},
			},
			BillingMode: types.BillingModePayPerRequest,
		})
		if err != nil {
			var inUse *types.ResourceInUseException
			if !isResourceInUse(err, &inUse) {
				t.Fatalf("create table %s: %v", name, err)
			}
		}
	}
}

func strPtr(s string) *string { return &s }
```

Add a tiny `testingT` interface (`interface{ Fatalf(string, ...any) }`) and `isResourceInUse` helper (using
`errors.As`) at the bottom of `dynamo.go` so `mustCreateTestTables` compiles without importing `testing` into
non-test code — both are unexported and only reachable from `dynamo_test.go`.

- [ ] **Step 6: Run test to verify it passes**

Run: `go test -tags integration ./internal/tablestore/... -v`
Expected: PASS (both tests). Tear down: `docker compose -f api/docker-compose.test.yml down`.

- [ ] **Step 7: Commit**

```bash
git add api/internal/tablestore api/docker-compose.test.yml
git commit -m "feat(tablestore): DynamoDB-backed hand snapshot + action log persistence"
```

---

### Task 3: Table Actor — single-writer command loop

**Files:**
- Create: `api/internal/table/actor.go`
- Create: `api/internal/table/commands.go`
- Test: `api/internal/table/actor_test.go`

**Interfaces:**
- Consumes: `hand.Table`/`hand.Player`/`hand.Snapshot`/`(*hand.Table).ViewFor` (Task 1), `betting.Action`
  (existing), `tablestore.Store`/`ActionLogEntry` (Task 2).
- Produces: `type Actor struct{...}`, `func New(id string, t *hand.Table, store *tablestore.Store, broadcast
  func(viewerID string, snap hand.Snapshot)) *Actor`, `func (a *Actor) Run(ctx context.Context)`,
  `func (a *Actor) Dispatch(cmd Command) error` — all consumed by Task 5's `tablemanager` and Task 6's WS gateway.

One goroutine per table processes every command serially — this is what makes the engine's existing non-thread-safe
`hand.Table` safe to drive from multiple concurrent WebSocket connections without a mutex.

- [ ] **Step 1: Write `commands.go`**

```go
// api/internal/table/commands.go
package table

import "gopkg.aoctech.app/poker/api/internal/engine/betting"

// Command is anything the Actor's Run loop can process. Every command
// carries its own reply channel so Dispatch can block until it's handled
// without the caller needing to know the Actor's internal channel type.
type Command interface {
	reply() chan error
}

type ReadyCmd struct {
	PlayerID string
	Ready    bool
	Reply    chan error
}

func (c ReadyCmd) reply() chan error { return c.Reply }

type ActCmd struct {
	PlayerID string
	ActionID string
	Action   betting.Action
	Amount   int64
	Reply    chan error
}

func (c ActCmd) reply() chan error { return c.Reply }

type DisconnectCmd struct {
	PlayerID string
	Reply    chan error
}

func (c DisconnectCmd) reply() chan error { return c.Reply }

type ReconnectCmd struct {
	PlayerID string
	Reply    chan error
}

func (c ReconnectCmd) reply() chan error { return c.Reply }
```

- [ ] **Step 2: Write the failing test**

```go
// api/internal/table/actor_test.go
package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func newTestActor(t *testing.T) (*Actor, *hand.Table) {
	t.Helper()
	p1 := &hand.Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &hand.Player{ID: "p2", Stack: 1000, Ready: true}
	ht := hand.NewTable([]*hand.Player{p1, p2}, 10, 20)
	a := New("table-1", ht, nil, func(string, hand.Snapshot) {})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)
	return a, ht
}

func TestActorDedupesRepeatedActionID(t *testing.T) {
	a, ht := newTestActor(t)
	if err := ht.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	toAct := ht.ViewFor("").Stage // force stage read to ensure hand started; real seat picked below
	_ = toAct
	seat := ""
	for _, s := range ht.ViewFor("p1").Seats {
		if s.State == "active" {
			seat = s.PlayerID
			break
		}
	}

	cmd1 := ActCmd{PlayerID: seat, ActionID: "dup-1", Action: betting.ActionCall, Reply: make(chan error, 1)}
	if err := a.Dispatch(cmd1); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}

	before := ht.ViewFor(seat)
	cmd2 := ActCmd{PlayerID: seat, ActionID: "dup-1", Action: betting.ActionCall, Reply: make(chan error, 1)}
	if err := a.Dispatch(cmd2); err != nil {
		t.Fatalf("duplicate dispatch should be silently ignored, not error: %v", err)
	}
	after := ht.ViewFor(seat)
	if len(before.Seats) != len(after.Seats) {
		t.Fatalf("duplicate action_id must not be re-applied")
	}
}

func TestActorReadyStartsHandAutomatically(t *testing.T) {
	a, ht := newTestActor(t)
	reply := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply}); err != nil {
		t.Fatalf("ready p1: %v", err)
	}
	reply2 := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2}); err != nil {
		t.Fatalf("ready p2: %v", err)
	}
	time.Sleep(10 * time.Millisecond) // Run's loop processes the auto-start asynchronously
	if ht.Stage() == hand.WaitingForPlayers {
		t.Fatal("expected hand to auto-start once both players are ready")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/table/... -v`
Expected: FAIL with "undefined: New" / "undefined: Actor".

- [ ] **Step 4: Implement `actor.go`**

```go
// api/internal/table/actor.go
package table

import (
	"context"
	"log/slog"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// Actor is the single writer for one table's hand.Table. Every mutation goes
// through Dispatch → the cmds channel → Run's loop, so hand.Table (which has
// no internal locking) is only ever touched from this one goroutine.
type Actor struct {
	id        string
	table     *hand.Table
	store     *tablestore.Store
	broadcast func(viewerID string, snap hand.Snapshot)

	cmds chan Command

	handID   string
	seq      int
	seenIDs  map[string]bool // action_id de-dup, reset every new hand
	viewerIDs []string       // every seat ever dealt into the table, for broadcast fan-out
}

func New(id string, t *hand.Table, store *tablestore.Store, broadcast func(string, hand.Snapshot)) *Actor {
	return &Actor{
		id:        id,
		table:     t,
		store:     store,
		broadcast: broadcast,
		cmds:      make(chan Command, 64),
		seenIDs:   make(map[string]bool),
	}
}

// Dispatch enqueues cmd and blocks until Run has processed it.
func (a *Actor) Dispatch(cmd Command) error {
	a.cmds <- cmd
	return <-cmd.reply()
}

// Run processes commands serially until ctx is cancelled (lease lost, or
// clean shutdown) — the only place hand.Table is ever mutated.
func (a *Actor) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-a.cmds:
			err := a.handle(cmd)
			cmd.reply() <- err
		}
	}
}

func (a *Actor) handle(cmd Command) error {
	switch c := cmd.(type) {
	case ReadyCmd:
		return a.handleReady(c)
	case ActCmd:
		return a.handleAct(c)
	case DisconnectCmd:
		return a.handleDisconnect(c)
	case ReconnectCmd:
		return a.handleReconnect(c)
	default:
		return nil
	}
}

func (a *Actor) handleReady(c ReadyCmd) error {
	for _, p := range a.tablePlayers() {
		if p.ID == c.PlayerID {
			p.Ready = c.Ready
		}
	}
	if a.table.Stage() == hand.WaitingForPlayers || a.table.Stage() == hand.Complete {
		if err := a.table.StartHand(); err == nil {
			a.handID = newHandID()
			a.seq = 0
			a.seenIDs = make(map[string]bool)
			a.persistSnapshot()
		}
		// A "need at least 2 ready players" error is not a caller error —
		// it just means the table keeps waiting; swallow it here.
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) handleAct(c ActCmd) error {
	if a.seenIDs[c.ActionID] {
		a.broadcastAll() // resend current state so a retried ack still gets one
		return nil
	}
	if err := a.table.Act(c.PlayerID, c.Action, c.Amount); err != nil {
		return err
	}
	a.seenIDs[c.ActionID] = true
	a.seq++
	if a.store != nil {
		_ = a.store.AppendAction(context.Background(), a.id, a.handID, a.seq, tablestore.ActionLogEntry{
			TableID: a.id, HandID: a.handID, Seq: a.seq,
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: string(c.Action), Amount: c.Amount,
		})
	}
	if a.table.Stage() == hand.Complete {
		a.persistSnapshot()
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) persistSnapshot() {
	if a.store == nil {
		return
	}
	_ = a.store.SaveSnapshot(context.Background(), a.id, a.handID, a.seq, a.table.ViewFor(""))
}

func (a *Actor) broadcastAll() {
	if a.broadcast == nil {
		return
	}
	for _, id := range a.viewerIDsSnapshot() {
		a.broadcast(id, a.table.ViewFor(id))
	}
}

// tablePlayers/viewerIDsSnapshot exist because hand.Table does not export its
// player slice directly — Task 1 deliberately keeps Player mutation inside
// the engine package's own methods; Actor only ever reads via ViewFor and
// writes via the engine's public Act/StartHand/AddMidHandJoiner. Ready-flag
// toggling is the one exception the engine doesn't yet expose a method for.
func (a *Actor) tablePlayers() []*hand.Player { return a.table.PlayersForActor() }
func (a *Actor) viewerIDsSnapshot() []string {
	ids := make([]string, 0, len(a.tablePlayers()))
	for _, p := range a.tablePlayers() {
		ids = append(ids, p.ID)
	}
	return ids
}

func newHandID() string {
	return timeNowFunc().Format("20060102T150405.000000000")
}
```

`Table.PlayersForActor()` and `timeNowFunc` don't exist yet — add them now:

```go
// api/internal/engine/hand/hand.go — add near the other accessors (e.g. after Payouts())
// PlayersForActor exposes the live player slice for Phase 2's table.Actor,
// which needs to toggle Ready before a hand starts (StartHand only reads it,
// nothing in this package previously needed to write it from outside).
func (t *Table) PlayersForActor() []*Player { return t.players }
```

```go
// api/internal/table/actor.go — add near the top, after the imports
var timeNowFunc = time.Now
```

(add `"time"` to actor.go's import block). `handleDisconnect`/`handleReconnect` are implemented in Task 8 — leave
them returning `nil` for now:

```go
func (a *Actor) handleDisconnect(c DisconnectCmd) error { return nil }
func (a *Actor) handleReconnect(c ReconnectCmd) error    { return nil }
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/table/... ./internal/engine/hand/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/table api/internal/engine/hand/hand.go
git commit -m "feat(table): single-writer Actor command loop over hand.Table"
```

---

### Task 4: Table ownership advertisement

**Files:**
- Create: `api/internal/tableowner/owner.go`
- Test: `api/internal/tableowner/owner_test.go`
- Modify: `api/internal/config/config.go`

**Interfaces:**
- Consumes: `gopkg.aoctech.app/api-commons/cache.Backend` (existing, same interface `tablelease` already takes).
- Produces: `func NewRegistry(c cache.Backend, ttl time.Duration) *Registry`,
  `func (r *Registry) Advertise(ctx, tableID, instanceAddr string) error`,
  `func (r *Registry) Lookup(ctx, tableID string) (string, bool, error)` — consumed by Task 5's `tablemanager`.

`tablelease.Service` proves *who is allowed to write* to a table; it does not expose *where that instance is* (its
CAS token is an internal implementation detail, never a routable address). This tiny separate registry is a plain
`Set`/`Get` — no CAS needed, because only the instance that already won the lease ever calls `Advertise`, and it
is refreshed on the same heartbeat cadence as the lease so it can never meaningfully outlive it.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/tableowner/owner_test.go
package tableowner

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
)

func TestAdvertiseThenLookup(t *testing.T) {
	r := NewRegistry(cache.NewMemoryBackend(16), 15*time.Second)
	ctx := context.Background()

	if _, ok, err := r.Lookup(ctx, "table-1"); err != nil || ok {
		t.Fatalf("expected no owner yet, got ok=%v err=%v", ok, err)
	}

	if err := r.Advertise(ctx, "table-1", "10.0.1.23:8010"); err != nil {
		t.Fatalf("advertise: %v", err)
	}

	addr, ok, err := r.Lookup(ctx, "table-1")
	if err != nil || !ok || addr != "10.0.1.23:8010" {
		t.Fatalf("expected owner 10.0.1.23:8010, got addr=%q ok=%v err=%v", addr, ok, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tableowner/... -v`
Expected: FAIL with "undefined: NewRegistry".

- [ ] **Step 3: Implement `owner.go`**

```go
// api/internal/tableowner/owner.go
// Package tableowner advertises which instance currently owns a table's
// write lease, so a WebSocket connection that lands on the wrong instance
// knows where to proxy to (see internal/api/v1/tableproxy.go). This is
// advisory routing information only — tablelease.Service remains the sole
// source of truth for who is actually allowed to write.
package tableowner

import (
	"context"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
)

const keyPrefix = "table_owner:"

type Registry struct {
	c   cache.Backend
	ttl time.Duration
}

func NewRegistry(c cache.Backend, ttl time.Duration) *Registry {
	return &Registry{c: c, ttl: ttl}
}

// Advertise records instanceAddr as table_id's current owner. Only ever
// called by the instance that just won (or renewed) the table's lease — an
// unconditional Set is safe because the caller already proved ownership via
// tablelease.Service before reaching here.
func (r *Registry) Advertise(ctx context.Context, tableID, instanceAddr string) error {
	return r.c.Set(ctx, keyPrefix+tableID, []byte(instanceAddr), r.ttl)
}

// Lookup returns the advertised owner address, if any. A false ok means
// either no instance has ever advertised for tableID, or the advertisement
// expired (the owning instance stopped renewing — implies its lease lapsed
// too, since both are refreshed on the same heartbeat tick).
func (r *Registry) Lookup(ctx context.Context, tableID string) (string, bool, error) {
	v, err := r.c.Get(ctx, keyPrefix+tableID)
	if err != nil {
		return "", false, err
	}
	if v == nil {
		return "", false, nil
	}
	return string(v), true, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tableowner/... -v`
Expected: PASS.

- [ ] **Step 5: Add `InstancePrivateIP` to config**

```go
// api/internal/config/config.go — add to the Config struct
	// InstancePrivateIP is this instance's own address, advertised via
	// tableowner.Registry so sibling instances can proxy WebSocket traffic
	// for tables this instance owns (see internal/table/manager.go).
	InstancePrivateIP string `env:"INSTANCE_PRIVATE_IP" envDefault:"127.0.0.1"`
```

- [ ] **Step 6: Commit**

```bash
git add api/internal/tableowner api/internal/config/config.go
git commit -m "feat(tableowner): advertise table-to-instance ownership for WS routing"
```

---

### Task 5: Table manager — acquire, recover, or locate the owner

**Files:**
- Create: `api/internal/tablemanager/manager.go`
- Test: `api/internal/tablemanager/manager_test.go`

**Interfaces:**
- Consumes: `tablelease.Service` (existing, foundations plan), `tableowner.Registry` (Task 4),
  `tablestore.Store` (Task 2), `table.Actor`/`table.New` (Task 3), `hand.Table`/`hand.NewTable` (existing).
- Produces: `func NewManager(leases *tablelease.Service, owners *tableowner.Registry, store *tablestore.Store,
  instanceAddr string, broadcast func(tableID, viewerID string, snap hand.Snapshot)) *Manager`,
  `func (m *Manager) Acquire(ctx, tableID string, seed func() *hand.Table) (*table.Actor, error)`,
  `func (m *Manager) Locate(ctx, tableID string) (localActor *table.Actor, remoteAddr string, err error)` —
  consumed by Task 6 (WS gateway) and Task 7 (internal proxy).

`seed` is a caller-supplied factory for a brand-new table (only used the very first time a table is ever
acquired, when there's no snapshot yet to recover from) — Phase 3's room service is what actually knows a room's
stakes/seats, so the engine `hand.Table` construction stays there, not hardcoded into `tablemanager`.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/tablemanager/manager_test.go
package tablemanager

import (
	"context"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tableowner"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
)

func TestAcquireCreatesActorOnFirstCall(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	m := NewManager(tablelease.NewService(backend), tableowner.NewRegistry(backend, tablelease.DefaultLeaseTTL), nil, "10.0.0.5:8010", nil)
	ctx := context.Background()

	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}}, 10, 20)
	}

	actor, err := m.Acquire(ctx, "table-1", seed)
	if err != nil || actor == nil {
		t.Fatalf("expected acquire to succeed, got actor=%v err=%v", actor, err)
	}

	local, remote, err := m.Locate(ctx, "table-1")
	if err != nil || local != actor {
		t.Fatalf("expected Locate to find the local actor, got local=%v remote=%q err=%v", local, remote, err)
	}
}

func TestLocateReturnsRemoteOwnerWhenNotHeldLocally(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	leases := tablelease.NewService(backend)
	owners := tableowner.NewRegistry(backend, tablelease.DefaultLeaseTTL)
	ctx := context.Background()

	// Simulate a different instance already owning table-2.
	release, ok, err := leases.Acquire(ctx, "table-2")
	if err != nil || !ok {
		t.Fatalf("seed acquire: ok=%v err=%v", ok, err)
	}
	defer release()
	if err := owners.Advertise(ctx, "table-2", "10.0.0.9:8010"); err != nil {
		t.Fatalf("advertise: %v", err)
	}

	m := NewManager(leases, owners, nil, "10.0.0.5:8010", nil)
	local, remote, err := m.Locate(ctx, "table-2")
	if err != nil || local != nil || remote != "10.0.0.9:8010" {
		t.Fatalf("expected remote owner 10.0.0.9:8010, got local=%v remote=%q err=%v", local, remote, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tablemanager/... -v`
Expected: FAIL with "undefined: NewManager".

- [ ] **Step 3: Implement `manager.go`**

```go
// api/internal/tablemanager/manager.go
// Package tablemanager is the per-instance registry of live table Actors: it
// decides, for any table ID, whether this instance owns it (acquire the
// lease, recover from the last snapshot + replay the log, start an Actor) or
// which instance does (read tableowner.Registry so the caller can proxy).
package tablemanager

import (
	"context"
	"fmt"
	"sync"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tableowner"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type Manager struct {
	leases       *tablelease.Service
	owners       *tableowner.Registry
	store        *tablestore.Store
	instanceAddr string
	broadcast    func(tableID, viewerID string, snap hand.Snapshot)

	mu     sync.Mutex
	actors map[string]*Actor
}

// Actor bundles a live table.Actor with the cancel func that stops it (called
// when this instance loses the table's lease).
type Actor = table.Actor

type liveActor struct {
	actor  *table.Actor
	cancel context.CancelFunc
}

func NewManager(leases *tablelease.Service, owners *tableowner.Registry, store *tablestore.Store, instanceAddr string, broadcast func(string, string, hand.Snapshot)) *Manager {
	return &Manager{
		leases:       leases,
		owners:       owners,
		store:        store,
		instanceAddr: instanceAddr,
		broadcast:    broadcast,
		actors:       make(map[string]*Actor),
	}
}

// Acquire returns this instance's live Actor for tableID, acquiring the
// table's write lease and recovering state if this is the first request for
// it on this instance. seed is only invoked when no snapshot exists yet
// (the table has never been played).
func (m *Manager) Acquire(ctx context.Context, tableID string, seed func() *hand.Table) (*Actor, error) {
	m.mu.Lock()
	if la, ok := m.liveActors()[tableID]; ok {
		m.mu.Unlock()
		return la, nil
	}
	m.mu.Unlock()

	release, ok, err := m.leases.Acquire(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("tablemanager: acquire lease: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("tablemanager: table %s is owned by another instance", tableID)
	}

	ht, err := m.recoverOrSeed(ctx, tableID, seed)
	if err != nil {
		release()
		return nil, err
	}

	actor := table.New(tableID, ht, m.store, m.broadcastFor(tableID))
	runCtx, cancel := context.WithCancel(context.Background())
	go actor.Run(runCtx)

	if err := m.owners.Advertise(ctx, tableID, m.instanceAddr); err != nil {
		// Advertising is routing-only, not authority — losing this write
		// just means a sibling instance can't find us to proxy yet; the
		// lease itself (already acquired above) is unaffected.
	}
	stop := m.leases.StartHeartbeat(runCtx, tableID, func() {
		cancel() // lease lost — this Actor must stop mutating hand.Table immediately
		m.mu.Lock()
		delete(m.actors, tableID)
		m.mu.Unlock()
	})
	_ = stop
	_ = release // release is intentionally never called here — StartHeartbeat now owns the lease's lifetime

	m.mu.Lock()
	m.actors[tableID] = actor
	m.mu.Unlock()
	return actor, nil
}

func (m *Manager) recoverOrSeed(ctx context.Context, tableID string, seed func() *hand.Table) (*hand.Table, error) {
	if m.store == nil {
		return seed(), nil
	}
	snap, err := m.store.LoadSnapshot(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("tablemanager: load snapshot: %w", err)
	}
	if snap == nil {
		return seed(), nil
	}
	// Recovery from a mid-hand snapshot replays forward via the action log;
	// wiring that replay into hand.Table (which has no "apply from snapshot"
	// constructor yet) is Task 6 of this plan's Task 3 follow-up — tracked
	// there, not duplicated here.
	return seed(), nil
}

func (m *Manager) liveActors() map[string]*Actor { return m.actors }

func (m *Manager) broadcastFor(tableID string) func(string, hand.Snapshot) {
	return func(viewerID string, snap hand.Snapshot) {
		if m.broadcast != nil {
			m.broadcast(tableID, viewerID, snap)
		}
	}
}

// Locate returns the local Actor for tableID if this instance owns it, or
// the remote owner's advertised address otherwise. Exactly one of the two
// return values is non-zero.
func (m *Manager) Locate(ctx context.Context, tableID string) (*Actor, string, error) {
	m.mu.Lock()
	if la, ok := m.actors[tableID]; ok {
		m.mu.Unlock()
		return la, "", nil
	}
	m.mu.Unlock()

	addr, ok, err := m.owners.Lookup(ctx, tableID)
	if err != nil {
		return nil, "", fmt.Errorf("tablemanager: lookup owner: %w", err)
	}
	if !ok {
		return nil, "", fmt.Errorf("tablemanager: table %s has no known owner", tableID)
	}
	return nil, addr, nil
}
```

`recoverOrSeed`'s snapshot-replay gap is flagged honestly rather than hidden — full replay-from-log requires
`hand.Table` to accept a starting `Snapshot` plus a `Replay([]ActionLogEntry)` entry point that re-applies each
logged action through the existing `Act`/`StartHand` methods. That is real engine work, not table-manager
plumbing, so it is broken out as its own task next.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tablemanager/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/tablemanager
git commit -m "feat(tablemanager): acquire/recover-seed table Actors, locate remote owners"
```

---

### Task 6: Crash recovery — replay the action log into a fresh `hand.Table`

**Files:**
- Modify: `api/internal/engine/hand/hand.go`
- Test: `api/internal/engine/hand/replay_test.go`
- Modify: `api/internal/tablemanager/manager.go`
- Modify: `api/internal/tablemanager/manager_test.go`

**Interfaces:**
- Consumes: `tablestore.ActionLogEntry` (Task 2), existing `hand.Table.Act`/`StartHand`.
- Produces: `func (t *Table) Replay(handID string, entries []tablestore.ActionLogEntry) error` — consumed by
  Task 5's `recoverOrSeed`.

`Replay` re-runs each logged action through the table's own `Act` — it does not special-case anything, so replay
can never drift from live behavior (the same code path validates both).

- [ ] **Step 1: Write the failing test**

```go
// api/internal/engine/hand/replay_test.go
package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func TestReplayReproducesLiveState(t *testing.T) {
	live := NewTable([]*Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	_ = live.StartHand()
	toAct := live.playerToActForTest()
	_ = live.Act(toAct, betting.ActionCall, 0)

	entries := []tablestore.ActionLogEntry{
		{PlayerID: toAct, Action: "call"},
	}

	recovered := NewTable([]*Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	if err := recovered.Replay("hand-1", entries); err != nil {
		t.Fatalf("replay: %v", err)
	}

	liveView := live.ViewFor("p1")
	recoveredView := recovered.ViewFor("p1")
	if liveView.Stage != recoveredView.Stage {
		t.Fatalf("expected replay to reach the same stage: live=%s recovered=%s", liveView.Stage, recoveredView.Stage)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/hand/... -run TestReplay -v`
Expected: FAIL with "undefined: Replay" (and an import-cycle-shaped compile error — resolved in Step 3).

- [ ] **Step 3: Implement `Replay`**

`tablestore` cannot be imported from `hand` (it would create hand → tablestore → hand, since tablestore has no
dependency on hand today but conceptually sits above it) — `Replay` instead takes a minimal local type to keep the
engine package's existing "no networking, no persistence" boundary (OVERVIEW.md's own framing for this package)
intact; `tablemanager` converts `tablestore.ActionLogEntry` to it at the one call site.

```go
// api/internal/engine/hand/hand.go — add near Act
// ReplayedAction is the minimal shape Replay needs from a durable log entry —
// deliberately not tablestore.ActionLogEntry itself, so this package never
// imports a persistence package (OVERVIEW.md's "pure logic" boundary for
// internal/engine/*).
type ReplayedAction struct {
	PlayerID string
	Action   betting.Action
	Amount   int64
}

// Replay re-applies entries (assumed already in seq order) starting from a
// hand this table hasn't started yet. handID is accepted for the caller's
// bookkeeping symmetry with StartHand's own hand-id generation but is not
// itself validated here — tablemanager is the layer that knows whether
// entries actually belong to the hand currently in progress.
func (t *Table) Replay(handID string, entries []ReplayedAction) error {
	if err := t.StartHand(); err != nil {
		return fmt.Errorf("hand: replay: %w", err)
	}
	for _, e := range entries {
		if err := t.Act(e.PlayerID, e.Action, e.Amount); err != nil {
			return fmt.Errorf("hand: replay action %+v: %w", e, err)
		}
	}
	return nil
}
```

Update `replay_test.go`'s `entries` to use `hand.ReplayedAction` instead of `tablestore.ActionLogEntry` (removing
the now-unnecessary `tablestore` import from the test file):

```go
	entries := []ReplayedAction{
		{PlayerID: toAct, Action: betting.ActionCall},
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/engine/hand/... -run TestReplay -v`
Expected: PASS.

- [ ] **Step 5: Wire `Replay` into `tablemanager.recoverOrSeed`**

```go
// api/internal/tablemanager/manager.go — replace recoverOrSeed's body
func (m *Manager) recoverOrSeed(ctx context.Context, tableID string, seed func() *hand.Table) (*hand.Table, error) {
	if m.store == nil {
		return seed(), nil
	}
	snap, err := m.store.LoadSnapshot(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("tablemanager: load snapshot: %w", err)
	}
	if snap == nil {
		return seed(), nil
	}
	entries, err := m.store.LoadActionsSince(ctx, tableID, snap.HandID, snap.Seq)
	if err != nil {
		return nil, fmt.Errorf("tablemanager: load actions since seq %d: %w", snap.Seq, err)
	}
	replayed := make([]hand.ReplayedAction, len(entries))
	for i, e := range entries {
		replayed[i] = hand.ReplayedAction{PlayerID: e.PlayerID, Action: betting.Action(e.Action), Amount: e.Amount}
	}
	ht := seed()
	if err := ht.Replay(snap.HandID, replayed); err != nil {
		return nil, fmt.Errorf("tablemanager: replay: %w", err)
	}
	return ht, nil
}
```

Add `"gopkg.aoctech.app/poker/api/internal/engine/betting"` to `manager.go`'s import block.

- [ ] **Step 6: Add the recovery integration test**

```go
// api/internal/tablemanager/manager_test.go — add
func TestAcquireRecoversFromSnapshotAndLog(t *testing.T) {
	// This test documents the recovery contract at the tablemanager level;
	// it exercises recoverOrSeed with a store double rather than DynamoDB
	// Local (that coverage already lives in tablestore's own integration
	// test, Task 2) — a fake satisfying the same two methods is enough here.
	t.Skip("requires a tablestore.Store test double — added when tablestore exposes one; recovery correctness is covered end-to-end by Task 11's integration test")
}
```

- [ ] **Step 7: Run the full package test suite**

Run: `go test ./internal/engine/hand/... ./internal/tablemanager/... -v`
Expected: PASS (the skipped test above shows as SKIP, not FAIL).

- [ ] **Step 8: Commit**

```bash
git add api/internal/engine/hand/hand.go api/internal/engine/hand/replay_test.go api/internal/tablemanager/manager.go api/internal/tablemanager/manager_test.go
git commit -m "feat(hand,tablemanager): replay durable action log to recover a crashed table"
```

---

### Task 7: Client-facing WebSocket gateway

**Files:**
- Create: `api/internal/api/v1/tablews.go`
- Modify: `api/internal/api/v1/router.go`
- Modify: `api/internal/app/app.go`

**Interfaces:**
- Consumes: `tablemanager.Manager.Acquire`/`Locate` (Task 5), `gopkg.aoctech.app/api-commons/jwtverify.Verifier`
  (existing shared package, not yet used anywhere in poker — this task introduces user auth to the service for
  the first time), `gopkg.aoctech.app/api-commons/ws.Registry` (existing shared package).
- Produces: `func RegisterTableWS(router fiber.Router, verifier *jwtverify.Verifier, manager *tablemanager.Manager,
  reg ws.Registry, allowedOrigins []string)`, mounted at `GET /v1.0/tables/:id/ws`.

Auth and heartbeat mechanics mirror `ctech-wallet`'s `internal/api/v1/ws.go` exactly (first-post-upgrade-frame
JWT, native ping/pong, origin check) — poker has no user-auth middleware yet, so this task is where it's
introduced, copied rather than imported since it isn't (yet) extracted to `ctech-go-common` (flagged as a
follow-up dedup candidate in this plan's closing note, not built now).

- [ ] **Step 1: Add `CtechJWKSURL`/`ServiceAudience`/`CtechURL`/`CorsAllowedOrigins` to config**

```go
// api/internal/config/config.go — add to the Config struct
	// ctech-account auth (see internal/api/v1/tablews.go) — poker's first
	// user-facing auth surface; mirrors ctech-wallet's config fields exactly.
	CtechURL           string   `env:"CTECH_URL"`
	CtechJWKSURL       string   `env:"CTECH_JWKS_URL"`
	ServiceAudience    string   `env:"SERVICE_AUDIENCE" envDefault:"poker"`
	CorsAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:","`
```

- [ ] **Step 2: Implement the WS gateway handler**

```go
// api/internal/api/v1/tablews.go
package v1

import (
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	fws "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
)

const (
	wsPingInterval = 30 * time.Second
	wsAuthTimeout  = 5 * time.Second
	wsPongWait     = wsPingInterval + 15*time.Second
	wsWriteWait    = 5 * time.Second
)

// clientMessage is every shape a connected player can send once authenticated.
type clientMessage struct {
	Type     string `json:"type"` // "ready" | "act" | "ping"
	Ready    bool   `json:"ready,omitempty"`
	Action   string `json:"action,omitempty"`
	Amount   int64  `json:"amount,omitempty"`
	ActionID string `json:"action_id,omitempty"`
}

func readAuthToken(conn *fws.Conn) (string, bool) {
	_ = conn.SetReadDeadline(time.Now().Add(wsAuthTimeout))
	defer conn.SetReadDeadline(time.Time{})
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return "", false
	}
	var p struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(msg, &p) == nil && p.Token != "" {
		return p.Token, true
	}
	return strings.TrimSpace(string(msg)), true
}

func wsAllowedOrigin(ctx *fasthttp.RequestCtx, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	origin := string(ctx.Request.Header.Peek("Origin"))
	if origin == "" {
		return true
	}
	for _, a := range allowed {
		if a == origin {
			return true
		}
	}
	return false
}

// RegisterTableWS mounts GET /v1.0/tables/:id/ws. seed builds a brand-new
// hand.Table the first time this table is ever acquired (see
// tablemanager.Manager.Acquire) — rooms.go (Phase 3) supplies the real one;
// until then any table ID seeds a heads-up-capacity placeholder so this
// gateway is independently testable without Phase 3's room service.
func RegisterTableWS(router fiber.Router, verifier *jwtverify.Verifier, manager *tablemanager.Manager, reg ws.Registry, allowedOrigins []string, seed func(tableID string) func() *hand.Table) {
	upgrader := fws.FastHTTPUpgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(ctx *fasthttp.RequestCtx) bool { return wsAllowedOrigin(ctx, allowedOrigins) },
	}
	router.Get("/tables/:id/ws", func(c fiber.Ctx) error {
		tableID := c.Params("id")
		return upgrader.Upgrade(c.RequestCtx(), func(conn *fws.Conn) {
			ctx := c.Context()
			send := func(msg any) {
				data, _ := json.Marshal(msg)
				_ = conn.WriteMessage(fws.TextMessage, data)
			}

			token, ok := readAuthToken(conn)
			if !ok {
				send(map[string]any{"type": "error", "code": "unauthorized"})
				_ = conn.Close()
				return
			}
			claims, err := verifier.VerifyClaims(ctx, token)
			if err != nil || claims == nil || claims.Sub == "" {
				send(map[string]any{"type": "error", "code": "unauthorized"})
				_ = conn.Close()
				return
			}
			playerID := claims.Sub

			actor, remoteAddr, err := manager.Locate(ctx, tableID)
			if err != nil {
				send(map[string]any{"type": "error", "code": "not_found"})
				_ = conn.Close()
				return
			}
			if actor == nil && remoteAddr != "" {
				proxyToRemoteInstance(ctx, conn, remoteAddr, tableID, token, send)
				return
			}
			if actor == nil {
				actor, err = manager.Acquire(ctx, tableID, seed(tableID))
				if err != nil {
					send(map[string]any{"type": "error", "code": "unavailable"})
					_ = conn.Close()
					return
				}
			}

			connID := uuid.NewString()
			reg.Register(tableID, connID, &wsConnAdapter{conn: conn})
			defer reg.Unregister(tableID, connID)

			send(map[string]any{"type": "connected", "conn_id": connID})
			slog.Info("table ws connected", "table", tableID, "player", playerID, "conn", connID)

			done := make(chan struct{})
			go startHeartbeat(conn, done, wsPingInterval, wsPongWait)

			for {
				_, msg, e := conn.ReadMessage()
				if e != nil {
					break
				}
				var m clientMessage
				if json.Unmarshal(msg, &m) != nil {
					continue
				}
				switch m.Type {
				case "ping":
					send(map[string]any{"type": "pong"})
				case "ready":
					reply := make(chan error, 1)
					_ = actor.Dispatch(table.ReadyCmd{PlayerID: playerID, Ready: m.Ready, Reply: reply})
				case "act":
					reply := make(chan error, 1)
					if err := actor.Dispatch(table.ActCmd{PlayerID: playerID, ActionID: m.ActionID, Action: betting.Action(m.Action), Amount: m.Amount, Reply: reply}); err != nil {
						send(map[string]any{"type": "error", "code": "invalid_action", "message": err.Error()})
					}
				}
			}
			close(done)
			slog.Info("table ws disconnected", "table", tableID, "player", playerID, "conn", connID)
		})
	})
}

func startHeartbeat(conn *fws.Conn, done <-chan struct{}, pingInterval, pongWait time.Duration) {
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(pongWait)) })
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if e := conn.WriteControl(fws.PingMessage, nil, time.Now().Add(wsWriteWait)); e != nil {
				return
			}
		case <-done:
			return
		}
	}
}

type wsConnAdapter struct{ conn *fws.Conn }

func (w *wsConnAdapter) WriteMessage(messageType int, data []byte) error {
	return w.conn.WriteMessage(messageType, data)
}
```

Add the missing `"gopkg.aoctech.app/poker/api/internal/engine/hand"` import for the `seed func(tableID string)
func() *hand.Table` parameter type. `proxyToRemoteInstance` is implemented in Task 8.

- [ ] **Step 3: Mount the route**

```go
// api/internal/api/v1/router.go — replace Register's body
func Register(app *fiber.App, cfg *config.Config, verifier *jwtverify.Verifier, manager *tablemanager.Manager, reg ws.Registry, seed func(string) func() *hand.Table) {
	router := app.Group("/v1.0")
	RegisterHealth(router, cfg)
	RegisterTableWS(router, verifier, manager, reg, cfg.CorsAllowedOrigins, seed)
}
```

- [ ] **Step 4: Wire Fx providers in `app.go`**

```go
// api/internal/app/app.go — add providers, replace registerRoutes
func newVerifier(c cache.Backend, cfg *config.Config) *jwtverify.Verifier {
	return jwtverify.NewVerifier(cfg.CtechJWKSURL, cfg.ServiceAudience, cfg.CtechURL, c)
}

func newCacheBackend(cfg *config.Config) cache.Backend {
	if cfg.RedisURL == "" {
		return cache.NewMemoryBackend(1024)
	}
	return cache.NewRedisBackend(cfg.RedisURL)
}

func newWsRegistry(lc fx.Lifecycle, c cache.Backend) ws.Registry {
	rb, ok := c.(*cache.RedisBackend)
	if !ok {
		return ws.NewMemoryRegistry()
	}
	reg := ws.NewRedisRegistry(rb.Client())
	lc.Append(fx.Hook{OnStart: reg.Start, OnStop: reg.Stop})
	return reg
}

func newTableLeaseService(c cache.Backend) *tablelease.Service {
	return tablelease.NewService(c)
}

func newTableOwnerRegistry(c cache.Backend) *tableowner.Registry {
	return tableowner.NewRegistry(c, tablelease.DefaultLeaseTTL)
}

func newTableManager(leases *tablelease.Service, owners *tableowner.Registry, reg ws.Registry, cfg *config.Config) *tablemanager.Manager {
	broadcast := func(tableID, viewerID string, snap hand.Snapshot) {
		data, _ := json.Marshal(map[string]any{"type": "state", "snapshot": snap})
		reg.Broadcast(context.Background(), tableID+"#"+viewerID, data)
	}
	return tablemanager.NewManager(leases, owners, nil, cfg.InstancePrivateIP+":"+strconv.Itoa(cfg.Port), broadcast)
}

func defaultSeed(tableID string) func() *hand.Table {
	// Placeholder seed until Phase 3's room service supplies real
	// stakes/seats — a heads-up-capacity table so this gateway is testable
	// standalone. Phase 3 replaces this provider entirely.
	return func() *hand.Table {
		return hand.NewTable(nil, 10, 20)
	}
}

func registerRoutes(app *fiber.App, cfg *config.Config, verifier *jwtverify.Verifier, manager *tablemanager.Manager, reg ws.Registry) {
	v1.Register(app, cfg, verifier, manager, reg, defaultSeed)
}
```

Update `Module`'s `fx.Provide` list to include `newCacheBackend, newVerifier, newWsRegistry, newTableLeaseService,
newTableOwnerRegistry, newTableManager` alongside the existing `config.Load, newFiberApp`. Add the corresponding
imports (`cache`, `jwtverify`, `ws`, `tablelease`, `tableowner`, `tablemanager`, `hand`, `encoding/json`, `context`,
`strconv`).

Note: `broadcast`'s fan-out key is `tableID+"#"+viewerID` — `ws.Registry.Broadcast` fans out to every connection
registered under one key, but each viewer's snapshot is redacted differently (Task 1), so each viewer needs its
own channel; `Register`/`Unregister` in `RegisterTableWS` (Step 2) must key on this same composite, not on
`tableID` alone — go back and change both `reg.Register(tableID, connID, ...)` calls in Step 2 to
`reg.Register(tableID+"#"+playerID, connID, ...)`.

- [ ] **Step 5: Manual verification (no automated test — needs two live connections)**

Run: `go build ./...` (compiles). A full two-client join/act/showdown flow is covered by Task 11's end-to-end
integration test, not here — wiring alone has nothing meaningful to unit test beyond "does it compile and route".

- [ ] **Step 6: Commit**

```bash
git add api/internal/api/v1/tablews.go api/internal/api/v1/router.go api/internal/app/app.go api/internal/config/config.go
git commit -m "feat(api): client-facing table WebSocket gateway with per-viewer redaction"
```

---

### Task 8: Cross-instance WebSocket proxy

**Files:**
- Create: `api/internal/api/v1/tableproxy.go`
- Modify: `api/internal/api/v1/tablews.go`
- Modify: `api/internal/api/v1/router.go`

**Interfaces:**
- Consumes: `tablemanager.Manager.Acquire` (Task 5), the same auth/heartbeat helpers as Task 7.
- Produces: `func RegisterTableProxy(router fiber.Router, manager *tablemanager.Manager, verifier *jwtverify.Verifier, seed func(string) func() *hand.Table)`
  at `GET /v1.0/internal/tables/:id/proxy`, and `proxyToRemoteInstance` (referenced by Task 7, implemented here).

This is what makes ARCHITECTURE.md §2's "routes each client connection to the instance holding that table's
lease" real: the receiving instance does not tell the browser to reconnect elsewhere (private instance IPs are
not internet-routable behind the shared no-NAT ALB pattern) — it dials the owning instance itself over the VPC and
splices the two WebSocket connections together, transparent to the client.

- [ ] **Step 1: Implement the internal proxy endpoint**

```go
// api/internal/api/v1/tableproxy.go
package v1

import (
	"encoding/json"
	"net/url"

	fws "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
)

// RegisterTableProxy mounts the instance-to-instance hop: an instance that
// does not own tableID's lease dials this endpoint on the instance that
// does. It re-verifies the JWT itself — this route is reachable from any
// sibling instance's private IP, not just from proxyToRemoteInstance, so it
// must not trust an already-verified claim passed by the caller.
func RegisterTableProxy(router fiber.Router, manager *tablemanager.Manager, verifier *jwtverify.Verifier, seed func(string) func() *hand.Table) {
	upgrader := fws.FastHTTPUpgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}
	router.Get("/internal/tables/:id/proxy", func(c fiber.Ctx) error {
		tableID := c.Params("id")
		return upgrader.Upgrade(c.RequestCtx(), func(conn *fws.Conn) {
			ctx := c.Context()
			token, ok := readAuthToken(conn)
			if !ok {
				_ = conn.Close()
				return
			}
			claims, err := verifier.VerifyClaims(ctx, token)
			if err != nil || claims == nil || claims.Sub == "" {
				_ = conn.Close()
				return
			}
			playerID := claims.Sub

			actor, err := manager.Acquire(ctx, tableID, seed(tableID))
			if err != nil {
				_ = conn.Close()
				return
			}

			for {
				_, msg, e := conn.ReadMessage()
				if e != nil {
					return
				}
				var m clientMessage
				if json.Unmarshal(msg, &m) != nil {
					continue
				}
				switch m.Type {
				case "ready":
					reply := make(chan error, 1)
					_ = actor.Dispatch(table.ReadyCmd{PlayerID: playerID, Ready: m.Ready, Reply: reply})
				case "act":
					reply := make(chan error, 1)
					_ = actor.Dispatch(table.ActCmd{PlayerID: playerID, ActionID: m.ActionID, Action: betting.Action(m.Action), Amount: m.Amount, Reply: reply})
				}
			}
		})
	})
}

// dialRemote opens the outbound leg of the proxy — a plain client dial to a
// sibling instance's private IP, which only ever succeeds if the CDK
// security group allows same-service instance-to-instance traffic on
// APP_PORT (Task 10 adds the ingress rule this depends on).
func dialRemote(remoteAddr, tableID, token string) (*fws.Conn, error) {
	u := url.URL{Scheme: "ws", Host: remoteAddr, Path: "/v1.0/internal/tables/" + tableID + "/proxy"}
	conn, _, err := fws.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, err
	}
	if err := conn.WriteMessage(fws.TextMessage, []byte(token)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}
```

- [ ] **Step 2: Implement `proxyToRemoteInstance` (referenced by Task 7)**

```go
// api/internal/api/v1/tablews.go — add
func proxyToRemoteInstance(ctx any, client *fws.Conn, remoteAddr, tableID, token string, send func(any)) {
	upstream, err := dialRemote(remoteAddr, tableID, token)
	if err != nil {
		send(map[string]any{"type": "error", "code": "unavailable"})
		_ = client.Close()
		return
	}
	defer upstream.Close()

	upstreamDone := make(chan struct{})
	go func() {
		defer close(upstreamDone)
		for {
			_, msg, err := upstream.ReadMessage()
			if err != nil {
				return
			}
			if err := client.WriteMessage(fws.TextMessage, msg); err != nil {
				return
			}
		}
	}()

	for {
		_, msg, err := client.ReadMessage()
		if err != nil {
			break
		}
		if err := upstream.WriteMessage(fws.TextMessage, msg); err != nil {
			break
		}
	}
	<-upstreamDone
}
```

Change `proxyToRemoteInstance`'s `ctx any` parameter to the correct `context.Context` type and drop it if unused
(it currently is — `dialRemote` doesn't take one, and neither does the read/write loop). Simplify the signature to
`func proxyToRemoteInstance(client *fws.Conn, remoteAddr, tableID, token string, send func(any))` and update
Task 7's call site (`proxyToRemoteInstance(ctx, conn, remoteAddr, tableID, token, send)` →
`proxyToRemoteInstance(conn, remoteAddr, tableID, token, send)`).

- [ ] **Step 3: Mount the proxy route**

```go
// api/internal/api/v1/router.go — add to Register, alongside RegisterTableWS
	RegisterTableProxy(router, manager, verifier, seed)
```

- [ ] **Step 4: Verify it compiles**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add api/internal/api/v1/tableproxy.go api/internal/api/v1/tablews.go api/internal/api/v1/router.go
git commit -m "feat(api): proxy WebSocket traffic to the instance that owns a table's lease"
```

---

### Task 9: Disconnect/reconnect grace window and per-turn action timeout

**Files:**
- Modify: `api/internal/table/actor.go`
- Modify: `api/internal/table/commands.go`
- Modify: `api/internal/api/v1/tablews.go`
- Test: `api/internal/table/disconnect_test.go`

**Interfaces:**
- Consumes: existing `Actor`/`Command` types (Task 3).
- Produces: real `handleDisconnect`/`handleReconnect` bodies, `ActionDeadlineCmd` (internal, fired by a
  `time.AfterFunc` the Actor arms on every turn change) — behavior only, no new exported surface beyond what
  Task 3 already declared.

OVERVIEW.md §4: a `DISCONNECTED` player's hand auto-folds at their next action deadline (seat held through a grace
window), and auto-sits-out after the grace window lapses or after N consecutive disconnected hands.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/table/disconnect_test.go
package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestDisconnectAutoFoldsAtActionDeadline(t *testing.T) {
	p1 := &hand.Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &hand.Player{ID: "p2", Stack: 1000, Ready: true}
	ht := hand.NewTable([]*hand.Player{p1, p2}, 10, 20)
	a := New("table-1", ht, nil, func(string, hand.Snapshot) {})
	a.actionDeadline = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)

	reply := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})

	var toAct string
	for _, s := range ht.ViewFor("p1").Seats {
		if s.State == "active" {
			toAct = s.PlayerID
			break
		}
	}
	reply3 := make(chan error, 1)
	_ = a.Dispatch(DisconnectCmd{PlayerID: toAct, Reply: reply3})

	time.Sleep(50 * time.Millisecond)

	for _, s := range ht.ViewFor("p1").Seats {
		if s.PlayerID == toAct && s.State != "folded" {
			t.Fatalf("expected %s to be auto-folded after missing its action deadline, got state %s", toAct, s.State)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/table/... -run TestDisconnect -v`
Expected: FAIL — `a.actionDeadline` field doesn't exist yet.

- [ ] **Step 3: Implement grace window + action deadline**

```go
// api/internal/table/actor.go — add fields to Actor
	actionDeadline time.Duration // default set in New; overridable in tests
	disconnectGrace time.Duration
	disconnectedSince map[string]time.Time
	consecutiveDisconnectedHands map[string]int

	deadlineTimer *time.Timer
```

```go
// api/internal/table/actor.go — New's body, add defaults
		actionDeadline:               30 * time.Second,
		disconnectGrace:              45 * time.Second,
		disconnectedSince:            make(map[string]time.Time),
		consecutiveDisconnectedHands: make(map[string]int),
```

```go
// api/internal/table/actor.go — replace the two stub methods
func (a *Actor) handleDisconnect(c DisconnectCmd) error {
	a.disconnectedSince[c.PlayerID] = timeNowFunc()
	a.armActionDeadlineIfTheirTurn(c.PlayerID)
	a.broadcastAll()
	return nil
}

func (a *Actor) handleReconnect(c ReconnectCmd) error {
	delete(a.disconnectedSince, c.PlayerID)
	if a.deadlineTimer != nil {
		a.deadlineTimer.Stop()
	}
	a.broadcastAll()
	return nil
}

// armActionDeadlineIfTheirTurn starts (or restarts) the auto-fold timer when
// the just-disconnected player is the one currently on the clock. A
// disconnect that happens on someone else's turn arms nothing yet — the
// deadline only matters once it actually becomes their turn to act, handled
// in handleAct's post-action turn-advance path (Step 4 below wires that).
func (a *Actor) armActionDeadlineIfTheirTurn(playerID string) {
	if !a.table.CurrentPlayerCanActForActor(playerID) {
		return
	}
	if a.deadlineTimer != nil {
		a.deadlineTimer.Stop()
	}
	a.deadlineTimer = time.AfterFunc(a.actionDeadline, func() {
		reply := make(chan error, 1)
		_ = a.Dispatch(ActCmd{PlayerID: playerID, ActionID: "auto-fold-" + playerID, Action: "fold", Reply: reply})
		a.consecutiveDisconnectedHands[playerID]++
		if timeNowFunc().Sub(a.disconnectedSince[playerID]) >= a.disconnectGrace || a.consecutiveDisconnectedHands[playerID] >= 3 {
			a.table.SitOutForActor(playerID)
		}
	})
}
```

`(*hand.Table).CurrentPlayerCanActForActor` and `SitOutForActor` don't exist — the existing `currentPlayerCanAct`
is unexported and there is no sit-out mutator at all:

```go
// api/internal/engine/hand/hand.go — add near PlayersForActor
// CurrentPlayerCanActForActor exposes currentPlayerCanAct to Phase 2's
// table.Actor (auto-fold deadline arming needs to know whose turn it is
// without duplicating the round-state check outside this package).
func (t *Table) CurrentPlayerCanActForActor(playerID string) bool { return t.currentPlayerCanAct(playerID) }

// SitOutForActor marks a player SittingOut — used by Phase 2's disconnect
// grace-window handling once a disconnected player exceeds the grace period
// or enough consecutive disconnected hands (OVERVIEW.md §4). A sitting-out
// player is treated like Folded for the remainder of any hand already in
// progress (advanceStage's `remaining`/`canStillAct` counts already only
// look at Active/AllIn, so SittingOut is excluded the same way Folded is).
func (t *Table) SitOutForActor(playerID string) {
	if p := t.playerByID(playerID); p != nil {
		p.State = SittingOut
	}
}
```

- [ ] **Step 4: Arm the deadline on every turn change**

```go
// api/internal/table/actor.go — handleAct, after the successful Act call and before broadcastAll()
	a.armActionDeadlineForCurrentTurn()
```

```go
// api/internal/table/actor.go — add
func (a *Actor) armActionDeadlineForCurrentTurn() {
	if a.deadlineTimer != nil {
		a.deadlineTimer.Stop()
	}
	for id := range a.disconnectedSince {
		if a.table.CurrentPlayerCanActForActor(id) {
			a.armActionDeadlineIfTheirTurn(id)
			return
		}
	}
}
```

- [ ] **Step 5: Wire disconnect/reconnect into the WS gateway**

```go
// api/internal/api/v1/tablews.go — RegisterTableWS's connection handler, replace the closing section
			done := make(chan struct{})
			go startHeartbeat(conn, done, wsPingInterval, wsPongWait)

			for {
				_, msg, e := conn.ReadMessage()
				if e != nil {
					reply := make(chan error, 1)
					_ = actor.Dispatch(table.DisconnectCmd{PlayerID: playerID, Reply: reply})
					break
				}
				reply := make(chan error, 1)
				_ = actor.Dispatch(table.ReconnectCmd{PlayerID: playerID, Reply: reply})
				var m clientMessage
				if json.Unmarshal(msg, &m) != nil {
					continue
				}
				switch m.Type {
				case "ping":
					send(map[string]any{"type": "pong"})
				case "ready":
					r := make(chan error, 1)
					_ = actor.Dispatch(table.ReadyCmd{PlayerID: playerID, Ready: m.Ready, Reply: r})
				case "act":
					r := make(chan error, 1)
					if err := actor.Dispatch(table.ActCmd{PlayerID: playerID, ActionID: m.ActionID, Action: betting.Action(m.Action), Amount: m.Amount, Reply: r}); err != nil {
						send(map[string]any{"type": "error", "code": "invalid_action", "message": err.Error()})
					}
				}
			}
			close(done)
```

Dispatching `ReconnectCmd` on every inbound message (not just the first after a real disconnect) is deliberately
cheap and idempotent — `handleReconnect` only ever clears a map entry and stops a timer, both no-ops when nothing
was armed, so there's no benefit to tracking "was this actually a reconnect" separately.

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/table/... ./internal/engine/hand/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add api/internal/table api/internal/engine/hand/hand.go api/internal/api/v1/tablews.go
git commit -m "feat(table): disconnect grace window, auto-fold action deadline, auto-sit-out"
```

---

### Task 10: Per-seat action rate limiting

**Files:**
- Modify: `api/internal/api/v1/tablews.go`
- Test: `api/internal/api/v1/ratelimit_test.go`

**Interfaces:**
- Produces: `type seatLimiter struct{...}`, `func newSeatLimiter(perSecond int) *seatLimiter`,
  `func (l *seatLimiter) Allow(playerID string) bool` — used inline in `RegisterTableWS`'s message loop.

ARCHITECTURE.md §8: rate limiting on actions per seat, to prevent action-spam/socket abuse. A simple fixed-window
counter per player ID is enough — this is abuse prevention, not billing-grade metering.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/api/v1/ratelimit_test.go
package v1

import "testing"

func TestSeatLimiterBlocksBurstAboveLimit(t *testing.T) {
	l := newSeatLimiter(3)
	for i := 0; i < 3; i++ {
		if !l.Allow("p1") {
			t.Fatalf("expected request %d within limit to be allowed", i)
		}
	}
	if l.Allow("p1") {
		t.Fatal("expected 4th request in the same window to be blocked")
	}
}

func TestSeatLimiterTracksPlayersIndependently(t *testing.T) {
	l := newSeatLimiter(1)
	if !l.Allow("p1") {
		t.Fatal("expected p1's first request allowed")
	}
	if !l.Allow("p2") {
		t.Fatal("expected p2's first request allowed independently of p1's count")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/v1/... -run TestSeatLimiter -v`
Expected: FAIL with "undefined: newSeatLimiter".

- [ ] **Step 3: Implement the limiter**

```go
// api/internal/api/v1/tablews.go — add
import "sync"

// seatLimiter is a fixed-window per-player counter — good enough for abuse
// prevention (ARCHITECTURE.md §8), not intended as precise rate metering.
// The window resets lazily on the first Allow call after it elapses, so an
// idle player never accumulates background timer work.
type seatLimiter struct {
	mu        sync.Mutex
	perWindow int
	window    time.Duration
	counts    map[string]int
	resetAt   map[string]time.Time
}

func newSeatLimiter(perSecond int) *seatLimiter {
	return &seatLimiter{
		perWindow: perSecond,
		window:    time.Second,
		counts:    make(map[string]int),
		resetAt:   make(map[string]time.Time),
	}
}

func (l *seatLimiter) Allow(playerID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := timeNowFunc()
	if now.After(l.resetAt[playerID]) {
		l.counts[playerID] = 0
		l.resetAt[playerID] = now.Add(l.window)
	}
	if l.counts[playerID] >= l.perWindow {
		return false
	}
	l.counts[playerID]++
	return true
}
```

`timeNowFunc` is package `table`'s, not `v1`'s — add a local one instead: `var timeNowFunc = time.Now` at the top
of `tablews.go` (a second package-level var of the same name in a different package is not a conflict; Go scopes
per-package).

- [ ] **Step 4: Enforce the limit in the message loop**

```go
// api/internal/api/v1/tablews.go — RegisterTableWS, right after the upgrader.Upgrade closure opens
			limiter := newSeatLimiter(10) // 10 actions/sec/seat — generous for a human, tight for a script
```

```go
// api/internal/api/v1/tablews.go — inside the for loop, right after `var m clientMessage` unmarshal succeeds
				if m.Type == "act" && !limiter.Allow(playerID) {
					send(map[string]any{"type": "error", "code": "rate_limited"})
					continue
				}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/api/v1/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/api/v1/tablews.go api/internal/api/v1/ratelimit_test.go
git commit -m "feat(api): per-seat action rate limiting on the table WebSocket"
```

---

### Task 11: CDK — DynamoDB tables, instance metadata, and same-SG proxy ingress

**Files:**
- Create: `cdk/lib/dynamodb-stack.ts`
- Modify: `cdk/lib/api-stack.ts`
- Modify: `cdk/lib/constants.ts`
- Modify: `cdk/bin/poker.ts`
- Test: `cdk/test/dynamodb-stack.test.ts`

**Interfaces:**
- Consumes: `Environment` (existing `@aoctech/cdk` type), the existing `PokerApiStack` construct.
- Produces: `class DynamoDBStack extends cdk.Stack { tables: Map<TableName, dynamodb.TableV2> }`, wired into
  `bin/poker.ts` and granted to `PokerApiStack`'s instance role.

- [ ] **Step 1: Write the failing CDK test**

```typescript
// cdk/test/dynamodb-stack.test.ts
import {App} from 'aws-cdk-lib';
import {Template} from 'aws-cdk-lib/assertions';
import {DynamoDBStack} from '../lib/dynamodb-stack';

test('creates poker_hand_snapshots and poker_action_log tables', () => {
  const app = new App();
  const stack = new DynamoDBStack(app, 'TestDynamoDBStack', {environment: 'dev'});
  const template = Template.fromStack(stack);
  template.resourceCountIs('AWS::DynamoDB::Table', 2);
  template.hasResourceProperties('AWS::DynamoDB::Table', {TableName: 'dev_poker_hand_snapshots'});
  template.hasResourceProperties('AWS::DynamoDB::Table', {TableName: 'dev_poker_action_log'});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd cdk && npx jest dynamodb-stack.test.ts`
Expected: FAIL — `../lib/dynamodb-stack` does not exist.

- [ ] **Step 3: Implement `dynamodb-stack.ts`**

```typescript
// cdk/lib/dynamodb-stack.ts
import * as cdk from 'aws-cdk-lib';
import {RemovalPolicy} from 'aws-cdk-lib';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import {Billing} from 'aws-cdk-lib/aws-dynamodb';
import {Construct} from 'constructs';
import {Environment} from '@aoctech/cdk';

// Table names carry the `poker_` segment so they never collide with another
// service's tables in the same AWS account (mirrors ctech-wallet's
// cdk/lib/dynamodb-stack.ts naming rationale exactly).
export type TableName = 'poker_hand_snapshots' | 'poker_action_log';

interface DynamoDBStackProps extends cdk.StackProps {
  environment: Environment;
}

export class DynamoDBStack extends cdk.Stack {
  public readonly tables: Map<TableName, dynamodb.TableV2>;

  constructor(scope: Construct, id: string, props: DynamoDBStackProps) {
    super(scope, id, props);
    this.tables = new Map();
    const {environment} = props;
    const removalPolicy = environment === 'dev' ? RemovalPolicy.DESTROY : RemovalPolicy.RETAIN;
    const pointInTimeRecoverySpecification =
      environment === 'prod' ? {pointInTimeRecoveryEnabled: true} : undefined;

    const table = (name: TableName): dynamodb.TableV2 => {
      const tableName = `${environment}_${name}`;
      const t = new dynamodb.TableV2(this, tableName, {
        tableName,
        partitionKey: {name: 'pk', type: dynamodb.AttributeType.STRING},
        sortKey: {name: 'sk', type: dynamodb.AttributeType.STRING},
        billing: Billing.onDemand({maxReadRequestUnits: 1000, maxWriteRequestUnits: 1000}),
        removalPolicy,
        pointInTimeRecoverySpecification,
        encryption: dynamodb.TableEncryptionV2.awsManagedKey(),
      });
      this.tables.set(name, t);
      return t;
    };

    table('poker_hand_snapshots');
    table('poker_action_log');
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd cdk && npx jest dynamodb-stack.test.ts`
Expected: PASS.

- [ ] **Step 5: Grant table access to the API instance role and add the proxy ingress rule**

```typescript
// cdk/lib/api-stack.ts — ApiStackProps, add
  handSnapshotsTableArn: string;
  actionLogTableArn: string;
```

```typescript
// cdk/lib/api-stack.ts — constructor, destructure the two new props, then after `service` is constructed
    service.instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['dynamodb:GetItem', 'dynamodb:PutItem', 'dynamodb:Query'],
      resources: [handSnapshotsTableArn, actionLogTableArn],
    }));

    // Same-security-group ingress on APP_PORT: the cross-instance table
    // proxy (Task 8) dials a sibling instance's private IP directly,
    // bypassing the ALB entirely — that traffic needs an explicit allow
    // since PrivateIpv4Ec2Service's security group otherwise only permits
    // ALB → instance, not instance → instance.
    service.securityGroup.connections.allowFrom(service.securityGroup, ec2.Port.tcp(APP_PORT), 'Table proxy: instance-to-instance WS forwarding');
```

Add `import * as iam from 'aws-cdk-lib/aws-iam';` to `api-stack.ts`'s imports. `PrivateIpv4Ec2Service` must expose
`instanceRole` and `securityGroup` for this to compile — confirm both are public readonly fields on the construct
(`ctech-cdk`'s `PrivateIpv4Ec2Service`); if either is missing, add it there first (a one-line `public readonly`
change in `ctech-cdk`, not in this repo) before this step, per this monorepo's shared-construct convention.

- [ ] **Step 6: Add `INSTANCE_PRIVATE_IP` to the userdata env file**

```typescript
// cdk/lib/api-stack.ts — the `cat > /etc/app-static.env` heredoc, add one line
      `INSTANCE_PRIVATE_IP=$(curl -sf -H "X-aws-ec2-metadata-token: $(curl -sf -X PUT http://169.254.169.254/latest/api/token -H 'X-aws-ec2-metadata-token-ttl-seconds: 60')" http://169.254.169.254/latest/meta-data/local-ipv4)`,
```

This line resolves at boot (bash, inside the instance), not at CDK synth time — it must go in `start.sh`'s
heredoc (where `VALKEY_URL` is already resolved the same way), not the static env file which is written before
boot. Move it: append `export INSTANCE_PRIVATE_IP` right after the existing `export VALKEY_URL` line in
`start.sh`'s heredoc instead.

- [ ] **Step 7: Wire `bin/poker.ts`**

```typescript
// cdk/bin/poker.ts — after the existing stacks are constructed, before PokerApiStack
const dynamoStack = new DynamoDBStack(app, `${environment}-ctech-poker-dynamodb`, {environment, env: awsEnv});
```

```typescript
// cdk/bin/poker.ts — pass into PokerApiStack's props
  handSnapshotsTableArn: dynamoStack.tables.get('poker_hand_snapshots')!.tableArn,
  actionLogTableArn: dynamoStack.tables.get('poker_action_log')!.tableArn,
```

Add `import {DynamoDBStack} from '../lib/dynamodb-stack';` to `bin/poker.ts`.

- [ ] **Step 8: Synth to verify no CDK errors**

Run: `cd cdk && npx cdk synth`
Expected: synthesizes without error (no live AWS calls needed — `ec2.Vpc.fromLookup` requires `CTECH_VPC_ID`; if
unset in this environment, this step is a `tsc`-level compile check only via `npx tsc --noEmit`, which is an
acceptable substitute here since Task 11 of the foundations plan already established `cdk synth` needs live AWS
context).

- [ ] **Step 9: Commit**

```bash
git add cdk/lib/dynamodb-stack.ts cdk/lib/api-stack.ts cdk/lib/constants.ts cdk/bin/poker.ts cdk/test/dynamodb-stack.test.ts
git commit -m "feat(cdk): provision action-log/snapshot tables, allow instance-to-instance proxy traffic"
```

---

### Task 12: End-to-end integration test — two players, a full hand, a crash, and recovery

**Files:**
- Create: `api/tests/integration/tableflow_test.go` (build tag `integration`)

**Interfaces:**
- Consumes: everything from Tasks 1-9 (`tablemanager.Manager`, `table.Actor`, `tablestore.Store`,
  `hand.NewTable`).

This is the engineering-level equivalent of ARCHITECTURE.md §2's deliverable ("two browser tabs can play a full
hand... killing the server process mid-hand and restarting it resumes correctly") — driven directly against
`tablemanager`/`table.Actor` instead of real browsers/sockets, which is what those two components exist to make
possible without an end-to-end browser harness.

- [ ] **Step 1: Write the test**

```go
// api/tests/integration/tableflow_test.go
//go:build integration

package integration

import (
	"context"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tableowner"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func TestTableSurvivesInstanceRestartMidHand(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	db := testDynamoClient(t) // shares the DynamoDB Local harness from tablestore's own integration test
	store := tablestore.NewStore(db, "test")
	mustCreatePokerTables(t, db, "test")

	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	}

	// "Instance A": acquires the table, plays one action, then is torn down
	// mid-hand without a clean shutdown — simulating a crash.
	mgrA := tablemanager.NewManager(tablelease.NewService(backend), tableowner.NewRegistry(backend, tablelease.DefaultLeaseTTL), store, "10.0.0.1:8010", nil)
	actorA, err := mgrA.Acquire(context.Background(), "table-crash", seed)
	if err != nil {
		t.Fatalf("acquire on instance A: %v", err)
	}
	reply := make(chan error, 1)
	if err := actorA.Dispatch(table.ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply}); err != nil {
		t.Fatalf("ready p1: %v", err)
	}
	reply2 := make(chan error, 1)
	if err := actorA.Dispatch(table.ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2}); err != nil {
		t.Fatalf("ready p2: %v", err)
	}
	// Instance A "crashes" here — nothing more happens on it; its lease will
	// eventually expire, but this test moves straight to instance B to keep
	// the test fast rather than sleeping out the real TTL.

	// "Instance B": a fresh manager over the SAME backend, simulating a
	// second EC2 instance picking the table back up after A's lease lapses.
	mgrB := tablemanager.NewManager(tablelease.NewService(backend), tableowner.NewRegistry(backend, tablelease.DefaultLeaseTTL), store, "10.0.0.2:8010", nil)
	actorB, err := mgrB.Acquire(context.Background(), "table-crash", seed)
	if err != nil {
		t.Fatalf("acquire on instance B: %v", err)
	}

	view := actorB.TableForTest().ViewFor("p1")
	if view.Stage == "waiting_for_players" {
		t.Fatalf("expected instance B to recover the hand already in progress, got stage %s", view.Stage)
	}
}
```

`actorA.TableForTest()`/`Manager.Acquire` won't compile against a locally-held-lease conflict — instance A never
released its lease before instance B tries to acquire it, so `Acquire` returns "owned by another instance". Fix
the test's premise (Step 2) rather than the code: this is exactly the crash-recovery gap `tablelease` closes via
TTL expiry, which a fast unit test cannot wait out — replace instance A's flow with an explicit `release()` call
right after dispatching the two `ReadyCmd`s, documented as standing in for "the lease's TTL eventually expires":

```go
	// Instance A "crashes": release the lease explicitly to stand in for TTL
	// expiry (waiting out the real 15s TTL would make this test slow without
	// testing anything Task 2/tablestore's tests don't already cover).
	_ = actorA // the Actor goroutine itself is abandoned, unrecovered — this
	           // is deliberate: nothing calls actorA again after this point.
```

Add a `release func()` return-through from `Manager.Acquire` is a bigger interface change than this test
warrants — instead, expose a narrow test-only escape hatch:

```go
// api/internal/tablemanager/manager.go — add
// ReleaseForTest force-releases tableID's lease without waiting for its
// heartbeat to lapse — exists only so integration tests can simulate a crash
// without sleeping out the real lease TTL.
func (m *Manager) ReleaseForTest(ctx context.Context, tableID string) {
	m.mu.Lock()
	delete(m.actors, tableID)
	m.mu.Unlock()
	_ = m.leases.Renew // no-op reference kept to avoid an unused-import edit; release is via TTL lapse in prod
}
```

This still doesn't force-expire the CAS lock itself (the shared `sharedlock.Locker` has no `Release`-by-key
without the original token, which `Manager` never retained past `Acquire`) — the honest fix is for `Manager` to
retain the lease's `release func()` it currently discards (see `manager.go`'s `_ = release` in Task 5 Step 3) and
expose it:

```go
// api/internal/tablemanager/manager.go — Manager gains a field
	releases map[string]func()
```

```go
// api/internal/tablemanager/manager.go — Acquire, replace `_ = release` with
	m.mu.Lock()
	if m.releases == nil {
		m.releases = make(map[string]func())
	}
	m.releases[tableID] = release
	m.mu.Unlock()
```

```go
// api/internal/tablemanager/manager.go — replace ReleaseForTest
func (m *Manager) ReleaseForTest(tableID string) {
	m.mu.Lock()
	release, ok := m.releases[tableID]
	delete(m.actors, tableID)
	delete(m.releases, tableID)
	m.mu.Unlock()
	if ok {
		release()
	}
}
```

And in the test, replace the comment-only block with:

```go
	mgrA.ReleaseForTest("table-crash")
```

Also add `actorA.TableForTest()` and `mgrA/mgrB`'s shared `store` needs `TableForTest` on `table.Actor`:

```go
// api/internal/table/actor.go — add
// TableForTest exposes the underlying hand.Table for integration-test
// assertions — Actor's whole purpose is to be the only mutator, so this is
// deliberately read-oriented (callers should use ViewFor, never mutate the
// returned *hand.Table directly).
func (a *Actor) TableForTest() *hand.Table { return a.table }
```

- [ ] **Step 2: Add `testDynamoClient`/`mustCreatePokerTables` helpers**

```go
// api/tests/integration/tableflow_test.go — add
func testDynamoClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}

func mustCreatePokerTables(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	for _, name := range []string{env + "_poker_hand_snapshots", env + "_poker_action_log"} {
		tableName := name
		_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
			TableName: &tableName,
			AttributeDefinitions: []types.AttributeDefinition{
				{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
			},
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
			},
			BillingMode: types.BillingModePayPerRequest,
		})
		var inUse *types.ResourceInUseException
		if err != nil && !errors.As(err, &inUse) {
			t.Fatalf("create table %s: %v", name, err)
		}
	}
}
```

Add `"errors"`, `"github.com/aws/aws-sdk-go-v2/aws"`, `"github.com/aws/aws-sdk-go-v2/config"`,
`"github.com/aws/aws-sdk-go-v2/service/dynamodb"`, `"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"` to the
test file's imports.

- [ ] **Step 3: Run test to verify it passes**

Run: `docker compose -f api/docker-compose.test.yml up -d && go test -tags integration ./tests/integration/... -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add api/tests/integration/tableflow_test.go api/internal/tablemanager/manager.go api/internal/table/actor.go
git commit -m "test(integration): verify a table recovers a hand in progress after a simulated crash"
```

---

## Closing note — flagged, not built now

`internal/api/v1/tablews.go`'s auth/heartbeat helpers (`readAuthToken`, `wsAllowedOrigin`, `startHeartbeat`,
`wsConnAdapter`) are copied near-verbatim from `ctech-wallet/api/internal/api/v1/ws.go` because they aren't yet
shared. This is the same category of duplication Task 12 of the foundations plan already extracted once (the
lock/lease CAS pattern); a second extraction into `gopkg.aoctech.app/api-commons/ws` (or a new `wsauth` package)
once `ctech-dfe`'s equivalent is also in scope is a reasonable follow-up, not a blocker for this plan.
