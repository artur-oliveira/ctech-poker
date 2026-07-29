// Package walletclient calls ctech-wallet's internal M2M endpoints using
// poker's own client_credentials token: plain credit/debit against the
// sandbox ledger, and a hold/release/cashout/activation contract against
// the real-money "game" ledger (Phase 5, gated by config.RealMoneyEnabled).
package walletclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/oauth2client"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/metrics"
)

const (
	pathToken         = "/v1.0/token"
	pathSandboxCredit = "/v1.0/internal/wallet/sandbox/credit"
	pathSandboxDebit  = "/v1.0/internal/wallet/sandbox/debit"
	pathRealDebit     = "/v1.0/internal/wallet/real/debit"

	pathGameHold    = "/v1.0/internal/wallet/game/hold"
	pathGameRelease = "/v1.0/internal/wallet/game/hold/%s/release"
	pathGameCashout = "/v1.0/internal/wallet/game/cashout"
	pathGameStatus  = "/v1.0/internal/wallet/game/status/%s"
	pathBalance     = "/v1.0/internal/wallet/balance/%s"

	scopeCredit      = "internal:wallet:credit"
	scopeDebit       = "internal:wallet:debit"
	scopeDebitReal   = "internal:wallet:debit-real"
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
	debitRealTokens   *oauth2client.TokenManager
	gameHoldTokens    *oauth2client.TokenManager
	gameCashoutTokens *oauth2client.TokenManager
	gameStatusTokens  *oauth2client.TokenManager
	balanceTokens     *oauth2client.TokenManager
	env               string
	breakersMu        sync.Mutex
	breakers          map[string]breakerState
	retryDelay        func(time.Duration)
}

type breakerState struct {
	failures  int
	openUntil time.Time
}

const (
	maxWalletAttempts = 3
	breakerThreshold  = 5
	breakerCooldown   = 15 * time.Second
)

var errCircuitOpen = errors.New("walletclient: circuit open")

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
func New(cfg *config.Config, cacheB cache.Backend) *Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   2 * time.Second,
		ResponseHeaderTimeout: 3 * time.Second,
		ExpectContinueTimeout: time.Second,
		IdleConnTimeout:       60 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
	}
	httpClient := &http.Client{Timeout: 6 * time.Second, Transport: transport}
	baseAuth := strings.TrimRight(cfg.CtechURL, "/")
	base := strings.TrimRight(cfg.WalletURL, "/")
	return &Client{
		base:              base,
		http:              httpClient,
		creditTokens:      oauth2client.New(httpClient, cacheB, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeCredit),
		debitTokens:       oauth2client.New(httpClient, cacheB, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeDebit),
		debitRealTokens:   oauth2client.New(httpClient, cacheB, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeDebitReal),
		gameHoldTokens:    oauth2client.New(httpClient, cacheB, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeGameHold),
		gameCashoutTokens: oauth2client.New(httpClient, cacheB, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeGameCashout),
		gameStatusTokens:  oauth2client.New(httpClient, cacheB, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeGameStatus),
		balanceTokens:     oauth2client.New(httpClient, cacheB, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeBalance),
		env:               cfg.Env,
		breakers:          make(map[string]breakerState),
		retryDelay:        time.Sleep,
	}
}

func retryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func retryAfter(resp *http.Response, fallback time.Duration) time.Duration {
	if resp == nil {
		return fallback
	}
	value := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return min(2*time.Second, time.Duration(seconds)*time.Second)
	}
	if at, err := http.ParseTime(value); err == nil {
		return min(2*time.Second, max(0, time.Until(at)))
	}
	return fallback
}

func (c *Client) breakerAllows(endpoint string, now time.Time) bool {
	c.breakersMu.Lock()
	defer c.breakersMu.Unlock()
	state := c.breakers[endpoint]
	return state.openUntil.IsZero() || !now.Before(state.openUntil)
}

func (c *Client) recordResult(endpoint string, transientFailure bool) {
	c.breakersMu.Lock()
	defer c.breakersMu.Unlock()
	if !transientFailure {
		delete(c.breakers, endpoint)
		return
	}
	state := c.breakers[endpoint]
	state.failures++
	if state.failures >= breakerThreshold {
		state.openUntil = time.Now().Add(breakerCooldown)
		metrics.EmitTableMetric(c.env, "WalletCircuitOpened", 1, map[string]string{"endpoint": endpoint})
	}
	c.breakers[endpoint] = state
}

// do executes one wallet request with a bounded retry budget. Callers may set
// retrySafe only when the operation is intrinsically idempotent (GET/release)
// or carries a non-empty idempotency key.
func (c *Client) do(req *http.Request, retrySafe bool) (*http.Response, error) {
	endpoint := req.URL.Path
	if !c.breakerAllows(endpoint, time.Now()) {
		metrics.EmitTableMetric(c.env, "WalletCircuitOpenRejected", 1, map[string]string{"endpoint": endpoint})
		return nil, errCircuitOpen
	}
	start := time.Now()
	for attempt := 0; attempt < maxWalletAttempts; attempt++ {
		current := req
		if attempt > 0 {
			current = req.Clone(req.Context())
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				current.Body = body
			}
		}
		resp, err := c.http.Do(current)
		transient := err != nil || (resp != nil && retryableStatus(resp.StatusCode))
		if !transient {
			c.recordResult(endpoint, false)
			metrics.EmitTableMetric(c.env, "WalletLatencyMs", float64(time.Since(start).Milliseconds()), map[string]string{"endpoint": endpoint})
			return resp, err
		}
		if !retrySafe || attempt == maxWalletAttempts-1 || req.Context().Err() != nil {
			c.recordResult(endpoint, true)
			return resp, err
		}
		ceiling := min(800*time.Millisecond, 100*time.Millisecond*(1<<attempt))
		delay := retryAfter(resp, time.Duration(rand.Int64N(int64(ceiling)+1)))
		if resp != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
		}
		metrics.EmitTableMetric(c.env, "WalletRetries", 1, map[string]string{"endpoint": endpoint})
		c.retryDelay(delay)
	}
	return nil, errors.New("walletclient: retry loop exhausted")
}

func (c *Client) Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error {
	return c.movement(ctx, c.base+pathSandboxCredit, c.creditTokens, userID, amount, idempotencyKey, reason)
}

func (c *Client) Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error {
	return c.movement(ctx, c.base+pathSandboxDebit, c.debitTokens, userID, amount, idempotencyKey, reason)
}

// DebitReal charges a fixed amount directly against the player's real
// (withdrawable) wallet — used for poker's fixed table-entry fee, which is
// platform revenue and never part of the at-stake game-wallet pot (see
// buyin.Service.BuyIn). ctech-wallet already exposes this endpoint for
// ctech-billing's subscription charges; poker's own M2M client additionally
// needs the internal:wallet:debit-real scope granted in ctech-account before
// this can succeed in any real environment (see this plan's Global Constraints
// — a cross-repo/config blocker, not a code gap here).
func (c *Client) DebitReal(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error {
	return c.movement(ctx, c.base+pathRealDebit, c.debitRealTokens, userID, amount, idempotencyKey, reason)
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

	resp, err := c.do(req, idempotencyKey != "")
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

	resp, err := c.do(req, true)
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

	resp, err := c.do(req, idempotencyKey != "")
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

	resp, err := c.do(req, true)
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

	resp, err := c.do(req, true)
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

	resp, err := c.do(req, idempotencyKey != "")
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
