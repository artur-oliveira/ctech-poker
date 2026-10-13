package userpurge

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// idKeys name the attribute that says whose a nested record is
// (hand.Player.id, OpponentSummary.player_id, ReplaySeat.player_id, chat
// entries, action-log rows).
var idKeys = []string{"id", "player_id"}

// piiKeys are dropped from any record whose idKeys name the erased player:
// display name, avatar URL, playstyle badge and chat text.
var piiKeys = []string{"name", "avatar_url", "playstyle_badge", "message"}

// newPseudonym is the random id that replaces one erased player in other
// players' rows. It lives in memory for one purge run only and is never
// written next to the sub (data inventory §7, plan ruling R8).
func newPseudonym() string { return "anon_" + strings.ToLower(rand.Text()) }

func str(v string) types.AttributeValue { return &types.AttributeValueMemberS{Value: v} }

// anonymizeAV replaces sub with pseudo in every string, string-set element,
// list element, map value and map key of v, and drops piiKeys from every map
// that identifies sub. It reports whether anything changed.
func anonymizeAV(v types.AttributeValue, sub, pseudo string) (types.AttributeValue, bool) {
	switch t := v.(type) {
	case *types.AttributeValueMemberS:
		if strings.Contains(t.Value, sub) {
			return str(strings.ReplaceAll(t.Value, sub, pseudo)), true
		}
	case *types.AttributeValueMemberSS:
		out, changed := make([]string, len(t.Value)), false
		for i, s := range t.Value {
			out[i] = strings.ReplaceAll(s, sub, pseudo)
			changed = changed || out[i] != s
		}
		if changed {
			return &types.AttributeValueMemberSS{Value: out}, true
		}
	case *types.AttributeValueMemberL:
		out, changed := make([]types.AttributeValue, len(t.Value)), false
		for i, e := range t.Value {
			var c bool
			out[i], c = anonymizeAV(e, sub, pseudo)
			changed = changed || c
		}
		if changed {
			return &types.AttributeValueMemberL{Value: out}, true
		}
	case *types.AttributeValueMemberM:
		if out, changed := anonymizeMap(t.Value, sub, pseudo); changed {
			return &types.AttributeValueMemberM{Value: out}, true
		}
	}
	return v, false
}

// anonymizeMap is anonymizeAV for a map, which is also a whole item (keys included).
func anonymizeMap(m map[string]types.AttributeValue, sub, pseudo string) (map[string]types.AttributeValue, bool) {
	identifies := false
	for _, k := range idKeys {
		if s, ok := m[k].(*types.AttributeValueMemberS); ok && s.Value == sub {
			identifies = true
		}
	}
	out, changed := make(map[string]types.AttributeValue, len(m)), false
	for k, v := range m {
		if identifies && slices.Contains(piiKeys, k) {
			changed = true
			continue
		}
		nv, c := anonymizeAV(v, sub, pseudo)
		nk := strings.ReplaceAll(k, sub, pseudo)
		changed = changed || c || nk != k
		out[nk] = nv
	}
	return out, changed
}

// anonymizeItem rewrites a stored item in place: pk and sk keep their
// values, so a Put lands on the same item. Rows whose key holds the sub are
// re-keyed instead (run.rekey).
func anonymizeItem(item map[string]types.AttributeValue, sub, pseudo string) (map[string]types.AttributeValue, bool) {
	body := maps.Clone(item)
	delete(body, "pk")
	delete(body, "sk")
	out, changed := anonymizeMap(body, sub, pseudo)
	for _, k := range []string{"pk", "sk"} {
		if v, ok := item[k]; ok {
			out[k] = v
		}
	}
	return out, changed
}

// anonymizeJSON applies the same rules to decoded JSON.
func anonymizeJSON(v any, sub, pseudo string) (any, bool) {
	switch t := v.(type) {
	case string:
		if strings.Contains(t, sub) {
			return strings.ReplaceAll(t, sub, pseudo), true
		}
	case []any:
		out, changed := make([]any, len(t)), false
		for i, e := range t {
			var c bool
			out[i], c = anonymizeJSON(e, sub, pseudo)
			changed = changed || c
		}
		if changed {
			return out, true
		}
	case map[string]any:
		identifies := false
		for _, k := range idKeys {
			if s, ok := t[k].(string); ok && s == sub {
				identifies = true
			}
		}
		out, changed := make(map[string]any, len(t)), false
		for k, e := range t {
			if identifies && slices.Contains(piiKeys, k) {
				changed = true
				continue
			}
			ne, c := anonymizeJSON(e, sub, pseudo)
			nk := strings.ReplaceAll(k, sub, pseudo)
			changed = changed || c || nk != k
			out[nk] = ne
		}
		if changed {
			return out, true
		}
	}
	return v, false
}

// anonymizeJSONLines rewrites one action-log archive object (JSON Lines, see
// cmd/archiver). Numbers stay json.Number: the archive is the audit trail of
// every chip amount and must not lose precision past 2^53.
func anonymizeJSONLines(body []byte, sub, pseudo string) ([]byte, bool, error) {
	var out bytes.Buffer
	changed := false
	for _, line := range bytes.Split(body, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			return nil, false, err
		}
		nv, c := anonymizeJSON(v, sub, pseudo)
		changed = changed || c
		enc, err := json.Marshal(nv)
		if err != nil {
			return nil, false, err
		}
		out.Write(enc)
		out.WriteByte('\n')
	}
	return out.Bytes(), changed, nil
}
