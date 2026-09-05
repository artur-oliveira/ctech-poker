package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
)

// verifierBytes yields a base64url string of 43 chars (RFC 7636's minimum),
// comfortably inside the [43,128] range the spec requires.
const verifierBytes = 32

// GeneratePKCE returns a fresh code_verifier and its S256 code_challenge
// (RFC 7636).
func GeneratePKCE() (verifier, challenge string, err error) {
	raw := make([]byte, verifierBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// LoopbackReceiver is a one-shot local HTTP server that catches the OAuth
// authorization-code redirect on 127.0.0.1 (RFC 8252 §7.3).
type LoopbackReceiver struct {
	ln net.Listener
}

// NewLoopbackReceiver binds 127.0.0.1:port immediately (port 0 picks an
// ephemeral one) so RedirectURI is known before the browser ever opens.
func NewLoopbackReceiver(port int) (*LoopbackReceiver, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	return &LoopbackReceiver{ln: ln}, nil
}

// RedirectURI is the exact redirect_uri to send in the authorize request.
func (r *LoopbackReceiver) RedirectURI() string {
	return fmt.Sprintf("http://%s/callback", r.ln.Addr().String())
}

// Wait serves exactly one /callback request, validates its state parameter
// against wantState, and returns the authorization code. It returns early if
// ctx is done first.
func (r *LoopbackReceiver) Wait(ctx context.Context, wantState string) (string, error) {
	type result struct {
		code string
		err  error
	}
	resultCh := make(chan result, 1)

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			q := req.URL.Query()
			if oauthErr := q.Get("error"); oauthErr != "" {
				_, _ = fmt.Fprintln(w, "Login failed. You can close this tab.")
				resultCh <- result{err: fmt.Errorf("%w: %s: %s", ErrAuthFailed, oauthErr, q.Get("error_description"))}
				return
			}
			if q.Get("state") != wantState {
				_, _ = fmt.Fprintln(w, "Login failed (state mismatch). You can close this tab.")
				resultCh <- result{err: errors.New("oauth callback: state mismatch")}
				return
			}
			_, _ = fmt.Fprintln(w, "Login successful. You can close this tab.")
			resultCh <- result{code: q.Get("code")}
		}),
	}
	go func() {
		_ = srv.Serve(r.ln)
	}()
	defer func(srv *http.Server) {
		_ = srv.Close()
	}(srv)

	select {
	case res := <-resultCh:
		return res.code, res.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Close releases the listener if Wait was never called (or errored before
// serving). Safe to call after Wait too.
func (r *LoopbackReceiver) Close() error {
	return r.ln.Close()
}
