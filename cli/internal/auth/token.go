package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrAuthFailed wraps any error the account token endpoint reports (a
// standard OAuth `error`/`error_description` pair, or a non-2xx with no
// parseable body).
var ErrAuthFailed = errors.New("authentication failed")

// TokenClient talks to ctech-account's POST /v1.0/token.
type TokenClient struct {
	baseURL  string
	clientID string
	hc       *http.Client
}

func NewTokenClient(accountBaseURL, clientID string, hc *http.Client) *TokenClient {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &TokenClient{baseURL: strings.TrimRight(accountBaseURL, "/"), clientID: clientID, hc: hc}
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (t *TokenClient) postForm(ctx context.Context, form url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/v1.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.hc.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil && resp.StatusCode >= 400 {
		return tokenResponse{}, fmt.Errorf("%w: http %d", ErrAuthFailed, resp.StatusCode)
	}
	if tr.Error != "" || resp.StatusCode >= 400 {
		return tokenResponse{}, fmt.Errorf("%w: %s: %s", ErrAuthFailed, tr.Error, tr.ErrorDescription)
	}
	return tr, nil
}

func credentialsFrom(tr tokenResponse, via string) Credentials {
	return Credentials{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		ObtainedVia:  via,
	}
}

// ExchangeAPIKey trades a long-lived API key for a short-lived access token
// (grant_type=api_key). The key itself is kept on the returned Credentials so
// Refresh can re-exchange it later — this grant issues no refresh token.
func (t *TokenClient) ExchangeAPIKey(ctx context.Context, apiKey string) (Credentials, error) {
	tr, err := t.postForm(ctx, url.Values{
		"grant_type": {"api_key"},
		"api_key":    {apiKey},
		"client_id":  {t.clientID},
	})
	if err != nil {
		return Credentials{}, err
	}
	c := credentialsFrom(tr, "api_key")
	c.APIKey = apiKey
	return c, nil
}

// ExchangeCode completes the PKCE authorization_code grant.
func (t *TokenClient) ExchangeCode(ctx context.Context, code, verifier, redirectURI string) (Credentials, error) {
	tr, err := t.postForm(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
		"client_id":     {t.clientID},
	})
	if err != nil {
		return Credentials{}, err
	}
	return credentialsFrom(tr, "pkce"), nil
}

// Refresh renews an access token: rotating refresh_token for a PKCE-obtained
// credential, or re-exchanging the stored API key for one obtained that way.
func (t *TokenClient) Refresh(ctx context.Context, c Credentials) (Credentials, error) {
	switch c.ObtainedVia {
	case "api_key":
		return t.ExchangeAPIKey(ctx, c.APIKey)
	default: // "pkce"
		tr, err := t.postForm(ctx, url.Values{
			"grant_type":    {"refresh_token"},
			"refresh_token": {c.RefreshToken},
			"client_id":     {t.clientID},
		})
		if err != nil {
			return Credentials{}, err
		}
		return credentialsFrom(tr, "pkce"), nil
	}
}
