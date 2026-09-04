//go:build integration

package matchup

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

func TestRecordHandIsIdempotentAndGetIsSymmetric(t *testing.T) {
	db := matchupTestClient(t)
	env := fmt.Sprintf("matchup_test_%d", time.Now().UnixNano())
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(env + "_" + tableMatchups), BillingMode: types.BillingModePayPerRequest,
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
	outcome := hand.HandOutcome{
		Winners:       []string{"z"},
		Participants:  []string{"a", "z"},
		Payouts:       map[string]int64{"z": 190},
		Contributions: map[string]int64{"a": 100, "z": 100},
	}
	for range 2 {
		if err := store.RecordHand(context.Background(), "sandbox", "table-1", "hand-1", outcome); err != nil {
			t.Fatal(err)
		}
	}
	fromA, err := store.Get(context.Background(), "sandbox", "a", "z")
	if err != nil || fromA.Stats.HandsTogether != 1 || fromA.Stats.HeadsUpHandsTogether != 1 ||
		fromA.Stats.WinsHigh != 1 || fromA.Stats.NetChangeLow != -100 || fromA.Stats.NetChangeHigh != 90 {
		t.Fatalf("fromA=%+v err=%v", fromA, err)
	}
	fromZ, err := store.Get(context.Background(), "sandbox", "z", "a")
	if err != nil || fromZ.IDLow != "a" || fromZ.IDHigh != "z" || fromZ.Stats.HandsTogether != 1 {
		t.Fatalf("fromZ=%+v err=%v", fromZ, err)
	}
	empty, err := store.Get(context.Background(), "sandbox", "a", "nobody")
	if err != nil || empty.Stats.HandsTogether != 0 {
		t.Fatalf("expected zeroed stats for an unseen pair, got %+v err=%v", empty, err)
	}
}

// TestAppliedHandsSetIsPrunedAndStillRejectsReplays runs a pair past
// appliedHandsCap shared hands so pruneApplied actually fires against
// DynamoDB, then replays the most recent hand: every hand must be counted
// exactly once, and the newest hands must still be inside the replay window
// the pruned set keeps (issue #201).
func TestAppliedHandsSetIsPrunedAndStillRejectsReplays(t *testing.T) {
	db := matchupTestClient(t)
	env := fmt.Sprintf("matchup_prune_%d", time.Now().UnixNano())
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(env + "_" + tableMatchups), BillingMode: types.BillingModePayPerRequest,
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
	outcome := hand.HandOutcome{Winners: []string{"z"}, Participants: []string{"a", "z"}}
	hands := appliedHandsCap + 4
	for i := range hands {
		handID := fmt.Sprintf("hand-%03d", i)
		for range 2 { // every hand delivered twice: the guard must absorb one
			if err := store.RecordHand(context.Background(), "sandbox", "table-1", handID, outcome); err != nil {
				t.Fatal(err)
			}
		}
	}
	// The last hand replayed once more, after the set has been pruned.
	if err := store.RecordHand(context.Background(), "sandbox", "table-1", fmt.Sprintf("hand-%03d", hands-1), outcome); err != nil {
		t.Fatal(err)
	}
	stats, err := store.Get(context.Background(), "sandbox", "a", "z")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Stats.HandsTogether != int64(hands) {
		t.Fatalf("handsTogether=%d want=%d (a replay was double-counted or a hand was lost)", stats.Stats.HandsTogether, hands)
	}
	item, err := db.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(env + "_" + tableMatchups),
		Key:       map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "pair#sandbox#a#z"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	set, ok := item.Item["applied_hands"].(*types.AttributeValueMemberSS)
	if !ok || len(set.Value) > appliedHandsCap {
		t.Fatalf("applied_hands not pruned: %+v", item.Item["applied_hands"])
	}
}

func matchupTestClient(t *testing.T) *dynamodb.Client {
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
