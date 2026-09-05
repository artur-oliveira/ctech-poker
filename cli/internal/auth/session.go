package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gopkg.aoctech.app/poker/cli/internal/config"
)

// ErrLoggedOut is returned by Session.Token when no credentials are stored.
var ErrLoggedOut = errors.New("not logged in — run `poker login`")

// pkceLoginTimeout bounds how long LoginPKCE waits for the browser round trip.
const pkceLoginTimeout = 120 * time.Second

// LoopbackPort is the fixed 127.0.0.1 port the PKCE callback binds. It must
// be a fixed value — ctech-account's IsRedirectURIAllowed
// (internal/domain/oauth/client/model.go) is an exact string match with no
// RFC 8252 §7.3 port-agnostic loopback comparison, so
// `http://127.0.0.1:<LoopbackPort>/callback` has to be registered on the
// poker-cli client verbatim.
//
// Known trade-off of a fixed (vs. ephemeral) port: another local process can
// bind it first, which denies this login attempt (NewLoopbackReceiver's Listen
// fails, surfaced as a clear error) but cannot steal the authorization code —
// the PKCE code_verifier that turns a code into a token never leaves this
// process, and the random `state` is checked before the code is used. Same-user
// local processes are outside this CLI's threat model (RFC 8252's own model for
// native-app loopback redirects); this only widens a pre-existing DoS surface,
// not a credential-theft one.
const LoopbackPort = 51789

// readScopes are the read-only poker:* scopes the poker-cli OAuth client is
// granted (docs/specs/2026-09-05-poker-cli.md §2 / cli/CLAUDE.md).
const readScopes = "poker:rooms:read poker:players:read poker:sessions:read poker:hands:read poker:achievements:read poker:stats:read"

// Session composes the credential store with a TokenClient, and is the one
// thing the rest of the CLI depends on for a valid bearer token.
type Session struct {
	cfg   config.Settings
	token *TokenClient
	path  string
}

func NewSession(cfg config.Settings, hc *http.Client) *Session {
	return &Session{
		cfg:   cfg,
		token: NewTokenClient(cfg.AccountBaseURL, cfg.ClientID, hc),
		path:  config.CredentialsPath(cfg),
	}
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// LoginPKCE runs the full RFC 8252 loopback flow: a local callback server,
// the browser round trip (via openBrowser, invoked in the background so a
// blocking fake — or a slow real browser launch — can never deadlock this
// call), and the code-for-token exchange. Saves the result on success.
func (s *Session) LoginPKCE(ctx context.Context, openBrowser func(url string) error) error {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		return err
	}
	state, err := randomState()
	if err != nil {
		return err
	}

	receiver, err := NewLoopbackReceiver(LoopbackPort)
	if err != nil {
		return fmt.Errorf("cannot bind the login callback on 127.0.0.1:%d (another `poker login` running?): %w", LoopbackPort, err)
	}
	defer func(receiver *LoopbackReceiver) {
		_ = receiver.Close()
	}(receiver)
	redirectURI := receiver.RedirectURI()

	authorizeURL := strings.TrimRight(s.cfg.AccountBaseURL, "/") + "/v1.0/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {s.cfg.ClientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"scope":                 {readScopes},
		"state":                 {state},
	}.Encode()

	go func() { _ = openBrowser(authorizeURL) }()

	waitCtx, cancel := context.WithTimeout(ctx, pkceLoginTimeout)
	defer cancel()
	code, err := receiver.Wait(waitCtx, state)
	if err != nil {
		return err
	}

	creds, err := s.token.ExchangeCode(ctx, code, verifier, redirectURI)
	if err != nil {
		return err
	}
	return SaveCredentials(s.path, creds)
}

// LoginAPIKey exchanges apiKey for an access token and saves it.
func (s *Session) LoginAPIKey(ctx context.Context, apiKey string) error {
	creds, err := s.token.ExchangeAPIKey(ctx, apiKey)
	if err != nil {
		return err
	}
	return SaveCredentials(s.path, creds)
}

// Logout removes any stored credentials. Idempotent.
func (s *Session) Logout() error {
	return ClearCredentials(s.path)
}

// Token returns a valid bearer access token, refreshing (and persisting the
// refreshed credentials) first if the stored one is near expiry.
func (s *Session) Token(ctx context.Context) (string, error) {
	creds, ok, err := LoadCredentials(s.path)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrLoggedOut
	}
	if creds.NeedsRefresh(time.Now()) {
		creds, err = s.token.Refresh(ctx, creds)
		if err != nil {
			return "", err
		}
		if err := SaveCredentials(s.path, creds); err != nil {
			return "", err
		}
	}
	return creds.AccessToken, nil
}
