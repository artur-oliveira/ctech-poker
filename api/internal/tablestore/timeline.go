package tablestore

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

// TimelineEvent is one line of a hand's compact timeline: what a player did,
// for how much, when. Deliberately without ReplayFrame — the frame is the
// whole board and up to nine seats per action, which is what makes the full
// action log expensive to read and to render (#221, #302). A caller that
// needs the frames wants LoadActionsSince, not this.
type TimelineEvent struct {
	Seq       int    `dynamodbav:"seq" json:"seq"`
	PlayerID  string `dynamodbav:"player_id" json:"player_id"`
	Action    string `dynamodbav:"action" json:"action"`
	Amount    int64  `dynamodbav:"amount" json:"amount"`
	Timestamp int64  `dynamodbav:"timestamp" json:"timestamp"` // unix millis
}

// cosmeticTimelineActions are the events a timeline deliberately drops: they
// are already delivered live at the table, carry no game state, and are the
// bulk of a busy table's log. A support agent reconstructing a hand wants the
// decisions, not the emoji.
var cosmeticTimelineActions = map[string]bool{
	"chat":       true,
	"reaction":   true,
	"peek_cards": true,
}

// timelineProjection keeps the read to TimelineEvent's five attributes. Three
// of them (action, amount, timestamp) are DynamoDB reserved words, so the
// projection is aliased — which is why this uses QueryRaw rather than
// dynamo.QueryOpts.ProjectionExpression, whose typed form cannot carry
// ExpressionAttributeNames.
var timelineProjectionNames = map[string]string{
	"#action":    "action",
	"#amount":    "amount",
	"#timestamp": "timestamp",
}

const timelineProjection = "seq,player_id,#action,#amount,#timestamp"

// LoadTimeline returns the compact, game-relevant event sequence of one hand,
// oldest first — the same partition LoadActionsSince reads, minus every
// action's ReplayFrame and minus the cosmetic events.
//
// It never writes a second copy of anything: #221's lesson is that a timeline
// which persists its own per-event record just reproduces the write
// amplification the frame already caused. This is a projection over rows that
// already exist.
func (s *Store) LoadTimeline(ctx context.Context, tableID, handID string) ([]TimelineEvent, error) {
	if tableID == "" || handID == "" {
		return nil, nil
	}
	out, err := s.log.QueryRaw(ctx, &dynamodb.QueryInput{
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: tableID + "#" + handID},
		},
		ExpressionAttributeNames: timelineProjectionNames,
		ProjectionExpression:     aws.String(timelineProjection),
		// Oldest first, for the same reason LoadActionsSince asks for it: the
		// stored Seq is backfilled from result order for rows written before
		// the field existed, and every consumer renders in that order.
		ScanIndexForward: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("tablestore: load timeline: %w", err)
	}
	return timelineFrom(out.Items), nil
}

// timelineFrom decodes and filters a projected action-log page: cosmetic
// events dropped, Seq backfilled from result order for rows written before
// the field existed (same rule LoadActionsSince applies).
func timelineFrom(items []map[string]types.AttributeValue) []TimelineEvent {
	events := make([]TimelineEvent, 0, len(items))
	for i, item := range items {
		event, decodeErr := dynamo.Decode[TimelineEvent](item)
		if decodeErr != nil || event == nil {
			continue
		}
		if event.Seq == 0 {
			event.Seq = i + 1
		}
		if cosmeticTimelineActions[event.Action] {
			continue
		}
		events = append(events, *event)
	}
	return events
}
