package tui

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// subFromJWT reads the `sub` claim from a JWT without verifying it — the CLI
// only needs the player id to tag "you" in the table view; the server is the
// authority on identity for every actual action.
func subFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.Sub
}
