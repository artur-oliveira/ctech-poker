package responsible

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// walletCarriedPrefix keys a self-exclusion kept past an erased account in the wallet's users
// table (ctech-wallet domain/wallet/erasure.go SelfExclusionCarryPrefix).
const walletCarriedPrefix = "SELFEXCL#"

const walletPageSize = 100

// WalletTable is an Exporter over the wallet's users table, read with the operator's own AWS
// credentials by the one-off import. The wallet's API role is not allowed to Scan (by design),
// so the rows are read here rather than through a wallet route.
type WalletTable struct {
	DB    *dynamodb.Client
	Table string // e.g. prod_wallet_users
}

type walletUserRow struct {
	PK                      string               `dynamodbav:"pk"`
	TermsAddendumVersion    string               `dynamodbav:"terms_addendum_version"`
	TermsAcceptedAt         string               `dynamodbav:"terms_accepted_at"`
	GamblingAddendumVersion string               `dynamodbav:"gambling_addendum_version"`
	GamblingActivatedAt     string               `dynamodbav:"gambling_activated_at"`
	SelfExclusion           *SelfExclusion       `dynamodbav:"self_exclusion"`
	GameLimits              *GameLimits          `dynamodbav:"game_limits"`
	GameDepositCounters     *GameDepositCounters `dynamodbav:"game_deposit_counters"`
	TTL                     int64                `dynamodbav:"ttl"`
}

// ResponsibleExport reads one page. cursor is the opaque continuation of the previous page.
func (w WalletTable) ResponsibleExport(ctx context.Context, cursor string) (*ExportPage, error) {
	start, err := decodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	out, err := w.DB.Scan(ctx, &dynamodb.ScanInput{TableName: aws.String(w.Table), ExclusiveStartKey: start, Limit: aws.Int32(walletPageSize)})
	if err != nil {
		return nil, fmt.Errorf("responsible: scanning %s: %w", w.Table, err)
	}
	page := &ExportPage{Users: []ExportedUser{}, CarriedExclusions: []CarriedExclusion{}}
	for _, item := range out.Items {
		var row walletUserRow
		if err := attributevalue.UnmarshalMap(item, &row); err != nil {
			return nil, fmt.Errorf("responsible: decoding a wallet user row: %w", err)
		}
		if hash, ok := strings.CutPrefix(row.PK, walletCarriedPrefix); ok {
			if row.SelfExclusion != nil {
				page.CarriedExclusions = append(page.CarriedExclusions, CarriedExclusion{CPFHMAC: hash, SelfExclusion: *row.SelfExclusion, ExpiresAt: row.TTL})
			}
			continue
		}
		if row.GamblingAddendumVersion == "" && row.SelfExclusion == nil && row.GameLimits == nil && row.GameDepositCounters == nil {
			continue // nothing to hand over
		}
		page.Users = append(page.Users, ExportedUser{
			UserID: row.PK, TermsAddendumVersion: row.TermsAddendumVersion, TermsAcceptedAt: row.TermsAcceptedAt,
			GamblingAddendumVersion: row.GamblingAddendumVersion, GamblingActivatedAt: row.GamblingActivatedAt,
			SelfExclusion: row.SelfExclusion, GameLimits: row.GameLimits, GameDepositCounters: row.GameDepositCounters,
		})
	}
	if len(out.LastEvaluatedKey) > 0 {
		if page.NextCursor, err = encodeCursor(out.LastEvaluatedKey); err != nil {
			return nil, err
		}
	}
	return page, nil
}

func encodeCursor(key map[string]types.AttributeValue) (string, error) {
	var plain map[string]any
	if err := attributevalue.UnmarshalMap(key, &plain); err != nil {
		return "", err
	}
	raw, err := json.Marshal(plain)
	return base64.StdEncoding.EncodeToString(raw), err
}

func decodeCursor(cursor string) (map[string]types.AttributeValue, error) {
	if cursor == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("responsible: bad cursor: %w", err)
	}
	var plain map[string]any
	if err := json.Unmarshal(raw, &plain); err != nil {
		return nil, fmt.Errorf("responsible: bad cursor: %w", err)
	}
	return attributevalue.MarshalMap(plain)
}
