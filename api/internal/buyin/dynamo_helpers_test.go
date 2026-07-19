//go:build integration

package buyin

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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
