//go:build integration

package dailyreward

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

func TestStoreKeepsPendingPrizeAndCompletes(t *testing.T) {
	db := rouletteTestClient(t)
	env := fmt.Sprintf("roulette_test_%d", time.Now().UnixNano())
	name := env + "_" + tableSpins
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(name), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS}, {AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS}},
		KeySchema:            []types.KeySchemaElement{{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash}, {AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange}},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db, env)
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, brt)
	first, err := store.Claim(context.Background(), "p1", cooldownKey(now), 500, now)
	if err != nil || first.Amount != 500 || first.Status != StatusPending {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	retry, err := store.Claim(context.Background(), "p1", cooldownKey(now), 1000, now)
	if err != nil || retry.Amount != 500 || retry.Status != StatusPending {
		t.Fatalf("retry=%+v err=%v", retry, err)
	}
	if err := store.Complete(context.Background(), "p1", cooldownKey(now), now); err != nil {
		t.Fatal(err)
	}
	completed, err := store.Claim(context.Background(), "p1", cooldownKey(now), 100, now)
	if err != nil || completed.Amount != 500 || completed.Status != StatusCompleted {
		t.Fatalf("completed=%+v err=%v", completed, err)
	}
}

func rouletteTestClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatal(err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}
