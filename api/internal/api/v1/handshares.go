package v1

import (
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/handshare"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

const maxSharedActions = 200

type createHandShareRequest struct {
	Kind             string `json:"kind"`
	IncludeHeroCards bool   `json:"include_hero_cards"`
	ExpiryDays       int    `json:"expiry_days"`
}

type handShareHandlers struct {
	hands  sessionLogReader
	logs   *tablestore.Store
	shares *handshare.Store
}

func RegisterHandShares(router fiber.Router, auth fiber.Handler, hands sessionLogReader, logs *tablestore.Store, shares *handshare.Store) {
	h := &handShareHandlers{hands: hands, logs: logs, shares: shares}
	router.Get("/hand-shares/:token", h.public)
	g := router.Group("/players/me", auth)
	g.Post("/hands/:handId/share", h.create)
	g.Delete("/hand-shares/:token", h.revoke)
}

func (h *handShareHandlers) create(c fiber.Ctx) error {
	var req createHandShareRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	if req.ExpiryDays == 0 {
		req.ExpiryDays = 7
	}
	ownerID := c.Locals(localsUserID).(string)
	source, err := h.hands.GetHand(c.Context(), ownerID, c.Params("handId"))
	if err != nil {
		return problem.InternalServer("failed to load hand", c, err).Send(c)
	}
	if source == nil {
		return problem.NotFound("hand not found").Send(c)
	}
	entries, err := h.logs.LoadActionsSince(c.Context(), source.TableID, source.HandID, 0)
	if err != nil {
		return problem.InternalServer("failed to load replay frames", c, err).Send(c)
	}
	if len(entries) > maxSharedActions {
		entries = entries[len(entries)-maxSharedActions:]
	}
	aliases := aliasesFor(source, ownerID)
	share := handshare.Share{
		OwnerID: ownerID, Kind: req.Kind, Outcome: source.Outcome, NetChange: source.NetChange,
		EndedAt: source.EndedAt, Board: append([]string(nil), source.Board...),
		Opponents: anonymizedOpponents(source), Actions: anonymizedActions(entries, aliases),
		SourceHand: source.HandID, SourceTable: source.TableID,
	}
	if req.IncludeHeroCards {
		share.HeroCards = append([]string(nil), source.HoleCards...)
	}
	created, err := h.shares.Create(c.Context(), share, req.ExpiryDays)
	switch {
	case errors.Is(err, handshare.ErrInvalidKind):
		return problem.BadRequest("kind must be brag or bad_beat").Send(c)
	case errors.Is(err, handshare.ErrInvalidExpiry):
		return problem.BadRequest("expiry_days must be between 1 and 30").Send(c)
	case err != nil:
		return problem.InternalServer("failed to create hand share", c, err).Send(c)
	default:
		return c.Status(fiber.StatusCreated).JSON(created)
	}
}

func (h *handShareHandlers) public(c fiber.Ctx) error {
	share, err := h.shares.Get(c.Context(), c.Params("token"))
	if err != nil {
		return problem.InternalServer("failed to load hand share", c, err).Send(c)
	}
	if share == nil {
		return problem.NotFound("hand share not found").Send(c)
	}
	return c.JSON(share)
}

func (h *handShareHandlers) revoke(c fiber.Ctx) error {
	err := h.shares.Revoke(c.Context(), c.Locals(localsUserID).(string), c.Params("token"))
	if errors.Is(err, handshare.ErrNotOwner) {
		return problem.NotFound("hand share not found").Send(c)
	}
	if err != nil {
		return problem.InternalServer("failed to revoke hand share", c, err).Send(c)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func aliasesFor(source *sessionlog.HandItem, ownerID string) map[string]string {
	aliases := map[string]string{ownerID: "hero"}
	for i, opponent := range source.Opponents {
		aliases[opponent.PlayerID] = fmt.Sprintf("player_%d", i+1)
	}
	return aliases
}

func aliasFor(aliases map[string]string, playerID string) string {
	if playerID == "" {
		return ""
	}
	if alias := aliases[playerID]; alias != "" {
		return alias
	}
	alias := fmt.Sprintf("player_%d", len(aliases))
	aliases[playerID] = alias
	return alias
}

func anonymizedOpponents(source *sessionlog.HandItem) []handshare.Opponent {
	out := make([]handshare.Opponent, len(source.Opponents))
	for i, opponent := range source.Opponents {
		out[i] = handshare.Opponent{
			Alias: fmt.Sprintf("Jogador %d", i+1), Won: opponent.Won,
			// The player's own history only contains opponent cards that were
			// genuinely shown. Copying this field cannot reveal a folded hand.
			HoleCards: append([]string(nil), opponent.HoleCards...),
		}
	}
	return out
}

func anonymizedActions(entries []tablestore.ActionLogEntry, aliases map[string]string) []handshare.Action {
	out := make([]handshare.Action, 0, len(entries))
	for _, entry := range entries {
		action := handshare.Action{
			Seq: entry.Seq, PlayerID: aliasFor(aliases, entry.PlayerID), Action: entry.Action,
			Amount: entry.Amount, Timestamp: entry.Timestamp,
		}
		if entry.Frame != nil {
			frame := &handshare.ReplayFrame{
				Stage: entry.Frame.Stage, Board: append([]string(nil), entry.Frame.Board...),
				CurrentPlayerID:    aliasFor(aliases, entry.Frame.CurrentPlayerID),
				DealerPlayerID:     aliasFor(aliases, entry.Frame.DealerPlayerID),
				SmallBlindPlayerID: aliasFor(aliases, entry.Frame.SmallBlindPlayerID),
				BigBlindPlayerID:   aliasFor(aliases, entry.Frame.BigBlindPlayerID),
				Pot:                entry.Frame.Pot, Payouts: make(map[string]int64), Seats: make([]handshare.ReplaySeat, 0, len(entry.Frame.Seats)),
			}
			for _, winner := range entry.Frame.Winners {
				frame.Winners = append(frame.Winners, aliasFor(aliases, winner))
			}
			for playerID, amount := range entry.Frame.Payouts {
				frame.Payouts[aliasFor(aliases, playerID)] = amount
			}
			for _, seat := range entry.Frame.Seats {
				alias := aliasFor(aliases, seat.PlayerID)
				name := "Você"
				if alias != "hero" {
					name = "Jogador"
				}
				frame.Seats = append(frame.Seats, handshare.ReplaySeat{
					PlayerID: alias, Name: name, Stack: seat.Stack, State: seat.State,
					Contributed: seat.Contributed, DealtIn: seat.DealtIn,
				})
			}
			action.Frame = frame
		}
		out = append(out, action)
	}
	return out
}
