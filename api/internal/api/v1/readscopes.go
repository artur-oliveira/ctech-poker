package v1

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

// Public delegated and API-key scopes are intentionally read-only. There are
// no grantable scopes for creating/joining a table, sending game actions,
// claiming rewards, mutating profiles/notes/shares, refunds, or buying
// anything. Those interactive operations are restricted to access tokens
// issued to Poker's first-party public SPA client.
const (
	firstPartyPokerClientID    = "poker"
	ScopeRoomsRead             = "poker:rooms:read"
	ScopePlayersRead           = "poker:players:read"
	ScopeSessionsRead          = "poker:sessions:read"
	ScopeHandsRead             = "poker:hands:read"
	ScopeAchievementsRead      = "poker:achievements:read"
	ScopeStatsRead             = "poker:stats:read"
	ScopeLeaderboardRead       = "poker:leaderboard:read"
	ScopeDailyRewardRead       = "poker:daily-reward:read"
	ScopePlayerNotesRead       = "poker:player-notes:read"
	ScopeSandboxPurchasesRead  = "poker:sandbox-purchases:read"
	ScopeReactionPurchasesRead = "poker:reaction-purchases:read"
)

var publicReadScopes = []string{
	ScopeRoomsRead, ScopePlayersRead, ScopeSessionsRead, ScopeHandsRead,
	ScopeAchievementsRead, ScopeStatsRead, ScopeLeaderboardRead,
	ScopeDailyRewardRead, ScopePlayerNotesRead, ScopeSandboxPurchasesRead,
	ScopeReactionPurchasesRead,
}

func hasPokerScope(claims *jwtverify.Claims) bool {
	for _, scope := range claims.Scopes() {
		if strings.HasPrefix(scope, "poker:") {
			return true
		}
	}
	return false
}

func isFirstPartyPokerSession(claims *jwtverify.Claims) bool {
	return claims != nil && claims.SID != "" && claims.AZP == firstPartyPokerClientID
}

// enforceReadOnlyScope keeps public delegated/API-key grants read-only while
// allowing interactive operations only for the first-party Poker UI client.
// Legacy UI tokens without poker:* scopes remain accepted during rollout.
func enforceReadOnlyScope(c fiber.Ctx, claims *jwtverify.Claims) *problem.Problem {
	if c.Method() != fiber.MethodGet {
		if isFirstPartyPokerSession(claims) {
			return nil
		}
		return problem.Forbidden("interactive poker operations require the first-party Poker client")
	}
	if !hasPokerScope(claims) {
		return nil
	}
	want := requiredReadScope(c.Path())
	if want == "" || !claims.HasScope(want) {
		return problem.Forbidden("scope does not grant this poker resource")
	}
	return nil
}

func requiredReadScope(path string) string {
	switch {
	case strings.HasPrefix(path, "/v1.0/rooms"):
		return ScopeRoomsRead
	case strings.HasPrefix(path, "/v1.0/tables/") && strings.HasSuffix(path, "/history"):
		return ScopeHandsRead
	case strings.HasPrefix(path, "/v1.0/players/me/sessions"):
		return ScopeSessionsRead
	case strings.HasPrefix(path, "/v1.0/players/me/hands"), strings.HasPrefix(path, "/v1.0/players/me/hand/"):
		return ScopeHandsRead
	case strings.HasPrefix(path, "/v1.0/players/me/achievements"):
		return ScopeAchievementsRead
	case strings.HasPrefix(path, "/v1.0/players/me/poker-stats"):
		return ScopeStatsRead
	case strings.HasPrefix(path, "/v1.0/players"):
		return ScopePlayersRead
	case strings.HasPrefix(path, "/v1.0/leaderboard"):
		return ScopeLeaderboardRead
	case strings.HasPrefix(path, "/v1.0/daily-reward"):
		return ScopeDailyRewardRead
	case strings.HasPrefix(path, "/v1.0/player-notes"):
		return ScopePlayerNotesRead
	case strings.HasPrefix(path, "/v1.0/sandbox-purchases"):
		return ScopeSandboxPurchasesRead
	case strings.HasPrefix(path, "/v1.0/reaction-purchases"):
		return ScopeReactionPurchasesRead
	default:
		return ""
	}
}
