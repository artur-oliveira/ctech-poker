//go:build integration

package recentplayers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

// TestDynamoRecordHandIsIdempotentAndListCoalescesOpponents covers issue
// #199's model end to end: one row per participant per hand, a replayed hand
// rewriting the same rows instead of counting twice, and List deriving
// hands_together / last_played_at ordering from those rows.
func TestDynamoRecordHandIsIdempotentAndListCoalescesOpponents(t *testing.T) {
	store := newRecentTestStore(t)
	playedAt := time.Now().UTC().Truncate(time.Millisecond)
	first := HandCompletion{TableID: "table", HandID: "hand-1", Players: []string{"a", "b", "c"}, PlayedAt: playedAt}
	for range 2 { // the same hand delivered twice must not double-count
		if err := store.RecordHand(context.Background(), first); err != nil {
			t.Fatal(err)
		}
	}
	// A later hand against b only: b must sort ahead of c, and count 2.
	second := HandCompletion{TableID: "table", HandID: "hand-2", Players: []string{"a", "b"}, PlayedAt: playedAt.Add(time.Minute)}
	if err := store.RecordHand(context.Background(), second); err != nil {
		t.Fatal(err)
	}

	page, err := store.List(context.Background(), "a", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Players) != 2 || page.Players[0].OpponentPlayerID != "b" || page.Players[1].OpponentPlayerID != "c" {
		t.Fatalf("opponents not ordered by recency: %+v", page.Players)
	}
	if page.Players[0].HandsTogether != 2 || page.Players[1].HandsTogether != 1 {
		t.Fatalf("hands_together=%d/%d want 2/1 (a replay was counted twice or a hand was lost)",
			page.Players[0].HandsTogether, page.Players[1].HandsTogether)
	}
	if page.Players[0].LastPlayedAt != second.PlayedAt.UnixMilli() {
		t.Fatalf("last_played_at=%d want=%d", page.Players[0].LastPlayedAt, second.PlayedAt.UnixMilli())
	}
	// The relation is symmetric: c's own list sees a and b from the same rows.
	fromC, err := store.List(context.Background(), "c", nil, 50)
	if err != nil || len(fromC.Players) != 2 {
		t.Fatalf("fromC=%+v err=%v", fromC.Players, err)
	}
}

// TestListPaginatesByOffsetWithoutRepeats walks a full ring's worth of
// opponents one page at a time — the coalesced cursor must cover every
// opponent exactly once.
func TestListPaginatesByOffsetWithoutRepeats(t *testing.T) {
	store := newRecentTestStore(t)
	players := make([]string, maxPlayersPerHand)
	for i := range players {
		players[i] = fmt.Sprintf("p%d", i)
	}
	if err := store.RecordHand(context.Background(), HandCompletion{
		TableID: "table", HandID: "hand-1", Players: players, PlayedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	var startKey map[string]types.AttributeValue
	for range maxPlayersPerHand {
		page, err := store.List(context.Background(), "p0", startKey, 3)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Players {
			seen[item.OpponentPlayerID]++
		}
		if page.LastKey == nil {
			break
		}
		startKey = page.LastKey
	}
	if len(seen) != maxPlayersPerHand-1 {
		t.Fatalf("paged %d distinct opponents, want %d: %+v", len(seen), maxPlayersPerHand-1, seen)
	}
	for id, times := range seen {
		if times != 1 {
			t.Fatalf("opponent %s returned %d times across pages", id, times)
		}
	}
}

func newRecentTestStore(t *testing.T) *DynamoStore {
	t.Helper()
	db := recentTestClient(t)
	env := fmt.Sprintf("recent_test_%d", time.Now().UnixNano())
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(dynamo.TableName(env, tableRecentPlayers)), BillingMode: types.BillingModePayPerRequest,
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
	return NewStore(db, env)
}

func recentTestClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion("us-east-1"), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatal(err)
	}
	return dynamodb.NewFromConfig(cfg, func(options *dynamodb.Options) { options.BaseEndpoint = aws.String("http://localhost:8555") })
}
