//go:build integration

package cosmeticpurchase

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
)

// testEnv is unique per call (mirrors reactionpurchase's own helper) so a
// fresh table backs every test — DynamoDB Local persists in-memory across
// test-binary invocations within the same container uptime, and this
// package's tests assert on the absence of rows (no entitlement yet, no
// purchase yet), which a fixed/shared table name would falsify on a rerun.
func testEnv() string {
	return fmt.Sprintf("cosmeticpurchase_test_%d", time.Now().UnixNano())
}

func testDynamoClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}

func createTestTable(t *testing.T, db *dynamodb.Client, name string) {
	t.Helper()
	attrs := []types.AttributeDefinition{
		{AttributeName: strPtr("pk"), AttributeType: types.ScalarAttributeTypeS},
		{AttributeName: strPtr("sk"), AttributeType: types.ScalarAttributeTypeS},
	}
	keys := []types.KeySchemaElement{
		{AttributeName: strPtr("pk"), KeyType: types.KeyTypeHash},
		{AttributeName: strPtr("sk"), KeyType: types.KeyTypeRange},
	}
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: strPtr(name), AttributeDefinitions: attrs, KeySchema: keys, BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		var inUse *types.ResourceInUseException
		if !errors.As(err, &inUse) {
			t.Fatalf("create table %s: %v", name, err)
		}
	}
}

func strPtr(s string) *string { return &s }

func newTestEntitlementStore(t *testing.T) *EntitlementStore {
	t.Helper()
	db := testDynamoClient(t)
	env := testEnv()
	createTestTable(t, db, dynamo.TableName(env, tableEntitlements))
	return NewEntitlementStore(db, env)
}

// newTestStore builds the purchase table with gsi_player_kind, the index
// Store.List pages history off (issue #219).
func newTestStore(t *testing.T) *Store {
	t.Helper()
	db := testDynamoClient(t)
	env := testEnv()
	name := dynamo.TableName(env, tablePurchases)
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: strPtr(name), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: strPtr("pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: strPtr("sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: strPtr("kind"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: strPtr("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: strPtr("sk"), KeyType: types.KeyTypeRange},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{
			IndexName: strPtr(gsiPlayerKind),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: strPtr("pk"), KeyType: types.KeyTypeHash},
				{AttributeName: strPtr("kind"), KeyType: types.KeyTypeRange},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		}},
	})
	if err != nil {
		var inUse *types.ResourceInUseException
		if !errors.As(err, &inUse) {
			t.Fatalf("create table %s: %v", name, err)
		}
	}
	return NewStore(db, env)
}
