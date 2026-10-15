package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"gopkg.aoctech.app/poker/api/internal/responsible"
)

func responsibleApp(t *testing.T) *fiber.App {
	t.Helper()
	svc := responsible.NewService(responsible.NewMemoryStore()).WithClock(func() time.Time { return time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC) })
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "user-1"); return c.Next() }
	RegisterResponsible(app.Group("/v1.0"), auth, svc)
	return app
}

func call(t *testing.T, app *fiber.App, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestResponsibleRoutesWalkTheWholeFlow(t *testing.T) {
	app := responsibleApp(t)
	base := "/v1.0/players/me/responsible"

	if code, st := call(t, app, http.MethodGet, base+"/", ""); code != 200 || st["accepted"] != false || st["addendum_version"] != responsible.CurrentGamblingAddendumVersion {
		t.Fatalf("initial status = %d %v", code, st)
	}
	if code, _ := call(t, app, http.MethodPost, base+"/accept", ""); code != http.StatusNoContent {
		t.Fatalf("accept = %d", code)
	}
	if code, _ := call(t, app, http.MethodPut, base+"/limits", `{"daily":500,"weekly":100,"monthly":900}`); code != http.StatusBadRequest {
		t.Fatalf("limits out of order = %d, want 400", code)
	}
	if code, lim := call(t, app, http.MethodPut, base+"/limits", `{"daily":500,"weekly":2000,"monthly":9000}`); code != 200 || lim["daily"] != float64(500) {
		t.Fatalf("limits = %d %v", code, lim)
	}
	// An increase waits: the response keeps the current limit and carries the pending set.
	if code, lim := call(t, app, http.MethodPut, base+"/limits", `{"daily":900,"weekly":2000,"monthly":9000}`); code != 200 || lim["daily"] != float64(500) || lim["pending"] == nil {
		t.Fatalf("increase = %d %v", code, lim)
	}
	if code, ex := call(t, app, http.MethodPost, base+"/self-exclusion", `{"period":"7d"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown period = %d %v", code, ex)
	}
	if code, ex := call(t, app, http.MethodPost, base+"/self-exclusion", `{"period":"30d"}`); code != http.StatusCreated || ex["until"] == "" {
		t.Fatalf("exclusion = %d %v", code, ex)
	}
	if code, p := call(t, app, http.MethodDelete, base+"/self-exclusion", ""); code != http.StatusConflict || p["type"] != "/problems/exclusion-change-rejected" {
		t.Fatalf("revoking a fixed period = %d %v", code, p)
	}
	if code, st := call(t, app, http.MethodGet, base+"/", ""); code != 200 || st["accepted"] != true || st["excluded"] == nil {
		t.Fatalf("final status = %d %v", code, st)
	}
}
