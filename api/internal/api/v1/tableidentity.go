package v1

import (
	"context"

	"gopkg.aoctech.app/api-commons/observability"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/presence"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
)

// identityCmdFor builds the SetIdentityCmd carrying playerID's complete
// persisted display identity: name, avatar URL and playstyle badge.
//
// All three always travel together on purpose. hand.Table's
// SetPlayerIdentityForActor replaces the three seat fields as one unit, so a
// caller that pushes only the field it happens to care about (a rename, say)
// silently blanks the other two on every seat watching. This is the single
// place that assembles them; the WS gateway (on connect) and the profile
// handler (on rename, #64) both go through it.
//
// Returns ok=false when there is no profile to push — the caller then leaves
// whatever the table already has rather than clearing it.
func identityCmdFor(ctx context.Context, players *player.Service, stats pokerStatsReader, cfg *config.Config, playerID string) (table.SetIdentityCmd, bool) {
	if players == nil {
		return table.SetIdentityCmd{}, false
	}
	profile, err := players.GetOrCreate(ctx, playerID)
	if err != nil || profile == nil {
		if err != nil {
			observability.Warn(ctx, "table identity profile lookup failed", err, "player_id", playerID)
		}
		return table.SetIdentityCmd{}, false
	}
	playstyleBadge := ""
	if profile.PlaystylePublic && stats != nil {
		if playerStats, statsErr := stats.Get(ctx, playerID, roomstore.CurrencyModeSandbox); statsErr == nil {
			if badges := pokerstats.StyleFor(playerStats, pokerstats.MinHandsPublic); len(badges) > 0 {
				playstyleBadge = badges[0].Key
			}
		}
	}
	baseURL := ""
	if cfg != nil {
		baseURL = cfg.AvatarBaseURL
	}
	return table.SetIdentityCmd{
		PlayerID:       playerID,
		Name:           profile.Name,
		AvatarURL:      player.AvatarURL(profile, baseURL),
		PlaystyleBadge: playstyleBadge,
	}, true
}

// tableIdentityPusher pushes a player's current display identity into the
// table they are sitting at right now, without waiting for them to reconnect
// (#64's "a seated player's rename reaches their table"). Presence is the
// fleet-wide answer to "which table"; the actor is then obtained the same way
// every other entry point obtains one — any instance may run any table's
// actor, and SetIdentityCmd commits through the same conditional write, so
// this is safe from whichever instance served the profile request.
type tableIdentityPusher struct {
	manager  *tablemanager.Manager
	seed     func(string) func() *hand.Table
	presence *presence.Service
	players  *player.Service
	stats    pokerStatsReader
	cfg      *config.Config
}

// push is best-effort: a profile update must not fail because a table is
// momentarily unreachable — the player's next connect re-pushes the identity
// anyway, and every read path already resolves names live.
func (p *tableIdentityPusher) push(ctx context.Context, playerID string) {
	if p == nil || p.manager == nil || p.presence == nil || p.seed == nil {
		return
	}
	entries, err := p.presence.GetMany(ctx, []string{playerID})
	if err != nil {
		observability.Warn(ctx, "table identity presence lookup failed", err, "player_id", playerID)
		return
	}
	entry := entries[playerID]
	if entry.Status != presence.StatusInTable || entry.RoomID == "" {
		return
	}
	cmd, ok := identityCmdFor(ctx, p.players, p.stats, p.cfg, playerID)
	if !ok {
		return
	}
	actor, err := p.manager.GetOrCreateActor(ctx, entry.RoomID, p.seed(entry.RoomID))
	if err != nil {
		observability.Warn(ctx, "table identity actor lookup failed", err, "table_id", entry.RoomID)
		return
	}
	cmd.Reply = make(chan error, 1)
	if err := actor.Dispatch(cmd); err != nil {
		observability.Warn(ctx, "table identity dispatch failed", err, "table_id", entry.RoomID)
	}
}
