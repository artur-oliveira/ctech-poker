//go:build integration

package entitlement

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// testClient / createTestTable mirror buyin's own copies — this repo keeps a
// per-package copy of these DynamoDB Local helpers rather than a shared
// test-helpers package (same as roomstore/tablestore/buyin).
func testClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}

func testStore(t *testing.T) *Store {
	t.Helper()
	db := testClient(t)
	env := fmt.Sprintf("entitlement_test_%d", time.Now().UnixNano())
	tableName := env + "_" + tableEntitlements
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
	if err != nil {
		var inUse *types.ResourceInUseException
		if !errors.As(err, &inUse) {
			t.Fatalf("create table: %v", err)
		}
	}
	return NewStore(db, env)
}

func TestClaimIsExactlyOnceUnderConcurrency(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	base := Entitlement{PlayerID: "player-1", OriginTableID: "table-1", BoundTableID: "table-1", Tier: "micro", FeeCents: 100, ExpiresAt: time.Now().Add(Window)}

	const n = 8
	var wg sync.WaitGroup
	results := make([]error, n)
	winners := make([]Entitlement, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			winners[i], results[i] = s.Claim(ctx, base)
		}(i)
	}
	wg.Wait()

	successes, alreadyClaimed := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAlreadyClaimed):
			alreadyClaimed++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 || alreadyClaimed != n-1 {
		t.Fatalf("expected exactly 1 success and %d ErrAlreadyClaimed, got %d successes and %d already-claimed", n-1, successes, alreadyClaimed)
	}

	ents, err := s.ActiveFor(ctx, "player-1")
	if err != nil {
		t.Fatalf("ActiveFor: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("expected exactly one persisted entitlement, got %d", len(ents))
	}

	// Every racer — the one that won and every one that lost — must agree on
	// the exact same persisted CreatedAt (to whole-second precision, which is
	// all a caller's derived fee idempotency key uses), since a caller
	// derives a shared fee idempotency key from it. A loser reporting
	// anything but the winner's own CreatedAt would reopen the free-seat
	// race this atomic read-back closes.
	for i, err := range results {
		if err == nil {
			continue
		}
		if winners[i].CreatedAt.Unix() != ents[0].CreatedAt.Unix() {
			t.Fatalf("loser %d reported CreatedAt %v, want the winner's persisted %v", i, winners[i].CreatedAt, ents[0].CreatedAt)
		}
	}
}

func TestClaimRejectsSecondCallForSameOriginTable(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	e := Entitlement{PlayerID: "player-1", OriginTableID: "table-1", BoundTableID: "table-1", Tier: "micro", FeeCents: 100, ExpiresAt: time.Now().Add(Window)}

	first, err := s.Claim(ctx, e)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	second, err := s.Claim(ctx, e)
	if !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("second claim: got %v, want ErrAlreadyClaimed", err)
	}
	if second.CreatedAt.Unix() != first.CreatedAt.Unix() {
		t.Fatalf("expected the losing claim to report the winner's own CreatedAt %v, got %v", first.CreatedAt, second.CreatedAt)
	}
}

func TestActiveForOmitsExpiredEntitlements(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	expired := Entitlement{PlayerID: "player-1", OriginTableID: "table-expired", BoundTableID: "table-expired", Tier: "micro", FeeCents: 100, ExpiresAt: time.Now().Add(-time.Hour)}
	active := Entitlement{PlayerID: "player-1", OriginTableID: "table-active", BoundTableID: "table-active", Tier: "micro", FeeCents: 100, ExpiresAt: time.Now().Add(Window)}
	if _, err := s.Claim(ctx, expired); err != nil {
		t.Fatalf("claim expired: %v", err)
	}
	if _, err := s.Claim(ctx, active); err != nil {
		t.Fatalf("claim active: %v", err)
	}

	ents, err := s.ActiveFor(ctx, "player-1")
	if err != nil {
		t.Fatalf("ActiveFor: %v", err)
	}
	if len(ents) != 1 || ents[0].OriginTableID != "table-active" {
		t.Fatalf("expected only the active entitlement, got %+v", ents)
	}
}

func TestRebindMovesBoundTableID(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	e := Entitlement{PlayerID: "player-1", OriginTableID: "table-origin", BoundTableID: "table-origin", Tier: "micro", FeeCents: 100, ExpiresAt: time.Now().Add(Window)}
	if _, err := s.Claim(ctx, e); err != nil {
		t.Fatalf("claim: %v", err)
	}

	if err := s.Rebind(ctx, "player-1", "table-origin", "table-new"); err != nil {
		t.Fatalf("rebind: %v", err)
	}

	ents, err := s.ActiveFor(ctx, "player-1")
	if err != nil {
		t.Fatalf("ActiveFor: %v", err)
	}
	if len(ents) != 1 || ents[0].BoundTableID != "table-new" || ents[0].OriginTableID != "table-origin" {
		t.Fatalf("expected bound_table_id moved to table-new with origin unchanged, got %+v", ents)
	}
}

func TestRebindFailsForExpiredEntitlement(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	e := Entitlement{PlayerID: "player-1", OriginTableID: "table-origin", BoundTableID: "table-origin", Tier: "micro", FeeCents: 100, ExpiresAt: time.Now().Add(-time.Minute)}
	if _, err := s.Claim(ctx, e); err != nil {
		t.Fatalf("claim: %v", err)
	}

	if err := s.Rebind(ctx, "player-1", "table-origin", "table-new"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rebind expired: got %v, want ErrNotFound", err)
	}
}

func TestRebindFailsForUnknownEntitlement(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Rebind(ctx, "player-1", "table-never-claimed", "table-new"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rebind unknown: got %v, want ErrNotFound", err)
	}
}
