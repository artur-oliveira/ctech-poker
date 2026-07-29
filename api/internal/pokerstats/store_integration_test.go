//go:build integration

package pokerstats

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

func TestRecordHandIsIdempotent(t *testing.T) {
	db := pokerStatsTestClient(t)
	env := fmt.Sprintf("pokerstats_test_%d", time.Now().UnixNano())
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(env + "_" + tableStats), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db, env)
	metrics := []HandMetric{
		{PlayerID: "a", VPIP: true, PFR: true},
		{PlayerID: "b", VPIP: true, ThreeBetChance: true},
	}
	for range 2 {
		if err := store.RecordHand(context.Background(), "sandbox", "table", "hand", metrics); err != nil {
			t.Fatal(err)
		}
	}
	a, err := store.Get(context.Background(), "a", "sandbox")
	if err != nil || a.Hands != 1 || a.VPIPHands != 1 || a.PFRHands != 1 || a.VPIPRate != 1 {
		t.Fatalf("a=%+v err=%v", a, err)
	}
	b, err := store.Get(context.Background(), "b", "sandbox")
	if err != nil || b.Hands != 1 || b.ThreeBetChances != 1 || b.ThreeBetRate != 0 {
		t.Fatalf("b=%+v err=%v", b, err)
	}
}

func pokerStatsTestClient(t *testing.T) *dynamodb.Client {
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
