package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/handreveal"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

type fakeHandRevealSessions struct {
	hand *sessionlog.HandItem
}

func (f *fakeHandRevealSessions) ListSessions(context.Context, string, int, map[string]types.AttributeValue) ([]sessionlog.SessionItem, map[string]types.AttributeValue, error) {
	return nil, nil, nil
}
func (f *fakeHandRevealSessions) ListHands(context.Context, string, string, int, map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error) {
	return nil, nil, nil
}
func (f *fakeHandRevealSessions) ListHandsByTable(context.Context, string, string, string, int, map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error) {
	return nil, nil, nil
}
func (f *fakeHandRevealSessions) GetHand(context.Context, string, string, string) (*sessionlog.HandItem, error) {
	return f.hand, nil
}

type fakeHandRevealStore struct {
	record *handreveal.HandRecord
}

func (f *fakeHandRevealStore) Get(context.Context, string) (*handreveal.HandRecord, error) {
	return f.record, nil
}

type fakeHandRevealService struct {
	paid    bool
	payErr  error
	payCall int
}

func (f *fakeHandRevealService) HasPaid(context.Context, string, string) (bool, error) { return f.paid, nil }
func (f *fakeHandRevealService) PayForReveal(context.Context, string, string, string, int64) error {
	f.payCall++
	return f.payErr
}

func newHandRevealApp(playerID string, sessions sessionLogReader, records handRevealStore, svc handRevealService) *fiber.App {
	app := fiber.New()
	auth := func(c fiber.Ctx) error {
		c.Locals(localsUserID, playerID)
		return c.Next()
	}
	RegisterHandReveal(app.Group("/v1.0"), auth, sessions, records, svc, nil)
	return app
}

func TestRevealWinnerRejectsNonParticipant(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: nil}
	app := newHandRevealApp("buyer", sessions, &fakeHandRevealStore{}, &fakeHandRevealService{})

	req := httptest.NewRequest(http.MethodPost, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for a non-participant, got %d", resp.StatusCode)
	}
}

func TestRevealWinnerRejectsMissingArchive(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: &sessionlog.HandItem{HandID: "hand-1", TableID: "table-1"}}
	app := newHandRevealApp("buyer", sessions, &fakeHandRevealStore{record: nil}, &fakeHandRevealService{})

	req := httptest.NewRequest(http.MethodPost, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when no archive exists (showdown/real-money/pre-feature hand), got %d", resp.StatusCode)
	}
}

func TestRevealWinnerRejectsWinnerBuyingOwnHand(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: &sessionlog.HandItem{HandID: "hand-1", TableID: "table-1"}}
	records := &fakeHandRevealStore{record: &handreveal.HandRecord{HandID: "hand-1", WinnerID: "buyer", BigBlind: 200}}
	app := newHandRevealApp("buyer", sessions, records, &fakeHandRevealService{})

	req := httptest.NewRequest(http.MethodPost, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for the winner buying their own hand, got %d", resp.StatusCode)
	}
}

func TestRevealWinnerRejectsAlreadyShown(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: &sessionlog.HandItem{HandID: "hand-1", TableID: "table-1"}}
	records := &fakeHandRevealStore{record: &handreveal.HandRecord{HandID: "hand-1", WinnerID: "winner", WinnerShown: true, BigBlind: 200}}
	app := newHandRevealApp("buyer", sessions, records, &fakeHandRevealService{})

	req := httptest.NewRequest(http.MethodPost, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 when the winner already voluntarily showed, got %d", resp.StatusCode)
	}
}

func TestRevealWinnerRejectsAlreadyPaid(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: &sessionlog.HandItem{HandID: "hand-1", TableID: "table-1"}}
	records := &fakeHandRevealStore{record: &handreveal.HandRecord{HandID: "hand-1", WinnerID: "winner", BigBlind: 200}}
	app := newHandRevealApp("buyer", sessions, records, &fakeHandRevealService{paid: true})

	req := httptest.NewRequest(http.MethodPost, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 when already purchased, got %d", resp.StatusCode)
	}
}

func TestRevealWinnerSuccessReturnsCards(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: &sessionlog.HandItem{HandID: "hand-1", TableID: "table-1"}}
	records := &fakeHandRevealStore{record: &handreveal.HandRecord{
		HandID: "hand-1", WinnerID: "winner", BigBlind: 200,
		PlayerHands: map[string]handreveal.PlayerHandCode{"winner": {Cards: [2]string{"Ah", "Kd"}}},
	}}
	svc := &fakeHandRevealService{}
	app := newHandRevealApp("buyer", sessions, records, svc)

	req := httptest.NewRequest(http.MethodPost, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Cards []string `json:"cards"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Cards) != 2 || body.Cards[0] != "Ah" || body.Cards[1] != "Kd" {
		t.Fatalf("unexpected cards in response: %+v", body.Cards)
	}
	if svc.payCall != 1 {
		t.Fatalf("expected PayForReveal to be called once, got %d", svc.payCall)
	}
}

func TestCheckReturnsFeeAndCardsOnlyWhenPaid(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: &sessionlog.HandItem{HandID: "hand-1", TableID: "table-1"}}
	records := &fakeHandRevealStore{record: &handreveal.HandRecord{
		HandID: "hand-1", WinnerID: "winner", BigBlind: 200,
		PlayerHands: map[string]handreveal.PlayerHandCode{"winner": {Cards: [2]string{"Ah", "Kd"}}},
	}}
	app := newHandRevealApp("buyer", sessions, records, &fakeHandRevealService{paid: false})

	req := httptest.NewRequest(http.MethodGet, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Fee         int64    `json:"fee"`
		AlreadyPaid bool     `json:"already_paid"`
		Cards       []string `json:"cards"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Fee != 200 || body.AlreadyPaid || body.Cards != nil {
		t.Fatalf("expected fee=200, already_paid=false, no cards before purchase; got %+v", body)
	}

	app = newHandRevealApp("buyer", sessions, records, &fakeHandRevealService{paid: true})
	req = httptest.NewRequest(http.MethodGet, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.AlreadyPaid || len(body.Cards) != 2 {
		t.Fatalf("expected already_paid=true with cards once paid; got %+v", body)
	}
}

func TestCheckReturns404WithNoArchive(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: &sessionlog.HandItem{HandID: "hand-1", TableID: "table-1"}}
	app := newHandRevealApp("buyer", sessions, &fakeHandRevealStore{record: nil}, &fakeHandRevealService{})

	req := httptest.NewRequest(http.MethodGet, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when no archive exists, got %d", resp.StatusCode)
	}
}
