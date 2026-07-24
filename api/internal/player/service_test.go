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
