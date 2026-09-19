package hand

import "testing"

func TestBotPolicyPersistsAndTargetsEachFormat(t *testing.T) {
	for _, tc := range []struct{ seats, target int }{{2, 2}, {6, 4}, {9, 6}} {
		table := NewTable([]*Player{{ID: "human", Stack: 5000, Ready: true}}, 25, 50)
		table.ConfigureRake("sandbox")
		if err := table.ConfigureBotsForActor("human", 3500, tc.seats, 1234); err != nil {
			t.Fatal(err)
		}
		if got := table.BotSeatsNeededForActor(); got != tc.target-1 {
			t.Fatalf("%d seats: got %d", tc.seats, got)
		}
		if err := table.AddBotForActor("bot-1", "Lia", "tag"); err != nil {
			t.Fatal(err)
		}
		restored := NewTableFromState(table.ExportState())
		got := restored.BotPolicyForActor()
		if !got.Enabled || got.TargetOccupancy != tc.target || got.OwnerID != "human" {
			t.Fatalf("policy not restored: %+v", got)
		}
	}
}

func TestSecondHumanStopsBotFill(t *testing.T) {
	table := NewTable([]*Player{{ID: "h1", Stack: 5000, Ready: true}}, 25, 50)
	table.ConfigureRake("sandbox")
	if err := table.ConfigureBotsForActor("h1", 3500, 9, 1234); err != nil {
		t.Fatal(err)
	}
	if err := table.AddWaitingPlayer(&Player{ID: "h2", Stack: 3500, Ready: true}); err != nil {
		t.Fatal(err)
	}
	if got := table.BotSeatsNeededForActor(); got != 0 {
		t.Fatalf("got %d", got)
	}
}
