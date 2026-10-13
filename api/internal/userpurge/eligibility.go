// Package userpurge is poker's side of the LGPD account-deletion saga
// (ctech-account docs/specs/2026-10-06-account-deletion-saga-protocol.md):
// blockers for the eligibility check and the purge that implements the data
// inventory §7. The protocol itself lives in api-commons/erasure.
package userpurge

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
)

// Blocker codes. Stable: ctech-account's UI translates them.
const (
	BlockerSeatedAtTable   = "poker.seated_at_table"
	BlockerChipsHeld       = "poker.chips_held"
	BlockerPendingCashout  = "poker.pending_cashout"
	BlockerPendingFeeDebit = "poker.pending_fee_debit"
)

type seatFinder interface {
	FindLatestOpenSession(ctx context.Context, playerID string) (string, error)
}

type settlementLister interface {
	ListForPlayer(ctx context.Context, playerID string, limit int, startKey map[string]types.AttributeValue) ([]reconcile.PendingCashout, map[string]types.AttributeValue, error)
}

// Eligibility computes what stops a user's erasure right now.
type Eligibility struct {
	seats   seatFinder
	pending settlementLister
}

func NewEligibility(seats seatFinder, pending settlementLister) *Eligibility {
	return &Eligibility{seats: seats, pending: pending}
}

// Blockers lists poker's blockers for sub: a seat (real-money seats hold
// chips in the wallet), and every unresolved settlement. There is no
// tournament concept in poker (plan ruling R13).
func (e *Eligibility) Blockers(ctx context.Context, sub string) ([]erasure.Blocker, error) {
	var out []erasure.Blocker
	tableID, err := e.seats.FindLatestOpenSession(ctx, sub)
	if err != nil {
		return nil, fmt.Errorf("userpurge: open session of %s: %w", sub, err)
	}
	if tableID != "" {
		out = append(out, erasure.Blocker{Code: BlockerSeatedAtTable, Detail: map[string]any{"table_id": tableID}})
	}
	counts := map[string]int{}
	var start map[string]types.AttributeValue
	for {
		rows, next, err := e.pending.ListForPlayer(ctx, sub, 100, start)
		if err != nil {
			return nil, fmt.Errorf("userpurge: settlements of %s: %w", sub, err)
		}
		for _, p := range rows {
			switch {
			case p.Resolved:
			case len(p.HoldIDs) > 0:
				counts[BlockerChipsHeld]++
			case p.Kind == reconcile.KindFeeDebit:
				counts[BlockerPendingFeeDebit]++
			default:
				counts[BlockerPendingCashout]++
			}
		}
		if len(next) == 0 {
			break
		}
		start = next
	}
	for _, code := range []string{BlockerChipsHeld, BlockerPendingCashout, BlockerPendingFeeDebit} {
		if n := counts[code]; n > 0 {
			out = append(out, erasure.Blocker{Code: code, Detail: map[string]any{"count": n}})
		}
	}
	return out, nil
}
