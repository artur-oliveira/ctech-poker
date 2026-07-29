package v1

import (
	"encoding/json"
	"strings"
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

func TestHandShareProjectionDoesNotExposeIdentitySignals(t *testing.T) {
	source := &sessionlog.HandItem{Opponents: []sessionlog.OpponentSummary{{
		PlayerID: "secret-opponent", Name: "Ana", AvatarURL: "https://example.test/avatar.jpg",
	}}}
	projection := struct {
		Opponents any `json:"opponents"`
		Actions   any `json:"actions"`
	}{
		Opponents: anonymizedOpponents(source),
		Actions: anonymizedActions([]tablestore.ActionLogEntry{{
			PlayerID: "secret-opponent",
			Frame: &tablestore.ReplayFrame{Seats: []tablestore.ReplaySeat{{
				PlayerID: "secret-opponent", Name: "Ana",
			}}},
		}}, aliasesFor(source, "secret-owner")),
	}
	body, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret-opponent", "avatar_url", "playstyle_badge"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("shared hand leaked %q: %s", forbidden, body)
		}
	}
}
