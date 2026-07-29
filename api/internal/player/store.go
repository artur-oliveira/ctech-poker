package player

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const tablePlayerProfiles = "poker_player_profiles"

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tablePlayerProfiles)}
}

func (s *Store) GetOrCreate(ctx context.Context, userID string) (*PlayerProfile, error) {
	if userID == "" {
		return nil, fmt.Errorf("player: empty user id")
	}
	if profile, err := s.get(ctx, userID); err != nil || profile != nil {
		return profile, err
	}
	now := dynamo.NowStr()
	profile := PlayerProfile{UserID: userID, Name: RandomName(), CreatedAt: now, UpdatedAt: now}
	item, err := dynamo.Encode(profile)
	if err != nil {
		return nil, fmt.Errorf("player: encode: %w", err)
	}
	err = s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(item)})
	if err != nil && !dynamo.IsConditionFailed(err) {
		return nil, fmt.Errorf("player: create: %w", err)
	}
	return s.get(ctx, userID)
}

func (s *Store) get(ctx context.Context, userID string) (*PlayerProfile, error) {
	item, err := s.base.GetItem(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("player: get: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	profile, err := dynamo.Decode[PlayerProfile](item)
	if err != nil {
		return nil, fmt.Errorf("player: decode: %w", err)
	}
	return profile, nil
}

// Get loads an existing profile without creating a row for arbitrary public
// showcase lookups.
func (s *Store) Get(ctx context.Context, userID string) (*PlayerProfile, error) {
	if userID == "" {
		return nil, fmt.Errorf("player: empty user id")
	}
	return s.get(ctx, userID)
}

func (s *Store) SetName(ctx context.Context, userID, name string) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	ok, err := s.base.UpdateItem(ctx, userID, nil, map[string]any{
		"name":       name,
		"updated_at": dynamo.NowStr(),
	})
	if err != nil {
		return fmt.Errorf("player: set name: %w", err)
	}
	if !ok {
		return fmt.Errorf("player: profile disappeared while setting name")
	}
	return nil
}

func (s *Store) SetAvatar(ctx context.Context, userID, key string, version int) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	ok, err := s.base.UpdateItem(ctx, userID, nil, map[string]any{
		"avatar_key": key, "avatar_version": version, "avatar_blocked": false, "updated_at": dynamo.NowStr(),
	})
	if err != nil {
		return fmt.Errorf("player: set avatar: %w", err)
	}
	if !ok {
		return fmt.Errorf("player: profile disappeared while setting avatar")
	}
	return nil
}

func (s *Store) ClearAvatar(ctx context.Context, userID string) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	_, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key:              map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: userID}},
		UpdateExpression: aws.String("REMOVE #key SET #blocked = :blocked, #updated = :updated"),
		ExpressionAttributeNames: map[string]string{
			"#key": "avatar_key", "#blocked": "avatar_blocked", "#updated": "updated_at",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":blocked": &types.AttributeValueMemberBOOL{Value: false},
			":updated": &types.AttributeValueMemberS{Value: dynamo.NowStr()},
		},
	})
	if err != nil {
		return fmt.Errorf("player: clear avatar: %w", err)
	}
	return nil
}

// ReportAvatar durably records distinct reporters on the target profile. The
// set is an atomic DynamoDB ADD, so concurrent reports cannot overwrite one
// another and a retry by the same player stays idempotent.
func (s *Store) ReportAvatar(ctx context.Context, targetID, reporterID string) error {
	if targetID == "" || reporterID == "" || targetID == reporterID {
		return fmt.Errorf("player: invalid avatar report")
	}
	profile, err := s.Get(ctx, targetID)
	if err != nil {
		return err
	}
	if profile == nil {
		return fmt.Errorf("player: avatar report target not found")
	}
	_, err = s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key:                      map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: targetID}},
		UpdateExpression:         aws.String("ADD #reporters :reporter SET #updated = :updated"),
		ExpressionAttributeNames: map[string]string{"#reporters": "avatar_reporters", "#updated": "updated_at"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":reporter": &types.AttributeValueMemberSS{Value: []string{reporterID}},
			":updated":  &types.AttributeValueMemberS{Value: dynamo.NowStr()},
		},
	})
	if err != nil {
		return fmt.Errorf("player: report avatar: %w", err)
	}
	return nil
}

func (s *Store) SetWalletMode(ctx context.Context, userID, mode string) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	ok, err := s.base.UpdateItem(ctx, userID, nil, map[string]any{
		"wallet_mode": mode,
		"updated_at":  dynamo.NowStr(),
	})
	if err != nil {
		return fmt.Errorf("player: set wallet mode: %w", err)
	}
	if !ok {
		return fmt.Errorf("player: profile disappeared while setting wallet mode")
	}
	return nil
}

func (s *Store) SetDeckVariant(ctx context.Context, userID, variant string) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	ok, err := s.base.UpdateItem(ctx, userID, nil, map[string]any{
		"deck_variant": variant,
		"updated_at":   dynamo.NowStr(),
	})
	if err != nil {
		return fmt.Errorf("player: set deck variant: %w", err)
	}
	if !ok {
		return fmt.Errorf("player: profile disappeared while setting deck variant")
	}
	return nil
}

func (s *Store) SetShowcase(ctx context.Context, userID string, public, playstylePublic bool, featured []string) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	ok, err := s.base.UpdateItem(ctx, userID, nil, map[string]any{
		"showcase_public":       public,
		"playstyle_public":      playstylePublic,
		"featured_achievements": featured,
		"updated_at":            dynamo.NowStr(),
	})
	if err != nil {
		return fmt.Errorf("player: set showcase: %w", err)
	}
	if !ok {
		return fmt.Errorf("player: profile disappeared while setting showcase")
	}
	return nil
}

func (s *Store) AcceptTerms(ctx context.Context, userID string) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	now := dynamo.NowStr()
	ok, err := s.base.UpdateItem(ctx, userID, nil, map[string]any{
		"poker_terms_version":     CurrentPokerTermsVersion,
		"poker_terms_accepted_at": now,
		"updated_at":              now,
	})
	if err != nil {
		return fmt.Errorf("player: accept terms: %w", err)
	}
	if !ok {
		return fmt.Errorf("player: profile disappeared while accepting terms")
	}
	return nil
}
