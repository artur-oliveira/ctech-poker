// Package auth handles login (PKCE loopback and API-key exchange), token
// refresh, and the on-disk credential store for the CTech Poker CLI.
package auth

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// refreshSkew is how far ahead of the real expiry a token is treated as
// needing refresh, so a request started just before expiry doesn't race it.
const refreshSkew = 60 * time.Second

// Credentials is what the CLI persists after a successful login.
type Credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	APIKey       string    `json:"api_key,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
	// ObtainedVia is "pkce" or "api_key" — it decides how Refresh renews the
	// access token (rotate the refresh token, or re-exchange the API key).
	ObtainedVia string `json:"obtained_via"`
}

// NeedsRefresh reports whether the access token is within refreshSkew of
// expiring (or already past it), as of now.
func (c Credentials) NeedsRefresh(now time.Time) bool {
	return !c.ExpiresAt.After(now.Add(refreshSkew))
}

// LoadCredentials reads path. A missing or unparseable file is reported as
// (zero value, false, nil) — logged out, never a hard error — so a corrupt
// credentials file doesn't crash every command.
func LoadCredentials(path string) (Credentials, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Credentials{}, false, nil
		}
		return Credentials{}, false, err
	}
	var c Credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return Credentials{}, false, nil
	}
	return c, true, nil
}

// SaveCredentials writes c to path, creating its directory (0700) as needed
// and writing the file atomically (temp file + rename) at mode 0600.
func SaveCredentials(path string, c Credentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// ClearCredentials removes path. Absent is success — logout is idempotent.
func ClearCredentials(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
