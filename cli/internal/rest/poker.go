package rest

import (
	"context"
	"net/http"
)

// Stake is one entry in the /rooms/stakes catalog.
type Stake struct {
	SmallBlind int64  `json:"small_blind"`
	BigBlind   int64  `json:"big_blind"`
	Tier       string `json:"tier,omitempty"`
	FeeCents   int64  `json:"fee_cents,omitempty"`
}

type stakesResponse struct {
	CurrencyMode string  `json:"currency_mode"`
	Unit         string  `json:"unit"`
	Stakes       []Stake `json:"stakes"`
}

// Stakes lists the stake catalog for a currency mode ("sandbox" or "real").
func (c *Client) Stakes(ctx context.Context, currencyMode string) ([]Stake, error) {
	var resp stakesResponse
	if err := c.Do(ctx, http.MethodGet, "/v1.0/rooms/stakes?currency_mode="+currencyMode, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Stakes, nil
}

// JoinOrCreateReq is a bucket spec (stakes + seats), not a room id — the
// server picks or creates the table (POST /v1.0/rooms/join-or-create).
type JoinOrCreateReq struct {
	CurrencyMode   string `json:"currency_mode,omitempty"`
	SmallBlind     int64  `json:"small_blind"`
	BigBlind       int64  `json:"big_blind"`
	MaxSeats       int    `json:"max_seats"`
	Amount         int64  `json:"amount"`
	AutoRebuy      bool   `json:"auto_rebuy,omitempty"`
	IdempotencyKey string `json:"idem_key,omitempty"`
}

type JoinOrCreateResp struct {
	RoomID  string `json:"room_id"`
	Created bool   `json:"created"`
}

func (c *Client) JoinOrCreate(ctx context.Context, req JoinOrCreateReq) (JoinOrCreateResp, error) {
	var resp JoinOrCreateResp
	err := c.Do(ctx, http.MethodPost, "/v1.0/rooms/join-or-create", req, &resp)
	return resp, err
}

// Room is a room's public shape (roomstore.Room's JSON tags, mirrored). It
// has no display name — the server tracks rooms only by id and stakes.
type Room struct {
	ID           string `json:"room_id"`
	Visibility   string `json:"visibility"`
	CurrencyMode string `json:"currency_mode"`
	SmallBlind   int64  `json:"small_blind"`
	BigBlind     int64  `json:"big_blind"`
	MaxSeats     int    `json:"max_seats"`
	BuyInMin     int64  `json:"buy_in_min"`
	BuyInMax     int64  `json:"buy_in_max"`
	ShareCode    string `json:"share_code,omitempty"`
	Status       string `json:"status"`
	SeatsTaken   int    `json:"seats_taken"`
}

func (c *Client) Room(ctx context.Context, id string) (Room, error) {
	var room Room
	err := c.Do(ctx, http.MethodGet, "/v1.0/rooms/"+id, nil, &room)
	return room, err
}

// JoinReq joins an existing room by id (POST /v1.0/rooms/:id/join).
type JoinReq struct {
	Amount         int64  `json:"amount"`
	ShareCode      string `json:"share_code,omitempty"`
	AutoRebuy      bool   `json:"auto_rebuy,omitempty"`
	IdempotencyKey string `json:"idem_key,omitempty"`
}

func (c *Client) JoinRoom(ctx context.Context, id string, req JoinReq) error {
	return c.Do(ctx, http.MethodPost, "/v1.0/rooms/"+id+"/join", req, nil)
}

func (c *Client) LeaveRoom(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodPost, "/v1.0/rooms/"+id+"/leave", nil, nil)
}

// Profile is the caller's own poker profile (GET /v1.0/players/me — the
// server auto-creates it on first read, so this never 404s for an
// authenticated player).
type Profile struct {
	UserID         string `json:"user_id"`
	Name           string `json:"name"`
	FriendCode     string `json:"friend_code"`
	AvatarURL      string `json:"avatar_url"`
	WalletMode     string `json:"wallet_mode"`
	GameBalance    int64  `json:"game_balance"`
	SandboxBalance int64  `json:"sandbox_balance"`
}

func (c *Client) Me(ctx context.Context) (Profile, error) {
	var p Profile
	err := c.Do(ctx, http.MethodGet, "/v1.0/players/me", nil, &p)
	return p, err
}

// AchievementTier is one threshold/reward step of an achievement.
type AchievementTier struct {
	Stars     int `json:"stars"`
	Threshold int `json:"threshold"`
}

// Achievement is one catalog entry's complete state for the caller
// (achievements.AchievementState, mirrored).
type Achievement struct {
	Key        string            `json:"key"`
	Metric     string            `json:"metric"`
	Secret     bool              `json:"secret,omitempty"`
	Tiers      []AchievementTier `json:"tiers"`
	Progress   int               `json:"progress"`
	Stars      int               `json:"stars"`
	Unlocked   bool              `json:"unlocked"`
	Completed  bool              `json:"completed"`
	NextTarget *int              `json:"next_target"`
	MaxTarget  int               `json:"max_target"`
	UnlockedAt string            `json:"unlocked_at,omitempty"`
}

type AchievementSummary struct {
	Mode   string `json:"mode"`
	Totals struct {
		Revealed  int `json:"revealed"`
		Unlocked  int `json:"unlocked"`
		Completed int `json:"completed"`
		Stars     int `json:"stars"`
		MaxStars  int `json:"max_stars"`
	} `json:"totals"`
	Achievements []Achievement `json:"achievements"`
}

// Achievements fetches the caller's complete achievement state for
// currency mode "sandbox" (GET /v1.0/players/me/achievements/summary) —
// unpaginated, the catalog is bounded.
func (c *Client) Achievements(ctx context.Context) (AchievementSummary, error) {
	var s AchievementSummary
	err := c.Do(ctx, http.MethodGet, "/v1.0/players/me/achievements/summary?currency_mode=sandbox", nil, &s)
	return s, err
}

// Session is one entry from the player's session history
// (sessionlog.SessionItem, mirrored). EndedAt is 0 while the session is open.
type Session struct {
	TableID       string `json:"table_id"`
	BuyinAmount   int64  `json:"buyin_amount"`
	CashoutAmount int64  `json:"cashout_amount"`
	NetPnL        int64  `json:"net_pnl"`
	JoinedAt      int64  `json:"joined_at"`
	EndedAt       int64  `json:"ended_at"`
}

// CurrentSession returns the player's most recent session (the first page of
// GET /v1.0/players/me/sessions, newest first).
func (c *Client) CurrentSession(ctx context.Context) (Session, error) {
	var page Page[Session]
	if err := c.Do(ctx, http.MethodGet, "/v1.0/players/me/sessions", nil, &page); err != nil {
		return Session{}, err
	}
	if len(page.Data) == 0 {
		return Session{}, nil
	}
	return page.Data[0], nil
}

// Reaction is one entry in the reaction catalog (reactionpurchase.CatalogEntry,
// mirrored). ID is the catalog key sent as reaction_id on the table socket.
type Reaction struct {
	ID      string `json:"id"`
	Premium bool   `json:"premium"`
	Owned   bool   `json:"owned"`
}

func (c *Client) ReactionCatalog(ctx context.Context) ([]Reaction, error) {
	var page Page[Reaction]
	if err := c.Do(ctx, http.MethodGet, "/v1.0/wallet/reaction-purchase/catalog", nil, &page); err != nil {
		return nil, err
	}
	return page.Data, nil
}
