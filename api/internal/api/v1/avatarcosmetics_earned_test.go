package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

type fakeProgress struct {
	rows  map[string][]achievements.PlayerAchievementProgress
	errs  map[string]error
	block bool
	reads map[string]int
}

func (f *fakeProgress) AllAchievements(ctx context.Context, _ string, mode string) ([]achievements.PlayerAchievementProgress, error) {
	if f.reads == nil {
		f.reads = map[string]int{}
	}
	f.reads[mode]++
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.rows[mode], f.errs[mode]
}

type fakeOwnership struct {
	owned    map[cosmetics.Kind]map[string]bool
	grantErr error
	reasons  []string
}

func (f *fakeOwnership) OwnedIDs(_ context.Context, _ string, kind cosmetics.Kind) (map[string]bool, error) {
	out := map[string]bool{}
	for id := range f.owned[kind] {
		out[id] = true
	}
	return out, nil
}

func (f *fakeOwnership) Grant(_ context.Context, _ string, kind cosmetics.Kind, itemID, reason string) (bool, error) {
	f.reasons = append(f.reasons, string(kind)+"/"+itemID+" "+reason)
	if f.grantErr != nil {
		return false, f.grantErr
	}
	if f.owned == nil {
		f.owned = map[cosmetics.Kind]map[string]bool{}
	}
	if f.owned[kind] == nil {
		f.owned[kind] = map[string]bool{}
	}
	f.owned[kind][itemID] = true
	return true, nil
}

func ownedCall(t *testing.T, own avatarOwnership, progress achievementProgressReader, kind string) []string {
	t.Helper()
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }
	RegisterAvatarCosmeticsOwnership(app, auth, own, progress)
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/players/me/cosmetics/"+kind+"/owned", nil), fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Data []string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Data
}

func handsPlayed(count int) []achievements.PlayerAchievementProgress {
	return []achievements.PlayerAchievementProgress{{Key: achievements.KeyHandsPlayed, Count: count}}
}

func TestOwnedReadSelfHealsEarnedItemsOnce(t *testing.T) {
	own := &fakeOwnership{}
	progress := &fakeProgress{rows: map[string][]achievements.PlayerAchievementProgress{
		roomstore.CurrencyModeSandbox: handsPlayed(1000), // stars 2: dealer-visor (hat, 1) + veteran (badge, 2)
	}}

	ids := ownedCall(t, own, progress, "hat")
	want := []string{
		"hat/dealer-visor achievement:hands_played:2:backfill", // earned stars, like the pipeline
		"badge/veteran achievement:hands_played:2:backfill",
	}
	slices.Sort(want)
	got := slices.Sorted(slices.Values(own.reasons))
	if !slices.Equal(got, want) {
		t.Fatalf("grants = %v, want %v", got, want)
	}
	if !slices.Equal(ids, []string{"dealer-visor"}) {
		t.Fatalf("owned list = %v, want the freshly granted dealer-visor in the same response", ids)
	}

	own.reasons = nil
	ownedCall(t, own, progress, "hat")
	if len(own.reasons) != 0 {
		t.Fatalf("second call granted %v, want zero Grant calls", own.reasons)
	}
}

func TestOwnedReadNoProgressReadsEachModeOnceGrantsNothing(t *testing.T) {
	own := &fakeOwnership{}
	progress := &fakeProgress{}
	ownedCall(t, own, progress, "frame")
	if len(own.reasons) != 0 {
		t.Fatalf("grants = %v, want none", own.reasons)
	}
	if progress.reads[roomstore.CurrencyModeSandbox] != 1 || progress.reads[roomstore.CurrencyModeReal] != 1 || len(progress.reads) != 2 {
		t.Fatalf("progress reads = %v, want exactly one per mode", progress.reads)
	}
}

func TestOwnedReadTakesMaxStarsAcrossModes(t *testing.T) {
	own := &fakeOwnership{}
	progress := &fakeProgress{rows: map[string][]achievements.PlayerAchievementProgress{
		roomstore.CurrencyModeSandbox: handsPlayed(10),
		roomstore.CurrencyModeReal:    handsPlayed(150),
	}}
	ownedCall(t, own, progress, "hat")
	if !slices.Equal(own.reasons, []string{"hat/dealer-visor achievement:hands_played:1:backfill"}) {
		t.Fatalf("grants = %v, want only dealer-visor (qualifies in real mode)", own.reasons)
	}
}

func TestOwnedReadTierSkipGrantsAllLowerRows(t *testing.T) {
	own := &fakeOwnership{}
	progress := &fakeProgress{rows: map[string][]achievements.PlayerAchievementProgress{
		roomstore.CurrencyModeReal: handsPlayed(20000), // stars 4
	}}
	ownedCall(t, own, progress, "frame")
	for _, id := range []string{"dealer-visor", "veteran", "chip-ring"} {
		if !own.owned[cosmetics.KindHat][id] && !own.owned[cosmetics.KindBadge][id] && !own.owned[cosmetics.KindFrame][id] {
			t.Errorf("%s not granted; grants = %v", id, own.reasons)
		}
	}
	if len(own.reasons) != 3 {
		t.Fatalf("grants = %v, want exactly 3", own.reasons)
	}
	for _, r := range own.reasons {
		if !strings.HasSuffix(r, ":4:backfill") {
			t.Errorf("reason %q, want the earned stars and :backfill", r)
		}
	}
}

// folded_streak is a reset counter: the current count can drop below a tier
// already earned, but the unlock stamp proves the first tier was crossed.
func TestOwnedReadResetCounterWithUnlockStampGrantsFirstTier(t *testing.T) {
	own := &fakeOwnership{}
	progress := &fakeProgress{rows: map[string][]achievements.PlayerAchievementProgress{
		roomstore.CurrencyModeSandbox: {{Key: achievements.KeyFoldedStreak, Count: 3, UnlockedAt: "2026-11-01T00:00:00Z"}},
	}}
	ownedCall(t, own, progress, "badge")
	if !slices.Equal(own.reasons, []string{"badge/iron-nerve achievement:folded_streak:1:backfill"}) {
		t.Fatalf("grants = %v, want iron-nerve", own.reasons)
	}
}

func TestOwnedReadProgressErrorInOneModeStillCountsTheOther(t *testing.T) {
	own := &fakeOwnership{}
	progress := &fakeProgress{
		rows: map[string][]achievements.PlayerAchievementProgress{roomstore.CurrencyModeReal: handsPlayed(100)},
		errs: map[string]error{roomstore.CurrencyModeSandbox: errors.New("dynamo down")},
	}
	ids := ownedCall(t, own, progress, "hat")
	if !slices.Equal(ids, []string{"dealer-visor"}) {
		t.Fatalf("owned list = %v, want dealer-visor from the real-mode progress", ids)
	}
}

func TestOwnedReadGrantErrorDoesNotFailRoute(t *testing.T) {
	own := &fakeOwnership{
		grantErr: errors.New("conditional write exploded"),
		owned:    map[cosmetics.Kind]map[string]bool{cosmetics.KindHat: {"top-hat": true}},
	}
	progress := &fakeProgress{rows: map[string][]achievements.PlayerAchievementProgress{
		roomstore.CurrencyModeSandbox: handsPlayed(1000),
	}}
	ids := ownedCall(t, own, progress, "hat")
	if len(own.reasons) != 2 {
		t.Fatalf("grants = %v, want both attempted despite the first failing", own.reasons)
	}
	if !slices.Equal(ids, []string{"top-hat"}) {
		t.Fatalf("owned list = %v, want the existing list", ids)
	}
}

func TestOwnedReadSelfHealIsBoundedByTimeout(t *testing.T) {
	own := &fakeOwnership{owned: map[cosmetics.Kind]map[string]bool{cosmetics.KindHat: {"beanie": true}}}
	start := time.Now()
	ids := ownedCall(t, own, &fakeProgress{block: true}, "hat")
	if elapsed := time.Since(start); elapsed > earnedGrantTimeout+2*time.Second {
		t.Fatalf("route took %v, want bounded by %v", elapsed, earnedGrantTimeout)
	}
	if !slices.Equal(ids, []string{"beanie"}) {
		t.Fatalf("owned list = %v", ids)
	}
}
