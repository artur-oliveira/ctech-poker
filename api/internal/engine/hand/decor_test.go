package hand

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
)

func decorTable() *Table {
	return NewTable([]*Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
}

func TestSetPlayerIdentityDecorChangeReportsChanged(t *testing.T) {
	tb := decorTable()
	d := cosmetics.Decor{Frame: "gold", Hat: "cap", Badges: []string{"a", "b"}}
	if !tb.SetPlayerIdentityForActor("p1", "Ann", "", "", d) {
		t.Fatal("first set should report changed")
	}
	if tb.SetPlayerIdentityForActor("p1", "Ann", "", "", cosmetics.Decor{Frame: "gold", Hat: "cap", Badges: []string{"a", "b"}}) {
		t.Fatal("identical repeat should be a no-op")
	}
	if !tb.SetPlayerIdentityForActor("p1", "Ann", "", "", cosmetics.Decor{Frame: "gold", Hat: "cap", Badges: []string{"b", "a"}}) {
		t.Fatal("badge order change should report changed")
	}
}

func TestSetPlayerIdentityClearsDecorWhenAllElseEmpty(t *testing.T) {
	tb := decorTable()
	tb.SetPlayerIdentityForActor("p1", "", "", "", cosmetics.Decor{Frame: "gold"})
	if !tb.SetPlayerIdentityForActor("p1", "", "", "", cosmetics.Decor{}) {
		t.Fatal("clearing decor must report true")
	}
	if tb.playerByID("p1").Decor != nil {
		t.Fatal("seat decor not cleared")
	}
	if tb.SetPlayerIdentityForActor("p1", "", "", "", cosmetics.Decor{}) {
		t.Fatal("all-empty on empty seat must be a no-op")
	}
}

func TestSeatViewCarriesDecor(t *testing.T) {
	tb := decorTable()
	tb.SetPlayerIdentityForActor("p1", "Ann", "", "", cosmetics.Decor{Frame: "f", Hat: "h", Accessory: "x", Badges: []string{"b1"}})
	if err := tb.StartHand(); err != nil {
		t.Fatal(err)
	}
	for _, s := range tb.ViewFor("p2").Seats {
		if s.PlayerID == "p1" && (s.FrameID != "f" || s.HatID != "h" || s.AccessoryID != "x" || len(s.BadgeIDs) != 1 || s.BadgeIDs[0] != "b1") {
			t.Fatalf("seat decor = %+v", s)
		}
	}
}

func TestPlayerLoadsWithoutDecorAttributes(t *testing.T) {
	av, err := attributevalue.MarshalMap(Player{ID: "p1", Stack: 5})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := av["decor"]; ok {
		t.Fatal("zero decor should be omitted from the item")
	}
	var p Player
	if err := attributevalue.UnmarshalMap(av, &p); err != nil {
		t.Fatal(err)
	}
	if p.Decor != nil {
		t.Fatalf("decor = %+v", p.Decor)
	}
}
