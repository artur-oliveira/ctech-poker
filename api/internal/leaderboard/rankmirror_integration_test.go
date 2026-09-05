//go:build integration

package leaderboard

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/valkey-io/valkey-go"
)

// TestRankMirrorMatchesCountPathForEveryPlayer is the whole safety net for
// issue #202: the O(log n) Valkey answer must be byte-identical to the
// DynamoDB COUNT answer it replaces — for every player on the board, ties
// included. The fixture deliberately gives most players the same hands_won so
// the player_id tiebreak is exercised rather than incidental.
func TestRankMirrorMatchesCountPathForEveryPlayer(t *testing.T) {
	ctx := context.Background()
	db := leaderboardTestClient(t)
	env := fmt.Sprintf("rankmirror_test_%d", time.Now().UnixNano())
	createHandsWonTable(t, db, env)

	counting := NewStore(db, env)
	const players = 60
	ids := make([]string, 0, players)
	for i := 0; i < players; i++ {
		id := fmt.Sprintf("p%03d", i)
		ids = append(ids, id)
		// Five distinct scores across 60 players: every score has ~12 rows
		// tied on it, so almost every rank depends on the player_id tiebreak.
		if err := counting.IncrementStats(ctx, id, "", "sandbox", 10, i%5); err != nil {
			t.Fatal(err)
		}
	}

	mirrored := NewStore(db, env).WithRankMirror(valkeyTestClient(t))
	for _, id := range ids {
		entry, err := counting.PlayerEntry(ctx, id, "sandbox")
		if err != nil || entry == nil {
			t.Fatalf("player entry %s: %v", id, err)
		}
		wantRank, wantTotal, err := counting.rankByCount(ctx, "sandbox", "hands_won", *entry)
		if err != nil {
			t.Fatal(err)
		}
		gotRank, gotTotal, ok := mirrored.rankFromMirror(ctx, "sandbox", "hands_won", *entry)
		if !ok {
			t.Fatalf("%s: mirror reported a miss after it should be materialized", id)
		}
		if gotRank != wantRank || gotTotal != wantTotal {
			t.Fatalf("%s: mirror says %d/%d, COUNT says %d/%d", id, gotRank, gotTotal, wantRank, wantTotal)
		}
	}

	// The first call above paid the one rebuild; the rest were served without
	// touching DynamoDB at all. Prove the board is a live key with a bounded
	// lifetime rather than something that grew unbounded per request.
	client := valkeyTestClient(t)
	size, err := client.Do(ctx, client.B().Zcard().Key(rankMirrorKey("sandbox", "hands_won")).Build()).ToInt64()
	if err != nil || size != players {
		t.Fatalf("expected a materialized board of %d members, got %d (%v)", players, size, err)
	}
	ttl, err := client.Do(ctx, client.B().Ttl().Key(rankMirrorKey("sandbox", "hands_won")).Build()).ToInt64()
	if err != nil || ttl <= 0 || ttl > int64(RankMirrorTTL.Seconds()) {
		t.Fatalf("board must carry the freshness-SLA TTL, got %d (%v)", ttl, err)
	}
}

// TestRankMirrorRebuildIsClaimedOnce pins the stampede guard: a cold board is
// rebuilt by one caller, and everyone else degrades to the COUNT path instead
// of each paying their own full partition read.
func TestRankMirrorRebuildIsClaimedOnce(t *testing.T) {
	ctx := context.Background()
	mirror := &rankMirror{client: valkeyTestClient(t), ttl: RankMirrorTTL}
	key := fmt.Sprintf("poker:leaderboard:rank:test:%d", time.Now().UnixNano())

	first, err := mirror.claimRebuild(ctx, key)
	if err != nil || !first {
		t.Fatalf("first rebuilder must win the claim, got %v (%v)", first, err)
	}
	second, err := mirror.claimRebuild(ctx, key)
	if err != nil || second {
		t.Fatalf("second rebuilder must lose the claim, got %v (%v)", second, err)
	}
	if err := mirror.publish(ctx, key, []boardMember{{playerID: "a", score: 1}}); err != nil {
		t.Fatal(err)
	}
	again, err := mirror.claimRebuild(ctx, key)
	if err != nil || !again {
		t.Fatalf("publishing must release the claim, got %v (%v)", again, err)
	}
}

func valkeyTestClient(t *testing.T) valkey.Client {
	t.Helper()
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{"127.0.0.1:6399"}, DisableCache: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func createHandsWonTable(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(env + "_" + tableStats), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi_hands_won_pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("hands_won"), AttributeType: types.ScalarAttributeTypeN},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{
			IndexName: aws.String(gsiHandsWon),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("gsi_hands_won_pk"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("hands_won"), KeyType: types.KeyTypeRange},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
}
