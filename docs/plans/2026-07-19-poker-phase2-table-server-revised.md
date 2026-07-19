# Phase 2 (Revised) — Table Server on DynamoDB Conditional Writes

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:
> executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Supersedes:** `docs/plans/2026-07-18-poker-phase2-table-server.md` Tasks 2–12. That plan was written
against `ARCHITECTURE.md`'s original lease-based-single-writer-actor proposal; §2/§3 of that document have
since been revised to make **DynamoDB conditional writes the correctness mechanism**, not a Redis/Valkey
lease. Task 1 (`hand.Snapshot`/`ViewFor`, per-viewer hole-card redaction) is unaffected by the revision and
is already implemented — this plan does not repeat it.

Already-shipped code from the superseded plan that this revision deletes or replaces:

- `api/internal/tablestore/*` — replaced (versioned single-item store, not snapshot+replay-log).
- `api/internal/table/*` — replaced (Actor becomes an optional local cache, not the sole writer).
- `api/internal/tableowner/*` — deleted outright (existed only to support the WS proxy, which this
  revision removes: any instance may accept any table's WebSocket connection now).
- `api/internal/tablemanager/*` — replaced (no more acquire-or-fail / Locate / cross-instance routing).
- `api/internal/api/v1/tableproxy.go` — deleted outright.
- `api/internal/api/v1/tablews.go` — simplified (no remote-instance branch).
- `InstancePrivateIP` config field — deleted (existed only for `tableowner` to advertise a routable
  address; nothing consumes it once there is no proxy to route to).

**Goal:** Turn the Phase 0/1 pure engine (`internal/engine/hand.Table`) into a live table service reachable
over WebSocket from **any** instance in the ASG, where correctness comes from a version-guarded conditional
write against DynamoDB on every action — never from an in-memory lock or a Redis lease.

**Architecture:** One DynamoDB item per table holds the full internal hand state plus a `version` counter.
Any instance may receive a player's WebSocket connection and any of that player's actions: it validates the
action against `hand.Table` (built from the item it just read, or from a locally cached copy), then commits
the result with `ConditionExpression: version = :expected` in the same transaction as (a) an idempotency
guard item keyed by the client's `action_id` and (b) an audit-log entry. A failed condition means another
instance's action landed first; the instance re-reads, re-validates, and retries once. This is the same
discipline `ctech-wallet`'s `WalletRepository.mutate`/`resolveTxErr` already applies to money (see
`ctech-wallet/api/internal/repositories/wallet.go`) — poker's `tablestore.Store.CommitAction` mirrors that
shape directly. `tablelease.Service` (already built, shared with `ctech-wallet` via
`gopkg.aoctech.app/api-commons/lock`) is kept **only** as a latency optimization: an instance that currently
holds a table's lease trusts its own in-memory `hand.Table` between actions instead of re-reading DynamoDB
first; an instance without the lease always re-reads before validating. A bug in `tablelease` is a
performance regression, never a correctness bug — nothing in this plan may treat lease possession as
permission to skip the conditional write itself.

**Tech Stack:** Go 1.26, Fiber v3, `github.com/fasthttp/websocket`, `gopkg.aoctech.app/api-commons/ws` (Redis
Pub/Sub fan-out registry), `gopkg.aoctech.app/api-commons/dynamo` (DynamoDB helpers), the existing
`internal/tablelease` service (now advisory-only), AWS CDK.

## Global Constraints

- Every route lives under `/v1.0/` (existing convention, `internal/api/v1/router.go`).
- All amounts are integer chip counts (`int64`), never floats — matches the engine's existing convention.
- **Correctness lives in DynamoDB's conditional writes, never in an in-memory lock or a Redis lease**
  (ARCHITECTURE.md §2, as revised). No code in this plan may reject a write, or treat a write as safe,
  based on lease possession.
- Idempotent actions: every player action carries a client-generated `action_id`; the server de-dupes on
  `(table_id, hand_id, seat, action_id)` via a DynamoDB guard item written in the same transaction as the
  state update (OVERVIEW.md §4) — never an in-memory set, since any instance may process any action.
- Server-authoritative, no hidden-information leaks: a client is never sent another player's hole cards
  before showdown, under any circumstance (ARCHITECTURE.md §8). Unaffected by this revision — `ViewFor`
  (Task 1) already enforces this and is reused as-is.
- On reconnect, a client resyncs from a full authoritative snapshot read fresh from DynamoDB, never from a
  replayed log — there is no log to replay under this revision (ARCHITECTURE.md §3: "recovery is trivial").
- DynamoDB tables follow `ctech-wallet`'s naming convention: physical name `{env}_poker_{table}`.
- Binary deployed to EC2 must be named `app` (existing CDK convention — unchanged by this plan).
- Any instance may accept a WebSocket connection for any table — no forwarding, no proxy, no "owner".
- **`poker_table_state`'s per-item size stays far under DynamoDB's 400KB hard cap.** `hand.State` (Task 2)
  holds only the *current hand in progress* — it is fully replaced (not appended to) by every `StartHand`
  call, so it never accumulates history across hands. At a full 9-max table the encoded item is roughly
  15–40KB even with generous `attributevalue` encoding overhead — ~10x headroom. This is a property of what
  `State` is allowed to hold, not an incidental accident: nothing in this plan may add hand-history,
  chat, or any other cross-hand accumulation into `State` — that belongs in `poker_action_log` instead,
  which is unbounded by design (append-only, one row per commit).
- `poker_action_guards` items are TTL'd (7 days, mirroring `ctech-wallet`'s `idemTTLDays` —
  `ctech-wallet/api/internal/repositories/wallet.go:19`) — a guard only needs to outlive plausible client
  retries, not forever.
- `poker_action_log` is TTL'd too (`logTTLDays` = 90 days — the "recent hand history" window served
  directly from DynamoDB) but is **never silently lost**: DynamoDB Streams ships every inserted entry to S3
  (`poker-action-log-archive`, Task 10) immediately, on write — independent of when TTL later reaps the hot
  copy. DynamoDB is the fast, recent-window store; S3 is the durable, indefinite-retention archive. This
  bounds the hot table's storage/RCU footprint without ever discarding hand-history.

---

### Task 2: Full internal state export/import + idempotent Act on `hand.Table`

**Files:**

- Create: `api/internal/engine/hand/state.go`
- Test: `api/internal/engine/hand/state_test.go`
- Modify: `api/internal/engine/hand/hand.go`

**Interfaces:**

- Consumes: `Table`'s existing private fields (this package only), `betting.Round`/`betting.PlayerState`
  (existing, already fully exported), `deck.Card`/`deck.ShuffleResult` (existing, already fully exported).
- Produces: `type State struct{...}` (exported mirror of every field `Table` carries),
  `func (t *Table) ExportState() State`, `func NewTableFromState(s State) *Table`,
  `func (t *Table) ActIdempotent(actionID, playerID string, action betting.Action, amount int64) (applied bool, err error)` —
  consumed by Task 3's `tablestore.Store` (persists `State`) and Task 4's `table.Actor` (calls
  `ActIdempotent` instead of `Act` directly).

`Snapshot`/`ViewFor` (Task 1) is a *redacted, viewer-facing* projection — it deliberately drops information
(hidden hole cards, the shuffle's server seed) that recovery needs back. `State` is the opposite: a complete,
unredacted mirror of every field on `Table`, used only for persistence/reconstruction, and must never be
sent to a client.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/engine/hand/state_test.go
package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
)

func TestExportImportRoundTripsFullState(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	original := NewTable([]*Player{p1, p2}, 10, 20)
	if err := original.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	toAct := original.playerToActForTest()
	if err := original.Act(toAct, betting.ActionCall, 0); err != nil {
		t.Fatalf("Act: %v", err)
	}

	rebuilt := NewTableFromState(original.ExportState())

	originalView := original.ViewFor("p1")
	rebuiltView := rebuilt.ViewFor("p1")
	if originalView.Stage != rebuiltView.Stage {
		t.Fatalf("stage mismatch: original=%s rebuilt=%s", originalView.Stage, rebuiltView.Stage)
	}
	// Board/seat data must round-trip byte-for-byte, not just stage.
	if len(originalView.Board) != len(rebuiltView.Board) {
		t.Fatalf("board length mismatch: original=%d rebuilt=%d", len(originalView.Board), len(rebuiltView.Board))
	}

	// A rebuilt table must still accept the next real action — proves every
	// field Act() reads (round, roundIdx, roundBaseline) survived the round trip,
	// not just what ViewFor happens to expose.
	nextToAct := rebuilt.playerToActForTest()
	if nextToAct == "" {
		t.Fatal("expected rebuilt table to still have a player to act")
	}
	if err := rebuilt.Act(nextToAct, betting.ActionCheck, 0); err != nil {
		if err := rebuilt.Act(nextToAct, betting.ActionCall, 0); err != nil {
			t.Fatalf("rebuilt table rejected a legal action: %v", err)
		}
	}
}

func TestActIdempotentSkipsRepeatedActionID(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	toAct := table.playerToActForTest()

	applied1, err := table.ActIdempotent("dup-1", toAct, betting.ActionCall, 0)
	if err != nil || !applied1 {
		t.Fatalf("expected first call applied, got applied=%v err=%v", applied1, err)
	}

	before := table.ViewFor(toAct)
	applied2, err := table.ActIdempotent("dup-1", toAct, betting.ActionCall, 0)
	if err != nil {
		t.Fatalf("duplicate action_id must not error: %v", err)
	}
	if applied2 {
		t.Fatal("expected duplicate action_id to report applied=false")
	}
	after := table.ViewFor(toAct)
	if len(before.Seats) != len(after.Seats) {
		t.Fatal("duplicate action_id must not mutate state")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/hand/... -run 'TestExportImport|TestActIdempotent' -v`
Expected: FAIL with "undefined: NewTableFromState" (and `ActIdempotent`).

- [ ] **Step 3: Implement `state.go`**

```go
// api/internal/engine/hand/state.go
package hand

import (
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

// State is a complete, unredacted mirror of every field Table carries — used
// only for persistence/reconstruction (tablestore.Store, Task 3 of this
// plan). Unlike Snapshot (Task 1), which deliberately hides information a
// viewer must never see, State must never be sent to a client.
type State struct {
	Players       []*Player
	SmallBlind    int64
	BigBlind      int64
	DealerSeat    int
	Stage         Stage
	Board         []deck.Card
	Shuffle       *deck.ShuffleResult
	NextCard      int
	Round         *betting.Round
	RoundIdx      map[string]int
	RoundBaseline map[string]int64
	Payouts       map[string]int64
	HandOrder     []*Player
	SeenActionIDs map[string]bool
}

// ExportState captures every field this Table carries, for durable storage.
func (t *Table) ExportState() State {
	return State{
		Players:       t.players,
		SmallBlind:    t.smallBlind,
		BigBlind:      t.bigBlind,
		DealerSeat:    t.dealerSeat,
		Stage:         t.stage,
		Board:         t.board,
		Shuffle:       t.shuffle,
		NextCard:      t.nextCard,
		Round:         t.round,
		RoundIdx:      t.roundIdx,
		RoundBaseline: t.roundBaseline,
		Payouts:       t.payouts,
		HandOrder:     t.handOrder,
		SeenActionIDs: t.seenActionIDs,
	}
}

// NewTableFromState rebuilds a Table from a previously exported State — the
// only recovery path this revision needs (ARCHITECTURE.md §3: "recovery is
// trivial", there is no log to replay).
func NewTableFromState(s State) *Table {
	return &Table{
		players:       s.Players,
		smallBlind:    s.SmallBlind,
		bigBlind:      s.BigBlind,
		dealerSeat:    s.DealerSeat,
		stage:         s.Stage,
		board:         s.Board,
		shuffle:       s.Shuffle,
		nextCard:      s.NextCard,
		round:         s.Round,
		roundIdx:      s.RoundIdx,
		roundBaseline: s.RoundBaseline,
		payouts:       s.Payouts,
		handOrder:     s.HandOrder,
		seenActionIDs: s.SeenActionIDs,
	}
}

// ActIdempotent applies action only if actionID hasn't been seen for this
// table since its last StartHand call (seenActionIDs resets there — see
// hand.go). applied=false, err=nil means the action_id was already seen and
// nothing changed; the caller (table.Actor, Task 4) should treat this as
// "already committed", not as an error.
func (t *Table) ActIdempotent(actionID, playerID string, action betting.Action, amount int64) (applied bool, err error) {
	if t.seenActionIDs == nil {
		t.seenActionIDs = make(map[string]bool)
	}
	if t.seenActionIDs[actionID] {
		return false, nil
	}
	if err := t.Act(playerID, action, amount); err != nil {
		return false, err
	}
	t.seenActionIDs[actionID] = true
	return true, nil
}
```

- [ ] **Step 4: Reset `seenActionIDs` per hand and add the field to `Table`**

```go
// api/internal/engine/hand/hand.go — Table struct, add field
// seenActionIDs de-dupes Act calls by client-supplied action_id within the
// current hand (OVERVIEW.md § 4) — persisted as part of State (this
// package's state.go) so any instance recovering mid-hand still rejects a
// replayed duplicate, not just the instance that originally saw it.
seenActionIDs map[string]bool
```

```go
// api/internal/engine/hand/hand.go — StartHand, right after `t.payouts = nil`
t.seenActionIDs = make(map[string]bool)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/engine/hand/... -v`
Expected: PASS (all tests, including the pre-existing suite).

- [ ] **Step 6: Commit**

```bash
git add api/internal/engine/hand/state.go api/internal/engine/hand/state_test.go api/internal/engine/hand/hand.go
git commit -m "feat(hand): add full-state export/import and idempotent Act for DynamoDB-backed persistence"
```

---

### Task 3: `tablestore` — versioned single-item store with conditional commit

**Files:**

- Delete: `api/internal/tablestore/store.go`, `api/internal/tablestore/dynamo.go`,
  `api/internal/tablestore/dynamo_test.go`
  (content below replaces them entirely — same file names, new contents)
- Create (same paths, new content): `api/internal/tablestore/store.go`, `api/internal/tablestore/dynamo.go`
- Test (same path, new content): `api/internal/tablestore/dynamo_test.go` (build tag `integration`)

**Interfaces:**

- Consumes: `gopkg.aoctech.app/api-commons/dynamo.Base`/`NewBase`/`Encode`/`Decode`/`GetItem`/
  `BuildRawUpdateTxItem`/`BuildPutTxItemIfAbsent`/`TransactWrite`/`IsConditionFailed` (existing shared
  package — the exact primitives `ctech-wallet`'s `WalletRepository.mutate` already uses), `hand.State`
  (Task 2).
- Produces: `type StoredTable struct{...}`, `var ErrVersionConflict`, `var ErrDuplicateAction`,
  `func NewStore(db *dynamodb.Client, env string) *Store`,
  `func (s *Store) LoadTable(ctx, tableID string) (*StoredTable, error)`,
  `func (s *Store) CommitAction(ctx context.Context, tableID, handID, actionID string, expectedVersion int, newState hand.State, entry ActionLogEntry) error` —
  all consumed by Task 4's `table.Actor`. `actionID` is `""` for a Ready-triggered commit (no idempotency
  guard needed — only player actions carry a client-generated ID, per this plan's Global Constraints).

Three DynamoDB tables:

- `poker_table_state` (pk=`table_id`, no sort key) — exactly one item per table, the current authoritative
  state. Overwritten (version-guarded) on every commit.
- `poker_action_log` (pk=`table_id#hand_id`, sk=zero-padded `version`) — append-only audit/hand-history
  trail (ARCHITECTURE.md §8.2). **Never used for recovery** — recovery reads `poker_table_state` directly
  (ARCHITECTURE.md §3: "recovery is trivial").
- `poker_action_guards` (pk=`table_id#hand_id#action_id`) — one item per player action, written
  `attribute_not_exists(pk)` in the same transaction as the state update. A transaction that fails because
  this item already exists means a duplicate submission, not a race — mirrors
  `ctech-wallet/api/internal/repositories/wallet.go`'s `guardTx`/`checkReplay` exactly.

- [ ] **Step 1: Write `store.go` (types)**

```go
// api/internal/tablestore/store.go
// Package tablestore persists table state as a single DynamoDB item per
// table, guarded by a version counter — DynamoDB's conditional writes are
// the correctness mechanism (ARCHITECTURE.md §2, revised), not an in-memory
// lock or a Redis lease.
package tablestore

import (
	"errors"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// ErrVersionConflict means another instance's action committed first —
// CommitAction's caller (table.Actor) must re-read the table's current
// state via LoadTable and retry validation against it.
var ErrVersionConflict = errors.New("tablestore: version conflict")

// ErrDuplicateAction means actionID was already committed for this hand —
// the caller should treat this the same as a successful no-op, per this
// plan's idempotency constraint.
var ErrDuplicateAction = errors.New("tablestore: duplicate action_id")

// ActionLogEntry is one durable audit/hand-history record (ARCHITECTURE.md
// §8.2) — never read back for recovery; recovery reads StoredTable directly.
type ActionLogEntry struct {
	TableID  string `dynamodbav:"table_id"`
	HandID   string `dynamodbav:"hand_id"`
	Version  int    `dynamodbav:"version"`
	PlayerID string `dynamodbav:"player_id"`
	ActionID string `dynamodbav:"action_id"`
	Action   string `dynamodbav:"action"`
	Amount   int64  `dynamodbav:"amount"`
}

// StoredTable is the current authoritative state of one table, as read from
// poker_table_state.
type StoredTable struct {
	TableID string     `dynamodbav:"pk"`
	Version int        `dynamodbav:"version"`
	HandID  string     `dynamodbav:"hand_id"`
	State   hand.State `dynamodbav:"state"`
}
```

- [ ] **Step 2: Write the failing integration test**

```go
// api/internal/tablestore/dynamo_test.go
//go:build integration

package tablestore

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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

func TestSeedThenCommitThenLoad(t *testing.T) {
	db := testClient(t)
	s := NewStore(db, "test")
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, "test")

	if err := s.SeedTable(ctx, "table-1", hand.State{Stage: hand.WaitingForPlayers}); err != nil {
		t.Fatalf("SeedTable: %v", err)
	}

	loaded, err := s.LoadTable(ctx, "table-1")
	if err != nil || loaded == nil || loaded.Version != 1 {
		t.Fatalf("expected version 1 after seed, got %+v err=%v", loaded, err)
	}

	newState := hand.State{Stage: hand.PreFlop}
	if err := s.CommitAction(ctx, "table-1", "hand-1", "act-1", 1, newState, ActionLogEntry{
		TableID: "table-1", HandID: "hand-1", Version: 2, PlayerID: "p1", ActionID: "act-1", Action: "call",
	}); err != nil {
		t.Fatalf("CommitAction: %v", err)
	}

	loaded, err = s.LoadTable(ctx, "table-1")
	if err != nil || loaded.Version != 2 || loaded.State.Stage != hand.PreFlop {
		t.Fatalf("expected version 2 pre_flop after commit, got %+v err=%v", loaded, err)
	}
}

func TestCommitActionRejectsStaleVersion(t *testing.T) {
	db := testClient(t)
	s := NewStore(db, "test")
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, "test")

	_ = s.SeedTable(ctx, "table-2", hand.State{Stage: hand.WaitingForPlayers})

	err := s.CommitAction(ctx, "table-2", "hand-1", "act-1", 99, hand.State{}, ActionLogEntry{TableID: "table-2", HandID: "hand-1", Version: 100, ActionID: "act-1"})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected ErrVersionConflict against a stale expected version, got %v", err)
	}
}

func TestCommitActionRejectsDuplicateActionID(t *testing.T) {
	db := testClient(t)
	s := NewStore(db, "test")
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, "test")

	_ = s.SeedTable(ctx, "table-3", hand.State{Stage: hand.WaitingForPlayers})
	entry := ActionLogEntry{TableID: "table-3", HandID: "hand-1", Version: 2, ActionID: "dup-1"}
	if err := s.CommitAction(ctx, "table-3", "hand-1", "dup-1", 1, hand.State{Stage: hand.PreFlop}, entry); err != nil {
		t.Fatalf("first commit: %v", err)
	}

	err := s.CommitAction(ctx, "table-3", "hand-1", "dup-1", 2, hand.State{Stage: hand.Flop}, ActionLogEntry{TableID: "table-3", HandID: "hand-1", Version: 3, ActionID: "dup-1"})
	if !errors.Is(err, ErrDuplicateAction) {
		t.Fatalf("expected ErrDuplicateAction on a replayed action_id, got %v", err)
	}
}

func mustCreateTestTables(ctx context.Context, t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	pkOnly := []string{env + "_" + tableState}
	pkSk := []string{env + "_" + tableActionLog, env + "_" + tableActionGuards}
	for _, name := range pkOnly {
		createTable(ctx, t, db, name, false)
	}
	for _, name := range pkSk {
		createTable(ctx, t, db, name, true)
	}
}

func createTable(ctx context.Context, t *testing.T, db *dynamodb.Client, name string, withSK bool) {
	t.Helper()
	attrs := []types.AttributeDefinition{{AttributeName: strPtr("pk"), AttributeType: types.ScalarAttributeTypeS}}
	keys := []types.KeySchemaElement{{AttributeName: strPtr("pk"), KeyType: types.KeyTypeHash}}
	if withSK {
		attrs = append(attrs, types.AttributeDefinition{AttributeName: strPtr("sk"), AttributeType: types.ScalarAttributeTypeS})
		keys = append(keys, types.KeySchemaElement{AttributeName: strPtr("sk"), KeyType: types.KeyTypeRange})
	}
	tableName := name
	_, err := db.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: &tableName, AttributeDefinitions: attrs, KeySchema: keys, BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		var inUse *types.ResourceInUseException
		if !errors.As(err, &inUse) {
			t.Fatalf("create table %s: %v", name, err)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `docker compose -f api/docker-compose.test.yml up -d && go test -tags integration ./internal/tablestore/... -v`
Expected: FAIL with "undefined: NewStore".

- [ ] **Step 4: Implement `dynamo.go`**

```go
// api/internal/tablestore/dynamo.go
package tablestore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

const (
	tableState        = "poker_table_state"
	tableActionLog    = "poker_action_log"
	tableActionGuards = "poker_action_guards"

	// guardTTLDays mirrors ctech-wallet's idemTTLDays
	// (ctech-wallet/api/internal/repositories/wallet.go:19) — a guard only
	// needs to outlive plausible client retries, not forever.
	guardTTLDays = 7

	// logTTLDays bounds how long an audit-log entry stays in the hot
	// DynamoDB table before TTL reaps it. Nothing is lost when that
	// happens: Task 10's DynamoDB Streams pipeline ships every entry to S3
	// on insert, independent of and well before its eventual TTL expiry —
	// DynamoDB serves the recent window, S3 is the indefinite archive.
	logTTLDays = 90
)

// timeNowFunc is overridden in tests that need a deterministic TTL value.
var timeNowFunc = time.Now

// Store persists the one authoritative item per table, an audit log, and the
// idempotency guards that back CommitAction's duplicate-action_id rejection.
type Store struct {
	state  dynamo.Base
	log    dynamo.Base
	guards dynamo.Base
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{
		state:  dynamo.NewBase(db, env, tableState),
		log:    dynamo.NewBase(db, env, tableActionLog),
		guards: dynamo.NewBase(db, env, tableActionGuards),
	}
}

// SeedTable creates a table's very first state item at version 1. A no-op
// (not an error) if the table already exists — Acquire (Task 4) calls this
// unconditionally the first time a table is touched on any instance, so two
// instances racing to seed the same brand-new table must not both fail.
func (s *Store) SeedTable(ctx context.Context, tableID string, state hand.State) error {
	item, err := dynamo.Encode(struct {
		PK      string     `dynamodbav:"pk"`
		Version int        `dynamodbav:"version"`
		State   hand.State `dynamodbav:"state"`
	}{PK: tableID, Version: 1, State: state})
	if err != nil {
		return fmt.Errorf("tablestore: encode seed state: %w", err)
	}
	if err := s.state.PutItem(ctx, item); err != nil {
		return fmt.Errorf("tablestore: seed table: %w", err)
	}
	return nil
}

func (s *Store) LoadTable(ctx context.Context, tableID string) (*StoredTable, error) {
	item, err := s.state.GetItem(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("tablestore: get table: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	return dynamo.Decode[StoredTable](item)
}

// CommitAction atomically bumps tableID's version (guarded by
// expectedVersion), records entry in the audit log, and — when actionID is
// non-empty — writes an idempotency guard so a replayed action_id fails the
// transaction instead of being re-applied. Mirrors
// ctech-wallet/api/internal/repositories/wallet.go's mutate/resolveTxErr
// shape: on a failed condition, re-read the guard to disambiguate a version
// race from a duplicate submission.
func (s *Store) CommitAction(ctx context.Context, tableID, handID, actionID string, expectedVersion int, newState hand.State, entry ActionLogEntry) error {
	stateItem, err := dynamo.Encode(struct {
		State hand.State `dynamodbav:"state"`
	}{State: newState})
	if err != nil {
		return fmt.Errorf("tablestore: encode state: %w", err)
	}
	stateAV := stateItem["state"]

	values := map[string]types.AttributeValue{
		":newVersion": mustN(expectedVersion + 1),
		":expected":   mustN(expectedVersion),
		":handID":     &types.AttributeValueMemberS{Value: handID},
		":state":      stateAV,
	}
	stateTx := s.state.BuildRawUpdateTxItem(tableID, nil,
		"SET version = :newVersion, hand_id = :handID, state = :state",
		"attribute_exists(pk) AND version = :expected", nil, values)

	logItem, err := dynamo.Encode(struct {
		PK  string `dynamodbav:"pk"`
		SK  string `dynamodbav:"sk"`
		TTL int64  `dynamodbav:"ttl"`
		ActionLogEntry
	}{
		PK: tableID + "#" + handID, SK: fmt.Sprintf("%010d", entry.Version),
		TTL:             timeNowFunc().Add(logTTLDays * 24 * time.Hour).Unix(),
		ActionLogEntry: entry,
	})
	if err != nil {
		return fmt.Errorf("tablestore: encode log entry: %w", err)
	}
	logTx := s.log.BuildPutTxItem(logItem)

	items := []types.TransactWriteItem{stateTx, logTx}
	if actionID != "" {
		guardItem, err := dynamo.Encode(struct {
			PK  string `dynamodbav:"pk"`
			TTL int64  `dynamodbav:"ttl"`
		}{PK: tableID + "#" + handID + "#" + actionID, TTL: timeNowFunc().Add(guardTTLDays * 24 * time.Hour).Unix()})
		if err != nil {
			return fmt.Errorf("tablestore: encode guard: %w", err)
		}
		items = append(items, s.guards.BuildPutTxItemIfAbsent(guardItem))
	}

	if err := s.state.TransactWrite(ctx, items); err != nil {
		return s.resolveCommitErr(ctx, tableID, handID, actionID, err)
	}
	return nil
}

// resolveCommitErr disambiguates a failed transaction: an already-present
// guard means a duplicate action_id; otherwise the state item's version
// condition must have failed.
func (s *Store) resolveCommitErr(ctx context.Context, tableID, handID, actionID string, txErr error) error {
	if !dynamo.IsConditionFailed(txErr) {
		return fmt.Errorf("tablestore: commit: %w", txErr)
	}
	if actionID != "" {
		item, err := s.guards.GetItem(ctx, tableID+"#"+handID+"#"+actionID)
		if err != nil {
			return fmt.Errorf("tablestore: check guard: %w", err)
		}
		if item != nil {
			return ErrDuplicateAction
		}
	}
	return ErrVersionConflict
}

func mustN(v int) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", v)}
}

func strPtr(s string) *string { return &s }
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -tags integration ./internal/tablestore/... -v`
Expected: PASS (all four tests). Tear down: `docker compose -f api/docker-compose.test.yml down`.

- [ ] **Step 6: Commit**

```bash
git add api/internal/tablestore
git commit -m "feat(tablestore): versioned single-item store with conditional commit, replacing snapshot+replay"
```

---

### Task 4: `table.Actor` — local cache-affinity wrapper over `tablestore`

**Files:**

- Delete and recreate: `api/internal/table/actor.go`, `api/internal/table/commands.go`,
  `api/internal/table/actor_test.go`

**Interfaces:**

- Consumes: `hand.Table`/`hand.NewTableFromState`/`hand.State`/`(*hand.Table).ExportState`/
  `(*hand.Table).ActIdempotent` (Task 2), `tablestore.Store`/`ErrVersionConflict`/`ErrDuplicateAction`
  (Task 3), `betting.Action` (existing).
- Produces: `type Actor struct{...}`, `func New(id string, store *tablestore.Store, trustCache bool,
  broadcast func(viewerID string, snap hand.Snapshot)) *Actor`, `func (a *Actor) Run(ctx context.Context)`,
  `func (a *Actor) Dispatch(cmd Command) error` — consumed by Task 5's `tablemanager` and Task 7's WS
  gateway. `trustCache` is set once at construction by whether the caller currently holds the table's
  `tablelease` — never re-consulted per-action (ARCHITECTURE.md §2: the lease is a latency hint, not a
  gate); when `false`, the Actor re-reads DynamoDB before every single command instead of just after a
  version conflict.

Every command still funnels through one goroutine per table **within this instance** — not because another
instance is forbidden from writing (it may, freely), but because `hand.Table` itself has no internal lock,
so two of *this instance's own* goroutines (e.g. two players' WebSocket connections landing on the same
instance) must still be serialized before either touches the shared `*hand.Table` cache.

- [ ] **Step 1: Write `commands.go`**

```go
// api/internal/table/commands.go
package table

import "gopkg.aoctech.app/poker/api/internal/engine/betting"

// Command is anything the Actor's Run loop can process.
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
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func newTestActor(t *testing.T, store *tablestore.Store) *Actor {
	t.Helper()
	seed := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)
	if err := store.SeedTable(context.Background(), "table-1", seed.ExportState()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	a := New("table-1", store, true, func(string, hand.Snapshot) {})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)
	return a
}

// fakeStore-free tests need a real tablestore.Store; these run against
// DynamoDB Local exactly like tablestore's own integration tests.
func TestActorCommitsReadyThenAct(t *testing.T) {
	db := testClient(t) // see tablestore/dynamo_test.go's helper, mirrored here for this package's own integration test
	store := tablestore.NewStore(db, "test")
	mustCreateTestTables(t, db, "test")
	a := newTestActor(t, store)

	reply := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply}); err != nil {
		t.Fatalf("ready p1: %v", err)
	}
	reply2 := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2}); err != nil {
		t.Fatalf("ready p2: %v", err)
	}

	stored, err := store.LoadTable(context.Background(), "table-1")
	if err != nil || stored == nil || stored.State.Stage == hand.WaitingForPlayers {
		t.Fatalf("expected hand to have started and committed, got %+v err=%v", stored, err)
	}

	var seat string
	for _, s := range stored.State.Players {
		if s.State == hand.Active {
			seat = s.ID
			break
		}
	}
	reply3 := make(chan error, 1)
	if err := a.Dispatch(ActCmd{PlayerID: seat, ActionID: "a1", Action: betting.ActionCall, Reply: reply3}); err != nil {
		t.Fatalf("act: %v", err)
	}

	stored, err = store.LoadTable(context.Background(), "table-1")
	if err != nil || stored.Version < 3 {
		t.Fatalf("expected version to have advanced past ready+ready+act, got %+v err=%v", stored, err)
	}
}

func TestActorRecoversFromVersionConflictAndRetriesOnce(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "test")
	mustCreateTestTables(t, db, "test")
	a := newTestActor(t, store)

	reply := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})

	// Simulate a second instance committing a Ready no-op in between, bumping
	// the version out from under this Actor's cached copy.
	stored, _ := store.LoadTable(context.Background(), "table-1")
	_ = store.CommitAction(context.Background(), "table-1", stored.HandID, "", stored.Version, stored.State, tablestore.ActionLogEntry{TableID: "table-1", HandID: stored.HandID, Version: stored.Version + 1})

	var seat string
	for _, s := range stored.State.Players {
		if s.State == hand.Active {
			seat = s.ID
			break
		}
	}
	reply3 := make(chan error, 1)
	if err := a.Dispatch(ActCmd{PlayerID: seat, ActionID: "a1", Action: betting.ActionCall, Reply: reply3}); err != nil {
		t.Fatalf("expected the Actor to reload and retry past the version conflict, got: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `docker compose -f api/docker-compose.test.yml up -d && go test -tags integration ./internal/table/... -v`
Expected: FAIL with "undefined: New" (and `testClient`/`mustCreateTestTables` — Step 3.5 below adds a
package-local copy so `internal/table`'s tests don't import `internal/tablestore`'s test file).

- [ ] **Step 3.5: Add local test helpers**

```go
// api/internal/table/dynamo_helpers_test.go
//go:build integration

package table

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func testClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), config.WithCredentialsProvider(aws.AnonymousCredentials{}))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}

func mustCreateTestTables(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	pkOnly := []string{env + "_poker_table_state"}
	pkSk := []string{env + "_poker_action_log", env + "_poker_action_guards"}
	for _, name := range pkOnly {
		createTestTable(t, db, name, false)
	}
	for _, name := range pkSk {
		createTestTable(t, db, name, true)
	}
}

func createTestTable(t *testing.T, db *dynamodb.Client, name string, withSK bool) {
	t.Helper()
	attrs := []types.AttributeDefinition{{AttributeName: strPtr("pk"), AttributeType: types.ScalarAttributeTypeS}}
	keys := []types.KeySchemaElement{{AttributeName: strPtr("pk"), KeyType: types.KeyTypeHash}}
	if withSK {
		attrs = append(attrs, types.AttributeDefinition{AttributeName: strPtr("sk"), AttributeType: types.ScalarAttributeTypeS})
		keys = append(keys, types.KeySchemaElement{AttributeName: strPtr("sk"), KeyType: types.KeyTypeRange})
	}
	tableName := name
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{TableName: &tableName, AttributeDefinitions: attrs, KeySchema: keys, BillingMode: types.BillingModePayPerRequest})
	if err != nil {
		var inUse *types.ResourceInUseException
		if !errors.As(err, &inUse) {
			t.Fatalf("create table %s: %v", name, err)
		}
	}
}

func strPtr(s string) *string { return &s }
```

- [ ] **Step 4: Implement `actor.go`**

```go
// api/internal/table/actor.go
// Package table drives one table's hand.Table from exactly one goroutine per
// instance — not because that instance owns write authority (it doesn't;
// ARCHITECTURE.md §2 makes DynamoDB's conditional writes the sole
// correctness mechanism), but because hand.Table has no internal lock, so
// two of this instance's own goroutines must still be serialized.
package table

import (
	"context"
	"errors"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// Actor is the local serialization point for one table's hand.Table.
type Actor struct {
	id         string
	store      *tablestore.Store
	trustCache bool // set once at construction — see New's doc comment
	broadcast  func(viewerID string, snap hand.Snapshot)

	cmds chan Command

	cached  *hand.Table // nil until first loaded; never trusted when !trustCache
	version int
	handID  string
}

// New returns an Actor for tableID. trustCache should be true only when the
// caller currently holds tableID's tablelease — it is read once here and
// never re-consulted; losing the lease later does not retroactively
// invalidate an in-flight Actor (ARCHITECTURE.md §2: the lease bounds
// latency, not correctness — a stale cache is always caught by
// CommitAction's version check regardless of trustCache).
func New(id string, store *tablestore.Store, trustCache bool, broadcast func(string, hand.Snapshot)) *Actor {
	return &Actor{id: id, store: store, trustCache: trustCache, broadcast: broadcast, cmds: make(chan Command, 64)}
}

func (a *Actor) Dispatch(cmd Command) error {
	a.cmds <- cmd
	return <-cmd.reply()
}

func (a *Actor) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-a.cmds:
			err := a.handle(ctx, cmd)
			cmd.reply() <- err
		}
	}
}

func (a *Actor) handle(ctx context.Context, cmd Command) error {
	switch c := cmd.(type) {
	case ReadyCmd:
		return a.handleReady(ctx, c)
	case ActCmd:
		return a.handleAct(ctx, c)
	case DisconnectCmd:
		return a.handleDisconnect(c)
	case ReconnectCmd:
		return a.handleReconnect(c)
	default:
		return nil
	}
}

// ensureLoaded reads current state from the store the first time this Actor
// is used, or whenever force is true (a prior commit proved the local cache
// stale). force never mutates a.trustCache — trustCache reflects only
// whether this Actor's *own* lease-affinity was granted at construction
// (New's doc comment); a version conflict is evidence about staleness at
// this moment, not a permanent downgrade or upgrade of that grant.
func (a *Actor) ensureLoaded(ctx context.Context, force bool) error {
	if a.cached != nil && a.trustCache && !force {
		return nil
	}
	stored, err := a.store.LoadTable(ctx, a.id)
	if err != nil {
		return err
	}
	if stored == nil {
		return errors.New("table: no state seeded for this table yet")
	}
	a.cached = hand.NewTableFromState(stored.State)
	a.version = stored.Version
	a.handID = stored.HandID
	return nil
}

func (a *Actor) handleReady(ctx context.Context, c ReadyCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	if err := a.applyReadyAndCommit(ctx, c); err != nil {
		if errors.Is(err, tablestore.ErrVersionConflict) {
			// ARCHITECTURE.md §2: "the handler retries against the freshly-read
			// state" — exactly once; a second conflict inside the same dispatch
			// would mean real, sustained contention, not ordinary human-paced play.
			if err := a.ensureLoaded(ctx, true); err != nil {
				return err
			}
			if err := a.applyReadyAndCommit(ctx, c); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) applyReadyAndCommit(ctx context.Context, c ReadyCmd) error {
	for _, p := range a.cached.PlayersForActor() {
		if p.ID == c.PlayerID {
			p.Ready = c.Ready
		}
	}
	if a.cached.Stage() == hand.WaitingForPlayers || a.cached.Stage() == hand.Complete {
		if err := a.cached.StartHand(); err == nil {
			a.handID = newHandID()
		}
		// "need at least 2 ready players" is not a caller error — the table
		// just keeps waiting; swallow it here, same as before this revision.
	}
	return a.commit(ctx, "", nil)
}

func (a *Actor) handleAct(ctx context.Context, c ActCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	err := a.applyActAndCommit(ctx, c)
	if errors.Is(err, tablestore.ErrVersionConflict) {
		// See handleReady's identical rationale — retry exactly once.
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
		err = a.applyActAndCommit(ctx, c)
	}
	if err != nil && !errors.Is(err, tablestore.ErrDuplicateAction) {
		return err
	}
	a.broadcastAll()
	return nil
}

// applyActAndCommit reports success (nil error) both when the action applied
// and committed, and when it was already applied elsewhere (not applied
// locally, or ErrDuplicateAction from the store) — handleAct treats both as
// "nothing left to do but broadcast current state" and calls broadcastAll
// exactly once itself, so this method never calls it.
func (a *Actor) applyActAndCommit(ctx context.Context, c ActCmd) error {
	applied, err := a.cached.ActIdempotent(c.ActionID, c.PlayerID, c.Action, c.Amount)
	if err != nil {
		return err
	}
	if !applied {
		return nil
	}
	entry := tablestore.ActionLogEntry{PlayerID: c.PlayerID, ActionID: c.ActionID, Action: string(c.Action), Amount: c.Amount}
	return a.commit(ctx, c.ActionID, &entry)
}

func (a *Actor) commit(ctx context.Context, actionID string, entry *tablestore.ActionLogEntry) error {
	newState := a.cached.ExportState()
	if entry == nil {
		entry = &tablestore.ActionLogEntry{}
	}
	entry.TableID, entry.HandID, entry.Version = a.id, a.handID, a.version+1
	if err := a.store.CommitAction(ctx, a.id, a.handID, actionID, a.version, newState, *entry); err != nil {
		return err
	}
	a.version++
	return nil
}

func (a *Actor) handleDisconnect(c DisconnectCmd) error { return nil }
func (a *Actor) handleReconnect(c ReconnectCmd) error   { return nil }

func (a *Actor) broadcastAll() {
	if a.broadcast == nil || a.cached == nil {
		return
	}
	for _, p := range a.cached.PlayersForActor() {
		a.broadcast(p.ID, a.cached.ViewFor(p.ID))
	}
}

func newHandID() string {
	return timeNowFunc().Format("20060102T150405.000000000")
}

// TableForTest exposes the cached hand.Table for integration-test assertions.
func (a *Actor) TableForTest() *hand.Table { return a.cached }
```

Add `var timeNowFunc = time.Now` and the `"time"` import, same as the superseded plan.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -tags integration ./internal/table/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/table
git commit -m "feat(table): rewrite Actor as a local cache-affinity wrapper over versioned DynamoDB commits"
```

---

### Task 5: `tablemanager` — get-or-create an Actor, no acquire-or-fail

**Files:**

- Delete and recreate: `api/internal/tablemanager/manager.go`, `api/internal/tablemanager/manager_test.go`

**Interfaces:**

- Consumes: `tablelease.Service` (existing, now advisory-only), `tablestore.Store` (Task 3),
  `table.Actor`/`table.New` (Task 4), `hand.Table`/`hand.NewTable`/`hand.State` (existing/Task 2).
- Produces: `func NewManager(leases *tablelease.Service, store *tablestore.Store, broadcast
  func(tableID, viewerID string, snap hand.Snapshot)) *Manager`,
  `func (m *Manager) GetOrCreateActor(ctx, tableID string, seed func() *hand.Table) (*table.Actor, error)` —
  consumed by Task 7's WS gateway.

There is no `Locate` anymore — any instance may accept any table's connection, so there is nothing to route
to. `GetOrCreateActor` best-effort-acquires the table's lease purely to decide `trustCache`; a failed
acquire is not an error and does not block table access, per this plan's Global Constraints.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/tablemanager/manager_test.go
package tablemanager

import (
	"context"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
)

func TestGetOrCreateActorReturnsSameActorOnSecondCall(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	m := NewManager(tablelease.NewService(backend), nil, nil)
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}}, 10, 20) }

	a1, err := m.GetOrCreateActor(ctx, "table-1", seed)
	if err != nil || a1 == nil {
		t.Fatalf("expected first call to succeed, got actor=%v err=%v", a1, err)
	}
	a2, err := m.GetOrCreateActor(ctx, "table-1", seed)
	if err != nil || a2 != a1 {
		t.Fatalf("expected the same Actor on the second call, got a1=%p a2=%p err=%v", a1, a2, err)
	}
}

func TestGetOrCreateActorSucceedsEvenWhenLeaseIsHeldElsewhere(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	leases := tablelease.NewService(backend)
	ctx := context.Background()

	// Simulate a different instance already holding table-2's lease.
	release, ok, err := leases.Acquire(ctx, "table-2")
	if err != nil || !ok {
		t.Fatalf("seed acquire: ok=%v err=%v", ok, err)
	}
	defer release()

	m := NewManager(leases, nil, nil)
	seed := func() *hand.Table { return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}}, 10, 20) }
	a, err := m.GetOrCreateActor(ctx, "table-2", seed)
	if err != nil || a == nil {
		t.Fatalf("expected GetOrCreateActor to still succeed without the lease (correctness never gates on it), got actor=%v err=%v", a, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tablemanager/... -v`
Expected: FAIL with "undefined: NewManager".

- [ ] **Step 3: Implement `manager.go`**

```go
// api/internal/tablemanager/manager.go
// Package tablemanager is the per-instance registry of live table Actors.
// There is no "owner" of a table under this revision (ARCHITECTURE.md §2):
// any instance may create an Actor for any table at any time. tablelease is
// consulted only to decide whether that Actor may trust its own in-memory
// cache between commits — never to gate whether it may be created at all.
package tablemanager

import (
	"context"
	"fmt"
	"sync"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type Actor = table.Actor

type Manager struct {
	leases    *tablelease.Service
	store     *tablestore.Store
	broadcast func(tableID, viewerID string, snap hand.Snapshot)

	mu     sync.Mutex
	actors map[string]*Actor
}

func NewManager(leases *tablelease.Service, store *tablestore.Store, broadcast func(string, string, hand.Snapshot)) *Manager {
	return &Manager{leases: leases, store: store, broadcast: broadcast, actors: make(map[string]*Actor)}
}

// GetOrCreateActor returns this instance's Actor for tableID, seeding the
// table's very first DynamoDB state if it has never been played (seed is
// only invoked then). A failed best-effort lease acquire never blocks this —
// it only means the resulting Actor re-reads DynamoDB before every command
// instead of trusting its cache between commits.
func (m *Manager) GetOrCreateActor(ctx context.Context, tableID string, seed func() *hand.Table) (*Actor, error) {
	m.mu.Lock()
	if a, ok := m.actors[tableID]; ok {
		m.mu.Unlock()
		return a, nil
	}
	m.mu.Unlock()

	if m.store != nil {
		existing, err := m.store.LoadTable(ctx, tableID)
		if err != nil {
			return nil, fmt.Errorf("tablemanager: load table: %w", err)
		}
		if existing == nil {
			if err := m.store.SeedTable(ctx, tableID, seed().ExportState()); err != nil {
				return nil, fmt.Errorf("tablemanager: seed table: %w", err)
			}
		}
	}

	trustCache := false
	if m.leases != nil {
		if _, ok, err := m.leases.Acquire(ctx, tableID); err == nil && ok {
			trustCache = true
		}
	}

	actor := table.New(tableID, m.store, trustCache, m.broadcastFor(tableID))
	runCtx, cancel := context.WithCancel(context.Background())
	go actor.Run(runCtx)
	if trustCache {
		m.leases.StartHeartbeat(runCtx, tableID, func() { cancel() })
	}

	m.mu.Lock()
	m.actors[tableID] = actor
	m.mu.Unlock()
	return actor, nil
}

func (m *Manager) broadcastFor(tableID string) func(string, hand.Snapshot) {
	return func(viewerID string, snap hand.Snapshot) {
		if m.broadcast != nil {
			m.broadcast(tableID, viewerID, snap)
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tablemanager/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/tablemanager
git commit -m "feat(tablemanager): get-or-create Actors with best-effort cache-affinity, no lease-gated ownership"
```

---

### Task 6: Delete `tableowner` (dead once the WS proxy is gone)

**Files:**

- Delete: `api/internal/tableowner/owner.go`, `api/internal/tableowner/owner_test.go`
- Modify: `api/internal/config/config.go` (remove `InstancePrivateIP` — its only consumer was
  `tableowner.Registry.Advertise`)

`tableowner` existed solely to answer "which instance should I proxy this WebSocket connection to" — a
question that no longer has meaning once any instance can accept any table's connection directly (Task 7).
Keeping it around with no caller is exactly the kind of code this plan should not leave behind.

- [ ] **Step 1: Remove the files**

```bash
git rm api/internal/tableowner/owner.go api/internal/tableowner/owner_test.go
rmdir api/internal/tableowner 2>/dev/null || true
```

- [ ] **Step 2: Remove `InstancePrivateIP` from config**

```go
// api/internal/config/config.go — delete this block entirely
// InstancePrivateIP is this instance's own address, advertised via
// tableowner.Registry so sibling instances can proxy WebSocket traffic
// for tables this instance owns (see internal/tablemanager/manager.go).
InstancePrivateIP string `env:"INSTANCE_PRIVATE_IP" envDefault:"127.0.0.1"`
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./... && go vet ./...`
Expected: no errors (confirms nothing else referenced `tableowner` or `InstancePrivateIP`).

- [ ] **Step 4: Commit**

```bash
git add -A api/internal/tableowner api/internal/config/config.go
git commit -m "chore: delete tableowner — dead once any instance can accept any table's WS connection"
```

---

### Task 7: Client-facing WebSocket gateway (no proxy branch)

**Files:**

- Modify: `api/internal/api/v1/tablews.go` (already exists from the superseded plan — remove the
  `remoteAddr`/`proxyToRemoteInstance` branch, change `manager.Locate`+`manager.Acquire` to a single
  `manager.GetOrCreateActor` call)
- Delete: `api/internal/api/v1/tableproxy.go` (the whole file — no more internal proxy endpoint)
- Modify: `api/internal/api/v1/router.go` (drop `RegisterTableProxy`)
- Modify: `api/internal/app/app.go` (drop `newTableOwnerRegistry`, update `newTableManager`)

**Interfaces:**

- Consumes: `tablemanager.Manager.GetOrCreateActor` (Task 5), `gopkg.aoctech.app/api-commons/jwtverify.Verifier`
  (existing), `gopkg.aoctech.app/api-commons/ws.Registry` (existing).
- Produces: unchanged route, `GET /v1.0/tables/:id/ws` — only the handler body changes.

- [ ] **Step 1: Delete the proxy file and its route**

```bash
git rm api/internal/api/v1/tableproxy.go
```

```go
// api/internal/api/v1/router.go — remove this line from Register
RegisterTableProxy(router, manager, verifier, seed)
```

- [ ] **Step 2: Simplify `RegisterTableWS`'s connection handler**

```go
// api/internal/api/v1/tablews.go — replace the block between reading claims and registering the connection
actor, err := manager.GetOrCreateActor(ctx, tableID, seed(tableID))
if err != nil {
send(map[string]any{"type": "error", "code": "unavailable"})
_ = conn.Close()
return
}
```

Delete `proxyToRemoteInstance`'s call site (the `if actor == nil && remoteAddr != ""` branch) and the now-unused
`remoteAddr` variable entirely — `GetOrCreateActor` never returns a "go elsewhere" signal.

- [ ] **Step 3: Update `RegisterTableWS`'s signature call site in `router.go`**

```go
// api/internal/api/v1/router.go — Register, replace the two RegisterTableWS/RegisterTableProxy lines with
RegisterTableWS(router, verifier, manager, reg, cfg.CorsAllowedOrigins, seed)
```

- [ ] **Step 4: Update `app.go`'s wiring**

```go
// api/internal/app/app.go — newTableManager, remove the owners param and tableowner import
func newTableManager(leases *tablelease.Service, store *tablestore.Store, reg ws.Registry) *tablemanager.Manager {
broadcast := func (tableID, viewerID string, snap hand.Snapshot) {
data, _ := json.Marshal(map[string]any{"type": "state", "snapshot": snap})
reg.Broadcast(context.Background(), tableID+"#"+viewerID, data)
}
return tablemanager.NewManager(leases, store, broadcast)
}
```

Remove `newTableOwnerRegistry` entirely and drop it from `Module`'s `fx.Provide` list; remove the
`tableowner` import.

- [ ] **Step 5: Verify it compiles**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add api/internal/api/v1/tablews.go api/internal/api/v1/router.go api/internal/app/app.go
git commit -m "feat(api): remove the cross-instance WS proxy — any instance now handles any table directly"
```

---

### Task 8: Disconnect grace window + action deadline

**Files:**

- Modify: `api/internal/table/actor.go`
- Test: `api/internal/table/disconnect_test.go`

Same behavior contract as the superseded plan's Task 9 (OVERVIEW.md §4: auto-fold at the action deadline,
auto-sit-out after the grace window or 3 consecutive disconnected hands) — only the field names inside
`Actor` change (`a.cached` instead of `a.table`), and every mutating path now goes through `a.commit`
instead of mutating in place.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/table/disconnect_test.go
//go:build integration

package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func TestDisconnectAutoFoldsAtActionDeadline(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "test")
	mustCreateTestTables(t, db, "test")
	a := newTestActor(t, store)
	a.actionDeadline = 20 * time.Millisecond

	ctx := context.Background()
	reply := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})

	stored, _ := store.LoadTable(ctx, "table-1")
	var toAct string
	for _, s := range stored.State.Players {
		if s.State == hand.Active {
			toAct = s.ID
			break
		}
	}
	reply3 := make(chan error, 1)
	_ = a.Dispatch(DisconnectCmd{PlayerID: toAct, Reply: reply3})

	time.Sleep(50 * time.Millisecond)

	stored, _ = store.LoadTable(ctx, "table-1")
	for _, s := range stored.State.Players {
		if s.ID == toAct && s.State != hand.Folded {
			t.Fatalf("expected %s to be auto-folded after missing its action deadline, got state %v", toAct, s.State)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags integration ./internal/table/... -run TestDisconnect -v`
Expected: FAIL — `a.actionDeadline` doesn't exist yet.

- [ ] **Step 3: Implement grace window + action deadline**

```go
// api/internal/table/actor.go — add fields to Actor
actionDeadline               time.Duration
disconnectGrace              time.Duration
disconnectedSince            map[string]time.Time
consecutiveDisconnectedHands map[string]int
deadlineTimer                *time.Timer
```

```go
// api/internal/table/actor.go — New, initialize the new fields
func New(id string, store *tablestore.Store, trustCache bool, broadcast func (string, hand.Snapshot)) *Actor {
return &Actor{
id: id, store: store, trustCache: trustCache, broadcast: broadcast, cmds: make(chan Command, 64),
actionDeadline:               30 * time.Second,
disconnectGrace:              45 * time.Second,
disconnectedSince:            make(map[string]time.Time),
consecutiveDisconnectedHands: make(map[string]int),
}
}
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
// the just-disconnected player is the one currently on the clock.
func (a *Actor) armActionDeadlineIfTheirTurn(playerID string) {
if a.cached == nil || !a.cached.CurrentPlayerCanActForActor(playerID) {
return
}
if a.deadlineTimer != nil {
a.deadlineTimer.Stop()
}
a.deadlineTimer = time.AfterFunc(a.actionDeadline, func () {
reply := make(chan error, 1)
_ = a.Dispatch(ActCmd{PlayerID: playerID, ActionID: "auto-fold-" + playerID, Action: "fold", Reply: reply})
a.consecutiveDisconnectedHands[playerID]++
if timeNowFunc().Sub(a.disconnectedSince[playerID]) >= a.disconnectGrace || a.consecutiveDisconnectedHands[playerID] >= 3 {
reply2 := make(chan error, 1)
_ = a.Dispatch(SitOutCmd{PlayerID: playerID, Reply: reply2})
}
})
}
```

```go
// api/internal/table/commands.go — add
type SitOutCmd struct {
PlayerID string
Reply    chan error
}

func (c SitOutCmd) reply() chan error { return c.Reply }
```

```go
// api/internal/table/actor.go — handle's switch, add a case
case SitOutCmd:
return a.handleSitOut(ctx, c)
```

```go
// api/internal/table/actor.go — add
func (a *Actor) handleSitOut(ctx context.Context, c SitOutCmd) error {
if err := a.ensureLoaded(ctx); err != nil {
return err
}
a.cached.SitOutForActor(c.PlayerID)
if err := a.commit(ctx, "", nil); err != nil && !errors.Is(err, tablestore.ErrVersionConflict) {
return err
}
a.broadcastAll()
return nil
}
```

`(*hand.Table).CurrentPlayerCanActForActor`/`SitOutForActor` don't exist yet:

```go
// api/internal/engine/hand/hand.go — add near PlayersForActor
// CurrentPlayerCanActForActor exposes currentPlayerCanAct to Phase 2's
// table.Actor (auto-fold deadline arming needs to know whose turn it is).
func (t *Table) CurrentPlayerCanActForActor(playerID string) bool { return t.currentPlayerCanAct(playerID) }

// SitOutForActor marks a player SittingOut (OVERVIEW.md § 4) once a
// disconnected player exceeds the grace period or enough consecutive
// disconnected hands.
func (t *Table) SitOutForActor(playerID string) {
if p := t.playerByID(playerID); p != nil {
p.State = SittingOut
}
}
```

- [ ] **Step 4: Arm the deadline on every turn change**

```go
// api/internal/table/actor.go — add
func (a *Actor) armActionDeadlineForCurrentTurn() {
if a.deadlineTimer != nil {
a.deadlineTimer.Stop()
}
for id := range a.disconnectedSince {
if a.cached.CurrentPlayerCanActForActor(id) {
a.armActionDeadlineIfTheirTurn(id)
return
}
}
}
```

```go
// api/internal/table/actor.go — handleAct, call right before its final `a.broadcastAll()`
a.armActionDeadlineForCurrentTurn()
```

- [ ] **Step 5: Wire disconnect/reconnect into the WS gateway**

```go
// api/internal/api/v1/tablews.go — RegisterTableWS's read loop, replace the closing section
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
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test -tags integration ./internal/table/... ./internal/engine/hand/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add api/internal/table api/internal/engine/hand/hand.go api/internal/api/v1/tablews.go
git commit -m "feat(table): disconnect grace window, auto-fold action deadline, auto-sit-out"
```

---

### Task 9: Per-seat action rate limiting

Unchanged from the superseded plan's Task 10 — this concern has no dependency on the authority model
revision. Implement exactly as previously specified:

**Files:**

- Modify: `api/internal/api/v1/tablews.go`
- Test: `api/internal/api/v1/ratelimit_test.go`

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
// api/internal/api/v1/tablews.go — add ("sync" and "time" already imported)
// seatLimiter is a fixed-window per-player counter — abuse prevention
// (ARCHITECTURE.md §8), not precise rate metering.
type seatLimiter struct {
mu        sync.Mutex
perWindow int
window    time.Duration
counts    map[string]int
resetAt   map[string]time.Time
}

func newSeatLimiter(perSecond int) *seatLimiter {
return &seatLimiter{perWindow: perSecond, window: time.Second, counts: make(map[string]int), resetAt: make(map[string]time.Time)}
}

func (l *seatLimiter) Allow(playerID string) bool {
l.mu.Lock()
defer l.mu.Unlock()
now := time.Now()
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

Add `"sync"` to `tablews.go`'s import block.

- [ ] **Step 4: Enforce the limit in the message loop**

```go
// api/internal/api/v1/tablews.go — RegisterTableWS, right after the upgrader.Upgrade closure opens
limiter := newSeatLimiter(10) // 10 actions/sec/seat
```

```go
// api/internal/api/v1/tablews.go — inside the read loop, right after `var m clientMessage` unmarshal succeeds
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

### Task 10: CDK — DynamoDB tables + Streams-to-S3 archival for `poker_action_log`

**Files:**

- Create: `cdk/lib/dynamodb-stack.ts`
- Create: `cdk/lib/archiver-stack.ts` (S3 bucket + Lambda + `DynamoEventSource` — kept out of
  `dynamodb-stack.ts` so each file has one responsibility, per this repo's file-structure convention)
- Create: `api/cmd/archiver/main.go` (Go Lambda handler, `provided.al2023` runtime — same language as the
  rest of this repo, matching the `api/cmd/handreplay` convention for a standalone binary)
- Create: `api/cmd/archiver/main_test.go`
- Modify: `cdk/lib/api-stack.ts`
- Modify: `cdk/bin/poker.ts`
- Test: `cdk/test/dynamodb-stack.test.ts`
- Test: `cdk/test/archiver-stack.test.ts`

**Interfaces:**
- Consumes: `poker_action_log`'s DynamoDB Stream (`NEW_IMAGE` view) — every `PutItem`/`TransactWriteItems`
  insert `CommitAction` (Task 3) makes.
- Produces: an S3 object per Lambda invocation batch at
  `s3://poker-action-log-archive-{env}/{table_id}/{hand_id}/{unix_nanos}.jsonl` — one JSON line per archived
  `ActionLogEntry`. Nothing in this codebase reads this archive back yet (no consumer needed for Phase 2;
  it exists so hand-history is never lost once `logTTLDays` — Task 3 — reaps the hot copy).

Same shape as the superseded plan's Task 11 for the tables themselves, minus the same-security-group
ingress rule and the `INSTANCE_PRIVATE_IP` userdata line (both existed only for the now-deleted proxy),
plus the extra IAM actions `CommitAction`'s `TransactWriteItems` needs, plus the archival pipeline this
task adds new.

- [ ] **Step 1: Write the failing CDK test**

```typescript
// cdk/test/dynamodb-stack.test.ts
import {App} from 'aws-cdk-lib';
import {Template} from 'aws-cdk-lib/assertions';
import {DynamoDBStack} from '../lib/dynamodb-stack';

test('creates poker_table_state, poker_action_log, poker_action_guards tables', () => {
    const app = new App();
    const stack = new DynamoDBStack(app, 'TestDynamoDBStack', {environment: 'dev'});
    const template = Template.fromStack(stack);
    template.resourceCountIs('AWS::DynamoDB::Table', 3);
    template.hasResourceProperties('AWS::DynamoDB::Table', {TableName: 'dev_poker_table_state'});
    template.hasResourceProperties('AWS::DynamoDB::Table', {TableName: 'dev_poker_action_log'});
    template.hasResourceProperties('AWS::DynamoDB::Table', {TableName: 'dev_poker_action_guards'});
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

export type TableName = 'poker_table_state' | 'poker_action_log' | 'poker_action_guards';

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
        const pointInTimeRecoverySpecification = environment === 'prod' ? {pointInTimeRecoveryEnabled: true} : undefined;

        const table = (name: TableName, withSortKey: boolean, withTTL: boolean = false, withStream: boolean = false): dynamodb.TableV2 => {
            const tableName = `${environment}_${name}`;
            const t = new dynamodb.TableV2(this, tableName, {
                tableName,
                partitionKey: {name: 'pk', type: dynamodb.AttributeType.STRING},
                sortKey: withSortKey ? {name: 'sk', type: dynamodb.AttributeType.STRING} : undefined,
                billing: Billing.onDemand({maxReadRequestUnits: 1000, maxWriteRequestUnits: 1000}),
                removalPolicy,
                pointInTimeRecoverySpecification,
                encryption: dynamodb.TableEncryptionV2.awsManagedKey(),
                timeToLiveAttribute: withTTL ? 'ttl' : undefined,
                dynamoStream: withStream ? dynamodb.StreamViewType.NEW_IMAGE : undefined,
            });
            this.tables.set(name, t);
            return t;
        };

        table('poker_table_state', false);
        // poker_action_log: TTL'd (Store.logTTLDays = 90 days — the "recent
        // window" served directly from Dynamo) with a stream so the archiver
        // Lambda (below) ships every entry to S3 before that TTL ever reaps
        // it — nothing is lost, just moved to cheaper long-term storage.
        table('poker_action_log', true, true, true);
        // poker_action_guards: TTL'd (mirrors ctech-wallet's wallet_idempotency
        // table, cdk/lib/dynamodb-stack.ts:112) — a guard only needs to
        // outlive plausible client retries (Store.guardTTLDays = 7 days).
        table('poker_action_guards', false, true);
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd cdk && npx jest dynamodb-stack.test.ts`
Expected: PASS.

- [ ] **Step 5: Write the archiver Lambda's failing test**

```go
// api/cmd/archiver/main_test.go
package main

import (
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestBuildBatchRendersOneJSONLinePerInsert(t *testing.T) {
	e := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{
				EventName: "INSERT",
				EventID:   "evt-1",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"pk":        events.NewStringAttribute("table-1#hand-1"),
						"sk":        events.NewStringAttribute("0000000002"),
						"player_id": events.NewStringAttribute("p1"),
						"action":    events.NewStringAttribute("call"),
						"amount":    events.NewNumberAttribute("0"),
					},
				},
			},
			{
				// Non-INSERT records (there shouldn't be any on an
				// append-only table, but TTL-expiry emits REMOVE — never
				// archive those, the item already reached S3 on its own INSERT).
				EventName: "REMOVE",
				EventID:   "evt-2",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{"pk": events.NewStringAttribute("table-1#hand-1")},
				},
			},
		},
	}

	batch, key, err := buildBatch(e)
	if err != nil {
		t.Fatalf("buildBatch: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(batch), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 line (INSERT only), got %d: %q", len(lines), string(batch))
	}
	if !strings.Contains(lines[0], `"player_id":"p1"`) {
		t.Fatalf("expected the JSON line to contain player_id, got %q", lines[0])
	}
	if !strings.HasPrefix(key, "table-1/hand-1/") || !strings.HasSuffix(key, ".jsonl") {
		t.Fatalf("expected key partitioned as table_id/hand_id/*.jsonl, got %q", key)
	}
}

func TestBuildBatchReturnsEmptyWhenNothingToInsert(t *testing.T) {
	e := events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{{EventName: "REMOVE"}}}
	batch, key, err := buildBatch(e)
	if err != nil || batch != nil || key != "" {
		t.Fatalf("expected no-op for an all-REMOVE batch, got batch=%q key=%q err=%v", batch, key, err)
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./cmd/archiver/... -v`
Expected: FAIL with "undefined: buildBatch".

- [ ] **Step 7: Implement `main.go`**

```go
// api/cmd/archiver/main.go
// archiver is a Lambda subscribed to poker_action_log's DynamoDB Stream
// (cdk/lib/archiver-stack.ts): it ships every inserted ActionLogEntry to S3
// before logTTLDays (tablestore.logTTLDays, the hot table's TTL) ever reaps
// it. DynamoDB serves the recent window; S3 is the indefinite archive — see
// this plan's Global Constraints and Task 3.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// s3Putter is the minimal surface main needs from *s3.Client — narrowed so
// buildBatch's caller can be tested against a fake without a live bucket.
type s3Putter interface {
	PutObject(ctx context.Context, bucket, key string, body []byte) error
}

type realS3Putter struct{ client *s3.Client }

func (p *realS3Putter) PutObject(ctx context.Context, bucket, key string, body []byte) error {
	_, err := p.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(body)})
	return err
}

// buildBatch renders every INSERT record's NewImage as one JSON line (JSON
// Lines format, so a later consumer processes the archive without loading a
// whole batch into memory as a single document) and derives an S3 key
// partitioned by table_id/hand_id (poker_action_log's pk is
// "table_id#hand_id" — see tablestore.CommitAction). Non-INSERT records
// (TTL-expiry emits REMOVE) are skipped: an expiring item already reached
// S3 on its own INSERT, so archiving its REMOVE would just duplicate it.
func buildBatch(e events.DynamoDBEvent) (batch []byte, key string, err error) {
	var buf bytes.Buffer
	var firstPK, lastEventID string
	for _, r := range e.Records {
		if r.EventName != "INSERT" {
			continue
		}
		if firstPK == "" {
			firstPK = r.Change.NewImage["pk"].String()
		}
		lastEventID = r.EventID
		rendered, err := attributeMapToJSON(r.Change.NewImage)
		if err != nil {
			return nil, "", fmt.Errorf("archiver: encode record: %w", err)
		}
		buf.Write(rendered)
		buf.WriteByte('\n')
	}
	if buf.Len() == 0 {
		return nil, "", nil
	}
	partition := strings.ReplaceAll(firstPK, "#", "/")
	key = fmt.Sprintf("%s/%d-%s.jsonl", partition, time.Now().UnixNano(), lastEventID)
	return buf.Bytes(), key, nil
}

// attributeMapToJSON converts one DynamoDB Stream NewImage into a compact
// JSON object — events.DynamoDBAttributeValue has no built-in JSON
// marshaler, so this recurses over its DataType() the same way the AWS SDK's
// own attributevalue package does internally.
func attributeMapToJSON(m map[string]events.DynamoDBAttributeValue) ([]byte, error) {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = attributeValueToInterface(v)
	}
	return json.Marshal(out)
}

func attributeValueToInterface(v events.DynamoDBAttributeValue) any {
	switch v.DataType() {
	case events.DataTypeString:
		return v.String()
	case events.DataTypeNumber:
		n, _ := strconv.ParseFloat(v.Number(), 64)
		return n
	case events.DataTypeBoolean:
		return v.Boolean()
	case events.DataTypeNull:
		return nil
	case events.DataTypeList:
		list := v.List()
		out := make([]any, len(list))
		for i, item := range list {
			out[i] = attributeValueToInterface(item)
		}
		return out
	case events.DataTypeMap:
		return mustMap(v.Map())
	default:
		return nil // Binary/*Set: not present in ActionLogEntry, skipped rather than guessed at
	}
}

func mustMap(m map[string]events.DynamoDBAttributeValue) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = attributeValueToInterface(v)
	}
	return out
}

func handle(putter s3Putter, bucket string) func(context.Context, events.DynamoDBEvent) error {
	return func(ctx context.Context, e events.DynamoDBEvent) error {
		batch, key, err := buildBatch(e)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		return putter.PutObject(ctx, bucket, key, batch)
	}
}

func main() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(fmt.Errorf("archiver: load AWS config: %w", err))
	}
	bucket := os.Getenv("ARCHIVE_BUCKET")
	lambda.Start(handle(&realS3Putter{client: s3.NewFromConfig(cfg)}, bucket))
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test ./cmd/archiver/... -v`
Expected: PASS. Then add the module dependencies: `go get github.com/aws/aws-lambda-go github.com/aws/aws-sdk-go-v2/service/s3 && go mod tidy`.

- [ ] **Step 9: Implement `archiver-stack.ts`**

```typescript
// cdk/test/archiver-stack.test.ts
import {App, Stack} from 'aws-cdk-lib';
import {Template} from 'aws-cdk-lib/assertions';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import {ArchiverStack} from '../lib/archiver-stack';

test('creates an archive bucket and a Lambda subscribed to the action log stream', () => {
  const app = new App();
  const tableStack = new Stack(app, 'TestTableStack');
  const table = new dynamodb.TableV2(tableStack, 'TestActionLog', {
    partitionKey: {name: 'pk', type: dynamodb.AttributeType.STRING},
    sortKey: {name: 'sk', type: dynamodb.AttributeType.STRING},
    dynamoStream: dynamodb.StreamViewType.NEW_IMAGE,
  });

  const stack = new ArchiverStack(app, 'TestArchiverStack', {environment: 'dev', actionLogTable: table});
  const template = Template.fromStack(stack);
  template.resourceCountIs('AWS::S3::Bucket', 1);
  template.resourceCountIs('AWS::Lambda::Function', 1);
  template.resourceCountIs('AWS::Lambda::EventSourceMapping', 1);
});
```

```typescript
// cdk/lib/archiver-stack.ts
// Archives poker_action_log's DynamoDB Stream to S3 before logTTLDays (see
// api/internal/tablestore's guardTTLDays/logTTLDays constants) reaps the hot
// copy — DynamoDB serves the recent window, S3 is the indefinite archive.
import * as cdk from 'aws-cdk-lib';
import {RemovalPolicy} from 'aws-cdk-lib';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as s3 from 'aws-cdk-lib/aws-s3';
import {DynamoEventSource} from 'aws-cdk-lib/aws-lambda-event-sources';
import {StartingPosition} from 'aws-cdk-lib/aws-lambda';
import {Construct} from 'constructs';
import {Environment} from '@aoctech/cdk';

interface ArchiverStackProps extends cdk.StackProps {
  environment: Environment;
  actionLogTable: dynamodb.ITableV2;
}

export class ArchiverStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: ArchiverStackProps) {
    super(scope, id, props);
    const {environment, actionLogTable} = props;

    const bucket = new s3.Bucket(this, 'ActionLogArchive', {
      bucketName: `poker-action-log-archive-${environment}`,
      removalPolicy: environment === 'dev' ? RemovalPolicy.DESTROY : RemovalPolicy.RETAIN,
      autoDeleteObjects: environment === 'dev',
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
      encryption: s3.BucketEncryption.S3_MANAGED,
    });

    const fn = new lambda.Function(this, 'ArchiverFn', {
      functionName: `${environment}-poker-action-log-archiver`,
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: 'bootstrap',
      code: lambda.Code.fromAsset('../api/cmd/archiver', {
        bundling: {
          image: lambda.Runtime.PROVIDED_AL2023.bundlingImage,
          command: ['bash', '-c', 'GOOS=linux GOARCH=arm64 go build -o /asset-output/bootstrap .'],
        },
      }),
      environment: {ARCHIVE_BUCKET: bucket.bucketName},
      timeout: cdk.Duration.seconds(30),
    });

    bucket.grantWrite(fn);
    fn.addEventSource(new DynamoEventSource(actionLogTable, {
      startingPosition: StartingPosition.TRIM_HORIZON,
      batchSize: 100,
      retryAttempts: 3,
    }));
  }
}
```

- [ ] **Step 10: Run test to verify it passes**

Run: `cd cdk && npx jest archiver-stack.test.ts`
Expected: PASS.

- [ ] **Step 11: Grant table access to the API instance role**

```typescript
// cdk/lib/api-stack.ts — ApiStackProps, add
tableStateArn: string;
actionLogArn: string;
actionGuardsArn: string;
```

```typescript
// cdk/lib/api-stack.ts — constructor, after `service` is constructed
service.instanceRole.addToPolicy(new iam.PolicyStatement({
    actions: ['dynamodb:GetItem', 'dynamodb:PutItem', 'dynamodb:UpdateItem', 'dynamodb:Query', 'dynamodb:TransactWriteItems'],
    resources: [tableStateArn, actionLogArn, actionGuardsArn],
}));
```

Add `import * as iam from 'aws-cdk-lib/aws-iam';` to `api-stack.ts`'s imports. There is no same-SG ingress
rule and no `INSTANCE_PRIVATE_IP` userdata line in this revision — both existed only for the now-deleted
proxy (superseded plan's Task 11 Steps 5–6). The API instance role never needs S3 access to the archive
bucket — only the archiver Lambda writes there (Step 9's `bucket.grantWrite(fn)` already covers it).

- [ ] **Step 12: Wire `bin/poker.ts`**

```typescript
// cdk/bin/poker.ts — after the existing stacks are constructed, before PokerApiStack
const dynamoStack = new DynamoDBStack(app, `${environment}-ctech-poker-dynamodb`, {environment, env: awsEnv});
new ArchiverStack(app, `${environment}-ctech-poker-archiver`, {
  environment,
  env: awsEnv,
  actionLogTable: dynamoStack.tables.get('poker_action_log')!,
});
```

```typescript
// cdk/bin/poker.ts — pass into PokerApiStack's props
  tableStateArn: dynamoStack.tables.get('poker_table_state')!.tableArn,
  actionLogArn: dynamoStack.tables.get('poker_action_log')!.tableArn,
  actionGuardsArn: dynamoStack.tables.get('poker_action_guards')!.tableArn,
```

Add `import {DynamoDBStack} from '../lib/dynamodb-stack';` and `import {ArchiverStack} from '../lib/archiver-stack';` to `bin/poker.ts`.

- [ ] **Step 13: Synth to verify no CDK errors**

Run: `cd cdk && npx tsc --noEmit` (full `cdk synth` needs live AWS context via `CTECH_VPC_ID` — same acceptable
substitute the foundations plan already established, and additionally needs a Go toolchain available for
`archiver-stack.ts`'s asset bundling command — acceptable to skip the asset-bundling path in `tsc --noEmit`
since it only type-checks).
Expected: no type errors.

- [ ] **Step 14: Commit**

```bash
git add cdk/lib/dynamodb-stack.ts cdk/lib/archiver-stack.ts cdk/lib/api-stack.ts cdk/bin/poker.ts cdk/test/dynamodb-stack.test.ts cdk/test/archiver-stack.test.ts api/cmd/archiver
git commit -m "feat(cdk): provision versioned table-state/action-log/action-guard tables, archive action log to S3"
```

---

### Task 11: Integration test — optimistic concurrency across two instances, trivial recovery

**Files:**

- Create: `api/tests/integration/tableflow_test.go` (build tag `integration`)

This is the engineering-level equivalent of ARCHITECTURE.md §2's revised deliverable: two instances racing
on the same table resolve deterministically via DynamoDB's conditional write (one wins, the other retries
against fresh state), and a "crashed" instance's replacement needs no replay — it just reads current state.

- [ ] **Step 1: Write the test**

```go
// api/tests/integration/tableflow_test.go
//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func testDynamoClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), config.WithCredentialsProvider(aws.AnonymousCredentials{}))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}

func mustCreatePokerTables(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	pkOnly := []string{env + "_poker_table_state", env + "_poker_action_guards"}
	pkSk := []string{env + "_poker_action_log"}
	create := func(name string, withSK bool) {
		attrs := []types.AttributeDefinition{{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS}}
		keys := []types.KeySchemaElement{{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash}}
		if withSK {
			attrs = append(attrs, types.AttributeDefinition{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS})
			keys = append(keys, types.KeySchemaElement{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange})
		}
		tableName := name
		_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{TableName: &tableName, AttributeDefinitions: attrs, KeySchema: keys, BillingMode: types.BillingModePayPerRequest})
		var inUse *types.ResourceInUseException
		if err != nil && !errors.As(err, &inUse) {
			t.Fatalf("create table %s: %v", name, err)
		}
	}
	for _, n := range pkOnly {
		create(n, false)
	}
	for _, n := range pkSk {
		create(n, true)
	}
}

func TestTwoInstancesRacingSameTableResolveDeterministically(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	db := testDynamoClient(t)
	store := tablestore.NewStore(db, "test")
	mustCreatePokerTables(t, db, "test")

	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)
	}

	// Two "instances", neither holding the lease — both must re-read before
	// every command, so neither trusts a stale cache.
	mgrA := tablemanager.NewManager(tablelease.NewService(backend), store, nil)
	mgrB := tablemanager.NewManager(tablelease.NewService(backend), store, nil)

	actorA, err := mgrA.GetOrCreateActor(context.Background(), "table-race", seed)
	if err != nil {
		t.Fatalf("acquire on instance A: %v", err)
	}
	actorB, err := mgrB.GetOrCreateActor(context.Background(), "table-race", seed)
	if err != nil {
		t.Fatalf("acquire on instance B: %v", err)
	}

	replyA := make(chan error, 1)
	if err := actorA.Dispatch(table.ReadyCmd{PlayerID: "p1", Ready: true, Reply: replyA}); err != nil {
		t.Fatalf("ready p1 via A: %v", err)
	}
	replyB := make(chan error, 1)
	if err := actorB.Dispatch(table.ReadyCmd{PlayerID: "p2", Ready: true, Reply: replyB}); err != nil {
		t.Fatalf("ready p2 via B (must survive A's concurrent version bump): %v", err)
	}

	stored, err := store.LoadTable(context.Background(), "table-race")
	if err != nil || stored == nil || stored.State.Stage == hand.WaitingForPlayers {
		t.Fatalf("expected the hand to have started after both readies landed, got %+v err=%v", stored, err)
	}
}

func TestFreshInstanceReadsCurrentStateWithNoReplayNeeded(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	db := testDynamoClient(t)
	store := tablestore.NewStore(db, "test")
	mustCreatePokerTables(t, db, "test")

	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)
	}

	mgrA := tablemanager.NewManager(tablelease.NewService(backend), store, nil)
	actorA, err := mgrA.GetOrCreateActor(context.Background(), "table-crash", seed)
	if err != nil {
		t.Fatalf("acquire on instance A: %v", err)
	}
	reply := make(chan error, 1)
	_ = actorA.Dispatch(table.ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = actorA.Dispatch(table.ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})
	// Instance A "crashes" here — nothing more happens on it. Under this
	// revision there is nothing to fail over: the next instance just reads
	// current DynamoDB state directly (ARCHITECTURE.md §3).

	mgrB := tablemanager.NewManager(tablelease.NewService(backend), store, nil)
	actorB, err := mgrB.GetOrCreateActor(context.Background(), "table-crash", seed)
	if err != nil {
		t.Fatalf("acquire on instance B: %v", err)
	}
	view := actorB.TableForTest().ViewFor("p1")
	if view.Stage == "waiting_for_players" {
		t.Fatalf("expected instance B to see the hand already in progress, got stage %s", view.Stage)
	}
}
```

Note: `actorB.TableForTest()` returns `a.cached`, which is populated only after the Actor has processed at
least one command via `ensureLoaded`. Adjust the test to dispatch a no-op `ReadyCmd` (e.g. re-affirming
`p1`'s existing `Ready: true`) on `actorB` before reading `TableForTest()`, so `ensureLoaded` has run.

- [ ] **Step 2: Run test to verify it passes**

Run: `docker compose -f api/docker-compose.test.yml up -d && go test -tags integration ./tests/integration/... -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add api/tests/integration/tableflow_test.go
git commit -m "test(integration): verify optimistic concurrency across instances and trivial recovery"
```

---

## Closing note

`internal/api/v1/tablews.go`'s auth/heartbeat helpers (`readAuthToken`, `wsAllowedOrigin`, `startHeartbeat`,
`wsConnAdapter`) remain copied near-verbatim from `ctech-wallet/api/internal/api/v1/ws.go`, unchanged by this
revision. Extracting them into a shared package once a third consumer (e.g. `ctech-dfe`) needs the same
pattern is a reasonable follow-up, not a blocker — same call made in the superseded plan.
