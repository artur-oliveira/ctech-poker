// Package walletclient calls ctech-wallet's internal sandbox credit/debit
// endpoints using poker's own M2M client_credentials token. Real-money
// hold/capture is Phase 5 (gated on prerequisites ctech-wallet doesn't
// expose yet) — this client only ever touches the sandbox ledger.
package walletclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gopkg.aoctech.app/api-commons/oauth2client"
	"gopkg.aoctech.app/poker/api/internal/config"
)

const (
	pathToken         = "/v1.0/token"
	pathSandboxCredit = "/v1.0/internal/wallet/sandbox/credit"
	pathSandboxDebit  = "/v1.0/internal/wallet/sandbox/debit"

	scopeCredit = "internal:wallet:credit"
	scopeDebit  = "internal:wallet:debit"
)

// MovementRequest mirrors ctech-wallet's MovementOpRequest exactly (see
// ctech-wallet/api/internal/api/v1/dto.go) — amounts are integer centavos
// (poker's own chip counts are already integer, so no conversion happens
// here; a "chip" and a "sandbox centavo" are the same unit by convention).
type MovementRequest struct {
	UserID         string `json:"user_id"`
	Amount         int64  `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
	Reason         string `json:"reason"`
}

type Client struct {
	base         string
	http         *http.Client
	creditTokens *oauth2client.TokenManager
	debitTokens  *oauth2client.TokenManager
}

// New builds the wallet client. Separate TokenManagers per scope mirror
// ctech-wallet's own kycclient pattern of one scope per token manager — a
// credit-scoped token must never be reused for a debit call or vice versa.
func New(cfg *config.Config) *Client {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	base := strings.TrimRight(cfg.WalletURL, "/")
	return &Client{
		base:         base,
		http:         httpClient,
		creditTokens: oauth2client.New(httpClient, base+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeCredit),
		debitTokens:  oauth2client.New(httpClient, base+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeDebit),
	}
}

func (c *Client) Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error {
	return c.movement(ctx, c.base+pathSandboxCredit, c.creditTokens, userID, amount, idempotencyKey, reason)
}

func (c *Client) Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error {
	return c.movement(ctx, c.base+pathSandboxDebit, c.debitTokens, userID, amount, idempotencyKey, reason)
}

func (c *Client) movement(ctx context.Context, url string, tokens *oauth2client.TokenManager, userID string, amount int64, idempotencyKey, reason string) error {
	token, err := tokens.Get(ctx)
	if err != nil {
		return fmt.Errorf("walletclient: token: %w", err)
	}
	body, err := json.Marshal(MovementRequest{UserID: userID, Amount: amount, IdempotencyKey: idempotencyKey, Reason: reason})
	if err != nil {
		return fmt.Errorf("walletclient: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("walletclient: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("walletclient: status %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}
