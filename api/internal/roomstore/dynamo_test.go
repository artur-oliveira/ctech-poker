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

// mustCreateTestTable provisions the poker_rooms table with its two GSIs
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

	list, _, err := s.ListPublic(ctx, 10, "")
	if err != nil {
		t.Fatalf("list public: %v", err)
	}
	if len(list) != 1 || list[0].ID != "room-pub-1" {
		t.Fatalf("expected only the public room listed, got %+v", list)
	}
}
