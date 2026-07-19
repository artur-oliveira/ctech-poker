package problem

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
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
