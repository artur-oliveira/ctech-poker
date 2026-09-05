package reports

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tablePlayerReports = "poker_player_reports"
	gsiReportStatus    = "gsi_status"
	gsiReporter        = "gsi_reporter"
	resolvedRetention  = 180 * 24 * time.Hour
)

var (
	ErrNotFound          = errors.New("reports: report not found")
	ErrInvalidTransition = errors.New("reports: invalid status transition")
)

type Store interface {
	Create(ctx context.Context, report Report, idempotencyKey string) (*Report, bool, error)
	Get(ctx context.Context, targetPlayerID, storageKey string) (*Report, error)
	ListByStatus(ctx context.Context, status Status, cursor string, limit int) (Page, error)
	ListByReporter(ctx context.Context, reporterID, cursor string, limit int) (Page, error)
	SetStatus(ctx context.Context, targetPlayerID, storageKey string, status Status, resolution Resolution, moderatorID string) error
}

type DynamoStore struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *DynamoStore {
	return &DynamoStore{base: dynamo.NewBase(db, env, tablePlayerReports)}
}

func reportIdentity(reporterID, targetID, idempotencyKey string) (string, string) {
	sum := sha256.Sum256([]byte(reporterID + "\x00" + targetID + "\x00" + idempotencyKey))
	hash := hex.EncodeToString(sum[:16])
	return hash, reporterID + "#" + hash
}

func (s *DynamoStore) Create(ctx context.Context, report Report, idempotencyKey string) (*Report, bool, error) {
	report.ReportID, report.StorageKey = reportIdentity(report.ReporterPlayerID, report.TargetPlayerID, idempotencyKey)
	if report.Status == "" {
		report.Status = StatusOpen
	}
	if report.CreatedAt == 0 {
		report.CreatedAt = time.Now().UTC().UnixMilli()
	}
	report.StatusPartition = string(report.Status)
	report.StatusSort = fmt.Sprintf("%020d#%s", report.CreatedAt, report.ReportID)
	report.ReporterPartition = report.ReporterPlayerID
	report.ReporterSort = fmt.Sprintf("%020d#%s", report.CreatedAt, report.ReportID)
	item, err := attributevalue.MarshalMap(report)
	if err != nil {
		return nil, false, fmt.Errorf("reports: encode: %w", err)
	}
	_, err = s.base.PutItemRaw(ctx, &dynamodb.PutItemInput{Item: item,
		ConditionExpression:                 aws.String("attribute_not_exists(pk) AND attribute_not_exists(sk)"),
		ReturnValuesOnConditionCheckFailure: types.ReturnValuesOnConditionCheckFailureAllOld})
	if err == nil {
		return &report, true, nil
	}
	var condition *types.ConditionalCheckFailedException
	if !errors.As(err, &condition) {
		return nil, false, fmt.Errorf("reports: create: %w", err)
	}
	if len(condition.Item) > 0 {
		existing, decodeErr := dynamo.Decode[Report](condition.Item)
		if decodeErr != nil {
			return nil, false, fmt.Errorf("reports: decode idempotent result: %w", decodeErr)
		}
		return existing, false, nil
	}
	existing, getErr := s.Get(ctx, report.TargetPlayerID, report.StorageKey)
	return existing, false, getErr
}

func (s *DynamoStore) Get(ctx context.Context, targetPlayerID, storageKey string) (*Report, error) {
	item, err := s.base.GetItem(ctx, targetPlayerID, storageKey)
	if err != nil {
		return nil, fmt.Errorf("reports: get: %w", err)
	}
	if item == nil {
		return nil, ErrNotFound
	}
	report, err := dynamo.Decode[Report](item)
	if err != nil {
		return nil, fmt.Errorf("reports: decode: %w", err)
	}
	return report, nil
}

type cursorPayload map[string]any

func encodeCursor(key map[string]types.AttributeValue) string {
	if len(key) == 0 {
		return ""
	}
	var plain map[string]any
	if attributevalue.UnmarshalMap(key, &plain) != nil {
		return ""
	}
	raw, err := json.Marshal(cursorPayload(plain))
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(cursor string) map[string]types.AttributeValue {
	if cursor == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil
	}
	var plain cursorPayload
	if json.Unmarshal(raw, &plain) != nil {
		return nil
	}
	key, err := attributevalue.MarshalMap(map[string]any(plain))
	if err != nil {
		return nil
	}
	return key
}

func (s *DynamoStore) ListByStatus(ctx context.Context, status Status, cursor string, limit int) (Page, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	out, err := s.base.QueryRaw(ctx, &dynamodb.QueryInput{
		IndexName: aws.String(gsiReportStatus), KeyConditionExpression: aws.String("gsi_status_pk = :status"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":status": &types.AttributeValueMemberS{Value: string(status)}},
		ScanIndexForward:          aws.Bool(true), Limit: aws.Int32(int32(limit)), ExclusiveStartKey: decodeCursor(cursor),
	})
	if err != nil {
		return Page{}, fmt.Errorf("reports: list by status: %w", err)
	}
	page := Page{Reports: make([]Report, 0, len(out.Items)), NextCursor: encodeCursor(out.LastEvaluatedKey)}
	for _, item := range out.Items {
		report, decodeErr := dynamo.Decode[Report](item)
		if decodeErr != nil {
			return Page{}, fmt.Errorf("reports: decode list item: %w", decodeErr)
		}
		page.Reports = append(page.Reports, *report)
	}
	return page, nil
}

// ListByReporter answers "what did I file?" for a player — newest first —
// via the gsi_reporter sparse GSI populated by every Create call. Never used
// for moderation review; that stays on ListByStatus/gsi_status.
func (s *DynamoStore) ListByReporter(ctx context.Context, reporterID, cursor string, limit int) (Page, error) {
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	out, err := s.base.QueryRaw(ctx, &dynamodb.QueryInput{
		IndexName: aws.String(gsiReporter), KeyConditionExpression: aws.String("gsi_reporter_pk = :reporter"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":reporter": &types.AttributeValueMemberS{Value: reporterID}},
		ScanIndexForward:          aws.Bool(false), Limit: aws.Int32(int32(limit)), ExclusiveStartKey: decodeCursor(cursor), // lgtm[go/incorrect-integer-conversion] -- limit clamped to [1,100] above
	})
	if err != nil {
		return Page{}, fmt.Errorf("reports: list by reporter: %w", err)
	}
	page := Page{Reports: make([]Report, 0, len(out.Items)), NextCursor: encodeCursor(out.LastEvaluatedKey)}
	for _, item := range out.Items {
		report, decodeErr := dynamo.Decode[Report](item)
		if decodeErr != nil {
			return Page{}, fmt.Errorf("reports: decode list item: %w", decodeErr)
		}
		page.Reports = append(page.Reports, *report)
	}
	return page, nil
}

func (s *DynamoStore) SetStatus(ctx context.Context, targetPlayerID, storageKey string, status Status, resolution Resolution, moderatorID string) error {
	if targetPlayerID == "" || storageKey == "" || moderatorID == "" {
		return ErrInvalidTransition
	}
	now := time.Now().UTC()
	values := map[string]types.AttributeValue{
		":open":      &types.AttributeValueMemberS{Value: string(StatusOpen)},
		":reviewing": &types.AttributeValueMemberS{Value: string(StatusReviewing)},
		":status":    &types.AttributeValueMemberS{Value: string(status)},
		":moderator": &types.AttributeValueMemberS{Value: moderatorID},
	}
	expression := ""
	condition := "#status = :open OR #status = :reviewing"
	if status == StatusResolved {
		if !ValidResolution(resolution) {
			return ErrInvalidTransition
		}
		values[":resolution"] = &types.AttributeValueMemberS{Value: string(resolution)}
		values[":resolved"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(now.UnixMilli(), 10)}
		values[":ttl"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(resolvedRetention).Unix(), 10)}
		expression = "SET #status = :status, gsi_status_pk = :status, resolution = :resolution, resolved_at = :resolved, resolved_by = :moderator, #ttl = :ttl"
	} else if status != StatusReviewing || resolution != "" {
		return ErrInvalidTransition
	} else {
		values[":reviewed"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(now.UnixMilli(), 10)}
		expression = "SET #status = :status, gsi_status_pk = :status, reviewed_by = :moderator, reviewed_at = :reviewed REMOVE resolution, resolved_at, resolved_by, #ttl"
	}
	_, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key:              map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: targetPlayerID}, "sk": &types.AttributeValueMemberS{Value: storageKey}},
		UpdateExpression: aws.String(expression), ConditionExpression: aws.String(condition),
		ExpressionAttributeNames: map[string]string{"#status": "status", "#ttl": "ttl"}, ExpressionAttributeValues: values,
	})
	if err != nil {
		if dynamo.IsConditionFailed(err) {
			return ErrInvalidTransition
		}
		return fmt.Errorf("reports: set status: %w", err)
	}
	return nil
}

func ValidStatus(status Status) bool {
	return status == StatusOpen || status == StatusReviewing || status == StatusResolved
}
func ValidResolution(value Resolution) bool {
	return value == ResolutionNoAction || value == ResolutionContentRemoved || value == ResolutionWarningRequested || value == ResolutionSuspensionRequested
}
