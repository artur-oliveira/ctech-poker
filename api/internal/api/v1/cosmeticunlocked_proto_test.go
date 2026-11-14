package v1

import (
	"testing"

	goproto "google.golang.org/protobuf/proto"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
)

// The cosmetic_unlocked frame's two payload fields must survive the wire, at
// their own field numbers (22/23), next to the achievement frame's key/stars.
func TestCosmeticUnlockedFrameRoundTrip(t *testing.T) {
	in := &pokerproto.ServerMessage{Type: "cosmetic_unlocked", CosmeticKind: "hat", CosmeticId: "dealer-visor"}
	data, err := goproto.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out pokerproto.ServerMessage
	if err := goproto.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.GetType() != "cosmetic_unlocked" || out.GetCosmeticKind() != "hat" || out.GetCosmeticId() != "dealer-visor" {
		t.Fatalf("round trip lost fields: %+v", &out)
	}
	fields := out.ProtoReflect().Descriptor().Fields()
	if fields.ByName("cosmetic_kind").Number() != 22 || fields.ByName("cosmetic_id").Number() != 23 {
		t.Fatalf("cosmetic_kind/cosmetic_id must be fields 22/23")
	}
}
