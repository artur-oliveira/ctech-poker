package botcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"uuid"

	"gopkg.aoctech.app/api-commons/observability"
)

const (
	siteverifyURL   = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	turnstileAction = "poker_bot_check"
	maxTokenLength  = 2048
)

var ErrVerificationFailed = errors.New("botcheck: verification failed")

type Service struct {
	secret           string
	expectedHostname string
	endpoint         string
	client           *http.Client
}

type verifyRequest struct {
	Secret         string `json:"secret"`
	Response       string `json:"response"`
	RemoteIP       string `json:"remoteip,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

type verifyResponse struct {
	Success  bool     `json:"success"`
	Hostname string   `json:"hostname"`
	Action   string   `json:"action"`
	Errors   []string `json:"error-codes"`
}

func New(secret, expectedHostname string) *Service {
	return &Service{
		secret: strings.TrimSpace(secret), expectedHostname: strings.TrimSpace(expectedHostname),
		endpoint: siteverifyURL, client: &http.Client{Timeout: 6 * time.Second},
	}
}

func (s *Service) Enabled() bool { return s != nil && s.secret != "" }

// Verify performs mandatory server-side validation. Turnstile tokens are
// single-use and expire after five minutes; no successful token is cached.
func (s *Service) Verify(ctx context.Context, token, remoteIP string) error {
	if !s.Enabled() {
		return errors.New("botcheck: service disabled")
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxTokenLength {
		return ErrVerificationFailed
	}
	body, err := json.Marshal(verifyRequest{
		Secret: s.secret, Response: token, RemoteIP: remoteIP, IdempotencyKey: uuid.New().String(),
	})
	if err != nil {
		return fmt.Errorf("botcheck: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("botcheck: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("botcheck: siteverify: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			observability.Warn(ctx, "botcheck response body close failed", closeErr)
		}
	}()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if err != nil {
		return fmt.Errorf("botcheck: read response: %w", err)
	}
	var result verifyResponse
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || json.Unmarshal(raw, &result) != nil {
		return ErrVerificationFailed
	}
	if !result.Success || result.Action != turnstileAction {
		return ErrVerificationFailed
	}
	if s.expectedHostname != "" && result.Hostname != s.expectedHostname {
		return ErrVerificationFailed
	}
	return nil
}

// SetTransportForTest avoids a network listener in unit tests.
func (s *Service) SetTransportForTest(transport http.RoundTripper) {
	s.client.Transport = transport
}
