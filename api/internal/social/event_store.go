package social

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableSocialEvents = "poker_social_events"
	tableRooms        = "poker_rooms"
	gsiSocialInbox    = "gsi_inbox"
	gsiSocialUnread   = "gsi_unread"
	eventRetention    = 90 * 24 * time.Hour
	inviteGuardPrefix = "invite_guard#"
	inviteGrantPrefix = "invite#"
	roomMetaSK        = "meta"
)

var (
	ErrEventNotFound     = errors.New("social: event not found")
	ErrInviteExpired     = errors.New("social: invite expired")
	ErrInviteNotPending  = errors.New("social: invite is not pending")
	ErrInviteAlreadySent = errors.New("social: invite already pending")
)

// EventStore is the durable inbox boundary. Invite acceptance commits the
// event state and private-room grant in one DynamoDB transaction.
type EventStore interface {
	Create(ctx context.Context, event Event, idempotencyKey string) (*Event, error)
	CreateInvite(ctx context.Context, event Event, idempotencyKey string) (*Event, error)
	Get(ctx context.Context, recipientPlayerID, eventID string) (*Event, error)
	List(ctx context.Context, recipientPlayerID string, limit int, startKey map[string]types.AttributeValue) ([]Event, map[string]types.AttributeValue, error)
	UnreadCount(ctx context.Context, recipientPlayerID string) (int, error)
	MarkRead(ctx context.Context, recipientPlayerID string, eventIDs []string) error
	AcceptInvite(ctx context.Context, event Event, acceptedAt time.Time) (*Event, error)
	DeclineInvite(ctx context.Context, event Event, declinedAt time.Time) (*Event, error)
}

type DynamoEventStore struct {
	db         *dynamodb.Client
	eventTable string
	roomTable  string
}

func NewEventStore(db *dynamodb.Client, env string) *DynamoEventStore {
	return &DynamoEventStore{
		db: db, eventTable: dynamo.TableName(env, tableSocialEvents), roomTable: dynamo.TableName(env, tableRooms),
	}
}

func deterministicEventID(event Event, idempotencyKey string) string {
	sum := sha256.Sum256([]byte(string(event.Type) + "\x00" + event.ActorPlayerID + "\x00" + event.RecipientPlayerID + "\x00" + event.RoomID + "\x00" + idempotencyKey))
	return hex.EncodeToString(sum[:16])
}

func prepareEvent(event Event, idempotencyKey string) Event {
	if event.EventID == "" {
		event.EventID = deterministicEventID(event, idempotencyKey)
	}
	if event.Status == "" {
		event.Status = EventStatusPending
	}
	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().UTC().UnixMilli()
	}
	event.Unread = true
	event.InboxPartition = event.RecipientPlayerID
	event.InboxSort = fmt.Sprintf("%020d#%s", event.CreatedAt, event.EventID)
	event.UnreadPartition = event.RecipientPlayerID + "#unread"
	event.UnreadSort = event.InboxSort
	event.TTL = time.UnixMilli(event.CreatedAt).Add(eventRetention).Unix()
	return event
}

func (s *DynamoEventStore) Create(ctx context.Context, event Event, idempotencyKey string) (*Event, error) {
	event = prepareEvent(event, idempotencyKey)
	item, err := attributevalue.MarshalMap(event)
	if err != nil {
		return nil, fmt.Errorf("social events: encode: %w", err)
	}
	_, err = s.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.eventTable), Item: item,
		ConditionExpression: aws.String("attribute_not_exists(pk) AND attribute_not_exists(sk)"),
	})
	if err == nil {
		return &event, nil
	}
	var conditional *types.ConditionalCheckFailedException
	if !errors.As(err, &conditional) {
		return nil, fmt.Errorf("social events: create: %w", err)
	}
	existing, getErr := s.Get(ctx, event.RecipientPlayerID, event.EventID)
	if getErr != nil {
		return nil, getErr
	}
	return existing, nil
}

type inviteGuard struct {
	RecipientPlayerID string `dynamodbav:"pk"`
	SK                string `dynamodbav:"sk"`
	EventID           string `dynamodbav:"event_id"`
	TTL               int64  `dynamodbav:"ttl"`
}

func inviteGuardSK(actorID, roomID string) string { return inviteGuardPrefix + actorID + "#" + roomID }

func (s *DynamoEventStore) CreateInvite(ctx context.Context, event Event, idempotencyKey string) (*Event, error) {
	event = prepareEvent(event, idempotencyKey)
	encodedEvent, err := attributevalue.MarshalMap(event)
	if err != nil {
		return nil, fmt.Errorf("social events: encode invite: %w", err)
	}
	guard := inviteGuard{RecipientPlayerID: event.RecipientPlayerID, SK: inviteGuardSK(event.ActorPlayerID, event.RoomID), EventID: event.EventID, TTL: time.UnixMilli(event.ExpiresAt).Unix()}
	encodedGuard, err := attributevalue.MarshalMap(guard)
	if err != nil {
		return nil, fmt.Errorf("social events: encode invite guard: %w", err)
	}
	_, err = s.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Put: &types.Put{TableName: aws.String(s.eventTable), Item: encodedEvent, ConditionExpression: aws.String("attribute_not_exists(pk) AND attribute_not_exists(sk)")}},
		{Put: &types.Put{TableName: aws.String(s.eventTable), Item: encodedGuard, ConditionExpression: aws.String("attribute_not_exists(pk) OR #ttl < :now"), ExpressionAttributeNames: map[string]string{"#ttl": "ttl"}, ExpressionAttributeValues: map[string]types.AttributeValue{":now": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().UTC().Unix())}}}},
	}})
	if err == nil {
		return &event, nil
	}
	var canceled *types.TransactionCanceledException
	if !errors.As(err, &canceled) {
		return nil, fmt.Errorf("social events: create invite: %w", err)
	}
	// The exact event already existing is the successful result of a retry,
	// even if its guard has since expired or been removed by accept/decline.
	if existing, existingErr := s.Get(ctx, event.RecipientPlayerID, event.EventID); existingErr == nil {
		return existing, nil
	}
	guardItem, getErr := s.db.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(s.eventTable), Key: eventKey(event.RecipientPlayerID, guard.SK), ConsistentRead: aws.Bool(true)})
	if getErr != nil {
		return nil, fmt.Errorf("social events: get invite guard: %w", getErr)
	}
	if len(guardItem.Item) > 0 {
		var existingGuard inviteGuard
		if attributevalue.UnmarshalMap(guardItem.Item, &existingGuard) == nil && existingGuard.TTL >= time.Now().UTC().Unix() {
			existing, existingErr := s.Get(ctx, event.RecipientPlayerID, existingGuard.EventID)
			if existingErr == nil && existing.Status == EventStatusPending {
				return existing, ErrInviteAlreadySent
			}
		}
	}
	return nil, fmt.Errorf("social events: create invite transaction canceled: %w", err)
}

func eventKey(recipientID, eventID string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: recipientID},
		"sk": &types.AttributeValueMemberS{Value: eventID},
	}
}

func (s *DynamoEventStore) Get(ctx context.Context, recipientPlayerID, eventID string) (*Event, error) {
	out, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(s.eventTable), Key: eventKey(recipientPlayerID, eventID), ConsistentRead: aws.Bool(true)})
	if err != nil {
		return nil, fmt.Errorf("social events: get: %w", err)
	}
	if len(out.Item) == 0 {
		return nil, ErrEventNotFound
	}
	var event Event
	if err := attributevalue.UnmarshalMap(out.Item, &event); err != nil {
		return nil, fmt.Errorf("social events: decode: %w", err)
	}
	return &event, nil
}

func (s *DynamoEventStore) List(ctx context.Context, recipientPlayerID string, limit int, startKey map[string]types.AttributeValue) ([]Event, map[string]types.AttributeValue, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	out, err := s.db.Query(ctx, &dynamodb.QueryInput{
		TableName: aws.String(s.eventTable), IndexName: aws.String(gsiSocialInbox),
		KeyConditionExpression: aws.String("gsi_inbox_pk = :pk"), ExpressionAttributeValues: map[string]types.AttributeValue{":pk": &types.AttributeValueMemberS{Value: recipientPlayerID}},
		Limit: aws.Int32(int32(limit)), ScanIndexForward: aws.Bool(false), ExclusiveStartKey: startKey,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("social events: list: %w", err)
	}
	items := make([]Event, 0, len(out.Items))
	for _, item := range out.Items {
		var event Event
		if err := attributevalue.UnmarshalMap(item, &event); err != nil {
			return nil, nil, fmt.Errorf("social events: decode list item: %w", err)
		}
		items = append(items, event)
	}
	return items, out.LastEvaluatedKey, nil
}

func (s *DynamoEventStore) UnreadCount(ctx context.Context, recipientPlayerID string) (int, error) {
	var count int32
	var startKey map[string]types.AttributeValue
	for {
		out, err := s.db.Query(ctx, &dynamodb.QueryInput{
			TableName: aws.String(s.eventTable), IndexName: aws.String(gsiSocialUnread), Select: types.SelectCount,
			KeyConditionExpression: aws.String("gsi_unread_pk = :pk"), ExpressionAttributeValues: map[string]types.AttributeValue{":pk": &types.AttributeValueMemberS{Value: recipientPlayerID + "#unread"}}, ExclusiveStartKey: startKey,
		})
		if err != nil {
			return 0, fmt.Errorf("social events: unread count: %w", err)
		}
		count += out.Count
		if len(out.LastEvaluatedKey) == 0 {
			return int(count), nil
		}
		startKey = out.LastEvaluatedKey
	}
}

func (s *DynamoEventStore) MarkRead(ctx context.Context, recipientPlayerID string, eventIDs []string) error {
	if len(eventIDs) > 100 {
		return errors.New("social events: at most 100 events can be marked read")
	}
	for _, eventID := range eventIDs {
		_, err := s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName: aws.String(s.eventTable), Key: eventKey(recipientPlayerID, eventID),
			UpdateExpression:          aws.String("SET unread = :false REMOVE gsi_unread_pk, gsi_unread_sk"),
			ConditionExpression:       aws.String("attribute_exists(pk) AND attribute_exists(sk)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{":false": &types.AttributeValueMemberBOOL{Value: false}},
		})
		var conditional *types.ConditionalCheckFailedException
		if err != nil && !errors.As(err, &conditional) {
			return fmt.Errorf("social events: mark read: %w", err)
		}
	}
	return nil
}

func (s *DynamoEventStore) AcceptInvite(ctx context.Context, event Event, acceptedAt time.Time) (*Event, error) {
	if event.ExpiresAt <= acceptedAt.UTC().UnixMilli() {
		return nil, ErrInviteExpired
	}
	grantItem := map[string]types.AttributeValue{
		"pk":         &types.AttributeValueMemberS{Value: event.RoomID},
		"sk":         &types.AttributeValueMemberS{Value: inviteGrantPrefix + event.RecipientPlayerID},
		"player_id":  &types.AttributeValueMemberS{Value: event.RecipientPlayerID},
		"event_id":   &types.AttributeValueMemberS{Value: event.EventID},
		"expires_at": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", event.ExpiresAt)},
		"ttl":        &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.UnixMilli(event.ExpiresAt).Unix())},
	}
	values := map[string]types.AttributeValue{
		":pending":  &types.AttributeValueMemberS{Value: string(EventStatusPending)},
		":accepted": &types.AttributeValueMemberS{Value: string(EventStatusAccepted)},
		":now":      &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", acceptedAt.UTC().UnixMilli())},
		":false":    &types.AttributeValueMemberBOOL{Value: false},
	}
	_, err := s.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Update: &types.Update{TableName: aws.String(s.eventTable), Key: eventKey(event.RecipientPlayerID, event.EventID), UpdateExpression: aws.String("SET #status = :accepted, unread = :false REMOVE gsi_unread_pk, gsi_unread_sk"), ConditionExpression: aws.String("#status = :pending AND expires_at > :now"), ExpressionAttributeNames: map[string]string{"#status": "status"}, ExpressionAttributeValues: values}},
		{ConditionCheck: &types.ConditionCheck{
			TableName: aws.String(s.roomTable), Key: eventKey(event.RoomID, roomMetaSK),
			ConditionExpression:      aws.String("attribute_exists(pk) AND (#status = :waiting OR #status = :active) AND seats_taken < max_seats"),
			ExpressionAttributeNames: map[string]string{"#status": "status"},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":waiting": &types.AttributeValueMemberS{Value: "waiting"},
				":active":  &types.AttributeValueMemberS{Value: "active"},
			},
		}},
		{Put: &types.Put{TableName: aws.String(s.roomTable), Item: grantItem}},
		{Delete: &types.Delete{TableName: aws.String(s.eventTable), Key: eventKey(event.RecipientPlayerID, inviteGuardSK(event.ActorPlayerID, event.RoomID))}},
	}})
	if err != nil {
		var canceled *types.TransactionCanceledException
		if errors.As(err, &canceled) {
			room, roomErr := s.db.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(s.roomTable), Key: eventKey(event.RoomID, roomMetaSK), ConsistentRead: aws.Bool(true)})
			if roomErr == nil {
				if len(room.Item) == 0 {
					return nil, ErrRoomClosed
				}
				var state struct {
					Status     string `dynamodbav:"status"`
					SeatsTaken int    `dynamodbav:"seats_taken"`
					MaxSeats   int    `dynamodbav:"max_seats"`
				}
				if attributevalue.UnmarshalMap(room.Item, &state) == nil {
					if state.Status != "waiting" && state.Status != "active" {
						return nil, ErrRoomClosed
					}
					if state.SeatsTaken >= state.MaxSeats {
						return nil, ErrRoomFull
					}
				}
			}
			return nil, ErrInviteNotPending
		}
		return nil, fmt.Errorf("social events: accept invite: %w", err)
	}
	event.Status, event.Unread = EventStatusAccepted, false
	event.UnreadPartition, event.UnreadSort = "", ""
	return &event, nil
}

func (s *DynamoEventStore) DeclineInvite(ctx context.Context, event Event, declinedAt time.Time) (*Event, error) {
	if event.ExpiresAt <= declinedAt.UTC().UnixMilli() {
		return nil, ErrInviteExpired
	}
	_, err := s.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Update: &types.Update{TableName: aws.String(s.eventTable), Key: eventKey(event.RecipientPlayerID, event.EventID), UpdateExpression: aws.String("SET #status = :declined, unread = :false REMOVE gsi_unread_pk, gsi_unread_sk"), ConditionExpression: aws.String("#status = :pending AND expires_at > :now"), ExpressionAttributeNames: map[string]string{"#status": "status"}, ExpressionAttributeValues: map[string]types.AttributeValue{
			":pending": &types.AttributeValueMemberS{Value: string(EventStatusPending)}, ":declined": &types.AttributeValueMemberS{Value: string(EventStatusDeclined)}, ":now": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", declinedAt.UTC().UnixMilli())}, ":false": &types.AttributeValueMemberBOOL{Value: false},
		}}},
		{Delete: &types.Delete{TableName: aws.String(s.eventTable), Key: eventKey(event.RecipientPlayerID, inviteGuardSK(event.ActorPlayerID, event.RoomID))}},
	}})
	if err != nil {
		var canceled *types.TransactionCanceledException
		if errors.As(err, &canceled) {
			return nil, ErrInviteNotPending
		}
		return nil, fmt.Errorf("social events: decline invite: %w", err)
	}
	event.Status, event.Unread = EventStatusDeclined, false
	event.UnreadPartition, event.UnreadSort = "", ""
	return &event, nil
}
