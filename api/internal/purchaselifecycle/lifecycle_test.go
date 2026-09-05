package purchaselifecycle

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// TestDecideMatrix is the single transition matrix issue #211 asks for: every
// (wallet status, local status) pair the PIX flows can observe, in one table
// instead of two copies of the same if-chain in reactionpurchase and
// cosmeticpurchase.
func TestDecideMatrix(t *testing.T) {
	local := func(s string) *string { return &s }
	cases := []struct {
		wallet string
		local  *string
		want   Decision
	}{
		// No local row: only a status a row can be rebuilt from is acted on.
		{StatusConfirmed, nil, Decision{Step: StepGrantConfirmed}},
		{StatusPending, nil, Decision{Recover: true, Step: StepIgnore}},
		{StatusFailed, nil, Decision{Recover: true, Step: StepCloseTerminal}},
		{StatusExpired, nil, Decision{Recover: true, Step: StepCloseTerminal}},
		{StatusRefunded, nil, Decision{Recover: true, Step: StepCloseTerminal}},
		{StatusProcessing, nil, Decision{Step: StepDrop}},
		{"something_new", nil, Decision{Step: StepDrop}},

		// A refund in flight or done outranks a confirmed wallet read:
		// re-confirming would resurrect the ownership the refund revoked.
		{StatusConfirmed, local(StatusRefunding), Decision{Step: StepIgnore}},
		{StatusConfirmed, local(StatusRefunded), Decision{Step: StepIgnore}},
		{StatusConfirmed, local(StatusPending), Decision{Step: StepGrantConfirmed}},
		{StatusConfirmed, local(StatusConfirmed), Decision{Step: StepGrantConfirmed}},
		{StatusConfirmed, local(StatusFailed), Decision{Step: StepGrantConfirmed}},

		// Only a refunding row completes a refund.
		{StatusRefunded, local(StatusRefunding), Decision{Step: StepCompleteRefund}},
		{StatusRefunded, local(StatusConfirmed), Decision{Step: StepCloseTerminal}},
		{StatusRefunded, local(StatusRefunded), Decision{Step: StepCloseTerminal}},

		// Every other terminal wallet status closes the purchase, including
		// over a locally confirmed grant — wallet is authoritative there.
		{StatusFailed, local(StatusPending), Decision{Step: StepCloseTerminal}},
		{StatusExpired, local(StatusPending), Decision{Step: StepCloseTerminal}},
		{StatusFailed, local(StatusConfirmed), Decision{Step: StepCloseTerminal}},

		// Non-terminal wallet status never regresses a settled local row.
		{StatusPending, local(StatusConfirmed), Decision{Step: StepIgnore}},
		{StatusPending, local(StatusRefunding), Decision{Step: StepIgnore}},
		{StatusPending, local(StatusRefunded), Decision{Step: StepIgnore}},
		{StatusPending, local(StatusFailed), Decision{Step: StepIgnore}},
		{StatusPending, local(StatusExpired), Decision{Step: StepIgnore}},

		// Replay vs. real movement.
		{StatusPending, local(StatusPending), Decision{Step: StepIgnore}},
		{StatusPending, local(StatusProcessing), Decision{Step: StepUpdateStatus}},
		{"in_analysis", local(StatusPending), Decision{Step: StepUpdateStatus}},
	}
	for _, tc := range cases {
		name := fmt.Sprintf("wallet=%s/local=%s", tc.wallet, "none")
		if tc.local != nil {
			name = fmt.Sprintf("wallet=%s/local=%s", tc.wallet, *tc.local)
		}
		t.Run(name, func(t *testing.T) {
			if got := Decide(tc.wallet, tc.local); got != tc.want {
				t.Fatalf("Decide = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// A rebuilt row is only ever rebuilt from the wallet's own status, so it can
// never come back needing a status update on top.
func TestRecoverNeverPairsWithAStatusUpdate(t *testing.T) {
	for _, status := range []string{StatusPending, StatusFailed, StatusExpired, StatusRefunded, StatusConfirmed, StatusProcessing, "unknown"} {
		if d := Decide(status, nil); d.Recover && d.Step == StepUpdateStatus {
			t.Fatalf("Decide(%q, nil) = %+v: a rebuilt row already carries the wallet status", status, d)
		}
	}
}

func TestDefinitiveWalletRejection(t *testing.T) {
	cases := []struct {
		status int
		want   bool
	}{
		{http.StatusBadRequest, true},
		{http.StatusConflict, true},
		{http.StatusUnprocessableEntity, true},
		// Ambiguous: the call may already have moved money, so the caller must
		// keep its reservation and resume instead of rolling back.
		{http.StatusRequestTimeout, false},
		{http.StatusTooEarly, false},
		{http.StatusTooManyRequests, false},
		{http.StatusInternalServerError, false},
		{http.StatusBadGateway, false},
	}
	for _, tc := range cases {
		if got := DefinitiveWalletRejection(&walletclient.Error{Status: tc.status}); got != tc.want {
			t.Fatalf("DefinitiveWalletRejection(%d) = %v, want %v", tc.status, got, tc.want)
		}
	}
	if DefinitiveWalletRejection(errors.New("connection reset")) {
		t.Fatal("a transport error is never a definitive rejection")
	}
	if DefinitiveWalletRejection(nil) {
		t.Fatal("no error is not a rejection")
	}
}
