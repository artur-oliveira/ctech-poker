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
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableHandShares = "poker_hand_shares"
	MaxExpiryDays   = 30

	// ownerIndex is the sparse GSI that answers "which shares does this owner
	// still have?" in one Query. poker_hand_shares stays PK-only for the
	// public read (the token IS the key, so a public link resolves in one
	// GetItem); the index only adds the owner-keyed view the revocation UI
	// needs, sorted by creation time so newest-first paging is the index
	// order rather than an in-memory sort over everything the owner owns.
	ownerIndex       = "gsi_owner"
	attrOwnerID      = "owner_id"
	attrCreatedAt    = "created_at"
	MaxListPageSize  = 100
	defaultListLimit = 50
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
	Token     string `dynamodbav:"pk" json:"token"`
	OwnerID   string `dynamodbav:"owner_id" json:"-"`
	Kind      string `dynamodbav:"kind" json:"kind"`
	Outcome   string `dynamodbav:"outcome" json:"outcome"`
	NetChange int64  `dynamodbav:"net_change" json:"net_change"`
	EndedAt   int64  `dynamodbav:"ended_at" json:"ended_at"`
	// SmallBlind / BigBlind are the blind level the shared hand was played at
	// (#75). Zero means unknown (a hand recorded before sessionlog stored it
	// and whose replay frames carried no blind either) — the public replayer
	// must hide the blind marker rather than assume a default.
	SmallBlind  int64      `dynamodbav:"small_blind,omitempty" json:"small_blind,omitempty"`
	BigBlind    int64      `dynamodbav:"big_blind,omitempty" json:"big_blind,omitempty"`
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

// ListByOwner returns one page of ownerID's still-live shares, newest first —
// the list the revocation UI is built from (#77). It is a single Query on
// gsi_owner: no per-token GetItem fan-out, and no write, so listing costs
// exactly one read regardless of how many shares the owner has ever made.
//
// Shares past their expiry are skipped in-page (DynamoDB's TTL sweep is
// eventual, so an expired row can still be in the index), which means a page
// can come back shorter than limit while nextKey is non-nil — the caller
// pages until nextKey is nil, it never infers "done" from a short page.
func (s *Store) ListByOwner(ctx context.Context, ownerID string, limit int, startKey map[string]types.AttributeValue) ([]Share, map[string]types.AttributeValue, error) {
	if strings.TrimSpace(ownerID) == "" {
		return nil, nil, ErrNotOwner
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > MaxListPageSize {
		limit = MaxListPageSize
	}
	result, err := s.base.Query(ctx, dynamo.QueryOpts{
		PK: ownerID, PKField: attrOwnerID, SKField: attrCreatedAt,
		// Descending on created_at: newest first is the index order.
		IndexName: ownerIndex, ScanIndexForward: false, Limit: limit, ExclusiveStartKey: startKey,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("handshare: list by owner: %w", err)
	}
	now := time.Now().UnixMilli()
	out := make([]Share, 0, len(result.Items))
	for _, item := range result.Items {
		share, err := dynamo.Decode[Share](item)
		if err != nil {
			return nil, nil, fmt.Errorf("handshare: decode: %w", err)
		}
		if share.ExpiresAt <= now {
			continue
		}
		out = append(out, *share)
	}
	return out, result.LastEvaluatedKey, nil
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
	if _, err = s.base.DeleteItem(ctx, token); err != nil {
		return err
	}
	return nil
}
