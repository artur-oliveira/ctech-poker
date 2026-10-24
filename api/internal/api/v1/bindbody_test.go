package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// Handlers bind through go-common validation: unknown fields, trailing data and
// malformed JSON are 400 without echoing the body.
func TestBindBodyIsStrict(t *testing.T) {
	type req struct {
		A int `json:"a"`
	}
	app := fiber.New()
	app.Post("/x", func(c fiber.Ctx) error {
		var r req
		if p := bindBody(c, &r); p != nil {
			return p.Send(c)
		}
		return c.SendStatus(http.StatusNoContent)
	})
	for body, want := range map[string]int{`{"a":1}`: 204, `{"a":1,"b":2}`: 400, `{"a":1} x`: 400, `nope`: 400, ``: 400} {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(r)
		if err != nil || resp.StatusCode != want {
			t.Fatalf("body %q: code=%v err=%v want %d", body, resp, err, want)
		}
	}
}

func TestLeaveAcceptsEmptyBody(t *testing.T) {
	var req LeaveRoomRequest
	app := fiber.New()
	app.Post("/leave", func(c fiber.Ctx) error {
		if len(c.Body()) > 0 {
			if p := bindBody(c, &req); p != nil {
				return p.Send(c)
			}
		}
		return c.SendStatus(http.StatusNoContent)
	})
	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/leave", nil))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
}
