package tablestore

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
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
func (s *Store) SaveSnapshot(ctx context.Context, tableID, handID string, seq int, state hand.Snapshot) error {
	item, err := dynamo.Encode(struct {
		PK     string        `dynamodbav:"pk"`
		SK     string        `dynamodbav:"sk"`
		HandID string        `dynamodbav:"hand_id"`
		Seq    int           `dynamodbav:"seq"`
		State  hand.Snapshot `dynamodbav:"state"`
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
// production tables are provisioned by CDK (Task 11), never by app code.
func mustCreateTestTables(ctx context.Context, t testingT, db *dynamodb.Client, env string) {
	for _, name := range []string{env + "_" + tableSnapshots, env + "_" + tableActionLog} {
		tableName := name
		_, err := db.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName: &tableName,
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
			if !errors.As(err, &inUse) {
				t.Fatalf("create table %s: %v", name, err)
			}
		}
	}
}

func strPtr(s string) *string { return &s }

// testingT is the minimal *testing.T surface mustCreateTestTables needs,
// kept as an unexported interface so this file (non-test code) never
// imports the "testing" package.
type testingT interface{ Fatalf(string, ...any) }
