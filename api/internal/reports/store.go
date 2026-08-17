package reports

import "context"

type Store interface {
	Create(ctx context.Context, report Report, idempotencyKey string) (*Report, error)
	Get(ctx context.Context, targetPlayerID, storageKey string) (*Report, error)
	ListByStatus(ctx context.Context, status Status, cursor string, limit int) (Page, error)
	SetStatus(ctx context.Context, targetPlayerID, storageKey string, status Status, resolution, moderatorID string) error
}
