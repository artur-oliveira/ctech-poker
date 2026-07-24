//go:build integration

package leaderboard

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

func TestWinRateUsesMaterializedGSI(t *testing.T) {
	db := leaderboardTestClient(t)
	env := fmt.Sprintf("leaderboard_test_%d", time.Now().UnixNano())
	name := env + "_" + tableStats
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(name), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS}, {AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi_win_rate_pk"), AttributeType: types.ScalarAttributeTypeS}, {AttributeName: aws.String("win_rate_score"), AttributeType: types.ScalarAttributeTypeN},
		},
		KeySchema:              []types.KeySchemaElement{{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash}, {AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange}},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{IndexName: aws.String(gsiWinRate), KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("gsi_win_rate_pk"), KeyType: types.KeyTypeHash}, {AttributeName: aws.String("win_rate_score"), KeyType: types.KeyTypeRange}}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewServiceWithStore(NewStore(db, env))
	ctx := context.Background()
	names := map[string]string{"winner": "Winner Name"}
	if err := svc.RecordHand(ctx, hand.HandOutcome{Winners: []string{"winner"}, Participants: []string{"winner", "other"}}, names); err != nil {
		t.Fatal(err)
	}
	top, err := svc.Top(ctx, "win_rate", 10)
	if err != nil || len(top) != 2 || top[0].PlayerID != "winner" || top[0].WinRate != 1 {
		t.Fatalf("top=%+v err=%v", top, err)
	}
	if top[0].PlayerName != "Winner Name" {
		t.Fatalf("expected denormalized player_name persisted to DynamoDB, got %+v", top[0])
	}
}

func leaderboardTestClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatal(err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}
