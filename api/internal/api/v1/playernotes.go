package v1

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/playernotes"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type playerNoteStore interface {
	List(ctx context.Context, viewerID string) ([]playernotes.Note, error)
	GetMany(ctx context.Context, viewerID string, opponentIDs []string) ([]playernotes.Note, error)
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

// list answers the caller's private notes. With `?opponent_ids=` it reads
// exactly the players on the screen asking (the seats at a table, the players
// in one hand) in a single BatchGetItem; without it, it still returns the
// whole unpaginated list, which no first-party screen uses any more (#209) —
// it is kept for a cached older client and for a future notes-management
// screen, which will want a cursor before it ships.
func (h *playerNoteHandlers) list(c fiber.Ctx) error {
	viewerID := c.Locals(localsUserID).(string)
	ids, scoped := parseOpponentIDs(c.Query("opponent_ids"))
	if scoped && ids == nil {
		return problem.BadRequest("opponent_ids must contain between 1 and 25 ids").Send(c)
	}
	var notes []playernotes.Note
	var err error
	if scoped {
		notes, err = h.store.GetMany(c.Context(), viewerID, ids)
	} else {
		notes, err = h.store.List(c.Context(), viewerID)
	}
	if errors.Is(err, playernotes.ErrTooManyOpponents) {
		return problem.BadRequest("opponent_ids must contain between 1 and 25 ids").Send(c)
	}
	if err != nil {
		return problem.InternalServer("failed to list private notes", c, err).Send(c)
	}
	return c.JSON(fiber.Map{"data": notes})
}

// parseOpponentIDs reports whether the caller scoped the request at all, and
// the ids if they did. An explicitly empty or oversized list is rejected
// rather than silently widened back into the full-list read.
func parseOpponentIDs(query string) (ids []string, scoped bool) {
	if strings.TrimSpace(query) == "" {
		return nil, false
	}
	for _, part := range strings.Split(query, ",") {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 || len(ids) > playernotes.MaxBatchOpponentIDs {
		return nil, true
	}
	return ids, true
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
