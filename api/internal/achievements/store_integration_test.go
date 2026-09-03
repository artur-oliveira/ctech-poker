//go:build integration

package achievements

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

// TestClaimHandCountersIsIdempotentAgainstDynamoDB is the real-store
// counterpart to TestClaimHandCountersOnlyFirstCallerWins/
// TestRecordHandGuardPreventsDoubleCountOnDuplicateInvocation in
// service_test.go: proves the conditional PutItem actually round-trips
// through DynamoDB Local as "first claim wins, every later claim for the
// same (table_id, hand_id) loses" — the fix for issue #66 (achievements +
// leaderboard counters double-counting on a Valkey blip).
func TestClaimHandCountersIsIdempotentAgainstDynamoDB(t *testing.T) {
	db := achievementsTestClient(t)
	env := fmt.Sprintf("achievements_test_%d", time.Now().UnixNano())
	name := env + "_" + tableProgress
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName:   aws.String(name),
		BillingMode: types.BillingModePayPerRequest,
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

	first, err := store.ClaimHandCounters(context.Background(), "table-1", "hand-1")
	if err != nil || !first {
		t.Fatalf("first claim: got (%v, %v), want (true, nil)", first, err)
	}
	for range 3 {
		retry, err := store.ClaimHandCounters(context.Background(), "table-1", "hand-1")
		if err != nil || retry {
			t.Fatalf("retried claim: got (%v, %v), want (false, nil)", retry, err)
		}
	}
	otherHand, err := store.ClaimHandCounters(context.Background(), "table-1", "hand-2")
	if err != nil || !otherHand {
		t.Fatalf("different hand claim: got (%v, %v), want (true, nil)", otherHand, err)
	}
	otherTable, err := store.ClaimHandCounters(context.Background(), "table-2", "hand-1")
	if err != nil || !otherTable {
		t.Fatalf("different table claim: got (%v, %v), want (true, nil)", otherTable, err)
	}
}

func achievementsTestClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}
