package player

import (
	"context"
	"errors"
)

var ErrTermsNotAccepted = errors.New("poker terms not accepted")

type profileStore interface {
	GetOrCreate(context.Context, string) (*PlayerProfile, error)
	AcceptTerms(context.Context, string) error
}

type Service struct{ store profileStore }

func NewService(store profileStore) *Service { return &Service{store: store} }

func (s *Service) GetOrCreate(ctx context.Context, userID string) (*PlayerProfile, error) {
	return s.store.GetOrCreate(ctx, userID)
}

func (s *Service) AcceptTerms(ctx context.Context, userID string) (*PlayerProfile, error) {
	if err := s.store.AcceptTerms(ctx, userID); err != nil {
		return nil, err
	}
	return s.store.GetOrCreate(ctx, userID)
}

func (s *Service) RequireAccepted(ctx context.Context, userID string) error {
	profile, err := s.store.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}
	if !profile.TermsAccepted() {
		return ErrTermsNotAccepted
	}
	return nil
}
