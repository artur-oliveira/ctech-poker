package v1

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func TestAnonymizedActionsRemovePlayerIDsAndNames(t *testing.T) {
	source := &sessionlog.HandItem{Opponents: []sessionlog.OpponentSummary{{PlayerID: "secret-opponent", Name: "Ana"}}}
	actions := anonymizedActions([]tablestore.ActionLogEntry{{
		PlayerID: "secret-opponent", Action: "call",
		Frame: &tablestore.ReplayFrame{Seats: []tablestore.ReplaySeat{{
			PlayerID: "secret-opponent", Name: "Ana",
		}}},
	}}, aliasesFor(source, "secret-owner"))
	if actions[0].PlayerID != "player_1" || actions[0].Frame.Seats[0].Name != "Jogador" {
		t.Fatalf("identity leaked in projection: %+v", actions)
	}
}
