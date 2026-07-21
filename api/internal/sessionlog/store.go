package sessionlog

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableSessions = "poker_player_sessions"
	tableHands    = "poker_player_hands"
)

type SessionItem struct {
	PK            string `dynamodbav:"pk"` // player_id
	SK            string `dynamodbav:"sk"` // timestamp / session_id
	TableID       string `dynamodbav:"table_id"`
	BuyinAmount   int64  `dynamodbav:"buyin_amount"`
	CashoutAmount int64  `dynamodbav:"cashout_amount"`
	NetPnL        int64  `dynamodbav:"net_pnl"`
	JoinedAt      int64  `dynamodbav:"joined_at"`
	EndedAt       int64  `dynamodbav:"ended_at"`
}

type HandItem struct {
	PK        string `dynamodbav:"pk"` // player_id
	SK        string `dynamodbav:"sk"` // timestamp / hand_id
	TableID   string `dynamodbav:"table_id"`
	HandID    string `dynamodbav:"hand_id"`
	Outcome   string `dynamodbav:"outcome"`
	NetChange int64  `dynamodbav:"net_change"`
	EndedAt   int64  `dynamodbav:"ended_at"`
}

type Store struct {
	sessions dynamo.Base
	hands    dynamo.Base
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{
		sessions: dynamo.NewBase(db, env, tableSessions),
		hands:    dynamo.NewBase(db, env, tableHands),
	}
}

func (s *Store) RecordSession(ctx context.Context, item SessionItem) error {
	if item.SK == "" {
		item.SK = fmt.Sprintf("%d", time.Now().UnixMilli())
	}
	encoded, err := dynamo.Encode(item)
	if err != nil {
		return err
	}
	return s.sessions.PutItem(ctx, encoded)
}

func (s *Store) ListSessions(ctx context.Context, playerID string, limit int) ([]SessionItem, error) {
	if limit <= 0 {
		limit = 50
	}
	res, err := s.sessions.Query(ctx, dynamo.QueryOpts{PK: playerID, Limit: limit, ScanIndexForward: false})
	if err != nil {
		return nil, err
	}
	out := make([]SessionItem, 0, len(res.Items))
	for _, raw := range res.Items {
		item, err := dynamo.Decode[SessionItem](raw)
		if err == nil && item != nil {
			out = append(out, *item)
		}
	}
	return out, nil
}

func (s *Store) RecordHand(ctx context.Context, item HandItem) error {
	if item.SK == "" {
		item.SK = fmt.Sprintf("%d#%s", time.Now().UnixMilli(), item.HandID)
	}
	encoded, err := dynamo.Encode(item)
	if err != nil {
		return err
	}
	return s.hands.PutItem(ctx, encoded)
}

func (s *Store) ListHands(ctx context.Context, playerID string, limit int) ([]HandItem, error) {
	if limit <= 0 {
		limit = 50
	}
	res, err := s.hands.Query(ctx, dynamo.QueryOpts{PK: playerID, Limit: limit, ScanIndexForward: false})
	if err != nil {
		return nil, err
	}
	out := make([]SessionItem, 0, len(res.Items))
	_ = out
	outHands := make([]HandItem, 0, len(res.Items))
	for _, raw := range res.Items {
		item, err := dynamo.Decode[HandItem](raw)
		if err == nil && item != nil {
			outHands = append(outHands, *item)
		}
	}
	return outHands, nil
}
