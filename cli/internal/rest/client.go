// Package rest is a typed HTTP client for the poker API's REST surface
// (rooms, players, achievements). It injects the bearer token, sets the
// Origin header the WS/HTTP origin allow-list expects, and decodes RFC 9457
// problem responses into a typed error.
package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OriginHeader is sent on every request — the poker API enforces an Origin
// allow-list (api/CLAUDE.md issue #44); confirm this value is on that list
// for each environment before relying on it.
var OriginHeader = "https://poker.ctech.app"

// Client is a small wrapper around *http.Client bound to one API base URL and
// bearer-token source.
type Client struct {
	baseURL string
	token   func(context.Context) (string, error)
	hc      *http.Client
}

func New(baseURL string, token func(context.Context) (string, error), hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, hc: hc}
}

// ProblemError is an RFC 9457 application/problem+json response.
type ProblemError struct {
	Status int    `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Type   string `json:"type"`
}

func (e *ProblemError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s (%d): %s", e.Title, e.Status, e.Detail)
	}
	return fmt.Sprintf("%s (%d)", e.Title, e.Status)
}

// AsProblem is errors.As for *ProblemError, spelled out for readability at
// call sites that don't otherwise use the errors package.
func AsProblem(err error, target **ProblemError) bool {
	return errors.As(err, target)
}

// IsStatus reports whether err is a *ProblemError carrying this HTTP status.
func IsStatus(err error, status int) bool {
	var pe *ProblemError
	return errors.As(err, &pe) && pe.Status == status
}

// Page is the `sendPage` envelope every poker API list endpoint returns.
type Page[T any] struct {
	Data           []T    `json:"data"`
	HasNext        bool   `json:"has_next"`
	NextCursor     string `json:"next_cursor"`
	HasPrevious    bool   `json:"has_previous"`
	PreviousCursor string `json:"previous_cursor"`
}

// Do issues method/path with body (JSON-encoded if non-nil) and decodes the
// response into out (ignored if nil). A non-2xx response decodes into a
// *ProblemError instead.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", OriginHeader)
	tok, err := c.token(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var pe ProblemError
		_ = json.NewDecoder(resp.Body).Decode(&pe) // best effort; zero value still carries the status below
		if pe.Status == 0 {
			pe.Status = resp.StatusCode
		}
		if pe.Title == "" {
			pe.Title = resp.Status
		}
		return &pe
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
