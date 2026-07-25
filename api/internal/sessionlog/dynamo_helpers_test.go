//go:build integration

package sessionlog

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

func newTestStore(t *testing.T) *Store {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	db := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
	env := "sessionlog_test"
	createTestTable(t, db, dynamo.TableName(env, tableSessions))
	createTestTable(t, db, dynamo.TableName(env, tableHands))
	return NewStore(db, env)
}

func createTestTable(t *testing.T, db *dynamodb.Client, name string) {
	t.Helper()
	attrs := []types.AttributeDefinition{
		{AttributeName: new("pk"), AttributeType: types.ScalarAttributeTypeS},
		{AttributeName: new("sk"), AttributeType: types.ScalarAttributeTypeS},
	}
	keys := []types.KeySchemaElement{
		{AttributeName: new("pk"), KeyType: types.KeyTypeHash},
		{AttributeName: new("sk"), KeyType: types.KeyTypeRange},
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

func new(s string) *string { return &s }
