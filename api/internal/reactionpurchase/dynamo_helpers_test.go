//go:build integration

package reactionpurchase

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

const testEnv = "reactionpurchase_test"

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
	createTestTable(t, db, dynamo.TableName(testEnv, tableEntitlements))
	return NewEntitlementStore(db, testEnv)
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db := testDynamoClient(t)
	createTestTable(t, db, dynamo.TableName(testEnv, tablePurchases))
	return NewStore(db, testEnv)
}
