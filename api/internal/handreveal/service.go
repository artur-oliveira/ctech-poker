package handreveal

import (
	"context"
	"fmt"
)

// wallet is the narrow slice of walletclient.Client this package needs —
// mirrors dailyreward's `credit` interface so tests use a fake, not a real
// HTTP client.
type wallet interface {
	Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

const (
	reasonDebit  = "hand_reveal_history"
	reasonCredit = "hand_reveal_history_payout"
)

type Service struct {
	wallet   wallet
	payments *PaymentStore
}

func NewService(wallet wallet, payments *PaymentStore) *Service {
	return &Service{wallet: wallet, payments: payments}
}

func (s *Service) HasPaid(ctx context.Context, handID, viewerID string) (bool, error) {
	return s.payments.HasPaid(ctx, handID, viewerID)
}

// PayForReveal debits buyerID the full fee, credits winnerID half (integer
// division — the remainder is uncredited rake, same split as
// Table.RequestWinnerCards), and records the purchase. Idempotent per
// (handID, buyerID): ClaimPayment reuses an existing pending row instead of
// creating a second one, and both wallet calls carry a stable idempotency
// key, so a retried call after a partial failure (e.g. the debit succeeded
// but the credit didn't) resumes safely instead of double-charging the
// buyer or double-crediting the winner.
func (s *Service) PayForReveal(ctx context.Context, buyerID, winnerID, handID string, fee int64) error {
	status, err := s.payments.ClaimPayment(ctx, handID, buyerID)
	if err != nil {
		return fmt.Errorf("handreveal: claim payment: %w", err)
	}
	if status == StatusCompleted {
		return nil
	}
	debitKey := handID + "#" + buyerID + "#debit"
	if err := s.wallet.Debit(ctx, buyerID, fee, debitKey, reasonDebit); err != nil {
		return err
	}
	creditKey := handID + "#" + buyerID + "#credit"
	if err := s.wallet.Credit(ctx, winnerID, fee/2, creditKey, reasonCredit); err != nil {
		return err
	}
	return s.payments.CompletePayment(ctx, handID, buyerID)
}
