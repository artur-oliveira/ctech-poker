package v1

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type HistoryAction struct {
	Seq      int    `json:"seq"`
	PlayerID string `json:"player_id"`
	Action   string `json:"action"`
	Amount   int64  `json:"amount"`
}

type historyStore interface {
	LoadActionsSince(ctx context.Context, tableID, handID string, afterSeq int) ([]HistoryAction, error)
}

type tablestoreAdapter struct {
	store *tablestore.Store
}

func (a *tablestoreAdapter) LoadActionsSince(ctx context.Context, tableID, handID string, afterSeq int) ([]HistoryAction, error) {
	entries, err := a.store.LoadActionsSince(ctx, tableID, handID, afterSeq)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryAction, len(entries))
	for i, e := range entries {
		out[i] = HistoryAction{Seq: e.Seq, PlayerID: e.PlayerID, Action: e.Action, Amount: e.Amount}
	}
	return out, nil
}

func RegisterHandHistory(router fiber.Router, auth fiber.Handler, store historyStore) {
	router.Get("/tables/:tableId/hands/:handId/history", auth, func(c fiber.Ctx) error {
		tableID := c.Params("tableId")
		handID := c.Params("handId")
		actions, err := store.LoadActionsSince(c.Context(), tableID, handID, 0)
		if err != nil {
			return problem.InternalServer("failed to load hand history").Send(c)
		}
		return c.JSON(fiber.Map{"table_id": tableID, "hand_id": handID, "actions": actions})
	})
}
