// Package wsclient is the table WebSocket client: connects, sends the auth
// frame, and exchanges binary protobuf ClientMessage/ServerMessage envelopes
// with the poker API's table gateway.
package wsclient

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/coder/websocket"
	googleproto "google.golang.org/protobuf/proto"
	"gopkg.aoctech.app/poker/cli/internal/proto"
)

// maxMessageBytes mirrors the server's own frame cap (api/internal/api/v1/tablews.go wsMaxMessageBytes).
const maxMessageBytes = 32 * 1024

type sendReq struct {
	msg  *proto.ClientMessage
	done chan error
}

// Client is one table WebSocket connection.
type Client struct {
	url    string
	token  func(context.Context) (string, error)
	origin string

	conn *websocket.Conn

	messages chan *proto.ServerMessage
	sendCh   chan sendReq

	mu     sync.Mutex
	connID string
	err    error
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
// the read and write loops; Messages() then streams everything after
// "connected".
func (c *Client) Connect(ctx context.Context, shareCode string) error {
	tok, err := c.token(ctx)
	if err != nil {
		return err
	}

	conn, _, err := websocket.Dial(ctx, c.url, &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Origin": {c.origin}},
	})
	if err != nil {
		return err
	}
	conn.SetReadLimit(maxMessageBytes)
	c.conn = conn

	authMsg := &proto.ClientMessage{Type: "auth", Token: tok, ShareCode: shareCode}
	if err := c.writeFrame(ctx, authMsg); err != nil {
		conn.CloseNow()
		return err
	}

	first, err := c.readFrame(ctx)
	if err != nil {
		conn.CloseNow()
		return err
	}
	if first.Type != "connected" {
		conn.CloseNow()
		return fmt.Errorf("wsclient: expected a connected frame, got %q", first.Type)
	}
	c.mu.Lock()
	c.connID = first.ConnId
	c.mu.Unlock()

	go c.readLoop()
	go c.writeLoop()
	return nil
}

func (c *Client) writeFrame(ctx context.Context, m *proto.ClientMessage) error {
	b, err := googleproto.Marshal(m)
	if err != nil {
		return err
	}
	return c.conn.Write(ctx, websocket.MessageBinary, b)
}

func (c *Client) readFrame(ctx context.Context) (*proto.ServerMessage, error) {
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	var m proto.ServerMessage
	if err := googleproto.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) readLoop() {
	defer close(c.messages)
	for {
		m, err := c.readFrame(context.Background())
		if err != nil {
			c.mu.Lock()
			if c.err == nil {
				c.err = err
			}
			c.mu.Unlock()
			return
		}
		c.messages <- m
	}
}

func (c *Client) writeLoop() {
	for req := range c.sendCh {
		req.done <- c.writeFrame(context.Background(), req.msg)
	}
}

// Send enqueues m on the single writer goroutine, serialising every outbound
// frame. Blocks until the write completes or ctx is done.
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

// Messages streams every ServerMessage received after "connected". Closed
// when the connection ends; check Err() afterwards for why.
func (c *Client) Messages() <-chan *proto.ServerMessage { return c.messages }

// Err is the terminal error once Messages() is closed (nil on a clean Close).
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// ConnID is the connection id the server assigned on "connected".
func (c *Client) ConnID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connID
}

// Close ends the connection. Safe to call once.
func (c *Client) Close() error {
	if c.conn == nil {
		return errors.New("wsclient: not connected")
	}
	return c.conn.Close(websocket.StatusNormalClosure, "")
}
