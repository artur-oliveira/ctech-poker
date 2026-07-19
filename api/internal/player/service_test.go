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
