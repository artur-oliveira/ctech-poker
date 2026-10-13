package userpurge

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	testSub    = "11111111-1111-4111-8111-111111111111"
	testPseudo = "anon_test"
)

func TestAnonymizeItemDropsPIIOfTheErasedPlayerOnly(t *testing.T) {
	item := map[string]types.AttributeValue{
		"pk": str("other"), "sk": str("sandbox#h1"),
		"opponents": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"player_id": str(testSub), "name": str("Alice"),
				"avatar_url": str("https://x/v1.0/avatars/" + testSub + "/1.jpg"),
				"won":        &types.AttributeValueMemberBOOL{Value: true},
			}},
			&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{"player_id": str("bob"), "name": str("Bob")}},
		}},
		"payouts":   &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{testSub: &types.AttributeValueMemberN{Value: "10"}}},
		"reporters": &types.AttributeValueMemberSS{Value: []string{testSub, "bob"}},
	}
	out, changed := anonymizeItem(item, testSub, testPseudo)
	if !changed {
		t.Fatal("expected a change")
	}
	opps := out["opponents"].(*types.AttributeValueMemberL).Value
	alice := opps[0].(*types.AttributeValueMemberM).Value
	if alice["player_id"].(*types.AttributeValueMemberS).Value != testPseudo {
		t.Fatalf("player_id not pseudonymized: %+v", alice["player_id"])
	}
	for _, k := range []string{"name", "avatar_url"} {
		if _, ok := alice[k]; ok {
			t.Fatalf("%s of the erased player kept", k)
		}
	}
	if _, ok := alice["won"]; !ok {
		t.Fatal("game data must stay")
	}
	if opps[1].(*types.AttributeValueMemberM).Value["name"].(*types.AttributeValueMemberS).Value != "Bob" {
		t.Fatal("another player's name must stay")
	}
	if _, ok := out["payouts"].(*types.AttributeValueMemberM).Value[testPseudo]; !ok {
		t.Fatal("map key not pseudonymized")
	}
	if got := out["reporters"].(*types.AttributeValueMemberSS).Value; got[0] != testPseudo || got[1] != "bob" {
		t.Fatalf("string set: %v", got)
	}
	if out["pk"].(*types.AttributeValueMemberS).Value != "other" || out["sk"].(*types.AttributeValueMemberS).Value != "sandbox#h1" {
		t.Fatal("keys must not move")
	}
	if _, again := anonymizeItem(out, testSub, testPseudo); again {
		t.Fatal("a second pass must be a no-op")
	}
}

func TestAnonymizeItemKeepsKeysAndDropsOwnChat(t *testing.T) {
	item := map[string]types.AttributeValue{"pk": str(testSub), "player_id": str(testSub), "message": str("oi")}
	out, changed := anonymizeItem(item, testSub, testPseudo)
	if !changed || out["pk"].(*types.AttributeValueMemberS).Value != testSub {
		t.Fatal("pk must stay; rows keyed by the sub are re-keyed, not rewritten")
	}
	if _, ok := out["message"]; ok {
		t.Fatal("chat text of the erased player kept")
	}
}

func TestAnonymizeJSONLinesKeepsNumbersExact(t *testing.T) {
	body := []byte(`{"player_id":"` + testSub + `","message":"oi","amount":12345678901234567890,"frame":{"seats":[{"player_id":"` + testSub + `","name":"Alice"},{"player_id":"bob","name":"Bob"}]}}` + "\n" +
		`{"player_id":"bob","amount":1}` + "\n")
	out, changed, err := anonymizeJSONLines(body, testSub, testPseudo)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	text := string(out)
	for _, gone := range []string{testSub, "Alice", `"message"`} {
		if strings.Contains(text, gone) {
			t.Fatalf("%q survived: %s", gone, text)
		}
	}
	for _, kept := range []string{"12345678901234567890", "Bob", testPseudo} {
		if !strings.Contains(text, kept) {
			t.Fatalf("%q lost: %s", kept, text)
		}
	}
	if _, again, _ := anonymizeJSONLines(out, testSub, testPseudo); again {
		t.Fatal("a second pass must be a no-op")
	}
}

func TestNewPseudonymIsRandom(t *testing.T) {
	a, b := newPseudonym(), newPseudonym()
	if a == b || !strings.HasPrefix(a, "anon_") {
		t.Fatalf("got %q and %q", a, b)
	}
}
