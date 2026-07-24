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

	pathGameHold    = "/v1.0/internal/wallet/game/hold"
	pathGameRelease = "/v1.0/internal/wallet/game/hold/%s/release"
	pathGameCashout = "/v1.0/internal/wallet/game/cashout"
	pathGameStatus  = "/v1.0/internal/wallet/game/status/%s"
	pathBalance     = "/v1.0/internal/wallet/balance/%s"

	scopeCredit      = "internal:wallet:credit"
	scopeDebit       = "internal:wallet:debit"
	scopeGameHold    = "internal:wallet:game-hold"
	scopeGameCashout = "internal:wallet:game-cashout"
	scopeGameStatus  = "internal:wallet:game-status"
	scopeBalance     = "internal:wallet:balance"
)

// Error is a passthrough of ctech-wallet's own RFC 9457 problem+json body —
// poker's problem package uses the same shape, so a caller can turn this
// straight into problem.New(err.Status, err.Type, err.Title, err.Detail)
// instead of masking it as an internal server error.
type Error struct {
	Status int    `json:"status"`
	Type   string `json:"type"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

func (e *Error) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("walletclient: %s: %s", e.Title, e.Detail)
	}
	return fmt.Sprintf("walletclient: %s", e.Title)
}

// walletError parses a failed response body as ctech-wallet's problem+json.
// Falls back to a plain status/body error when the body isn't a well-formed
// problem (status/title missing) — callers should treat that fallback the
// same as any other internal error, not passthrough it to their own client.
func walletError(statusCode int, raw []byte) error {
	var p Error
	if json.Unmarshal(raw, &p) == nil && p.Status != 0 && p.Title != "" {
		return &p
	}
	return fmt.Errorf("walletclient: status %d: %s", statusCode, string(raw))
}

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
	base              string
	http              *http.Client
	creditTokens      *oauth2client.TokenManager
	debitTokens       *oauth2client.TokenManager
	gameHoldTokens    *oauth2client.TokenManager
	gameCashoutTokens *oauth2client.TokenManager
	gameStatusTokens  *oauth2client.TokenManager
	balanceTokens     *oauth2client.TokenManager
}

// Balances is ctech-wallet's M2M balance snapshot (GET
// /internal/wallet/balance/:user_id) — real is deliberately never exposed
// here, only game (ring-fenced real-money) and sandbox.
type Balances struct {
	GameBalance    int64 `json:"game_balance"`
	SandboxBalance int64 `json:"sandbox_balance"`
}

// New builds the wallet client. Separate TokenManagers per scope mirror
// ctech-wallet's own kycclient pattern of one scope per token manager — a
// credit-scoped token must never be reused for a debit call or vice versa.
func New(cfg *config.Config) *Client {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	baseAuth := strings.TrimRight(cfg.CtechURL, "/")
	base := strings.TrimRight(cfg.WalletURL, "/")
	return &Client{
		base:              base,
		http:              httpClient,
		creditTokens:      oauth2client.New(httpClient, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeCredit),
		debitTokens:       oauth2client.New(httpClient, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeDebit),
		gameHoldTokens:    oauth2client.New(httpClient, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeGameHold),
		gameCashoutTokens: oauth2client.New(httpClient, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeGameCashout),
		gameStatusTokens:  oauth2client.New(httpClient, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeGameStatus),
		balanceTokens:     oauth2client.New(httpClient, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeBalance),
	}
}

func (c *Client) Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error {
	return c.movement(ctx, c.base+pathSandboxCredit, c.creditTokens, userID, amount, idempotencyKey, reason)
}

func (c *Client) Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error {
	return c.movement(ctx, c.base+pathSandboxDebit, c.debitTokens, userID, amount, idempotencyKey, reason)
}

// HoldGame reserves funds in the ring-fenced game wallet.
// tableRef is an opaque caller-supplied session identifier (e.g. table_id:seat).
func (c *Client) HoldGame(ctx context.Context, userID string, amount int64, tableRef, idempotencyKey, reason string) (string, error) {
	token, err := c.gameHoldTokens.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("walletclient: token: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"user_id":         userID,
		"amount":          amount,
		"table_ref":       tableRef,
		"idempotency_key": idempotencyKey,
	})
	if err != nil {
		return "", fmt.Errorf("walletclient: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+pathGameHold, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("walletclient: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("walletclient: status %d: %s", resp.StatusCode, string(raw))
	}
	var res struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("walletclient: decode response: %w", err)
	}
	return res.ID, nil
}

// ReleaseHold cancels a reservation in the ring-fenced game wallet.
func (c *Client) ReleaseHold(ctx context.Context, holdID string) error {
	token, err := c.gameHoldTokens.Get(ctx)
	if err != nil {
		return fmt.Errorf("walletclient: token: %w", err)
	}
	url := fmt.Sprintf(c.base+pathGameRelease, holdID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

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

// CashoutGame settles a reservation in the ring-fenced game wallet.
// holdIDs is the list of hold IDs to settle (wallet requires array).
// tableRef is an opaque caller-supplied session identifier.
func (c *Client) CashoutGame(ctx context.Context, userID string, amount int64, tableRef string, holdIDs []string, idempotencyKey, reason string) error {
	token, err := c.gameCashoutTokens.Get(ctx)
	if err != nil {
		return fmt.Errorf("walletclient: token: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"user_id":         userID,
		"amount":          amount,
		"table_ref":       tableRef,
		"hold_ids":        holdIDs,
		"idempotency_key": idempotencyKey,
	})
	if err != nil {
		return fmt.Errorf("walletclient: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+pathGameCashout, bytes.NewReader(body))
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

// IsGamblingActivated checks whether userID has completed ctech-wallet's
// ActivateGambling flow (verified KYC + gambling addendum).
func (c *Client) IsGamblingActivated(ctx context.Context, userID string) (bool, error) {
	token, err := c.gameStatusTokens.Get(ctx)
	if err != nil {
		return false, fmt.Errorf("walletclient: token: %w", err)
	}
	url := fmt.Sprintf(c.base+pathGameStatus, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("walletclient: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("walletclient: status %d: %s", resp.StatusCode, string(raw))
	}

	var body struct {
		Activated bool `json:"activated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, fmt.Errorf("walletclient: decode: %w", err)
	}
	return body.Activated, nil
}

// Balances reports userID's game+sandbox balances. real is never returned
// (ctech-wallet keeps it out of this endpoint's response entirely).
func (c *Client) Balances(ctx context.Context, userID string) (*Balances, error) {
	token, err := c.balanceTokens.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("walletclient: token: %w", err)
	}
	url := fmt.Sprintf(c.base+pathBalance, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("walletclient: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, walletError(resp.StatusCode, raw)
	}
	var b Balances
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		return nil, fmt.Errorf("walletclient: decode: %w", err)
	}
	return &b, nil
}

func (c *Client) movement(ctx context.Context, url string, tokens *oauth2client.TokenManager, userID string, amount int64, idempotencyKey, reason string) error {
	_, err := c.movementWithResponse(ctx, url, tokens, userID, amount, idempotencyKey, reason)
	return err
}

func (c *Client) movementWithResponse(ctx context.Context, url string, tokens *oauth2client.TokenManager, userID string, amount int64, idempotencyKey, reason string) (string, error) {
	token, err := tokens.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("walletclient: token: %w", err)
	}
	body, err := json.Marshal(MovementRequest{UserID: userID, Amount: amount, IdempotencyKey: idempotencyKey, Reason: reason})
	if err != nil {
		return "", fmt.Errorf("walletclient: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("walletclient: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return "", walletError(resp.StatusCode, raw)
	}

	var res struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("walletclient: decode response: %w", err)
	}
	return res.ID, nil
}
