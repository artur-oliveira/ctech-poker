package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	goproto "google.golang.org/protobuf/proto"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/cosmeticpurchase"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// sentFrame is one decoded websocket frame and the fan-out key it went to.
type sentFrame struct {
	key string
	msg *pokerproto.ServerMessage
}

type recordingRegistry struct {
	noopRegistry
	mu     sync.Mutex
	frames []sentFrame
}

func (r *recordingRegistry) Broadcast(_ context.Context, key string, data []byte) {
	var msg pokerproto.ServerMessage
	if err := goproto.Unmarshal(data, &msg); err != nil {
		panic(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, sentFrame{key: key, msg: &msg})
}

func (r *recordingRegistry) to(key string) []*pokerproto.ServerMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*pokerproto.ServerMessage
	for _, f := range r.frames {
		if f.key == key {
			out = append(out, f.msg)
		}
	}
	return out
}

// fakeGranter mirrors cosmeticpurchase.Service.Grant's contract: the first
// grant of an item creates it, a repeat is a no-op reporting created=false.
type fakeGranter struct {
	mu      sync.Mutex
	owned   map[string]bool
	fail    map[string]bool // item ids whose Grant errors
	calls   []string        // "player/kind/id/reason"
	created int
}

func newFakeGranter() *fakeGranter {
	return &fakeGranter{owned: map[string]bool{}, fail: map[string]bool{}}
}

func (g *fakeGranter) Grant(_ context.Context, playerID string, kind cosmetics.Kind, itemID, reason string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, fmt.Sprintf("%s/%s/%s/%s", playerID, kind, itemID, reason))
	if g.fail[itemID] {
		return false, errors.New("dynamo down")
	}
	key := playerID + "/" + string(kind) + "/" + itemID
	if g.owned[key] {
		return false, nil
	}
	g.owned[key] = true
	g.created++
	return true, nil
}

func assertAchievementFrame(t *testing.T, msg *pokerproto.ServerMessage, key string, stars int32) {
	t.Helper()
	if msg.GetType() != "achievement_unlocked" || msg.GetKey() != key || msg.GetStars() != stars {
		t.Fatalf("want achievement_unlocked %s/%d, got %+v", key, stars, msg)
	}
}

func assertCosmeticFrame(t *testing.T, msg *pokerproto.ServerMessage, kind cosmetics.Kind, id string) {
	t.Helper()
	if msg.GetType() != "cosmetic_unlocked" || msg.GetCosmeticKind() != string(kind) || msg.GetCosmeticId() != id {
		t.Fatalf("want cosmetic_unlocked %s/%s, got %+v", kind, id, msg)
	}
}

func TestUnlockGrantsMappedItemAfterAchievementFrame(t *testing.T) {
	reg, granter := &recordingRegistry{}, newFakeGranter()
	p := &handPipeline{reg: reg, cosmetics: granter}

	p.announceUnlocks(context.Background(), "t1", []achievements.TierUnlock{{PlayerID: "p1", Key: "hands_played", Stars: 1}})

	frames := reg.to("t1#p1")
	if len(frames) != 2 {
		t.Fatalf("want achievement + one cosmetic frame, got %d: %+v", len(frames), frames)
	}
	assertAchievementFrame(t, frames[0], "hands_played", 1)
	assertCosmeticFrame(t, frames[1], cosmetics.KindHat, "dealer-visor")
	if want := "p1/hat/dealer-visor/achievement:hands_played:1"; len(granter.calls) != 1 || granter.calls[0] != want {
		t.Fatalf("grant calls = %v, want [%s]", granter.calls, want)
	}
}

func TestRepeatedUnlockGrantsNothingTheSecondTime(t *testing.T) {
	reg, granter := &recordingRegistry{}, newFakeGranter()
	p := &handPipeline{reg: reg, cosmetics: granter}
	unlocks := []achievements.TierUnlock{{PlayerID: "p1", Key: "hands_played", Stars: 1}}

	p.announceUnlocks(context.Background(), "t1", unlocks)
	p.announceUnlocks(context.Background(), "t1", unlocks)

	frames := reg.to("t1#p1")
	if len(frames) != 3 {
		t.Fatalf("want 2 frames then 1 (no cosmetic on the repeat), got %d: %+v", len(frames), frames)
	}
	assertAchievementFrame(t, frames[2], "hands_played", 1)
	if granter.created != 1 {
		t.Fatalf("the item must be written once, created %d times", granter.created)
	}
}

func TestGrantErrorDoesNotStopOtherUnlocks(t *testing.T) {
	reg, granter := &recordingRegistry{}, newFakeGranter()
	granter.fail["dealer-visor"] = true
	p := &handPipeline{reg: reg, cosmetics: granter}

	p.announceUnlocks(context.Background(), "t1", []achievements.TierUnlock{
		{PlayerID: "p1", Key: "hands_played", Stars: 1},
		{PlayerID: "p2", Key: "wins", Stars: 3},
	})

	p1 := reg.to("t1#p1")
	if len(p1) != 1 {
		t.Fatalf("failed grant must send no cosmetic frame, got %+v", p1)
	}
	assertAchievementFrame(t, p1[0], "hands_played", 1)
	p2 := reg.to("t1#p2")
	if len(p2) != 2 {
		t.Fatalf("second unlock must still be processed, got %+v", p2)
	}
	assertAchievementFrame(t, p2[0], "wins", 3)
	assertCosmeticFrame(t, p2[1], cosmetics.KindHat, "laurel")
}

func TestUnmappedUnlockGrantsNothing(t *testing.T) {
	reg, granter := &recordingRegistry{}, newFakeGranter()
	p := &handPipeline{reg: reg, cosmetics: granter}

	p.announceUnlocks(context.Background(), "t1", []achievements.TierUnlock{{PlayerID: "p1", Key: achievements.KeyComeback, Stars: 1}})

	if len(granter.calls) != 0 {
		t.Fatalf("unmapped unlock must not grant, got %v", granter.calls)
	}
	if frames := reg.to("t1#p1"); len(frames) != 1 {
		t.Fatalf("want only the achievement frame, got %+v", frames)
	}
}

func TestTierSkippingUnlockGrantsEveryLowerRow(t *testing.T) {
	reg, granter := &recordingRegistry{}, newFakeGranter()
	p := &handPipeline{reg: reg, cosmetics: granter}

	p.announceUnlocks(context.Background(), "t1", []achievements.TierUnlock{{PlayerID: "p1", Key: "hands_played", Stars: 3}})

	frames := reg.to("t1#p1")
	if len(frames) != 4 {
		t.Fatalf("want achievement + 3 cosmetic frames, got %d: %+v", len(frames), frames)
	}
	assertAchievementFrame(t, frames[0], "hands_played", 3)
	assertCosmeticFrame(t, frames[1], cosmetics.KindHat, "dealer-visor")
	assertCosmeticFrame(t, frames[2], cosmetics.KindBadge, "veteran")
	assertCosmeticFrame(t, frames[3], cosmetics.KindFrame, "chip-ring")
}

func TestNilGranterStillAnnouncesAchievements(t *testing.T) {
	reg := &recordingRegistry{}
	p := &handPipeline{reg: reg}

	p.announceUnlocks(context.Background(), "t1", []achievements.TierUnlock{{PlayerID: "p1", Key: "hands_played", Stars: 1}})

	frames := reg.to("t1#p1")
	if len(frames) != 1 {
		t.Fatalf("want only the achievement frame, got %+v", frames)
	}
	assertAchievementFrame(t, frames[0], "hands_played", 1)
}

// unlockingPipeline runs a hand whose progress write reports every seat
// crossing hands_played tier 1 (99 -> 100), the one tier that maps to an item.
func unlockingPipeline(t *testing.T, seats int) (*handPipeline, *countingDynamo, hand.HandOutcome, map[string]string) {
	t.Helper()
	pipeline, stub := countingPipeline()
	stub.progressAttrs = `{"c#hands_played":{"N":"100"}}`
	outcome, names, _ := worstCaseHandOutcome(seats)
	return pipeline, stub, outcome, names
}

// End to end over the real stores: a failing Grant is logged and the rest of
// the hand (achievement frame, leaderboard) still runs.
func TestHandPipelineGrantFailureIsIsolated(t *testing.T) {
	pipeline, stub, outcome, names := unlockingPipeline(t, 2)
	reg, granter := &recordingRegistry{}, newFakeGranter()
	granter.fail["dealer-visor"] = true
	pipeline.reg, pipeline.cosmetics = reg, granter

	pipeline.run(context.Background(), "01M1HABKV5Z90SGHZ63K44EGHB", "01M1HABKV5Z90SGHZ63K44EGHC", outcome, names)

	for _, id := range outcome.Participants {
		frames := reg.to("01M1HABKV5Z90SGHZ63K44EGHB#" + id)
		if len(frames) != 1 {
			t.Fatalf("%s: want only the achievement frame, got %+v", id, frames)
		}
		assertAchievementFrame(t, frames[0], "hands_played", 1)
	}
	if len(granter.calls) != len(outcome.Participants) {
		t.Fatalf("every seat's unlock must be attempted, got %v", granter.calls)
	}
	if !touched(stub.tables, "poker_leaderboard_stats") {
		t.Fatalf("leaderboard must still be written after a failed grant")
	}
}

// TestHandPipelineGrantBudget extends #204's budget for the one new path:
// granting costs DynamoDB only when an unlock maps to an item, and then
// exactly one conditional TransactWriteItems per granted item. Measured as the
// delta between the same unlocking hand with and without the real granter, so
// the rest of the unlock path's cost (stamps, achievement points) stays pinned
// where it already is.
func TestHandPipelineGrantBudget(t *testing.T) {
	const seats = 6
	measure := func(withGranter bool) *countingDynamo {
		pipeline, stub, outcome, names := unlockingPipeline(t, seats)
		if withGranter {
			pipeline.cosmetics = cosmeticpurchase.NewService(nil, cosmeticpurchase.NewEntitlementStore(stub.client, "budget_test"), nil)
		}
		pipeline.run(context.Background(), "01M1HABKV5Z90SGHZ63K44EGHB", "01M1HABKV5Z90SGHZ63K44EGHC", outcome, names)
		return stub
	}
	without, with := measure(false), measure(true)
	total := func(c *countingDynamo) (n int) {
		for _, v := range c.calls {
			n += v
		}
		return n
	}
	if got := with.calls["TransactWriteItems"] - without.calls["TransactWriteItems"]; got != seats {
		t.Errorf("want one entitlement transaction per newly granted item (%d), got %d", seats, got)
	}
	if got := total(with) - total(without); got != seats {
		t.Errorf("grants added %d DynamoDB calls, want exactly %d", got, seats)
	}
	if got := with.writeItems - without.writeItems; got != seats {
		t.Errorf("grants added %d written items, want exactly %d", got, seats)
	}
	if !touched(with.tables, "poker_cosmetic_entitlements") {
		t.Errorf("no entitlement was written: tables %v", sortedOps(with.tables))
	}
}
