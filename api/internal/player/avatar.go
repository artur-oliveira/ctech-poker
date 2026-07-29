package player

import "strings"

// AvatarURL returns the public, versioned URL without exposing storage keys.
// A blocked avatar is indistinguishable from a missing avatar to clients.
func AvatarURL(profile *PlayerProfile, baseURL string) string {
	if profile == nil || profile.AvatarBlocked || profile.AvatarKey == "" || baseURL == "" {
		return ""
	}
	key := strings.TrimPrefix(profile.AvatarKey, "av/")
	return strings.TrimRight(baseURL, "/") + "/" + key
}
