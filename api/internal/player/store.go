package player

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tablePlayerProfiles = "poker_player_profiles"
	friendCodeIndex     = "gsi_friend_code"

	// MaxBatchProfileIDs is the largest player-id set GetMany accepts in one
	// call — DynamoDB's BatchGetItem caps a single RequestItems entry at 100
	// keys, so a caller with a larger set (e.g. read-time avatar/name
	// resolution over a big hand-history page) must chunk into batches of at
	// most this size rather than pass them all through in one call.
	MaxBatchProfileIDs = 100

	// attrAvatarReportCount is the profile's aggregated avatar-report tally.
	// It replaces avatar_reporters, a String Set of every distinct reporter
	// that nothing ever read and that grew without bound on an item
	// Get/GetOrCreate/GetMany load on hot paths — a profile could reach the
	// 400 KB item limit and start rejecting legitimate updates (#220). A
	// number cannot inflate the item, and the reporters themselves are
	// already recorded, one row each, in the moderation queue
	// (internal/reports, poker_player_reports).
	attrAvatarReportCount = "avatar_report_count"
	attrAvatarReporters   = "avatar_reporters"

	// AvatarReportCap stops the tally climbing forever on a target under a
	// sustained campaign: past it the counter simply stops moving (every
	// individual report is still recorded in poker_player_reports), so both
	// the profile item's size and the report's write cost stay flat.
	AvatarReportCap = 10_000
)

var ErrFriendCodeCollision = errors.New("player: friend code collision")

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tablePlayerProfiles)}
}

func (s *Store) GetOrCreate(ctx context.Context, userID string) (*PlayerProfile, error) {
	if userID == "" {
		return nil, fmt.Errorf("player: empty user id")
	}
	if profile, err := s.get(ctx, userID); err != nil {
		return nil, err
	} else if profile != nil {
		if profile.FriendCode != "" {
			return profile, nil
		}
		if err := s.ensureFriendCode(ctx, userID); err != nil {
			return nil, err
		}
		return s.get(ctx, userID)
	}
	now := dynamo.NowStr()
	profile := PlayerProfile{UserID: userID, Name: RandomName(), FriendCode: FriendCodeForUserID(userID), CreatedAt: now, UpdatedAt: now}
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

// FriendCodeForUserID returns a stable 60-bit identifier. It is intentionally
// derived from the opaque account ID, never from mutable profile data.
func FriendCodeForUserID(userID string) string {
	digest := sha256.Sum256([]byte(userID))
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest[:])[:12]
	return "PKR-" + encoded[:4] + "-" + encoded[4:8] + "-" + encoded[8:]
}

func NormalizeFriendCode(raw string) (string, bool) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if len(code) != len("PKR-XXXX-XXXX-XXXX") || !strings.HasPrefix(code, "PKR-") {
		return "", false
	}
	for i, r := range code {
		if i == 3 || i == 8 || i == 13 {
			if r != '-' {
				return "", false
			}
			continue
		}
		if i < 4 {
			continue
		}
		if !(r >= 'A' && r <= 'Z') && !(r >= '2' && r <= '7') {
			return "", false
		}
	}
	return code, true
}

func (s *Store) ensureFriendCode(ctx context.Context, userID string) error {
	_, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key:                 map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: userID}},
		UpdateExpression:    aws.String("SET #code = :code, #updated = :updated"),
		ConditionExpression: aws.String("attribute_exists(pk) AND attribute_not_exists(#code)"),
		ExpressionAttributeNames: map[string]string{
			"#code": "friend_code", "#updated": "updated_at",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":code":    &types.AttributeValueMemberS{Value: FriendCodeForUserID(userID)},
			":updated": &types.AttributeValueMemberS{Value: dynamo.NowStr()},
		},
	})
	if err != nil && !dynamo.IsConditionFailed(err) {
		return fmt.Errorf("player: assign friend code: %w", err)
	}
	return nil
}

// LookupByFriendCode resolves only the canonical exact code. Returning a
// collision error instead of an arbitrary account keeps discovery fail-safe.
func (s *Store) LookupByFriendCode(ctx context.Context, raw string) (*PlayerProfile, error) {
	code, ok := NormalizeFriendCode(raw)
	if !ok {
		return nil, nil
	}
	out, err := s.base.QueryRaw(ctx, &dynamodb.QueryInput{
		IndexName:              aws.String(friendCodeIndex),
		KeyConditionExpression: aws.String("friend_code = :code"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":code": &types.AttributeValueMemberS{Value: code},
		},
		Limit: aws.Int32(2),
	})
	if err != nil {
		return nil, fmt.Errorf("player: lookup friend code: %w", err)
	}
	if len(out.Items) > 1 {
		return nil, ErrFriendCodeCollision
	}
	if len(out.Items) == 0 {
		return nil, nil
	}
	profile, err := dynamo.Decode[PlayerProfile](out.Items[0])
	if err != nil {
		return nil, fmt.Errorf("player: decode friend-code lookup: %w", err)
	}
	return profile, nil
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

func (s *Store) GetMany(ctx context.Context, userIDs []string) (map[string]PlayerProfile, error) {
	result := make(map[string]PlayerProfile, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	if len(userIDs) > MaxBatchProfileIDs {
		return nil, fmt.Errorf("player: batch profile limit exceeded")
	}
	keys := make([]map[string]types.AttributeValue, 0, len(userIDs))
	seen := make(map[string]bool, len(userIDs))
	for _, userID := range userIDs {
		if userID == "" || seen[userID] {
			continue
		}
		seen[userID] = true
		keys = append(keys, map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: userID}})
	}
	request := map[string]types.KeysAndAttributes{s.base.TableName: {Keys: keys}}
	for attempt := 0; attempt < 4 && len(request) > 0; attempt++ {
		out, err := s.base.BatchGetItemRaw(ctx, &dynamodb.BatchGetItemInput{RequestItems: request})
		if err != nil {
			return nil, fmt.Errorf("player: batch get profiles: %w", err)
		}
		for _, item := range out.Responses[s.base.TableName] {
			profile, err := dynamo.Decode[PlayerProfile](item)
			if err != nil {
				return nil, fmt.Errorf("player: decode batch profile: %w", err)
			}
			result[profile.UserID] = *profile
		}
		request = out.UnprocessedKeys
	}
	if len(request) > 0 {
		return nil, fmt.Errorf("player: batch get profiles remained unprocessed")
	}
	return result, nil
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

// avatarReportGuardPK is the one-row-per-target-and-reporter idempotency
// guard that keeps the aggregate honest. It lives in this PK-only table
// beside the profiles: profile reads are exact-key GetItem / BatchGetItem, so
// a guard row is never in their way, and the sparse gsi_friend_code index
// ignores it (it carries no friend_code).
func avatarReportGuardPK(targetID, reporterID string) string {
	return "avreport#" + targetID + "#" + reporterID
}

// ReportAvatar counts one distinct reporter against a target's avatar. The
// distinct rule is a conditional put of the guard row, so a retry (or a
// player clicking twice) is a no-op instead of a second count; the tally
// itself is an atomic ADD, so concurrent reports cannot overwrite one
// another. Neither write can grow the profile item — that is the point of
// #220 — and the same update drops the legacy avatar_reporters set from any
// profile still carrying one.
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
	now := dynamo.NowStr()
	_, err = s.base.PutItemRaw(ctx, &dynamodb.PutItemInput{
		Item: map[string]types.AttributeValue{
			"pk":          &types.AttributeValueMemberS{Value: avatarReportGuardPK(targetID, reporterID)},
			"target_id":   &types.AttributeValueMemberS{Value: targetID},
			"reporter_id": &types.AttributeValueMemberS{Value: reporterID},
			"created_at":  &types.AttributeValueMemberS{Value: now},
		},
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	})
	if dynamo.IsConditionFailed(err) {
		// This reporter already reported this target: idempotent success.
		return nil
	}
	if err != nil {
		return fmt.Errorf("player: report avatar guard: %w", err)
	}
	_, err = s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key:              map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: targetID}},
		UpdateExpression: aws.String("SET #updated = :updated ADD #count :one REMOVE #reporters"),
		// The cap is a condition, not a clamp, so the counter is never
		// read-then-written; reaching it just leaves the tally alone.
		ConditionExpression: aws.String("attribute_exists(pk) AND (attribute_not_exists(#count) OR #count < :cap)"),
		ExpressionAttributeNames: map[string]string{
			"#count": attrAvatarReportCount, "#reporters": attrAvatarReporters, "#updated": "updated_at",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":one":     &types.AttributeValueMemberN{Value: "1"},
			":cap":     &types.AttributeValueMemberN{Value: strconv.Itoa(AvatarReportCap)},
			":updated": &types.AttributeValueMemberS{Value: now},
		},
	})
	if err != nil && !dynamo.IsConditionFailed(err) {
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

func (s *Store) SetTableTheme(ctx context.Context, userID, theme string) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	ok, err := s.base.UpdateItem(ctx, userID, nil, map[string]any{
		"table_theme": theme,
		"updated_at":  dynamo.NowStr(),
	})
	if err != nil {
		return fmt.Errorf("player: set table theme: %w", err)
	}
	if !ok {
		return fmt.Errorf("player: profile disappeared while setting table theme")
	}
	return nil
}

func (s *Store) SetShowcase(ctx context.Context, userID string, public, playstylePublic, tablePublic bool, featured []string) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	ok, err := s.base.UpdateItem(ctx, userID, nil, map[string]any{
		"showcase_public":       public,
		"playstyle_public":      playstylePublic,
		"table_public":          tablePublic,
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

func (s *Store) SetFavoriteReactions(ctx context.Context, userID string, favorites []string) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	ok, err := s.base.UpdateItem(ctx, userID, nil, map[string]any{
		"favorite_reactions": favorites,
		"updated_at":         dynamo.NowStr(),
	})
	if err != nil {
		return fmt.Errorf("player: set favorite reactions: %w", err)
	}
	if !ok {
		return fmt.Errorf("player: profile disappeared while setting favorite reactions")
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
