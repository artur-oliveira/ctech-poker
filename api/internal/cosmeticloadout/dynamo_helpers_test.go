//go:build integration

package cosmeticloadout

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

// testEnv is unique per call, same reasoning as cosmeticpurchase's own
// helper: a fresh table backs every test since some assertions depend on an
// empty starting partition (e.g. the per-player loadout cap).
func testEnv() string {
	return fmt.Sprintf("cosmeticloadout_test_%d", time.Now().UnixNano())
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

func strPtr(s string) *string { return &s }

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db := testDynamoClient(t)
	env := testEnv()
	name := dynamo.TableName(env, tableLoadouts)
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: strPtr(name), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: strPtr("pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: strPtr("sk"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: strPtr("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: strPtr("sk"), KeyType: types.KeyTypeRange},
		},
	})
	if err != nil {
		var inUse *types.ResourceInUseException
		if !errors.As(err, &inUse) {
			t.Fatalf("create table %s: %v", name, err)
		}
	}
	return NewStore(db, env)
}
