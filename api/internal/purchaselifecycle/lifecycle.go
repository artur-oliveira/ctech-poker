// Package purchaselifecycle holds the parts of the PIX product-purchase
// lifecycle that reactionpurchase and cosmeticpurchase had copied line for
// line: the status vocabulary, wallet error classification, and the transition
// matrix a re-verified wallet status implies for the local purchase row.
//
// Only the *decision* lives here, never the writes. Each product package keeps
// its own DynamoDB transactions, its own entitlement rules (a reaction's
// used_at, a cosmetic's current selection) and its own error values — the
// duplication issue #211 is about was the state machine, and sharing the
// effects too would mean an adapter interface over both stores for no
// correctness gain.
package purchaselifecycle

import (
	"errors"
	"net/http"

	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// Purchase and entitlement statuses. Wallet owns the purchase vocabulary;
// StatusProcessing (a sandbox-fichas debit in flight) and StatusActive (an
// entitlement that actually grants ownership) are poker-side additions.
const (
	StatusProcessing = "processing"
	StatusPending    = "pending"
	StatusActive     = "active"
	StatusConfirmed  = "confirmed"
	StatusRefunding  = "refunding"
	StatusRefunded   = "refunded"
	StatusFailed     = "failed"
	StatusExpired    = "expired"
)

// Terminal reports whether wallet will never move a purchase out of status on
// its own. These are the statuses that release a reservation or revoke a
// grant; "confirmed" is deliberately not one of them, since a confirmed
// purchase can still be refunded.
func Terminal(status string) bool {
	return status == StatusFailed || status == StatusExpired || status == StatusRefunded
}

// DefinitiveWalletRejection reports whether wallet refused a call in a way
// that can never succeed on retry, which is the only case where a caller may
// roll back its pre-charge reservation. Transport failures, 5xx, 408, 425 and
// 429 are deliberately excluded: an ambiguous call may already have moved
// money, so its reservation must survive for the retry to resume from.
func DefinitiveWalletRejection(err error) bool {
	var walletErr *walletclient.Error
	return errors.As(err, &walletErr) && walletErr.Status >= http.StatusBadRequest &&
		walletErr.Status < http.StatusInternalServerError && walletErr.Status != http.StatusRequestTimeout &&
		walletErr.Status != http.StatusTooEarly && walletErr.Status != http.StatusTooManyRequests
}

// Step is the one action a re-verified wallet status implies for the local
// purchase row.
type Step string

const (
	// StepIgnore: local state already accounts for what wallet reports.
	StepIgnore Step = "ignore"
	// StepDrop: no local row and nothing safe to reconstruct from — wallet
	// knows a purchase id poker does not, in a status that is not recoverable.
	StepDrop Step = "drop"
	// StepGrantConfirmed: converge history and entitlement on a paid purchase.
	StepGrantConfirmed Step = "grant_confirmed"
	// StepCompleteRefund: wallet finished a refund this side had begun.
	StepCompleteRefund Step = "complete_refund"
	// StepCloseTerminal: record the terminal status and release the
	// reservation (or revoke the grant, when wallet is authoritative that the
	// purchase was refunded).
	StepCloseTerminal Step = "close_terminal"
	// StepUpdateStatus: a non-terminal status moved (e.g. pending → whatever
	// wallet reports next); history follows, ownership does not change.
	StepUpdateStatus Step = "update_status"
)

// Decision is what the caller must do with one re-verified wallet purchase:
// reconstruct the missing local row first when Recover is set, then run Step.
type Decision struct {
	Recover bool
	Step    Step
}

// recoverable lists the wallet statuses a missing local row may be
// reconstructed from. A confirmed purchase is handled before this (it grants),
// and anything else — an unknown or future wallet status — is dropped rather
// than guessed at, because reconstructing a row is what later drives an
// entitlement claim.
func recoverable(walletStatus string) bool {
	return walletStatus == StatusPending || Terminal(walletStatus)
}

// Decide is the single transition matrix for create, re-verify, refund
// completion and webhook delivery. walletStatus is what wallet just reported;
// localStatus is the local row's status, or nil when there is no local row.
//
// The invariants it encodes, all of which were duplicated per product package
// before #211:
//   - Wallet purchase states are monotonic: a locally confirmed grant is never
//     regressed by a stale non-terminal read.
//   - A refund already in flight locally wins over a confirmed wallet read —
//     re-confirming would resurrect ownership the refund just revoked.
//   - Only a refunding local row completes a refund; every other terminal
//     wallet status closes the purchase instead.
func Decide(walletStatus string, localStatus *string) Decision {
	if walletStatus == StatusConfirmed {
		if localStatus != nil && (*localStatus == StatusRefunding || *localStatus == StatusRefunded) {
			return Decision{Step: StepIgnore}
		}
		return Decision{Step: StepGrantConfirmed}
	}

	// A reconstructed row starts at the wallet's own status, so everything
	// below reads the same whether the row was loaded or rebuilt.
	local, rebuilt := "", false
	if localStatus == nil {
		if !recoverable(walletStatus) {
			return Decision{Step: StepDrop}
		}
		local, rebuilt = walletStatus, true
	} else {
		local = *localStatus
	}

	switch {
	case walletStatus == StatusRefunded && local == StatusRefunding:
		return Decision{Recover: rebuilt, Step: StepCompleteRefund}
	case Terminal(walletStatus):
		return Decision{Recover: rebuilt, Step: StepCloseTerminal}
	case local == StatusRefunding || local == StatusRefunded || local == StatusFailed ||
		local == StatusExpired || local == StatusConfirmed || local == walletStatus:
		return Decision{Recover: rebuilt, Step: StepIgnore}
	default:
		return Decision{Recover: rebuilt, Step: StepUpdateStatus}
	}
}
