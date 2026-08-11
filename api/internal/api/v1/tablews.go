package v1

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/botcheck"
	"gopkg.aoctech.app/poker/api/internal/chatfilter"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/metrics"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"

	fws "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"

	goproto "google.golang.org/protobuf/proto"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
)

const (
	wsPingInterval = 30 * time.Second
	wsAuthTimeout  = 5 * time.Second
	wsPongWait     = wsPingInterval + 15*time.Second
	wsWriteWait    = 5 * time.Second
	// fasthttp/websocket buffers a whole message before handing it over and
	// applies no limit of its own, so without this one authenticated client
	// can make the server allocate an arbitrarily large frame. Every client
	// message this protocol defines is a small protobuf (the largest, a chat
	// line, is capped at chatMessageMaxLength characters), so 32 KiB is
	// already generous.
	wsMaxMessageBytes = 32 * 1024
	// Short enough to read at a glance in the seat speech bubble the UI
	// renders it in (ui/src/components/table/Seat.tsx); mirrored client-side
	// as CHAT_MESSAGE_MAX_LENGTH (ui/src/lib/chat.ts).
	chatMessageMaxLength = 50
)

var tableChatFilter = chatfilter.New([]string{"idiota", "burro"})

var tableReactions = map[string]bool{
	"clap": true, "laugh": true, "wow": true,
	"angry": true, "cry": true, "nervous": true,
	"cold": true, "fire": true, "respect": true, "sleepy": true,
	"chip": true, "coffee": true, "clover": true,
	"horseshoe": true, "tear": true, "tomato": true,
	"poop": true, "rofl": true, "duck": true, "turtle": true,
}

var targetedTableReactions = map[string]bool{
	"chip": true, "coffee": true, "clover": true,
	"horseshoe": true, "tear": true, "tomato": true,
	"poop": true, "rofl": true, "duck": true, "turtle": true,
}

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
	// Try parsing as Protobuf ClientMessage first
	var protoMsg pokerproto.ClientMessage
	if err := goproto.Unmarshal(msg, &protoMsg); err == nil && (protoMsg.Token != "" || protoMsg.Type == "auth") {
		return protoMsg.Token, protoMsg.ShareCode, true
	}
	// Fallback to JSON
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
	return slices.Contains(allowed, origin)
}

func rateLimitedTableMessage(messageType string) bool {
	switch messageType {
	case "act", "chat", "reaction", "preselect_action", "bot_challenge", "sync_state", "ready", "post_big_blind", "show_cards", "keep_seat", "set_run_it_twice", "ping":
		return true
	default:
		return false
	}
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

// tableConnectionTracker is process-local transport truth, independent of a
// table Actor's lifetime. When an actor is replaced after lease loss, every
// still-open connection on this instance is replayed into the new actor
// before the interrupted command is retried.
type tableConnectionTracker struct {
	mu    sync.Mutex
	conns map[string]map[string]map[string]struct{} // table -> player -> connID
}

type trackedTableConnection struct {
	playerID string
	connID   string
}

func newTableConnectionTracker() *tableConnectionTracker {
	return &tableConnectionTracker{conns: make(map[string]map[string]map[string]struct{})}
}

func (t *tableConnectionTracker) add(tableID, playerID, connID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conns[tableID] == nil {
		t.conns[tableID] = make(map[string]map[string]struct{})
	}
	if t.conns[tableID][playerID] == nil {
		t.conns[tableID][playerID] = make(map[string]struct{})
	}
	t.conns[tableID][playerID][connID] = struct{}{}
}

func (t *tableConnectionTracker) remove(tableID, playerID, connID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	players := t.conns[tableID]
	if players == nil {
		return
	}
	delete(players[playerID], connID)
	if len(players[playerID]) == 0 {
		delete(players, playerID)
	}
	if len(players) == 0 {
		delete(t.conns, tableID)
	}
}

func (t *tableConnectionTracker) listTable(tableID string) []trackedTableConnection {
	t.mu.Lock()
	defer t.mu.Unlock()
	players := t.conns[tableID]
	out := make([]trackedTableConnection, 0)
	for playerID, conns := range players {
		for connID := range conns {
			out = append(out, trackedTableConnection{playerID: playerID, connID: connID})
		}
	}
	return out
}

func newSeatLimiter(perSecond int) *seatLimiter {
	return &seatLimiter{perWindow: perSecond, window: time.Second, counts: make(map[string]int), resetAt: make(map[string]time.Time)}
}

func newWindowSeatLimiter(perWindow int, window time.Duration) *seatLimiter {
	return &seatLimiter{perWindow: perWindow, window: window, counts: make(map[string]int), resetAt: make(map[string]time.Time)}
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
func RegisterTableWS(
	router fiber.Router,
	verifier *jwtverify.Verifier,
	manager *tablemanager.Manager,
	reg ws.Registry,
	allowedOrigins []string,
	seed func(tableID string) func() *hand.Table,
	rooms *roomstore.Store,
	cfg *config.Config,
	players *player.Service,
	stats pokerStatsReader,
) {
	connectionTracker := newTableConnectionTracker()
	checker := botcheck.New(cfg.TurnstileSecret, cfg.TurnstileExpectedHostname)
	upgrader := fws.FastHTTPUpgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(ctx *fasthttp.RequestCtx) bool { return wsAllowedOrigin(ctx, allowedOrigins) },
	}
	router.Get("/tables/:id/ws", func(c fiber.Ctx) error {
		// c.Params/c.IP hand back strings pointing straight into fasthttp's
		// per-connection request buffer, which is recycled the moment this
		// handler returns. The callback below runs on a hijacked goroutine for
		// the whole life of the socket, so it must own its copies: an uncopied
		// tableID silently mutates into a later request's bytes mid-hand
		// ("check7H8PWBPBTZTT09ATFEGYN", "/me/handsWBPBTZTT09ATFEGYN"), and
		// every command the actor then runs loads a table ID that has no item
		// in DynamoDB — surfacing to players as "no state seeded for this
		// table yet" until they reload the page.
		tableID := strings.Clone(c.Params("id"))
		remoteIP := strings.Clone(c.IP())
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
			// Same lifetime problem as tableID above: the request context dies
			// with the request, while this goroutine keeps making DynamoDB and
			// Redis calls for as long as the socket lives. Own the context.
			ctx, cancelCtx := context.WithCancel(context.Background())
			defer cancelCtx()
			// Single adapter shared by this handler and the fan-out registry:
			// its mutex is the only thing serializing data-frame writes, so
			// every write path must go through it (fasthttp/websocket panics
			// on concurrent writes).
			conn.SetReadLimit(wsMaxMessageBytes)
			safeConn := &wsConnAdapter{conn: conn}
			send := func(msg *pokerproto.ServerMessage) {
				data, err := goproto.Marshal(msg)
				if err == nil {
					_ = safeConn.WriteMessage(fws.BinaryMessage, data)
				}
			}

			token, shareCode, ok := readAuthToken(conn)
			if !ok {
				send(&pokerproto.ServerMessage{Type: "error", Code: "unauthorized"})
				_ = conn.Close()
				return
			}
			// Empty sid = M2M client_credentials token — never a player (B9).
			claims, err := verifier.VerifyClaims(ctx, token)
			if err != nil || claims == nil || claims.Sub == "" || claims.SID == "" {
				send(&pokerproto.ServerMessage{Type: "error", Code: "unauthorized"})
				_ = conn.Close()
				return
			}
			playerID := claims.Sub

			// Private rooms are invite-only end to end: the WS gate mirrors
			// the HTTP join gate, so knowing a table ID never grants access.
			var room *roomstore.Room
			if rooms != nil {
				if room, err = rooms.Get(ctx, tableID); err != nil {
					send(&pokerproto.ServerMessage{Type: "error", Code: "unavailable"})
					_ = conn.Close()
					return
				}
				if room == nil {
					send(&pokerproto.ServerMessage{Type: "error", Code: "not_found"})
					_ = conn.Close()
					return
				}
			}
			if room != nil && !privateRoomAccessAllowed(room, playerID, shareCode) {
				send(&pokerproto.ServerMessage{Type: "error", Code: "forbidden"})
				_ = conn.Close()
				return
			}
			if room != nil && room.CurrencyMode != "sandbox" && !cfg.RealMoneyEnabled {
				send(&pokerproto.ServerMessage{Type: "error", Code: "unsupported_currency_mode"})
				_ = conn.Close()
				return
			}

			actor, err := manager.GetOrCreateActor(ctx, tableID, seed(tableID))
			if err != nil {
				send(&pokerproto.ServerMessage{Type: "error", Code: "unavailable"})
				_ = conn.Close()
				return
			}
			if room != nil {
				actor.SetEquityEnabledForActor(room.EquityDisplayEnabled)
				actor.SetRunItTwiceEnabledForActor(room.RunItTwiceEnabled)
			}
			connID := uuid.NewString()
			connectionRegistered := false

			// dispatch sends a command to the table actor, re-resolving a live
			// actor if the current one has stopped (it lost its lease). Guards
			// against the dead-actor Dispatch hang (T1).
			dispatch := func(cmd table.Command) error {
				for range 2 {
					err := actor.Dispatch(cmd)
					if !errors.Is(err, table.ErrActorStopped) {
						return err
					}
					fresh, rerr := manager.GetOrCreateActor(ctx, tableID, seed(tableID))
					if rerr != nil {
						return rerr
					}
					actor = fresh
					if room != nil {
						actor.SetEquityEnabledForActor(room.EquityDisplayEnabled)
						actor.SetRunItTwiceEnabledForActor(room.RunItTwiceEnabled)
					}
					if connectionRegistered {
						for _, live := range connectionTracker.listTable(tableID) {
							connectReply := make(chan error, 1)
							if rerr := actor.Dispatch(table.ConnectCmd{
								PlayerID: live.playerID, ConnID: live.connID, Reply: connectReply,
							}); rerr != nil {
								return rerr
							}
						}
					}
				}
				return actor.Dispatch(cmd)
			}

			// Push the persisted display name into this table's cache — the old
			// flow had the client resend "set_name" every connect; the name is
			// now server-authoritative (GET/POST /players/me), so the server
			// looks it up itself instead of trusting a client message.
			if players != nil {
				if profile, perr := players.GetOrCreate(ctx, playerID); perr == nil && profile != nil {
					playstyleBadge := ""
					if profile.PlaystylePublic && stats != nil {
						if playerStats, statsErr := stats.Get(ctx, playerID, roomstore.CurrencyModeSandbox); statsErr == nil {
							if badges := pokerstats.StyleFor(playerStats, pokerstats.MinHandsPublic); len(badges) > 0 {
								playstyleBadge = badges[0].Key
							}
						}
					}
					r := make(chan error, 1)
					_ = dispatch(table.SetIdentityCmd{PlayerID: playerID, Name: profile.Name,
						AvatarURL: player.AvatarURL(profile, cfg.AvatarBaseURL), PlaystyleBadge: playstyleBadge, Reply: r})
				}
			}

			connKey := tableID + "#" + playerID
			reg.Register(connKey, connID, safeConn)
			defer reg.Unregister(connKey, connID)
			connectionTracker.add(tableID, playerID, connID)
			defer connectionTracker.remove(tableID, playerID, connID)
			chatConnID := connID + "-chat"
			reg.Register(tableID+"#chat", chatConnID, safeConn)
			defer reg.Unregister(tableID+"#chat", chatConnID)

			connectReply := make(chan error, 1)
			if err := dispatch(table.ConnectCmd{PlayerID: playerID, ConnID: connID, Reply: connectReply}); err != nil {
				send(&pokerproto.ServerMessage{Type: "error", Code: "unavailable"})
				_ = conn.Close()
				return
			}
			connectionRegistered = true
			defer func() {
				disconnectReply := make(chan error, 1)
				_ = dispatch(table.DisconnectCmd{PlayerID: playerID, ConnID: connID, Reply: disconnectReply})
				connectionRegistered = false
			}()

			send(&pokerproto.ServerMessage{Type: "connected", ConnId: connID})
			slog.Info("table ws connected", "table", tableID, "player", playerID, "conn", connID)

			// Push the current table state to this connection immediately. The
			// actor only broadcasts on a state mutation, so a fresh socket would
			// otherwise sit on ping/pong until the next action (leaving the
			// client stuck on the loading screen). Sent directly to this conn,
			// not via the fan-out registry, so it reaches the viewer even when
			// they are not yet seated (spectator / pre-join).
			sendSnapshot := func(actionID string) {
				startedAt := time.Now()
				snapCh := make(chan hand.Snapshot, 1)
				snapReply := make(chan error, 1)
				if err := dispatch(table.SnapshotCmd{PlayerID: playerID, Snapshot: snapCh, Reply: snapReply}); err == nil {
					if snap, ok := <-snapCh; ok {
						send(&pokerproto.ServerMessage{
							Type: "state", Snapshot: ConvertSnapshot(snap), ActionId: actionID,
						})
						metrics.EmitTableMetric(cfg.Env, "SnapshotLatencyMs", float64(time.Since(startedAt).Milliseconds()), nil)
					}
				}
			}
			sendSnapshot("")

			limiter := newSeatLimiter(10) // 10 actions/sec/seat — generous for a human, tight for a script
			reactionLimiter := newWindowSeatLimiter(1, 2*time.Second)
			botRiskScore := 0
			challengeRequired := false
			done := make(chan struct{})
			go startHeartbeat(safeConn, done, wsPingInterval, wsPongWait)

			for {
				_, msg, e := conn.ReadMessage()
				if e != nil {
					break
				}
				reply := make(chan error, 1)
				_ = dispatch(table.ReconnectCmd{PlayerID: playerID, Reply: reply})

				var m pokerproto.ClientMessage
				if err := goproto.Unmarshal(msg, &m); err != nil {
					continue
				}
				if rateLimitedTableMessage(m.Type) && !limiter.Allow(playerID) {
					send(&pokerproto.ServerMessage{Type: "error", Code: "rate_limited", ActionId: m.ActionId})
					continue
				}
				requireActionID := func() bool {
					if strings.TrimSpace(m.ActionId) != "" {
						return true
					}
					send(&pokerproto.ServerMessage{
						Type: "error", Code: "missing_action_id", Message: "action_id is required",
					})
					return false
				}
				requireActionPrecondition := func() bool {
					if m.ExpectedSnapshotVersion > 0 && strings.TrimSpace(m.ExpectedHandId) != "" {
						return true
					}
					send(&pokerproto.ServerMessage{
						Type: "error", Code: "missing_precondition",
						Message:  "expected_snapshot_version and expected_hand_id are required",
						ActionId: m.ActionId,
					})
					return false
				}
				ensureActionID := func() {
					if strings.TrimSpace(m.ActionId) == "" {
						// Ready/show/post were historically sent without an ID.
						// Generate one during the rolling-deploy window; new
						// clients always provide their own correlation ID.
						m.ActionId = uuid.NewString()
					}
				}
				ack := func() {
					send(&pokerproto.ServerMessage{Type: "action_ack", ActionId: m.ActionId})
				}
				switch m.Type {
				case "ping":
					send(&pokerproto.ServerMessage{Type: "pong"})
				case "sync_state":
					sendSnapshot(m.ActionId)
				case "ready":
					ensureActionID()
					r := make(chan error, 1)
					if err := dispatch(table.ReadyCmd{PlayerID: playerID, ActionID: m.ActionId, Ready: m.Ready, Reply: r}); err != nil {
						send(&pokerproto.ServerMessage{Type: "error", Code: "invalid_action", Message: err.Error(), ActionId: m.ActionId})
					} else {
						ack()
					}
				case "act":
					if !requireActionID() || !requireActionPrecondition() {
						continue
					}
					if challengeRequired {
						send(&pokerproto.ServerMessage{Type: "bot_challenge", Code: "bot_challenge_required"})
						send(&pokerproto.ServerMessage{Type: "error", Code: "bot_challenge_required", ActionId: m.ActionId})
						continue
					}
					decisionLatency := time.Duration(-1)
					if checker.Enabled() {
						snapCh := make(chan hand.Snapshot, 1)
						snapReply := make(chan error, 1)
						if err := dispatch(table.SnapshotCmd{PlayerID: playerID, Snapshot: snapCh, Reply: snapReply}); err == nil {
							snap := <-snapCh
							if snap.CurrentPlayerID == playerID && snap.ActionBaseDeadlineUnixMs > 0 {
								timeout := table.DefaultTurnTimeout
								if room != nil {
									timeout = table.TurnTimeoutFor(room.TurnTimeoutSeconds)
								}
								turnStarted := time.UnixMilli(snap.ActionBaseDeadlineUnixMs).Add(-timeout)
								decisionLatency = time.Since(turnStarted)
							}
						}
					}
					r := make(chan error, 1)
					if err := dispatch(table.ActCmd{
						PlayerID: playerID, ActionID: m.ActionId,
						ExpectedSnapshotVersion: m.ExpectedSnapshotVersion,
						ExpectedHandID:          m.ExpectedHandId,
						Action:                  betting.Action(m.Action), Amount: m.Amount, Reply: r,
					}); err != nil {
						code := "invalid_action"
						if strings.Contains(err.Error(), "stale action state") {
							code = "stale_state"
						}
						send(&pokerproto.ServerMessage{Type: "error", Code: code, Message: err.Error(), ActionId: m.ActionId})
					} else {
						ack()
						if checker.Enabled() {
							switch {
							// Check/Fold preselection is an intentional
							// accessibility and speed feature. It can fire in
							// the same frame as a state update, so treating it
							// as bot evidence would punish normal use. Only
							// chip-committing decisions contribute positive
							// risk; slower/check/fold decisions decay it.
							case (m.Action == string(betting.ActionCall) || m.Action == string(betting.ActionRaise)) &&
								decisionLatency >= 0 && decisionLatency < 150*time.Millisecond:
								botRiskScore += 2
							case (m.Action == string(betting.ActionCall) || m.Action == string(betting.ActionRaise)) &&
								decisionLatency >= 0 && decisionLatency < 350*time.Millisecond:
								botRiskScore++
							case botRiskScore > 0:
								botRiskScore--
							}
							if botRiskScore >= 16 {
								challengeRequired = true
								send(&pokerproto.ServerMessage{Type: "bot_challenge"})
							}
						}
					}
				case "bot_challenge":
					if !checker.Enabled() || !challengeRequired {
						send(&pokerproto.ServerMessage{Type: "error", Code: "bot_challenge_not_required", ActionId: m.ActionId})
						continue
					}
					verifyCtx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
					err := checker.Verify(verifyCtx, m.TurnstileToken, remoteIP)
					cancel()
					if err != nil {
						send(&pokerproto.ServerMessage{Type: "error", Code: "bot_challenge_failed", ActionId: m.ActionId})
						continue
					}
					challengeRequired = false
					botRiskScore = 0
					send(&pokerproto.ServerMessage{Type: "bot_challenge_passed", ActionId: m.ActionId})
				case "post_big_blind":
					ensureActionID()
					r := make(chan error, 1)
					if err := dispatch(table.PostBigBlindCmd{PlayerID: playerID, ActionID: m.ActionId, Reply: r}); err != nil {
						send(&pokerproto.ServerMessage{Type: "error", Code: "invalid_post", Message: err.Error(), ActionId: m.ActionId})
					} else {
						ack()
					}
				case "show_cards":
					ensureActionID()
					r := make(chan error, 1)
					if err := dispatch(table.ShowCardsCmd{
						PlayerID: playerID, ActionID: m.ActionId, CardIndex: m.CardIndex, Reply: r,
					}); err != nil {
						send(&pokerproto.ServerMessage{Type: "error", Code: "invalid_action", Message: err.Error(), ActionId: m.ActionId})
					} else {
						ack()
					}
				case "set_run_it_twice":
					r := make(chan error, 1)
					enabled := m.RunItTwice != nil && *m.RunItTwice
					if err := dispatch(table.SetRunItTwiceCmd{PlayerID: playerID, Enabled: enabled, Reply: r}); err != nil {
						send(&pokerproto.ServerMessage{Type: "error", Code: "invalid_action", Message: err.Error()})
					}
				case "keep_seat":
					ensureActionID()
					r := make(chan error, 1)
					if err := dispatch(table.KeepSeatCmd{PlayerID: playerID, ActionID: m.ActionId, Reply: r}); err != nil {
						send(&pokerproto.ServerMessage{Type: "error", Code: "invalid_action", Message: err.Error(), ActionId: m.ActionId})
					} else {
						ack()
					}
				case "chat":
					message := strings.TrimSpace(m.Message)
					if message == "" {
						continue
					}
					if len(message) > chatMessageMaxLength {
						send(&pokerproto.ServerMessage{Type: "error", Code: "message_too_long"})
						continue
					}
					ensureActionID()
					message = tableChatFilter.Clean(message)
					r := make(chan error, 1)
					if err := dispatch(table.ChatCmd{PlayerID: playerID, ActionID: m.ActionId, Message: message, Reply: r}); err != nil {
						send(&pokerproto.ServerMessage{Type: "error", Code: "invalid_action", Message: err.Error(), ActionId: m.ActionId})
					} else {
						// Compatibility event for clients predating protocol v6. The
						// authoritative copy is also present in every state snapshot.
						data, _ := goproto.Marshal(&pokerproto.ServerMessage{
							Type: "chat", PlayerId: playerID, Message: message, ActionId: m.ActionId,
						})
						reg.Broadcast(ctx, tableID+"#chat", data)
						ack()
					}
				case "reaction":
					ensureActionID()
					if !tableReactions[m.ReactionId] {
						send(&pokerproto.ServerMessage{Type: "error", Code: "invalid_reaction", ActionId: m.ActionId})
						continue
					}
					if !reactionLimiter.Allow(playerID) {
						send(&pokerproto.ServerMessage{Type: "error", Code: "reaction_rate_limited", ActionId: m.ActionId})
						continue
					}
					snapCh := make(chan hand.Snapshot, 1)
					snapReply := make(chan error, 1)
					if err := dispatch(table.SnapshotCmd{PlayerID: playerID, Snapshot: snapCh, Reply: snapReply}); err != nil {
						send(&pokerproto.ServerMessage{Type: "error", Code: "unavailable", ActionId: m.ActionId})
						continue
					}
					snap := <-snapCh
					senderSeated, targetSeated := false, !targetedTableReactions[m.ReactionId]
					for _, seat := range snap.Seats {
						if seat.PlayerID == playerID {
							senderSeated = true
						}
						if seat.PlayerID == m.TargetPlayerId && seat.PlayerID != playerID {
							targetSeated = true
						}
					}
					if !senderSeated || !targetSeated {
						send(&pokerproto.ServerMessage{Type: "error", Code: "invalid_reaction_target", ActionId: m.ActionId})
						continue
					}
					r := make(chan error, 1)
					if err := dispatch(table.ReactionCmd{
						PlayerID: playerID, ActionID: m.ActionId, ReactionID: m.ReactionId,
						TargetPlayerID: m.TargetPlayerId, Reply: r,
					}); err != nil {
						send(&pokerproto.ServerMessage{Type: "error", Code: "invalid_action", Message: err.Error(), ActionId: m.ActionId})
					} else {
						data, _ := goproto.Marshal(&pokerproto.ServerMessage{
							Type: "reaction", PlayerId: playerID, ReactionId: m.ReactionId,
							TargetPlayerId: m.TargetPlayerId, ActionId: m.ActionId,
						})
						reg.Broadcast(ctx, tableID+"#chat", data)
						ack()
					}
				case "preselect_action":
					ensureActionID()
					r := make(chan error, 1)
					if err := dispatch(table.PreselectCmd{
						PlayerID: playerID, ActionID: m.ActionId, Selection: m.Action, Amount: m.Amount,
						ExpectedSnapshotVersion: m.ExpectedSnapshotVersion,
						ExpectedHandID:          m.ExpectedHandId,
						ExpectedStage:           m.ExpectedStage, Reply: r,
					}); err != nil {
						code := "invalid_action"
						if strings.Contains(err.Error(), "stale action state") {
							code = "stale_state"
						}
						send(&pokerproto.ServerMessage{Type: "error", Code: code, Message: err.Error(), ActionId: m.ActionId})
					} else {
						ack()
					}
				}
			}
			close(done)
			slog.Info("table ws disconnected", "table", tableID, "player", playerID, "conn", connID)
		})
	})
}

func startHeartbeat(adapter *wsConnAdapter, done <-chan struct{}, pingInterval, pongWait time.Duration) {
	adapter.conn.SetPongHandler(func(string) error { return adapter.conn.SetReadDeadline(time.Now().Add(pongWait)) })
	_ = adapter.conn.SetReadDeadline(time.Now().Add(pongWait))
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if e := adapter.WriteControl(fws.PingMessage, nil, time.Now().Add(wsWriteWait)); e != nil {
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

func (w *wsConnAdapter) WriteControl(messageType int, data []byte, deadline time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteControl(messageType, data, deadline)
}

func (w *wsConnAdapter) WriteMessage(_ int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	// Without a deadline, a peer that stops reading (full TCP receive window)
	// blocks this write indefinitely. ws.Registry.Broadcast already unregisters
	// a connection on any write error and moves on to the next one (memory.go),
	// so bounding the write here is all that's needed to stop one stalled
	// viewer from stalling every other broadcast this connection is part of —
	// including, for table conns, the single actor Run goroutine that calls
	// broadcastAll synchronously for every seated player.
	if err := w.conn.SetWriteDeadline(time.Now().Add(wsWriteWait)); err != nil {
		return err
	}
	return w.conn.WriteMessage(fws.BinaryMessage, data)
}

// RegisterGeneralWS mounts GET /v1.0/ws. It upgrades the request to a WebSocket connection
// that registers the client for global ("lobby") and user-specific ("user#<player_id>") messages.
func RegisterGeneralWS(
	router fiber.Router,
	verifier *jwtverify.Verifier,
	reg ws.Registry,
	allowedOrigins []string,
) {
	upgrader := fws.FastHTTPUpgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(ctx *fasthttp.RequestCtx) bool { return wsAllowedOrigin(ctx, allowedOrigins) },
	}
	router.Get("/ws", func(c fiber.Ctx) error {
		return upgrader.Upgrade(c.RequestCtx(), func(conn *fws.Conn) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("general ws handler panic", "panic", r)
					_ = conn.Close()
				}
			}()
			// See RegisterTableWS: the request context is dead once the
			// upgrade returns, this goroutine is not.
			ctx, cancelCtx := context.WithCancel(context.Background())
			defer cancelCtx()
			conn.SetReadLimit(wsMaxMessageBytes)
			safeConn := &wsConnAdapter{conn: conn}
			send := func(msg *pokerproto.ServerMessage) {
				data, err := goproto.Marshal(msg)
				if err == nil {
					_ = safeConn.WriteMessage(fws.BinaryMessage, data)
				}
			}

			token, _, ok := readAuthToken(conn)
			if !ok {
				send(&pokerproto.ServerMessage{Type: "error", Code: "unauthorized"})
				_ = conn.Close()
				return
			}
			claims, err := verifier.VerifyClaims(ctx, token)
			if err != nil || claims == nil || claims.Sub == "" || claims.SID == "" {
				send(&pokerproto.ServerMessage{Type: "error", Code: "unauthorized"})
				_ = conn.Close()
				return
			}
			playerID := claims.Sub
			connID := uuid.NewString()

			// Register for user-specific broadcasts
			userChan := "user#" + playerID
			reg.Register(userChan, connID, safeConn)
			defer reg.Unregister(userChan, connID)

			// Register for global/lobby broadcasts
			lobbyChan := "lobby"
			reg.Register(lobbyChan, connID, safeConn)
			defer reg.Unregister(lobbyChan, connID)

			send(&pokerproto.ServerMessage{Type: "connected", ConnId: connID})
			slog.Info("general ws connected", "player", playerID, "conn", connID)

			done := make(chan struct{})
			go startHeartbeat(safeConn, done, wsPingInterval, wsPongWait)

			for {
				_, msg, e := conn.ReadMessage()
				if e != nil {
					break
				}
				var m pokerproto.ClientMessage
				if err := goproto.Unmarshal(msg, &m); err != nil {
					continue
				}
				if m.Type == "ping" {
					send(&pokerproto.ServerMessage{Type: "pong"})
				}
			}
			close(done)
			slog.Info("general ws disconnected", "player", playerID, "conn", connID)
		})
	})
}

// ConvertSnapshot converts the engine's hand.Snapshot structure to the Protobuf TableSnapshot type.
func ConvertSnapshot(snap hand.Snapshot) *pokerproto.TableSnapshot {
	protoSeats := make([]*pokerproto.Seat, len(snap.Seats))
	for i, s := range snap.Seats {
		var equity *float64
		if s.Equity != nil {
			equity = new(*s.Equity)
		}
		dealtIn, ready := s.DealtIn, s.Ready
		var avatarURL *string
		if s.AvatarURL != "" {
			avatarURL = &s.AvatarURL
		}
		var playstyleBadge *string
		if s.PlaystyleBadge != "" {
			playstyleBadge = &s.PlaystyleBadge
		}
		var runItTwice *bool
		if s.RunItTwice {
			enabled := true
			runItTwice = &enabled
		}
		var autoRebuy *bool
		if s.AutoRebuy {
			enabled := true
			autoRebuy = &enabled
		}
		protoSeats[i] = &pokerproto.Seat{
			PlayerId:          s.PlayerID,
			Name:              s.Name,
			AvatarUrl:         avatarURL,
			PlaystyleBadge:    playstyleBadge,
			RunItTwice:        runItTwice,
			AutoRebuy:         autoRebuy,
			ConnectionState:   s.ConnectionState,
			Stack:             s.Stack,
			State:             s.State,
			DealtIn:           &dealtIn,
			Ready:             &ready,
			Contributed:       s.Contributed,
			HoleCards:         s.HoleCards,
			HoleCardsRevealed: s.HoleCardsRevealed,
			StackAtHandStart:  s.StackAtHandStart,
			TimeBankMs:        s.TimeBankMs,
			Equity:            equity,
			HandCategory:      s.HandCategory,
			HandScore:         s.HandScore,
		}
	}

	var protoLA *pokerproto.LegalActions
	if snap.LegalActions != nil {
		protoLA = &pokerproto.LegalActions{
			Actions:             snap.LegalActions.Actions,
			CallAmount:          snap.LegalActions.CallAmount,
			MinRaiseTo:          snap.LegalActions.MinRaiseTo,
			MaxRaiseTo:          snap.LegalActions.MaxRaiseTo,
			Step:                snap.LegalActions.Step,
			CurrentContribution: snap.LegalActions.CurrentContribution,
			CurrentBet:          snap.LegalActions.CurrentBet,
			OneThirdPotRaiseTo:  snap.LegalActions.OneThirdPotRaiseTo,
			HalfPotRaiseTo:      snap.LegalActions.HalfPotRaiseTo,
			TwoThirdsPotRaiseTo: snap.LegalActions.TwoThirdsPotRaiseTo,
			PotRaiseTo:          snap.LegalActions.PotRaiseTo,
		}
	}
	protoPots := make([]*pokerproto.Pot, len(snap.Pots))
	for i, pot := range snap.Pots {
		protoPots[i] = &pokerproto.Pot{
			Amount:            pot.Amount,
			EligiblePlayerIds: pot.EligiblePlayerIDs,
		}
	}
	protoPotResults := make([]*pokerproto.PotResult, len(snap.PotResults))
	for i, result := range snap.PotResults {
		protoPotResults[i] = &pokerproto.PotResult{
			Amount:            result.Amount,
			PayoutAmount:      result.PayoutAmount,
			EligiblePlayerIds: result.EligiblePlayerIDs,
			WinnerPlayerIds:   result.WinnerPlayerIDs,
			Payouts:           result.Payouts,
			Refund:            result.Refund,
			Runout:            int32(result.Runout),
		}
	}
	protoChat := make([]*pokerproto.ChatMessage, len(snap.ChatMessages))
	for i, message := range snap.ChatMessages {
		protoChat[i] = &pokerproto.ChatMessage{
			Id: message.ID, PlayerId: message.PlayerID, Message: message.Message, Timestamp: message.Timestamp,
		}
	}
	protoReactions := make([]*pokerproto.TableReaction, len(snap.Reactions))
	for i, reaction := range snap.Reactions {
		protoReactions[i] = &pokerproto.TableReaction{
			Id: reaction.ID, PlayerId: reaction.PlayerID, ReactionId: reaction.ReactionID,
			TargetPlayerId: reaction.TargetPlayerID, Timestamp: reaction.Timestamp, ExpiresAt: reaction.ExpiresAt,
		}
	}

	protoRevealedSalts := make(map[int32]*pokerproto.RevealedSalt, len(snap.RevealedCardSalts))
	for idx, saltView := range snap.RevealedCardSalts {
		protoRevealedSalts[int32(idx)] = &pokerproto.RevealedSalt{
			Card:    saltView.Card,
			SaltHex: saltView.SaltHex,
		}
	}
	protoUnrevealedHashes := make(map[int32]string, len(snap.UnrevealedCardHashes))
	for idx, hashHex := range snap.UnrevealedCardHashes {
		protoUnrevealedHashes[int32(idx)] = hashHex
	}

	return &pokerproto.TableSnapshot{
		Stage:                    snap.Stage,
		Board:                    snap.Board,
		Seats:                    protoSeats,
		Payouts:                  snap.Payouts,
		Winners:                  snap.Winners,
		Rake:                     snap.Rake,
		CurrentPlayerId:          snap.CurrentPlayerID,
		LegalActions:             protoLA,
		ActionDeadlineUnixMs:     snap.ActionDeadlineUnixMs,
		ActionBaseDeadlineUnixMs: snap.ActionBaseDeadlineUnixMs,
		NextHandUnixMs:           snap.NextHandUnixMs,
		IdleRemovalUnixMs:        snap.IdleRemovalUnixMs,
		WonWithoutShowdown:       snap.WonWithoutShowdown,
		ShuffleCommitHash:        snap.ShuffleCommitHash,
		ShuffleServerSeedHex:     snap.ShuffleServerSeedHex,
		SmallBlindPlayerId:       snap.SmallBlindPlayerID,
		BigBlindPlayerId:         snap.BigBlindPlayerID,
		DealerPlayerId:           snap.DealerPlayerID,
		SnapshotVersion:          snap.SnapshotVersion,
		Pots:                     protoPots,
		HandId:                   snap.HandID,
		PotResults:               protoPotResults,
		ProtocolVersion:          10,
		ChatMessages:             protoChat,
		Reactions:                protoReactions,
		ActionPreselection:       snap.ActionPreselection,
		ActionPreselectionAmount: snap.ActionPreselectionAmount,
		ProspectiveCallAmount:    snap.ProspectiveCallAmount,
		RootCommitHash:           snap.RootCommitHash,
		RevealedCardSalts:        protoRevealedSalts,
		UnrevealedCardHashes:     protoUnrevealedHashes,
		RunoutCards:              snap.RunoutCards,
		BoardTwo:                 snap.BoardTwo,
		BoardSplitAt:             int32(snap.BoardSplitAt),
	}
}

// ConvertRoom converts the roomstore.Room structure to the Protobuf Room type.
func ConvertRoom(r roomstore.Room) *pokerproto.Room {
	var protoEscalation *pokerproto.BlindEscalation
	if r.BlindEscalation != nil {
		protoEscalation = &pokerproto.BlindEscalation{
			IntervalMinutes: int32(r.BlindEscalation.IntervalMinutes),
			Multiplier:      int32(r.BlindEscalation.Multiplier),
			Max:             r.BlindEscalation.Max,
		}
	}
	return &pokerproto.Room{
		RoomId:               r.ID,
		Visibility:           r.Visibility,
		CurrencyMode:         r.CurrencyMode,
		SmallBlind:           r.SmallBlind,
		BigBlind:             r.BigBlind,
		MaxSeats:             int32(r.MaxSeats),
		BuyInMin:             r.BuyInMin,
		BuyInMax:             r.BuyInMax,
		EntryFeeCents:        r.EntryFeeCents,
		ShareCode:            r.ShareCode,
		BlindEscalation:      protoEscalation,
		TurnTimeoutSeconds:   int32(r.TurnTimeoutSeconds),
		EquityDisplayEnabled: r.EquityDisplayEnabled,
		Status:               r.Status,
		SeatsTaken:           int32(r.SeatsTaken),
		CreatedBy:            r.CreatedBy,
		CreatedAt:            r.CreatedAt,
		RunItTwiceEnabled:    r.RunItTwiceEnabled,
	}
}
