package handshare

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableHandShares = "poker_hand_shares"
	MaxExpiryDays   = 30
)

var (
	ErrInvalidKind   = errors.New("handshare: invalid kind")
	ErrInvalidExpiry = errors.New("handshare: invalid expiry")
	ErrNotOwner      = errors.New("handshare: not owner")
)

type Opponent struct {
	Alias     string   `dynamodbav:"alias" json:"alias"`
	HoleCards []string `dynamodbav:"hole_cards,omitempty" json:"hole_cards,omitempty"`
	Won       bool     `dynamodbav:"won,omitempty" json:"won,omitempty"`
}

type ReplaySeat struct {
	PlayerID    string `dynamodbav:"player_id" json:"player_id"`
	Name        string `dynamodbav:"name" json:"name"`
	Stack       int64  `dynamodbav:"stack" json:"stack"`
	State       string `dynamodbav:"state" json:"state"`
	Contributed int64  `dynamodbav:"contributed" json:"contributed"`
	DealtIn     bool   `dynamodbav:"dealt_in" json:"dealt_in"`
}

type ReplayFrame struct {
	Stage              string           `dynamodbav:"stage" json:"stage"`
	Board              []string         `dynamodbav:"board,omitempty" json:"board,omitempty"`
	Seats              []ReplaySeat     `dynamodbav:"seats,omitempty" json:"seats,omitempty"`
	CurrentPlayerID    string           `dynamodbav:"current_player_id,omitempty" json:"current_player_id,omitempty"`
	DealerPlayerID     string           `dynamodbav:"dealer_player_id,omitempty" json:"dealer_player_id,omitempty"`
	SmallBlindPlayerID string           `dynamodbav:"small_blind_player_id,omitempty" json:"small_blind_player_id,omitempty"`
	BigBlindPlayerID   string           `dynamodbav:"big_blind_player_id,omitempty" json:"big_blind_player_id,omitempty"`
	Pot                int64            `dynamodbav:"pot" json:"pot"`
	Payouts            map[string]int64 `dynamodbav:"payouts,omitempty" json:"payouts,omitempty"`
	Winners            []string         `dynamodbav:"winners,omitempty" json:"winners,omitempty"`
}

type Action struct {
	Seq       int          `dynamodbav:"seq" json:"seq"`
	PlayerID  string       `dynamodbav:"player_id" json:"player_id"`
	Action    string       `dynamodbav:"action" json:"action"`
	Amount    int64        `dynamodbav:"amount" json:"amount"`
	Timestamp int64        `dynamodbav:"timestamp" json:"timestamp"`
	Frame     *ReplayFrame `dynamodbav:"frame,omitempty" json:"frame,omitempty"`
}

type Share struct {
	Token       string     `dynamodbav:"pk" json:"token"`
	OwnerID     string     `dynamodbav:"owner_id" json:"-"`
	Kind        string     `dynamodbav:"kind" json:"kind"`
	Outcome     string     `dynamodbav:"outcome" json:"outcome"`
	NetChange   int64      `dynamodbav:"net_change" json:"net_change"`
	EndedAt     int64      `dynamodbav:"ended_at" json:"ended_at"`
	Board       []string   `dynamodbav:"board,omitempty" json:"board,omitempty"`
	HeroCards   []string   `dynamodbav:"hero_cards,omitempty" json:"hero_cards,omitempty"`
	Opponents   []Opponent `dynamodbav:"opponents,omitempty" json:"opponents,omitempty"`
	Actions     []Action   `dynamodbav:"actions,omitempty" json:"actions,omitempty"`
	CreatedAt   int64      `dynamodbav:"created_at" json:"created_at"`
	ExpiresAt   int64      `dynamodbav:"expires_at" json:"expires_at"`
	TTL         int64      `dynamodbav:"ttl" json:"-"`
	SourceHand  string     `dynamodbav:"source_hand" json:"-"`
	SourceTable string     `dynamodbav:"source_table" json:"-"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableHandShares)}
}

func NewToken() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (s *Store) Create(ctx context.Context, share Share, expiryDays int) (*Share, error) {
	if share.Kind != "brag" && share.Kind != "bad_beat" {
		return nil, ErrInvalidKind
	}
	if expiryDays < 1 || expiryDays > MaxExpiryDays {
		return nil, ErrInvalidExpiry
	}
	if strings.TrimSpace(share.OwnerID) == "" {
		return nil, ErrNotOwner
	}
	token, err := NewToken()
	if err != nil {
		return nil, fmt.Errorf("handshare: token: %w", err)
	}
	now := time.Now()
	share.Token = token
	share.CreatedAt = now.UnixMilli()
	share.ExpiresAt = now.Add(time.Duration(expiryDays) * 24 * time.Hour).UnixMilli()
	share.TTL = now.Add(time.Duration(expiryDays) * 24 * time.Hour).Unix()
	item, err := dynamo.Encode(share)
	if err != nil {
		return nil, fmt.Errorf("handshare: encode: %w", err)
	}
	if err := s.base.PutItem(ctx, item); err != nil {
		return nil, fmt.Errorf("handshare: save: %w", err)
	}
	return &share, nil
}

func (s *Store) Get(ctx context.Context, token string) (*Share, error) {
	item, err := s.base.GetItem(ctx, strings.TrimSpace(token))
	if err != nil || item == nil {
		return nil, err
	}
	share, err := dynamo.Decode[Share](item)
	if err != nil {
		return nil, fmt.Errorf("handshare: decode: %w", err)
	}
	if share.ExpiresAt <= time.Now().UnixMilli() {
		return nil, nil
	}
	return share, nil
}

func (s *Store) Revoke(ctx context.Context, ownerID, token string) error {
	share, err := s.Get(ctx, token)
	if err != nil {
		return err
	}
	if share == nil || share.OwnerID != ownerID {
		return ErrNotOwner
	}
	_, err = s.base.DeleteItem(ctx, token)
	return err
}
