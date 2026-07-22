package player

import (
	"context"
	"errors"
	"strings"

	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

var ErrTermsNotAccepted = errors.New("poker terms not accepted")
var ErrEmptyName = errors.New("player: name is empty")
var ErrInvalidWalletMode = errors.New("player: wallet_mode must be sandbox or real")

// maxDisplayNameLen bounds a player's display name — it is broadcast as-is to
// every other seat at a table, so it gets the same length ceiling as chat.
const maxDisplayNameLen = 60

type profileStore interface {
	GetOrCreate(context.Context, string) (*PlayerProfile, error)
	AcceptTerms(context.Context, string) error
	SetName(context.Context, string, string) error
	SetWalletMode(context.Context, string, string) error
}

// balanceFetcher is the subset of *walletclient.Client the profile endpoint
// needs to show a balance alongside the profile — narrowed so tests can fake
// it without a live wallet.
type balanceFetcher interface {
	Balances(ctx context.Context, userID string) (*walletclient.Balances, error)
}

type Service struct {
	store  profileStore
	wallet balanceFetcher
}

func NewService(store profileStore) *Service { return &Service{store: store} }

// WithWallet wires in balance lookups; without it, Balances returns zeros
// instead of erroring (a profile that predates wallet wiring, or a test).
func (s *Service) WithWallet(wallet balanceFetcher) *Service {
	s.wallet = wallet
	return s
}

// Balances reports userID's game+sandbox balances via ctech-wallet.
func (s *Service) Balances(ctx context.Context, userID string) (*walletclient.Balances, error) {
	if s.wallet == nil {
		return &walletclient.Balances{}, nil
	}
	return s.wallet.Balances(ctx, userID)
}

// SetWalletMode persists the player's sandbox/real display preference.
func (s *Service) SetWalletMode(ctx context.Context, userID, mode string) (*PlayerProfile, error) {
	if mode != WalletModeSandbox && mode != WalletModeReal {
		return nil, ErrInvalidWalletMode
	}
	if err := s.store.SetWalletMode(ctx, userID, mode); err != nil {
		return nil, err
	}
	return s.store.GetOrCreate(ctx, userID)
}

func (s *Service) GetOrCreate(ctx context.Context, userID string) (*PlayerProfile, error) {
	return s.store.GetOrCreate(ctx, userID)
}

func (s *Service) AcceptTerms(ctx context.Context, userID string) (*PlayerProfile, error) {
	if err := s.store.AcceptTerms(ctx, userID); err != nil {
		return nil, err
	}
	return s.store.GetOrCreate(ctx, userID)
}

// SetName sanitizes and persists a display name, always overwriting whatever
// was there before — used both for an explicit profile edit and for the
// frontend's one-time save of the id_token's name on first login.
func (s *Service) SetName(ctx context.Context, userID, name string) (*PlayerProfile, error) {
	name = sanitizeDisplayName(name)
	if name == "" {
		return nil, ErrEmptyName
	}
	if err := s.store.SetName(ctx, userID, name); err != nil {
		return nil, err
	}
	return s.store.GetOrCreate(ctx, userID)
}

func sanitizeDisplayName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return ""
	}
	runes := []rune(name)
	if len(runes) > maxDisplayNameLen {
		runes = runes[:maxDisplayNameLen]
	}
	return string(runes)
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
