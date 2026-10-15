// Package accountclient reads the one thing Poker needs from ctech-account's internal
// KYC API: the keyed hash of a player's CPF, the key under which a self-exclusion
// outlives the account that made it. Poker's own M2M client, scope internal:account:kyc.
package accountclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/oauth2client"
)

const (
	pathToken  = "/v1.0/token"
	pathKYCGet = "/v1.0/internal/kyc/%s"
	scopeKYC   = "internal:account:kyc"
)

// Client talks to ctech-account's internal KYC endpoint.
type Client struct {
	base   string
	http   *http.Client
	tokens *oauth2client.TokenManager
}

// New builds the client. base is CTECH_URL; clientID/secret are Poker's M2M credentials.
func New(base, clientID, clientSecret string, c cache.Backend) *Client {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	base = strings.TrimRight(base, "/")
	return &Client{base: base, http: httpClient, tokens: oauth2client.New(httpClient, c, base+pathToken, clientID, clientSecret, scopeKYC)}
}

// CPFHMAC returns the keyed hash of the player's CPF, or "" when the account has none
// on record (no KYC, or an account that does not serve the hash).
func (c *Client) CPFHMAC(ctx context.Context, userID string) (string, error) {
	token, err := c.tokens.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("accountclient: token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(c.base+pathKYCGet, url.PathEscape(userID)), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("accountclient: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", nil // no KYC record: nothing to key an exclusion by
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("accountclient: status %d: %s", resp.StatusCode, raw)
	}
	var body struct {
		CPFHMAC string `json:"cpf_hmac"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("accountclient: decode: %w", err)
	}
	return body.CPFHMAC, nil
}
