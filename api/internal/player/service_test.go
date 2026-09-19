package player

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/cosmetics"
)

type memoryStore struct{ profile PlayerProfile }

func (s *memoryStore) GetOrCreate(context.Context, string) (*PlayerProfile, error) {
	return &s.profile, nil
}
func (s *memoryStore) Get(context.Context, string) (*PlayerProfile, error) {
	return &s.profile, nil
}
func (s *memoryStore) AcceptTerms(context.Context, string) error {
	s.profile.PokerTermsVersion = CurrentPokerTermsVersion
	s.profile.TermsAcceptedAt = "now"
	return nil
}
func (s *memoryStore) SetName(_ context.Context, _ string, name string) error {
	s.profile.Name = name
	return nil
}
func (s *memoryStore) SetWalletMode(_ context.Context, _ string, mode string) error {
	s.profile.WalletMode = mode
	return nil
}
func (s *memoryStore) SetDeckVariant(_ context.Context, _ string, variant string) error {
	s.profile.DeckVariant = variant
	return nil
}
func (s *memoryStore) SetTableTheme(_ context.Context, _ string, theme string) error {
	s.profile.TableTheme = theme
	return nil
}
func (s *memoryStore) SetBetPresetMode(_ context.Context, _ string, mode string) error {
	s.profile.BetPresetMode = mode
	return nil
}
func (s *memoryStore) SetShowcase(_ context.Context, _ string, public, playstylePublic, tablePublic bool, featured []string) error {
	s.profile.ShowcasePublic = public
	s.profile.PlaystylePublic = playstylePublic
	s.profile.TablePublic = tablePublic
	s.profile.FeaturedAchievements = featured
	return nil
}
func (s *memoryStore) SetShowcaseLayout(_ context.Context, _ string, layout ShowcaseLayout) error {
	s.profile.ShowcaseLayout = layout
	return nil
}
func (s *memoryStore) SetFavoriteReactions(_ context.Context, _ string, favorites []string) error {
	s.profile.FavoriteReactions = favorites
	return nil
}
func (s *memoryStore) SetReactionWheel(_ context.Context, _ string, reactionIDs []string) error {
	s.profile.ReactionWheel = reactionIDs
	return nil
}
func (s *memoryStore) SetStatsGoals(_ context.Context, _ string, goals map[string]float64) error {
	s.profile.StatsGoals = goals
	return nil
}
func (s *memoryStore) SetEquippedFrame(_ context.Context, _ string, frameID string) error {
	s.profile.EquippedFrameID = frameID
	return nil
}
func (s *memoryStore) SetEquippedBadges(_ context.Context, _ string, badgeIDs []string) error {
	s.profile.EquippedBadgeIDs = badgeIDs
	return nil
}

type fakeCosmeticsChecker struct{ owned bool }

func (f *fakeCosmeticsChecker) IsOwned(context.Context, string, cosmetics.Kind, string) (bool, error) {
	return f.owned, nil
}

type fakeReactionChecker struct{ owned bool }

func (f *fakeReactionChecker) IsOwned(context.Context, string, string) (bool, error) {
	return f.owned, nil
}

func TestRequireAccepted(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "u1"}}
	svc := NewService(store)
	if err := svc.RequireAccepted(context.Background(), "u1"); !errors.Is(err, ErrTermsNotAccepted) {
		t.Fatalf("got %v", err)
	}
	if _, err := svc.AcceptTerms(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RequireAccepted(context.Background(), "u1"); err != nil {
		t.Fatalf("accepted profile rejected: %v", err)
	}
}

func TestSetName(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "u1"}}
	svc := NewService(store)

	profile, err := svc.SetName(context.Background(), "u1", "  Artur  ")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != "Artur" {
		t.Fatalf("Name = %q, want trimmed %q", profile.Name, "Artur")
	}

	long := ""
	for i := 0; i < maxDisplayNameLen+10; i++ {
		long += "a"
	}
	profile, err = svc.SetName(context.Background(), "u1", long)
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.Name) != maxDisplayNameLen {
		t.Fatalf("Name len = %d, want capped at %d", len(profile.Name), maxDisplayNameLen)
	}

	if _, err := svc.SetName(context.Background(), "u1", "   "); !errors.Is(err, ErrEmptyName) {
		t.Fatalf("got %v, want ErrEmptyName", err)
	}
}

func TestSetWalletMode(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "u1"}}
	svc := NewService(store)

	profile, err := svc.SetWalletMode(context.Background(), "u1", WalletModeReal)
	if err != nil {
		t.Fatal(err)
	}
	if profile.WalletMode != WalletModeReal {
		t.Fatalf("WalletMode = %q, want %q", profile.WalletMode, WalletModeReal)
	}

	if _, err := svc.SetWalletMode(context.Background(), "u1", "bogus"); !errors.Is(err, ErrInvalidWalletMode) {
		t.Fatalf("got %v, want ErrInvalidWalletMode", err)
	}
}

func TestSetDeckVariant(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "u1"}}
	svc := NewService(store)

	profile, err := svc.SetDeckVariant(context.Background(), "u1", "colorblind")
	if err != nil {
		t.Fatal(err)
	}
	if profile.DeckVariant != "colorblind" {
		t.Fatalf("DeckVariant = %q, want %q", profile.DeckVariant, "colorblind")
	}

	if _, err := svc.SetDeckVariant(context.Background(), "u1", "   "); !errors.Is(err, ErrInvalidDeckVariant) {
		t.Fatalf("got %v, want ErrInvalidDeckVariant", err)
	}
}

func TestSetDeckVariantRejectsUnknownID(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "u1"}}
	svc := NewService(store)
	if _, err := svc.SetDeckVariant(context.Background(), "u1", "not-a-real-deck"); !errors.Is(err, ErrInvalidDeckVariant) {
		t.Fatalf("got %v, want ErrInvalidDeckVariant", err)
	}
}

func TestSetDeckVariantPremiumRequiresOwnership(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "u1"}}
	svc := NewService(store)

	// No cosmetics dependency wired at all: fails closed.
	if _, err := svc.SetDeckVariant(context.Background(), "u1", "casino"); !errors.Is(err, ErrCosmeticNotOwned) {
		t.Fatalf("got %v, want ErrCosmeticNotOwned when cosmetics is unwired", err)
	}

	svc.WithCosmetics(&fakeCosmeticsChecker{owned: false})
	if _, err := svc.SetDeckVariant(context.Background(), "u1", "casino"); !errors.Is(err, ErrCosmeticNotOwned) {
		t.Fatalf("got %v, want ErrCosmeticNotOwned without an entitlement", err)
	}

	svc.WithCosmetics(&fakeCosmeticsChecker{owned: true})
	profile, err := svc.SetDeckVariant(context.Background(), "u1", "casino")
	if err != nil {
		t.Fatalf("SetDeckVariant with an entitlement: %v", err)
	}
	if profile.DeckVariant != "casino" {
		t.Fatalf("DeckVariant = %q, want %q", profile.DeckVariant, "casino")
	}
}

func TestSetTableTheme(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "u1"}}
	svc := NewService(store)

	profile, err := svc.SetTableTheme(context.Background(), "u1", "classic")
	if err != nil {
		t.Fatal(err)
	}
	if profile.TableTheme != "classic" {
		t.Fatalf("TableTheme = %q, want %q", profile.TableTheme, "classic")
	}

	if _, err := svc.SetTableTheme(context.Background(), "u1", "   "); !errors.Is(err, ErrInvalidTableTheme) {
		t.Fatalf("got %v, want ErrInvalidTableTheme", err)
	}
	if _, err := svc.SetTableTheme(context.Background(), "u1", "not-a-real-theme"); !errors.Is(err, ErrInvalidTableTheme) {
		t.Fatalf("got %v, want ErrInvalidTableTheme", err)
	}
}

func TestSetTableThemePremiumRequiresOwnership(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "u1"}}
	svc := NewService(store)

	if _, err := svc.SetTableTheme(context.Background(), "u1", "midnight"); !errors.Is(err, ErrCosmeticNotOwned) {
		t.Fatalf("got %v, want ErrCosmeticNotOwned when cosmetics is unwired", err)
	}

	svc.WithCosmetics(&fakeCosmeticsChecker{owned: false})
	if _, err := svc.SetTableTheme(context.Background(), "u1", "midnight"); !errors.Is(err, ErrCosmeticNotOwned) {
		t.Fatalf("got %v, want ErrCosmeticNotOwned without an entitlement", err)
	}

	svc.WithCosmetics(&fakeCosmeticsChecker{owned: true})
	profile, err := svc.SetTableTheme(context.Background(), "u1", "midnight")
	if err != nil {
		t.Fatalf("SetTableTheme with an entitlement: %v", err)
	}
	if profile.TableTheme != "midnight" {
		t.Fatalf("TableTheme = %q, want %q", profile.TableTheme, "midnight")
	}
}

func TestBalancesDefaultsToZeroWithoutWallet(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "u1"}}
	svc := NewService(store)

	balances, err := svc.Balances(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if balances.GameBalance != 0 || balances.SandboxBalance != 0 {
		t.Fatalf("got %+v, want zero balances", balances)
	}
}

func TestSetShowcaseStoresTablePublic(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "u1"}}
	svc := NewService(store)
	profile, err := svc.SetShowcase(context.Background(), "u1", true, false, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !profile.TablePublic {
		t.Fatal("expected table_public to persist")
	}
}

func TestSetShowcaseValidatesSelection(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "u1"}}
	svc := NewService(store)
	profile, err := svc.SetShowcase(context.Background(), "u1", true, true, false, []string{"wins", "hands_played"})
	if err != nil {
		t.Fatal(err)
	}
	if !profile.ShowcasePublic || len(profile.FeaturedAchievements) != 2 {
		t.Fatalf("unexpected showcase: %+v", profile)
	}
	if _, err := svc.SetShowcase(context.Background(), "u1", true, true, false, []string{"not-real"}); !errors.Is(err, ErrInvalidShowcase) {
		t.Fatalf("got %v, want ErrInvalidShowcase", err)
	}
}

func TestSetShowcaseLayoutPersistsOnlyACompleteSafeLayout(t *testing.T) {
	store := &memoryStore{}
	svc := NewService(store)
	layout := ShowcaseLayout{Order: []string{ShowcaseSectionBestHand, ShowcaseSectionAchievements, ShowcaseSectionMatchup}, Hidden: []string{ShowcaseSectionMatchup}}
	profile, err := svc.SetShowcaseLayout(context.Background(), "u1", layout)
	if err != nil || profile.ShowcaseLayout.Order[0] != ShowcaseSectionBestHand {
		t.Fatalf("got profile=%+v err=%v", profile, err)
	}
	_, err = svc.SetShowcaseLayout(context.Background(), "u1", ShowcaseLayout{Order: []string{ShowcaseSectionAchievements, ShowcaseSectionBestHand, ShowcaseSectionBestHand}})
	if !errors.Is(err, ErrInvalidShowcaseLayout) {
		t.Fatalf("got %v, want ErrInvalidShowcaseLayout", err)
	}
	_, err = svc.SetShowcaseLayout(context.Background(), "u1", ShowcaseLayout{Order: []string{ShowcaseSectionAchievements, ShowcaseSectionBestHand, ShowcaseSectionMatchup}, Hidden: []string{ShowcaseSectionAchievements}})
	if !errors.Is(err, ErrInvalidShowcaseLayout) {
		t.Fatalf("got %v, want ErrInvalidShowcaseLayout", err)
	}
}

func TestSetFavoriteReactionsValidatesCountAndCatalog(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "user-1"}}
	svc := NewService(store)

	if _, err := svc.SetFavoriteReactions(context.Background(), "user-1", []string{"clap", "cold", "fire", "poop"}); !errors.Is(err, ErrInvalidFavoriteReactions) {
		t.Fatalf("expected rejection of a 4th favorite, got %v", err)
	}
	if _, err := svc.SetFavoriteReactions(context.Background(), "user-1", []string{"not-a-reaction"}); !errors.Is(err, ErrInvalidFavoriteReactions) {
		t.Fatalf("expected rejection of an unknown reaction id, got %v", err)
	}

	profile, err := svc.SetFavoriteReactions(context.Background(), "user-1", []string{"clap", "cold"})
	if err != nil {
		t.Fatalf("SetFavoriteReactions: %v", err)
	}
	if len(profile.FavoriteReactions) != 2 {
		t.Fatalf("unexpected favorites: %+v", profile.FavoriteReactions)
	}
}

func TestSetFavoriteReactionsAllowsUnownedPremium(t *testing.T) {
	// Favoriting a premium reaction the player doesn't own yet is allowed —
	// it's a UI shortcut to the buy flow, not a claim of ownership
	// (docs/specs/2026-08-12-premium-reactions.md). handleReaction's
	// ownership check is what actually gates use.
	store := &memoryStore{profile: PlayerProfile{UserID: "user-1"}}
	svc := NewService(store)
	if _, err := svc.SetFavoriteReactions(context.Background(), "user-1", []string{"cold"}); err != nil {
		t.Fatalf("expected favoriting an unowned premium reaction to succeed, got %v", err)
	}
}

func TestSetReactionWheelValidatesCatalogAndOwnership(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "user-1"}}
	svc := NewService(store).WithReactions(&fakeReactionChecker{owned: false})

	if _, err := svc.SetReactionWheel(context.Background(), "user-1", []string{"not-a-reaction"}); !errors.Is(err, ErrInvalidReactionWheel) {
		t.Fatalf("expected rejection of an unknown reaction id, got %v", err)
	}
	if _, err := svc.SetReactionWheel(context.Background(), "user-1", []string{"clap", "clap"}); !errors.Is(err, ErrInvalidReactionWheel) {
		t.Fatalf("expected rejection of a duplicate reaction id, got %v", err)
	}
	// "cold" is a known premium catalog reaction (see internal/reactions);
	// rejected here because fakeReactionChecker reports it unowned.
	if _, err := svc.SetReactionWheel(context.Background(), "user-1", []string{"clap", "cold"}); !errors.Is(err, ErrReactionNotOwned) {
		t.Fatalf("expected rejection of an unowned premium reaction, got %v", err)
	}

	profile, err := svc.SetReactionWheel(context.Background(), "user-1", []string{"clap", "laugh"})
	if err != nil {
		t.Fatalf("SetReactionWheel: %v", err)
	}
	if len(profile.ReactionWheel) != 2 || profile.ReactionWheel[0] != "clap" || profile.ReactionWheel[1] != "laugh" {
		t.Fatalf("unexpected wheel: %+v", profile.ReactionWheel)
	}
}

func TestSetReactionWheelAllowsOwnedPremium(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "user-1"}}
	svc := NewService(store).WithReactions(&fakeReactionChecker{owned: true})
	if _, err := svc.SetReactionWheel(context.Background(), "user-1", []string{"cold"}); err != nil {
		t.Fatalf("expected an owned premium reaction to be accepted, got %v", err)
	}
}

// batchStore records the size of every GetMany batch the service issues, so
// the chunking below is asserted on the calls actually made rather than on
// the merged result alone.
type batchStore struct {
	memoryStore
	batches []int
}

func (s *batchStore) GetMany(_ context.Context, userIDs []string) (map[string]PlayerProfile, error) {
	s.batches = append(s.batches, len(userIDs))
	if len(userIDs) > MaxBatchProfileIDs {
		return nil, errors.New("player: batch profile limit exceeded")
	}
	out := make(map[string]PlayerProfile, len(userIDs))
	for _, id := range userIDs {
		out[id] = PlayerProfile{UserID: id, Name: "name-" + id}
	}
	return out, nil
}

// TestGetManyChunksAtBatchLimit pins the one shared resolution point issue #64
// routes every stale-name consumer through: a caller handing it more ids than
// BatchGetItem accepts must get all of them resolved, not an error, and no
// single store call may exceed the limit.
func TestGetManyChunksAtBatchLimit(t *testing.T) {
	ids := make([]string, MaxBatchProfileIDs*2+7)
	for i := range ids {
		ids[i] = "p" + strconv.Itoa(i)
	}
	store := &batchStore{}
	profiles, err := NewService(store).GetMany(context.Background(), ids)
	if err != nil {
		t.Fatalf("GetMany over %d ids: %v", len(ids), err)
	}
	if len(profiles) != len(ids) {
		t.Fatalf("resolved %d of %d ids", len(profiles), len(ids))
	}
	if len(store.batches) != 3 {
		t.Fatalf("expected 3 batches for %d ids, got %v", len(ids), store.batches)
	}
	for i, size := range store.batches {
		if size > MaxBatchProfileIDs {
			t.Fatalf("batch %d carried %d ids, over the %d limit", i, size, MaxBatchProfileIDs)
		}
	}
	// A set that already fits still goes out as exactly one call.
	small := &batchStore{}
	if _, err := NewService(small).GetMany(context.Background(), ids[:5]); err != nil {
		t.Fatal(err)
	}
	if len(small.batches) != 1 || small.batches[0] != 5 {
		t.Fatalf("a fitting set should be one call of 5, got %v", small.batches)
	}
}

func TestSetBetPresetMode(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "u1"}}
	svc := NewService(store)

	// Unset reads back as mixed — the server normalizes so the client has no
	// fallback logic of its own.
	if got := store.profile.EffectiveBetPresetMode(); got != BetPresetModeMixed {
		t.Fatalf("EffectiveBetPresetMode on an unset profile = %q, want %q", got, BetPresetModeMixed)
	}

	for _, mode := range []string{BetPresetModeMixed, BetPresetModeBB, BetPresetModePot} {
		profile, err := svc.SetBetPresetMode(context.Background(), "u1", mode)
		if err != nil {
			t.Fatalf("SetBetPresetMode(%q): %v", mode, err)
		}
		if profile.BetPresetMode != mode || profile.EffectiveBetPresetMode() != mode {
			t.Fatalf("BetPresetMode = %q, want %q", profile.BetPresetMode, mode)
		}
	}

	for _, bad := range []string{"", "   ", "MIXED", "big-blind", "potato"} {
		if _, err := svc.SetBetPresetMode(context.Background(), "u1", bad); !errors.Is(err, ErrInvalidBetPresetMode) {
			t.Fatalf("SetBetPresetMode(%q) = %v, want ErrInvalidBetPresetMode", bad, err)
		}
	}
	// A rejected write leaves the previous value alone.
	if store.profile.BetPresetMode != BetPresetModePot {
		t.Fatalf("BetPresetMode = %q after rejections, want %q", store.profile.BetPresetMode, BetPresetModePot)
	}

	// A value written by an older build that is no longer valid still reads
	// back as the default rather than leaking through.
	store.profile.BetPresetMode = "gone"
	if got := store.profile.EffectiveBetPresetMode(); got != BetPresetModeMixed {
		t.Fatalf("EffectiveBetPresetMode on an unknown value = %q, want %q", got, BetPresetModeMixed)
	}
}

// TestSetEquippedFrameValidatesCatalogAndOwnership is #292's core acceptance:
// a premium frame the player doesn't own can never be equipped.
func TestSetEquippedFrameValidatesCatalogAndOwnership(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "user-1"}}
	svc := NewService(store).WithCosmetics(&fakeCosmeticsChecker{owned: false})

	if _, err := svc.SetEquippedFrame(context.Background(), "user-1", "not-a-real-frame"); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("expected ErrInvalidFrame, got %v", err)
	}
	if _, err := svc.SetEquippedFrame(context.Background(), "user-1", "season-2026-q3"); !errors.Is(err, ErrCosmeticNotOwned) {
		t.Fatalf("expected ErrCosmeticNotOwned, got %v", err)
	}

	svc = NewService(store).WithCosmetics(&fakeCosmeticsChecker{owned: true})
	profile, err := svc.SetEquippedFrame(context.Background(), "user-1", "season-2026-q3")
	if err != nil {
		t.Fatalf("SetEquippedFrame: %v", err)
	}
	if profile.EquippedFrameID != "season-2026-q3" {
		t.Fatalf("unexpected equipped frame: %+v", profile)
	}

	// Unequipping (empty id) is always allowed, no ownership check needed.
	svc = NewService(store).WithCosmetics(&fakeCosmeticsChecker{owned: false})
	profile, err = svc.SetEquippedFrame(context.Background(), "user-1", "")
	if err != nil || profile.EquippedFrameID != "" {
		t.Fatalf("expected unequip to succeed, got profile=%+v err=%v", profile, err)
	}
}

// TestSetEquippedBadgesValidatesCountUniquenessAndOwnership is #292's other
// acceptance criterion: bounded count, no duplicates, each id owned.
func TestSetEquippedBadgesValidatesCountUniquenessAndOwnership(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "user-1"}}
	svc := NewService(store).WithCosmetics(&fakeCosmeticsChecker{owned: true})

	tooMany := make([]string, maxEquippedBadges+1)
	for i := range tooMany {
		tooMany[i] = "season-2026-q3-top10"
	}
	if _, err := svc.SetEquippedBadges(context.Background(), "user-1", tooMany); !errors.Is(err, ErrInvalidBadges) {
		t.Fatalf("expected ErrInvalidBadges for too many badges, got %v", err)
	}
	if _, err := svc.SetEquippedBadges(context.Background(), "user-1", []string{"season-2026-q3-top10", "season-2026-q3-top10"}); !errors.Is(err, ErrInvalidBadges) {
		t.Fatalf("expected ErrInvalidBadges for a duplicate id, got %v", err)
	}
	if _, err := svc.SetEquippedBadges(context.Background(), "user-1", []string{"not-a-real-badge"}); !errors.Is(err, ErrInvalidBadges) {
		t.Fatalf("expected ErrInvalidBadges for an unknown id, got %v", err)
	}

	unowned := NewService(store).WithCosmetics(&fakeCosmeticsChecker{owned: false})
	if _, err := unowned.SetEquippedBadges(context.Background(), "user-1", []string{"season-2026-q3-top10"}); !errors.Is(err, ErrCosmeticNotOwned) {
		t.Fatalf("expected ErrCosmeticNotOwned, got %v", err)
	}

	profile, err := svc.SetEquippedBadges(context.Background(), "user-1", []string{"season-2026-q3-top10", "season-2026-q3-champion"})
	if err != nil {
		t.Fatalf("SetEquippedBadges: %v", err)
	}
	if len(profile.EquippedBadgeIDs) != 2 {
		t.Fatalf("unexpected equipped badges: %+v", profile.EquippedBadgeIDs)
	}
}
