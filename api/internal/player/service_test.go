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
func (s *memoryStore) SetShowcase(_ context.Context, _ string, public, playstylePublic, tablePublic bool, featured []string) error {
	s.profile.ShowcasePublic = public
	s.profile.PlaystylePublic = playstylePublic
	s.profile.TablePublic = tablePublic
	s.profile.FeaturedAchievements = featured
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
