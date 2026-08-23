package v1

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/highlights"
)

type fakeSessionChecker struct{ hasSession bool }

func (f *fakeSessionChecker) HasSessionAtTable(_ context.Context, _, _ string) (bool, error) {
	return f.hasSession, nil
}

type fakeHighlightsStore struct{ highlight *highlights.Highlight }

func (f *fakeHighlightsStore) GetToday(_ context.Context, _ string) (*highlights.Highlight, error) {
	return f.highlight, nil
}

func newHighlightsTestApp(sessions *fakeSessionChecker, store *fakeHighlightsStore) *fiber.App {
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "player-1"); return c.Next() }
	RegisterHighlights(app.Group("/v1.0"), auth, sessions, store)
	return app
}

func TestHighlights_404WithNoSessionAtTable(t *testing.T) {
	app := newHighlightsTestApp(&fakeSessionChecker{hasSession: false}, &fakeHighlightsStore{
		highlight: &highlights.Highlight{TableID: "t1", Pot: 500},
	})
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/rooms/t1/highlights/today", nil))
	if err != nil || resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected 404 for a stranger, got %d err %v", resp.StatusCode, err)
	}
}

func TestHighlights_404WhenNoneRecordedYet(t *testing.T) {
	app := newHighlightsTestApp(&fakeSessionChecker{hasSession: true}, &fakeHighlightsStore{highlight: nil})
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/rooms/t1/highlights/today", nil))
	if err != nil || resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected 404 when no highlight exists yet, got %d err %v", resp.StatusCode, err)
	}
}

func TestHighlights_200WithStoredHighlight(t *testing.T) {
	app := newHighlightsTestApp(&fakeSessionChecker{hasSession: true}, &fakeHighlightsStore{
		highlight: &highlights.Highlight{TableID: "t1", Date: "2026-08-23", Pot: 500},
	})
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/rooms/t1/highlights/today", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 for a player who was at the table, got %d err %v", resp.StatusCode, err)
	}
}
