package hand

import "testing"

func TestEscalateBlindsCapsBigBlindAndPreservesRatio(t *testing.T) {
	table := NewTable(nil, 10, 20)
	table.EscalateBlindsForActor(150, 25)
	if table.bigBlind != 25 || table.smallBlind != 12 {
		t.Fatalf("got blinds %d/%d, want 12/25", table.smallBlind, table.bigBlind)
	}
	table.EscalateBlindsForActor(150, 25)
	if table.bigBlind != 25 {
		t.Fatalf("blind exceeded cap: %d", table.bigBlind)
	}
}

func TestEscalateBlindsRejectsInvalidConfig(t *testing.T) {
	table := NewTable(nil, 10, 20)
	table.EscalateBlindsForActor(100, 1000)
	if table.bigBlind != 20 {
		t.Fatalf("non-increasing multiplier changed blind to %d", table.bigBlind)
	}
}
