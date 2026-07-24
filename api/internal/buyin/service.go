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
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
)

// walletMover is the subset of *walletclient.Client this service needs —
// narrowed to an interface so tests can fake it without a live HTTP server.
type walletMover interface {
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	HoldGame(ctx context.Context, userID string, amount int64, tableRef, idempotencyKey, reason string) (string, error)
	ReleaseHold(ctx context.Context, holdID string) error
	CashoutGame(ctx context.Context, userID string, amount int64, tableRef string, holdIDs []string, idempotencyKey, reason string) error
}

type roomLookup interface {
	Get(ctx context.Context, roomID string) (*roomstore.Room, error)
}

type activationChecker interface {
	IsGamblingActivated(ctx context.Context, userID string) (bool, error)
}

type Service struct {
	wallet     walletMover
	game       walletMover
	manager    *tablemanager.Manager
	rooms      roomLookup
	activation activationChecker
	pending    *reconcile.PendingStore
	sessions   *sessionlog.Store
	players    interface {
		RequireAccepted(context.Context, string) error
	}
}

// ErrTermsNotAccepted is re-exported at the buy-in boundary so callers do
// not need to know which internal service enforces the gate.
var ErrTermsNotAccepted = player.ErrTermsNotAccepted

var ErrUnsupportedCurrencyMode = errors.New("buyin: unsupported currency mode")

func NewService(wallet walletMover, manager *tablemanager.Manager, rooms roomLookup) *Service {
	return &Service{wallet: wallet, manager: manager, rooms: rooms}
}

func NewServiceWithGame(wallet, game walletMover, manager *tablemanager.Manager, rooms roomLookup, activation activationChecker) *Service {
	return &Service{wallet: wallet, game: game, manager: manager, rooms: rooms, activation: activation}
}

func (s *Service) WithPendingStore(pending *reconcile.PendingStore) *Service {
	s.pending = pending
	return s
}

func (s *Service) WithSessionStore(sessions *sessionlog.Store) *Service {
	s.sessions = sessions
	return s
}

func NewServiceWithPlayers(wallet walletMover, manager *tablemanager.Manager, rooms roomLookup, players *player.Service) *Service {
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

func (s *Service) walletFor(ctx context.Context, roomID, playerID string) (walletMover, error) {
	room, err := s.rooms.Get(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("buyin: room lookup: %w", err)
	}
	if room == nil || room.CurrencyMode != "real" {
		return s.wallet, nil
	}
	if s.game == nil || s.activation == nil {
		return nil, fmt.Errorf("buyin: room %s is real-money but this Service was built without NewServiceWithGame", roomID)
	}
	ok, err := s.activation.IsGamblingActivated(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("buyin: activation check: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("buyin: player %s has not activated gambling on ctech-wallet", playerID)
	}
	return s.game, nil
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

	actor, err := s.manager.GetOrCreateActor(ctx, roomID, s.seedFor(ctx, roomID))
	if err != nil || actor == nil {
		return fmt.Errorf("buyin: table unavailable: %w", err)
	}

	seated, err := s.isSeated(actor, playerID)
	if err != nil {
		return fmt.Errorf("buyin: seat check: %w", err)
	}
	if seated {
		return nil
	}

	nonce := idemKey
	if nonce == "" {
		nonce = playerID
	}
	key := fmt.Sprintf("%s#%s#buyin#%s", roomID, playerID, nonce)

	mover, err := s.walletFor(ctx, roomID, playerID)
	if err != nil {
		return fmt.Errorf("buyin: %w", err)
	}

	var holdID string
	if mover == s.game {
		holdID, err = mover.HoldGame(ctx, playerID, amount, roomID, key, "poker_buyin")
		if err != nil {
			return fmt.Errorf("buyin: hold: %w", err)
		}
	} else {
		if err := mover.Debit(ctx, playerID, amount, key, "poker_buyin"); err != nil {
			return fmt.Errorf("buyin: debit: %w", err)
		}
	}

	reply := make(chan error, 1)
	joinErr := actor.Dispatch(table.JoinCmd{PlayerID: playerID, Stack: amount, MaxSeats: maxSeats, MidHand: midHand, HoldID: holdID, Reply: reply})
	if joinErr != nil {
		if errors.Is(joinErr, hand.ErrAlreadySeated) {
			return nil
		}
		if mover == s.game {
			if refundErr := mover.ReleaseHold(ctx, holdID); refundErr != nil {
				return fmt.Errorf("buyin: seat failed AND release failed (manual reconciliation needed): seat=%v refund=%w", joinErr, refundErr)
			}
		} else {
			if refundErr := mover.Credit(ctx, playerID, amount, idemKey+":refund", "poker_buyin_refund"); refundErr != nil {
				return fmt.Errorf("buyin: seat failed AND refund failed (manual reconciliation needed): seat=%v refund=%w", joinErr, refundErr)
			}
		}
		return fmt.Errorf("buyin: seat failed, debit refunded: %w", joinErr)
	}

	if s.sessions != nil {
		if err := s.sessions.RecordSession(ctx, sessionlog.SessionItem{
			PK: playerID, TableID: roomID, BuyinAmount: amount, JoinedAt: time.Now().UnixMilli(),
		}); err != nil {
			slog.Error("sessionlog: record session open failed", "player", playerID, "table", roomID, "err", err)
		}
	}

	return nil
}

// isSeated reports whether playerID already has a seat at the table. It reads
// the current viewer snapshot from the actor's Run goroutine (hand.Table has
// no lock), so it is safe to call concurrently with the actor's own
// broadcastAll.
func (s *Service) isSeated(actor *table.Actor, playerID string) (bool, error) {
	snapCh := make(chan hand.Snapshot, 1)
	reply := make(chan error, 1)
	if err := actor.Dispatch(table.SnapshotCmd{PlayerID: playerID, Snapshot: snapCh, Reply: reply}); err != nil {
		return false, err
	}
	select {
	case snap := <-snapCh:
		for _, seat := range snap.Seats {
			if seat.PlayerID == playerID {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, nil
	}
}

// Seated reports whether playerID currently holds a seat at roomID's live
// table and, if so, their current stack. Unlike isSeated (which reuses an
// actor the caller already has, e.g. mid-BuyIn), this acquires its own actor
// handle — it is the read path for GET /rooms/:id/seated, which lets a
// player reconnecting from a different device or a closed/reopened tab find
// out their real seat state from the server instead of guessing from local
// client storage.
func (s *Service) Seated(ctx context.Context, roomID, playerID string) (bool, int64, error) {
	actor, err := s.manager.GetOrCreateActor(ctx, roomID, s.seedFor(ctx, roomID))
	if err != nil || actor == nil {
		return false, 0, fmt.Errorf("buyin: table unavailable: %w", err)
	}

	snapCh := make(chan hand.Snapshot, 1)
	reply := make(chan error, 1)
	if err := actor.Dispatch(table.SnapshotCmd{PlayerID: playerID, Snapshot: snapCh, Reply: reply}); err != nil {
		return false, 0, err
	}
	select {
	case snap := <-snapCh:
		for _, seat := range snap.Seats {
			if seat.PlayerID == playerID {
				return true, seat.Stack, nil
			}
		}
		return false, 0, nil
	default:
		return false, 0, nil
	}
}

// CashOut removes playerID from roomID's live table and credits their final
// stack back to the appropriate wallet. For real-money rooms, credits the
// game wallet using the hold IDs returned from the seat; for sandbox, credits
// the sandbox wallet directly.
func (s *Service) CashOut(ctx context.Context, roomID, playerID, idemKey string) (int64, error) {
	mover, err := s.walletFor(ctx, roomID, playerID)
	if err != nil {
		return 0, fmt.Errorf("buyin: %w", err)
	}

	actor, err := s.manager.GetOrCreateActor(ctx, roomID, s.seedFor(ctx, roomID))
	if err != nil || actor == nil {
		return 0, fmt.Errorf("buyin: table unavailable: %w", err)
	}

	stackCh := make(chan int64, 1)
	holdIDCh := make(chan string, 1)
	reply := make(chan error, 1)
	if err := actor.Dispatch(table.LeaveCmd{PlayerID: playerID, Stack: stackCh, HoldID: holdIDCh, Reply: reply}); err != nil {
		return 0, fmt.Errorf("buyin: leave: %w", err)
	}
	stack := <-stackCh
	holdID := <-holdIDCh

	// Stable per (room, player) key by default; a fresh client nonce per
	// cash-out click makes a rebuy-then-cashout distinct (and still retry-safe).
	key := fmt.Sprintf("%s#%s#cashout", roomID, playerID)
	if idemKey != "" {
		key = fmt.Sprintf("%s#%s#cashout#%s", roomID, playerID, idemKey)
	}

	return stack, s.settle(ctx, roomID, playerID, stack, holdID, mover, key)
}

// SettleSystemRemoval credits a player's final stack back to their wallet and
// closes their sessionlog entry after table.Actor has already removed them
// outside any client request — an AFK sweep or a disconnect kick timeout
// (wired through tablemanager's onPlayerRemoved hook in app.go). Those
// removals commit inside the Actor's own goroutine before this Service ever
// sees them, so unlike CashOut there is no seat left to protect by failing
// early: without this, a system-removed player's chips were simply discarded
// (never credited to any wallet) and their session stayed open forever,
// which is also why the lobby's "return to table" banner could point at a
// table the player no longer holds a seat at.
func (s *Service) SettleSystemRemoval(ctx context.Context, roomID, playerID string, stack int64, holdID, reason string) error {
	mover, err := s.walletFor(ctx, roomID, playerID)
	if err != nil {
		return fmt.Errorf("buyin: settle system removal: %w", err)
	}
	key := fmt.Sprintf("%s#%s#system_leave#%s", roomID, playerID, reason)
	return s.settle(ctx, roomID, playerID, stack, holdID, mover, key)
}

// settle is CashOut and SettleSystemRemoval's shared tail: record a
// pending-cashout safety net, credit the resolved wallet, then close the
// player's open sessionlog entry. mover and the idempotency key are resolved
// by the caller since CashOut must fail before removing the player if the
// wallet can't be determined, while SettleSystemRemoval has no such seat to
// protect (the removal already happened).
func (s *Service) settle(ctx context.Context, roomID, playerID string, stack int64, holdID string, mover walletMover, key string) error {
	if s.pending != nil {
		mode := "sandbox"
		if room, _ := s.rooms.Get(ctx, roomID); room != nil {
			mode = room.CurrencyMode
		}
		var holdIDs []string
		if holdID != "" {
			holdIDs = []string{holdID}
		}
		_ = s.pending.Record(ctx, reconcile.PendingCashout{
			ID:             key,
			PlayerID:       playerID,
			Amount:         stack,
			CurrencyMode:   mode,
			HoldIDs:        holdIDs,
			TableRef:       roomID,
			IdempotencyKey: key,
		})
	}

	if mover == s.game {
		if holdID == "" {
			return fmt.Errorf("buyin: no hold ID found for player %s", playerID)
		}
		if err := mover.CashoutGame(ctx, playerID, stack, roomID, []string{holdID}, key, "poker_cashout"); err != nil {
			slog.Error("buyin: cash-out credit failed after seat removal — reconciliation job will retry",
				"player", playerID, "room", roomID, "amount", stack, "hold_id", holdID, "err", err)
			return fmt.Errorf("buyin: cash-out credit failed after seat removal — reconciliation job will retry for %s amount %d: %w", playerID, stack, err)
		}
	} else if err := mover.Credit(ctx, playerID, stack, key, "poker_cashout"); err != nil {
		slog.Error("buyin: cash-out credit failed after seat removal — reconciliation job will retry",
			"player", playerID, "room", roomID, "amount", stack, "err", err)
		return fmt.Errorf("buyin: cash-out credit failed after seat removal — reconciliation job will retry for %s amount %d: %w", playerID, stack, err)
	}

	if s.pending != nil {
		_ = s.pending.MarkResolved(ctx, key)
	}

	if s.sessions != nil {
		if open, err := s.sessions.FindOpenSession(ctx, playerID, roomID); err == nil && open != nil {
			open.EndedAt = time.Now().UnixMilli()
			open.CashoutAmount = stack
			open.NetPnL = stack - open.BuyinAmount
			if err := s.sessions.CloseSession(ctx, *open); err != nil {
				slog.Error("sessionlog: close session failed", "player", playerID, "table", roomID, "err", err)
			}
		}
	}

	return nil
}
