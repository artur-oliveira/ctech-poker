package reconcile

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSettlementViewNeverLeaksInternalFields pins #333's DTO boundary: even
// if a future field is added to PendingCashout, SettlementView's own field
// list (not a generic marshal of PendingCashout) is what reaches the player.
func TestSettlementViewNeverLeaksInternalFields(t *testing.T) {
	p := PendingCashout{
		ID: "p1", PlayerID: "player-1", Amount: 500, CurrencyMode: "real", Kind: KindFeeDebit,
		HoldIDs: []string{"hold-1"}, TableRef: "room-1", IdempotencyKey: "secret-idem-key",
		RecordedAt: "2026-09-01T00:00:00Z", LastError: "wallet: internal debit failure detail",
		Attempts: 2, LastAttemptAt: "2026-09-01T00:05:00Z",
	}
	raw, err := json.Marshal(p.SettlementView())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(raw)
	for _, forbidden := range []string{"hold_ids", "idempotency_key", "last_error", "attempts", "gsi_status", "secret-idem-key", "internal debit failure"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("settlement view leaked %q: %s", forbidden, body)
		}
	}
}

func TestSettlementViewStatusMapping(t *testing.T) {
	cases := []struct {
		name string
		p    PendingCashout
		want string
	}{
		{"pending", PendingCashout{}, SettlementStatusPending},
		{"resolved", PendingCashout{Resolved: true, ResolvedAt: "2026-09-01T00:00:00Z"}, SettlementStatusResolved},
		// manual_review: Attempts >= MaxAttempts flips gsi_status, never
		// surfaced as the raw LastError string a player can't act on.
		{"manual_review", PendingCashout{GSIStatus: pendingManualReview, Attempts: MaxAttempts, LastError: "raw wallet error"}, SettlementStatusManualReview},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := tc.p.SettlementView()
			if view.Status != tc.want {
				t.Fatalf("status=%q want=%q", view.Status, tc.want)
			}
			if strings.Contains(view.Status, "raw wallet error") {
				t.Fatalf("manual_review leaked raw error: %+v", view)
			}
		})
	}
}

func TestSettlementViewDefaultsEmptyKindToCashout(t *testing.T) {
	view := PendingCashout{Kind: ""}.SettlementView()
	if view.Kind != KindCashout {
		t.Fatalf("kind=%q want=%q", view.Kind, KindCashout)
	}
}
