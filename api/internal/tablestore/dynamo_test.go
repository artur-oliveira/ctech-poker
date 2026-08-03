//go:build integration

package tablestore

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
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func createTable(ctx context.Context, t testingT, db *dynamodb.Client, name string, withSK bool, gsis []types.GlobalSecondaryIndex) {
	attrs := []types.AttributeDefinition{{AttributeName: new("pk"), AttributeType: types.ScalarAttributeTypeS}}
	keys := []types.KeySchemaElement{{AttributeName: new("pk"), KeyType: types.KeyTypeHash}}
	if withSK {
		attrs = append(attrs, types.AttributeDefinition{AttributeName: new("sk"), AttributeType: types.ScalarAttributeTypeS})
		keys = append(keys, types.KeySchemaElement{AttributeName: new("sk"), KeyType: types.KeyTypeRange})
	}
	if len(gsis) > 0 {
		attrs = append(attrs,
			types.AttributeDefinition{AttributeName: new("gsi_active"), AttributeType: types.ScalarAttributeTypeS},
			types.AttributeDefinition{AttributeName: new("last_action_at"), AttributeType: types.ScalarAttributeTypeN},
		)
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

// testingT is the minimal *testing.T surface these helpers need, kept as an
// unexported interface so this file (non-test code) never imports "testing".
type testingT interface{ Fatalf(string, ...any) }

// mustCreateTestTables provisions all four tables against DynamoDB Local —
// production tables are provisioned by CDK, never by app code.
func mustCreateTestTables(ctx context.Context, t testingT, db *dynamodb.Client, env string) {
	createTable(ctx, t, db, env+"_"+tableActionGuards, false, nil)
	createTable(ctx, t, db, env+"_"+tableActionLog, true, nil)
	createTable(ctx, t, db, env+"_"+tableStateHistory, true, nil)
	createTable(ctx, t, db, env+"_"+tableState, false, []types.GlobalSecondaryIndex{{
		IndexName: new(gsiActiveLastAction),
		KeySchema: []types.KeySchemaElement{
			{AttributeName: new("gsi_active"), KeyType: types.KeyTypeHash},
			{AttributeName: new("last_action_at"), KeyType: types.KeyTypeRange},
		},
		Projection: &types.Projection{ProjectionType: types.ProjectionTypeKeysOnly},
	}})
}

func testClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://localhost:8555")
	})
}

func isolatedEnv() string { return fmt.Sprintf("tablestore_test_%d", time.Now().UnixNano()) }

func TestSeedThenCommitThenLoad(t *testing.T) {
	db := testClient(t)
	env := isolatedEnv()
	s := NewStore(db, env)
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, env)

	if err := s.SeedTable(ctx, "table-1", hand.State{Stage: hand.WaitingForPlayers}); err != nil {
		t.Fatalf("SeedTable: %v", err)
	}

	loaded, err := s.LoadTable(ctx, "table-1")
	if err != nil || loaded == nil || loaded.Version != 1 {
		t.Fatalf("expected version 1 after seed, got %+v err=%v", loaded, err)
	}

	newState := hand.State{Stage: hand.PreFlop}
	if err := s.CommitAction(ctx, "table-1", "hand-1", "act-1", 1, newState, TableActivity{}, 0, ActionLogEntry{
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
	env := isolatedEnv()
	s := NewStore(db, env)
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, env)

	_ = s.SeedTable(ctx, "table-2", hand.State{Stage: hand.WaitingForPlayers})

	err := s.CommitAction(ctx, "table-2", "hand-1", "act-1", 99, hand.State{}, TableActivity{}, 0, ActionLogEntry{TableID: "table-2", HandID: "hand-1", Version: 100, ActionID: "act-1"})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected ErrVersionConflict against a stale expected version, got %v", err)
	}
}

func TestSeedAndCommitSetLastActionAt(t *testing.T) {
	db := testClient(t)
	env := isolatedEnv()
	s := NewStore(db, env)
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, env)

	timeNowFunc = func() time.Time { return time.Unix(1000, 0) }
	defer func() { timeNowFunc = time.Now }()

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

	timeNowFunc = func() time.Time { return time.Unix(2000, 0) }
	if err := s.CommitAction(ctx, "table-4", "hand-1", "act-1", 1, hand.State{Stage: hand.PreFlop}, TableActivity{}, 0, ActionLogEntry{
		TableID: "table-4", HandID: "hand-1", Version: 2, PlayerID: "p1", ActionID: "act-1", Action: "call",
	}); err != nil {
		t.Fatalf("CommitAction: %v", err)
	}
	loaded, err = s.LoadTable(ctx, "table-4")
	if err != nil || loaded.LastActionAt != 2000 {
		t.Fatalf("expected last_action_at=2000 after commit, got %+v err=%v", loaded, err)
	}
}

// TestCommitActionStampsLogEntryTimestamp pins down that every ActionLogEntry
// gets a timestamp set by CommitAction itself (unix millis), regardless of
// what the caller passed in — actor.go never sets it, so without this the
// audit log had no record of when each action happened.
func TestCommitActionStampsLogEntryTimestamp(t *testing.T) {
	db := testClient(t)
	env := isolatedEnv()
	s := NewStore(db, env)
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, env)

	timeNowFunc = func() time.Time { return time.Unix(5000, 0) }
	defer func() { timeNowFunc = time.Now }()

	_ = s.SeedTable(ctx, "table-5", hand.State{Stage: hand.WaitingForPlayers})
	if err := s.CommitAction(ctx, "table-5", "hand-1", "act-1", 1, hand.State{Stage: hand.PreFlop}, TableActivity{}, 0, ActionLogEntry{
		TableID: "table-5", HandID: "hand-1", Version: 2, PlayerID: "p1", ActionID: "act-1", Action: "call",
	}); err != nil {
		t.Fatalf("CommitAction: %v", err)
	}

	entries, err := s.LoadActionsSince(ctx, "table-5", "hand-1", 0)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one logged action, got %+v err=%v", entries, err)
	}
	if want := time.Unix(5000, 0).UnixMilli(); entries[0].Timestamp != want {
		t.Fatalf("expected logged action timestamp %d, got %d", want, entries[0].Timestamp)
	}
}

// TestLoadActionsSinceReturnsChronologicalOrder pins down that actions come
// back oldest-first: hand-share/hand-history replay both trust list order as
// the timeline, and the position-based Seq fallback below assigns 1, 2, 3...
// off that same order — either one silently reverses if the underlying query
// ever regresses to DynamoDB's default newest-first order.
func TestLoadActionsSinceReturnsChronologicalOrder(t *testing.T) {
	db := testClient(t)
	env := isolatedEnv()
	s := NewStore(db, env)
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, env)

	_ = s.SeedTable(ctx, "table-6", hand.State{Stage: hand.WaitingForPlayers})
	for i, action := range []string{"call", "check", "raise"} {
		if err := s.CommitAction(ctx, "table-6", "hand-1", fmt.Sprintf("act-%d", i+1), i+1, hand.State{}, TableActivity{}, 0, ActionLogEntry{
			TableID: "table-6", HandID: "hand-1", Version: i + 2, PlayerID: "p1", ActionID: fmt.Sprintf("act-%d", i+1), Action: action,
		}); err != nil {
			t.Fatalf("CommitAction %s: %v", action, err)
		}
	}

	entries, err := s.LoadActionsSince(ctx, "table-6", "hand-1", 0)
	if err != nil || len(entries) != 3 {
		t.Fatalf("expected 3 logged actions, got %+v err=%v", entries, err)
	}
	want := []string{"call", "check", "raise"}
	for i, entry := range entries {
		if entry.Action != want[i] || entry.Seq != i+1 {
			t.Fatalf("expected entries oldest-first %v with seq 1..3, got %+v", want, entries)
		}
	}
}

func TestQueryStaleActiveFindsOnlyOldActiveTables(t *testing.T) {
	db := testClient(t)
	env := isolatedEnv()
	s := NewStore(db, env)
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, env)

	timeNowFunc = func() time.Time { return time.Unix(1000, 0) }
	_ = s.SeedTable(ctx, "stale-1", hand.State{Stage: hand.WaitingForPlayers})
	timeNowFunc = func() time.Time { return time.Unix(9000, 0) }
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

	timeNowFunc = func() time.Time { return time.Unix(1000, 0) }
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

func TestSaveTableStateHistoryPersistsSnapshot(t *testing.T) {
	db := testClient(t)
	env := isolatedEnv()
	s := NewStore(db, env)
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, env)

	if err := s.SaveTableStateHistory(ctx, "table-5", 1234567890, hand.State{Stage: hand.Complete}); err != nil {
		t.Fatalf("SaveTableStateHistory: %v", err)
	}

	item, err := s.history.GetItem(ctx, "table-5", "1234567890")
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	loaded, err := dynamo.Decode[StoredTable](item)
	if err != nil || loaded == nil || loaded.State.Stage != hand.Complete {
		t.Fatalf("expected a persisted Complete-stage snapshot, got %+v err=%v", loaded, err)
	}
}

func TestCommitActionRejectsDuplicateActionID(t *testing.T) {
	db := testClient(t)
	env := isolatedEnv()
	s := NewStore(db, env)
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, env)

	_ = s.SeedTable(ctx, "table-3", hand.State{Stage: hand.WaitingForPlayers})
	entry := ActionLogEntry{TableID: "table-3", HandID: "hand-1", Version: 2, ActionID: "dup-1"}
	if err := s.CommitAction(ctx, "table-3", "hand-1", "dup-1", 1, hand.State{Stage: hand.PreFlop}, TableActivity{}, 0, entry); err != nil {
		t.Fatalf("first commit: %v", err)
	}

	err := s.CommitAction(ctx, "table-3", "hand-1", "dup-1", 2, hand.State{Stage: hand.Flop}, TableActivity{}, 0, ActionLogEntry{TableID: "table-3", HandID: "hand-1", Version: 3, ActionID: "dup-1"})
	if !errors.Is(err, ErrDuplicateAction) {
		t.Fatalf("expected ErrDuplicateAction on a replayed action_id, got %v", err)
	}
}

// TestCommitActionIncludesExtraItemsWithoutActionID guards against the
// leave/kick settlement bug fixed 2026-08-03: buyin.Service's
// pending-cashout row rides in as an `extra` TransactWriteItem, but
// LeaveCmd (internal/table/commands.go) never carries an ActionID — a
// version of CommitAction that only appended extra inside the
// `actionID != ""` branch silently dropped it from every single leave
// (manual cash-out or system kick/AFK), committing the seat removal with
// no recovery row ever written. See
// docs/plans/2026-08-03-leave-settlement-atomicity.md.
func TestCommitActionIncludesExtraItemsWithoutActionID(t *testing.T) {
	db := testClient(t)
	env := isolatedEnv()
	s := NewStore(db, env)
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, env)
	createTable(ctx, t, db, env+"_extra_scratch", false, nil)
	extra := dynamo.NewBase(db, env, "extra_scratch")

	_ = s.SeedTable(ctx, "table-6", hand.State{Stage: hand.WaitingForPlayers})

	extraItem, err := dynamo.Encode(struct {
		PK string `dynamodbav:"pk"`
	}{PK: "settlement-1"})
	if err != nil {
		t.Fatalf("encode extra item: %v", err)
	}
	extraTx := extra.BuildPutTxItem(extraItem)

	entry := ActionLogEntry{TableID: "table-6", HandID: "hand-1", Version: 2}
	// actionID is "" on purpose here — mirrors every real Leave commit.
	if err := s.CommitAction(ctx, "table-6", "hand-1", "", 1, hand.State{Stage: hand.WaitingForPlayers}, TableActivity{}, 0, entry, extraTx); err != nil {
		t.Fatalf("CommitAction: %v", err)
	}

	item, err := extra.GetItem(ctx, "settlement-1")
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if item == nil {
		t.Fatal("expected the extra transact item to be written alongside the state commit even with an empty actionID")
	}
}
