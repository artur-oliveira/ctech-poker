//go:build integration

package botfunding

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

func TestFundingTransactionAgainstDynamoDB(t *testing.T) {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatal(err)
	}
	db := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
	env := fmt.Sprintf("botfunding_test_%d", time.Now().UnixNano())
	name := env + "_" + tableProgress
	_, err = db.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(name), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.DeleteTable(context.Background(), &dynamodb.DeleteTableInput{TableName: aws.String(name)})
	})

	store := NewStore(db, env)
	a, b := NewService(store, 100_000), NewService(store, 100_000)
	now := time.Unix(1_800_000_000, 0)
	a.now, b.now = func() time.Time { return now }, func() time.Time { return now }
	if allowed, err := a.Record(ctx, "p", "t", "h1", 60_000); err != nil || !allowed {
		t.Fatalf("first allowed=%v err=%v", allowed, err)
	}
	if _, _, err := b.Eligibility(ctx, "p"); err != nil {
		t.Fatal(err)
	}
	if allowed, err := a.Record(ctx, "p", "t", "h2", 40_000); err != nil || allowed {
		t.Fatalf("limit allowed=%v err=%v", allowed, err)
	}
	if allowed, err := b.Record(ctx, "p", "t", "h1", 60_000); err != nil || allowed {
		t.Fatalf("duplicate allowed=%v err=%v", allowed, err)
	}
	current, err := store.Load(ctx, "p")
	if err != nil || current.NetProfit != 100_000 {
		t.Fatalf("profit=%v err=%v", current, err)
	}

	now = now.Add(25 * time.Hour)
	if allowed, err := b.Record(ctx, "p", "t", "h3", 1); err != nil || !allowed {
		t.Fatalf("rollover allowed=%v err=%v", allowed, err)
	}
	if allowed, err := a.Record(ctx, "p", "t", "h1", 60_000); err != nil || !allowed {
		t.Fatalf("late retry allowed=%v err=%v", allowed, err)
	}
	current, err = store.Load(ctx, "p")
	if err != nil || current.NetProfit != 1 {
		t.Fatalf("new profit=%v err=%v", current, err)
	}
}
