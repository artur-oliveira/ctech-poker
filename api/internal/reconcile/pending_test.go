//go:build integration

package reconcile

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

func testClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://localhost:8555")
	})
}

func mustCreateTestTable(ctx context.Context, t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	tableName := dynamo.TableName(env, tablePending)
	_, _ = db.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(tableName)})
	_, err := db.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi_status"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
		},
		// Mirrors cdk/lib/dynamodb-stack.ts's pendingCashouts GSI: ListUnresolved
		// (pending.go) queries this index instead of scanning the whole table.
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{
			IndexName:  aws.String(pendingGSI),
			KeySchema:  []types.KeySchemaElement{{AttributeName: aws.String("gsi_status"), KeyType: types.KeyTypeHash}},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		}},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
}

func TestRecordThenListUnresolvedThenMarkResolved(t *testing.T) {
	db := testClient(t)
	ctx := context.Background()
	env := "test"
	mustCreateTestTable(ctx, t, db, env)

	store := NewPendingStore(db, env)
	p := PendingCashout{
		ID:             "co-1",
		PlayerID:       "user-1",
		Amount:         400,
		CurrencyMode:   "real",
		IdempotencyKey: "room-1#user-1#cashout",
	}
	if err := store.Record(ctx, p); err != nil {
		t.Fatalf("record: %v", err)
	}
	for _, id := range []string{"co-2", "co-3"} {
		if err := store.Record(ctx, PendingCashout{ID: id, PlayerID: "user-2", Amount: 100}); err != nil {
			t.Fatalf("record %s: %v", id, err)
		}
	}
	// Force DynamoDB pagination so the test guards LastEvaluatedKey handling
	// without needing a 1 MiB fixture.
	store.scanPageLimit = 1

	unresolved, err := store.ListUnresolved(ctx, 0)
	if err != nil || len(unresolved) != 3 {
		t.Fatalf("expected all 3 unresolved entries across pages, got %+v, err=%v", unresolved, err)
	}

	if err := store.MarkResolved(ctx, "co-1"); err != nil {
		t.Fatalf("mark resolved: %v", err)
	}

	unresolved, err = store.ListUnresolved(ctx, 0)
	if err != nil || len(unresolved) != 2 {
		t.Fatalf("expected only the resolved entry removed, got %+v", unresolved)
	}
}

func TestRecordFailedAttemptQuarantinesAfterMaxAttempts(t *testing.T) {
	db := testClient(t)
	ctx := context.Background()
	env := "test"
	mustCreateTestTable(ctx, t, db, env)

	store := NewPendingStore(db, env)
	if err := store.Record(ctx, PendingCashout{ID: "co-1", PlayerID: "user-1", Amount: 400, CurrencyMode: "real"}); err != nil {
		t.Fatalf("record: %v", err)
	}

	cause := errors.New("wallet unavailable")
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		unresolved, err := store.ListUnresolved(ctx, 0)
		if err != nil {
			t.Fatalf("list before attempt %d: %v", attempt, err)
		}
		if len(unresolved) != 1 {
			t.Fatalf("attempt %d: expected entry still in sweep, got %+v", attempt, unresolved)
		}

		n, quarantined, err := store.RecordFailedAttempt(ctx, unresolved[0], cause)
		if err != nil {
			t.Fatalf("record failed attempt %d: %v", attempt, err)
		}
		if n != attempt {
			t.Fatalf("expected attempt count %d, got %d", attempt, n)
		}
		if want := attempt >= MaxAttempts; quarantined != want {
			t.Fatalf("attempt %d: quarantined=%v, want %v", attempt, quarantined, want)
		}
	}

	// Quarantined entry has left the normal sweep.
	unresolved, err := store.ListUnresolved(ctx, 0)
	if err != nil {
		t.Fatalf("final list: %v", err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("expected quarantined entry out of sweep, got %+v", unresolved)
	}
}
