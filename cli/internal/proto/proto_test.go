package proto

import (
	"testing"

	googleproto "google.golang.org/protobuf/proto"
)

func TestClientMessageRoundTrip(t *testing.T) {
	in := &ClientMessage{Type: "act", Action: "raise", Amount: 240, ActionId: "a-1"}
	b, err := googleproto.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out ClientMessage
	if err := googleproto.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Type != "act" || out.Action != "raise" || out.Amount != 240 || out.ActionId != "a-1" {
		t.Fatalf("round trip mismatch: %+v", &out)
	}
}

func TestServerMessageCarriesSnapshot(t *testing.T) {
	in := &ServerMessage{Type: "state", Snapshot: &TableSnapshot{Stage: "flop", Board: []string{"Ah", "7c", "Kd"}}}
	b, err := googleproto.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out ServerMessage
	if err := googleproto.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Snapshot == nil || out.Snapshot.Stage != "flop" || len(out.Snapshot.Board) != 3 {
		t.Fatalf("round trip mismatch: %+v", out.Snapshot)
	}
}
