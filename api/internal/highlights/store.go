// Package highlights records the biggest pot won at each table each day —
// an automatic, system-detected highlight, no player action required. See
// docs/specs/2026-08-21-table-highlights-feed.md for why this is a separate,
// ownerless store rather than reusing internal/handshare.
package highlights

import (
	"context"
	"fmt"
	"sort"
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

// HighlightWinner is who actually won the pot, copied straight from
// hand.HandOutcome. It exists because Revealed above is empty whenever the
// hand ended without a showdown (an all-in everybody folds to), which left
// the card with a pot and no name — the winner was known server-side and
// thrown away. It also fixes the attribution the UI could only guess at:
// with a side pot, the best hand shown is not necessarily who won the most.
type HighlightWinner struct {
	PlayerID string `dynamodbav:"player_id" json:"player_id"`
	Name     string `dynamodbav:"name,omitempty" json:"name,omitempty"`
	Payout   int64  `dynamodbav:"payout" json:"payout"`
}

// Highlight is the biggest pot on record for one table on one UTC day. PK is
// TableID, SK is Date — overwritten in place as bigger pots come in, and
// naturally rolls over at UTC midnight since the SK changes.
type Highlight struct {
	TableID    string            `dynamodbav:"pk" json:"table_id"`
	Date       string            `dynamodbav:"sk" json:"date"`
	HandID     string            `dynamodbav:"hand_id" json:"hand_id"`
	Pot        int64             `dynamodbav:"pot" json:"pot"`
	Board      []string          `dynamodbav:"board,omitempty" json:"board,omitempty"`
	Winners    []HighlightWinner `dynamodbav:"winners,omitempty" json:"winners,omitempty"`
	Revealed   []RevealedHand    `dynamodbav:"revealed,omitempty" json:"revealed,omitempty"`
	RecordedAt int64             `dynamodbav:"recorded_at" json:"recorded_at"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableHighlights)}
}

// RecordHand overwrites today's highlight for tableID only if this hand's
// pot beats whatever is currently on record — same "update only if better"
// shape a leaderboard Top-N write uses.
func (s *Store) RecordHand(ctx context.Context, tableID, handID string, outcome hand.HandOutcome, names map[string]string) error {
	pot := ContestedPot(outcome)
	if pot <= 0 {
		return nil // no chips changed hands (e.g. a walkover) — nothing to highlight
	}
	item := Highlight{
		TableID: tableID, Date: time.Now().UTC().Format("2006-01-02"),
		HandID: handID, Pot: pot, Board: outcome.Board,
		Winners:  winnersOf(outcome, names),
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

// winnersOf copies the hand's actual winners — unlike revealedHandsOf, which
// is gated on a showdown having happened. Ordered by payout descending, then
// by player id, so a split pot renders the same caption on every read.
func winnersOf(outcome hand.HandOutcome, names map[string]string) []HighlightWinner {
	winners := make([]HighlightWinner, 0, len(outcome.Winners))
	seen := make(map[string]struct{}, len(outcome.Winners))
	for _, id := range outcome.Winners {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		winners = append(winners, HighlightWinner{PlayerID: id, Name: names[id], Payout: outcome.Payouts[id]})
	}
	if len(winners) == 0 {
		return nil
	}
	sort.Slice(winners, func(i, j int) bool {
		if winners[i].Payout != winners[j].Payout {
			return winners[i].Payout > winners[j].Payout
		}
		return winners[i].PlayerID < winners[j].PlayerID
	})
	return winners
}

// ContestedPot is the number this package means by "pote": the gross size of
// the layers that were actually fought over.
//
// Two deliberate choices, both of which a player can check against what the
// felt showed them:
//
//   - Refund layers are excluded. Uncalled excess returned to its own bettor
//     (an all-in everyone folds to) was never won by anyone, and counting it
//     would let a hand nobody contested top the day. This is why a felt
//     showing a 206.750 pot can legitimately fail to beat a 154.250 record:
//     125.750 of it went straight back to the player who bet it.
//   - The layer's gross Amount is used, not PayoutAmount. PayoutAmount is net
//     of rake, and rake is invisible on the table's pot display, so recording
//     it made the highlight read a few hundred chips lower than the number
//     every player at the table had just been looking at.
func ContestedPot(outcome hand.HandOutcome) int64 {
	pot := int64(0)
	for _, result := range outcome.PotResults {
		if !result.Refund {
			pot += result.Amount
		}
	}
	return pot
}
