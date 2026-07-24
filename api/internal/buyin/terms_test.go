package buyin

import (
	"context"
	"errors"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/player"
)

type gateWallet struct{ debits int }

func (w *gateWallet) Credit(context.Context, string, int64, string, string) error { return nil }
func (w *gateWallet) Debit(context.Context, string, int64, string, string) error {
	w.debits++
	return nil
}
func (w *gateWallet) HoldGame(context.Context, string, int64, string, string, string) (string, error) {
	return "h1", nil
}
func (w *gateWallet) ReleaseHold(context.Context, string) error { return nil }
func (w *gateWallet) CashoutGame(context.Context, string, int64, string, []string, string, string) error {
	return nil
}

type unacceptedProfiles struct{}

func (unacceptedProfiles) GetOrCreate(context.Context, string) (*player.PlayerProfile, error) {
	return &player.PlayerProfile{UserID: "u1"}, nil
}
func (unacceptedProfiles) AcceptTerms(context.Context, string) error     { return nil }
func (unacceptedProfiles) SetName(context.Context, string, string) error { return nil }
func (unacceptedProfiles) SetWalletMode(context.Context, string, string) error {
	return nil
}

func TestBuyInRequiresPokerTermsBeforeWalletDebit(t *testing.T) {
	wallet := &gateWallet{}
	players := player.NewService(unacceptedProfiles{})
	svc := NewServiceWithPlayers(wallet, nil, nil, players)
	err := svc.BuyIn(context.Background(), "room-1", "u1", 400, false, "")
	if !errors.Is(err, player.ErrTermsNotAccepted) {
		t.Fatalf("got %v", err)
	}
	if wallet.debits != 0 {
		t.Fatalf("wallet debited %d times", wallet.debits)
	}
}
