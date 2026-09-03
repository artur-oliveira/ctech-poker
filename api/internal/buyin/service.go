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

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"gopkg.aoctech.app/api-commons/observability"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/entitlement"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// walletMover is the subset of *walletclient.Client this service needs —
// narrowed to an interface so tests can fake it without a live HTTP server.
type walletMover interface {
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	HoldGame(ctx context.Context, userID string, amount int64, tableRef, idempotencyKey, reason string) (string, error)
	ReleaseHold(ctx context.Context, holdID string) error
	CashoutGame(ctx context.Context, userID string, amount int64, tableRef string, holdIDs []string, idempotencyKey, reason string) error
	DebitReal(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	Balances(ctx context.Context, userID string) (*walletclient.Balances, error)
}

type roomLookup interface {
	Get(ctx context.Context, roomID string) (*roomstore.Room, error)
}

type activationChecker interface {
	IsGamblingActivated(ctx context.Context, userID string) (bool, error)
}

type pendingStore interface {
	BuildRecordTx(reconcile.PendingCashout) (types.TransactWriteItem, error)
	Record(context.Context, reconcile.PendingCashout) error
	Get(context.Context, string) (*reconcile.PendingCashout, error)
	MarkResolved(context.Context, string) error
}

// entitlementStore is the subset of *entitlement.Store the real-money entry
// fee needs — narrowed to an interface so tests can fake it.
type entitlementStore interface {
	ActiveFor(ctx context.Context, playerID string) ([]entitlement.Entitlement, error)
	Claim(ctx context.Context, e entitlement.Entitlement) error
	Rebind(ctx context.Context, playerID, originTableID, expectedBoundTableID, newTableID string) error
}

type Service struct {
	wallet       walletMover
	game         walletMover
	manager      *tablemanager.Manager
	rooms        roomLookup
	activation   activationChecker
	pending      pendingStore
	entitlements entitlementStore
	now          func() time.Time
	sessions     *sessionlog.Store
	players      interface {
		RequireAccepted(context.Context, string) error
		GetOrCreate(context.Context, string) (*player.PlayerProfile, error)
	}
	avatarBaseURL string
	stats         interface {
		Get(context.Context, string, string) (pokerstats.Stats, error)
	}
	presence interface {
		SetInTable(context.Context, string, string) error
		Reconcile(context.Context, string) error
	}
}

// ErrTermsNotAccepted is re-exported at the buy-in boundary so callers do
// not need to know which internal service enforces the gate.
var ErrTermsNotAccepted = player.ErrTermsNotAccepted

var ErrUnsupportedCurrencyMode = errors.New("buyin: unsupported currency mode")

func NewService(wallet walletMover, manager *tablemanager.Manager, rooms roomLookup) *Service {
	return &Service{wallet: wallet, manager: manager, rooms: rooms, now: time.Now}
}

func NewServiceWithGame(wallet, game walletMover, manager *tablemanager.Manager, rooms roomLookup, activation activationChecker) *Service {
	return &Service{wallet: wallet, game: game, manager: manager, rooms: rooms, activation: activation, now: time.Now}
}

func (s *Service) WithPendingStore(pending pendingStore) *Service {
	s.pending = pending
	return s
}

func (s *Service) WithEntitlements(entitlements entitlementStore) *Service {
	s.entitlements = entitlements
	return s
}

func (s *Service) WithSessionStore(sessions *sessionlog.Store) *Service {
	s.sessions = sessions
	return s
}

func (s *Service) WithPlayers(players *player.Service) *Service {
	s.players = players
	return s
}

func (s *Service) WithAvatarBaseURL(baseURL string) *Service {
	s.avatarBaseURL = baseURL
	return s
}

func (s *Service) WithPokerStats(stats interface {
	Get(context.Context, string, string) (pokerstats.Stats, error)
}) *Service {
	s.stats = stats
	return s
}

func (s *Service) WithPresence(presence interface {
	SetInTable(context.Context, string, string) error
	Reconcile(context.Context, string) error
}) *Service {
	s.presence = presence
	return s
}

func NewServiceWithPlayers(wallet walletMover, manager *tablemanager.Manager, rooms roomLookup, players *player.Service) *Service {
	return &Service{wallet: wallet, manager: manager, rooms: rooms, players: players, now: time.Now}
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
// with a distinct idempotency key (the composite debit key plus a ":refund"
// suffix) so the reversal can never collide with — or be mistaken as a retry
// of — the original debit, nor collide with another player's refund.
func (s *Service) BuyIn(ctx context.Context, roomID, playerID string, amount int64, midHand bool, idemKey string) error {
	return s.buyIn(ctx, roomID, playerID, amount, midHand, false, idemKey)
}

// BuyInWithAutoRebuy is BuyIn plus the one-time auto-rebuy opt-in. Only
// meaningful for a brand-new seat: hand.Table.rebuyExisting ignores the
// incoming Player's AutoRebuy field entirely, so calling this on an
// already-seated player's rebuy is a harmless no-op, never a way to
// retroactively flip auto-rebuy on for an existing seat.
func (s *Service) BuyInWithAutoRebuy(ctx context.Context, roomID, playerID string, amount int64, midHand, autoRebuy bool, idemKey string) error {
	return s.buyIn(ctx, roomID, playerID, amount, midHand, autoRebuy, idemKey)
}

func (s *Service) buyIn(ctx context.Context, roomID, playerID string, amount int64, midHand, autoRebuy bool, idemKey string) error {
	maxSeats := 0
	var room *roomstore.Room
	if s.rooms != nil {
		var err error
		room, err = s.rooms.Get(ctx, roomID)
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

	seated, stack, occupiedSeats, err := s.isSeated(actor, playerID)
	if err != nil {
		return fmt.Errorf("buyin: seat check: %w", err)
	}
	// A seated player with chips left is an idempotent retry of a join
	// already committed — no-op. A seated player with Stack<=0 (busted, still
	// occupying the seat per runShowdown) is a genuine rebuy and must fall
	// through to debit + dispatch below, or the credit never happens.
	if seated && stack > 0 {
		return nil
	}
	// This is a fast-fail optimization, not the correctness mechanism: another
	// player can still win the final seat after this snapshot. The actor's
	// conditional join remains authoritative and the debit is compensated on
	// that race, but already-full tables no longer cause an avoidable
	// debit/refund round trip.
	if !seated && maxSeats > 0 && occupiedSeats >= maxSeats {
		return table.ErrNoSeatsAvailable
	}

	nonce := idemKey
	if nonce == "" {
		nonce = playerID
	}
	key := fmt.Sprintf("%s#%s#buyin#%s", roomID, playerID, nonce)

	// The table-entry fee is resolved and charged before any stack money
	// moves and before the player is seated (docs/plans/2026-08-21-entry-fee-entitlement.md):
	// a failure here aborts the buy-in cleanly, with nobody seated and no
	// stack debit to refund.
	if room != nil && room.CurrencyMode == roomstore.CurrencyModeReal && room.EntryFeeCents > 0 {
		if err := s.resolveEntitlement(ctx, room, playerID); err != nil {
			return err
		}
	}

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
	joinErr := actor.Dispatch(table.JoinCmd{PlayerID: playerID, Stack: amount, MaxSeats: maxSeats, MidHand: midHand, HoldID: holdID, AutoRebuy: autoRebuy, Reply: reply})
	if joinErr != nil {
		// hand.ErrAlreadySeated here is NOT a same-request retry — the isSeated
		// check above already short-circuits those before any debit happens.
		// Reaching this means a concurrent BuyIn for the same player won the
		// seat race after we passed isSeated but before this Dispatch landed:
		// our own debit above moved real money that will never be seated, so
		// it must be refunded like any other failed join, never silently
		// dropped.
		if mover == s.game {
			if refundErr := mover.ReleaseHold(ctx, holdID); refundErr != nil {
				return fmt.Errorf("buyin: seat failed AND release failed (manual reconciliation needed): seat=%v refund=%w", joinErr, refundErr)
			}
		} else {
			// Derive the refund key from the composite debit `key`, never the
			// raw idemKey: idemKey is caller-optional and empty on the auto-rebuy
			// sweep and webhook paths, so `idemKey+":refund"` collapses to the
			// constant ":refund" for every player and ctech-wallet dedupes the
			// second race-loser's refund away entirely (#42). `key` already folds
			// in roomID, playerID and the nonce (itself playerID when idemKey is
			// empty), so it is globally unique per refund while a genuine retry of
			// the SAME failed buy-in still reproduces it and dedupes.
			if refundErr := mover.Credit(ctx, playerID, amount, key+":refund", "poker_buyin_refund"); refundErr != nil {
				return fmt.Errorf("buyin: seat failed AND refund failed (manual reconciliation needed): seat=%v refund=%w", joinErr, refundErr)
			}
		}
		return fmt.Errorf("buyin: seat failed, debit refunded: %w", joinErr)
	}

	// Seed the display name at seating time, not lazily on this player's own
	// WS connect (tablews.go does that too, for reconnects) — otherwise every
	// other viewer sees this seat as "Visitante" for however long it takes
	// this player's client to actually open its socket after buying in.
	if s.players != nil {
		if profile, perr := s.players.GetOrCreate(ctx, playerID); perr != nil {
			observability.Warn(ctx, "buyin player profile lookup failed", perr, "table_id", roomID)
		} else if profile != nil {
			playstyleBadge := ""
			if profile.PlaystylePublic && s.stats != nil {
				if playerStats, statsErr := s.stats.Get(ctx, playerID, room.CurrencyMode); statsErr != nil {
					observability.Warn(ctx, "buyin player stats lookup failed", statsErr, "table_id", roomID)
				} else {
					if badges := pokerstats.StyleFor(playerStats, pokerstats.MinHandsPublic); len(badges) > 0 {
						playstyleBadge = badges[0].Key
					}
				}
			}
			nameReply := make(chan error, 1)
			if err := actor.Dispatch(table.SetIdentityCmd{PlayerID: playerID, Name: profile.Name,
				AvatarURL: player.AvatarURL(profile, s.avatarBaseURL), PlaystyleBadge: playstyleBadge, Reply: nameReply}); err != nil {
				observability.Warn(ctx, "buyin table identity dispatch failed", err, "table_id", roomID)
			}
		}
	}

	if s.sessions != nil {
		if open, err := s.sessions.FindOpenSession(ctx, playerID, roomID); err == nil && open != nil {
			// Atomic conditional ADD, not a read-modify-write: `key` is the
			// same composite idempotency key already used for the wallet
			// debit above, so a retried buy-in can never double-count this
			// rebuy in the session's cumulative total (issue #70).
			if err := s.sessions.AddBuyin(ctx, playerID, open.SK, amount, key); err != nil {
				slog.Error("sessionlog: update session rebuy failed", "player", playerID, "table", roomID, "err", err)
			}
		} else {
			if err := s.sessions.RecordSession(ctx, sessionlog.SessionItem{
				PK: playerID, TableID: roomID, BuyinAmount: amount, JoinedAt: time.Now().UnixMilli(),
			}); err != nil {
				slog.Error("sessionlog: record session open failed", "player", playerID, "table", roomID, "err", err)
			}
		}
	}
	if s.presence != nil {
		if err := s.presence.SetInTable(ctx, playerID, roomID); err != nil {
			slog.Warn("presence: mark player in table failed", "player", playerID, "table", roomID, "err", err)
		}
	}

	return nil
}

// resolveEntitlement makes sure playerID may enter room for free (an
// existing or newly-rebound reservation) or charges a fresh one — see
// docs/plans/2026-08-21-entry-fee-entitlement.md's Fase 2. Called only for
// real-money rooms with a non-zero entry fee; the caller must not have moved
// any stack money yet.
func (s *Service) resolveEntitlement(ctx context.Context, room *roomstore.Room, playerID string) error {
	if s.entitlements == nil {
		return errors.New("buyin: entitlement store unavailable; refusing real-money admission with an entry fee")
	}
	ents, err := s.entitlements.ActiveFor(ctx, playerID)
	if err != nil {
		return fmt.Errorf("buyin: load entitlements: %w", err)
	}
	for _, e := range ents {
		if e.Tier == room.Tier && e.BoundTableID == room.ID {
			// The row existing is NOT proof the fee was paid — the claim
			// commits before the debit precisely so a failed debit can be
			// retried under the same key. Confirm (or complete) the charge.
			return s.confirmFeeCharged(ctx, playerID, e)
		}
	}
	for _, e := range ents {
		if e.Tier != room.Tier || e.BoundTableID == room.ID {
			continue
		}
		unavailable, err := s.tableUnavailable(ctx, e.BoundTableID)
		if err != nil {
			// A failed availability check on some OTHER table must never block
			// this buy-in — skip this entitlement as a rebind candidate and let
			// the loop (or a fresh charge below) decide.
			slog.Warn("buyin: entitlement rebind availability check failed, skipping", "player", playerID, "table", e.BoundTableID, "err", err)
			continue
		}
		if !unavailable {
			continue
		}
		if err := s.entitlements.Rebind(ctx, playerID, e.OriginTableID, e.BoundTableID, room.ID); err != nil {
			if errors.Is(err, entitlement.ErrNotFound) {
				continue // lost the race (expired, deleted, or bound_table_id moved by a concurrent buy-in) — try the next candidate
			}
			return fmt.Errorf("buyin: rebind entitlement: %w", err)
		}
		// Same reasoning as the exact-match loop above: a rebound reservation
		// only covers this seat if its own fee actually cleared.
		return s.confirmFeeCharged(ctx, playerID, e)
	}
	return s.chargeEntryFee(ctx, room, playerID)
}

// tableUnavailable reports whether tableID (some OTHER real-money table a
// player's entitlement is currently bound to) is archived/deleted or full —
// the two conditions under which a reservation may rebind to a different
// table of the same tier (see the plan's "O rebind" section).
func (s *Service) tableUnavailable(ctx context.Context, tableID string) (bool, error) {
	room, err := s.rooms.Get(ctx, tableID)
	if err != nil {
		return false, fmt.Errorf("buyin: load bound table's room: %w", err)
	}
	if room == nil {
		// cmd/tablecleanup deletes the room row only after confirming the
		// table itself is archived — a missing room is a confirmed-unavailable
		// table, not an ambiguous one.
		return true, nil
	}
	// Two cheap reads instead of materialising an actor for a table this
	// request is not joining (issue #57): archiving that raced the room-row
	// delete is a single tablestore read, and occupancy comes from the
	// write-through seats_taken field the lobby already relies on.
	archived, err := s.manager.IsArchived(ctx, tableID)
	if err != nil {
		return false, fmt.Errorf("buyin: archived check for bound table: %w", err)
	}
	if archived {
		return true, nil
	}
	return room.MaxSeats > 0 && room.SeatsTaken >= room.MaxSeats, nil
}

// entitlementFeeKey is the idempotency key for e's entry-fee debit and for
// its poker_pending_cashouts recovery row. It is derived only from durable
// facts about the entitlement itself — the table it was claimed at, the
// player, and the exact reservation window — never from a per-request nonce,
// so any later request (a retry, a concurrent claim loser, cmd/reconcile's
// sweep) reproduces the same key and the wallet charges at most once. A
// genuinely new reservation at the same table after the window lapses gets a
// fresh ExpiresAt, hence a fresh key, and is charged again.
func entitlementFeeKey(playerID string, e entitlement.Entitlement) string {
	return fmt.Sprintf("%s#%s#buyinfee#%d", e.OriginTableID, playerID, e.ExpiresAt.Unix())
}

// confirmFeeCharged is the single "this player's entry fee is covered"
// decision. An entitlement row alone never answers it: Claim deliberately
// commits before DebitReal so a failed debit leaves a reservation with no
// payment behind it, and treating that row as coverage is what let a player
// take a free seat when the charge ultimately failed (issue #40). Coverage
// is decided by the fee's own recovery row instead — resolved means paid;
// anything else (unresolved, or never recorded because the process died
// between Claim and Record) means this request completes the same
// idempotent charge before anyone is seated.
func (s *Service) confirmFeeCharged(ctx context.Context, playerID string, e entitlement.Entitlement) error {
	if s.pending == nil {
		return errors.New("buyin: settlement store unavailable; refusing real-money admission with an entry fee")
	}
	if e.FeeCents <= 0 {
		return nil
	}
	feeKey := entitlementFeeKey(playerID, e)
	row, err := s.pending.Get(ctx, feeKey)
	if err != nil {
		return fmt.Errorf("buyin: load entry fee recovery row: %w", err)
	}
	if row != nil && row.Resolved {
		return nil
	}
	return s.payEntryFee(ctx, playerID, e, feeKey)
}

// payEntryFee records the recovery obligation, charges it, then resolves it.
// Record is immutable-first-write and DebitReal is deduped by feeKey, so
// running this concurrently (or repeatedly) for the same entitlement moves
// money at most once.
func (s *Service) payEntryFee(ctx context.Context, playerID string, e entitlement.Entitlement, feeKey string) error {
	if err := s.pending.Record(ctx, reconcile.PendingCashout{
		ID: feeKey, PlayerID: playerID, Amount: e.FeeCents, CurrencyMode: roomstore.CurrencyModeReal,
		Kind: reconcile.KindFeeDebit, TableRef: e.OriginTableID, IdempotencyKey: feeKey,
	}); err != nil {
		return fmt.Errorf("buyin: persist fee recovery intent: %w", err)
	}
	if err := s.game.DebitReal(ctx, playerID, e.FeeCents, feeKey, "poker_table_fee"); err != nil {
		slog.Error("ALARM: poker table-entry fee charge failed before seating; reconciliation will retry", "player", playerID, "room", e.OriginTableID, "amount", e.FeeCents, "err", err)
		return fmt.Errorf("buyin: table fee charge failed, reconciliation will retry: %w", err)
	}
	if err := s.pending.MarkResolved(ctx, feeKey); err != nil {
		// Not fatal: the charge itself is done and the row is idempotent, so
		// the next buy-in (or cmd/reconcile) re-runs the same deduped debit
		// and marks it resolved then.
		slog.Error("ALARM: poker table fee charged but recovery intent was not resolved", "fee_key", feeKey, "err", err)
	}
	return nil
}

// chargeEntryFee claims a fresh entitlement for room and charges its fixed
// fee. Claim commits before DebitReal so a debit failure never leaves a
// charge with no matching reservation; the entitlement itself is left in
// place on a debit failure (never seating this player) so an idempotent
// retry — this request's caller retrying the whole buy-in, or
// cmd/reconcile's fee_debit sweep — can complete the same charge exactly
// once, keyed by entitlementFeeKey.
func (s *Service) chargeEntryFee(ctx context.Context, room *roomstore.Room, playerID string) error {
	if s.pending == nil {
		return errors.New("buyin: settlement store unavailable; refusing real-money admission with an entry fee")
	}
	claim := entitlement.Entitlement{
		PlayerID: playerID, OriginTableID: room.ID, BoundTableID: room.ID,
		Tier: room.Tier, FeeCents: room.EntryFeeCents, ExpiresAt: s.now().Add(entitlement.Window),
	}
	if err := s.entitlements.Claim(ctx, claim); err != nil {
		if !errors.Is(err, entitlement.ErrAlreadyClaimed) {
			return fmt.Errorf("buyin: claim entitlement: %w", err)
		}
		// A concurrent BuyIn for this same (player, room) won the claim. The
		// loser must NOT report "covered" off the winner's mere existence:
		// the winner's own DebitReal may still fail, and seating on that
		// promise is the free-seat window of issue #40. Re-read the winning
		// row — its ExpiresAt, not ours, keys the charge — and confirm or
		// complete the same idempotent debit.
		winner, err := s.activeEntitlementAt(ctx, playerID, room.ID)
		if err != nil {
			return err
		}
		if winner == nil {
			// Claim said "still valid" but ActiveFor cannot see it: refuse the
			// seat rather than grant a possibly-unpaid one.
			return errors.New("buyin: entitlement claim lost the race but the winning reservation could not be read; refusing to seat")
		}
		return s.confirmFeeCharged(ctx, playerID, *winner)
	}
	return s.payEntryFee(ctx, playerID, claim, entitlementFeeKey(playerID, claim))
}

// activeEntitlementAt returns playerID's still-valid entitlement claimed at
// originTableID, or nil if there is none.
func (s *Service) activeEntitlementAt(ctx context.Context, playerID, originTableID string) (*entitlement.Entitlement, error) {
	ents, err := s.entitlements.ActiveFor(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("buyin: reload entitlements: %w", err)
	}
	for _, e := range ents {
		if e.OriginTableID == originTableID {
			return &e, nil
		}
	}
	return nil, nil
}

// FeeStatus reports what GET /rooms/:id/seated shows a player before they
// buy in: whether room's fixed entry fee is still due, and when their
// covering reservation expires if not. Read-only — never claims, charges, or
// rebinds anything, so it does not attempt the heavier same-tier rebind
// check chargeEntryFee/resolveEntitlement perform; a player whose only
// coverage is a rebindable entitlement on a different (now unavailable)
// table sees fee_due=true here even though BuyIn would let them in for free.
func (s *Service) FeeStatus(ctx context.Context, room *roomstore.Room, playerID string) (feeDue bool, expiresAtUnix int64, err error) {
	if room == nil || room.CurrencyMode != roomstore.CurrencyModeReal || room.EntryFeeCents <= 0 {
		return false, 0, nil
	}
	if s.entitlements == nil {
		return true, 0, nil
	}
	ents, err := s.entitlements.ActiveFor(ctx, playerID)
	if err != nil {
		return true, 0, fmt.Errorf("buyin: load entitlements: %w", err)
	}
	for _, e := range ents {
		if e.Tier != room.Tier || e.BoundTableID != room.ID {
			continue
		}
		// Same rule as resolveEntitlement: coverage is the fee's settled
		// recovery row, not the reservation row. A reservation whose debit
		// never cleared still owes a fee, and BuyIn will charge it — so
		// reporting fee_due=false here would be exactly the silent surprise
		// charge this endpoint exists to prevent.
		if paid, err := s.feeSettled(ctx, playerID, e); err != nil {
			return true, 0, err
		} else if paid {
			return false, e.ExpiresAt.Unix(), nil
		}
		return true, 0, nil
	}
	return true, 0, nil
}

// feeSettled reports whether e's entry fee has actually been charged and
// resolved. Read-only counterpart of confirmFeeCharged.
func (s *Service) feeSettled(ctx context.Context, playerID string, e entitlement.Entitlement) (bool, error) {
	if e.FeeCents <= 0 {
		return true, nil
	}
	if s.pending == nil {
		return false, nil
	}
	row, err := s.pending.Get(ctx, entitlementFeeKey(playerID, e))
	if err != nil {
		return false, fmt.Errorf("buyin: load entry fee recovery row: %w", err)
	}
	return row != nil && row.Resolved, nil
}

// isSeated reports whether playerID already has a seat at the table. It reads
// the current viewer snapshot from the actor's Run goroutine (hand.Table has
// no lock), so it is safe to call concurrently with the actor's own
// broadcastAll.
func (s *Service) isSeated(actor *table.Actor, playerID string) (bool, int64, int, error) {
	snapCh := make(chan hand.Snapshot, 1)
	reply := make(chan error, 1)
	if err := actor.Dispatch(table.SnapshotCmd{PlayerID: playerID, Snapshot: snapCh, Reply: reply}); err != nil {
		return false, 0, 0, err
	}
	select {
	case snap := <-snapCh:
		for _, seat := range snap.Seats {
			if seat.PlayerID == playerID {
				return true, seat.Stack, len(snap.Seats), nil
			}
		}
		return false, 0, len(snap.Seats), nil
	default:
		return false, 0, 0, nil
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

// SandboxBalance reports playerID's current sandbox wallet balance. Always
// reads the sandbox wallet (s.wallet), never the real-money one (s.game) —
// its only caller, the post-hand auto-rebuy sweep, is sandbox-only by design
// (see docs/specs/2026-08-10-auto-buyin-design.md's Scope section).
func (s *Service) SandboxBalance(ctx context.Context, playerID string) (int64, error) {
	balances, err := s.wallet.Balances(ctx, playerID)
	if err != nil {
		return 0, fmt.Errorf("buyin: balances: %w", err)
	}
	return balances.SandboxBalance, nil
}

// SeatSummary is deliberately narrower than a full hand.SeatView — only what
// app.autoRebuySweep needs to decide whether a busted seat should
// self-resolve.
type SeatSummary struct {
	Seated      bool
	Stack       int64
	AutoRebuy   bool
	BuyInAmount int64
}

// SeatedSummary is Seated plus the seat's auto-rebuy configuration. Kept
// separate from Seated (the read path for GET /rooms/:id/seated) so that
// endpoint's response shape never has to grow fields meant only for the
// internal sweep.
func (s *Service) SeatedSummary(ctx context.Context, roomID, playerID string) (SeatSummary, error) {
	actor, err := s.manager.GetOrCreateActor(ctx, roomID, s.seedFor(ctx, roomID))
	if err != nil || actor == nil {
		return SeatSummary{}, fmt.Errorf("buyin: table unavailable: %w", err)
	}

	snapCh := make(chan hand.Snapshot, 1)
	reply := make(chan error, 1)
	if err := actor.Dispatch(table.SnapshotCmd{PlayerID: playerID, Snapshot: snapCh, Reply: reply}); err != nil {
		return SeatSummary{}, err
	}
	select {
	case snap := <-snapCh:
		for _, seat := range snap.Seats {
			if seat.PlayerID == playerID {
				return SeatSummary{Seated: true, Stack: seat.Stack, AutoRebuy: seat.AutoRebuy, BuyInAmount: seat.BuyInAmount}, nil
			}
		}
		return SeatSummary{}, nil
	default:
		return SeatSummary{}, nil
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
	if s.pending == nil {
		return 0, errors.New("buyin: settlement store unavailable; refusing to remove seat")
	}

	mode := "sandbox"
	if room, err := s.rooms.Get(ctx, roomID); err != nil {
		return 0, fmt.Errorf("buyin: room lookup for settlement: %w", err)
	} else if room != nil {
		mode = room.CurrencyMode
	}

	// Stable per (room, player) key by default; a fresh client nonce per
	// cash-out click makes a rebuy-then-cashout distinct (and still retry-safe).
	key := fmt.Sprintf("%s#%s#cashout", roomID, playerID)
	if idemKey != "" {
		key = fmt.Sprintf("%s#%s#cashout#%s", roomID, playerID, idemKey)
	}

	actor, err := s.manager.GetOrCreateActor(ctx, roomID, s.seedFor(ctx, roomID))
	if err != nil || actor == nil {
		return 0, fmt.Errorf("buyin: table unavailable: %w", err)
	}

	stackCh := make(chan int64, 1)
	holdIDCh := make(chan string, 1)
	reply := make(chan error, 1)
	buildIntent := func(stack int64, holdID string) (types.TransactWriteItem, error) {
		var holdIDs []string
		if holdID != "" {
			holdIDs = []string{holdID}
		}
		return s.pending.BuildRecordTx(reconcile.PendingCashout{
			ID: key, PlayerID: playerID, Amount: stack, CurrencyMode: mode,
			HoldIDs: holdIDs, TableRef: roomID, IdempotencyKey: key,
		})
	}
	if err := actor.Dispatch(table.LeaveCmd{
		PlayerID: playerID, Stack: stackCh, HoldID: holdIDCh,
		SettlementIntent: buildIntent, Reply: reply,
	}); err != nil {
		return 0, fmt.Errorf("buyin: leave: %w", err)
	}
	stack := <-stackCh
	holdID := <-holdIDCh
	return stack, s.settle(ctx, roomID, playerID, stack, holdID, mover, key)
}

// systemLeaveKey builds the poker_pending_cashouts idempotency key for one
// system removal. The nonce is what keeps a rebuy-then-removed-again cycle at
// the same table from colliding onto a single create-only key: without it the
// second removal's whole co-committed transaction failed its condition
// forever and the seat could never be pulled (the player was wedged
// "leaving…" until an idle sweep with a different reason caught them). Mirrors
// the client nonce CashOut already appends. Falls back to the bare key when
// no nonce is supplied so an older caller (or a replayed pre-fix obligation
// row) still resolves.
func systemLeaveKey(roomID, playerID, reason, nonce string) string {
	if nonce == "" {
		return fmt.Sprintf("%s#%s#system_leave#%s", roomID, playerID, reason)
	}
	return fmt.Sprintf("%s#%s#system_leave#%s#%s", roomID, playerID, reason, nonce)
}

// BuildSystemSettlementIntent creates the immutable recovery row that an
// Actor co-writes with an AFK/disconnect/exit seat removal. nonce must be the
// same value the matching SettleSystemRemoval call receives (both come from
// the Actor's per-removal newSettlementNonce) or the follow-up credit would
// resolve a different key and reconcile would credit the wallet twice.
func (s *Service) BuildSystemSettlementIntent(ctx context.Context, roomID, playerID, reason, nonce string, stack int64, holdID string) (types.TransactWriteItem, error) {
	if s.pending == nil {
		return types.TransactWriteItem{}, errors.New("buyin: settlement store unavailable")
	}
	mode := "sandbox"
	room, err := s.rooms.Get(ctx, roomID)
	if err != nil {
		return types.TransactWriteItem{}, fmt.Errorf("buyin: load room for settlement intent: %w", err)
	}
	if room != nil {
		mode = room.CurrencyMode
	}
	key := systemLeaveKey(roomID, playerID, reason, nonce)
	var holdIDs []string
	if holdID != "" {
		holdIDs = []string{holdID}
	}
	return s.pending.BuildRecordTx(reconcile.PendingCashout{
		ID: key, PlayerID: playerID, Amount: stack, CurrencyMode: mode,
		HoldIDs: holdIDs, TableRef: roomID, IdempotencyKey: key,
	})
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
func (s *Service) SettleSystemRemoval(ctx context.Context, roomID, playerID string, stack int64, holdID, reason, nonce string) error {
	mover, err := s.walletFor(ctx, roomID, playerID)
	if err != nil {
		return fmt.Errorf("buyin: settle system removal: %w", err)
	}
	key := systemLeaveKey(roomID, playerID, reason, nonce)
	return s.settle(ctx, roomID, playerID, stack, holdID, mover, key)
}

// settle is CashOut and SettleSystemRemoval's shared tail: record a
// pending-cashout safety net, credit the resolved wallet, then close the
// player's open sessionlog entry. mover and the idempotency key are resolved
// by the caller since CashOut must fail before removing the player if the
// wallet can't be determined, while SettleSystemRemoval has no such seat to
// protect (the removal already happened).
func (s *Service) settle(ctx context.Context, roomID, playerID string, stack int64, holdID string, mover walletMover, key string) error {
	if s.pending == nil {
		return errors.New("buyin: settlement store unavailable")
	}
	mode := "sandbox"
	room, err := s.rooms.Get(ctx, roomID)
	if err != nil {
		return fmt.Errorf("buyin: load settlement room: %w", err)
	}
	if room != nil {
		mode = room.CurrencyMode
	}
	var holdIDs []string
	if holdID != "" {
		holdIDs = []string{holdID}
	}
	if err := s.pending.Record(ctx, reconcile.PendingCashout{
		ID: key, PlayerID: playerID, Amount: stack, CurrencyMode: mode,
		HoldIDs: holdIDs, TableRef: roomID, IdempotencyKey: key,
	}); err != nil {
		return fmt.Errorf("buyin: persist settlement recovery intent: %w", err)
	}

	// stack == 0 (player busted, nothing to return) — skip the wallet call
	// entirely, ctech-wallet's Credit/CashoutGame reject a zero amount as a
	// validation error, and there is nothing to reconcile for a $0 credit.
	if stack > 0 {
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
	}

	if err := s.pending.MarkResolved(ctx, key); err != nil {
		return fmt.Errorf("buyin: settlement completed but recovery intent finalization failed: %w", err)
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
	if s.presence != nil {
		if err := s.presence.Reconcile(ctx, playerID); err != nil {
			slog.Warn("presence: reconcile after table exit failed", "player", playerID, "table", roomID, "err", err)
		}
	}

	return nil
}
