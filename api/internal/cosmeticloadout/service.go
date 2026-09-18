package cosmeticloadout

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
)

var (
	ErrInvalidName       = errors.New("cosmeticloadout: name must be 1-60 characters")
	ErrInvalidSelections = errors.New("cosmeticloadout: selections must name at least one known deck/felt item")
	ErrLoadoutLimit      = errors.New("cosmeticloadout: player already has the maximum number of saved loadouts")
	ErrCosmeticNotOwned  = errors.New("cosmeticloadout: cosmetic is premium and not owned")
	ErrNotFound          = errors.New("cosmeticloadout: not found")
)

// ownershipChecker is the one method Service needs from
// *cosmeticpurchase.Service — kept narrow so tests never need a real
// DynamoDB-backed entitlement store.
type ownershipChecker interface {
	IsOwned(ctx context.Context, playerID string, kind cosmetics.Kind, itemID string) (bool, error)
}

// applySelectionFunc actually applies one kind's selection to the player's
// live profile. Injected rather than importing *player.Service directly,
// same reasoning as cosmeticpurchase's currentSelectionFunc: it keeps this
// package testable without a real player store, and avoids this package
// having to know player's exact method signatures.
type applySelectionFunc func(ctx context.Context, playerID string, kind cosmetics.Kind, itemID string) error

type Service struct {
	store     *Store
	ownership ownershipChecker
	apply     applySelectionFunc
}

func NewService(store *Store, ownership ownershipChecker) *Service {
	return &Service{store: store, ownership: ownership}
}

// SetApplyFunc wires in the callback Apply uses to actually change the
// player's active deck/felt selection. Apply fails closed (ErrApplyUnwired)
// if this was never called — a wiring mistake must never look like a no-op
// success on a feature that touches ownership-gated cosmetics.
func (s *Service) SetApplyFunc(fn applySelectionFunc) { s.apply = fn }

var ErrApplyUnwired = errors.New("cosmeticloadout: apply function not wired")

// normalizeSelections validates and trims a client-supplied selections map:
// every key must be a known cosmetics.Kind, every value a known catalog id
// for that kind, and a premium id must already be owned by playerID.
func (s *Service) normalizeSelections(ctx context.Context, playerID string, selections map[string]string) (map[string]string, error) {
	if len(selections) == 0 {
		return nil, ErrInvalidSelections
	}
	out := make(map[string]string, len(selections))
	for kindStr, itemID := range selections {
		kindStr = strings.TrimSpace(kindStr)
		itemID = strings.TrimSpace(itemID)
		if !knownKind(kindStr) || itemID == "" {
			return nil, ErrInvalidSelections
		}
		kind := cosmetics.Kind(kindStr)
		if !cosmetics.IsKnown(kind, itemID) {
			return nil, ErrInvalidSelections
		}
		if cosmetics.IsPremium(kind, itemID) {
			owned, err := s.checkOwned(ctx, playerID, kind, itemID)
			if err != nil {
				return nil, err
			}
			if !owned {
				return nil, ErrCosmeticNotOwned
			}
		}
		out[kindStr] = itemID
	}
	return out, nil
}

// checkOwned fails closed, same reasoning as player.Service.requireCosmetic:
// missing ownership wiring must never silently pass a premium item through.
func (s *Service) checkOwned(ctx context.Context, playerID string, kind cosmetics.Kind, itemID string) (bool, error) {
	if s.ownership == nil {
		return false, nil
	}
	return s.ownership.IsOwned(ctx, playerID, kind, itemID)
}

// Create validates name/selections, enforces maxLoadoutsPerPlayer, and
// persists a new loadout. Ownership of any premium item named in selections
// is checked here (save time) and, independently, re-checked by Apply —
// never trusted from one to the other, since a refund can happen in between.
func (s *Service) Create(ctx context.Context, playerID, name string, selections map[string]string) (*Loadout, error) {
	name = normalizeName(name)
	if name == "" {
		return nil, ErrInvalidName
	}
	normalized, err := s.normalizeSelections(ctx, playerID, selections)
	if err != nil {
		return nil, err
	}
	existing, err := s.store.List(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if len(existing) >= maxLoadoutsPerPlayer {
		return nil, ErrLoadoutLimit
	}
	loadout := Loadout{
		PlayerID: playerID, LoadoutID: uuid.New().String(), Name: name,
		Selections: normalized, CreatedAt: nowRFC3339(),
	}
	if err := s.store.Create(ctx, loadout); err != nil {
		return nil, err
	}
	return &loadout, nil
}

func (s *Service) List(ctx context.Context, playerID string) ([]Loadout, error) {
	return s.store.List(ctx, playerID)
}

func (s *Service) Delete(ctx context.Context, playerID, loadoutID string) error {
	ok, err := s.store.Delete(ctx, playerID, loadoutID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// Apply re-validates every selection in the saved loadout against current
// ownership (never trusting the save-time check — see Create's doc comment)
// and then, only if every item still checks out, applies each one through
// the injected callback. A no-longer-owned item fails the whole apply rather
// than silently skipping it, so a player is never left with a
// partially-applied loadout because one item was refunded after saving.
func (s *Service) Apply(ctx context.Context, playerID, loadoutID string) (*Loadout, error) {
	loadout, err := s.store.Get(ctx, playerID, loadoutID)
	if err != nil {
		return nil, err
	}
	if loadout == nil {
		return nil, ErrNotFound
	}
	if s.apply == nil {
		return nil, ErrApplyUnwired
	}
	// Re-validate before applying anything: a saved selection must still be a
	// known catalog id and, if premium, still owned.
	revalidated, err := s.normalizeSelections(ctx, playerID, loadout.Selections)
	if err != nil {
		return nil, err
	}
	for kindStr, itemID := range revalidated {
		if err := s.apply(ctx, playerID, cosmetics.Kind(kindStr), itemID); err != nil {
			return nil, err
		}
	}
	return loadout, nil
}
