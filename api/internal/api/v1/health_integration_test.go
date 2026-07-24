//go:build integration

package v1

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestDynamoDBHealthDescribesExpectedTable(t *testing.T) {
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatal(err)
	}
	db := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
	table := fmt.Sprintf("health_test_%d", time.Now().UnixNano())
	_, err = db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(table), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS}},
		KeySchema:            []types.KeySchemaElement{{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash}},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := checkDynamoDB(context.Background(), db, table, time.Now().UTC().Format(time.RFC3339Nano))
	if entry.Status != statusPass || entry.ComponentName != componentDynamoDB || entry.ObservedUnit != unitMillisecond {
		t.Fatalf("entry=%+v", entry)
	}
}
