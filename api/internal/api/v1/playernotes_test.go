package v1

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/playernotes"
)

type fakeNoteStore struct {
	listCalls   int
	batchCalls  [][]string
	notes       []playernotes.Note
	savedLabels []string
}

func (f *fakeNoteStore) List(context.Context, string) ([]playernotes.Note, error) {
	f.listCalls++
	if f.notes != nil {
		return f.notes, nil
	}
	return []playernotes.Note{{OpponentID: "everyone"}}, nil
}

func (f *fakeNoteStore) GetMany(_ context.Context, _ string, opponentIDs []string) ([]playernotes.Note, error) {
	f.batchCalls = append(f.batchCalls, opponentIDs)
	return []playernotes.Note{{OpponentID: opponentIDs[0]}}, nil
}

func (f *fakeNoteStore) Save(_ context.Context, _, opponentID, tag, text string, labels []string) (*playernotes.Note, error) {
	f.savedLabels = labels
	return &playernotes.Note{OpponentID: opponentID, Tag: tag, Text: text, Labels: labels}, nil
}

func noteTestApp() (*fiber.App, *fakeNoteStore) {
	store := &fakeNoteStore{}
	app := fiber.New()
	RegisterPlayerNotes(app.Group("/v1.0"), func(c fiber.Ctx) error {
		c.Locals(localsUserID, "viewer")
		return c.Next()
	}, store)
	return app, store
}

func TestPlayerNotesScopedByOpponentIDs(t *testing.T) {
	app, store := noteTestApp()

	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/players/me/notes/?opponent_ids=a,%20b%20,,a", nil))
	if err != nil || response.StatusCode != fiber.StatusOK {
		t.Fatalf("response=%v err=%v", response.StatusCode, err)
	}
	if store.listCalls != 0 {
		t.Fatal("a scoped request must never fall back to the whole-history read")
	}
	if len(store.batchCalls) != 1 || strings.Join(store.batchCalls[0], ",") != "a,b,a" {
		t.Fatalf("expected the trimmed ids to reach the store, got %v", store.batchCalls)
	}
	var payload struct {
		Data []playernotes.Note `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 || payload.Data[0].OpponentID != "a" {
		t.Fatalf("unexpected payload: %+v", payload.Data)
	}
}

func TestPlayerNotesRejectsEmptyOrOversizedOpponentIDs(t *testing.T) {
	// A parameter that is present but unusable must be an error, never a
	// silent widening back into the unpaginated read (#209).
	for _, query := range []string{
		"?opponent_ids=,",
		"?opponent_ids=" + strings.TrimSuffix(strings.Repeat("p,", playernotes.MaxBatchOpponentIDs+1), ","),
	} {
		app, store := noteTestApp()
		response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/players/me/notes/"+query, nil))
		if err != nil || response.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("%s response=%v err=%v", query, response.StatusCode, err)
		}
		if store.listCalls != 0 || len(store.batchCalls) != 0 {
			t.Fatalf("%s must not read anything: list=%d batch=%v", query, store.listCalls, store.batchCalls)
		}
	}
}

func TestPlayerNotesWithoutOpponentIDsStillListsEverything(t *testing.T) {
	// Kept for cached older clients; no first-party screen calls it any more.
	app, store := noteTestApp()
	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/players/me/notes/", nil))
	if err != nil || response.StatusCode != fiber.StatusOK {
		t.Fatalf("response=%v err=%v", response.StatusCode, err)
	}
	if store.listCalls != 1 || len(store.batchCalls) != 0 {
		t.Fatalf("expected the unscoped list, got list=%d batch=%v", store.listCalls, store.batchCalls)
	}
}

// The label/free-text search is the whole point of #345, and it must run over
// the caller's own notes without adding a second read.
func TestPlayerNotesSearchFiltersInMemory(t *testing.T) {
	app, store := noteTestApp()
	store.notes = []playernotes.Note{
		{OpponentID: "a", Text: "paga demais", Labels: []string{"calling-station"}},
		{OpponentID: "b", Text: "3-bet leve", Labels: []string{"agressivo"}},
	}

	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/players/me/notes/?label=agressivo", nil))
	if err != nil || response.StatusCode != fiber.StatusOK {
		t.Fatalf("response=%v err=%v", response.StatusCode, err)
	}
	defer func() { _ = response.Body.Close() }()
	var payload struct {
		Data []playernotes.Note `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 || payload.Data[0].OpponentID != "b" {
		t.Fatalf("data=%+v", payload.Data)
	}
	if store.listCalls != 1 {
		t.Fatalf("search must reuse the single list read, got %d", store.listCalls)
	}
}

func TestPlayerNotesSaveForwardsLabels(t *testing.T) {
	app, store := noteTestApp()
	body := strings.NewReader(`{"note":"3-bet leve","labels":["Agressivo","3-bet"]}`)
	request := httptest.NewRequest(fiber.MethodPost, "/v1.0/players/me/notes/opponent", body)
	request.Header.Set("Content-Type", "application/json")

	response, err := app.Test(request)
	if err != nil || response.StatusCode != fiber.StatusOK {
		t.Fatalf("response=%v err=%v", response.StatusCode, err)
	}
	_ = response.Body.Close()
	if strings.Join(store.savedLabels, ",") != "Agressivo,3-bet" {
		t.Fatalf("labels reaching the store=%v", store.savedLabels)
	}
}
