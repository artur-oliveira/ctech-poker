package problem

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"

	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

func TestSendUsesRFC9457ContentTypeAndShape(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error { return BadRequest("invalid stake").Send(c) })
	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Header.Get("Content-Type"); got != ContentType {
		t.Fatalf("content-type=%q", got)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["type"] != "/problems/bad-request" || body["title"] != "Bad Request" || body["status"] != float64(400) || body["detail"] != "invalid stake" {
		t.Fatalf("problem=%+v", body)
	}
	if _, legacy := body["error"]; legacy {
		t.Fatalf("legacy error field present: %+v", body)
	}
}

func TestTableFullHasStableProblemType(t *testing.T) {
	p := TableFull()
	if p.Type != "/problems/table-full" || p.Status != http.StatusConflict {
		t.Fatalf("table-full problem=%+v", p)
	}
}

// #319: next_action/retry_after_seconds are optional extension members —
// present when a constructor knows the answer, entirely absent (via
// omitempty) otherwise, so old and new clients read the same body either way.

func TestTableFullSetsNextActionRetry(t *testing.T) {
	p := TableFull()
	if p.NextAction != "retry" {
		t.Fatalf("next_action=%q, want retry", p.NextAction)
	}
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["next_action"] != "retry" {
		t.Fatalf("serialized next_action=%v", decoded["next_action"])
	}
	if _, present := decoded["retry_after_seconds"]; present {
		t.Fatalf("retry_after_seconds should be omitted when unset: %+v", decoded)
	}
}

func TestUnauthorizedSetsNextActionReauthenticate(t *testing.T) {
	p := Unauthorized("token expired")
	if p.NextAction != "reauthenticate" {
		t.Fatalf("next_action=%q, want reauthenticate", p.NextAction)
	}
}

func TestBadRequestOmitsNextActionAndRetryAfter(t *testing.T) {
	body, err := json.Marshal(BadRequest("bad stake"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, present := decoded["next_action"]; present {
		t.Fatalf("next_action should be omitted: %+v", decoded)
	}
	if _, present := decoded["retry_after_seconds"]; present {
		t.Fatalf("retry_after_seconds should be omitted: %+v", decoded)
	}
}

func TestTooManyRequestsSetsRetryAfterSecondsFromLimiterWindow(t *testing.T) {
	p := TooManyRequests(60)
	if p.Status != http.StatusTooManyRequests {
		t.Fatalf("status=%d", p.Status)
	}
	if p.NextAction != "retry" || p.RetryAfterSeconds != 60 {
		t.Fatalf("next_action=%q retry_after_seconds=%d", p.NextAction, p.RetryAfterSeconds)
	}
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["retry_after_seconds"] != float64(60) {
		t.Fatalf("serialized retry_after_seconds=%v", decoded["retry_after_seconds"])
	}
}

func TestFromWalletErrorPropagatesNextActionWhenPresent(t *testing.T) {
	werr := &walletclient.Error{
		Status: http.StatusTooManyRequests, Type: "/problems/too-many-requests",
		Title: "Too Many Requests", Detail: "wallet busy",
		NextAction: "wait", RetryAfterSeconds: 5,
	}
	p, ok := FromWalletError(werr)
	if !ok {
		t.Fatal("expected ok=true for a walletclient.Error")
	}
	if p.NextAction != "wait" || p.RetryAfterSeconds != 5 {
		t.Fatalf("next_action=%q retry_after_seconds=%d", p.NextAction, p.RetryAfterSeconds)
	}
}

func TestFromWalletErrorLeavesNextActionUnsetWhenAbsent(t *testing.T) {
	werr := &walletclient.Error{
		Status: http.StatusConflict, Type: "/problems/conflict", Title: "Conflict", Detail: "insufficient balance",
	}
	p, ok := FromWalletError(werr)
	if !ok {
		t.Fatal("expected ok=true for a walletclient.Error")
	}
	if p.NextAction != "" {
		t.Fatalf("next_action should stay unset: %q", p.NextAction)
	}
}
