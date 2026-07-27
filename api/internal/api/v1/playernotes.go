package v1

import (
	"context"
	"errors"
	"net/url"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/playernotes"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type playerNoteStore interface {
	List(ctx context.Context, viewerID string) ([]playernotes.Note, error)
	Save(ctx context.Context, viewerID, opponentID, tag, text string) (*playernotes.Note, error)
}

type playerNoteHandlers struct{ store playerNoteStore }

type playerNoteRequest struct {
	Tag  string `json:"tag"`
	Note string `json:"note"`
}

func RegisterPlayerNotes(router fiber.Router, auth fiber.Handler, store playerNoteStore) {
	h := &playerNoteHandlers{store: store}
	g := router.Group("/players/me/notes", auth)
	g.Get("/", h.list)
	g.Post("/:opponentId", h.save)
}

func (h *playerNoteHandlers) list(c fiber.Ctx) error {
	notes, err := h.store.List(c.Context(), c.Locals(localsUserID).(string))
	if err != nil {
		return problem.InternalServer("failed to list private notes", c, err).Send(c)
	}
	return c.JSON(fiber.Map{"data": notes})
}

func (h *playerNoteHandlers) save(c fiber.Ctx) error {
	opponentID, err := url.PathUnescape(c.Params("opponentId"))
	if err != nil {
		return problem.BadRequest("opponent id is invalid").Send(c)
	}
	var req playerNoteRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	note, err := h.store.Save(
		c.Context(),
		c.Locals(localsUserID).(string),
		opponentID,
		req.Tag,
		req.Note,
	)
	switch {
	case errors.Is(err, playernotes.ErrInvalidOpponent):
		return problem.BadRequest("opponent must be another player").Send(c)
	case errors.Is(err, playernotes.ErrInvalidTag):
		return problem.BadRequest("tag is invalid").Send(c)
	case errors.Is(err, playernotes.ErrNoteTooLong):
		return problem.BadRequest("note must have at most 500 characters").Send(c)
	case err != nil:
		return problem.InternalServer("failed to save private note", c, err).Send(c)
	}
	if note == nil {
		return c.JSON(fiber.Map{"deleted": true})
	}
	return c.JSON(note)
}
