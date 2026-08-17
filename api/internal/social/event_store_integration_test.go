//go:build integration

package social

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

func TestDynamoInviteAcceptanceCreatesPrivateRoomGrantAndUpdatesInbox(t *testing.T) {
	ctx := context.Background()
	db := socialTestClient(t)
	env := fmt.Sprintf("social_invite_test_%d", time.Now().UnixNano())
	eventsTable := env + "_" + tableSocialEvents
	roomsTable := env + "_" + tableRooms
	createSocialEventTestTable(t, db, eventsTable)
	createSimpleCompositeTable(t, db, roomsTable)
	t.Cleanup(func() {
		_, _ = db.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(eventsTable)})
		_, _ = db.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(roomsTable)})
	})

	rooms := roomstore.NewStore(db, env)
	room := roomstore.Room{ID: "private-room", Visibility: "private", ShareCode: "MUST-NOT-LEAK", Status: "waiting", MaxSeats: 6, CreatedBy: "sender"}
	if err := rooms.Create(ctx, room); err != nil {
		t.Fatal(err)
	}
	store := NewEventStore(db, env)
	now := time.Now().UTC()
	created, err := store.CreateInvite(ctx, Event{
		RecipientPlayerID: "guest", ActorPlayerID: "sender", Type: EventTableInvite,
		RoomID: room.ID, CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(InviteLifetime).UnixMilli(),
	}, "invite-once")
	if err != nil {
		t.Fatal(err)
	}

	accepted, err := store.AcceptInvite(ctx, *created, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != EventStatusAccepted || accepted.Unread {
		t.Fatalf("accepted event=%+v", accepted)
	}
	grant, err := rooms.GetInviteGrant(ctx, room.ID, "guest")
	if err != nil {
		t.Fatal(err)
	}
	if grant == nil || grant.EventID != created.EventID {
		t.Fatalf("grant=%+v", grant)
	}
	stored, err := store.Get(ctx, "guest", created.EventID)
	if err != nil || stored.Status != EventStatusAccepted || stored.Unread {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
}

func createSimpleCompositeTable(t *testing.T, db *dynamodb.Client, name string) {
	t.Helper()
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(name), BillingMode: types.BillingModePayPerRequest,
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
}

func createSocialEventTestTable(t *testing.T, db *dynamodb.Client, name string) {
	t.Helper()
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(name), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi_inbox_pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi_inbox_sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi_unread_pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi_unread_sk"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{IndexName: aws.String(gsiSocialInbox), Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}, KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("gsi_inbox_pk"), KeyType: types.KeyTypeHash}, {AttributeName: aws.String("gsi_inbox_sk"), KeyType: types.KeyTypeRange}}},
			{IndexName: aws.String(gsiSocialUnread), Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}, KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("gsi_unread_pk"), KeyType: types.KeyTypeHash}, {AttributeName: aws.String("gsi_unread_sk"), KeyType: types.KeyTypeRange}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}
