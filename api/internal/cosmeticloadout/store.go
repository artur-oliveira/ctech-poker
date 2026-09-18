// Package cosmeticloadout implements issue #313: named, saved deck+felt (and
// future kind) combinations a player can preview and apply in one action
// instead of switching each cosmetic slot separately.
//
// Ownership is never re-modeled here — cosmeticpurchase.Service.IsOwned stays
// the single source of truth for "does this player own this premium item",
// both when a loadout is saved and, again, when it is applied (an item can be
// refunded after being saved into a loadout, so Apply re-checks rather than
// trusting the save-time check). Applying a loadout reuses
// player.Service.SetDeckVariant/SetTableTheme — the same validate-then-persist
// path a manual per-slot change already goes through — via a small injected
// callback, mirroring how cosmeticpurchase itself avoids importing player
// directly (see internal/app.wireCosmeticCurrentSelection).
package cosmeticloadout

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
)

const tableLoadouts = "poker_cosmetic_loadouts"

// maxLoadoutsPerPlayer bounds both the DynamoDB partition and the choice
// surface a client has to render — the same reasoning issue #220 already
// established elsewhere in this module: never let a per-player item list grow
// unbounded. Because the cap is enforced on every write, List never needs a
// cursor: a player's whole loadout set always fits in one page.
const maxLoadoutsPerPlayer = 5

// maxNameLen mirrors player.maxDisplayNameLen's reasoning: a client-supplied
// label with no length ceiling is an unbounded string in a DynamoDB item.
const maxNameLen = 60

// Loadout is one named, saved cosmetic combination. Selections is keyed by
// cosmetics.Kind ("deck", "felt", ...) — bounded by the fixed, tiny set of
// kinds cosmetics.go defines, never by anything a player controls the length
// of, so it carries none of #220's unbounded-list risk.
type Loadout struct {
	PlayerID   string            `dynamodbav:"pk" json:"-"`
	LoadoutID  string            `dynamodbav:"sk" json:"id"`
	Name       string            `dynamodbav:"name" json:"name"`
	Selections map[string]string `dynamodbav:"selections" json:"selections"`
	CreatedAt  string            `dynamodbav:"created_at" json:"created_at"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableLoadouts)}
}

// Create persists a new loadout row. The caller (Service.Create) is
// responsible for the per-player cap and every content validation — this is
// the storage layer only.
func (s *Store) Create(ctx context.Context, l Loadout) error {
	encoded, err := dynamo.Encode(l)
	if err != nil {
		return fmt.Errorf("cosmeticloadout: encode loadout: %w", err)
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(encoded)}); err != nil {
		return fmt.Errorf("cosmeticloadout: create loadout: %w", err)
	}
	return nil
}

// List returns every loadout this player has saved. There is deliberately no
// pagination parameter: maxLoadoutsPerPlayer keeps the whole set inside one
// DynamoDB page forever.
func (s *Store) List(ctx context.Context, playerID string) ([]Loadout, error) {
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: playerID, Limit: maxLoadoutsPerPlayer + 1})
	if err != nil {
		return nil, fmt.Errorf("cosmeticloadout: list loadouts: %w", err)
	}
	out := make([]Loadout, 0, len(result.Items))
	for _, item := range result.Items {
		l, err := dynamo.Decode[Loadout](item)
		if err != nil {
			return nil, fmt.Errorf("cosmeticloadout: decode loadout: %w", err)
		}
		out = append(out, *l)
	}
	return out, nil
}

func (s *Store) Get(ctx context.Context, playerID, loadoutID string) (*Loadout, error) {
	item, err := s.base.GetItem(ctx, playerID, loadoutID)
	if err != nil {
		return nil, fmt.Errorf("cosmeticloadout: get loadout: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	return dynamo.Decode[Loadout](item)
}

func (s *Store) Delete(ctx context.Context, playerID, loadoutID string) (bool, error) {
	return s.base.DeleteItem(ctx, playerID, loadoutID)
}

// normalizeName trims and bounds a client-supplied loadout name.
func normalizeName(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > maxNameLen {
		name = name[:maxNameLen]
	}
	return name
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// knownKind reports whether kind is one cosmetics.go actually defines —
// Selections must never carry an arbitrary client-supplied key.
func knownKind(kind string) bool {
	return kind == string(cosmetics.KindDeck) || kind == string(cosmetics.KindFelt)
}
