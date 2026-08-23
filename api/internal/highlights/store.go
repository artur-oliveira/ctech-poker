// Package highlights records the biggest pot won at each table each day —
// an automatic, system-detected highlight, no player action required. See
// docs/specs/2026-08-21-table-highlights-feed.md for why this is a separate,
// ownerless store rather than reusing internal/handshare.
package highlights

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

const tableHighlights = "poker_table_highlights"

// RevealedHand is one participant's hole cards, copied into a Highlight only
// when hand.PlayerHandInfo.Revealed was already true — never a raw
// HoleCards field, never re-derived client-side.
type RevealedHand struct {
	PlayerID  string   `dynamodbav:"player_id" json:"player_id"`
	Name      string   `dynamodbav:"name,omitempty" json:"name,omitempty"`
	HoleCards []string `dynamodbav:"hole_cards" json:"hole_cards"`
}

// Highlight is the biggest pot on record for one table on one UTC day. PK is
// TableID, SK is Date — overwritten in place as bigger pots come in, and
// naturally rolls over at UTC midnight since the SK changes.
type Highlight struct {
	TableID    string         `dynamodbav:"pk" json:"table_id"`
	Date       string         `dynamodbav:"sk" json:"date"`
	HandID     string         `dynamodbav:"hand_id" json:"hand_id"`
	Pot        int64          `dynamodbav:"pot" json:"pot"`
	Board      []string       `dynamodbav:"board,omitempty" json:"board,omitempty"`
	Revealed   []RevealedHand `dynamodbav:"revealed,omitempty" json:"revealed,omitempty"`
	RecordedAt int64          `dynamodbav:"recorded_at" json:"recorded_at"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableHighlights)}
}

// RecordHand overwrites today's highlight for tableID only if this hand's
// pot beats whatever is currently on record — same "update only if better"
// shape a leaderboard Top-N write uses.
func (s *Store) RecordHand(ctx context.Context, tableID, handID string, outcome hand.HandOutcome, names map[string]string) error {
	pot := int64(0)
	for _, amount := range outcome.Payouts {
		pot += amount
	}
	if pot <= 0 {
		return nil // no chips changed hands (e.g. a walkover) — nothing to highlight
	}
	item := Highlight{
		TableID: tableID, Date: time.Now().UTC().Format("2006-01-02"),
		HandID: handID, Pot: pot, Board: outcome.Board,
		Revealed: revealedHandsOf(outcome, names), RecordedAt: time.Now().UnixMilli(),
	}
	encoded, err := dynamo.Encode(item)
	if err != nil {
		return fmt.Errorf("highlights: encode: %w", err)
	}
	_, err = s.base.PutItemRaw(ctx, &dynamodb.PutItemInput{
		Item:                encoded,
		ConditionExpression: aws.String("attribute_not_exists(pk) OR pot < :pot"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pot": &types.AttributeValueMemberN{Value: strconv.FormatInt(pot, 10)},
		},
	})
	if err != nil {
		if dynamo.IsConditionFailed(err) {
			return nil
		}
		return fmt.Errorf("highlights: record hand: %w", err)
	}
	return nil
}

// GetToday returns tableID's biggest-pot highlight for the current UTC day,
// or nil if none has been recorded yet.
func (s *Store) GetToday(ctx context.Context, tableID string) (*Highlight, error) {
	item, err := s.base.GetItem(ctx, tableID, time.Now().UTC().Format("2006-01-02"))
	if err != nil || item == nil {
		return nil, err
	}
	return dynamo.Decode[Highlight](item)
}

// revealedHandsOf copies only participants whose hand was actually shown —
// hand.PlayerHandInfo.Revealed == true, the same flag sessionlog.OpponentSummary
// and handshare's anonymizedOpponents gate on. A folded/mucked hand must
// never appear here.
func revealedHandsOf(outcome hand.HandOutcome, names map[string]string) []RevealedHand {
	var revealed []RevealedHand
	for _, id := range outcome.Participants {
		info, ok := outcome.PlayerHands[id]
		if !ok || !info.Revealed {
			continue
		}
		revealed = append(revealed, RevealedHand{PlayerID: id, Name: names[id], HoleCards: info.HoleCards[:]})
	}
	return revealed
}
