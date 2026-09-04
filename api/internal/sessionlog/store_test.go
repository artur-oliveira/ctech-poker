package sessionlog

import (
	"context"
	"testing"
)

type fakeSessionLogStore struct {
	sessions []SessionItem
	hands    []HandItem
}

func (f *fakeSessionLogStore) RecordSession(_ context.Context, item SessionItem) error {
	f.sessions = append(f.sessions, item)
	return nil
}

func (f *fakeSessionLogStore) ListSessions(_ context.Context, playerID string, _ int) ([]SessionItem, error) {
	out := []SessionItem{}
	for _, s := range f.sessions {
		if s.PK == playerID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeSessionLogStore) RecordHand(_ context.Context, item HandItem) error {
	f.hands = append(f.hands, item)
	return nil
}

func (f *fakeSessionLogStore) ListHands(_ context.Context, playerID string, _ int) ([]HandItem, error) {
	out := []HandItem{}
	for _, h := range f.hands {
		if h.PK == playerID {
			out = append(out, h)
		}
	}
	return out, nil
}

func TestFakeSessionLogStore(t *testing.T) {
	st := &fakeSessionLogStore{}
	ctx := context.Background()

	_ = st.RecordSession(ctx, SessionItem{PK: "usr-1", TableID: "tbl-1", NetPnL: 50})
	_ = st.RecordHand(ctx, HandItem{PK: "usr-1", HandID: "h1", NetChange: 20})

	sessions, err := st.ListSessions(ctx, "usr-1", 10)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d, err %v", len(sessions), err)
	}

	hands, err := st.ListHands(ctx, "usr-1", 10)
	if err != nil || len(hands) != 1 {
		t.Fatalf("expected 1 hand, got %d, err %v", len(hands), err)
	}
}

// TestOpenTableIDIsPresentOnlyWhileTheSessionIsOpen pins down the sparse key
// backing gsi_open_table (#224): an open session carries open_table_id (so it
// is in the index), a closed one carries none (so a full-item CloseSession
// put evicts it). Get this wrong in either direction and FindOpenSession
// either misses a live session or keeps reporting a cashed-out player as
// seated.
func TestOpenTableIDIsPresentOnlyWhileTheSessionIsOpen(t *testing.T) {
	if got := openTableID(SessionItem{TableID: "t1"}); got != "t1" {
		t.Fatalf("expected an open session to index its table, got %q", got)
	}
	if got := openTableID(SessionItem{TableID: "t1", EndedAt: 99}); got != "" {
		t.Fatalf("expected a closed session to leave the index, got %q", got)
	}
}

func TestHandSKSeparatesCurrencyModes(t *testing.T) {
	if sandbox, real := handSK("sandbox", "h1"), handSK("real", "h1"); sandbox == real {
		t.Fatalf("currency modes shared a hand key: %q", sandbox)
	} else if sandbox != "sandbox#h1" || real != "real#h1" {
		t.Fatalf("unexpected scoped keys: %q %q", sandbox, real)
	}
}
