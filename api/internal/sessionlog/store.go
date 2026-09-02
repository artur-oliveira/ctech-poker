package sessionlog

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

// sessionSK builds the sort key for a session item: a nanosecond timestamp
// makes cross-table collisions (two RecordSession calls landing in the same
// millisecond, e.g. multi-tabling) practically impossible — a millisecond-
// only key would let a second table's PutItem silently clobber the first
// table's still-open session item, orphaning it with no way to ever close it.
func sessionSK() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

const (
	tableSessions      = "poker_player_sessions"
	tableHands         = "poker_player_hands"
	tableHandsGsiTable = "gsi_table_id"

	// sessionTTLDays bounds how long a session item (open or closed) stays in
	// poker_player_sessions — this table only answers "which table is/was a
	// player at", it isn't the durable history (that's poker_player_hands, no
	// TTL). Set once at creation, not refreshed on close: a session outliving
	// this window before cashing out just drops its "currently seated" lookup
	// early, which is harmless since the wallet stays the source of truth for
	// balance (CloseSession's doc comment).
	sessionTTLDays = 30
)

type SessionItem struct {
	PK            string `dynamodbav:"pk" json:"pk"` // player_id
	SK            string `dynamodbav:"sk" json:"sk"` // timestamp / session_id
	TableID       string `dynamodbav:"table_id" json:"table_id"`
	BuyinAmount   int64  `dynamodbav:"buyin_amount" json:"buyin_amount"`
	CashoutAmount int64  `dynamodbav:"cashout_amount" json:"cashout_amount"`
	NetPnL        int64  `dynamodbav:"net_pnl" json:"net_pnl"`
	JoinedAt      int64  `dynamodbav:"joined_at" json:"joined_at"`
	// EndedAt is an epoch-milliseconds timestamp (time.Now().UnixMilli()),
	// never seconds — 0 means the session is still open (see CloseSession).
	// Every reader of this field (buyin.Service, the /players/me/sessions
	// response) must keep treating it as ms; see HandItem.EndedAt below and
	// #74 for the cross-endpoint unit contract this backs.
	EndedAt int64 `dynamodbav:"ended_at" json:"ended_at"`
	TTL     int64 `dynamodbav:"ttl,omitempty" json:"-"`
}

type HandItem struct {
	PK           string `dynamodbav:"pk" json:"pk"` // player_id
	SK           string `dynamodbav:"sk" json:"sk"` // currency_mode#hand_id
	CurrencyMode string `dynamodbav:"currency_mode" json:"currency_mode"`
	TableID      string `dynamodbav:"table_id" json:"table_id"`
	HandID       string `dynamodbav:"hand_id" json:"hand_id"`
	Outcome      string `dynamodbav:"outcome" json:"outcome"` // won | lost | tied
	NetChange    int64  `dynamodbav:"net_change" json:"net_change"`
	// EndedAt is an epoch-milliseconds timestamp (time.Now().UnixMilli(), set
	// once in app.go's onHandComplete pipeline). Every endpoint that emits a
	// hand — /players/me/hands, /players/me/hand/:id, and the public
	// showcase's best_hand — MUST pass this value through unchanged. Do not
	// divide/multiply by 1000 on any one endpoint; that per-endpoint drift is
	// exactly the bug #74 fixed (a `< 1e12` runtime heuristic had crept into
	// the frontend to cope with it).
	EndedAt   int64             `dynamodbav:"ended_at" json:"ended_at"`
	Board     []string          `dynamodbav:"board,omitempty" json:"board,omitempty"`
	BoardTwo  []string          `dynamodbav:"board_two,omitempty" json:"board_two,omitempty"`
	HoleCards []string          `dynamodbav:"hole_cards,omitempty" json:"hole_cards,omitempty"`
	Opponents []OpponentSummary `dynamodbav:"opponents,omitempty" json:"opponents,omitempty"`
	// ServerSeed and CommitHash are the hand's shuffle fairness proof
	// (hand.HandOutcome.ServerSeed/CommitHash), hex-encoded — lets the
	// player independently verify the deck they were dealt (B32).
	ServerSeed     string `dynamodbav:"server_seed,omitempty" json:"server_seed,omitempty"`
	CommitHash     string `dynamodbav:"commit_hash,omitempty" json:"commit_hash,omitempty"`
	RootCommitHash string `dynamodbav:"root_commit_hash,omitempty" json:"root_commit_hash,omitempty"`
	// RevealedCardSalts / UnrevealedCardHashes are this player's per-position
	// deck proof (hand.FairnessProof), keyed by deck position as a string.
	// They are what makes a hand verifiable when ServerSeed is withheld —
	// a hand that ended without a full showdown must never publish the seed
	// (it would expose mucked hole cards), but the card+salt reveals plus the
	// committed hashes still recompute RootCommitHash.
	RevealedCardSalts    map[string]RevealedSalt `dynamodbav:"revealed_card_salts,omitempty" json:"revealed_card_salts,omitempty"`
	UnrevealedCardHashes map[string]string       `dynamodbav:"unrevealed_card_hashes,omitempty" json:"unrevealed_card_hashes,omitempty"`
}

// RevealedSalt is one revealed deck position: the card and the salt its
// committed hash was built from, mirroring hand.RevealedSaltView on the wire.
type RevealedSalt struct {
	Card    string `dynamodbav:"card" json:"card"`
	SaltHex string `dynamodbav:"salt_hex" json:"salt_hex"`
}

// OpponentSummary is one other participant of a recorded hand, for a
// player's own match-history view. HoleCards is only populated when that
// opponent's hand was actually shown to the table (showdown or voluntary
// show) — never a folded hand, matching hand.PlayerHandInfo.Revealed.
type OpponentSummary struct {
	PlayerID  string   `dynamodbav:"player_id" json:"player_id"`
	Name      string   `dynamodbav:"name,omitempty" json:"name,omitempty"`
	AvatarURL string   `dynamodbav:"avatar_url,omitempty" json:"avatar_url,omitempty"`
	HoleCards []string `dynamodbav:"hole_cards,omitempty" json:"hole_cards,omitempty"`
	// Won is explicit so a 3+-way hand's history is readable without
	// inferring the winner from the viewer's own Outcome — that inference
	// only works heads-up (exactly one opponent).
	Won bool `dynamodbav:"won,omitempty" json:"won,omitempty"`
}

type Store struct {
	sessions dynamo.Base
	hands    dynamo.Base
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{
		sessions: dynamo.NewBase(db, env, tableSessions),
		hands:    dynamo.NewBase(db, env, tableHands),
	}
}

func (s *Store) RecordSession(ctx context.Context, item SessionItem) error {
	if item.SK == "" {
		item.SK = sessionSK()
	}
	if item.TTL == 0 {
		item.TTL = time.Now().Add(sessionTTLDays * 24 * time.Hour).Unix()
	}
	encoded, err := dynamo.Encode(item)
	if err != nil {
		return err
	}
	return s.sessions.PutItem(ctx, encoded)
}

func (s *Store) ListSessions(ctx context.Context, playerID string, limit int, startKey map[string]dynamotypes.AttributeValue) ([]SessionItem, map[string]dynamotypes.AttributeValue, error) {
	if limit <= 0 {
		limit = 50
	}
	res, err := s.sessions.Query(ctx, dynamo.QueryOpts{PK: playerID, Limit: limit, ScanIndexForward: false, ExclusiveStartKey: startKey})
	if err != nil {
		return nil, nil, err
	}
	out := make([]SessionItem, 0, len(res.Items))
	for _, raw := range res.Items {
		item, err := dynamo.Decode[SessionItem](raw)
		if err == nil && item != nil {
			out = append(out, *item)
		}
	}
	return out, res.LastEvaluatedKey, nil
}

// FindOpenSession returns the most recent session recorded for playerID at
// tableID that has not yet been closed (EndedAt == 0), or nil if none exists.
// Pages through the player's ENTIRE session partition (filtered server-side
// to tableID) rather than a single capped page — a player who has since
// opened sessions at other tables (multi-tabling, or just a lot of history)
// would otherwise push this table's still-open session past a fixed-size
// "most recent N" window, leaving it stuck open (ended_at never set) forever.
func (s *Store) FindOpenSession(ctx context.Context, playerID, tableID string) (*SessionItem, error) {
	var startKey map[string]dynamotypes.AttributeValue
	for {
		res, err := s.sessions.Query(ctx, dynamo.QueryOpts{
			PK: playerID, Limit: 50, ScanIndexForward: false,
			FilterField: "table_id", FilterValue: tableID,
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			return nil, err
		}
		for _, raw := range res.Items {
			item, err := dynamo.Decode[SessionItem](raw)
			if err == nil && item != nil && item.EndedAt == 0 {
				return item, nil
			}
		}
		if res.LastEvaluatedKey == nil {
			return nil, nil
		}
		startKey = res.LastEvaluatedKey
	}
}

// HasSessionAtTable reports whether playerID has ever had a session (open or
// closed, any time) at tableID — used to scope access to table-scoped views
// (e.g. the highlights feed) to players who were actually there, the same
// privacy boundary the rest of the match-history surface uses.
func (s *Store) HasSessionAtTable(ctx context.Context, playerID, tableID string) (bool, error) {
	var startKey map[string]dynamotypes.AttributeValue
	for {
		res, err := s.sessions.Query(ctx, dynamo.QueryOpts{
			PK: playerID, Limit: 50, ScanIndexForward: false,
			FilterField: "table_id", FilterValue: tableID,
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			return false, err
		}
		if len(res.Items) > 0 {
			return true, nil
		}
		if res.LastEvaluatedKey == nil {
			return false, nil
		}
		startKey = res.LastEvaluatedKey
	}
}

// FindLatestOpenSession returns the table id of the player's newest unclosed
// session, or "" when there is none. It reconciles friend-visible in_table
// presence after a process restart or a WebSocket reconnect; whether that id
// is ever published to anyone is decided by api/v1/social.go's gates, never
// here.
func (s *Store) FindLatestOpenSession(ctx context.Context, playerID string) (string, error) {
	var startKey map[string]dynamotypes.AttributeValue
	for {
		res, err := s.sessions.Query(ctx, dynamo.QueryOpts{PK: playerID, Limit: 50, ScanIndexForward: false, ExclusiveStartKey: startKey})
		if err != nil {
			return "", err
		}
		for _, raw := range res.Items {
			item, decodeErr := dynamo.Decode[SessionItem](raw)
			if decodeErr == nil && item != nil && item.EndedAt == 0 {
				return item.TableID, nil
			}
		}
		if len(res.LastEvaluatedKey) == 0 {
			return "", nil
		}
		startKey = res.LastEvaluatedKey
	}
}

// CloseSession overwrites the same session item (same PK/SK) with its final
// EndedAt/CashoutAmount/NetPnL — a plain PutItem, since this is an audit trail,
// not the wallet balance's source of truth (that stays ctech-wallet). TTL is
// reset to sessionTTLDays from now: item.TTL still carries the value set at
// RecordSession (open time), and RecordSession's "default only if zero" guard
// would otherwise leave a long-open session's TTL unchanged at close.
func (s *Store) CloseSession(ctx context.Context, item SessionItem) error {
	item.TTL = time.Now().Add(sessionTTLDays * 24 * time.Hour).Unix()
	return s.RecordSession(ctx, item)
}

func (s *Store) RecordHand(ctx context.Context, item HandItem) error {
	if item.SK == "" {
		item.SK = handSK(item.CurrencyMode, item.HandID)
	}
	encoded, err := dynamo.Encode(item)
	if err != nil {
		return err
	}
	return s.hands.PutItem(ctx, encoded)
}

func (s *Store) ListHands(ctx context.Context, playerID, mode string, limit int, startKey map[string]dynamotypes.AttributeValue) ([]HandItem, map[string]dynamotypes.AttributeValue, error) {
	if limit <= 0 {
		limit = 50
	}
	res, err := s.hands.Query(ctx, dynamo.QueryOpts{
		PK: playerID, SKPrefix: mode + "#", Limit: limit,
		ScanIndexForward: false, ExclusiveStartKey: startKey,
	})
	if err != nil {
		return nil, nil, err
	}
	outHands := make([]HandItem, 0, len(res.Items))
	for _, raw := range res.Items {
		item, err := dynamo.Decode[HandItem](raw)
		if err == nil && item != nil {
			outHands = append(outHands, *item)
		}
	}
	return outHands, res.LastEvaluatedKey, nil
}

// ListRecentHandsAcrossModes supplies the bounded lazy bootstrap for recent
// opponents. The history table has no chronological GSI, so it reads at most
// limit player-scoped rows and sorts that bounded set by EndedAt in memory.
func (s *Store) ListRecentHandsAcrossModes(ctx context.Context, playerID string, limit int) ([]HandItem, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	res, err := s.hands.Query(ctx, dynamo.QueryOpts{PK: playerID, Limit: limit, ScanIndexForward: false})
	if err != nil {
		return nil, err
	}
	items := make([]HandItem, 0, len(res.Items))
	for _, raw := range res.Items {
		item, decodeErr := dynamo.Decode[HandItem](raw)
		if decodeErr == nil && item != nil {
			items = append(items, *item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].EndedAt > items[j].EndedAt })
	return items, nil
}

func (s *Store) ListHandsByTable(ctx context.Context, playerID, mode, tableID string, limit int, startKey map[string]dynamotypes.AttributeValue) ([]HandItem, map[string]dynamotypes.AttributeValue, error) {
	res, err := s.hands.QueryComposite(ctx, dynamo.CompositeQueryOpts{
		PK:        playerID,
		IndexName: tableHandsGsiTable,
		SKEq: []dynamo.KV{
			{Field: "table_id", Value: tableID},
		},
		SKLastField: "sk", SKLastOp: "begins_with", SKLastValue: mode + "#",
		Limit:             limit,
		ScanIndexForward:  false,
		ExclusiveStartKey: startKey,
	})
	if err != nil {
		return nil, nil, err
	}
	outHands := make([]HandItem, 0, len(res.Items))
	for _, raw := range res.Items {
		item, err := dynamo.Decode[HandItem](raw)
		if err == nil && item != nil {
			outHands = append(outHands, *item)
		}
	}
	return outHands, res.LastEvaluatedKey, nil
}

func (s *Store) GetHand(ctx context.Context, playerID, mode, handID string) (*HandItem, error) {
	res, err := s.hands.GetItem(ctx, playerID, handSK(mode, handID))
	if err != nil {
		return nil, err
	} else if res == nil {
		return nil, nil
	}
	loaded, err := dynamo.Decode[HandItem](res)

	if err != nil {
		return nil, err
	}
	return loaded, nil
}

func handSK(mode, handID string) string {
	return mode + "#" + handID
}
