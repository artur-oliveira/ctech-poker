//go:build integration

package roomstore

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func testClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://localhost:8555")
	})
}

// mustCreateTestTable provisions the poker_rooms table with its three GSIs
// against DynamoDB Local — production tables are provisioned by CDK, never
// by app code.
func mustCreateTestTable(ctx context.Context, t *testing.T, db *dynamodb.Client, env string) {
	name := env + "_" + tableRooms
	_, err := db.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(name),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi_public"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi_bucket"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi_share_code"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
		},
		BillingMode: types.BillingModePayPerRequest,
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String(gsiPublic),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("gsi_public"), KeyType: types.KeyTypeHash},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
			{
				IndexName: aws.String(gsiBucket),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("gsi_bucket"), KeyType: types.KeyTypeHash},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
			{
				IndexName: aws.String(gsiShareCode),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("gsi_share_code"), KeyType: types.KeyTypeHash},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
		},
	})
	if err != nil {
		var inUse *types.ResourceInUseException
		if !errors.As(err, &inUse) {
			t.Fatalf("create table %s: %v", name, err)
		}
	}
}

func TestCreateGetAndListPublic(t *testing.T) {
	db := testClient(t)
	s := NewStore(db, "test")
	ctx := context.Background()
	mustCreateTestTable(ctx, t, db, "test")

	pub := Room{ID: "room-pub-1", Visibility: "public", CurrencyMode: "sandbox", SmallBlind: 10, BigBlind: 20, MaxSeats: 9, BuyInMin: 400, BuyInMax: 2000, EquityDisplayEnabled: true, Status: "waiting", CreatedBy: "u1", CreatedAt: "2026-07-18T00:00:00Z"}
	if err := s.Create(ctx, pub); err != nil {
		t.Fatalf("create public: %v", err)
	}

	priv := Room{ID: "room-priv-1", Visibility: "private", CurrencyMode: "sandbox", SmallBlind: 5, BigBlind: 10, MaxSeats: 6, BuyInMin: 200, BuyInMax: 1000, ShareCode: "ABC123", EquityDisplayEnabled: false, Status: "waiting", CreatedBy: "u2", CreatedAt: "2026-07-18T00:00:01Z"}
	if err := s.Create(ctx, priv); err != nil {
		t.Fatalf("create private: %v", err)
	}

	got, err := s.Get(ctx, "room-pub-1")
	if err != nil || got == nil || got.SmallBlind != 10 {
		t.Fatalf("get: %+v, err=%v", got, err)
	}

	byCode, err := s.GetByShareCode(ctx, "ABC123")
	if err != nil || byCode == nil || byCode.ID != "room-priv-1" {
		t.Fatalf("get by share code: %+v, err=%v", byCode, err)
	}

	list, _, err := s.ListPublic(ctx, 10, nil)
	if err != nil {
		t.Fatalf("list public: %v", err)
	}
	if len(list) != 1 || list[0].ID != "room-pub-1" {
		t.Fatalf("expected only the public room listed, got %+v", list)
	}
}

// The point of gsi_bucket: one join attempt reads its own bucket's rooms and
// nothing else, no matter how many public rooms exist in other buckets (#213).
func TestListBucketReadsOnlyTheRequestedBucket(t *testing.T) {
	db := testClient(t)
	s := NewStore(db, "test")
	ctx := context.Background()
	mustCreateTestTable(ctx, t, db, "test")

	room := func(id string, bigBlind int64, maxSeats int, visibility string) Room {
		return Room{
			ID: id, Visibility: visibility, CurrencyMode: "sandbox", SmallBlind: bigBlind / 2,
			BigBlind: bigBlind, MaxSeats: maxSeats, BuyInMin: bigBlind * 20, BuyInMax: bigBlind * 100,
			Status: "waiting", CreatedBy: "u1", CreatedAt: "2026-09-04T00:00:00Z",
		}
	}
	// The bucket itself, then one room off it on every axis that defines a bucket.
	for _, r := range []Room{
		room("bucket-a-1", 20, 6, "public"),
		room("bucket-a-2", 20, 6, "public"),
		room("other-blinds", 50, 6, "public"),
		room("other-seats", 20, 9, "public"),
		room("private-same-bucket", 20, 6, "private"),
	} {
		if err := s.Create(ctx, r); err != nil {
			t.Fatalf("create %s: %v", r.ID, err)
		}
	}
	realMoney := room("real-same-stakes", 20, 6, "public")
	realMoney.CurrencyMode = CurrencyModeReal
	if err := s.Create(ctx, realMoney); err != nil {
		t.Fatalf("create real: %v", err)
	}

	got, err := s.ListBucket(ctx, CurrencyModeSandbox, 10, 20, 6)
	if err != nil {
		t.Fatalf("list bucket: %v", err)
	}
	ids := map[string]bool{}
	for _, r := range got {
		ids[r.ID] = true
	}
	if len(got) != 2 || !ids["bucket-a-1"] || !ids["bucket-a-2"] {
		t.Fatalf("expected exactly the two sandbox 10/20 six-max public rooms, got %v", ids)
	}
}

func TestBucketKeyTreatsAMissingCurrencyModeAsSandbox(t *testing.T) {
	if BucketKey("", 10, 20, 6) != BucketKey(CurrencyModeSandbox, 10, 20, 6) {
		t.Fatal("a room predating currency_mode must land in the sandbox bucket")
	}
}
