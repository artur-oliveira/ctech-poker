package sessionlog

import (
	"context"
	"time"

	"gopkg.aoctech.app/api-commons/dynamo"
)

// RecapHandScan bounds how many of a session's hands one recap reads. A long
// live session can outrun it; the recap then reports the most recent
// RecapHandScan hands and says so through Truncated, rather than paging the
// whole table to build a summary card (same ceiling-not-pagination reasoning
// as ShowcaseHandScan).
const RecapHandScan = 200

// Recap is the end-of-session summary: how long the player sat, what it cost
// or paid, and the two hands worth remembering (issue #310).
//
// It is computed on read from rows that already exist — the SessionItem and
// that table's HandItems — and is never persisted: nothing in CloseSession's
// write path changes, so a session that ends while the process dies still
// recaps correctly the next time it is asked for, and #204's per-hand budget
// is untouched.
type Recap struct {
	SessionID string `json:"session_id"`
	TableID   string `json:"table_id"`
	JoinedAt  int64  `json:"joined_at"`
	// EndedAt is 0 while the session is still open, exactly as on
	// SessionItem; DurationMs is then measured to now instead.
	EndedAt       int64 `json:"ended_at"`
	DurationMs    int64 `json:"duration_ms"`
	BuyinAmount   int64 `json:"buyin_amount"`
	CashoutAmount int64 `json:"cashout_amount"`
	NetPnL        int64 `json:"net_pnl"`
	HandsPlayed   int   `json:"hands_played"`
	HandsWon      int   `json:"hands_won"`
	// BiggestWin / BiggestLoss are the session's key moments. Both are the
	// caller's OWN rows, so the public projection is exactly the right shape:
	// hole cards here are the viewer's own, never an opponent's.
	BiggestWin  *PublicHandSummary `json:"biggest_win,omitempty"`
	BiggestLoss *PublicHandSummary `json:"biggest_loss,omitempty"`
	// Truncated reports that the session had more than RecapHandScan hands,
	// so the counters cover only the most recent ones.
	Truncated bool `json:"truncated"`
}

// SessionRecap builds the recap for one of playerID's own sessions. mode
// selects the hand partition (hands are keyed currency_mode#hand_id), and
// sessionID is the SessionItem's sort key as returned by ListSessions.
// Returns nil when the player has no such session — never another player's:
// both reads are keyed on playerID.
//
// Cost is one GetItem plus one bounded, projected Query, the same shape
// BestPublicHand already established. The hands are filtered to the session's
// own window in memory because the table GSI is keyed by table, not by time,
// and a player can have several sessions at one table.
func (s *Store) SessionRecap(ctx context.Context, playerID, mode, sessionID string) (*Recap, error) {
	raw, err := s.sessions.GetItem(ctx, playerID, sessionID)
	if err != nil || raw == nil {
		return nil, err
	}
	session, err := dynamo.Decode[SessionItem](raw)
	if err != nil || session == nil {
		return nil, err
	}

	res, err := s.hands.QueryComposite(ctx, dynamo.CompositeQueryOpts{
		PK:        playerID,
		IndexName: tableHandsGsiTable,
		SKEq: []dynamo.KV{
			{Field: fieldTableID, Value: session.TableID},
		},
		SKLastField: "sk", SKLastOp: "begins_with", SKLastValue: mode + "#",
		Limit:            RecapHandScan,
		ScanIndexForward: false,
	})
	if err != nil {
		return nil, err
	}
	hands := make([]HandItem, 0, len(res.Items))
	for _, item := range res.Items {
		handItem, decodeErr := dynamo.Decode[HandItem](item)
		if decodeErr != nil || handItem == nil {
			continue
		}
		hands = append(hands, *handItem)
	}
	return aggregateRecap(*session, hands, len(res.Items) >= RecapHandScan), nil
}

// aggregateRecap is the recap's whole logic, split out from the two reads so
// it can be tested without DynamoDB: which of the table's hands belong to
// this sitting, and which two of them are worth showing.
func aggregateRecap(session SessionItem, hands []HandItem, truncated bool) *Recap {
	recap := &Recap{
		SessionID: session.SK, TableID: session.TableID,
		JoinedAt: session.JoinedAt, EndedAt: session.EndedAt,
		BuyinAmount: session.BuyinAmount, CashoutAmount: session.CashoutAmount,
		NetPnL: session.NetPnL, Truncated: truncated,
	}
	endedAt := session.EndedAt
	if endedAt == 0 {
		endedAt = time.Now().UnixMilli()
	}
	if endedAt > session.JoinedAt {
		recap.DurationMs = endedAt - session.JoinedAt
	}
	for _, handItem := range hands {
		// A player can sit at the same table more than once, and the GSI is
		// keyed by table alone — so the session's own window is what
		// separates this sitting's hands from an earlier one's.
		if handItem.EndedAt < session.JoinedAt || (session.EndedAt != 0 && handItem.EndedAt > session.EndedAt) {
			continue
		}
		recap.HandsPlayed++
		if handItem.NetChange > 0 {
			recap.HandsWon++
		}
		summary := PublicHandSummary{
			HandID: handItem.HandID, TableID: handItem.TableID,
			NetChange: handItem.NetChange, EndedAt: handItem.EndedAt,
			Board: handItem.Board, HoleCards: handItem.HoleCards,
		}
		if handItem.NetChange > 0 && (recap.BiggestWin == nil || handItem.NetChange > recap.BiggestWin.NetChange) {
			best := summary
			recap.BiggestWin = &best
		}
		if handItem.NetChange < 0 && (recap.BiggestLoss == nil || handItem.NetChange < recap.BiggestLoss.NetChange) {
			worst := summary
			recap.BiggestLoss = &worst
		}
	}
	return recap
}
