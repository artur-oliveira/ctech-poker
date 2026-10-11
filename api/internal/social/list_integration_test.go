//go:build integration

package social

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"gopkg.aoctech.app/api-commons/dynamo"
)

// DynamoDB applies a Query's Limit to the rows it evaluates, before the
// FilterExpression. A player's edge partition mixes friends, requests, mutes
// and blocks, so a friends (or blocked) page used to come back empty whenever
// the first Limit rows were something else — here, 60 muted players sorting
// before the friends.
func TestListFindsEdgesPastTheFirstPage(t *testing.T) {
	ctx := context.Background()
	db := socialTestClient(t)
	env := fmt.Sprintf("social_list_test_%d", time.Now().UnixNano())
	name := env + "_" + tableSocialEdges
	createSocialEdgeTestTable(t, db, name)
	t.Cleanup(func() { _, _ = db.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(name)}) })
	store := NewStore(db, env)

	put := func(e Edge) {
		item, err := dynamo.Encode(e)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(name), Item: item}); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 60 {
		put(Edge{OwnerPlayerID: "me", OtherPlayerID: fmt.Sprintf("a-muted-%02d", i), Muted: true, UpdatedAt: 1, Version: 1})
	}
	var friends, blocked []string
	for i := range 3 {
		id := fmt.Sprintf("f-friend-%d", i)
		friends = append(friends, id)
		put(Edge{OwnerPlayerID: "me", OtherPlayerID: id, Relationship: RelationshipFriend, UpdatedAt: 1, Version: 1})
	}
	for i := range 2 {
		id := fmt.Sprintf("z-blocked-%d", i)
		blocked = append(blocked, id)
		put(Edge{OwnerPlayerID: "me", OtherPlayerID: id, Blocked: true, UpdatedAt: 1, Version: 1})
	}

	others := func(edges []Edge) []string {
		out := make([]string, 0, len(edges))
		for _, e := range edges {
			out = append(out, e.OtherPlayerID)
		}
		return out
	}

	edges, next, err := store.List(ctx, "me", RelationshipFriend, false, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := others(edges); !slices.Equal(got, friends) {
		t.Fatalf("friends page = %v, want %v", got, friends)
	}
	if next != nil {
		t.Fatalf("partition exhausted, cursor must be nil, got %v", next)
	}

	edges, _, err = store.List(ctx, "me", RelationshipNone, true, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := others(edges); !slices.Equal(got, blocked) {
		t.Fatalf("blocked page = %v, want %v", got, blocked)
	}

	// Paging friends one at a time visits each exactly once.
	var got []string
	var start map[string]types.AttributeValue
	for range 100 {
		page, cursor, err := store.List(ctx, "me", RelationshipFriend, false, 1, start)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, others(page)...)
		if cursor == nil {
			break
		}
		start = cursor
	}
	if !slices.Equal(got, friends) {
		t.Fatalf("paged friends = %v, want %v", got, friends)
	}
}
