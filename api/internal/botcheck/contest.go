package botcheck

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"

	"uuid"
)

const (
	tableBotCheckContests  = "poker_botcheck_contests"
	MaxContestReasonLength = 500

	// ContestStatusPending is the only status this package ever writes.
	// Moving a contest to "reviewed"/"upheld"/whatever a moderation screen
	// decides is explicitly out of scope here (#322: "decidir o mecanismo é
	// trabalho de UX+segurança, não está definido aqui") — Record only ever
	// creates the auditable claim.
	ContestStatusPending = "pending"
)

var (
	ErrContestPlayerRequired = errors.New("botcheck: player is required")
	ErrContestReasonTooLong  = errors.New("botcheck: contest reason too long")
)

// Contest is a player's auditable claim that a bot-challenge block (a failed
// Verify, or being forced into challengeRequired) was a false positive.
// Recording one NEVER unblocks the player or retroactively accepts a failed
// Turnstile token — Verify's fail-closed result stands regardless. This is a
// completely separate, append-only trail for a human (or a later automated
// pass) to review (#322).
type Contest struct {
	PlayerID  string `dynamodbav:"pk" json:"-"`
	ContestID string `dynamodbav:"sk" json:"contest_id"`
	TableID   string `dynamodbav:"table_id,omitempty" json:"table_id,omitempty"`
	Reason    string `dynamodbav:"reason,omitempty" json:"reason,omitempty"`
	Status    string `dynamodbav:"status" json:"status"`
	CreatedAt string `dynamodbav:"created_at" json:"created_at"`
}

// ContestStore is intentionally separate from Service above: Service.Verify
// is the fail-closed verification path, and nothing in this store ever
// calls Turnstile or changes what Verify accepts. Keeping them as two types
// makes "contesting a block" and "passing verification" impossible to
// accidentally wire into the same call site.
type ContestStore struct{ base dynamo.Base }

func NewContestStore(db *dynamodb.Client, env string) *ContestStore {
	return &ContestStore{base: dynamo.NewBase(db, env, tableBotCheckContests)}
}

// Record persists one contestation. playerID comes from the caller's
// verified JWT, never a client-supplied field (same rule every other
// self-service endpoint in this repo follows).
func (s *ContestStore) Record(ctx context.Context, playerID, tableID, reason string) (*Contest, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return nil, ErrContestPlayerRequired
	}
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > MaxContestReasonLength {
		return nil, ErrContestReasonTooLong
	}
	contest := Contest{
		PlayerID: playerID, ContestID: uuid.New().String(), TableID: strings.TrimSpace(tableID),
		Reason: reason, Status: ContestStatusPending, CreatedAt: dynamo.NowStr(),
	}
	item, err := dynamo.Encode(contest)
	if err != nil {
		return nil, fmt.Errorf("botcheck: encode contest: %w", err)
	}
	if err := s.base.PutItem(ctx, item); err != nil {
		return nil, fmt.Errorf("botcheck: record contest: %w", err)
	}
	return &contest, nil
}

// List answers every contest playerID has filed, most recent last — the
// audit trail a reviewer or a future moderation screen reads.
func (s *ContestStore) List(ctx context.Context, playerID string) ([]Contest, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return nil, ErrContestPlayerRequired
	}
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: playerID, Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("botcheck: list contests: %w", err)
	}
	contests := make([]Contest, 0, len(result.Items))
	for _, item := range result.Items {
		contest, err := dynamo.Decode[Contest](item)
		if err != nil {
			return nil, fmt.Errorf("botcheck: decode contest: %w", err)
		}
		contests = append(contests, *contest)
	}
	return contests, nil
}
