package walletclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponsibleExportPagesWithTheScopedToken(t *testing.T) {
	var seen []string
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("scope") != scopeResponsibleExport {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"export-token","expires_in":3600}`))
	})
	mux.HandleFunc("/v1.0/internal/wallet/responsible/export", func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Query().Get("cursor")+"|"+r.Header.Get("Authorization"))
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"users":[{"user_id":"u1","gambling_addendum_version":"2.1"}],"carried_exclusions":[],"next_cursor":"c2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"users":[],"carried_exclusions":[{"cpf_hmac":"h","exclusion":{"period":"indefinite"},"expires_at":1}],"next_cursor":""}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := newTestClient(srv)

	first, err := c.ResponsibleExport(t.Context(), "")
	if err != nil || len(first.Users) != 1 || first.Users[0].UserID != "u1" || first.NextCursor != "c2" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := c.ResponsibleExport(t.Context(), "c2")
	if err != nil || len(second.CarriedExclusions) != 1 || second.CarriedExclusions[0].CPFHMAC != "h" {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if len(seen) != 2 || seen[0] != "|Bearer export-token" || seen[1] != "c2|Bearer export-token" {
		t.Fatalf("requests=%v", seen)
	}
}
