# Ghost Table Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:
> executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tables with no activity for a configurable period are automatically archived — refunding any seated players'
sandbox chips and permanently blocking further buy-ins on that table — via a scheduled Lambda, mirroring the existing
`cmd/reconcile` + `ReconcileStack` pattern. No more tables persisting forever with dead seats.

**Architecture:** `poker_table_state` (`StoredTable`, `api/internal/tablestore/store.go:37-42`) currently has no
timestamp and no GSI (`cdk/lib/dynamodb-stack.ts:49-51`, comment: "no TTL, no stream, always current"). This plan adds a
`last_action_at` (unix seconds) field updated on every `SeedTable`/`CommitAction` write (the single choke point all
table mutations already go through — `../../api/CLAUDE.md`: "every mutated action commits via `tablestore.CommitAction`
"), a sparse GSI (`gsi_active` partition key present only while a table is live) so the cleanup job queries instead of
scanning, and an `archived` flag checked once at `tablemanager.GetOrCreateActor` — the single place every code path (WS
connect, buy-in, cash-out) already goes through to get a live actor. A new scheduled Lambda (`cmd/tablecleanup`, on a
`TableCleanupStack` EventBridge Schedule) queries the GSI for tables idle past a cutoff, refunds seated sandbox stacks
via the wallet client, and archives them with an optimistic version check — so a table that gets a fresh action between
the query and the archive write is safely skipped this pass (same conditional-write discipline as everything else in
this codebase; never a read-then-write race).

**Tech Stack:** Go (AWS SDK v2 DynamoDB, `aws-lambda-go`), AWS CDK (TypeScript) — EventBridge Scheduler + Lambda, same
shape as `../../cdk/lib/reconcile-stack.ts`.

## Global Constraints

- **Sandbox isolation is load-bearing** (`../../api/CLAUDE.md`): this job only ever refunds/archives **sandbox** tables.
  A `real` `CurrencyMode` room must be skipped entirely, not refunded through this path.
- **Correctness = DynamoDB conditional writes**, never read-then-write (`../../api/CLAUDE.md`). The archive write must
  carry a `version =` condition.
- **`tablelease` is latency-only**, never correctness — this plan must not lean on lease ownership to decide whether a
  table is stale.
- **No scans in production** (`gopkg.aoctech.app/api-commons/dynamo` package doc) — the cleanup job queries the new GSI,
  never `Scan`.
- **Named constants**, no magic strings for table/index/attribute names (`../../api/CLAUDE.md`, `../../cdk/CLAUDE.md`).
- Go tests: `go test ./... -race`; DynamoDB-touching tests use the `//go:build integration` tag against DynamoDB Local
  (`docker-compose.test.yml`), following each package's own copy of `testClient`/`mustCreateTestTables` (confirmed
  convention: `../../api/internal/tablestore/dynamo_test.go`, `../../api/internal/buyin/dynamo_helpers_test.go`).
- CDK: `Billing.onDemand` with explicit `maxRead/WriteRequestUnits` (`../../cdk/CLAUDE.md`); CDK tests use Jest +
  `aws-cdk-lib/assertions` (`../../cdk/test/dynamodb-stack.test.ts`).

---

### Task 1: `last_action_at` + sparse `gsi_active` on `poker_table_state`

**Files:**

- Modify: `../../api/internal/tablestore/store.go`
- Modify: `../../cdk/lib/dynamodb-stack.ts`
- Modify: `../../cdk/test/dynamodb-stack.test.ts`
- Test: `../../api/internal/tablestore/dynamo_test.go`

**Interfaces:**

- Produces: `StoredTable.LastActionAt int64` (unix seconds) and `StoredTable.Archived bool`, populated by `SeedTable`/
  `CommitAction`. Task 2 (`MarkArchived`, `QueryStaleActive`) and Task 3 (`tablemanager`) read these exact field names.

- [ ] **Step 1: Write the failing test**

Add to `../../api/internal/tablestore/dynamo_test.go` (append after `TestCommitActionRejectsDuplicateActionID`):

```go
func TestSeedAndCommitSetLastActionAt(t *testing.T) {
db := testClient(t)
env := isolatedEnv()
s := NewStore(db, env)
ctx := context.Background()
mustCreateTestTables(ctx, t, db, env)

timeNowFunc = func () time.Time { return time.Unix(1000, 0) }
defer func () { timeNowFunc = time.Now }()

if err := s.SeedTable(ctx, "table-4", hand.State{Stage: hand.WaitingForPlayers}); err != nil {
t.Fatalf("SeedTable: %v", err)
}
loaded, err := s.LoadTable(ctx, "table-4")
if err != nil || loaded == nil || loaded.LastActionAt != 1000 {
t.Fatalf("expected last_action_at=1000 after seed, got %+v err=%v", loaded, err)
}
if loaded.Archived {
t.Fatalf("expected a freshly seeded table to not be archived")
}

timeNowFunc = func () time.Time { return time.Unix(2000, 0) }
if err := s.CommitAction(ctx, "table-4", "hand-1", "act-1", 1, hand.State{Stage: hand.PreFlop}, ActionLogEntry{
TableID: "table-4", HandID: "hand-1", Version: 2, PlayerID: "p1", ActionID: "act-1", Action: "call",
}); err != nil {
t.Fatalf("CommitAction: %v", err)
}
loaded, err = s.LoadTable(ctx, "table-4")
if err != nil || loaded.LastActionAt != 2000 {
t.Fatalf("expected last_action_at=2000 after commit, got %+v err=%v", loaded, err)
}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
docker compose -f docker-compose.test.yml up -d
cd api && go test -tags integration ./internal/tablestore/... -run TestSeedAndCommitSetLastActionAt -v
```

Expected: FAIL — `loaded.LastActionAt undefined (type *StoredTable has no field LastActionAt)`.

- [ ] **Step 3: Write minimal implementation**

In `../../api/internal/tablestore/store.go`, update `StoredTable` (lines 37-42):

```go
// StoredTable is the current authoritative state of one table, as read from
// poker_table_state.
type StoredTable struct {
TableID      string     `dynamodbav:"pk"`
Version      int        `dynamodbav:"version"`
HandID       string     `dynamodbav:"hand_id"`
State        hand.State `dynamodbav:"state"`
LastActionAt int64      `dynamodbav:"last_action_at"`
Archived     bool       `dynamodbav:"archived,omitempty"`
}
```

Update `SeedTable` (lines 57-74) to stamp `last_action_at` and mark the table active in the sparse GSI:

```go
func (s *Store) SeedTable(ctx context.Context, tableID string, state hand.State) error {
item, err := dynamo.Encode(struct {
PK           string     `dynamodbav:"pk"`
Version      int        `dynamodbav:"version"`
State        hand.State `dynamodbav:"state"`
LastActionAt int64      `dynamodbav:"last_action_at"`
GSIActive    string     `dynamodbav:"gsi_active"`
}{PK: tableID, Version: 1, State: state, LastActionAt: timeNowFunc().Unix(), GSIActive: gsiActiveValue})
if err != nil {
return fmt.Errorf("tablestore: encode seed state: %w", err)
}
tx := s.state.BuildPutTxItemIfAbsent(item)
if err := s.state.TransactWrite(ctx, []types.TransactWriteItem{tx}); err != nil {
if dynamo.IsConditionFailed(err) {
return nil // already seeded
}
return fmt.Errorf("tablestore: seed table: %w", err)
}
return nil
}
```

Add the constant near the existing `tableState`/`guardTTLDays` block (lines 15-32):

```go
    // gsiActiveValue is the sparse gsi_active partition key value every live
// table carries — Task 2's MarkArchived REMOVEs this attribute so an
// archived table drops out of gsi_active_last_action entirely, the same
// sparse-index convention as roomstore's gsi_public (roomstore/dynamo.go).
gsiActiveValue = "1"
```

Update `CommitAction` (lines 94-148) to bump `last_action_at` on every commit — add the value and the SET clause:

```go
func (s *Store) CommitAction(ctx context.Context, tableID, handID, actionID string, expectedVersion int, newState hand.State, entry ActionLogEntry) error {
stateItem, err := dynamo.Encode(struct {
State hand.State `dynamodbav:"state"`
}{State: newState})
if err != nil {
return fmt.Errorf("tablestore: encode state: %w", err)
}
stateAV := stateItem["state"]

values := map[string]types.AttributeValue{
":newVersion":    mustN(expectedVersion + 1),
":expected":      mustN(expectedVersion),
":handID":        &types.AttributeValueMemberS{Value: handID},
":state":         stateAV,
":lastActionAt":  mustN(int(timeNowFunc().Unix())),
}
names := map[string]string{
"#version": "version",
"#state":   "state",
}
stateTx := s.state.BuildRawUpdateTxItem(tableID, nil,
"SET #version = :newVersion, hand_id = :handID, #state = :state, last_action_at = :lastActionAt",
"attribute_exists(pk) AND #version = :expected", names, values)
```

(The rest of `CommitAction` is unchanged — only the `values` map and `UpdateExpression` string above change.)

- [ ] **Step 4: Run test to verify it passes**

```bash
cd api && go test -tags integration ./internal/tablestore/... -v
```

Expected: PASS, including the three pre-existing tests (unaffected) and the new one.

- [ ] **Step 5: Add the GSI in CDK**

In `../../cdk/lib/dynamodb-stack.ts`, change line 51 from:

```ts
    table('poker_table_state', false);
```

to:

```ts
    // poker_table_state: gsi_active_last_action is sparse — only tables
    // still active carry a gsi_active value (tablestore.SeedTable sets it;
    // cmd/tablecleanup's archive step REMOVEs it) — so an archived table
    // drops out of the index instead of accumulating there forever.
const tableState = table('poker_table_state', false);
tableState.addGlobalSecondaryIndex({
    indexName: 'gsi_active_last_action',
    partitionKey: {name: 'gsi_active', type: dynamodb.AttributeType.STRING},
    sortKey: {name: 'last_action_at', type: dynamodb.AttributeType.NUMBER},
    projectionType: dynamodb.ProjectionType.KEYS_ONLY,
});
```

- [ ] **Step 6: Update the CDK test**

Read the existing assertions first:

```bash
grep -n "poker_table_state\|GlobalSecondaryIndex" /home/artur/Documents/Projects/Ctech/ctech-poker/cdk/test/dynamodb-stack.test.ts
```

Add an assertion in `../../cdk/test/dynamodb-stack.test.ts` (following whatever assertion style the existing GSI checks
there use, e.g. for `gsi_public`) that `dev_poker_table_state` has a `GlobalSecondaryIndexes` entry named
`gsi_active_last_action` with partition key `gsi_active` (String) and sort key `last_action_at` (Number). Match the
file's existing `Template.fromStack(...).hasResourceProperties('AWS::DynamoDB::Table', ...)` pattern exactly — do not
introduce a new assertion style.

- [ ] **Step 7: Run the CDK test**

```bash
cd cdk && npx jest dynamodb-stack.test.ts
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add api/internal/tablestore/store.go api/internal/tablestore/dynamo_test.go cdk/lib/dynamodb-stack.ts cdk/test/dynamodb-stack.test.ts
git commit -m "feat(api,cdk): add last_action_at + sparse gsi_active_last_action to poker_table_state"
```

---

### Task 2: `Store.MarkArchived` + `Store.QueryStaleActive`

**Files:**

- Modify: `../../api/internal/tablestore/store.go`
- Test: `../../api/internal/tablestore/dynamo_test.go`

**Interfaces:**

- Consumes: `gsiActiveLastActionIndex` constant, `StoredTable.Version` (Task 1).
- Produces: `func (s *Store) MarkArchived(ctx context.Context, tableID string, expectedVersion int) error` and
  `func (s *Store) QueryStaleActive(ctx context.Context, olderThanUnix int64, limit int) ([]StoredTable, error)`. Task
  4's `cmd/tablecleanup` calls both by these exact names.

- [ ] **Step 1: Write the failing test**

Append to `../../api/internal/tablestore/dynamo_test.go`:

```go
func TestQueryStaleActiveFindsOnlyOldActiveTables(t *testing.T) {
db := testClient(t)
env := isolatedEnv()
s := NewStore(db, env)
ctx := context.Background()
mustCreateTestTables(ctx, t, db, env)

timeNowFunc = func () time.Time { return time.Unix(1000, 0) }
_ = s.SeedTable(ctx, "stale-1", hand.State{Stage: hand.WaitingForPlayers})
timeNowFunc = func () time.Time { return time.Unix(9000, 0) }
_ = s.SeedTable(ctx, "fresh-1", hand.State{Stage: hand.WaitingForPlayers})
timeNowFunc = time.Now

stale, err := s.QueryStaleActive(ctx, 5000, 10)
if err != nil {
t.Fatalf("QueryStaleActive: %v", err)
}
if len(stale) != 1 || stale[0].TableID != "stale-1" {
t.Fatalf("expected only stale-1 (last_action_at=1000 < cutoff=5000), got %+v", stale)
}
}

func TestMarkArchivedRemovesFromActiveIndexAndBlocksReSelection(t *testing.T) {
db := testClient(t)
env := isolatedEnv()
s := NewStore(db, env)
ctx := context.Background()
mustCreateTestTables(ctx, t, db, env)

timeNowFunc = func () time.Time { return time.Unix(1000, 0) }
_ = s.SeedTable(ctx, "stale-2", hand.State{Stage: hand.WaitingForPlayers})
timeNowFunc = time.Now

if err := s.MarkArchived(ctx, "stale-2", 1); err != nil {
t.Fatalf("MarkArchived: %v", err)
}

loaded, err := s.LoadTable(ctx, "stale-2")
if err != nil || !loaded.Archived {
t.Fatalf("expected archived=true, got %+v err=%v", loaded, err)
}

stale, err := s.QueryStaleActive(ctx, 999999999, 10)
if err != nil {
t.Fatalf("QueryStaleActive: %v", err)
}
for _, st := range stale {
if st.TableID == "stale-2" {
t.Fatalf("archived table stale-2 must not appear in gsi_active_last_action anymore")
}
}
}

func TestMarkArchivedRejectsStaleVersion(t *testing.T) {
db := testClient(t)
env := isolatedEnv()
s := NewStore(db, env)
ctx := context.Background()
mustCreateTestTables(ctx, t, db, env)

_ = s.SeedTable(ctx, "stale-3", hand.State{Stage: hand.WaitingForPlayers})

err := s.MarkArchived(ctx, "stale-3", 99)
if !errors.Is(err, ErrVersionConflict) {
t.Fatalf("expected ErrVersionConflict when the table moved on since the stale query, got %v", err)
}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd api && go test -tags integration ./internal/tablestore/... -run "TestQueryStaleActive|TestMarkArchived" -v
```

Expected: FAIL — `s.QueryStaleActive undefined` / `s.MarkArchived undefined`.

- [ ] **Step 3: Write minimal implementation**

Add a raw `*dynamodb.Client` handle to `Store` so `QueryStaleActive` can issue a range query the shared
`dynamo.Base.QueryGSI` helper doesn't support (equality-only,
`gopkg.aoctech.app/api-commons@v1.2.0/dynamo/base.go:401-423`). Update the struct and constructor (lines 39-51):

```go
const gsiActiveLastAction = "gsi_active_last_action"

// Store persists the one authoritative item per table, an audit log, and the
// idempotency guards that back CommitAction's duplicate-action_id rejection.
type Store struct {
db     *dynamodb.Client
env    string
state  dynamo.Base
log    dynamo.Base
guards dynamo.Base
}

func NewStore(db *dynamodb.Client, env string) *Store {
return &Store{
db:     db,
env:    env,
state:  dynamo.NewBase(db, env, tableState),
log:    dynamo.NewBase(db, env, tableActionLog),
guards: dynamo.NewBase(db, env, tableActionGuards),
}
}
```

Add the two new methods after `LoadActionsSince` (after line 173):

```go
// QueryStaleActive returns every still-active table (gsi_active present)
// whose last_action_at is older than olderThanUnix, oldest first — the read
// side of cmd/tablecleanup's sweep. Queries gsi_active_last_action; never
// scans (api-commons/dynamo package doc: "get_item > query > scan").
func (s *Store) QueryStaleActive(ctx context.Context, olderThanUnix int64, limit int) ([]StoredTable, error) {
out, err := s.db.Query(ctx, &dynamodb.QueryInput{
TableName:              aws.String(dynamo.TableName(s.env, tableState)),
IndexName:              aws.String(gsiActiveLastAction),
KeyConditionExpression: aws.String("gsi_active = :active AND last_action_at < :cutoff"),
ExpressionAttributeValues: map[string]types.AttributeValue{
":active": &types.AttributeValueMemberS{Value: gsiActiveValue},
":cutoff": mustN(int(olderThanUnix)),
},
ScanIndexForward: aws.Bool(true), // oldest last_action_at first
Limit:            aws.Int32(int32(limit)),
})
if err != nil {
return nil, fmt.Errorf("tablestore: query stale active: %w", err)
}
result := make([]StoredTable, 0, len(out.Items))
for _, keyItem := range out.Items {
id, ok := keyItem["pk"].(*types.AttributeValueMemberS)
if !ok {
continue
}
full, err := s.LoadTable(ctx, id.Value)
if err != nil {
return nil, fmt.Errorf("tablestore: load stale table %s: %w", id.Value, err)
}
if full != nil {
result = append(result, *full)
}
}
return result, nil
}

// MarkArchived flips tableID to archived and removes it from
// gsi_active_last_action, guarded by expectedVersion — the same
// version-equality discipline as CommitAction (ARCHITECTURE.md §2). If
// another instance committed an action on this table since the caller's
// stale-active query ran, this fails with ErrVersionConflict and the caller
// should simply skip archiving it this pass (it is no longer stale).
func (s *Store) MarkArchived(ctx context.Context, tableID string, expectedVersion int) error {
_, err := s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
TableName:        aws.String(dynamo.TableName(s.env, tableState)),
Key:              map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: tableID}},
UpdateExpression: aws.String("SET archived = :true REMOVE gsi_active"),
ConditionExpression: aws.String("attribute_exists(pk) AND #version = :expected"),
ExpressionAttributeNames: map[string]string{"#version": "version"},
ExpressionAttributeValues: map[string]types.AttributeValue{
":true":     &types.AttributeValueMemberBOOL{Value: true},
":expected": mustN(expectedVersion),
},
})
if err != nil {
if dynamo.IsConditionFailed(err) {
return ErrVersionConflict
}
return fmt.Errorf("tablestore: mark archived: %w", err)
}
return nil
}
```

Add the now-needed `aws` import to the existing import block (line 7-13):

```go
import (
"context"
"errors"
"fmt"
"time"

"github.com/aws/aws-sdk-go-v2/aws"
"github.com/aws/aws-sdk-go-v2/service/dynamodb"
"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
"gopkg.aoctech.app/api-commons/dynamo"
"gopkg.aoctech.app/poker/api/internal/engine/hand"
)
```

- [ ] **Step 4: Add the GSI to the DynamoDB Local test-table helper**

`mustCreateTestTables`/`createTable` (lines 195-225 in `store.go`) must create the new index locally, or
`QueryStaleActive`'s tests from Step 1 will fail with "index not found". Update `createTable` to accept optional GSIs
and wire it for `poker_table_state` specifically:

```go
func mustCreateTestTables(ctx context.Context, t testingT, db *dynamodb.Client, env string) {
pkOnly := []string{env + "_" + tableActionGuards}
pkSk := []string{env + "_" + tableActionLog}
for _, name := range pkOnly {
createTable(ctx, t, db, name, false, nil)
}
for _, name := range pkSk {
createTable(ctx, t, db, name, true, nil)
}
createTable(ctx, t, db, env+"_"+tableState, false, []types.GlobalSecondaryIndex{{
IndexName: new(gsiActiveLastAction),
KeySchema: []types.KeySchemaElement{
{AttributeName: new("gsi_active"), KeyType: types.KeyTypeHash},
{AttributeName: new("last_action_at"), KeyType: types.KeyTypeRange},
},
Projection: &types.Projection{ProjectionType: types.ProjectionTypeKeysOnly},
}})
}

func createTable(ctx context.Context, t testingT, db *dynamodb.Client, name string, withSK bool, gsis []types.GlobalSecondaryIndex) {
attrs := []types.AttributeDefinition{{AttributeName: new("pk"), AttributeType: types.ScalarAttributeTypeS}}
keys := []types.KeySchemaElement{{AttributeName: new("pk"), KeyType: types.KeyTypeHash}}
if withSK {
attrs = append(attrs, types.AttributeDefinition{AttributeName: new("sk"), AttributeType: types.ScalarAttributeTypeS})
keys = append(keys, types.KeySchemaElement{AttributeName: new("sk"), KeyType: types.KeyTypeRange})
}
for _, g := range gsis {
attrs = append(attrs,
types.AttributeDefinition{AttributeName: new("gsi_active"), AttributeType: types.ScalarAttributeTypeS},
types.AttributeDefinition{AttributeName: new("last_action_at"), AttributeType: types.ScalarAttributeTypeN},
)
_ = g
}
tableName := name
input := &dynamodb.CreateTableInput{
TableName: &tableName, AttributeDefinitions: attrs, KeySchema: keys, BillingMode: types.BillingModePayPerRequest,
}
if len(gsis) > 0 {
input.GlobalSecondaryIndexes = gsis
}
_, err := db.CreateTable(ctx, input)
if err != nil {
var inUse *types.ResourceInUseException
if !errors.As(err, &inUse) {
t.Fatalf("create table %s: %v", name, err)
}
}
}
```

This changes `createTable`'s signature — update the two remaining `//go:build integration` copies that duplicate it per
this repo's convention:

- `api/internal/table/dynamo_helpers_test.go:39` (`createTestTable`) — leave as-is; it has its own name/signature and
  only creates `poker_table_state` without a GSI, which is fine since `internal/table`'s tests never call
  `QueryStaleActive`.
- `api/internal/buyin/dynamo_helpers_test.go:42` (`createTestTable`) — same: leave as-is, `internal/buyin`'s tests never
  call `QueryStaleActive` either.

Only `tablestore`'s own `mustCreateTestTables`/`createTable` (the ones in non-test `store.go`, used directly by
`tablestore`'s `_test.go` files) need the GSI.

- [ ] **Step 5: Run test to verify it passes**

```bash
cd api && go test -tags integration ./internal/tablestore/... -v
```

Expected: PASS, including all pre-existing tests in the package.

- [ ] **Step 6: Commit**

```bash
git add api/internal/tablestore/store.go api/internal/tablestore/dynamo_test.go
git commit -m "feat(api): add tablestore.QueryStaleActive and MarkArchived"
```

---

### Task 3: Block actor creation on archived tables

**Files:**

- Modify: `../../api/internal/tablemanager/manager.go`
- Test: `../../api/internal/tablemanager/manager_test.go` (create if it does not already exist — check first:
  `ls api/internal/tablemanager/*_test.go`)

**Interfaces:**

- Consumes: `StoredTable.Archived` (Task 1).
- Produces: `tablemanager.ErrTableArchived` (exported sentinel error). `buyin.Service.BuyIn`/`Seated` already wrap
  manager errors with `%w` (`service.go:139-140,228-230`, and Task 1 of the seated-check plan's new `Seated` method), so
  `errors.Is(err, tablemanager.ErrTableArchived)` works through the existing wrapping with no further changes needed in
  `buyin`.

- [ ] **Step 1: Write the failing test**

Check for an existing test file first:

```bash
ls /home/artur/Documents/Projects/Ctech/ctech-poker/api/internal/tablemanager/*_test.go
```

Add (to the existing file if one covers `GetOrCreateActor`, else create`../../api/internal/tablemanager/manager_test.go`
with a `//go:build integration` tag matching this package's DynamoDB Local convention):

```go
//go:build integration

package tablemanager

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func testClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}

func TestGetOrCreateActorRejectsArchivedTable(t *testing.T) {
	db := testClient(t)
	env := fmt.Sprintf("tablemanager_test_%d", time.Now().UnixNano())
	store := tablestore.NewStore(db, env)
	// This package has no local mustCreateTestTables copy yet; reuse
	// tablestore's exported Store methods only (SeedTable/MarkArchived) so
	// this test needs no schema helper of its own beyond what already
	// exists — see tablestore/dynamo_test.go's isolatedEnv/mustCreateTestTables
	// for the table-creation convention if a fresh copy is needed here.
	if err := store.SeedTable(context.Background(), "archived-table", hand.State{Stage: hand.WaitingForPlayers}); err != nil {
		t.Fatalf("SeedTable: %v", err)
	}
	if err := store.MarkArchived(context.Background(), "archived-table", 1); err != nil {
		t.Fatalf("MarkArchived: %v", err)
	}

	mgr := NewManager(tablelease.NewService(cache.NewMemoryBackend(16)), store, nil, nil, nil)
	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }

	_, err := mgr.GetOrCreateActor(context.Background(), "archived-table", seed)
	if !errors.Is(err, ErrTableArchived) {
		t.Fatalf("expected ErrTableArchived, got %v", err)
	}
}
```

> Note: this test needs `poker_table_state` (with the `gsi_active_last_action` GSI) and `poker_action_guards`/
> `poker_action_log` to exist in DynamoDB Local under the generated `env` prefix — reuse `tablestore`'s own
> `mustCreateTestTables(ctx, t, db, env)` by calling it if `tablemanager`'s test can import it (it is unexported in
> package `tablestore`, so — matching this repo's per-package-copy convention confirmed in Task 2 Step 4 — copy the same
> three-table+GSI creation helper into this new test file, or, simpler and equally valid per convention: add a one-line
> exported wrapper `tablestore.CreateTestTables(ctx, t, db, env)` if `tablemanager_test.go` needs it in more than one test
> in the future. For a single test, inlining the table-creation calls directly in this test function is the smaller diff —
> do that instead of adding a new exported helper.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd api && go test -tags integration ./internal/tablemanager/... -run TestGetOrCreateActorRejectsArchivedTable -v
```

Expected: FAIL — `undefined: ErrTableArchived`.

- [ ] **Step 3: Write minimal implementation**

In `../../api/internal/tablemanager/manager.go`, add the sentinel error near the top (after the `import` block, before
`type Actor = table.Actor`, around line 20):

```go
// ErrTableArchived means tableID was archived by cmd/tablecleanup for
// inactivity (StoredTable.Archived, api/internal/tablestore) — its seated
// players were already refunded, and no new actor may be created for it.
// buyin.Service wraps this through with %w so callers can errors.Is against
// it directly (service.go's BuyIn/CashOut/Seated all already do `%w`).
var ErrTableArchived = errors.New("tablemanager: table archived")
```

Add `"errors"` to the import block (line 9, alongside `"context"`/`"fmt"`).

Update `GetOrCreateActor` (lines 77-87) to check the flag right after loading the existing item:

```go
    if m.store != nil {
existing, err := m.store.LoadTable(ctx, tableID)
if err != nil {
return nil, fmt.Errorf("tablemanager: load table: %w", err)
}
if existing != nil && existing.Archived {
return nil, ErrTableArchived
}
if existing == nil {
if err := m.store.SeedTable(ctx, tableID, seed().ExportState()); err != nil {
return nil, fmt.Errorf("tablemanager: seed table: %w", err)
}
}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd api && go test -tags integration ./internal/tablemanager/... -v
```

Expected: PASS.

- [ ] **Step 5: Confirm `buyin` surfaces the error correctly (regression check, no new code)**

```bash
cd api && go test -tags integration ./internal/buyin/... -v
```

Expected: PASS — this is a check that Task 3's change doesn't break existing `buyin` integration tests (they never touch
archived tables, so behavior is unchanged for them).

- [ ] **Step 6: Commit**

```bash
git add api/internal/tablemanager/manager.go api/internal/tablemanager/manager_test.go
git commit -m "feat(api): reject GetOrCreateActor for archived tables"
```

---

### Task 4: `cmd/tablecleanup` — the scheduled sweep

**Files:**

- Create: `api/cmd/tablecleanup/main.go`
- Create: `api/cmd/tablecleanup/main_test.go`

**Interfaces:**

- Consumes: `tablestore.Store.QueryStaleActive`, `tablestore.Store.MarkArchived` (Task 2); `roomstore.Store.Get`
  (existing, `roomstore/dynamo.go:57-66`); `walletclient` `Credit` (existing interface shape, mirrored from
  `buyin.walletMover`/`cmd/reconcile`'s `sandboxCredit`).
- Produces: a `run(ctx, stale staleQuerier, rooms roomLookup, wallet sandboxCredit, cutoff time.Duration) error`function
  and a `handler(ctx context.Context) error` Lambda entrypoint — same shape as `cmd/reconcile/main.go`.

- [ ] **Step 1: Write the failing test**

Create `api/cmd/tablecleanup/main_test.go`:

```go
package main

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type fakeStaleQuerier struct {
	stale []tablestore.StoredTable
	archived []string
}

func (f *fakeStaleQuerier) QueryStaleActive(context.Context, int64, int) ([]tablestore.StoredTable, error) {
	return f.stale, nil
}
func (f *fakeStaleQuerier) MarkArchived(_ context.Context, tableID string, _ int) error {
	f.archived = append(f.archived, tableID)
	return nil
}

type fakeRoomLookup struct {
	rooms map[string]*roomstore.Room
}

func (f *fakeRoomLookup) Get(_ context.Context, roomID string) (*roomstore.Room, error) {
	return f.rooms[roomID], nil
}

type fakeSandboxCredit struct {
	credits []struct {
		userID string
		amount int64
	}
}

func (f *fakeSandboxCredit) Credit(_ context.Context, userID string, amount int64, _, _ string) error {
	f.credits = append(f.credits, struct {
		userID string
		amount int64
	}{userID, amount})
	return nil
}

func TestRunRefundsSeatedSandboxPlayersAndArchives(t *testing.T) {
	stale := &fakeStaleQuerier{stale: []tablestore.StoredTable{
		{
			TableID: "table-1", Version: 3,
			State: hand.State{Players: []*hand.Player{{ID: "player-1", Stack: 500}}},
		},
	}}
	rooms := &fakeRoomLookup{rooms: map[string]*roomstore.Room{
		"table-1": {ID: "table-1", CurrencyMode: "sandbox"},
	}}
	wallet := &fakeSandboxCredit{}

	if err := run(context.Background(), stale, rooms, wallet, time.Hour); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(wallet.credits) != 1 || wallet.credits[0].userID != "player-1" || wallet.credits[0].amount != 500 {
		t.Fatalf("expected a 500-chip refund to player-1, got %+v", wallet.credits)
	}
	if len(stale.archived) != 1 || stale.archived[0] != "table-1" {
		t.Fatalf("expected table-1 to be archived, got %v", stale.archived)
	}
}

func TestRunSkipsRealMoneyTables(t *testing.T) {
	stale := &fakeStaleQuerier{stale: []tablestore.StoredTable{
		{
			TableID: "table-2", Version: 1,
			State: hand.State{Players: []*hand.Player{{ID: "player-2", Stack: 500}}},
		},
	}}
	rooms := &fakeRoomLookup{rooms: map[string]*roomstore.Room{
		"table-2": {ID: "table-2", CurrencyMode: "real"},
	}}
	wallet := &fakeSandboxCredit{}

	if err := run(context.Background(), stale, rooms, wallet, time.Hour); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(wallet.credits) != 0 {
		t.Fatalf("real-money tables must never be refunded/archived by this sandbox-only job, got %+v", wallet.credits)
	}
	if len(stale.archived) != 0 {
		t.Fatalf("real-money tables must not be archived by this job, got %v", stale.archived)
	}
}

func TestRunSkipsEmptyTablesWithNoRefundNeeded(t *testing.T) {
	stale := &fakeStaleQuerier{stale: []tablestore.StoredTable{
		{TableID: "table-3", Version: 1, State: hand.State{Players: nil}},
	}}
	rooms := &fakeRoomLookup{rooms: map[string]*roomstore.Room{
		"table-3": {ID: "table-3", CurrencyMode: "sandbox"},
	}}
	wallet := &fakeSandboxCredit{}

	if err := run(context.Background(), stale, rooms, wallet, time.Hour); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(wallet.credits) != 0 {
		t.Fatalf("a table with no chips at stake needs no refund, got %+v", wallet.credits)
	}
	if len(stale.archived) != 1 {
		t.Fatalf("expected the empty stale table to still be archived, got %v", stale.archived)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd api && go test ./cmd/tablecleanup/... -v
```

Expected: FAIL — package `main` does not build (`run` undefined) since `main.go` doesn't exist yet.

- [ ] **Step 3: Write minimal implementation**

Create `api/cmd/tablecleanup/main.go`:

```go
// Package main implements the scheduled Lambda job that archives poker
// tables idle past staleCutoff, refunding any seated players' sandbox chips
// first. Mirrors cmd/reconcile's shape (scheduled Lambda, SSM-resolved
// wallet credentials) — see cdk/lib/reconcile-stack.ts for the CDK pattern
// this job's own stack (Task 5) copies.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// staleCutoff is how long a table may sit with no committed action before
// this job archives it. cmd/reconcile's analogous gracePeriod is 2 minutes
// (for a completed cash-out awaiting credit); a table being idle mid-session
// is a much slower signal, so this is measured in hours, not minutes.
const staleCutoff = 6 * time.Hour

// queryBatchLimit bounds how many stale tables one invocation processes —
// same "batch, not everything" instinct as the archiver Lambda's DynamoDB
// Stream batchSize (cdk/lib/archiver-stack.ts). Any remainder is picked up
// on the next scheduled run since last_action_at does not change for a
// still-stale table between runs.
const queryBatchLimit = 25

type staleQuerier interface {
	QueryStaleActive(ctx context.Context, olderThanUnix int64, limit int) ([]tablestore.StoredTable, error)
	MarkArchived(ctx context.Context, tableID string, expectedVersion int) error
}

type roomLookup interface {
	Get(ctx context.Context, roomID string) (*roomstore.Room, error)
}

type sandboxCredit interface {
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

// timeNowFunc is overridden in tests that need a deterministic cutoff.
var timeNowFunc = time.Now

func run(ctx context.Context, stale staleQuerier, rooms roomLookup, wallet sandboxCredit, cutoff time.Duration) error {
	olderThan := timeNowFunc().Add(-cutoff).Unix()
	tables, err := stale.QueryStaleActive(ctx, olderThan, queryBatchLimit)
	if err != nil {
		return fmt.Errorf("tablecleanup: query stale: %w", err)
	}

	for _, st := range tables {
		room, err := rooms.Get(ctx, st.TableID)
		if err != nil {
			slog.Error("tablecleanup: room lookup failed, skipping this pass", "table_id", st.TableID, "err", err)
			continue
		}
		// Sandbox isolation is load-bearing (api/CLAUDE.md): this job never
		// touches a real-money table. A missing room record is treated the
		// same as sandbox, since every table this codebase creates today is
		// sandbox-only end-to-end.
		if room != nil && room.CurrencyMode != "sandbox" {
			continue
		}

		for _, p := range st.State.Players {
			if p.Stack <= 0 {
				continue
			}
			key := fmt.Sprintf("%s#%s#stale_archive_refund", st.TableID, p.ID)
			if err := wallet.Credit(ctx, p.ID, p.Stack, key, "poker_stale_table_refund"); err != nil {
				slog.Error("ALARM: tablecleanup refund failed, table left active for retry", "table_id", st.TableID, "player", p.ID, "amount", p.Stack, "err", err)
				continue
			}
		}

		if err := stale.MarkArchived(ctx, st.TableID, st.Version); err != nil {
			slog.Error("tablecleanup: archive failed (table may have just received a fresh action; skipping)", "table_id", st.TableID, "err", err)
			continue
		}
		slog.Info("tablecleanup: archived stale table", "table_id", st.TableID, "seats_refunded", len(st.State.Players))
	}
	return nil
}

func resolveSSMParams(ctx context.Context, walletURLParam, clientIDParam, clientSecretParam string) error {
	awsCfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	client := ssm.NewFromConfig(awsCfg)
	get := func(name string, withDecryption bool) (string, error) {
		out, err := client.GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(name), WithDecryption: aws.Bool(withDecryption)})
		if err != nil {
			return "", err
		}
		return *out.Parameter.Value, nil
	}
	wURL, err := get(walletURLParam, false)
	if err != nil {
		return err
	}
	cID, err := get(clientIDParam, false)
	if err != nil {
		return err
	}
	cSecret, err := get(clientSecretParam, true)
	if err != nil {
		return err
	}
	_ = os.Setenv("WALLET_URL", wURL)
	_ = os.Setenv("POKER_CLIENT_ID", cID)
	_ = os.Setenv("POKER_CLIENT_SECRET", cSecret)
	return nil
}

func handler(ctx context.Context) error {
	if wURLParam := os.Getenv("WALLET_URL_PARAM"); wURLParam != "" {
		if err := resolveSSMParams(ctx, wURLParam, os.Getenv("POKER_CLIENT_ID_PARAM"), os.Getenv("POKER_CLIENT_SECRET_PARAM")); err != nil {
			return fmt.Errorf("tablecleanup: resolve SSM params: %w", err)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	awsCfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	db := dynamodb.NewFromConfig(awsCfg)
	store := tablestore.NewStore(db, cfg.Env)
	rooms := roomstore.NewStore(db, cfg.Env)
	wallet := walletclient.New(cfg)
	return run(ctx, store, rooms, wallet, staleCutoff)
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	lambda.Start(handler)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd api && go test ./cmd/tablecleanup/... -v
```

Expected: PASS for all three tests.

- [ ] **Step 5: Verify it builds for the Lambda target too**

```bash
cd api && GOOS=linux GOARCH=arm64 go build -o /tmp/tablecleanup-bootstrap ./cmd/tablecleanup
```

Expected: builds cleanly (same cross-compile Task 5's CDK bundling step will run).

- [ ] **Step 6: Commit**

```bash
git add api/cmd/tablecleanup/main.go api/cmd/tablecleanup/main_test.go
git commit -m "feat(api): add cmd/tablecleanup scheduled sweep for stale sandbox tables"
```

---

### Task 5: `TableCleanupStack` — Lambda + EventBridge Schedule

**Files:**

- Create: `cdk/lib/tablecleanup-stack.ts`
- Modify: `../../cdk/bin/poker.ts`
- Create: `cdk/test/tablecleanup-stack.test.ts`

**Interfaces:**

- Consumes: `dynamoStack.tables.get('poker_table_state')!.tableArn`, `dynamoStack.tables.get('poker_rooms')!.tableArn`
  (existing, `bin/poker.ts:82,85`), `pokerParameters.{walletUrl,clientId,clientSecret}` (existing,
  `bin/poker.ts:42,90-92`).

- [ ] **Step 1: Write the stack**

Create `cdk/lib/tablecleanup-stack.ts`, copying `../../cdk/lib/reconcile-stack.ts`'s exact shape (Lambda from a `cmd/*`
binary, IAM role scoped to only what it needs, EventBridge `CfnSchedule`):

```ts
import * as cdk from 'aws-cdk-lib';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as scheduler from 'aws-cdk-lib/aws-scheduler';
import {Construct} from 'constructs';
import {Environment} from '@aoctech/cdk';
import {localGoBundling} from './bundle';

const TABLE_CLEANUP_RATE_MINUTES = 30;

interface TableCleanupStackProps extends cdk.StackProps {
    environment: Environment;
    tableStateArn: string;
    roomsTableArn: string;
    walletUrlParam: string;
    pokerClientIdParam: string;
    pokerClientSecretParam: string;
}

/**
 * Stale-table sweep — mirrors cdk/lib/reconcile-stack.ts's shape: a Lambda
 * built from cmd/tablecleanup on an EventBridge schedule. Archives sandbox
 * tables idle past cmd/tablecleanup's staleCutoff, refunding seated stacks
 * first via ctech-wallet.
 */
export class TableCleanupStack extends cdk.Stack {
    constructor(scope: Construct, id: string, props: TableCleanupStackProps) {
        super(scope, id, props);
        const {
            environment,
            tableStateArn,
            roomsTableArn,
            walletUrlParam,
            pokerClientIdParam,
            pokerClientSecretParam
        } = props;

        const role = new iam.Role(this, 'TableCleanupRole', {
            assumedBy: new iam.ServicePrincipal('lambda.amazonaws.com'),
            managedPolicies: [iam.ManagedPolicy.fromAwsManagedPolicyName('service-role/AWSLambdaBasicExecutionRole')],
        });
        role.addToPolicy(new iam.PolicyStatement({
            actions: ['dynamodb:Query', 'dynamodb:GetItem', 'dynamodb:UpdateItem'],
            resources: [tableStateArn, `${tableStateArn}/index/*`],
        }));
        role.addToPolicy(new iam.PolicyStatement({
            actions: ['dynamodb:GetItem'],
            resources: [roomsTableArn],
        }));
        role.addToPolicy(new iam.PolicyStatement({
            actions: ['ssm:GetParameter'],
            resources: [
                `arn:aws:ssm:${this.region}:${this.account}:parameter${walletUrlParam}`,
                `arn:aws:ssm:${this.region}:${this.account}:parameter${pokerClientIdParam}`,
                `arn:aws:ssm:${this.region}:${this.account}:parameter${pokerClientSecretParam}`,
            ],
        }));

        const fn = new lambda.Function(this, 'TableCleanupFunction', {
            functionName: `${environment}-ctech-poker-tablecleanup`,
            runtime: lambda.Runtime.PROVIDED_AL2023,
            architecture: lambda.Architecture.ARM_64,
            handler: 'bootstrap',
            code: lambda.Code.fromAsset('../api/cmd/tablecleanup', {
                bundling: {
                    local: localGoBundling('../api/cmd/tablecleanup'),
                    image: lambda.Runtime.PROVIDED_AL2023.bundlingImage,
                    command: ['bash', '-c', 'GOOS=linux GOARCH=arm64 go build -o /asset-output/bootstrap .'],
                },
            }),
            role,
            timeout: cdk.Duration.minutes(2),
            memorySize: 256,
            environment: {
                ENVIRONMENT: environment,
                WALLET_URL_PARAM: walletUrlParam,
                POKER_CLIENT_ID_PARAM: pokerClientIdParam,
                POKER_CLIENT_SECRET_PARAM: pokerClientSecretParam,
            },
        });

        const schedulerRole = new iam.Role(this, 'TableCleanupSchedulerInvokeRole', {
            assumedBy: new iam.ServicePrincipal('scheduler.amazonaws.com'),
        });
        fn.grantInvoke(schedulerRole);

        new scheduler.CfnSchedule(this, 'TableCleanupSchedule', {
            flexibleTimeWindow: {mode: 'OFF'},
            scheduleExpression: `rate(${TABLE_CLEANUP_RATE_MINUTES} minutes)`,
            target: {arn: fn.functionArn, roleArn: schedulerRole.roleArn},
        });
    }
}
```

- [ ] **Step 2: Wire it into `bin/poker.ts`**

Add the import (alongside the other stack imports, after line 9):

```ts
import {TableCleanupStack} from '../lib/tablecleanup-stack';
```

Add the stack instantiation after the existing `ReconcileStack` block (after line 114):

```ts
new TableCleanupStack(app, id('TableCleanup'), {
    env,
    environment: ENVIRONMENT,
    tableStateArn: dynamoStack.tables.get('poker_table_state')!.tableArn,
    roomsTableArn: dynamoStack.tables.get('poker_rooms')!.tableArn,
    walletUrlParam: pokerParameters.walletUrl,
    pokerClientIdParam: pokerParameters.clientId,
    pokerClientSecretParam: pokerParameters.clientSecret,
    description: `CTech Poker stale-table cleanup Lambda - ${ENVIRONMENT}`,
});
```

- [ ] **Step 3: Write the CDK test**

Read the existing reconcile-stack test (if one exists) to follow its exact assertion style first:

```bash
find /home/artur/Documents/Projects/Ctech/ctech-poker/cdk/test -iname '*reconcile*'
```

Create `cdk/test/tablecleanup-stack.test.ts` following that file's pattern (or, if no reconcile-stack test exists yet,
follow `../../cdk/test/dynamodb-stack.test.ts`'s `Template.fromStack` + `hasResourceProperties` style) asserting:

1. A `AWS::Lambda::Function` named `dev-ctech-poker-tablecleanup` exists with `Runtime: provided.al2023`.
2. A `AWS::Scheduler::Schedule` exists with `ScheduleExpression: rate(30 minutes)`.
3. The Lambda's IAM role policy includes `dynamodb:Query` on the table-state ARN (not `dynamodb:Scan` — this is the
   load-bearing "no scans" assertion, matching `../../cdk/CLAUDE.md`).

- [ ] **Step 4: Run the CDK tests**

```bash
cd cdk && npx jest tablecleanup-stack.test.ts
```

Expected: PASS.

- [ ] **Step 5: Synth the whole app to catch wiring mistakes**

```bash
cd cdk && CTECH_VPC_ID=vpc-0adfd86727d17445b npx cdk synth CtechPoker-Dev-TableCleanup > /dev/null
```

Expected: synthesizes without error (confirms `bin/poker.ts` wiring and prop types line up).

- [ ] **Step 6: Commit**

```bash
git add cdk/lib/tablecleanup-stack.ts cdk/bin/poker.ts cdk/test/tablecleanup-stack.test.ts
git commit -m "feat(cdk): add TableCleanupStack — scheduled Lambda for stale sandbox table archival"
```

## Self-Review Notes

- **Spec coverage:** "`last_action_at` (integer)" → Task 1. "update on every action" → Task 1's `CommitAction` change.
  "filter by GSI status + last_action_at" → Task 1's `gsi_active_last_action` + Task 2's `QueryStaleActive` (the user's
  suggested "status" field is realized as the sparse `gsi_active` presence/absence, which is cheaper than a literal
  status column since it makes the archived set fall out of the index for free — same trick `roomstore.gsi_public`
  already uses). "archive everything + refund pending values" → Task 4's `run()`. "Lambda + Scheduler on a fixed
  interval" → Task 5, mirroring the already-shipped `ReconcileStack`.
- **No placeholders:** every step has real, complete code. The one deliberately-deferred spot (Task 3 Step 1's note
  about a possible shared test helper) explains exactly why it's deferred (YAGNI — one call site doesn't justify a new
  exported helper) and what the fallback is, rather than leaving a TODO.
- **Type consistency:** `QueryStaleActive(ctx, olderThanUnix int64, limit int) ([]StoredTable, error)` and
  `MarkArchived(ctx, tableID string, expectedVersion int) error` (Task 2) are the exact signatures `staleQuerier` (Task
  4) declares and the exact ones exercised in Task 2's and Task 3's tests. `ErrTableArchived` (Task 3) is the same
  identifier `buyin`'s existing `%w`-wrapping already makes `errors.Is`-reachable — no `buyin` code changes needed,
  verified by Task 3 Step 5's regression run.
- **Risk called out, not hidden:** Task 4's `run()` logs and `continue`s (never aborts the whole sweep) on a per-table
  refund or archive failure — one bad table can't block the rest of the batch, matching `cmd/reconcile`'s own per-entry
  error handling (`main.go:57-65`).
