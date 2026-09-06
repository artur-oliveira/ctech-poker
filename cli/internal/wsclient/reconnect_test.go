package wsclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	googleproto "google.golang.org/protobuf/proto"
	"gopkg.aoctech.app/poker/cli/internal/proto"
)

func TestNextBackoffStaysWithinFullJitterRange(t *testing.T) {
	always := func(n int64) int64 { return n - 1 } // deterministic: always the top of the range
	if got := nextBackoff(1, always); got != time.Second-1 {
		t.Errorf("attempt 1 = %v", got)
	}
	if got := nextBackoff(5, always); got != 16*time.Second-1 {
		t.Errorf("attempt 5 = %v", got)
	}
	if got := nextBackoff(10, always); got != 30*time.Second-1 {
		t.Errorf("attempt 10 should be capped at 30s, got %v", got)
	}
}

func TestRunReconnectsAfterADropAndResyncs(t *testing.T) {
	var upgrades int32
	var gotSyncState int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gen := atomic.AddInt32(&upgrades, 1)
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer c.CloseNow()

		if _, _, err := c.Read(r.Context()); err != nil { // auth frame
			return
		}
		out, _ := googleproto.Marshal(&proto.ServerMessage{Type: "connected", ConnId: "c"})
		c.Write(r.Context(), websocket.MessageBinary, out)

		if gen == 1 {
			// First generation: drop the connection shortly after connecting.
			time.Sleep(20 * time.Millisecond)
			return
		}
		// Second generation: expect the resync frame, then stay up.
		_, data, err := c.Read(r.Context())
		if err == nil {
			var m proto.ClientMessage
			googleproto.Unmarshal(data, &m)
			if m.Type == "sync_state" {
				atomic.StoreInt32(&gotSyncState, 1)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	cl := New(wsURL, staticToken("t"), "https://poker.aoctech.app")
	if err := cl.Connect(context.Background(), ""); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go cl.Run(ctx)

	var sawReconnecting, sawReconnected bool
	deadline := time.After(1500 * time.Millisecond)
	for !sawReconnected {
		select {
		case msg := <-cl.Messages():
			switch msg.Type {
			case TypeReconnecting:
				sawReconnecting = true
			case TypeReconnected:
				sawReconnected = true
			}
		case <-deadline:
			t.Fatalf("timed out: reconnecting=%v reconnected=%v", sawReconnecting, sawReconnected)
		}
	}
	if !sawReconnecting {
		t.Error("never saw a TypeReconnecting marker")
	}
	if atomic.LoadInt32(&upgrades) < 2 {
		t.Errorf("expected a second upgrade, got %d", upgrades)
	}
	// The sync_state write completing on the client is not the same instant
	// the server's Read returns it — give that a moment before asserting.
	for i := 0; i < 50 && atomic.LoadInt32(&gotSyncState) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&gotSyncState) != 1 {
		t.Error("reconnect never sent sync_state")
	}
}
