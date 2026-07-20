// Package buyin orchestrates sandbox chip movement (ctech-wallet) with
// seating a player into a live table (Phase 2's table.Actor). Debit-then-seat
// on buy-in, remove-then-credit on cash-out — see this plan's Architecture
// note for why the order is fixed and never the other way round.
package buyin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
)

// walletMover is the subset of *walletclient.Client this service needs —
// narrowed to an interface so tests can fake it without a live HTTP server.
type walletMover interface {
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

type Service struct {
	wallet  walletMover
	manager *tablemanager.Manager
	rooms   *roomstore.Store
	players interface {
		RequireAccepted(context.Context, string) error
	}
}

// ErrTermsNotAccepted is re-exported at the buy-in boundary so callers do
// not need to know which internal service enforces the gate.
var ErrTermsNotAccepted = player.ErrTermsNotAccepted

var ErrUnsupportedCurrencyMode = errors.New("buyin: unsupported currency mode")

func NewService(wallet walletMover, manager *tablemanager.Manager, rooms *roomstore.Store) *Service {
	return &Service{wallet: wallet, manager: manager, rooms: rooms}
}

func NewServiceWithPlayers(wallet walletMover, manager *tablemanager.Manager, rooms *roomstore.Store, players *player.Service) *Service {
	return &Service{wallet: wallet, manager: manager, rooms: rooms, players: players}
}

// seedFor builds the first-touch table seed for roomID, using the room's real
// stakes when they can be looked up and falling back to the 10/20 placeholder
// otherwise (nil rooms store, lookup error, or unknown room) — same fallback
// convention as app.defaultSeed. Task 5 centralizes this as roomBackedSeed and
// wires it at the app/WS layer; this is only the copy buyin needs for its own
// GetOrCreateActor calls. seed() only ever runs on a table's very first touch,
// so the fallback only matters if buyin is the first thing to touch the table.
func (s *Service) seedFor(ctx context.Context, roomID string) func() *hand.Table {
	return func() *hand.Table {
		if s.rooms != nil {
			if room, err := s.rooms.Get(ctx, roomID); err == nil && room != nil {
				return table.SeedForRoom(room)
			}
		}
		return hand.NewTable(nil, 10, 20)
	}
}

// BuyIn debits amount from playerID's sandbox wallet, then seats them into
// roomID's live table. If seating fails, the debit is immediately reversed
// with a distinct idempotency key (":refund" suffix) so the reversal can
// never collide with — or be mistaken as a retry of — the original debit.
func (s *Service) BuyIn(ctx context.Context, roomID, playerID string, amount int64, midHand bool, idemKey string) error {
	maxSeats := 0
	if s.rooms != nil {
		room, err := s.rooms.Get(ctx, roomID)
		if err != nil {
			return fmt.Errorf("buyin: load room: %w", err)
		}
		if room == nil {
			return fmt.Errorf("buyin: room not found")
		}
		if room.CurrencyMode != "sandbox" {
			return ErrUnsupportedCurrencyMode
		}
		if room.BigBlind <= 0 || amount < room.BuyInMin || amount > room.BuyInMax || amount <= 0 || amount%room.BigBlind != 0 {
			return fmt.Errorf("buyin: amount outside room limits")
		}
		maxSeats = room.MaxSeats
	}
	if s.players != nil {
		if err := s.players.RequireAccepted(ctx, playerID); err != nil {
			return err
		}
	}
	// Stable per (room, player) key so a retried request cannot double-debit.
	// Repeat rebuys are not allowed; a genuine rebuy carries a fresh client
	// nonce, which becomes a distinct key. When the caller passes nothing we
	// fall back to the playerID (still stable, still retry-safe).
	nonce := idemKey
	if nonce == "" {
		nonce = playerID
	}
	key := fmt.Sprintf("%s#%s#buyin#%s", roomID, playerID, nonce)
	if err := s.wallet.Debit(ctx, playerID, amount, key, "poker_buyin"); err != nil {
		return fmt.Errorf("buyin: debit: %w", err)
	}

	actor, err := s.manager.GetOrCreateActor(ctx, roomID, s.seedFor(ctx, roomID))
	if err != nil || actor == nil {
		if refundErr := s.wallet.Credit(ctx, playerID, amount, idemKey+":refund", "poker_buyin_refund"); refundErr != nil {
			return fmt.Errorf("buyin: table unavailable AND refund failed (manual reconciliation needed): actor=%v refund=%w", err, refundErr)
		}
		return fmt.Errorf("buyin: table unavailable, debit refunded: %w", err)
	}

	reply := make(chan error, 1)
	joinErr := actor.Dispatch(table.JoinCmd{PlayerID: playerID, Stack: amount, MaxSeats: maxSeats, MidHand: midHand, Reply: reply})
	if joinErr != nil {
		if refundErr := s.wallet.Credit(ctx, playerID, amount, idemKey+":refund", "poker_buyin_refund"); refundErr != nil {
			return fmt.Errorf("buyin: seat failed AND refund failed (manual reconciliation needed): seat=%v refund=%w", joinErr, refundErr)
		}
		return fmt.Errorf("buyin: seat failed, debit refunded: %w", joinErr)
	}
	return nil
}

// CashOut removes playerID from roomID's live table and credits their final
// stack back to the sandbox wallet. Unlike BuyIn, there is no compensating
// action on failure: if the credit call fails after a successful removal, the
// player's chips are gone from the table but not yet in their wallet — this is
// flagged as a genuine gap (see Task 3's closing note), not silently glossed
// over.
func (s *Service) CashOut(ctx context.Context, roomID, playerID, idemKey string) (int64, error) {
	if s.rooms != nil {
		room, err := s.rooms.Get(ctx, roomID)
		if err != nil {
			return 0, fmt.Errorf("buyin: load room: %w", err)
		}
		if room == nil {
			return 0, fmt.Errorf("buyin: room not found")
		}
		if room.CurrencyMode != "sandbox" {
			return 0, ErrUnsupportedCurrencyMode
		}
	}
	actor, err := s.manager.GetOrCreateActor(ctx, roomID, s.seedFor(ctx, roomID))
	if err != nil || actor == nil {
		return 0, fmt.Errorf("buyin: table unavailable: %w", err)
	}

	stackCh := make(chan int64, 1)
	reply := make(chan error, 1)
	if err := actor.Dispatch(table.LeaveCmd{PlayerID: playerID, Stack: stackCh, Reply: reply}); err != nil {
		return 0, fmt.Errorf("buyin: leave: %w", err)
	}
	stack := <-stackCh

	// Stable per (room, player) key by default; a fresh client nonce per
	// cash-out click makes a rebuy-then-cashout distinct (and still retry-safe).
	key := fmt.Sprintf("%s#%s#cashout", roomID, playerID)
	if idemKey != "" {
		key = fmt.Sprintf("%s#%s#cashout#%s", roomID, playerID, idemKey)
	}
	if err := s.wallet.Credit(ctx, playerID, stack, key, "poker_cashout"); err != nil {
		// Seat already removed; the chips are between table and wallet. Surface
		// a clear error and log the exact (player, amount) for ops reconciliation.
		slog.Error("buyin: cash-out credit failed after seat removal — manual reconciliation",
			"player", playerID, "room", roomID, "amount", stack, "err", err)
		return stack, fmt.Errorf("buyin: cash-out credit failed after seat removal — manual reconciliation needed for %s amount %d: %w", playerID, stack, err)
	}
	return stack, nil
}
