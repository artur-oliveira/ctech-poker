package recentplayers

import "context"

type Store interface {
	RecordHand(ctx context.Context, hand HandCompletion) error
	List(ctx context.Context, viewerPlayerID, cursor string, limit int) (Page, error)
}
