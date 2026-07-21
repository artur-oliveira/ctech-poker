package roulette

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
)

type SpinRecord struct {
	Amount int64
	Status string
}

type tier struct {
	amount int64
	weight int
}

var tiers = []tier{{100, 50}, {200, 30}, {500, 15}, {1000, 5}}

type credit interface {
	Credit(context.Context, string, int64, string, string) error
}

// spinStore persists the selected prize before the external wallet call. A
// retry therefore always uses the same amount and idempotency key.
type spinStore interface {
	Claim(context.Context, string, string, int64, time.Time) (SpinRecord, error)
	Complete(context.Context, string, string, time.Time) error
	Get(context.Context, string, string) (SpinRecord, error)
}

type Service struct {
	wallet credit
	store  spinStore
	now    func() time.Time
	pick   func() (int64, error)
}

func NewService(wallet credit, store spinStore) *Service {
	return &Service{wallet: wallet, store: store, now: time.Now, pick: pickTier}
}

func (s *Service) Spin(ctx context.Context, playerID string) (int64, error) {
	if playerID == "" {
		return 0, fmt.Errorf("roulette: empty player id")
	}
	now := s.now()
	day := cooldownKey(now)
	proposed, err := s.pick()
	if err != nil {
		return 0, fmt.Errorf("roulette: pick tier: %w", err)
	}
	record, err := s.store.Claim(ctx, playerID, day, proposed, now)
	if err != nil {
		return 0, fmt.Errorf("roulette: claim spin: %w", err)
	}
	if record.Status == StatusCompleted {
		return record.Amount, nil
	}

	idemKey := fmt.Sprintf("%s#roulette#%s", playerID, day)
	if err := s.wallet.Credit(ctx, playerID, record.Amount, idemKey, "sandbox_roulette"); err != nil {
		return 0, fmt.Errorf("roulette: credit pending: %w", err)
	}
	if err := s.store.Complete(ctx, playerID, day, now); err != nil {
		return 0, fmt.Errorf("roulette: mark completed: %w", err)
	}
	return record.Amount, nil
}

func (s *Service) RemainingTime(ctx context.Context, playerID string) (int64, error) {
	if playerID == "" {
		return 0, fmt.Errorf("roulette: empty player id")
	}
	now := s.now()
	day := cooldownKey(now)
	record, err := s.store.Get(ctx, playerID, day)
	if err != nil {
		return 0, fmt.Errorf("roulette: get record: %w", err)
	}
	if record.Amount == 0 && record.Status == "" {
		return 0, nil
	}

	nowBRT := now.In(brt)
	tomorrow := time.Date(nowBRT.Year(), nowBRT.Month(), nowBRT.Day()+1, 0, 0, 0, 0, brt)
	return int64(tomorrow.Sub(nowBRT).Seconds()), nil
}

func pickTier() (int64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	roll := int(binary.BigEndian.Uint64(b[:]) % 100)
	for _, t := range tiers {
		if roll < t.weight {
			return t.amount, nil
		}
		roll -= t.weight
	}
	return tiers[0].amount, nil
}
