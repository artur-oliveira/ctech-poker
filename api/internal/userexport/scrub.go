package userexport

import "strings"

// idKeys name the attribute that says whose a nested record is
// (hand.Player.id, OpponentSummary.player_id, ...). Same keys as userpurge.
var idKeys = []string{"id", "player_id"}

// othersPII are dropped from any nested record that names someone other than
// the person: display name, avatar, badge and chat text (E4: other people appear
// as opaque ids).
var othersPII = []string{"name", "avatar_url", "playstyle_badge", "message"}

// secretFragments drop any attribute whose name contains one, at any depth: a
// defensive net, poker stores none of these today.
var secretFragments = []string{"password", "secret", "token", "cpf"}

func isSecret(key string) bool {
	k := strings.ToLower(key)
	for _, f := range secretFragments {
		if strings.Contains(k, f) {
			return true
		}
	}
	return false
}

func namesOther(m map[string]any, sub string) bool {
	for _, k := range idKeys {
		if id, ok := m[k].(string); ok && id != "" && id != sub {
			return true
		}
	}
	return false
}

// scrub returns v without secrets and without other players' personal data.
func scrub(v any, sub string) any {
	switch t := v.(type) {
	case map[string]any:
		other := namesOther(t, sub)
		out := make(map[string]any, len(t))
		for k, e := range t {
			if isSecret(k) {
				continue
			}
			if other && contains(othersPII, k) {
				continue
			}
			out[k] = scrub(e, sub)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = scrub(e, sub)
		}
		return out
	}
	return v
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}
