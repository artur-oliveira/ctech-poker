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
