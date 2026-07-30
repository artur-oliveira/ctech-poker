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
	// Reaction rows only. Cosmetic and already public at the table, so replaying
	// them exposes nothing new; without these the replay could name the action
	// but not show which emoji was thrown. Chat Message is deliberately NOT
	// projected here: table talk is not part of a hand's public record.
	ReactionID     string                  `json:"reaction_id,omitempty"`
	TargetPlayerID string                  `json:"target_player_id,omitempty"`
	Timestamp      int64                   `json:"timestamp"` // unix millis, from ActionLogEntry.Timestamp
	Frame          *tablestore.ReplayFrame `json:"frame,omitempty"`
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
		out[i] = HistoryAction{
			Seq: e.Seq, PlayerID: e.PlayerID, Action: e.Action, Amount: e.Amount,
			ReactionID: e.ReactionID, TargetPlayerID: e.TargetPlayerID,
			Timestamp: e.Timestamp, Frame: e.Frame,
		}
	}
	return out, nil
}

type handHistoryHandlers struct{ store historyStore }

func RegisterHandHistory(router fiber.Router, auth fiber.Handler, store historyStore) {
	h := &handHistoryHandlers{store: store}
	router.Get("/tables/:tableId/hands/:handId/history", auth, h.history)
}

func (h *handHistoryHandlers) history(c fiber.Ctx) error {
	tableID := c.Params("tableId")
	handID := c.Params("handId")
	actions, err := h.store.LoadActionsSince(c.Context(), tableID, handID, 0)
	if err != nil {
		return problem.InternalServer("failed to load hand history", c, err).Send(c)
	}
	return c.JSON(fiber.Map{"table_id": tableID, "hand_id": handID, "actions": actions})
}
