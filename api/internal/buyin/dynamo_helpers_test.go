//go:build integration

package buyin

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
	"gopkg.aoctech.app/poker/api/internal/entitlement"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

// testClient / mustCreateTestTables mirror api/internal/table's own copies —
// this repo keeps a per-package copy of these DynamoDB Local helpers rather
// than a shared test-helpers package (same as roomstore/tablestore).
func testClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}

func mustCreateTestTables(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	pkOnly := []string{env + "_poker_table_state", env + "_poker_action_guards"}
	pkSk := []string{env + "_poker_action_log"}
	for _, name := range pkOnly {
		createTestTable(t, db, name, false)
	}
	for _, name := range pkSk {
		createTestTable(t, db, name, true)
	}
}

func createTestTable(t *testing.T, db *dynamodb.Client, name string, withSK bool, indexes ...types.GlobalSecondaryIndex) {
	t.Helper()
	attrs := []types.AttributeDefinition{{AttributeName: new("pk"), AttributeType: types.ScalarAttributeTypeS}}
	keys := []types.KeySchemaElement{{AttributeName: new("pk"), KeyType: types.KeyTypeHash}}
	if withSK {
		attrs = append(attrs, types.AttributeDefinition{AttributeName: new("sk"), AttributeType: types.ScalarAttributeTypeS})
		keys = append(keys, types.KeySchemaElement{AttributeName: new("sk"), KeyType: types.KeyTypeRange})
	}
	for _, idx := range indexes {
		for _, key := range idx.KeySchema {
			if *key.AttributeName != "pk" && *key.AttributeName != "sk" {
				attrs = append(attrs, types.AttributeDefinition{AttributeName: key.AttributeName, AttributeType: types.ScalarAttributeTypeS})
			}
		}
	}
	tableName := name
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{TableName: &tableName, AttributeDefinitions: attrs, KeySchema: keys, GlobalSecondaryIndexes: indexes, BillingMode: types.BillingModePayPerRequest})
	if err != nil {
		var inUse *types.ResourceInUseException
		if !errors.As(err, &inUse) {
			t.Fatalf("create table %s: %v", name, err)
		}
	}
}

func new(s string) *string { return &s }

func testSessionStore(t *testing.T) *sessionlog.Store {
	t.Helper()
	db := testClient(t)
	env := fmt.Sprintf("buyin_sessions_test_%d", time.Now().UnixNano())
	createTestTable(t, db, env+"_poker_player_sessions", true, sessionTestIndexes()...)
	createTestTable(t, db, env+"_poker_player_hands", true)
	return sessionlog.NewStore(db, env)
}

// sessionTestIndexes mirrors the poker_player_sessions indexes in
// cdk/lib/dynamodb-stack.ts. sessionlog's own helper keeps a copy too — this
// repo duplicates DynamoDB Local helpers per package rather than sharing them.
func sessionTestIndexes() []types.GlobalSecondaryIndex {
	return []types.GlobalSecondaryIndex{
		{
			IndexName: new("gsi_open_table"),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: new("pk"), KeyType: types.KeyTypeHash},
				{AttributeName: new("open_table_id"), KeyType: types.KeyTypeRange},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		},
		{
			IndexName: new("gsi_player_table"),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: new("pk"), KeyType: types.KeyTypeHash},
				{AttributeName: new("table_id"), KeyType: types.KeyTypeRange},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeKeysOnly},
		},
	}
}

func testPendingStore(t *testing.T) *reconcile.PendingStore {
	t.Helper()
	db := testClient(t)
	env := fmt.Sprintf("buyin_pending_test_%d", time.Now().UnixNano())
	createTestTable(t, db, env+"_poker_pending_cashouts", true)
	return reconcile.NewPendingStore(db, env)
}

func testEntitlementStore(t *testing.T) *entitlement.Store {
	t.Helper()
	db := testClient(t)
	env := fmt.Sprintf("buyin_entitlement_test_%d", time.Now().UnixNano())
	createTestTable(t, db, env+"_poker_table_entitlements", true)
	return entitlement.NewStore(db, env)
}
