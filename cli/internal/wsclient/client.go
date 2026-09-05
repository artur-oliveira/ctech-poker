// Package wsclient is the table WebSocket client: connects, sends the auth
// frame, and exchanges binary protobuf ClientMessage/ServerMessage envelopes
// with the poker API's table gateway.
package wsclient

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/coder/websocket"
	googleproto "google.golang.org/protobuf/proto"
	"gopkg.aoctech.app/poker/cli/internal/proto"
)

// maxMessageBytes mirrors the server's own frame cap (api/internal/api/v1/tablews.go wsMaxMessageBytes).
const maxMessageBytes = 32 * 1024

const pingInterval = 25 * time.Second

// Synthetic ServerMessage types the TUI can key on. Never sent by the server
// — Run() emits them locally to signal a reconnect in progress / recovered.
const (
	TypeReconnecting = "__reconnecting"
	TypeReconnected  = "__reconnected"
)

type sendReq struct {
	msg  *proto.ClientMessage
	done chan error
}

// Client is one table WebSocket connection, with Run providing
// reconnect-with-resync on top of it. Connect and Run must never be called
// concurrently with each other (Connect first, then `go cl.Run(ctx)`) — both
// touch currentDone with no lock, which is safe only because of that
// ordering.
type Client struct {
	url    string
	token  func(context.Context) (string, error)
	origin string

	messages chan *proto.ServerMessage
	sendCh   chan sendReq

	mu     sync.Mutex
	conn   *websocket.Conn
	connID string
	err    error

	shareCode   string
	currentDone chan struct{}
	closeOnce   sync.Once
}

// New builds a Client for wsURL (a ws:// or wss:// table-socket URL). origin
// is sent as the Origin header — the API enforces an origin allow-list on
// the WS upgrade (api/CLAUDE.md issue #44).
func New(wsURL string, token func(context.Context) (string, error), origin string) *Client {
	return &Client{
		url:      wsURL,
		token:    token,
		origin:   origin,
		messages: make(chan *proto.ServerMessage, 32),
		sendCh:   make(chan sendReq),
	}
}

// Connect dials the socket, sends the auth frame (with shareCode for private
// rooms), and waits for the server's "connected" reply. On success it starts
// the read and write loops for this connection generation; Messages() then
// streams everything after "connected".
func (c *Client) Connect(ctx context.Context, shareCode string) error {
	c.shareCode = shareCode
	done, err := c.dialAndHandshake(ctx, shareCode)
	if err != nil {
		return err
	}
	c.currentDone = done
	return nil
}

// dialAndHandshake performs one full connect: dial, auth frame, wait for
// "connected", then start this generation's read/write loops. Returns a
// channel that closes when this generation's connection ends.
func (c *Client) dialAndHandshake(ctx context.Context, shareCode string) (chan struct{}, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}

	conn, res, err := websocket.Dial(ctx, c.url, &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Origin": {c.origin}},
	})
	if res != nil && res.Body != nil {
		defer func() { _ = res.Body.Close() }()
	}
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(maxMessageBytes)

	authMsg := &proto.ClientMessage{Type: "auth", Token: tok, ShareCode: shareCode}
	if err := writeFrameOn(ctx, conn, authMsg); err != nil {
		_ = conn.CloseNow() // best effort — the write error above is what the caller needs, never mask it
		return nil, err
	}

	first, err := readFrameOn(ctx, conn)
	if err != nil {
		_ = conn.CloseNow()
		return nil, err
	}
	if first.Type != "connected" {
		_ = conn.CloseNow()
		return nil, fmt.Errorf("wsclient: expected a connected frame, got %q", first.Type)
	}

	c.mu.Lock()
	c.conn = conn
	c.connID = first.ConnId
	c.mu.Unlock()

	done := make(chan struct{})
	go c.readLoop(conn, done)
	go c.writeLoop(conn, done)
	go c.pingLoop(conn, done)
	return done, nil
}

func writeFrameOn(ctx context.Context, conn *websocket.Conn, m *proto.ClientMessage) error {
	b, err := googleproto.Marshal(m)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageBinary, b)
}

func readFrameOn(ctx context.Context, conn *websocket.Conn) (*proto.ServerMessage, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	var m proto.ServerMessage
	if err := googleproto.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// readLoop pushes every frame from this generation's conn onto the shared
// Messages() stream, and closes done (once) the instant the connection ends
// — it never closes Messages() itself, so a reconnect can keep streaming on
// the same channel the caller is already reading.
func (c *Client) readLoop(conn *websocket.Conn, done chan struct{}) {
	defer close(done)
	for {
		m, err := readFrameOn(context.Background(), conn)
		if err != nil {
			c.mu.Lock()
			c.err = err
			c.mu.Unlock()
			return
		}
		c.messages <- m
	}
}

// writeLoop drains the shared sendCh for as long as this generation's
// connection is alive; it stops (letting the next generation's writeLoop
// take over) the instant done closes, so a frame sent during a reconnect gap
// simply waits in Send() rather than racing two writers over one conn.
func (c *Client) writeLoop(conn *websocket.Conn, done <-chan struct{}) {
	for {
		select {
		case req := <-c.sendCh:
			req.done <- writeFrameOn(context.Background(), conn, req.msg)
		case <-done:
			return
		}
	}
}

func (c *Client) pingLoop(conn *websocket.Conn, done <-chan struct{}) {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			_ = c.Send(context.Background(), &proto.ClientMessage{Type: "ping"})
		case <-done:
			return
		}
	}
}

// Send enqueues m on the current generation's writer, serialising every
// outbound frame. Blocks until the write completes or ctx is done — during a
// reconnect gap (no writeLoop draining sendCh) it simply waits.
func (c *Client) Send(ctx context.Context, m *proto.ClientMessage) error {
	done := make(chan error, 1)
	select {
	case c.sendCh <- sendReq{msg: m, done: done}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Messages streams every ServerMessage received after "connected", across
// reconnects, plus the synthetic TypeReconnecting/TypeReconnected markers.
// Closed only by Run() giving up or by Close() — never by a single dropped
// connection.
func (c *Client) Messages() <-chan *proto.ServerMessage { return c.messages }

// Err is the most recent read error (set on every disconnect, recoverable or
// not — check ctx/ Run's return to know whether it gave up).
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// ConnID is the current connection id the server assigned on "connected".
func (c *Client) ConnID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connID
}

// Close ends the connection and the Messages() stream. Safe to call once;
// Run observes the resulting disconnect and exits instead of reconnecting
// once its ctx is also done.
func (c *Client) Close() error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return errors.New("wsclient: not connected")
	}
	err := conn.Close(websocket.StatusNormalClosure, "")
	c.closeOnce.Do(func() { close(c.messages) })
	return err
}

// Run owns the connection's lifecycle after an initial Connect: when the
// current generation ends, it emits a TypeReconnecting marker, retries with
// full-jitter backoff (1s..30s) until a new generation connects, sends
// sync_state to resync, and emits TypeReconnected. Returns when ctx is done
// (after closing the connection) or Close() was called for good.
func (c *Client) Run(ctx context.Context) {
	for {
		select {
		case <-c.currentDone:
		case <-ctx.Done():
			_ = c.Close()
			return
		}
		if ctx.Err() != nil {
			_ = c.Close()
			return
		}

		c.messages <- &proto.ServerMessage{Type: TypeReconnecting}

		attempt := 0
		for {
			attempt++
			select {
			case <-time.After(nextBackoff(attempt, randInt63n)):
			case <-ctx.Done():
				_ = c.Close()
				return
			}
			done, err := c.dialAndHandshake(ctx, c.shareCode)
			if err == nil {
				c.currentDone = done
				break
			}
		}

		_ = c.Send(ctx, &proto.ClientMessage{Type: "sync_state"})
		c.messages <- &proto.ServerMessage{Type: TypeReconnected}
	}
}

// nextBackoff is the reconnect delay for the given attempt (1-based): full
// jitter between 0 and min(30s, 1s*2^(attempt-1)).
func nextBackoff(attempt int, rand func(n int64) int64) time.Duration {
	base := time.Second << (attempt - 1) // attempt=1 -> 1s, 2 -> 2s, ...
	const cap_ = 30 * time.Second
	if attempt <= 0 || base > cap_ || base <= 0 {
		base = cap_
	}
	return time.Duration(rand(int64(base)))
}

// randInt63n is nextBackoff's real jitter source: a value in [0, n).
func randInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	return rand.Int64N(n)
}
