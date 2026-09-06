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

// nameFromJWT reads the `name` claim, when present — some providers embed a
// display name directly, sparing the extra /profile round trip. Most OAuth
// access tokens carry none, in which case this returns "".
func nameFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.Name
}
