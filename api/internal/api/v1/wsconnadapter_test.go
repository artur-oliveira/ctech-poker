package v1

import (
	"net"
	"net/http"
	"testing"
	"time"

	fws "github.com/fasthttp/websocket"
	"github.com/valyala/fasthttp"
)

// TestWSConnAdapterWriteMessageHasDeadline guards against the 2026-07-27
// audit finding: wsConnAdapter.WriteMessage used to never call
// SetWriteDeadline, so a peer that stops reading could block a write
// indefinitely — and since the table actor's broadcastAll calls WriteMessage
// synchronously per seated player from its single Run goroutine, one stalled
// viewer stalled every other player's actions at that table. This test opens
// a real WS connection, has the client stop reading, floods the server side
// with writes through wsConnAdapter, and asserts a write eventually fails
// within roughly wsWriteWait instead of hanging.
func TestWSConnAdapterWriteMessageHasDeadline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	upgrader := fws.FastHTTPUpgrader{CheckOrigin: func(*fasthttp.RequestCtx) bool { return true }}
	serverErrCh := make(chan error, 1)
	elapsedCh := make(chan time.Duration, 1)

	server := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			err := upgrader.Upgrade(ctx, func(conn *fws.Conn) {
				adapter := &wsConnAdapter{conn: conn}
				payload := make([]byte, 64*1024) // large enough to fill OS/socket buffers quickly
				start := time.Now()
				var writeErr error
				// Keep writing until the peer's un-drained receive buffer backs
				// up the write path, or SetWriteDeadline cuts it off — whichever
				// comes first. A bounded loop keeps this test itself bounded even
				// if the deadline regressed back to "never".
				deadline := time.Now().Add(20 * time.Second)
				for time.Now().Before(deadline) {
					writeErr = adapter.WriteMessage(fws.BinaryMessage, payload)
					if writeErr != nil {
						break
					}
				}
				elapsedCh <- time.Since(start)
				serverErrCh <- writeErr
			})
			if err != nil {
				serverErrCh <- err
				elapsedCh <- 0
			}
		},
	}
	go func() { _ = server.Serve(ln) }()
	defer server.Shutdown()

	url := "ws://" + ln.Addr().String() + "/"
	dialer := fws.Dialer{}
	clientConn, resp, err := dialer.Dial(url, http.Header{})
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	defer clientConn.Close()
	// Deliberately never call clientConn.ReadMessage() — simulates a stalled
	// reader (slow/dead client) that never drains its receive buffer.

	select {
	case err := <-serverErrCh:
		elapsed := <-elapsedCh
		if err == nil {
			t.Fatal("expected WriteMessage to eventually fail once the peer stopped reading, got nil error")
		}
		if elapsed > 15*time.Second {
			t.Fatalf("WriteMessage took %s to fail — SetWriteDeadline does not appear to be bounding the write", elapsed)
		}
		t.Logf("WriteMessage failed after %s: %v", elapsed, err)
	case <-time.After(25 * time.Second):
		t.Fatal("server-side WriteMessage never returned — write is unbounded (no deadline applied)")
	}
}
