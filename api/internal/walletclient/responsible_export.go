package walletclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"gopkg.aoctech.app/poker/api/internal/responsible"
)

// ResponsibleExport reads one page of the wallet's responsible-gambling state (scope
// internal:wallet:responsible-export). Used once, by the import that moves those
// rules to Poker; cursor is empty for the first page.
func (c *Client) ResponsibleExport(ctx context.Context, cursor string) (*responsible.ExportPage, error) {
	token, err := c.exportTokens.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("walletclient: token: %w", err)
	}
	target := c.base + pathResponsibleExport
	if cursor != "" {
		target += "?cursor=" + url.QueryEscape(cursor)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.do(req, true)
	if err != nil {
		return nil, fmt.Errorf("walletclient: request: %w", err)
	}
	defer closeResponseBody(ctx, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw := readResponseBody(ctx, resp.Body)
		return nil, fmt.Errorf("walletclient: status %d: %s", resp.StatusCode, string(raw))
	}
	var page responsible.ExportPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("walletclient: decode: %w", err)
	}
	return &page, nil
}
