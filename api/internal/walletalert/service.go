package walletalert

import (
	"context"
	"time"

	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// AlertKind names which threshold an Alert crossed.
type AlertKind string

const (
	AlertLowSandboxBalance     AlertKind = "low_sandbox_balance"
	AlertPurchaseLimitExceeded AlertKind = "purchase_limit_exceeded"
)

// Alert is a fired wallet-alert threshold. Delivery (in-app/push) is the
// caller's concern; this package only decides whether one fired.
type Alert struct {
	Kind AlertKind `json:"kind"`
}

type store interface {
	Get(ctx context.Context, playerID string) (*Prefs, error)
	Put(ctx context.Context, prefs Prefs) error
	Delete(ctx context.Context, playerID string) error
}

type Service struct {
	store store
	now   func() time.Time
}

func NewService(store store) *Service { return &Service{store: store, now: time.Now} }

func (s *Service) Get(ctx context.Context, playerID string) (*Prefs, error) {
	return s.store.Get(ctx, playerID)
}

// Set upserts playerID's thresholds. A zero value disables that particular
// threshold without needing a separate Delete.
func (s *Service) Set(ctx context.Context, playerID string, minSandboxBalance, maxPurchaseCents int64) (Prefs, error) {
	prefs := Prefs{
		PlayerID: playerID, MinSandboxBalance: minSandboxBalance, MaxPurchaseCents: maxPurchaseCents,
		UpdatedAt: s.now().UTC().Format(time.RFC3339Nano),
	}
	if err := s.store.Put(ctx, prefs); err != nil {
		return Prefs{}, err
	}
	return prefs, nil
}

func (s *Service) Delete(ctx context.Context, playerID string) error {
	return s.store.Delete(ctx, playerID)
}

// Evaluate reports which of prefs's configured thresholds balances has
// crossed. Pure function over data the caller already fetched for another
// reason (a Balances response) — never issues a wallet call itself, and per
// #304's explicit acceptance criterion is never used for a business
// decision, only to notify. A nil prefs (nothing configured) always yields
// nil, and so does a zero threshold.
func Evaluate(prefs *Prefs, balances walletclient.Balances) []Alert {
	if prefs == nil {
		return nil
	}
	var alerts []Alert
	if prefs.MinSandboxBalance > 0 && balances.SandboxBalance < prefs.MinSandboxBalance {
		alerts = append(alerts, Alert{Kind: AlertLowSandboxBalance})
	}
	return alerts
}

// EvaluatePurchase reports whether a single purchase of purchaseAmountCents
// breaches prefs's configured per-purchase ceiling. Same rule as Evaluate:
// the caller already has purchaseAmountCents from a purchase response it
// just received, so this adds no new read anywhere.
//
// ponytail: a per-purchase ceiling, not a rolling daily/weekly spend total
// (the issue's UX copy mentions both). A rolling total needs aggregating
// purchase history — a real new read this repo doesn't already do at
// purchase time — so it's left out; add it if product actually asks for
// it. A per-purchase ceiling already covers the issue title's "limite de
// compra".
func EvaluatePurchase(prefs *Prefs, purchaseAmountCents int64) []Alert {
	if prefs == nil || prefs.MaxPurchaseCents <= 0 {
		return nil
	}
	if purchaseAmountCents > prefs.MaxPurchaseCents {
		return []Alert{{Kind: AlertPurchaseLimitExceeded}}
	}
	return nil
}
