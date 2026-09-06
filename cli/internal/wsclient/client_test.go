package wsclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	googleproto "google.golang.org/protobuf/proto"
	"gopkg.aoctech.app/poker/cli/internal/proto"
)

func staticToken(tok string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return tok, nil }
}

func TestConnectSendsAuthFrameFirstAndReceivesConnected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer c.CloseNow()

		_, data, err := c.Read(r.Context())
		if err != nil {
			t.Error(err)
			return
		}
		var first proto.ClientMessage
		if err := googleproto.Unmarshal(data, &first); err != nil {
			t.Error(err)
			return
		}
		if first.Type != "auth" || first.Token != "jwt-xyz" {
			t.Errorf("first frame not a valid auth frame: %+v", &first)
		}

		out, _ := googleproto.Marshal(&proto.ServerMessage{Type: "connected", ConnId: "c1"})
		c.Write(r.Context(), websocket.MessageBinary, out)

		st, _ := googleproto.Marshal(&proto.ServerMessage{Type: "state", Snapshot: &proto.TableSnapshot{Stage: "preflop"}})
		c.Write(r.Context(), websocket.MessageBinary, st)

		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	cl := New(wsURL, staticToken("jwt-xyz"), "https://poker.aoctech.app")
	if err := cl.Connect(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if cl.ConnID() != "c1" {
		t.Errorf("conn id = %q", cl.ConnID())
	}

	select {
	case msg := <-cl.Messages():
		if msg.Type != "state" || msg.Snapshot.Stage != "preflop" {
			t.Fatalf("unexpected first stream message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the state message")
	}
	cl.Close()
}

func TestConnectToleratesFramesBeforeConnected(t *testing.T) {
	// The server can race a broadcast onto the wire ahead of "connected".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		defer c.CloseNow()
		if _, _, err := c.Read(r.Context()); err != nil {
			return
		}
		st, _ := googleproto.Marshal(&proto.ServerMessage{Type: "state", Snapshot: &proto.TableSnapshot{Stage: "flop"}})
		c.Write(r.Context(), websocket.MessageBinary, st)
		ok, _ := googleproto.Marshal(&proto.ServerMessage{Type: "connected", ConnId: "c2"})
		c.Write(r.Context(), websocket.MessageBinary, ok)
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	cl := New(wsURL, staticToken("t"), "https://poker.aoctech.app")
	if err := cl.Connect(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if cl.ConnID() != "c2" {
		t.Errorf("conn id = %q", cl.ConnID())
	}
	select {
	case msg := <-cl.Messages():
		if msg.Type != "state" || msg.Snapshot.Stage != "flop" {
			t.Fatalf("pre-connected frame not replayed: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the replayed state frame")
	}
}

func TestConnectRejectsErrorFrame(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		defer c.CloseNow()
		if _, _, err := c.Read(r.Context()); err != nil {
			return
		}
		e, _ := googleproto.Marshal(&proto.ServerMessage{Type: "error", Code: "forbidden"})
		c.Write(r.Context(), websocket.MessageBinary, e)
		time.Sleep(50 * time.Millisecond)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	cl := New(wsURL, staticToken("t"), "https://poker.aoctech.app")
	if err := cl.Connect(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expected a forbidden rejection, got %v", err)
	}
}

func TestSendSerializesFramesThroughOneWriter(t *testing.T) {
	received := make(chan *proto.ClientMessage, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		defer c.CloseNow()
		for i := 0; i < 3; i++ {
			_, data, err := c.Read(r.Context())
			if err != nil {
				return
			}
			var m proto.ClientMessage
			googleproto.Unmarshal(data, &m)
			received <- &m
			if i == 0 { // the auth frame — reply "connected" so Connect() unblocks
				out, _ := googleproto.Marshal(&proto.ServerMessage{Type: "connected", ConnId: "c1"})
				c.Write(r.Context(), websocket.MessageBinary, out)
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	cl := New(wsURL, staticToken("t"), "https://poker.aoctech.app")
	if err := cl.Connect(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	<-received // the auth frame

	for i := 0; i < 2; i++ {
		if err := cl.Send(context.Background(), &proto.ClientMessage{Type: "ping"}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		select {
		case m := <-received:
			if m.Type != "ping" {
				t.Fatalf("unexpected message: %+v", m)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for a sent ping")
		}
	}
}
