//go:build integration

package reconcile

import (
	"context"
	"testing"
	"time"

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
		o.BaseEndpoint = aws.String("http://localhost:8000")
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
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
		},
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

	unresolved, err := store.ListUnresolved(ctx, 0)
	if err != nil || len(unresolved) != 1 || unresolved[0].ID != "co-1" {
		t.Fatalf("expected 1 unresolved entry, got %+v, err=%v", unresolved, err)
	}

	if err := store.MarkResolved(ctx, "co-1"); err != nil {
		t.Fatalf("mark resolved: %v", err)
	}

	unresolved, err = store.ListUnresolved(ctx, 0)
	if err != nil || len(unresolved) != 0 {
		t.Fatalf("expected 0 unresolved entries after MarkResolved, got %+v", unresolved)
	}
}
