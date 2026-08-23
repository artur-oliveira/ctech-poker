//go:build integration

package highlights

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
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestRecordHand_OnlyOverwritesOnBiggerPot(t *testing.T) {
	db := highlightsTestClient(t)
	env := fmt.Sprintf("highlights_test_%d", time.Now().UnixNano())
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(env + "_" + tableHighlights), BillingMode: types.BillingModePayPerRequest,
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
	store := NewStore(db, env)
	ctx := context.Background()

	bigOutcome := hand.HandOutcome{Payouts: map[string]int64{"a": 500}}
	if err := store.RecordHand(ctx, "table", "hand-1", bigOutcome, nil); err != nil {
		t.Fatal(err)
	}
	smallOutcome := hand.HandOutcome{Payouts: map[string]int64{"a": 100}}
	if err := store.RecordHand(ctx, "table", "hand-2", smallOutcome, nil); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetToday(ctx, "table")
	if err != nil || got == nil || got.Pot != 500 || got.HandID != "hand-1" {
		t.Fatalf("expected smaller pot not to overwrite the recorded one, got=%+v err=%v", got, err)
	}

	biggerOutcome := hand.HandOutcome{Payouts: map[string]int64{"a": 900}}
	if err := store.RecordHand(ctx, "table", "hand-3", biggerOutcome, nil); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetToday(ctx, "table")
	if err != nil || got == nil || got.Pot != 900 || got.HandID != "hand-3" {
		t.Fatalf("expected bigger pot to overwrite the recorded one, got=%+v err=%v", got, err)
	}
}

func highlightsTestClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	return dynamodb.NewFromConfig(cfg, func(options *dynamodb.Options) {
		options.BaseEndpoint = aws.String("http://localhost:8555")
	})
}
