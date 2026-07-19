package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
)

func TestReplayReproducesLiveState(t *testing.T) {
	live := NewTable([]*Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	_ = live.StartHand()
	toAct := live.playerToActForTest()
	_ = live.Act(toAct, betting.ActionCall, 0)

	entries := []ReplayedAction{
		{PlayerID: toAct, Action: betting.ActionCall},
	}

	recovered := NewTable([]*Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	if err := recovered.Replay("hand-1", entries); err != nil {
		t.Fatalf("replay: %v", err)
	}

	liveView := live.ViewFor("p1")
	recoveredView := recovered.ViewFor("p1")
	if liveView.Stage != recoveredView.Stage {
		t.Fatalf("expected replay to reach the same stage: live=%s recovered=%s", liveView.Stage, recoveredView.Stage)
	}
}
