package userpurge

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
)

type fakeSeats struct {
	table string
	err   error
}

func (f fakeSeats) FindLatestOpenSession(context.Context, string) (string, error) {
	return f.table, f.err
}

// fakeSettlements serves one page per ListForPlayer call, chained by a fake startKey.
type fakeSettlements struct{ pages [][]reconcile.PendingCashout }

func (f fakeSettlements) ListForPlayer(_ context.Context, _ string, _ int, start map[string]types.AttributeValue) ([]reconcile.PendingCashout, map[string]types.AttributeValue, error) {
	if len(f.pages) == 0 {
		return nil, nil, nil
	}
	i := 0
	if start != nil {
		i, _ = strconv.Atoi(start["page"].(*types.AttributeValueMemberS).Value)
	}
	var next map[string]types.AttributeValue
	if i+1 < len(f.pages) {
		next = map[string]types.AttributeValue{"page": &types.AttributeValueMemberS{Value: strconv.Itoa(i + 1)}}
	}
	return f.pages[i], next, nil
}

func TestBlockers(t *testing.T) {
	ctx := context.Background()
	t.Run("unknown user has none", func(t *testing.T) {
		got, err := NewEligibility(fakeSeats{}, fakeSettlements{}).Blockers(ctx, "u")
		if err != nil || len(got) != 0 {
			t.Fatalf("want no blockers, got %+v (err=%v)", got, err)
		}
	})
	t.Run("seat and unresolved settlements across pages", func(t *testing.T) {
		settlements := fakeSettlements{pages: [][]reconcile.PendingCashout{
			{{HoldIDs: []string{"h1"}}, {Resolved: true, HoldIDs: []string{"h0"}}},
			{{Kind: reconcile.KindFeeDebit}, {Kind: ""}, {Kind: reconcile.KindCashout}},
		}}
		got, err := NewEligibility(fakeSeats{table: "t1"}, settlements).Blockers(ctx, "u")
		if err != nil {
			t.Fatal(err)
		}
		want := []struct {
			code   string
			detail string
		}{
			{BlockerSeatedAtTable, "t1"}, {BlockerChipsHeld, "1"}, {BlockerPendingCashout, "2"}, {BlockerPendingFeeDebit, "1"},
		}
		if len(got) != len(want) {
			t.Fatalf("want %d blockers, got %+v", len(want), got)
		}
		for i, w := range want {
			if got[i].Code != w.code {
				t.Fatalf("blocker %d: want %s, got %s", i, w.code, got[i].Code)
			}
		}
		if got[0].Detail["table_id"] != "t1" || got[2].Detail["count"] != 2 {
			t.Fatalf("details: %+v", got)
		}
	})
	t.Run("a lookup failure is an error, never eligible", func(t *testing.T) {
		if _, err := NewEligibility(fakeSeats{err: errors.New("down")}, fakeSettlements{}).Blockers(ctx, "u"); err == nil {
			t.Fatal("want error")
		}
	})
}
