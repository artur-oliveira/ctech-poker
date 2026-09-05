//go:build integration

package integration

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/achievements"
)

// TestApplyHandProgressAggregatesAddsAndSets runs the aggregated per-hand
// write (issue #198) against the real service. A mock-backed unit test cannot
// catch what breaks here: DynamoDB rejects an UpdateExpression with an unused
// placeholder, an ADD on a nested path, or a name/value alias mismatch with a
// ValidationException — and that exception used to abort
// achievements.Service.RecordHand entirely, silently dropping every unlock
// the hand had already earned. It covers both branches of the write: the
// create (seed) path and the update path, with adds and sets in one call.
func TestApplyHandProgressAggregatesAddsAndSets(t *testing.T) {
	store := achievementsTestStore(t)
	playerID := uniqueTableID(t)
	ctx := context.Background()

	// First hand for this player: no aggregate row yet, so this exercises the
	// seed path, with a SET (streak reset) and several ADDs in one write.
	previous, current, err := store.ApplyHandProgress(ctx, playerID, "sandbox",
		map[string]int{achievements.KeyHandsPlayed: 1, achievements.KeyWins: 1},
		map[string]int{achievements.KeyFoldedStreak: 0})
	if err != nil {
		t.Fatalf("seed write: %v", err)
	}
	if previous[achievements.KeyWins] != 0 || current[achievements.KeyWins] != 1 {
		t.Fatalf("seed write returned previous=%v current=%v", previous, current)
	}

	// Second hand: the aggregate exists, so this takes the update path, and
	// the streak accumulates instead of resetting.
	previous, current, err = store.ApplyHandProgress(ctx, playerID, "sandbox",
		map[string]int{achievements.KeyHandsPlayed: 1, achievements.KeyFoldedStreak: 1}, nil)
	if err != nil {
		t.Fatalf("update write: %v", err)
	}
	if previous[achievements.KeyHandsPlayed] != 1 || current[achievements.KeyHandsPlayed] != 2 {
		t.Fatalf("update write returned previous=%v current=%v", previous, current)
	}
	if current[achievements.KeyFoldedStreak] != 1 {
		t.Fatalf("folded streak = %d after reset+accumulate, want 1", current[achievements.KeyFoldedStreak])
	}

	if err := store.StampTierUnlocks(ctx, playerID, "sandbox",
		[]string{achievements.KeyWins, achievements.KeyHandsPlayed}); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	rows, _, err := store.ListAchievements(ctx, playerID, "sandbox", 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	counts, stamped := map[string]int{}, map[string]string{}
	for _, row := range rows {
		counts[row.Key], stamped[row.Key] = row.Count, row.UnlockedAt
	}
	if counts[achievements.KeyHandsPlayed] != 2 || counts[achievements.KeyWins] != 1 {
		t.Fatalf("read back %+v, want hands_played=2 wins=1", counts)
	}
	if stamped[achievements.KeyWins] == "" || stamped[achievements.KeyHandsPlayed] == "" {
		t.Fatalf("unlock stamps missing from the aggregate: %+v", stamped)
	}
	// The other currency mode must be untouched by all of the above.
	realRows, _, err := store.ListAchievements(ctx, playerID, "real", 100, nil)
	if err != nil || len(realRows) != 0 {
		t.Fatalf("real-money rows=%+v err=%v, want none", realRows, err)
	}
}

// TestApplyHandProgressAbsorbsPreExistingPerKeyRows covers the migration
// path: a player whose counters were written before #198 keeps their totals
// and their unlock timestamps, and their next hand's tier crossing is
// computed against the totals they actually had — never against zero, which
// would re-unlock tiers they passed long ago.
func TestApplyHandProgressAbsorbsPreExistingPerKeyRows(t *testing.T) {
	store := achievementsTestStore(t)
	playerID := uniqueTableID(t)
	ctx := context.Background()

	putLegacyRow(t, playerID, "sandbox#"+achievements.KeyHandsPlayed, 41, "2026-01-01T00:00:00Z")
	putLegacyRow(t, playerID, "sandbox#"+achievements.KeyWins, 7, "")

	previous, current, err := store.ApplyHandProgress(ctx, playerID, "sandbox",
		map[string]int{achievements.KeyHandsPlayed: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if previous[achievements.KeyHandsPlayed] != 41 || current[achievements.KeyHandsPlayed] != 42 {
		t.Fatalf("legacy totals lost: previous=%v current=%v", previous, current)
	}
	rows, _, err := store.ListAchievements(ctx, playerID, "sandbox", 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	counts, stamped := map[string]int{}, map[string]string{}
	for _, row := range rows {
		counts[row.Key], stamped[row.Key] = row.Count, row.UnlockedAt
	}
	if counts[achievements.KeyHandsPlayed] != 42 || counts[achievements.KeyWins] != 7 {
		t.Fatalf("read back %+v, want hands_played=42 wins=7", counts)
	}
	if stamped[achievements.KeyHandsPlayed] != "2026-01-01T00:00:00Z" {
		t.Fatalf("legacy unlock stamp lost: %+v", stamped)
	}
}

func putLegacyRow(t *testing.T, playerID, sk string, counter int, unlockedAt string) {
	t.Helper()
	item := map[string]types.AttributeValue{
		"pk":      &types.AttributeValueMemberS{Value: playerID},
		"sk":      &types.AttributeValueMemberS{Value: sk},
		"counter": &types.AttributeValueMemberN{Value: strconv.Itoa(counter)},
	}
	if unlockedAt != "" {
		item["unlocked_at"] = &types.AttributeValueMemberS{Value: unlockedAt}
	}
	if _, err := testDynamoClient(t).PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(achievementsTestTable), Item: item,
	}); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
}

const (
	achievementsTestEnv   = "achv_test"
	achievementsTestTable = achievementsTestEnv + "_poker_achievement_progress"
)

func achievementsTestStore(t *testing.T) *achievements.Store {
	t.Helper()
	db := testDynamoClient(t)
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(achievementsTestTable),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	var inUse *types.ResourceInUseException
	if err != nil && !errors.As(err, &inUse) {
		t.Fatalf("create table: %v", err)
	}
	return achievements.NewStore(db, achievementsTestEnv)
}
