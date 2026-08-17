package recentplayers

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type Store interface {
	RecordHand(ctx context.Context, hand HandCompletion) error
	List(ctx context.Context, viewerPlayerID string, startKey map[string]types.AttributeValue, limit int) (Page, error)
}
