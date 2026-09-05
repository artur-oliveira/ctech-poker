//go:build integration

package social

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go/middleware"
)

// TestUnreadCountIsOneQueryRegardlessOfBacklog is issue #208's read budget as
// a call count: the badge feeds off this on every inbox notification, and it
// used to page the whole unread partition. The backlog here is deliberately
// bigger than one page's worth of saturation so a regression to the old
// pagination loop shows up as extra calls, not just a slower test.
func TestUnreadCountIsOneQueryRegardlessOfBacklog(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int64
	db := countingSocialTestClient(t, &calls)
	env := fmt.Sprintf("social_budget_test_%d", time.Now().UnixNano())
	eventsTable := env + "_" + tableSocialEvents
	createSocialEventTestTable(t, db, eventsTable)
	t.Cleanup(func() {
		_, _ = db.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(eventsTable)})
	})

	store := NewEventStore(db, env)
	const backlog = MaxUnreadCount + 40
	ids := make([]string, 0, backlog)
	for i := 0; i < backlog; i++ {
		created, err := store.Create(ctx, Event{
			RecipientPlayerID: "recipient", ActorPlayerID: "actor", Type: EventFriendRequest,
		}, fmt.Sprintf("event-%03d", i))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, created.EventID)
	}

	calls.Store(0)
	count, err := store.UnreadCount(ctx, "recipient")
	if err != nil {
		t.Fatal(err)
	}
	if count != MaxUnreadCount {
		t.Fatalf("unread count must saturate at %d, got %d", MaxUnreadCount, count)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("a saturated unread count must cost exactly one query, spent %d", n)
	}

	// MarkRead's write count is irreducible (one conditional update per
	// event); what must hold is that it clears every event it was handed, so
	// the badge actually goes to zero after one inbox open.
	if err := store.MarkRead(ctx, "recipient", ids[:maxMarkReadBatch]); err != nil {
		t.Fatal(err)
	}
	count, err = store.UnreadCount(ctx, "recipient")
	if err != nil {
		t.Fatal(err)
	}
	if count != backlog-maxMarkReadBatch {
		t.Fatalf("expected %d unread left, got %d", backlog-maxMarkReadBatch, count)
	}

	// A batch containing an event that no longer exists must still clear the
	// rest — the reason this cannot be one TransactWriteItems.
	if err := store.MarkRead(ctx, "recipient", append([]string{"gone"}, ids[maxMarkReadBatch:]...)); err != nil {
		t.Fatalf("a missing event must not fail the batch: %v", err)
	}
	if count, err = store.UnreadCount(ctx, "recipient"); err != nil || count != 0 {
		t.Fatalf("every event should be read, got %d (%v)", count, err)
	}
}

// TestEdgeCountStopsAtTheCap pins the other half of issue #208: Count is only
// ever compared against a limit, so it must stop there instead of paging the
// player's whole edge partition.
func TestEdgeCountStopsAtTheCap(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int64
	db := countingSocialTestClient(t, &calls)
	env := fmt.Sprintf("social_count_test_%d", time.Now().UnixNano())
	edgesTable := env + "_" + tableSocialEdges
	createSocialEdgeTestTable(t, db, edgesTable)
	t.Cleanup(func() {
		_, _ = db.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(edgesTable)})
	})

	store := NewStore(db, env)
	const friends = 40
	for i := 0; i < friends; i++ {
		item, err := dynamoEncodeEdge(Edge{
			OwnerPlayerID: "owner", OtherPlayerID: fmt.Sprintf("friend-%03d", i),
			Relationship: RelationshipFriend, Version: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(edgesTable), Item: item}); err != nil {
			t.Fatal(err)
		}
	}

	calls.Store(0)
	count, err := store.Count(ctx, "owner", RelationshipFriend, 10)
	if err != nil {
		t.Fatal(err)
	}
	if count != 10 {
		t.Fatalf("count must saturate at the cap it was given, got %d", count)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("a saturated count must cost one query, spent %d", n)
	}

	calls.Store(0)
	count, err = store.Count(ctx, "owner", RelationshipFriend, MaxFriends)
	if err != nil {
		t.Fatal(err)
	}
	if count != friends {
		t.Fatalf("below the cap the exact count is still required, got %d", count)
	}
	if n := calls.Load(); n > maxCountPages {
		t.Fatalf("count must stay inside its %d-page budget, spent %d", maxCountPages, n)
	}
}

// TestEdgeCountIgnoresOtherRelationshipsInThePartition is issue #278: with
// relationship resolved by FilterExpression, a partition padded with rows of
// another relationship burned the page budget before reaching the matching
// rows and silently under-reported. As a key condition on gsi_relationship the
// padding is invisible to the query.
func TestEdgeCountIgnoresOtherRelationshipsInThePartition(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int64
	db := countingSocialTestClient(t, &calls)
	env := fmt.Sprintf("social_count_index_test_%d", time.Now().UnixNano())
	edgesTable := env + "_" + tableSocialEdges
	createSocialEdgeTestTable(t, db, edgesTable)
	t.Cleanup(func() {
		_, _ = db.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(edgesTable)})
	})

	store := NewStore(db, env)
	put := func(other string, relationship Relationship) {
		t.Helper()
		item, err := dynamoEncodeEdge(Edge{OwnerPlayerID: "owner", OtherPlayerID: other, Relationship: relationship, Version: 1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(edgesTable), Item: item}); err != nil {
			t.Fatal(err)
		}
	}
	// Padding sized past the old maxCountPages*saturateAt budget, and named so
	// it sorts ahead of the matching rows on the base table's sort key — the
	// exact shape that used to exhaust the budget before reaching them.
	const padding = maxCountPages*MaxPendingOutgoing + 50
	for i := 0; i < padding; i++ {
		put(fmt.Sprintf("aa-incoming-%04d", i), RelationshipIncoming)
	}
	const pending = 7
	for i := 0; i < pending; i++ {
		put(fmt.Sprintf("zz-outgoing-%03d", i), RelationshipOutgoing)
	}

	calls.Store(0)
	count, err := store.Count(ctx, "owner", RelationshipOutgoing, MaxPendingOutgoing)
	if err != nil {
		t.Fatal(err)
	}
	if count != pending {
		t.Fatalf("count must be exact regardless of the rest of the partition, got %d want %d", count, pending)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("an indexed count must cost one query, spent %d", n)
	}
}

func dynamoEncodeEdge(edge Edge) (map[string]types.AttributeValue, error) {
	return map[string]types.AttributeValue{
		"pk":           &types.AttributeValueMemberS{Value: edge.OwnerPlayerID},
		"sk":           &types.AttributeValueMemberS{Value: edge.OtherPlayerID},
		"relationship": &types.AttributeValueMemberS{Value: string(edge.Relationship)},
		"version":      &types.AttributeValueMemberN{Value: "1"},
	}, nil
}

// countingSocialTestClient is socialTestClient plus a request counter, so a
// test can assert a read budget rather than only its result.
func countingSocialTestClient(t *testing.T, calls *atomic.Int64) *dynamodb.Client {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	return dynamodb.NewFromConfig(cfg, func(options *dynamodb.Options) {
		options.BaseEndpoint = aws.String("http://localhost:8555")
		options.APIOptions = append(options.APIOptions, func(stack *middleware.Stack) error {
			return stack.Initialize.Add(middleware.InitializeMiddlewareFunc("countCalls",
				func(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (middleware.InitializeOutput, middleware.Metadata, error) {
					calls.Add(1)
					return next.HandleInitialize(ctx, in)
				}), middleware.Before)
		})
	})
}
