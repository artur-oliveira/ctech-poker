package player

import (
	"context"
	"errors"
	"testing"
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
func (s *memoryStore) SetShowcase(_ context.Context, _ string, public, playstylePublic bool, featured []string) error {
	s.profile.ShowcasePublic = public
	s.profile.PlaystylePublic = playstylePublic
	s.profile.FeaturedAchievements = featured
	return nil
}
func (s *memoryStore) SetFavoriteReactions(_ context.Context, _ string, favorites []string) error {
	s.profile.FavoriteReactions = favorites
	return nil
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

func TestSetShowcaseValidatesSelection(t *testing.T) {
	store := &memoryStore{profile: PlayerProfile{UserID: "u1"}}
	svc := NewService(store)
	profile, err := svc.SetShowcase(context.Background(), "u1", true, true, []string{"wins", "hands_played"})
	if err != nil {
		t.Fatal(err)
	}
	if !profile.ShowcasePublic || len(profile.FeaturedAchievements) != 2 {
		t.Fatalf("unexpected showcase: %+v", profile)
	}
	if _, err := svc.SetShowcase(context.Background(), "u1", true, true, []string{"not-real"}); !errors.Is(err, ErrInvalidShowcase) {
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
