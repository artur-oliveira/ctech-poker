package v1

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/problem"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

// earnedGrantTimeout bounds the self-heal before the owned list is read: a
// slow progress/entitlement read must never slow the picker materially. A var
// so the timeout test runs in milliseconds.
var earnedGrantTimeout = 3 * time.Second

// now is the clock for sale-window checks; a var as the test seam.
var now = time.Now

// avatarOwnership is the slice of *cosmeticpurchase.Service this route uses;
// an interface so the route and its self-heal test without DynamoDB.
type avatarOwnership interface {
	OwnedIDs(ctx context.Context, playerID string, kind cosmetics.Kind) (map[string]bool, error)
	Grant(ctx context.Context, playerID string, kind cosmetics.Kind, itemID, reason string) (bool, error)
}

type achievementProgressReader interface {
	AllAchievements(ctx context.Context, playerID, mode string) ([]achievements.PlayerAchievementProgress, error)
}

// RegisterAvatarCosmeticsOwnership mounts the one read the avatar equip UI
// (#292) needs: which ids of a given avatar kind (frame, hat, accessory,
// badge) this player owns. Deliberately not RegisterCosmeticPurchase's
// `/wallet/cosmetic-purchase` group — that group's :kind param, catalog and
// create/refund routes are wallet-priced deck/felt purchases.
//
// With progress non-nil, the read first self-heals achievement items
// (grantEarned): the only recovery path for a grant the hand pipeline lost.
// nil progress keeps the plain list.
func RegisterAvatarCosmeticsOwnership(router fiber.Router, auth fiber.Handler, svc avatarOwnership, progress achievementProgressReader) {
	router.Get("/players/me/cosmetics/:kind/owned", auth, validAvatarKind, func(c fiber.Ctx) error {
		kind := cosmetics.Kind(c.Params("kind"))
		userID := c.Locals(localsUserID).(string)
		if progress != nil {
			ctx, cancel := context.WithTimeout(c.Context(), earnedGrantTimeout)
			if err := grantEarned(ctx, progress, svc, userID); err != nil {
				slog.Warn("avatar cosmetics self-heal incomplete", "player_id", userID, "error", err)
			}
			cancel()
		}
		owned, err := svc.OwnedIDs(c.Context(), userID, kind)
		if err != nil {
			return internalProblem("list owned cosmetics failed", err).Send(c)
		}
		ids := make([]string, 0, len(owned))
		for id := range owned {
			ids = append(ids, id)
		}
		return sendPage(c, ids, nil, "")
	})
}

// grantEarned grants every cosmetics.AchievementGrants row the player's
// progress qualifies for and does not already own (reason
// achievement:<key>:<stars>:backfill). Stars per key are the max across
// sandbox and real: a tier earned in either mode earns the item, as in the
// hand pipeline. Every step is best-effort: a failed mode read or Grant is
// logged and the rest still run; the first error is returned for the log only.
// Steady state: two progress reads, one OwnedIDs per kind with a qualifying
// row, zero writes.
// ponytail: two progress reads per owned-list call (profile pickers only); add a short per-player cooldown only if measured.
func grantEarned(ctx context.Context, progress achievementProgressReader, svc avatarOwnership, playerID string) error {
	var firstErr error
	note := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	stars := map[string]int{}
	for _, mode := range []string{roomstore.CurrencyModeSandbox, roomstore.CurrencyModeReal} {
		rows, err := progress.AllAchievements(ctx, playerID, mode)
		if err != nil {
			slog.Warn("avatar cosmetics self-heal: progress read failed", "player_id", playerID, "mode", mode, "error", err)
			note(fmt.Errorf("progress %s: %w", mode, err))
			continue
		}
		for _, row := range rows {
			s := achievements.StarsFor(row.Key, row.Count)
			// Reset counters (folded_streak) can fall below a tier already
			// earned; the unlock stamp still proves the first tier was crossed.
			if s == 0 && row.UnlockedAt != "" {
				s = 1
			}
			stars[row.Key] = max(stars[row.Key], s)
		}
	}
	owned := map[cosmetics.Kind]map[string]bool{}
	t := now()
	for _, row := range cosmetics.AchievementGrants {
		earned := stars[row.Key]
		// Outside its window Grant would only fail (and WARN) on every call.
		if earned < row.Stars || !cosmetics.Available(row.Kind, row.ID, t) {
			continue
		}
		ids, ok := owned[row.Kind]
		if !ok {
			var err error
			if ids, err = svc.OwnedIDs(ctx, playerID, row.Kind); err != nil {
				slog.Warn("avatar cosmetics self-heal: ownership read failed", "player_id", playerID, "kind", row.Kind, "error", err)
				note(fmt.Errorf("owned %s: %w", row.Kind, err))
				continue // retry the read for this kind's next row; Grant stays idempotent either way
			}
			owned[row.Kind] = ids
		}
		if ids[row.ID] {
			continue
		}
		reason := fmt.Sprintf("achievement:%s:%d:backfill", row.Key, earned)
		if _, err := svc.Grant(ctx, playerID, row.Kind, row.ID, reason); err != nil {
			slog.Warn("avatar cosmetics self-heal: grant failed", "player_id", playerID, "kind", row.Kind, "item_id", row.ID, "error", err)
			note(fmt.Errorf("grant %s/%s: %w", row.Kind, row.ID, err))
		}
	}
	return firstErr
}

func validAvatarKind(c fiber.Ctx) error {
	kind := cosmetics.Kind(c.Params("kind"))
	if !cosmetics.IsAvatarKind(kind) {
		return problem.BadRequest("kind must be \"frame\", \"hat\", \"accessory\" or \"badge\"").Send(c)
	}
	return c.Next()
}
