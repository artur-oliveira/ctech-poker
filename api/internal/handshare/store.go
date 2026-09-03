package handshare

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableHandShares = "poker_hand_shares"
	MaxExpiryDays   = 30

	// attrTokens holds an owner's live share tokens as a DynamoDB string set.
	// poker_hand_shares is a PK-only table (the token IS the key, so a public
	// link resolves in one GetItem), which leaves no room for an owner-keyed
	// query — so Create maintains this one extra row per owner instead.
	//
	// ponytail: O(live shares) GetItems per list call. A share lives at most
	// MaxExpiryDays, so the set stays small; if a player ever accumulates
	// enough shares for that to hurt, the upgrade is a gsi_owner index on
	// this table (a CDK change) rather than a bigger index row.
	attrTokens = "tokens"
	attrTTL    = "ttl"
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
	// The share itself is already live and its link already works; a failed
	// index write only costs the owner a row in the revocation list, so it
	// must not fail the creation the caller just succeeded at.
	if err := s.indexAdd(ctx, share.OwnerID, token); err != nil {
		slog.Warn("handshare: owner index add failed; share will not be listable", "owner", share.OwnerID, "err", err)
	}
	return &share, nil
}

func ownerIndexPK(ownerID string) string { return "owner#" + ownerID }

func (s *Store) indexAdd(ctx context.Context, ownerID, token string) error {
	_, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key:              map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: ownerIndexPK(ownerID)}},
		UpdateExpression: aws.String("SET #ttl = :ttl ADD #tokens :t"),
		ExpressionAttributeNames: map[string]string{
			"#ttl": attrTTL, "#tokens": attrTokens,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			// Refreshed on every add, and always past the longest expiry a
			// share in this set can have, so the row is reaped only once the
			// owner genuinely has nothing left to list.
			":ttl": &types.AttributeValueMemberN{Value: strconv.FormatInt(time.Now().Add(MaxExpiryDays*24*time.Hour).Unix(), 10)},
			":t":   &types.AttributeValueMemberSS{Value: []string{token}},
		},
	})
	return err
}

func (s *Store) indexRemove(ctx context.Context, ownerID string, tokens ...string) error {
	if len(tokens) == 0 {
		return nil
	}
	_, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key:                      map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: ownerIndexPK(ownerID)}},
		UpdateExpression:         aws.String("DELETE #tokens :t"),
		ExpressionAttributeNames: map[string]string{"#tokens": attrTokens},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":t": &types.AttributeValueMemberSS{Value: tokens},
		},
	})
	return err
}

// ListByOwner returns ownerID's still-live shares, newest first — the list
// the revocation UI is built from (#77). Tokens whose share has expired or
// been revoked are dropped from the response and pruned from the index on
// the way out, so the set self-heals without a sweeper.
func (s *Store) ListByOwner(ctx context.Context, ownerID string) ([]Share, error) {
	if strings.TrimSpace(ownerID) == "" {
		return nil, ErrNotOwner
	}
	item, err := s.base.GetItem(ctx, ownerIndexPK(ownerID))
	if err != nil {
		return nil, fmt.Errorf("handshare: get owner index: %w", err)
	}
	set, ok := item[attrTokens].(*types.AttributeValueMemberSS)
	if !ok {
		return []Share{}, nil
	}
	out := make([]Share, 0, len(set.Value))
	var stale []string
	for _, token := range set.Value {
		share, err := s.Get(ctx, token)
		if err != nil {
			return nil, err
		}
		// A token whose share is gone, or (defensively) whose owner does not
		// match, is never returned — Get is the single owner-agnostic reader
		// and this is the only place that fans out over someone else's keys.
		if share == nil || share.OwnerID != ownerID {
			stale = append(stale, token)
			continue
		}
		out = append(out, *share)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	if err := s.indexRemove(ctx, ownerID, stale...); err != nil {
		slog.Warn("handshare: owner index prune failed", "owner", ownerID, "err", err)
	}
	return out, nil
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
	// A leftover token here is harmless (ListByOwner prunes it on the next
	// read), so a failed index update must not report the revocation — which
	// did happen — as a failure.
	if err := s.indexRemove(ctx, ownerID, token); err != nil {
		slog.Warn("handshare: owner index remove failed", "owner", ownerID, "err", err)
	}
	return nil
}
