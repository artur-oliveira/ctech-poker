package hand

import "testing"

func TestTimeBankPersistsExhaustedZeroAcrossReload(t *testing.T) {
	game := NewTable([]*Player{{ID: "p1"}, {ID: "p2"}}, 10, 20)
	if got := game.TimeBankForActor("p1"); got != DefaultTimeBankMs {
		t.Fatalf("initial time bank=%d, want %d", got, DefaultTimeBankMs)
	}
	game.ConsumeTimeBankForActor("p1", DefaultTimeBankMs)

	reloaded := NewTableFromState(game.ExportState())
	if got := reloaded.TimeBankForActor("p1"); got != 0 {
		t.Fatalf("exhausted time bank reloaded as %d", got)
	}
}
