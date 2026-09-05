package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.aoctech.app/poker/cli/internal/auth"
	"gopkg.aoctech.app/poker/cli/internal/rest"
)

func testCmd() *cobra.Command {
	c := &cobra.Command{}
	c.SetContext(context.Background())
	return c
}

func staticToken(tok string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return tok, nil }
}

func failingToken(err error) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return "", err }
}

func TestRunProfilePrintsFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"user_id": "u-1", "name": "Ana", "friend_code": "PKR-AAAA-BBBB-CCCC",
			"wallet_mode": "sandbox", "sandbox_balance": 5000, "game_balance": 0,
		})
	}))
	defer srv.Close()

	var buf bytes.Buffer
	rc := rest.New(srv.URL, staticToken("t"), srv.Client())
	if err := runProfile(&buf, testCmd(), rc); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Ana") || !strings.Contains(out, "PKR-AAAA-BBBB-CCCC") {
		t.Fatalf("output missing expected fields: %q", out)
	}
}

func TestRunProfileLoggedOutHint(t *testing.T) {
	rc := rest.New("http://unused.invalid", failingToken(auth.ErrLoggedOut), http.DefaultClient)
	var buf bytes.Buffer
	err := runProfile(&buf, testCmd(), rc)
	if err == nil || !strings.Contains(err.Error(), "poker login") {
		t.Fatalf("want a login hint, got %v", err)
	}
}

func TestRunProfileForbiddenHintsAtPrerequisite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]any{"status": 403, "title": "Forbidden", "detail": "nope"})
	}))
	defer srv.Close()

	rc := rest.New(srv.URL, staticToken("t"), srv.Client())
	var buf bytes.Buffer
	err := runProfile(&buf, testCmd(), rc)
	if err == nil || !strings.Contains(err.Error(), "poker-cli") {
		t.Fatalf("want a hint about the poker-cli client prerequisite, got %v", err)
	}
}

func TestRunAchievementsPrintsTotalsAndEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"mode":   "sandbox",
			"totals": map[string]int{"revealed": 10, "unlocked": 3, "completed": 1, "stars": 7, "max_stars": 40},
			"achievements": []map[string]any{
				{"key": "first_win", "progress": 1, "stars": 1, "unlocked": true, "completed": true},
			},
		})
	}))
	defer srv.Close()

	var buf bytes.Buffer
	rc := rest.New(srv.URL, staticToken("t"), srv.Client())
	if err := runAchievements(&buf, testCmd(), rc); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "first_win") || !strings.Contains(out, "3/10") {
		t.Fatalf("output missing expected fields: %q", out)
	}
}
