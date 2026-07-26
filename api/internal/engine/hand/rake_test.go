package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func TestSandboxRakeUsesPercentageAndPlayerCaps(t *testing.T) {
	table := NewTable(nil, 50, 100)
	table.ConfigureRake("sandbox")
	table.board = make([]deck.Card, 3)

	table.handOrder = []*Player{{ID: "p1"}, {ID: "p2"}}
	if cap := table.rakeCap(); cap != 50 {
		t.Fatalf("heads-up cap = %d, want 50 (0.5 BB)", cap)
	}
	if rake := table.rakeForLayer(1000, table.rakeCap()); rake != 25 {
		t.Fatalf("2.5%% of 1000 = %d, want 25", rake)
	}
	if rake := table.rakeForLayer(10000, table.rakeCap()); rake != 50 {
		t.Fatalf("rake should be capped at 50, got %d", rake)
	}

	table.handOrder = []*Player{{}, {}, {}}
	if cap := table.rakeCap(); cap != 75 {
		t.Fatalf("3-player cap = %d, want 75 (0.75 BB)", cap)
	}
	table.handOrder = []*Player{{}, {}, {}, {}, {}}
	if cap := table.rakeCap(); cap != 100 {
		t.Fatalf("5-player cap = %d, want 100 (1 BB)", cap)
	}
}

func TestRealMoneyAndPreflopPotsHaveNoRake(t *testing.T) {
	table := NewTable(nil, 50, 100)
	table.handOrder = []*Player{{}, {}}
	table.board = make([]deck.Card, 3)
	table.ConfigureRake("real")
	if cap := table.rakeCap(); cap != 0 {
		t.Fatalf("real-money cap = %d, want zero (real-money never takes rake)", cap)
	}

	table.ConfigureRake("sandbox")
	table.board = nil
	if cap := table.rakeCap(); cap != 0 {
		t.Fatalf("preflop cap = %d, want zero", cap)
	}
}

func TestRakeConfigurationSurvivesPersistence(t *testing.T) {
	table := NewTable(nil, 50, 100)
	table.ConfigureRake("sandbox")
	table.rakeCollected = 17
	rebuilt := NewTableFromState(table.ExportState())
	if rebuilt.rakeBPS != 250 || rebuilt.rakeCollected != 17 {
		t.Fatalf("rake state was not preserved: bps=%d collected=%d", rebuilt.rakeBPS, rebuilt.rakeCollected)
	}
}
