package v1

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
)

// fakeSettlementStore is a minimal in-memory settlementReader that actually
// filters by playerID, so the IDOR test exercises the same boundary the real
// gsi_player_settlements query relies on rather than a stub returning
// everything unconditionally.
type fakeSettlementStore struct {
	byPlayer map[string][]reconcile.PendingCashout
	calls    int
}

func (s *fakeSettlementStore) ListForPlayer(_ context.Context, playerID string, limit int, startKey map[string]types.AttributeValue) ([]reconcile.PendingCashout, map[string]types.AttributeValue, error) {
	s.calls++
	items := s.byPlayer[playerID]
	if startKey != nil {
		// Simulate a second page: return nothing further so tests can assert
		// has_next flips false once the client actually pages.
		return nil, nil, nil
	}
	if limit > 0 && len(items) > limit {
		return items[:limit], map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "cursor"}}, nil
	}
	return items, nil, nil
}

func TestMySettlementsOnlyReturnsCallersOwnEntries(t *testing.T) {
	store := &fakeSettlementStore{byPlayer: map[string][]reconcile.PendingCashout{
		"viewer": {{ID: "p1", PlayerID: "viewer", Amount: 100, CurrencyMode: "real", Kind: reconcile.KindFeeDebit,
			HoldIDs: []string{"hold-1"}, IdempotencyKey: "secret-key", LastError: "raw wallet failure",
			TableRef: "room-1", RecordedAt: "2026-09-01T00:00:00Z"}},
		"someone-else": {{ID: "p2", PlayerID: "someone-else", Amount: 999, CurrencyMode: "real", RecordedAt: "2026-09-01T00:00:00Z"}},
	}}
	h := &playerHandlers{settlements: store}
	app := fiber.New()
	app.Get("/players/me/settlements", func(c fiber.Ctx) error {
		c.Locals(localsUserID, "viewer")
		return c.Next()
	}, h.mySettlements)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/players/me/settlements", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d err=%v", resp.StatusCode, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var body struct {
		Data []reconcile.SettlementView `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, raw)
	}
	if len(body.Data) != 1 || body.Data[0].ID != "p1" {
		t.Fatalf("expected exactly the caller's own settlement, got %s", raw)
	}
	if strings.Contains(string(raw), "999") || strings.Contains(string(raw), "someone-else") {
		t.Fatalf("leaked another player's settlement: %s", raw)
	}
	for _, forbidden := range []string{"hold_ids", "idempotency_key", "last_error", "secret-key", "raw wallet failure"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("leaked internal field %q: %s", forbidden, raw)
		}
	}
}

func TestMySettlementsShowsManualReviewAsGenericStatus(t *testing.T) {
	store := &fakeSettlementStore{byPlayer: map[string][]reconcile.PendingCashout{
		"viewer": {{ID: "p1", PlayerID: "viewer", Amount: 100, CurrencyMode: "real",
			GSIStatus: "manual_review", Attempts: reconcile.MaxAttempts, LastError: "wallet 500 after 5 retries",
			RecordedAt: "2026-09-01T00:00:00Z"}},
	}}
	h := &playerHandlers{settlements: store}
	app := fiber.New()
	app.Get("/players/me/settlements", func(c fiber.Ctx) error {
		c.Locals(localsUserID, "viewer")
		return c.Next()
	}, h.mySettlements)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/players/me/settlements", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d err=%v", resp.StatusCode, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), `"status":"manual_review"`) {
		t.Fatalf("expected generic manual_review status: %s", raw)
	}
	if strings.Contains(string(raw), "wallet 500 after 5 retries") {
		t.Fatalf("leaked raw failure text: %s", raw)
	}
}

func TestMySettlementsPaginatesOneQueryPerPage(t *testing.T) {
	items := make([]reconcile.PendingCashout, 0, 3)
	for i := 0; i < 3; i++ {
		items = append(items, reconcile.PendingCashout{ID: string(rune('a' + i)), PlayerID: "viewer", RecordedAt: "2026-09-01T00:00:00Z"})
	}
	store := &fakeSettlementStore{byPlayer: map[string][]reconcile.PendingCashout{"viewer": items}}
	h := &playerHandlers{settlements: store}
	app := fiber.New()
	app.Get("/players/me/settlements", func(c fiber.Ctx) error {
		c.Locals(localsUserID, "viewer")
		return c.Next()
	}, h.mySettlements)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/players/me/settlements?limit=2", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d err=%v", resp.StatusCode, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var page PaginatedResponse
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatalf("decode: %v body=%s", err, raw)
	}
	if !page.HasNext || page.NextCursor == nil {
		t.Fatalf("expected a next page: %s", raw)
	}
	if store.calls != 1 {
		t.Fatalf("expected exactly one query for this page, got %d", store.calls)
	}

	next := httptest.NewRequest(fiber.MethodGet, "/players/me/settlements?cursor="+*page.NextCursor, nil)
	resp2, err := app.Test(next)
	if err != nil || resp2.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d err=%v", resp2.StatusCode, err)
	}
	raw2, _ := io.ReadAll(resp2.Body)
	var page2 PaginatedResponse
	if err := json.Unmarshal(raw2, &page2); err != nil {
		t.Fatalf("decode: %v body=%s", err, raw2)
	}
	if page2.HasNext {
		t.Fatalf("expected no further page: %s", raw2)
	}
}
