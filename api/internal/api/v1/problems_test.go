package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/problem"
	"gopkg.aoctech.app/poker/api/internal/responsible"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

func problemBody(t *testing.T, p *problem.Problem) map[string]any {
	t.Helper()
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error { return p.Send(c) })
	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != p.Status || resp.Header.Get("Content-Type") != problem.ContentType {
		t.Fatalf("status=%d content-type=%q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestTableFullProblem(t *testing.T) {
	body := problemBody(t, tableFullProblem())
	if body["type"] != "/problems/table-full" || body["status"] != float64(409) || body["next_action"] != "retry" {
		t.Fatalf("body=%+v", body)
	}
	if _, present := body["retry_after_seconds"]; present {
		t.Fatalf("retry_after_seconds should be omitted: %+v", body)
	}
}

func TestUnauthorizedProblemSetsReauthenticate(t *testing.T) {
	if got := unauthorizedProblem("expired").NextAction; got != problem.NextActionReauthenticate {
		t.Fatalf("next_action=%q", got)
	}
}

func TestInternalProblemMapsDeadlineTo504(t *testing.T) {
	p := internalProblem("boom", context.DeadlineExceeded)
	if p.Status != http.StatusGatewayTimeout || p.Type != problem.TypeTimeout {
		t.Fatalf("deadline problem=%+v", p)
	}
	if p := internalProblem("boom", errors.New("x")); p.Status != http.StatusInternalServerError || p.Detail != "boom" {
		t.Fatalf("generic problem=%+v", p)
	}
}

func TestWalletProblemPassesThroughNextAction(t *testing.T) {
	p, ok := walletProblem(&walletclient.Error{
		Status: http.StatusTooManyRequests, Type: "/problems/too-many-requests",
		Title: "Too Many Requests", Detail: "wallet busy", NextAction: "wait", RetryAfterSeconds: 5,
	})
	if !ok || p.NextAction != "wait" || p.RetryAfterSeconds != 5 || p.Detail != "wallet busy" {
		t.Fatalf("p=%+v ok=%v", p, ok)
	}
	p, ok = walletProblem(&walletclient.Error{Status: http.StatusConflict, Type: "/problems/conflict", Title: "Conflict"})
	if !ok || p.NextAction != "" {
		t.Fatalf("next_action should stay unset: %+v", p)
	}
	if _, ok := walletProblem(errors.New("other")); ok {
		t.Fatal("non-wallet error must not match")
	}
}

func TestResponsibleProblemTypes(t *testing.T) {
	p, ok := responsibleProblem(responsible.ErrConsentRequired)
	if !ok || p.Type != "/problems/gambling-terms-required" || p.Status != http.StatusForbidden {
		t.Fatalf("p=%+v ok=%v", p, ok)
	}
	p, ok = responsibleProblem(responsible.ErrLimitsNotConfigured)
	if !ok || p.Type != "/problems/limits-not-configured" || p.Status != http.StatusConflict {
		t.Fatalf("p=%+v ok=%v", p, ok)
	}
	if _, ok := responsibleProblem(errors.New("other")); ok {
		t.Fatal("non-responsible error must not match")
	}
}
