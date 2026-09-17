package tablestore

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func canceled(codes ...string) error {
	reasons := make([]types.CancellationReason, 0, len(codes))
	for _, code := range codes {
		reasons = append(reasons, types.CancellationReason{Code: aws.String(code)})
	}
	return fmt.Errorf("dynamodb: %w", &types.TransactionCanceledException{
		Message:             aws.String("Transaction cancelled, please refer cancellation reasons for specific reasons"),
		CancellationReasons: reasons,
	})
}

// ErrVersionConflict is a verdict — "the table already advanced, reload and
// reconcile against it" — and every handler acts on it as one. A commit
// rejected before its condition was ever evaluated is the opposite: nothing
// was written, so the caller must abort and retry. Conflating them is what
// made table.Actor treat an all-in runout step that never happened as a
// street a sibling had already dealt, freezing the hand
// (docs/specs/2026-09-17-frozen-table-runout-and-sitout-fold.md). No store is
// touched on these paths, so a zero-value Store is enough.
func TestResolveCommitErrSeparatesRejectionsFromVerdicts(t *testing.T) {
	cases := []struct {
		name    string
		txErr   error
		want    error
		notWant error
	}{
		{
			name:    "another transaction held one of the items",
			txErr:   canceled("TransactionConflict", "None"),
			want:    ErrUnavailable,
			notWant: ErrVersionConflict,
		},
		{
			name:    "throttled",
			txErr:   canceled("None", "ThrottlingError"),
			want:    ErrUnavailable,
			notWant: ErrVersionConflict,
		},
		{
			name:  "the version condition genuinely lost",
			txErr: canceled("ConditionalCheckFailed", "None"),
			want:  ErrVersionConflict,
		},
		{
			name:    "an outright failure is never a verdict",
			txErr:   errors.New("dial tcp: connection refused"),
			want:    ErrUnavailable,
			notWant: ErrVersionConflict,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Store{}
			// actionID empty: the duplicate-action guard lookup is the only
			// branch that needs a live store, and it is not on these paths.
			err := s.resolveCommitErr(context.Background(), "table", "hand", "", tc.txErr)
			if !errors.Is(err, tc.want) {
				t.Fatalf("resolveCommitErr = %v, want %v", err, tc.want)
			}
			if tc.notWant != nil && errors.Is(err, tc.notWant) {
				t.Fatalf("resolveCommitErr = %v, must not be %v", err, tc.notWant)
			}
		})
	}
}
