package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func tokenFunc(tok string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return tok, nil }
}

func TestDoSetsAuthAndOriginHeadersAndDecodesBody(t *testing.T) {
	var gotAuth, gotOrigin string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotOrigin = r.Header.Get("Origin")
		json.NewEncoder(w).Encode(map[string]string{"hello": "world"})
	}))
	defer srv.Close()

	c := New(srv.URL, tokenFunc("jwt-1"), srv.Client())
	var out map[string]string
	if err := c.Do(context.Background(), http.MethodGet, "/v1.0/whatever", nil, &out); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer jwt-1" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotOrigin != OriginHeader {
		t.Errorf("Origin = %q, want %q", gotOrigin, OriginHeader)
	}
	if out["hello"] != "world" {
		t.Errorf("decoded body = %+v", out)
	}
}

func TestDoSendsJSONBody(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := New(srv.URL, tokenFunc("t"), srv.Client())
	body := map[string]any{"amount": float64(100)}
	if err := c.Do(context.Background(), http.MethodPost, "/x", body, nil); err != nil {
		t.Fatal(err)
	}
	if gotBody["amount"] != float64(100) {
		t.Fatalf("server received %+v", gotBody)
	}
}

func TestDoDecodesProblemResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]any{
			"type": "/problems/forbidden", "title": "Forbidden", "status": 403,
			"detail": "interactive poker operations require the first-party Poker client",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, tokenFunc("t"), srv.Client())
	err := c.Do(context.Background(), http.MethodPost, "/rooms/x/join", nil, nil)
	var pe *ProblemError
	if !AsProblem(err, &pe) {
		t.Fatalf("want *ProblemError, got %v (%T)", err, err)
	}
	if pe.Status != 403 || pe.Detail == "" {
		t.Fatalf("got %+v", pe)
	}
	if !IsStatus(err, 403) {
		t.Fatal("IsStatus(err, 403) should be true")
	}
}

func TestPageDecodesEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data":         []map[string]int{{"small_blind": 10, "big_blind": 20}},
			"has_next":     false,
			"next_cursor":  "",
			"has_previous": false,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, tokenFunc("t"), srv.Client())
	var page Page[struct {
		SmallBlind int `json:"small_blind"`
		BigBlind   int `json:"big_blind"`
	}]
	if err := c.Do(context.Background(), http.MethodGet, "/x", nil, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0].BigBlind != 20 || page.HasNext {
		t.Fatalf("got %+v", page)
	}
}
