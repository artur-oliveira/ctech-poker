package tui

import "testing"

func testSpecs() []commandSpec {
	return []commandSpec{
		{Name: "/profile", Desc: "a"},
		{Name: "/play", Desc: "b"},
		{Name: "/peek", Args: "[all|1|2]", Desc: "c"},
	}
}

func TestMenuHiddenWithoutSlashPrefix(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("profile")
	if m.visible {
		t.Fatal("menu should stay hidden without a leading slash")
	}
}

func TestMenuHiddenOnceArgumentsStart(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("/peek ")
	if m.visible {
		t.Fatal("menu should hide once the user moves on to arguments")
	}
}

func TestMenuNarrowsOnPrefix(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("/p")
	if !m.visible || len(m.items) != 3 {
		t.Fatalf("visible=%v items=%d", m.visible, len(m.items))
	}
	m.UpdateInput("/pl")
	if !m.visible || len(m.items) != 1 || m.items[0].Name != "/play" {
		t.Fatalf("got %+v", m.items)
	}
}

func TestMenuHidesOnExactSingleMatch(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("/play")
	if m.visible {
		t.Fatal("an exact single match should hide the menu (nothing left to suggest)")
	}
}

func TestMenuNavigation(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("/p")
	m.moveNext()
	if m.selected != 1 {
		t.Fatalf("selected = %d", m.selected)
	}
	m.moveNext()
	m.moveNext()
	if m.selected != 0 {
		t.Fatalf("wrap-around: selected = %d", m.selected)
	}
	m.movePrev()
	if m.selected != 2 {
		t.Fatalf("wrap-around backwards: selected = %d", m.selected)
	}
}

func TestAcceptZeroArgCommandSubmitsImmediately(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("/p")
	m.selected = 0 // /profile
	val, submit := m.accept()
	if val != "/profile" || !submit {
		t.Fatalf("val=%q submit=%v", val, submit)
	}
	if m.visible {
		t.Fatal("accept should hide the menu")
	}
}

func TestAcceptArgCommandFillsAndWaits(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("/pe")
	val, submit := m.accept()
	if val != "/peek " || submit {
		t.Fatalf("val=%q submit=%v", val, submit)
	}
}
