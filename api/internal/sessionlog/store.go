package sessionlog

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

// sessionSK builds the sort key for a session item: a nanosecond timestamp
// makes cross-table collisions (two RecordSession calls landing in the same
// millisecond, e.g. multi-tabling) practically impossible — a millisecond-
// only key would let a second table's PutItem silently clobber the first
// table's still-open session item, orphaning it with no way to ever close it.
func sessionSK() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

const (
	tableSessions = "poker_player_sessions"
	tableHands    = "poker_player_hands"
)

type SessionItem struct {
	PK            string `dynamodbav:"pk" json:"pk"` // player_id
	SK            string `dynamodbav:"sk" json:"sk"` // timestamp / session_id
	TableID       string `dynamodbav:"table_id" json:"table_id"`
	BuyinAmount   int64  `dynamodbav:"buyin_amount" json:"buyin_amount"`
	CashoutAmount int64  `dynamodbav:"cashout_amount" json:"cashout_amount"`
	NetPnL        int64  `dynamodbav:"net_pnl" json:"net_pnl"`
	JoinedAt      int64  `dynamodbav:"joined_at" json:"joined_at"`
	EndedAt       int64  `dynamodbav:"ended_at" json:"ended_at"`
}

type HandItem struct {
	PK        string            `dynamodbav:"pk" json:"pk"` // player_id
	SK        string            `dynamodbav:"sk" json:"sk"` // timestamp / hand_id
	TableID   string            `dynamodbav:"table_id" json:"table_id"`
	HandID    string            `dynamodbav:"hand_id" json:"hand_id"`
	Outcome   string            `dynamodbav:"outcome" json:"outcome"`
	NetChange int64             `dynamodbav:"net_change" json:"net_change"`
	EndedAt   int64             `dynamodbav:"ended_at" json:"ended_at"`
	Board     []string          `dynamodbav:"board,omitempty" json:"board,omitempty"`
	HoleCards []string          `dynamodbav:"hole_cards,omitempty" json:"hole_cards,omitempty"`
	Opponents []OpponentSummary `dynamodbav:"opponents,omitempty" json:"opponents,omitempty"`
}

// OpponentSummary is one other participant of a recorded hand, for a
// player's own match-history view. HoleCards is only populated when that
// opponent's hand was actually shown to the table (showdown or voluntary
// show) — never a folded hand, matching hand.PlayerHandInfo.Revealed.
type OpponentSummary struct {
	PlayerID  string   `dynamodbav:"player_id" json:"player_id"`
	Name      string   `dynamodbav:"name,omitempty" json:"name,omitempty"`
	HoleCards []string `dynamodbav:"hole_cards,omitempty" json:"hole_cards,omitempty"`
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
		item.SK = sessionSK()
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

// FindOpenSession returns the most recent session recorded for playerID at
// tableID that has not yet been closed (EndedAt == 0), or nil if none exists.
// Pages through the player's ENTIRE session partition (filtered server-side
// to tableID) rather than a single capped page — a player who has since
// opened sessions at other tables (multi-tabling, or just a lot of history)
// would otherwise push this table's still-open session past a fixed-size
// "most recent N" window, leaving it stuck open (ended_at never set) forever.
func (s *Store) FindOpenSession(ctx context.Context, playerID, tableID string) (*SessionItem, error) {
	var startKey map[string]dynamotypes.AttributeValue
	for {
		res, err := s.sessions.Query(ctx, dynamo.QueryOpts{
			PK: playerID, Limit: 50, ScanIndexForward: false,
			FilterField: "table_id", FilterValue: tableID,
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			return nil, err
		}
		for _, raw := range res.Items {
			item, err := dynamo.Decode[SessionItem](raw)
			if err == nil && item != nil && item.EndedAt == 0 {
				return item, nil
			}
		}
		if res.LastEvaluatedKey == nil {
			return nil, nil
		}
		startKey = res.LastEvaluatedKey
	}
}

// CloseSession overwrites the same session item (same PK/SK) with its final
// EndedAt/CashoutAmount/NetPnL — a plain PutItem, since this is an audit trail,
// not the wallet balance's source of truth (that stays ctech-wallet).
func (s *Store) CloseSession(ctx context.Context, item SessionItem) error {
	return s.RecordSession(ctx, item)
}

func (s *Store) RecordHand(ctx context.Context, item HandItem) error {
	if item.SK == "" {
		item.SK = fmt.Sprintf("%d#%s", time.Now().UnixNano(), item.HandID)
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
