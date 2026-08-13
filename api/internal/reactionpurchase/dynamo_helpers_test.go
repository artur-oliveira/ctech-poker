//go:build integration

package reactionpurchase

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

// testEnv is unique per call (mirrors dailyreward's store_integration_test.go)
// so a fresh table backs every test — DynamoDB Local persists in-memory
// across test-binary invocations within the same container uptime, and this
// package's tests assert on the absence of rows (no entitlement yet, no
// purchase yet), which a fixed/shared table name would falsify on a rerun.
func testEnv() string {
	return fmt.Sprintf("reactionpurchase_test_%d", time.Now().UnixNano())
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

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db := testDynamoClient(t)
	env := testEnv()
	createTestTable(t, db, dynamo.TableName(env, tablePurchases))
	return NewStore(db, env)
}
