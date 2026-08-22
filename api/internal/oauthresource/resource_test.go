package oauthresource

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestProtectedResourceMetadata(t *testing.T) {
	app := fiber.New()
	Register(app, "https://poker.example.test", "https://accounts.example.test")
	resp, err := app.Test(httptest.NewRequest("GET", "/.well-known/oauth-protected-resource", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Resource string   `json:"resource"`
		Scopes   []string `json:"scopes_supported"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || body.Resource != "https://poker.example.test" || len(body.Scopes) != 12 {
		t.Fatalf("status=%d metadata=%+v", resp.StatusCode, body)
	}
}
