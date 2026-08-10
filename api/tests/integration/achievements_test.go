//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/achievements"
)

// TestIncrementStreakResetAndAccumulate guards against achievements.Store's
// IncrementStreak sending an ExpressionAttributeValues entry that the chosen
// UpdateExpression branch never references (":one" on accumulate, ":value"
// on reset). DynamoDB rejects unused placeholders with a ValidationException
// on the real service -- a plain mock-backed unit test can't catch this,
// since it never validates the expression against the values. That
// ValidationException used to abort achievements.Service.RecordHand entirely,
// silently dropping every already-earned unlock for the hand (e.g. a
// straight win), not just the streak counter.
func TestIncrementStreakResetAndAccumulate(t *testing.T) {
	db := testDynamoClient(t)
	env := "achv_test"
	tableName := env + "_poker_achievement_progress"
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
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

	store := achievements.NewStore(db, env)
	playerID := uniqueTableID(t)

	if _, err := store.IncrementStreak(context.Background(), playerID, "sandbox", achievements.KeyFoldedStreak, true, 0); err != nil {
		t.Fatalf("reset: %v", err)
	}
	current, err := store.IncrementStreak(context.Background(), playerID, "sandbox", achievements.KeyFoldedStreak, false, 0)
	if err != nil {
		t.Fatalf("accumulate: %v", err)
	}
	if current != 1 {
		t.Fatalf("want streak 1 after one accumulate, got %d", current)
	}
}
