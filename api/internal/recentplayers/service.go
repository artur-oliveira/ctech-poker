package recentplayers

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

type HistorySource interface {
	ListRecentHandsAcrossModes(context.Context, string, int) ([]sessionlog.HandItem, error)
}

type BlockSource interface {
	BlockedInEitherDirection(context.Context, string, []string) (map[string]bool, error)
}

type Service struct {
	store   Store
	history HistorySource
	blocks  BlockSource
}

func NewService(store Store, history HistorySource, blocks BlockSource) *Service {
	return &Service{store: store, history: history, blocks: blocks}
}

func (s *Service) RecordHand(ctx context.Context, tableID, handID string, playerIDs []string, playedAt time.Time) error {
	return s.store.RecordHand(ctx, HandCompletion{TableID: tableID, HandID: handID, Players: playerIDs, PlayedAt: playedAt})
}

func (s *Service) List(ctx context.Context, viewerID string, startKey map[string]types.AttributeValue, limit int) (Page, error) {
	page, err := s.store.List(ctx, viewerID, startKey, limit)
	if err != nil {
		return Page{}, err
	}
	if len(page.Players) == 0 && len(startKey) == 0 && s.history != nil {
		if bootstrapErr := s.bootstrap(ctx, viewerID); bootstrapErr != nil {
			return Page{}, bootstrapErr
		}
		page, err = s.store.List(ctx, viewerID, startKey, limit)
		if err != nil {
			return Page{}, err
		}
	}
	ids := make([]string, 0, len(page.Players))
	for _, item := range page.Players {
		ids = append(ids, item.OpponentPlayerID)
	}
	blocked := map[string]bool{}
	if s.blocks != nil {
		blocked, err = s.blocks.BlockedInEitherDirection(ctx, viewerID, ids)
		if err != nil {
			return Page{}, err
		}
	}
	filtered := page.Players[:0]
	for _, item := range page.Players {
		if item.OpponentPlayerID != viewerID && !blocked[item.OpponentPlayerID] {
			filtered = append(filtered, item)
		}
	}
	page.Players = filtered
	return page, nil
}

func (s *Service) bootstrap(ctx context.Context, viewerID string) error {
	hands, err := s.history.ListRecentHandsAcrossModes(ctx, viewerID, 100)
	if err != nil {
		return err
	}
	for i := len(hands) - 1; i >= 0; i-- {
		item := hands[i]
		players := []string{viewerID}
		for _, opponent := range item.Opponents {
			players = append(players, opponent.PlayerID)
		}
		if err := s.RecordHand(ctx, item.TableID, item.HandID, players, time.UnixMilli(item.EndedAt)); err != nil {
			return err
		}
	}
	return nil
}
