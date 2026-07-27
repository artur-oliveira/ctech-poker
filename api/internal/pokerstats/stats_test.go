package pokerstats

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func TestAnalyzeVPIPPFRAndThreeBet(t *testing.T) {
	preflop := &tablestore.ReplayFrame{Stage: "pre_flop"}
	flop := &tablestore.ReplayFrame{Stage: "flop", Board: []string{"As", "Kd", "2c"}}
	got := Analyze([]string{"a", "b", "c"}, []tablestore.ActionLogEntry{
		{PlayerID: "a", Action: "raise", BettingAction: "raise", Frame: preflop},
		{PlayerID: "b", Action: "all_in", BettingAction: "call", Frame: preflop},
		{PlayerID: "c", Action: "raise", BettingAction: "raise", Frame: preflop},
		{PlayerID: "a", Action: "call", BettingAction: "call", Frame: flop},
		{PlayerID: "b", Action: "raise", BettingAction: "raise", Frame: &tablestore.ReplayFrame{Stage: "flop"}},
	})
	byID := map[string]HandMetric{}
	for _, metric := range got {
		byID[metric.PlayerID] = metric
	}
	if !byID["a"].VPIP || !byID["a"].PFR || byID["a"].ThreeBet || byID["a"].ThreeBetChance {
		t.Fatalf("opener metrics = %+v", byID["a"])
	}
	if !byID["b"].VPIP || byID["b"].PFR || !byID["b"].ThreeBetChance {
		t.Fatalf("all-in caller metrics = %+v", byID["b"])
	}
	if !byID["c"].VPIP || !byID["c"].PFR || !byID["c"].ThreeBet || !byID["c"].ThreeBetChance {
		t.Fatalf("three-bettor metrics = %+v", byID["c"])
	}
}

func TestAnalyzeOldAllInIsConservative(t *testing.T) {
	got := Analyze([]string{"a"}, []tablestore.ActionLogEntry{{PlayerID: "a", Action: "all_in"}})
	if len(got) != 1 || !got[0].VPIP || got[0].PFR || got[0].ThreeBet {
		t.Fatalf("old all-in metrics = %+v", got)
	}
}
