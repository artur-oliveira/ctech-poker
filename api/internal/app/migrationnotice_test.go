package app

import (
	"context"
	"sync"
	"testing"
	"time"

	fws "github.com/fasthttp/websocket"
	goproto "google.golang.org/protobuf/proto"

	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/wsdrain"
)

type recordingSocket struct {
	mu     sync.Mutex
	frames [][]byte
	types  []int
}

func (r *recordingSocket) WriteControl(mt int, data []byte, _ time.Time) error {
	r.record(mt, data)
	return nil
}

func (r *recordingSocket) WriteMessage(mt int, data []byte) error {
	r.record(mt, data)
	return nil
}

func (r *recordingSocket) record(mt int, data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.types = append(r.types, mt)
	r.frames = append(r.frames, append([]byte(nil), data...))
}

func (r *recordingSocket) snapshot() ([]int, [][]byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int(nil), r.types...), append([][]byte(nil), r.frames...)
}

// TestAnnounceTableMigrationPrecedesClose is issue #354's acceptance criterion:
// a locally-connected player is handed a "table_migrating" message before the
// going-away frame, inside the wsDrainGrace budget.
func TestAnnounceTableMigrationPrecedesClose(t *testing.T) {
	sock := &recordingSocket{}
	wsdrain.Track(sock)
	t.Cleanup(func() { wsdrain.Untrack(sock) })

	start := time.Now()
	announceTableMigration(context.Background())
	if elapsed := time.Since(start); elapsed > wsDrainGrace {
		t.Fatalf("announceTableMigration blocked %s, over the %s wsDrainGrace budget", elapsed, wsDrainGrace)
	}

	wsdrain.CloseAll(context.Background(), 50*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if types, _ := sock.snapshot(); len(types) >= 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	types, frames := sock.snapshot()
	if len(frames) < 2 {
		t.Fatalf("got %d frames, want at least the notice then the close", len(frames))
	}
	if types[0] != fws.BinaryMessage {
		t.Fatalf("first frame type = %d, want a BinaryMessage app frame", types[0])
	}
	var msg pokerproto.ServerMessage
	if err := goproto.Unmarshal(frames[0], &msg); err != nil {
		t.Fatalf("first frame is not a ServerMessage: %v", err)
	}
	if msg.Type != "table_migrating" {
		t.Fatalf("notice type = %q, want table_migrating", msg.Type)
	}
	if msg.Text == "" {
		t.Fatal("notice carries no text for the client banner")
	}
	if types[1] != fws.CloseMessage {
		t.Fatalf("second frame type = %d, want the CloseMessage after the notice", types[1])
	}
}

func TestAnnounceTableMigrationNoConnsIsANoop(t *testing.T) {
	start := time.Now()
	announceTableMigration(context.Background())
	// No tracked conns: must not even spend migrationNoticeGrace.
	if elapsed := time.Since(start); elapsed > migrationNoticeGrace {
		t.Fatalf("announceTableMigration waited %s with no conns, want a fast no-op", elapsed)
	}
}
