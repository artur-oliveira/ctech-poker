package v1

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/observability"
	"gopkg.aoctech.app/poker/api/internal/chatprefs"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type chatPrefsStore interface {
	Get(ctx context.Context, playerID string) (*chatprefs.Preferences, error)
	Save(ctx context.Context, playerID string, extraWords []string) (*chatprefs.Preferences, error)
}

// chatPrefsInvalidator drops the cached copy the moment a preference
// changes, so a table already reading from the cache picks up the new
// list at most ChatPrefsRefreshInterval later instead of waiting out the
// cache's own longer TTL fallback.
type chatPrefsInvalidator interface {
	Invalidate(ctx context.Context, playerID string) error
}

type chatPrefsHandlers struct {
	store       chatPrefsStore
	invalidator chatPrefsInvalidator
}

type chatPrefsRequest struct {
	// ExtraWords is the caller's personal addition on top of the table-wide
	// floor (#327). There is deliberately no field here that could remove or
	// disable a floor word — the floor is not something this endpoint can
	// ever touch.
	ExtraWords []string `json:"extra_words"`
}

// RegisterChatPrefs mounts /players/me/chat-prefs. store/invalidator may be
// nil (e.g. the narrow test construction seam), in which case the routes
// still mount but answer "unavailable" rather than panicking.
func RegisterChatPrefs(router fiber.Router, auth fiber.Handler, store chatPrefsStore, invalidator chatPrefsInvalidator) {
	h := &chatPrefsHandlers{store: store, invalidator: invalidator}
	g := router.Group("/players/me/chat-prefs", auth)
	g.Get("/", h.get)
	g.Put("/", h.save)
}

func (h *chatPrefsHandlers) get(c fiber.Ctx) error {
	if h.store == nil {
		return problem.InternalServer("chat preferences unavailable", c, errors.New("no store wired")).Send(c)
	}
	viewerID := c.Locals(localsUserID).(string)
	prefs, err := h.store.Get(c.Context(), viewerID)
	if err != nil {
		return problem.InternalServer("failed to load chat preferences", c, err).Send(c)
	}
	if prefs == nil {
		return c.JSON(fiber.Map{"extra_words": []string{}})
	}
	return c.JSON(prefs)
}

func (h *chatPrefsHandlers) save(c fiber.Ctx) error {
	if h.store == nil {
		return problem.InternalServer("chat preferences unavailable", c, errors.New("no store wired")).Send(c)
	}
	var req chatPrefsRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	viewerID := c.Locals(localsUserID).(string)
	prefs, err := h.store.Save(c.Context(), viewerID, req.ExtraWords)
	switch {
	case errors.Is(err, chatprefs.ErrTooManyWords):
		return problem.BadRequest("extra_words must have at most 50 entries").Send(c)
	case errors.Is(err, chatprefs.ErrWordTooLong):
		return problem.BadRequest("each extra word must be at most 32 characters").Send(c)
	case errors.Is(err, chatprefs.ErrInvalidPlayer):
		return problem.BadRequest("player is invalid").Send(c)
	case err != nil:
		return problem.InternalServer("failed to save chat preferences", c, err).Send(c)
	}
	if h.invalidator != nil {
		if err := h.invalidator.Invalidate(c.Context(), viewerID); err != nil {
			observability.Warn(c.Context(), "chat prefs cache invalidate failed", err, "player_id", viewerID)
		}
	}
	if prefs == nil {
		return c.JSON(fiber.Map{"extra_words": []string{}})
	}
	return c.JSON(prefs)
}
