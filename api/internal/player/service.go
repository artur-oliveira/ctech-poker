package player

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
	"gopkg.aoctech.app/poker/api/internal/reactions"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

var ErrTermsNotAccepted = errors.New("poker terms not accepted")
var ErrEmptyName = errors.New("player: name is empty")
var ErrInvalidWalletMode = errors.New("player: wallet_mode must be sandbox or real")
var ErrInvalidDeckVariant = errors.New("player: deck_variant must not be empty")
var ErrInvalidTableTheme = errors.New("player: table_theme must not be empty")
var ErrCosmeticNotOwned = errors.New("player: cosmetic is premium and not owned")
var ErrInvalidShowcase = errors.New("player: invalid showcase")
var ErrShowcasePrivate = errors.New("player: showcase is private")
var ErrInvalidFavoriteReactions = errors.New("player: invalid favorite reactions")

// maxDisplayNameLen bounds a player's display name — it is broadcast as-is to
// every other seat at a table, so it gets the same length ceiling as chat.
const maxDisplayNameLen = 60

type profileStore interface {
	GetOrCreate(context.Context, string) (*PlayerProfile, error)
	Get(context.Context, string) (*PlayerProfile, error)
	AcceptTerms(context.Context, string) error
	SetName(context.Context, string, string) error
	SetWalletMode(context.Context, string, string) error
	SetDeckVariant(context.Context, string, string) error
	SetTableTheme(context.Context, string, string) error
	SetShowcase(context.Context, string, bool, bool, bool, []string) error
	SetFavoriteReactions(context.Context, string, []string) error
}

// cosmeticsOwnershipChecker is the narrow slice of *cosmeticpurchase.Service
// SetDeckVariant/SetTableTheme need — satisfied by *cosmeticpurchase.Service
// without this package importing it directly (cosmeticpurchase itself
// depends on player's EffectiveDeckVariant/EffectiveTableTheme for its own
// refund check, so a direct import here would cycle).
type cosmeticsOwnershipChecker interface {
	IsOwned(ctx context.Context, playerID string, kind cosmetics.Kind, itemID string) (bool, error)
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
	store     profileStore
	wallet    balanceFetcher
	cosmetics cosmeticsOwnershipChecker
}

func NewService(store profileStore) *Service { return &Service{store: store} }

// WithWallet wires in balance lookups; without it, Balances returns zeros
// instead of erroring (a profile that predates wallet wiring, or a test).
func (s *Service) WithWallet(wallet balanceFetcher) *Service {
	s.wallet = wallet
	return s
}

// WithCosmetics wires in the premium deck/felt ownership check
// SetDeckVariant/SetTableTheme require before persisting a premium id.
func (s *Service) WithCosmetics(c cosmeticsOwnershipChecker) *Service {
	s.cosmetics = c
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

// SetDeckVariant persists the player's card-color-scheme preference. variant
// must be a known catalog id (docs/specs/2026-08-21-premium-cosmetics-overhaul.md
// Part 4 — this used to accept any string up to 60 chars with no catalog or
// ownership check at all); a premium id additionally requires an active
// entitlement.
func (s *Service) SetDeckVariant(ctx context.Context, userID, variant string) (*PlayerProfile, error) {
	variant = strings.TrimSpace(variant)
	if variant == "" {
		return nil, ErrInvalidDeckVariant
	}
	if !cosmetics.IsKnown(cosmetics.KindDeck, variant) {
		return nil, ErrInvalidDeckVariant
	}
	if cosmetics.IsPremium(cosmetics.KindDeck, variant) {
		if err := s.requireCosmetic(ctx, userID, cosmetics.KindDeck, variant); err != nil {
			return nil, err
		}
	}
	if err := s.store.SetDeckVariant(ctx, userID, variant); err != nil {
		return nil, err
	}
	return s.store.GetOrCreate(ctx, userID)
}

// SetTableTheme persists the player's felt preference, mirroring
// SetDeckVariant's validate-then-persist shape exactly — unlike deck_variant,
// which shipped without an ownership check and is being closed after the
// fact, table_theme never ships without one.
func (s *Service) SetTableTheme(ctx context.Context, userID, themeID string) (*PlayerProfile, error) {
	themeID = strings.TrimSpace(themeID)
	if themeID == "" {
		return nil, ErrInvalidTableTheme
	}
	if !cosmetics.IsKnown(cosmetics.KindFelt, themeID) {
		return nil, ErrInvalidTableTheme
	}
	if cosmetics.IsPremium(cosmetics.KindFelt, themeID) {
		if err := s.requireCosmetic(ctx, userID, cosmetics.KindFelt, themeID); err != nil {
			return nil, err
		}
	}
	if err := s.store.SetTableTheme(ctx, userID, themeID); err != nil {
		return nil, err
	}
	return s.store.GetOrCreate(ctx, userID)
}

// requireCosmetic fails closed: a premium id is rejected outright if
// ownership wiring is missing, since the ownership check must never silently
// pass just because s.cosmetics was never set (e.g. a wiring mistake).
func (s *Service) requireCosmetic(ctx context.Context, userID string, kind cosmetics.Kind, itemID string) error {
	if s.cosmetics == nil {
		return fmt.Errorf("%w: ownership check unavailable", ErrCosmeticNotOwned)
	}
	owned, err := s.cosmetics.IsOwned(ctx, userID, kind, itemID)
	if err != nil {
		return err
	}
	if !owned {
		return ErrCosmeticNotOwned
	}
	return nil
}

func (s *Service) GetOrCreate(ctx context.Context, userID string) (*PlayerProfile, error) {
	return s.store.GetOrCreate(ctx, userID)
}

func (s *Service) Get(ctx context.Context, userID string) (*PlayerProfile, error) {
	return s.store.Get(ctx, userID)
}

// GetMany resolves a set of player ids to their canonical profiles — the one
// place every consumer of a denormalized display name/avatar re-reads the
// truth at render time instead of trusting its own stale copy (issue #64).
// Chunks at MaxBatchProfileIDs so callers never have to: the store's
// BatchGetItem has a hard key ceiling, and a single leaderboard page or hand-
// history page can carry more distinct ids than one hand ever does.
func (s *Service) GetMany(ctx context.Context, userIDs []string) (map[string]PlayerProfile, error) {
	if store, ok := s.store.(interface {
		GetMany(context.Context, []string) (map[string]PlayerProfile, error)
	}); ok {
		if len(userIDs) <= MaxBatchProfileIDs {
			return store.GetMany(ctx, userIDs)
		}
		result := make(map[string]PlayerProfile, len(userIDs))
		for start := 0; start < len(userIDs); start += MaxBatchProfileIDs {
			end := min(start+MaxBatchProfileIDs, len(userIDs))
			batch, err := store.GetMany(ctx, userIDs[start:end])
			if err != nil {
				return nil, err
			}
			for id, profile := range batch {
				result[id] = profile
			}
		}
		return result, nil
	}
	result := make(map[string]PlayerProfile, len(userIDs))
	for _, userID := range userIDs {
		profile, err := s.store.Get(ctx, userID)
		if err != nil {
			return nil, err
		}
		if profile != nil {
			result[userID] = *profile
		}
	}
	return result, nil
}

func (s *Service) LookupByFriendCode(ctx context.Context, code string) (*PlayerProfile, error) {
	store, ok := s.store.(interface {
		LookupByFriendCode(context.Context, string) (*PlayerProfile, error)
	})
	if !ok {
		return nil, errors.New("player: friend-code lookup unavailable")
	}
	return store.LookupByFriendCode(ctx, code)
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

func (s *Service) SetShowcase(ctx context.Context, userID string, public, playstylePublic, tablePublic bool, featured []string) (*PlayerProfile, error) {
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
	if err := s.store.SetShowcase(ctx, userID, public, playstylePublic, tablePublic, normalized); err != nil {
		return nil, err
	}
	return s.store.GetOrCreate(ctx, userID)
}

// SetFavoriteReactions mirrors SetShowcase's validation shape exactly.
// Favoriting a premium reaction the player doesn't yet own is allowed — it's
// a UI shortcut to the buy flow, not a claim of ownership;
// Actor.handleReaction's ownership check is what actually gates use
// (docs/specs/2026-08-12-premium-reactions.md).
func (s *Service) SetFavoriteReactions(ctx context.Context, userID string, favorites []string) (*PlayerProfile, error) {
	if len(favorites) > 3 {
		return nil, ErrInvalidFavoriteReactions
	}
	seen := make(map[string]bool, len(favorites))
	normalized := make([]string, 0, len(favorites))
	for _, id := range favorites {
		id = strings.TrimSpace(id)
		if id == "" || !reactions.IsKnown(id) || seen[id] {
			return nil, ErrInvalidFavoriteReactions
		}
		seen[id] = true
		normalized = append(normalized, id)
	}
	if err := s.store.SetFavoriteReactions(ctx, userID, normalized); err != nil {
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
