// Package handreveal implements the paid history reveal of an
// uncontested winner's hole cards for a hand that already ended and was
// archived — the history counterpart to the live table's
// Table.RequestWinnerCards (engine/hand). See
// docs/specs/2026-08-21-pay-to-see-winner-cards-history.md.
package handreveal

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const tableHandReveals = "poker_hand_reveals"

// PlayerHandCode is one participant's two hole cards in "Ah"/"Tc" notation,
// matching hand.PlayerHandInfo.HoleCards' wire format.
type PlayerHandCode struct {
	Cards [2]string `dynamodbav:"cards"`
}

// HandRecord is the one-per-hand archive of a sandbox hand that ended
// without a showdown with exactly one winner — written once by the
// hand-complete hook and refreshed by the hand-updated hook if the winner
// later voluntarily shows (see api/internal/app/app.go's persistHandReveal).
// Unlike sessionlog.HandItem, this holds every participant's TRUE hole
// cards regardless of whether they were ever shown — nothing except the
// paid-reveal endpoint (api/internal/api/v1/handreveal.go) ever reads it.
type HandRecord struct {
	// PK is hand_id alone, not "table_id#hand_id": hand IDs are already
	// globally unique (sessionlog.Store.GetHand looks one up with no table
	// id), and the buy/check endpoints only ever have :handId from the URL.
	PK          string                    `dynamodbav:"pk"`
	TableID     string                    `dynamodbav:"table_id"`
	HandID      string                    `dynamodbav:"hand_id"`
	BigBlind    int64                     `dynamodbav:"big_blind"`
	WinnerID    string                    `dynamodbav:"winner_id"`
	WinnerShown bool                      `dynamodbav:"winner_shown"`
	PlayerHands map[string]PlayerHandCode `dynamodbav:"player_hands"`
	EndedAt     int64                     `dynamodbav:"ended_at"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableHandReveals)}
}

// Put upserts record, keyed by record.HandID. Called from both the
// hand-complete hook (first write) and the hand-updated hook (refresh
// WinnerShown if the winner later voluntarily shows) — safe to call
// repeatedly with the same HandID.
func (s *Store) Put(ctx context.Context, record HandRecord) error {
	record.PK = record.HandID
	item, err := dynamo.Encode(record)
	if err != nil {
		return fmt.Errorf("handreveal: encode: %w", err)
	}
	if err := s.base.PutItem(ctx, item); err != nil {
		return fmt.Errorf("handreveal: put: %w", err)
	}
	return nil
}

// Get returns nil, nil if no archive exists for handID — a showdown hand, a
// real-money hand, a 2+-winner split, or a hand that predates this feature
// all look identical: nothing was ever written.
func (s *Store) Get(ctx context.Context, handID string) (*HandRecord, error) {
	item, err := s.base.GetItem(ctx, handID)
	if err != nil {
		return nil, fmt.Errorf("handreveal: get: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	record, err := dynamo.Decode[HandRecord](item)
	if err != nil {
		return nil, fmt.Errorf("handreveal: decode: %w", err)
	}
	return record, nil
}
