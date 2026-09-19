// Package walletalert persists per-player wallet-alert preferences (issue
// #304, desmembrado de #215): a configurable minimum sandbox-balance
// threshold and a per-purchase spend ceiling. walletclient.Client itself
// stays stateless by design (see api/CLAUDE.md's walletclient conventions)
// — this is a small, separate store, evaluated against Balances/purchase
// responses the caller already has in hand rather than issuing any new
// ctech-wallet M2M call.
package walletalert

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const tablePrefs = "poker_wallet_alert_prefs"

// Prefs is one player's wallet-alert configuration. A zero threshold means
// "not configured" — Evaluate/EvaluatePurchase never fire on it.
type Prefs struct {
	PlayerID          string `dynamodbav:"pk" json:"-"`
	MinSandboxBalance int64  `dynamodbav:"min_sandbox_balance,omitempty" json:"min_sandbox_balance"`
	MaxPurchaseCents  int64  `dynamodbav:"max_purchase_cents,omitempty" json:"max_purchase_cents"`
	UpdatedAt         string `dynamodbav:"updated_at" json:"updated_at"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tablePrefs)}
}

// Get returns playerID's prefs, or (nil, nil) if the player never
// configured any — the "no preference configured" case the acceptance
// criteria explicitly require Evaluate to treat as no alert.
func (s *Store) Get(ctx context.Context, playerID string) (*Prefs, error) {
	item, err := s.base.GetItem(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("walletalert: get: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	return dynamo.Decode[Prefs](item)
}

// Put overwrites playerID's prefs wholesale. This is a rare, player-
// initiated write with no correctness stake — Evaluate/EvaluatePurchase
// never gate a business decision on it, only a notification (see #304's
// explicit acceptance criterion) — so a plain overwrite is enough; unlike
// every money-moving store in this repo, there is nothing here for a
// conditional write to protect.
func (s *Store) Put(ctx context.Context, prefs Prefs) error {
	encoded, err := dynamo.Encode(prefs)
	if err != nil {
		return fmt.Errorf("walletalert: encode: %w", err)
	}
	if err := s.base.PutItem(ctx, encoded); err != nil {
		return fmt.Errorf("walletalert: put: %w", err)
	}
	return nil
}

// Delete removes playerID's prefs — "remover um limiar" from the
// acceptance criteria.
func (s *Store) Delete(ctx context.Context, playerID string) error {
	if _, err := s.base.DeleteItem(ctx, playerID); err != nil {
		return fmt.Errorf("walletalert: delete: %w", err)
	}
	return nil
}
