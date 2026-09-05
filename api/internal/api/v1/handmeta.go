package v1

import (
	"context"
	"errors"
	"net/url"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/handmeta"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type handMetaStore interface {
	Get(ctx context.Context, playerID, handID string) (*handmeta.Meta, error)
	Save(ctx context.Context, playerID, handID string, streetNotes map[string]string, reviewMarked bool, collections []string) (*handmeta.Meta, error)
	ListMarked(ctx context.Context, playerID string) ([]handmeta.Meta, error)
	ListSavedFilters(ctx context.Context, playerID string) ([]handmeta.SavedFilter, error)
	SaveFilters(ctx context.Context, playerID string, filters []handmeta.SavedFilter) ([]handmeta.SavedFilter, error)
}

type handMetaHandlers struct{ store handMetaStore }

type handMetaRequest struct {
	StreetNotes  map[string]string `json:"street_notes"`
	ReviewMarked bool              `json:"review_marked"`
	Collections  []string          `json:"collections"`
}

type savedFiltersRequest struct {
	Filters []handmeta.SavedFilter `json:"filters"`
}

// RegisterHandMeta mounts the one endpoint #349 (street notes + review
// marker) and #347 (collections, via the same record's `collections` field)
// were coordinated to share, plus the small sibling resource #347's saved
// filters needed — a player-scoped list, not a per-hand one, so it gets its
// own path but the same store and table.
func RegisterHandMeta(router fiber.Router, auth fiber.Handler, store handMetaStore) {
	h := &handMetaHandlers{store: store}
	g := router.Group("/players/me", auth)
	g.Get("/hands/:handId/meta", h.getMeta)
	g.Put("/hands/:handId/meta", h.saveMeta)
	g.Get("/hand-collections", h.listMarked)
	g.Get("/hand-filters", h.listFilters)
	g.Put("/hand-filters", h.saveFilters)
}

func (h *handMetaHandlers) getMeta(c fiber.Ctx) error {
	handID, err := url.PathUnescape(c.Params("handId"))
	if err != nil {
		return problem.BadRequest("hand id is invalid").Send(c)
	}
	meta, err := h.store.Get(c.Context(), c.Locals(localsUserID).(string), handID)
	if err != nil {
		return problem.InternalServer("failed to load hand metadata", c, err).Send(c)
	}
	if meta == nil {
		// No annotation saved yet — an empty, well-shaped record, not a 404:
		// the client renders the same street-note form either way.
		return c.JSON(fiber.Map{"hand_id": handID, "review_marked": false})
	}
	return c.JSON(meta)
}

func (h *handMetaHandlers) saveMeta(c fiber.Ctx) error {
	handID, err := url.PathUnescape(c.Params("handId"))
	if err != nil {
		return problem.BadRequest("hand id is invalid").Send(c)
	}
	var req handMetaRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	// playerID always comes from the auth context, never the body — no
	// caller may write another player's annotation (IDOR).
	meta, err := h.store.Save(c.Context(), c.Locals(localsUserID).(string), handID, req.StreetNotes, req.ReviewMarked, req.Collections)
	switch {
	case errors.Is(err, handmeta.ErrInvalidHand):
		return problem.BadRequest("hand id is invalid").Send(c)
	case errors.Is(err, handmeta.ErrInvalidStreet):
		return problem.BadRequest("street is invalid").Send(c)
	case errors.Is(err, handmeta.ErrNoteTooLong):
		return problem.BadRequest("street note is too long").Send(c)
	case errors.Is(err, handmeta.ErrTooManyCollections):
		return problem.BadRequest("too many collections").Send(c)
	case errors.Is(err, handmeta.ErrCollectionNameInvalid):
		return problem.BadRequest("collection name is invalid").Send(c)
	case err != nil:
		return problem.InternalServer("failed to save hand metadata", c, err).Send(c)
	}
	if meta == nil {
		return c.JSON(fiber.Map{"hand_id": handID, "review_marked": false})
	}
	return c.JSON(meta)
}

// listMarked backs the /hands "Coleções" tab: every hand the player marked
// for review or filed into a collection.
func (h *handMetaHandlers) listMarked(c fiber.Ctx) error {
	marked, err := h.store.ListMarked(c.Context(), c.Locals(localsUserID).(string))
	if err != nil {
		return problem.InternalServer("failed to list marked hands", c, err).Send(c)
	}
	return c.JSON(fiber.Map{"data": marked})
}

func (h *handMetaHandlers) listFilters(c fiber.Ctx) error {
	filters, err := h.store.ListSavedFilters(c.Context(), c.Locals(localsUserID).(string))
	if err != nil {
		return problem.InternalServer("failed to list saved filters", c, err).Send(c)
	}
	return c.JSON(fiber.Map{"data": filters})
}

func (h *handMetaHandlers) saveFilters(c fiber.Ctx) error {
	var req savedFiltersRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	filters, err := h.store.SaveFilters(c.Context(), c.Locals(localsUserID).(string), req.Filters)
	switch {
	case errors.Is(err, handmeta.ErrTooManySavedFilters):
		return problem.BadRequest("too many saved filters").Send(c)
	case errors.Is(err, handmeta.ErrFilterNameInvalid):
		return problem.BadRequest("filter name is invalid").Send(c)
	case err != nil:
		return problem.InternalServer("failed to save filters", c, err).Send(c)
	}
	return c.JSON(fiber.Map{"data": filters})
}
