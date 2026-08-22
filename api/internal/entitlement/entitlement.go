// Package entitlement persists the real-money table-reservation entitlement:
// the fixed table-entry fee is charged once per table (or tier, via rebind)
// and covers a fixed window, not once per buy-in/rebuy — see
// docs/plans/2026-08-21-entry-fee-entitlement.md for the legal reasoning.
package entitlement

import (
	"errors"
	"time"
)

// Window is the entitlement's validity, absolute from the moment it is
// claimed (buyin.Service.chargeEntryFee sets ExpiresAt = now+Window). Never
// slid forward on later activity — a renewing window would make the paid
// reservation free poker forever.
const Window = 3 * time.Hour

// ErrAlreadyClaimed means an entitlement already exists for this
// (playerID, originTableID) pair — the conditional Claim write lost a race
// (or is a same-request retry). The caller treats this the same as its own
// successful claim.
var ErrAlreadyClaimed = errors.New("entitlement: already claimed")

// ErrNotFound means Rebind's target row does not exist, or exists but has
// already expired — both are the same "nothing left to move" outcome for the
// caller, which should fall back to treating the reservation as gone.
var ErrNotFound = errors.New("entitlement: not found or expired")

// Entitlement is one player's paid table reservation. OriginTableID (the
// table the fee was actually charged for) is immutable — it is the
// idempotency key (sk) that stops a concurrent buy-in from double-charging.
// BoundTableID is the table the reservation currently grants access to; it
// starts equal to OriginTableID and moves only via Rebind, when the bound
// table becomes unavailable (archived or full) — see buyin.Service.
type Entitlement struct {
	PlayerID      string
	OriginTableID string
	BoundTableID  string
	Tier          string
	FeeCents      int64
	ExpiresAt     time.Time
	CreatedAt     time.Time
}
