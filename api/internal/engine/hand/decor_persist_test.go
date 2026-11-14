package hand

import (
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"gopkg.aoctech.app/poker/api/internal/cosmetics"
)

func TestPersistedDecorUsesSnakeCaseAttributes(t *testing.T) {
	item, err := attributevalue.MarshalMap(Player{ID: "p1", Decor: &cosmetics.Decor{Frame: "gold", Hat: "crown", Badges: []string{"b1"}}})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := item["decor"].(*types.AttributeValueMemberM)
	if !ok {
		t.Fatalf("decor attr = %#v", item["decor"])
	}
	for _, k := range []string{"frame", "hat", "badges"} {
		if _, ok := m.Value[k]; !ok {
			t.Fatalf("missing decor.%s in %v", k, m.Value)
		}
	}
	if _, ok := m.Value["Frame"]; ok {
		t.Fatal("Go-cased attribute persisted")
	}
	if _, ok := m.Value["accessory"]; ok {
		t.Fatal("empty accessory must be omitted")
	}
}

func TestSetPlayerIdentityClonesBadges(t *testing.T) {
	tb := NewTable(nil, 10, 20)
	if err := tb.AddWaitingPlayer(&Player{ID: "p1", Stack: 400}); err != nil {
		t.Fatal(err)
	}
	badges := []string{"b1", "b2"}
	tb.SetPlayerIdentityForActor("p1", "n", "", "", cosmetics.Decor{Badges: badges})
	badges[0] = "x"
	if got := tb.playerByID("p1").decor().Badges; !slices.Equal(got, []string{"b1", "b2"}) {
		t.Fatalf("seat shares caller's slice: %v", got)
	}
}
