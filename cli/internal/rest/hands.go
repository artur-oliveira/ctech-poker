package rest

import (
	"context"
	"net/http"
	"net/url"
)

// Hand is one player-scoped completed hand. EndedAt is epoch milliseconds.
type Hand struct {
	PK                   string                  `json:"pk"`
	SK                   string                  `json:"sk"`
	CurrencyMode         string                  `json:"currency_mode"`
	TableID              string                  `json:"table_id"`
	HandID               string                  `json:"hand_id"`
	Outcome              string                  `json:"outcome"`
	NetChange            int64                   `json:"net_change"`
	EndedAt              int64                   `json:"ended_at"`
	SmallBlind           int64                   `json:"small_blind,omitempty"`
	BigBlind             int64                   `json:"big_blind,omitempty"`
	Board                []string                `json:"board,omitempty"`
	BoardTwo             []string                `json:"board_two,omitempty"`
	HoleCards            []string                `json:"hole_cards,omitempty"`
	Opponents            []OpponentSummary       `json:"opponents,omitempty"`
	ServerSeed           string                  `json:"server_seed,omitempty"`
	CommitHash           string                  `json:"commit_hash,omitempty"`
	RootCommitHash       string                  `json:"root_commit_hash,omitempty"`
	RevealedCardSalts    map[string]RevealedSalt `json:"revealed_card_salts,omitempty"`
	UnrevealedCardHashes map[string]string       `json:"unrevealed_card_hashes,omitempty"`
}

type OpponentSummary struct {
	PlayerID  string   `json:"player_id"`
	Name      string   `json:"name,omitempty"`
	HoleCards []string `json:"hole_cards,omitempty"`
	Won       bool     `json:"won,omitempty"`
}

type RevealedSalt struct {
	Card    string `json:"card"`
	SaltHex string `json:"salt_hex"`
}

// Hands returns one newest-first sandbox history page (up to 50 hands).
func (c *Client) Hands(ctx context.Context, cursor string) (Page[Hand], error) {
	path := "/v1.0/players/me/hands?mode=sandbox"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	var page Page[Hand]
	err := c.Do(ctx, http.MethodGet, path, nil, &page)
	return page, err
}

// Hand returns one player-scoped completed sandbox hand.
func (c *Client) Hand(ctx context.Context, handID string) (Hand, error) {
	var hand Hand
	err := c.Do(ctx, http.MethodGet, "/v1.0/players/me/hand/"+url.PathEscape(handID)+"?mode=sandbox", nil, &hand)
	return hand, err
}

type HandHistory struct {
	TableID string              `json:"table_id"`
	HandID  string              `json:"hand_id"`
	Actions []HandHistoryAction `json:"actions"`
}

type HandHistoryAction struct {
	Seq            int          `json:"seq"`
	PlayerID       string       `json:"player_id"`
	Action         string       `json:"action"`
	Amount         int64        `json:"amount"`
	Timestamp      int64        `json:"timestamp"`
	ReactionID     string       `json:"reaction_id,omitempty"`
	TargetPlayerID string       `json:"target_player_id,omitempty"`
	Frame          *ReplayFrame `json:"frame,omitempty"`
}

type ReplayFrame struct {
	Stage              string           `json:"stage"`
	Board              []string         `json:"board,omitempty"`
	BoardTwo           []string         `json:"board_two,omitempty"`
	BoardSplitAt       int              `json:"board_split_at,omitempty"`
	Seats              []ReplaySeat     `json:"seats,omitempty"`
	CurrentPlayerID    string           `json:"current_player_id,omitempty"`
	DealerPlayerID     string           `json:"dealer_player_id,omitempty"`
	SmallBlindPlayerID string           `json:"small_blind_player_id,omitempty"`
	BigBlindPlayerID   string           `json:"big_blind_player_id,omitempty"`
	Pot                int64            `json:"pot"`
	Payouts            map[string]int64 `json:"payouts,omitempty"`
	Winners            []string         `json:"winners,omitempty"`
}

type ReplaySeat struct {
	PlayerID    string `json:"player_id"`
	Name        string `json:"name,omitempty"`
	Stack       int64  `json:"stack"`
	State       string `json:"state"`
	Contributed int64  `json:"contributed"`
	DealtIn     bool   `json:"dealt_in"`
}

// HandHistory returns the durable public action timeline for one hand.
func (c *Client) HandHistory(ctx context.Context, tableID, handID string) (HandHistory, error) {
	var history HandHistory
	path := "/v1.0/tables/" + url.PathEscape(tableID) + "/hands/" + url.PathEscape(handID) + "/history"
	err := c.Do(ctx, http.MethodGet, path, nil, &history)
	return history, err
}
