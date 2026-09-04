package sessionlog

import (
	"context"
	"fmt"
	"sort"
	"strconv"
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

	// sessionsGsiOpenTable is a SPARSE index over poker_player_sessions:
	// pk = player_id, sk = open_table_id, and only an unclosed session
	// carries open_table_id (RecordSession derives it from EndedAt), so the
	// index holds exactly the handful of sessions a player currently has
	// open. It replaces the FilterExpression scan FindOpenSession and
	// FindLatestOpenSession used to page over the player's whole 30-day
	// history (#224).
	sessionsGsiOpenTable = "gsi_open_table"
	// sessionsGsiTable indexes EVERY session (open or closed) by the table it
	// was played at — pk = player_id, sk = table_id, KEYS_ONLY — so
	// HasSessionAtTable is a single one-item key query instead of a paged
	// filter over the whole partition.
	sessionsGsiTable = "gsi_player_table"

	fieldOpenTableID = "open_table_id"
	fieldTableID     = "table_id"

	// sessionTTLDays bounds how long a session item (open or closed) stays in
	// poker_player_sessions — this table only answers "which table is/was a
	// player at", it isn't the durable history (that's poker_player_hands, no
	// TTL). Set once at creation, not refreshed on close: a session outliving
	// this window before cashing out just drops its "currently seated" lookup
	// early, which is harmless since the wallet stays the source of truth for
	// balance (CloseSession's doc comment).
	sessionTTLDays = 30

	// openSessionScanLimit caps the sparse open-session index reads. A player
	// is seated at a handful of tables at once (multi-tabling), so this is a
	// ceiling that is never reached in practice, not a pagination window: it
	// bounds a pathological partition (a bug leaving sessions stuck open)
	// instead of silently truncating normal traffic.
	openSessionScanLimit = 25
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
	// OpenTableID is the sparse key of sessionsGsiOpenTable: it mirrors
	// TableID while the session is open and is absent once it closes. It is
	// derived, never supplied by callers — RecordSession recomputes it from
	// EndedAt on every write, and CloseSession's full-item PutItem therefore
	// drops the attribute, evicting the row from the index. Not serialized to
	// clients: it is a storage detail, not part of any session payload.
	OpenTableID string `dynamodbav:"open_table_id,omitempty" json:"-"`
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
	EndedAt int64 `dynamodbav:"ended_at" json:"ended_at"`
	// SmallBlind and BigBlind are the blind level the hand was played at, not
	// the room's current one — blind escalation moves it between hands, so a
	// replayer that reads the room would render an old hand at the wrong
	// scale (#75). Zero on hands recorded before this field existed; readers
	// must treat 0 as "unknown" and hide the marker rather than assume a
	// default.
	SmallBlind int64             `dynamodbav:"small_blind,omitempty" json:"small_blind,omitempty"`
	BigBlind   int64             `dynamodbav:"big_blind,omitempty" json:"big_blind,omitempty"`
	Board      []string          `dynamodbav:"board,omitempty" json:"board,omitempty"`
	BoardTwo   []string          `dynamodbav:"board_two,omitempty" json:"board_two,omitempty"`
	HoleCards  []string          `dynamodbav:"hole_cards,omitempty" json:"hole_cards,omitempty"`
	Opponents  []OpponentSummary `dynamodbav:"opponents,omitempty" json:"opponents,omitempty"`
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
//
// AvatarURL is denormalized here at hand-complete and never refreshed in
// storage, so a stored row can go stale the moment that opponent's avatar
// changes or is cleared — API callers must not serve it as-is. As of #68,
// internal/api/v1/player.go's resolveOpponentProfiles re-resolves it from the
// opponent's live profile before any hand-history response leaves the
// handler, clearing it to "" for an opponent whose avatar was since removed
// or whose profile no longer exists, rather than handing back a 404ing URL.
// Name has the same staleness problem and is intentionally not addressed
// here — see Issue #64.
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
	item.OpenTableID = openTableID(item)
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

// openTableID returns the sparse sessionsGsiOpenTable sort key for item: the
// table id while the session is unclosed, empty once EndedAt is set so the
// (omitempty) attribute disappears and the row leaves the index.
func openTableID(item SessionItem) string {
	if item.EndedAt != 0 {
		return ""
	}
	return item.TableID
}

// newestSession decodes raw index/table rows and returns the one with the
// highest SK — sessionSK is a fixed-width nanosecond stamp, so lexicographic
// order is chronological order. Picking the max in memory (over a handful of
// rows) rather than trusting ScanIndexForward keeps this correct on a GSI
// whose index key is not unique, where DynamoDB only orders duplicates by the
// base-table key.
func newestSession(raws []map[string]dynamotypes.AttributeValue) *SessionItem {
	var newest *SessionItem
	for _, raw := range raws {
		item, err := dynamo.Decode[SessionItem](raw)
		if err != nil || item == nil {
			continue
		}
		if newest == nil || item.SK > newest.SK {
			newest = item
		}
	}
	return newest
}

// FindOpenSession returns the most recent session recorded for playerID at
// tableID that has not yet been closed (EndedAt == 0), or nil if none exists.
// One key-equality query against the sparse sessionsGsiOpenTable index: no
// FilterExpression, no pagination, and cost independent of how much session
// history the player has accumulated — a player who has since opened sessions
// at other tables (multi-tabling, or just a lot of history) can no longer
// push this table's still-open session out of reach, leaving it stuck open
// (ended_at never set) forever.
func (s *Store) FindOpenSession(ctx context.Context, playerID, tableID string) (*SessionItem, error) {
	res, err := s.sessions.QueryComposite(ctx, dynamo.CompositeQueryOpts{
		PK:        playerID,
		IndexName: sessionsGsiOpenTable,
		SKEq:      []dynamo.KV{{Field: fieldOpenTableID, Value: tableID}},
		Limit:     openSessionScanLimit,
	})
	if err != nil {
		return nil, err
	}
	return newestSession(res.Items), nil
}

// HasSessionAtTable reports whether playerID has ever had a session (open or
// closed, any time) at tableID — used to scope access to table-scoped views
// (e.g. the highlights feed) to players who were actually there, the same
// privacy boundary the rest of the match-history surface uses. A single
// one-item query on sessionsGsiTable (KEYS_ONLY): existence is all the caller
// needs, so nothing but keys is read.
func (s *Store) HasSessionAtTable(ctx context.Context, playerID, tableID string) (bool, error) {
	res, err := s.sessions.QueryComposite(ctx, dynamo.CompositeQueryOpts{
		PK:        playerID,
		IndexName: sessionsGsiTable,
		SKEq:      []dynamo.KV{{Field: fieldTableID, Value: tableID}},
		Limit:     1,
	})
	if err != nil {
		return false, err
	}
	return len(res.Items) > 0, nil
}

// FindLatestOpenSession returns the table id of the player's newest unclosed
// session, or "" when there is none. It reconciles friend-visible in_table
// presence after a process restart or a WebSocket reconnect; whether that id
// is ever published to anyone is decided by api/v1/social.go's gates, never
// here. Reads the player's partition of the sparse open-session index, which
// only ever holds the tables they are seated at right now.
func (s *Store) FindLatestOpenSession(ctx context.Context, playerID string) (string, error) {
	res, err := s.sessions.Query(ctx, dynamo.QueryOpts{
		PK: playerID, IndexName: sessionsGsiOpenTable, Limit: openSessionScanLimit,
	})
	if err != nil {
		return "", err
	}
	if newest := newestSession(res.Items); newest != nil {
		return newest.TableID, nil
	}
	return "", nil
}

// buyinGuardPK returns the sessions-table partition key for a rebuy's
// idempotency guard row, namespaced away from real player-id partitions:
// no player id can ever equal "buyinguard#"+anything, so this guard can
// never surface in a query scoped to a real playerID (ListSessions,
// FindOpenSession, FindLatestOpenSession, HasSessionAtTable all query by the
// literal PK=playerID) even though it lives in the very same
// poker_player_sessions table.
func buyinGuardPK(playerID string) string {
	return "buyinguard#" + playerID
}

// AddBuyin atomically adds amount to the buyin_amount of the still-open
// session (playerID, sessionSK) — sessionSK is the SK a caller already
// resolved via FindOpenSession — instead of a read-modify-write ("Correctness
// = DynamoDB conditional writes... never read-then-write against table
// state", api/CLAUDE.md conventions). A rebuy/re-entry must accumulate onto
// the session's cumulative buy-in, never replace it, since RealityCheck and
// SessionRecap both derive their responsible-gaming "session result" from
// this figure — an undercounted total understates money actually put at
// risk (issue #70).
//
// idemKey scopes a create-only guard row in the same table, under
// buyinGuardPK's namespaced partition key, so a retried call for the exact
// same buy-in (a client resubmit, or the auto-rebuy sweep double-firing for
// one hand) can never double-count the rebuy. The guard write and the
// session update commit in a single TransactWriteItems call, so they can
// never partially apply. The session update additionally requires
// ended_at == 0: a rebuy racing a concurrent cash-out on the same seat must
// never reopen (or silently land inside) an already-closed session's total.
// Both failure shapes collapse to a conditional-check failure and are
// treated as a safe no-op, the same convention pokerstats.Store.RecordHand
// and matchup.Store.RecordHand already use for their own guarded increments.
func (s *Store) AddBuyin(ctx context.Context, playerID, sessionSK string, amount int64, idemKey string) error {
	if amount == 0 {
		return nil
	}
	guard, err := dynamo.Encode(struct {
		PK  string `dynamodbav:"pk"`
		SK  string `dynamodbav:"sk"`
		TTL int64  `dynamodbav:"ttl"`
	}{
		PK:  buyinGuardPK(playerID),
		SK:  idemKey,
		TTL: time.Now().Add(sessionTTLDays * 24 * time.Hour).Unix(),
	})
	if err != nil {
		return fmt.Errorf("sessionlog: encode buyin guard: %w", err)
	}

	sk := sessionSK
	update := s.sessions.BuildRawUpdateTxItem(playerID, &sk,
		"ADD buyin_amount :amt",
		"attribute_exists(pk) AND ended_at = :zero",
		nil,
		map[string]dynamotypes.AttributeValue{
			":amt":  &dynamotypes.AttributeValueMemberN{Value: strconv.FormatInt(amount, 10)},
			":zero": &dynamotypes.AttributeValueMemberN{Value: "0"},
		},
	)

	items := []dynamotypes.TransactWriteItem{
		s.sessions.BuildPutTxItemIfAbsent(guard),
		update,
	}
	if err := s.sessions.TransactWrite(ctx, items); err != nil {
		if dynamo.IsConditionFailed(err) {
			return nil
		}
		return fmt.Errorf("sessionlog: add buyin: %w", err)
	}
	return nil
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

// PublicHandSummary is a recorded hand reduced to the attributes the public
// profile showcase may publish. It is a separate type from HandItem on
// purpose: a HandItem carries opponent identities and cards, the shuffle
// seed, and the per-position fairness maps, none of which an anonymous
// visitor may see — so the showcase never holds a value that *could* leak
// them, rather than relying on every future edit of the response builder to
// keep leaving them out.
type PublicHandSummary struct {
	HandID    string   `dynamodbav:"hand_id" json:"hand_id"`
	TableID   string   `dynamodbav:"table_id" json:"table_id"`
	NetChange int64    `dynamodbav:"net_change" json:"net_change"`
	EndedAt   int64    `dynamodbav:"ended_at" json:"ended_at"`
	Board     []string `dynamodbav:"board,omitempty" json:"board,omitempty"`
	HoleCards []string `dynamodbav:"hole_cards,omitempty" json:"hole_cards,omitempty"`
}

// publicHandProjection is the ProjectionExpression that keeps a showcase read
// to PublicHandSummary's attributes. Every name here contains an underscore
// or is otherwise not a DynamoDB reserved word, so no aliasing is needed.
const publicHandProjection = "hand_id,table_id,net_change,ended_at,board,hole_cards"

// ShowcaseHandScan bounds how many recent hands the public showcase reads to
// pick its best one. Together with the projection above it is the whole
// per-view ceiling: one Query, at most this many rows, each just six small
// attributes — instead of up to 50 full HandItems (opponents, fairness maps,
// seeds) fetched to compute a six-field summary (#225).
const ShowcaseHandScan = 50

// BestPublicHand returns the biggest win among the player's most recent
// ShowcaseHandScan hands in mode, or nil when none of them won chips. Only
// public attributes are read: private ones never leave DynamoDB, so they
// cannot reach the response by accident.
//
// Deliberately uncached — a showcase view is a single bounded Query, so it
// always reflects the player's newest hand rather than trading freshness for
// a cache that would have to be invalidated on every hand completion.
func (s *Store) BestPublicHand(ctx context.Context, playerID, mode string) (*PublicHandSummary, error) {
	res, err := s.hands.Query(ctx, dynamo.QueryOpts{
		PK: playerID, SKPrefix: mode + "#", Limit: ShowcaseHandScan,
		ScanIndexForward: false, ProjectionExpression: publicHandProjection,
	})
	if err != nil {
		return nil, err
	}
	var best *PublicHandSummary
	for _, raw := range res.Items {
		item, decodeErr := dynamo.Decode[PublicHandSummary](raw)
		if decodeErr != nil || item == nil || item.NetChange <= 0 {
			continue
		}
		if best == nil || item.NetChange > best.NetChange {
			best = item
		}
	}
	return best, nil
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
