package v1

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/chatfilter"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"

	fws "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

const (
	wsPingInterval = 30 * time.Second
	wsAuthTimeout  = 5 * time.Second
	wsPongWait     = wsPingInterval + 15*time.Second
	wsWriteWait    = 5 * time.Second
)

// clientMessage is every shape a connected player can send once authenticated.
type clientMessage struct {
	Type     string `json:"type"` // "ready" | "act" | "post_big_blind" | "chat" | "ping"
	Ready    bool   `json:"ready,omitempty"`
	Action   string `json:"action,omitempty"`
	Amount   int64  `json:"amount,omitempty"`
	ActionID string `json:"action_id,omitempty"`
	Message  string `json:"message,omitempty"`
}

var tableChatFilter = chatfilter.New([]string{"idiota", "burro"})

// readAuthToken reads the first WebSocket frame after the upgrade and
// extracts the bearer JWT plus an optional private-room share code. The
// client sends {"token":"...","share_code":"..."} (or a raw token) once; a
// missing or unreadable frame fails closed so no connection hangs open.
// Mirrors ctech-wallet's internal/api/v1/ws.go.
func readAuthToken(conn *fws.Conn) (token, shareCode string, ok bool) {
	_ = conn.SetReadDeadline(time.Now().Add(wsAuthTimeout))
	defer func(conn *fws.Conn, t time.Time) {
		err := conn.SetReadDeadline(t)
		if err != nil {

		}
	}(conn, time.Time{})
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return "", "", false
	}
	var p struct {
		Token     string `json:"token"`
		ShareCode string `json:"share_code"`
	}
	if json.Unmarshal(msg, &p) == nil && p.Token != "" {
		return p.Token, p.ShareCode, true
	}
	return strings.TrimSpace(string(msg)), "", true
}

// wsAllowedOrigin mirrors the HTTP CORS policy for the WebSocket upgrade:
// when no origins are configured (dev) every origin is allowed; otherwise
// only listed origins may connect. A missing Origin header (non-browser
// clients) is always allowed.
func wsAllowedOrigin(ctx *fasthttp.RequestCtx, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	origin := string(ctx.Request.Header.Peek("Origin"))
	if origin == "" {
		return true
	}
	for _, a := range allowed {
		if a == origin {
			return true
		}
	}
	return false
}

// seatLimiter is a fixed-window per-player counter — abuse prevention
// (ARCHITECTURE.md §8), not precise rate metering.
type seatLimiter struct {
	mu        sync.Mutex
	perWindow int
	window    time.Duration
	counts    map[string]int
	resetAt   map[string]time.Time
}

func newSeatLimiter(perSecond int) *seatLimiter {
	return &seatLimiter{perWindow: perSecond, window: time.Second, counts: make(map[string]int), resetAt: make(map[string]time.Time)}
}

func (l *seatLimiter) Allow(playerID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if now.After(l.resetAt[playerID]) {
		l.counts[playerID] = 0
		l.resetAt[playerID] = now.Add(l.window)
	}
	if l.counts[playerID] >= l.perWindow {
		return false
	}
	l.counts[playerID]++
	return true
}

// RegisterTableWS mounts GET /v1.0/tables/:id/ws. seed builds a brand-new
// hand.Table the first time a given table is ever acquired (see
// tablemanager.Manager.GetOrCreateActor) — Phase 3's room service supplies
// the real stakes/seats; until then any table ID seeds a placeholder so this
// gateway is independently testable without Phase 3's room service. Any
// instance may accept any table's connection directly — there is no
// "owner" to proxy to under ARCHITECTURE.md §2's revised model.
func RegisterTableWS(router fiber.Router, verifier *jwtverify.Verifier, manager *tablemanager.Manager, reg ws.Registry, allowedOrigins []string, seed func(tableID string) func() *hand.Table, rooms *roomstore.Store, cfg *config.Config, players *player.Service) {
	upgrader := fws.FastHTTPUpgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(ctx *fasthttp.RequestCtx) bool { return wsAllowedOrigin(ctx, allowedOrigins) },
	}
	router.Get("/tables/:id/ws", func(c fiber.Ctx) error {
		tableID := c.Params("id")
		return upgrader.Upgrade(c.RequestCtx(), func(conn *fws.Conn) {
			// Post-upgrade the handler runs on a hijacked goroutine outside
			// Fiber's recover middleware — an unrecovered panic here kills the
			// whole process, not just this connection.
			defer func() {
				if r := recover(); r != nil {
					slog.Error("table ws handler panic", "table", tableID, "panic", r)
					_ = conn.Close()
				}
			}()
			ctx := c.Context()
			// Single adapter shared by this handler and the fan-out registry:
			// its mutex is the only thing serializing data-frame writes, so
			// every write path must go through it (fasthttp/websocket panics
			// on concurrent writes).
			safeConn := &wsConnAdapter{conn: conn}
			send := func(msg any) {
				data, _ := json.Marshal(msg)
				_ = safeConn.WriteMessage(fws.TextMessage, data)
			}

			token, shareCode, ok := readAuthToken(conn)
			if !ok {
				send(map[string]any{"type": "error", "code": "unauthorized"})
				_ = conn.Close()
				return
			}
			// Empty sid = M2M client_credentials token — never a player (B9).
			claims, err := verifier.VerifyClaims(ctx, token)
			if err != nil || claims == nil || claims.Sub == "" || claims.SID == "" {
				send(map[string]any{"type": "error", "code": "unauthorized"})
				_ = conn.Close()
				return
			}
			playerID := claims.Sub

			// Private rooms are invite-only end to end: the WS gate mirrors
			// the HTTP join gate, so knowing a table ID never grants access.
			var room *roomstore.Room
			if rooms != nil {
				if room, err = rooms.Get(ctx, tableID); err != nil {
					send(map[string]any{"type": "error", "code": "unavailable"})
					_ = conn.Close()
					return
				}
			}
			if room != nil && !privateRoomAccessAllowed(room, playerID, shareCode) {
				send(map[string]any{"type": "error", "code": "forbidden"})
				_ = conn.Close()
				return
			}
			if room != nil && room.CurrencyMode != "sandbox" && !cfg.RealMoneyEnabled {
				send(map[string]any{"type": "error", "code": "unsupported_currency_mode"})
				_ = conn.Close()
				return
			}

			actor, err := manager.GetOrCreateActor(ctx, tableID, seed(tableID))
			if err != nil {
				send(map[string]any{"type": "error", "code": "unavailable"})
				_ = conn.Close()
				return
			}
			if room != nil {
				actor.SetEquityEnabledForActor(room.EquityDisplayEnabled)
			}

			// dispatch sends a command to the table actor, re-resolving a live
			// actor if the current one has stopped (it lost its lease). Guards
			// against the dead-actor Dispatch hang (T1).
			dispatch := func(cmd table.Command) error {
				for i := 0; i < 2; i++ {
					err := actor.Dispatch(cmd)
					if !errors.Is(err, table.ErrActorStopped) {
						return err
					}
					fresh, rerr := manager.GetOrCreateActor(ctx, tableID, seed(tableID))
					if rerr != nil {
						return rerr
					}
					actor = fresh
				}
				return actor.Dispatch(cmd)
			}

			// Push the persisted display name into this table's cache — the old
			// flow had the client resend "set_name" every connect; the name is
			// now server-authoritative (GET/POST /players/me), so the server
			// looks it up itself instead of trusting a client message.
			if players != nil {
				if profile, perr := players.GetOrCreate(ctx, playerID); perr == nil && profile != nil && profile.Name != "" {
					r := make(chan error, 1)
					_ = dispatch(table.SetNameCmd{PlayerID: playerID, Name: profile.Name, Reply: r})
				}
			}

			connKey := tableID + "#" + playerID
			connID := uuid.NewString()
			reg.Register(connKey, connID, safeConn)
			defer reg.Unregister(connKey, connID)
			chatConnID := connID + "-chat"
			reg.Register(tableID+"#chat", chatConnID, safeConn)
			defer reg.Unregister(tableID+"#chat", chatConnID)

			send(map[string]any{"type": "connected", "conn_id": connID})
			slog.Info("table ws connected", "table", tableID, "player", playerID, "conn", connID)

			limiter := newSeatLimiter(10) // 10 actions/sec/seat — generous for a human, tight for a script
			done := make(chan struct{})
			go startHeartbeat(conn, done, wsPingInterval, wsPongWait)

			for {
				_, msg, e := conn.ReadMessage()
				if e != nil {
					reply := make(chan error, 1)
					_ = dispatch(table.DisconnectCmd{PlayerID: playerID, Reply: reply})
					break
				}
				reply := make(chan error, 1)
				_ = dispatch(table.ReconnectCmd{PlayerID: playerID, Reply: reply})

				var m clientMessage
				if json.Unmarshal(msg, &m) != nil {
					continue
				}
				if (m.Type == "act" || m.Type == "chat") && !limiter.Allow(playerID) {
					send(map[string]any{"type": "error", "code": "rate_limited"})
					continue
				}
				switch m.Type {
				case "ping":
					send(map[string]any{"type": "pong"})
				case "ready":
					r := make(chan error, 1)
					_ = dispatch(table.ReadyCmd{PlayerID: playerID, Ready: m.Ready, Reply: r})
				case "act":
					r := make(chan error, 1)
					if err := dispatch(table.ActCmd{PlayerID: playerID, ActionID: m.ActionID, Action: betting.Action(m.Action), Amount: m.Amount, Reply: r}); err != nil {
						send(map[string]any{"type": "error", "code": "invalid_action", "message": err.Error()})
					}
				case "post_big_blind":
					r := make(chan error, 1)
					if err := dispatch(table.PostBigBlindCmd{PlayerID: playerID, Reply: r}); err != nil {
						send(map[string]any{"type": "error", "code": "invalid_post", "message": err.Error()})
					}
				case "chat":
					message := strings.TrimSpace(m.Message)
					if message == "" {
						continue
					}
					if len(message) > 500 {
						send(map[string]any{"type": "error", "code": "message_too_long"})
						continue
					}
					data, _ := json.Marshal(map[string]any{"type": "chat", "player_id": playerID, "message": tableChatFilter.Clean(message)})
					reg.Broadcast(ctx, tableID+"#chat", data)
				}
			}
			close(done)
			slog.Info("table ws disconnected", "table", tableID, "player", playerID, "conn", connID)
		})
	})
}

func startHeartbeat(conn *fws.Conn, done <-chan struct{}, pingInterval, pongWait time.Duration) {
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(pongWait)) })
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if e := conn.WriteControl(fws.PingMessage, nil, time.Now().Add(wsWriteWait)); e != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// wsConnAdapter adapts fasthttp/websocket.Conn to ws.Conn, serializing
// writes: the registry broadcasts from actor goroutines while the read
// loop replies inline, and fasthttp/websocket allows only one concurrent
// data-frame writer per conn.
type wsConnAdapter struct {
	mu   sync.Mutex
	conn *fws.Conn
}

func (w *wsConnAdapter) WriteMessage(messageType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(messageType, data)
}
