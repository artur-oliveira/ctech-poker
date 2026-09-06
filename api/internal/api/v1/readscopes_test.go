package v1

import (
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/poker/api/internal/oauthresource"
)

func TestReadScopeManifestMatchesEnforcement(t *testing.T) {
	manifest, err := oauthresource.ManifestDocument()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 || manifest.ResourceServerID != "poker" {
		t.Fatalf("unexpected manifest identity: %+v", manifest)
	}
	want := append([]string(nil), publicReadScopes...)
	got := make([]string, 0, len(manifest.Scopes))
	for _, scope := range manifest.Scopes {
		if scope.Visibility != "public" || scope.Status != "active" {
			t.Fatalf("scope %q must be public and active", scope.Name)
		}
		if scope.Descriptions["en"] == "" || scope.Descriptions["pt-BR"] == "" {
			t.Fatalf("scope %q lacks bilingual descriptions", scope.Name)
		}
		if len(scope.Name) < len(":read") || scope.Name[len(scope.Name)-len(":read"):] != ":read" {
			t.Fatalf("non-read scope must never be published: %q", scope.Name)
		}
		got = append(got, scope.Name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("manifest=%v enforcement=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("manifest/enforcement drift: got=%v want=%v", got, want)
		}
	}
}

func TestScopedPokerTokensAreReadOnlyAndResourceBound(t *testing.T) {
	tests := []struct {
		name, method, path, claim, azp string
		want                           int
	}{
		{"room read", "GET", "/v1.0/rooms", ScopeRoomsRead, "api-key", 200},
		{"wrong resource", "GET", "/v1.0/rooms", ScopePlayersRead, "api-key", 403},
		{"daily reward cooldown", "GET", "/v1.0/sandbox-credits/", ScopeDailyRewardRead, firstPartyPokerClientID, 200},
		{"daily reward cooldown without trailing slash", "GET", "/v1.0/sandbox-credits", ScopeDailyRewardRead, firstPartyPokerClientID, 200},
		{"daily reward rejects purchase scope", "GET", "/v1.0/sandbox-credits/", ScopeSandboxPurchasesRead, firstPartyPokerClientID, 403},
		{"player notes", "GET", "/v1.0/players/me/notes/", ScopePlayerNotesRead, firstPartyPokerClientID, 200},
		{"player notes reject generic player scope", "GET", "/v1.0/players/me/notes/", ScopePlayersRead, firstPartyPokerClientID, 403},
		{"sandbox purchase list", "GET", "/v1.0/wallet/sandbox-purchase/", ScopeSandboxPurchasesRead, firstPartyPokerClientID, 200},
		{"reaction purchase catalog", "GET", "/v1.0/wallet/reaction-purchase/catalog", ScopeReactionPurchasesRead, firstPartyPokerClientID, 200},
		{"social read is scope-exempt for first-party UI", "GET", "/v1.0/social/friends", ScopeRoomsRead, firstPartyPokerClientID, 200},
		{"social summary is scope-exempt for first-party UI", "GET", "/v1.0/social/summary", ScopePlayersRead, firstPartyPokerClientID, 200},
		{"API key cannot create room", "POST", "/v1.0/rooms", ScopeRoomsRead, "api-key", 403},
		{"third-party client cannot buy", "POST", "/v1.0/sandbox-purchases", ScopeSandboxPurchasesRead, "third-party", 403},
		{"scoped first-party UI can create room", "POST", "/v1.0/rooms", ScopeRoomsRead, firstPartyPokerClientID, 200},
		{"legacy first-party UI session remains interactive", "POST", "/v1.0/rooms", "openid profile", firstPartyPokerClientID, 200},
		{"unscoped third-party session is not interactive", "POST", "/v1.0/rooms", "openid profile", "third-party", 403},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := fiber.New()
			claims := &jwtverify.Claims{Sub: "player", SID: "session", Scope: test.claim, AZP: test.azp}
			handler := func(c fiber.Ctx) error {
				if denied := enforceReadOnlyScope(c, claims); denied != nil {
					return denied.Send(c)
				}
				return c.SendStatus(200)
			}
			if test.method == fiber.MethodGet {
				app.Get(test.path, handler)
			} else {
				app.Post(test.path, handler)
			}
			resp, err := app.Test(httptest.NewRequest(test.method, test.path, nil))
			if err != nil || resp.StatusCode != test.want {
				t.Fatalf("status=%d want=%d err=%v", resp.StatusCode, test.want, err)
			}
		})
	}
}

func TestOnlyFirstPartyPokerSessionsAuthorizeWebSocket(t *testing.T) {
	tests := []struct {
		name   string
		claims *jwtverify.Claims
		want   bool
	}{
		{"scoped Poker UI", &jwtverify.Claims{SID: "session", AZP: firstPartyPokerClientID, Scope: ScopeRoomsRead}, true},
		{"legacy Poker UI", &jwtverify.Claims{SID: "session", AZP: firstPartyPokerClientID, Scope: "openid profile"}, true},
		{"poker-cli with a user session", &jwtverify.Claims{SID: "session", AZP: "poker-cli", Scope: ScopeRoomsRead}, true},
		{"API key", &jwtverify.Claims{SID: "session", AZP: "api-key", Scope: ScopeRoomsRead}, false},
		{"third-party client", &jwtverify.Claims{SID: "session", AZP: "third-party", Scope: ScopeRoomsRead}, false},
		{"M2M Poker client", &jwtverify.Claims{AZP: firstPartyPokerClientID, Scope: ScopeRoomsRead}, false},
		{"M2M poker-cli client", &jwtverify.Claims{AZP: "poker-cli", Scope: ScopeRoomsRead}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isFirstPartyPokerSession(test.claims); got != test.want {
				t.Fatalf("isFirstPartyPokerSession()=%v want=%v", got, test.want)
			}
		})
	}
}
