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
	// happens: the archiver Lambda (cdk/lib/archiver-stack.ts) ships every
	// entry to S3 on insert, independent of and well before its eventual
	// TTL expiry — DynamoDB serves the recent window, S3 is the
	// indefinite archive.
	logTTLDays = 90

	// gsiActiveValue is the sparse gsi_active partition key value every live
	// table carries — MarkArchived REMOVEs this attribute so an archived
	// table drops out of gsi_active_last_action entirely, the same
	// sparse-index convention as roomstore's gsi_public (roomstore/dynamo.go).
	gsiActiveValue = "1"
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

// SeedTable creates a table's very first state item at version 1. It is a
// conditional create (attribute_not_exists(pk)) so a first-touch race between
// two instances can never clobber an already-seeded table (M4). If the table
// already exists the conditional fails and we treat it as a successful no-op.
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
		":newVersion":   mustN(expectedVersion + 1),
		":expected":     mustN(expectedVersion),
		":handID":       &types.AttributeValueMemberS{Value: handID},
		":state":        stateAV,
		":lastActionAt": mustN(int(timeNowFunc().Unix())),
	}
	names := map[string]string{
		"#version": "version",
		"#state":   "state",
	}
	stateTx := s.state.BuildRawUpdateTxItem(tableID, nil,
		"SET #version = :newVersion, hand_id = :handID, #state = :state, last_action_at = :lastActionAt",
		"attribute_exists(pk) AND #version = :expected", names, values)

	logItem, err := dynamo.Encode(struct {
		PK  string `dynamodbav:"pk"`
		SK  string `dynamodbav:"sk"`
		TTL int64  `dynamodbav:"ttl"`
		ActionLogEntry
	}{
		PK: tableID + "#" + handID, SK: fmt.Sprintf("%010d", entry.Version),
		TTL:            timeNowFunc().Add(logTTLDays * 24 * time.Hour).Unix(),
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
func (s *Store) LoadActionsSince(ctx context.Context, tableID, handID string, afterSeq int) ([]ActionLogEntry, error) {
	pk := tableID + "#" + handID
	result, err := s.log.Query(ctx, dynamo.QueryOpts{PK: pk})
	if err != nil {
		return nil, fmt.Errorf("tablestore: load actions: %w", err)
	}
	out := make([]ActionLogEntry, 0, len(result.Items))
	for i, item := range result.Items {
		e, err := dynamo.Decode[ActionLogEntry](item)
		if err != nil || e == nil {
			continue
		}
		if e.Seq == 0 {
			e.Seq = i + 1
		}
		if e.Seq > afterSeq {
			out = append(out, *e)
		}
	}
	return out, nil
}

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

// mustCreateTestTables provisions all three tables against DynamoDB Local —
// production tables are provisioned by CDK, never by app code.
func mustCreateTestTables(ctx context.Context, t testingT, db *dynamodb.Client, env string) {
	pkOnly := []string{env + "_" + tableState, env + "_" + tableActionGuards}
	pkSk := []string{env + "_" + tableActionLog}
	for _, name := range pkOnly {
		createTable(ctx, t, db, name, false)
	}
	for _, name := range pkSk {
		createTable(ctx, t, db, name, true)
	}
}

func createTable(ctx context.Context, t testingT, db *dynamodb.Client, name string, withSK bool) {
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

func strPtr(s string) *string { return &s }

// testingT is the minimal *testing.T surface these helpers need, kept as an
// unexported interface so this file (non-test code) never imports "testing".
type testingT interface{ Fatalf(string, ...any) }
