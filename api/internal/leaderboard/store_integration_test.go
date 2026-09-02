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
	names := map[string]string{"grinder": "Grinder Name"}

	// One hand each: both players are sub-floor, so the sparse gsi_win_rate_pk
	// key is never written and the win_rate board stays empty (issue #63).
	if err := svc.RecordHand(ctx, "sandbox", hand.HandOutcome{Winners: []string{"flash"}, Participants: []string{"flash", "grinder"}}, nil); err != nil {
		t.Fatal(err)
	}
	if top, _, err := svc.Top(ctx, "sandbox", "win_rate", 10, nil); err != nil || len(top) != 0 {
		t.Fatalf("sub-floor players must not appear on win_rate board: top=%+v err=%v", top, err)
	}

	// Grind "grinder" past MinHandsForWinRateRank; "flash" stays at 1 hand 100%.
	for i := 0; i < MinHandsForWinRateRank+49; i++ {
		if err := svc.RecordHand(ctx, "sandbox", hand.HandOutcome{Winners: []string{"grinder"}, Participants: []string{"grinder", "other"}}, names); err != nil {
			t.Fatal(err)
		}
	}
	top, _, err := svc.Top(ctx, "sandbox", "win_rate", 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if top[0].PlayerID != "grinder" || top[0].WinRate <= 0.9 {
		t.Fatalf("the 150-hand grinder should top the board, got top=%+v", top)
	}
	for _, e := range top {
		if e.PlayerID == "flash" {
			t.Fatalf("1-hand 100%% player must be excluded, got top=%+v", top)
		}
	}
	if top[0].HandsPlayed != MinHandsForWinRateRank+50 {
		t.Fatalf("expected hand count carried on the row, got %+v", top[0])
	}
	if top[0].PlayerName != "Grinder Name" {
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
