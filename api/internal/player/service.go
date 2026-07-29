package player

import (
	"context"
	"errors"
	"strings"

	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

var ErrTermsNotAccepted = errors.New("poker terms not accepted")
var ErrEmptyName = errors.New("player: name is empty")
var ErrInvalidWalletMode = errors.New("player: wallet_mode must be sandbox or real")
var ErrInvalidDeckVariant = errors.New("player: deck_variant must not be empty")
var ErrInvalidShowcase = errors.New("player: invalid showcase")
var ErrShowcasePrivate = errors.New("player: showcase is private")

// maxDisplayNameLen bounds a player's display name — it is broadcast as-is to
// every other seat at a table, so it gets the same length ceiling as chat.
const maxDisplayNameLen = 60

// maxDeckVariantLen is a generous cap, not a catalog check — the variant
// catalog is cosmetic-only and lives on the frontend (src/lib/cardVariants.ts),
// so the backend just stores whatever id it's given.
const maxDeckVariantLen = 60

type profileStore interface {
	GetOrCreate(context.Context, string) (*PlayerProfile, error)
	Get(context.Context, string) (*PlayerProfile, error)
	AcceptTerms(context.Context, string) error
	SetName(context.Context, string, string) error
	SetWalletMode(context.Context, string, string) error
	SetDeckVariant(context.Context, string, string) error
	SetShowcase(context.Context, string, bool, []string) error
}

func (s *Service) ReportAvatar(ctx context.Context, targetID, reporterID string) error {
	store, ok := s.store.(interface {
		ReportAvatar(context.Context, string, string) error
	})
	if !ok {
		return errors.New("player: avatar reports unavailable")
	}
	return store.ReportAvatar(ctx, targetID, reporterID)
}

func (s *Service) SetAvatar(ctx context.Context, userID, key string, version int) (*PlayerProfile, error) {
	store, ok := s.store.(interface {
		SetAvatar(context.Context, string, string, int) error
	})
	if !ok {
		return nil, errors.New("player: avatar updates unavailable")
	}
	if err := store.SetAvatar(ctx, userID, key, version); err != nil {
		return nil, err
	}
	return s.store.GetOrCreate(ctx, userID)
}

func (s *Service) ClearAvatar(ctx context.Context, userID string) (*PlayerProfile, error) {
	store, ok := s.store.(interface {
		ClearAvatar(context.Context, string) error
	})
	if !ok {
		return nil, errors.New("player: avatar updates unavailable")
	}
	if err := store.ClearAvatar(ctx, userID); err != nil {
		return nil, err
	}
	return s.store.GetOrCreate(ctx, userID)
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

// SetDeckVariant persists the player's card-color-scheme preference. The id
// itself is opaque to the backend — it just needs to round-trip back to the
// frontend, which owns the catalog and picks the default when unset.
func (s *Service) SetDeckVariant(ctx context.Context, userID, variant string) (*PlayerProfile, error) {
	variant = strings.TrimSpace(variant)
	if variant == "" {
		return nil, ErrInvalidDeckVariant
	}
	if len(variant) > maxDeckVariantLen {
		variant = variant[:maxDeckVariantLen]
	}
	if err := s.store.SetDeckVariant(ctx, userID, variant); err != nil {
		return nil, err
	}
	return s.store.GetOrCreate(ctx, userID)
}

func (s *Service) GetOrCreate(ctx context.Context, userID string) (*PlayerProfile, error) {
	return s.store.GetOrCreate(ctx, userID)
}

func (s *Service) PublicShowcase(ctx context.Context, userID string) (*PlayerProfile, error) {
	profile, err := s.store.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if profile == nil || !profile.ShowcasePublic {
		return nil, ErrShowcasePrivate
	}
	return profile, nil
}

func (s *Service) SetShowcase(ctx context.Context, userID string, public bool, featured []string) (*PlayerProfile, error) {
	if len(featured) > 3 {
		return nil, ErrInvalidShowcase
	}
	valid := make(map[string]bool, len(achievements.Catalog))
	for _, item := range achievements.Catalog {
		valid[item.Key] = true
	}
	seen := make(map[string]bool, len(featured))
	normalized := make([]string, 0, len(featured))
	for _, key := range featured {
		key = strings.TrimSpace(key)
		if key == "" || !valid[key] || seen[key] {
			return nil, ErrInvalidShowcase
		}
		seen[key] = true
		normalized = append(normalized, key)
	}
	if err := s.store.SetShowcase(ctx, userID, public, normalized); err != nil {
		return nil, err
	}
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
