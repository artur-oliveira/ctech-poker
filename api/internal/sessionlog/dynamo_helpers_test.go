//go:build integration

package sessionlog

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	smithymiddleware "github.com/aws/smithy-go/middleware"
	"gopkg.aoctech.app/api-commons/dynamo"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, _ := newCountingTestStore(t)
	return store
}

// newCountingTestStore returns a store plus a counter of the DynamoDB Query
// calls it issues, so a test can pin the number of round trips a lookup costs
// and not just its result.
func newCountingTestStore(t *testing.T) (*Store, *atomic.Int64) {
	t.Helper()
	var queries atomic.Int64
	countQueries := func(stack *smithymiddleware.Stack) error {
		return stack.Initialize.Add(smithymiddleware.InitializeMiddlewareFunc("countQueries",
			func(ctx context.Context, in smithymiddleware.InitializeInput, next smithymiddleware.InitializeHandler) (smithymiddleware.InitializeOutput, smithymiddleware.Metadata, error) {
				if middleware.GetOperationName(ctx) == "Query" {
					queries.Add(1)
				}
				return next.HandleInitialize(ctx, in)
			}), smithymiddleware.After)
	}
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
		config.WithAPIOptions([]func(*smithymiddleware.Stack) error{countQueries}))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	db := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
	env := "sessionlog_test"
	createTestTable(t, db, dynamo.TableName(env, tableSessions), sessionTestIndexes())
	createTestTable(t, db, dynamo.TableName(env, tableHands), nil)
	return NewStore(db, env), &queries
}

// sessionTestIndexes mirrors cdk/lib/dynamodb-stack.ts's poker_player_sessions
// indexes: the sparse open-session index (the per-table existence index ships
// in a later deploy — DynamoDB creates one GSI per stack update).
func sessionTestIndexes() []types.GlobalSecondaryIndex {
	return []types.GlobalSecondaryIndex{
		{
			IndexName: new(sessionsGsiOpenTable),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: new("pk"), KeyType: types.KeyTypeHash},
				{AttributeName: new(fieldOpenTableID), KeyType: types.KeyTypeRange},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		},
	}
}

func createTestTable(t *testing.T, db *dynamodb.Client, name string, indexes []types.GlobalSecondaryIndex) {
	t.Helper()
	attrs := []types.AttributeDefinition{
		{AttributeName: new("pk"), AttributeType: types.ScalarAttributeTypeS},
		{AttributeName: new("sk"), AttributeType: types.ScalarAttributeTypeS},
	}
	for _, idx := range indexes {
		for _, key := range idx.KeySchema {
			if *key.AttributeName != "pk" && *key.AttributeName != "sk" {
				attrs = append(attrs, types.AttributeDefinition{AttributeName: key.AttributeName, AttributeType: types.ScalarAttributeTypeS})
			}
		}
	}
	keys := []types.KeySchemaElement{
		{AttributeName: new("pk"), KeyType: types.KeyTypeHash},
		{AttributeName: new("sk"), KeyType: types.KeyTypeRange},
	}
	tableName := name
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: &tableName, AttributeDefinitions: attrs, KeySchema: keys,
		GlobalSecondaryIndexes: indexes, BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		var inUse *types.ResourceInUseException
		if !errors.As(err, &inUse) {
			t.Fatalf("create table %s: %v", name, err)
		}
	}
}

func new(s string) *string { return &s }
