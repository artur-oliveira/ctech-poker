package userexport

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Every poker_* table the CDK declares must be exported or excluded on purpose.
func TestInventoryClassifiesEveryTable(t *testing.T) {
	re := regexp.MustCompile(`'(poker_[a-z_]+)'`)
	for _, file := range []string{"../../../cdk/lib/dynamodb-stack.ts", "../../../cdk/lib/constants.ts"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			if _, ok := inventory[m[1]]; !ok {
				t.Errorf("%s declares %s, which userexport does not classify: export it (add a source) or exclude it with a reason", file, m[1])
			}
		}
	}
}

// The inventory and the sources agree: exported tables have a source, and only
// exported tables do.
func TestInventoryMatchesSources(t *testing.T) {
	have := map[string]bool{}
	for _, s := range sources {
		have[s.table] = true
		if v := inventory[s.table]; !strings.HasPrefix(v, "exported") {
			t.Errorf("%s has a source but is classified %q", s.table, v)
		}
	}
	for table, v := range inventory {
		switch {
		case strings.HasPrefix(v, "exported") && !have[table]:
			t.Errorf("%s is classified exported but has no source", table)
		case !strings.HasPrefix(v, "exported") && !strings.HasPrefix(v, "excluded: "):
			t.Errorf("%s: classification must start with exported or \"excluded: <reason>\", got %q", table, v)
		}
	}
}

type fakeDB struct {
	byTable map[string][]map[string]types.AttributeValue
	queries []string
}

func (f *fakeDB) Query(_ context.Context, in *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	name := aws.ToString(in.TableName)
	pk := ""
	if v, ok := in.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS); ok {
		pk = v.Value
	}
	f.queries = append(f.queries, name+"|"+pk)
	var out []map[string]types.AttributeValue
	for _, it := range f.byTable[name] {
		if p, _ := it["pk"].(*types.AttributeValueMemberS); in.IndexName != nil || (p != nil && p.Value == pk) {
			out = append(out, it)
		}
	}
	return &dynamodb.QueryOutput{Items: out}, nil
}

func s(v string) types.AttributeValue { return &types.AttributeValueMemberS{Value: v} }

func TestPartHasOwnDataAndNoOneElsesPII(t *testing.T) {
	db := &fakeDB{byTable: map[string][]map[string]types.AttributeValue{
		"test_poker_player_profiles": {{"pk": s("alice"), "display_name": s("Alice")}},
		"test_poker_player_hands": {{"pk": s("alice"), "sk": s("real#h1"), "hole_cards": &types.AttributeValueMemberSS{Value: []string{"Ah", "Kd"}},
			"commit_hash": s("fairness-proof"),
			"opponents": &types.AttributeValueMemberL{Value: []types.AttributeValue{
				&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{"player_id": s("bob"), "name": s("Bob"), "avatar_url": s("https://x/bob.png")}},
				&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{"player_id": s("alice"), "name": s("Alice")}},
			}}}},
		"test_poker_player_poker_stats": {{"pk": s("stats#real#alice"), "hands": &types.AttributeValueMemberN{Value: "7"}}},
		"test_poker_chat_prefs":         {{"pk": s("someone-else"), "muted": s("x")}},
	}}
	e := New(db, "test")
	e.now = func() time.Time { return time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC) }
	data, err := e.Part(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		files[f.Name] = string(b)
	}
	for _, want := range []string{"player_profiles.json", "player_hands.json", "player_poker_stats.json", "README.txt"} {
		if _, ok := files[want]; !ok {
			t.Errorf("missing %s (have %d files)", want, len(files))
		}
	}
	if _, ok := files["chat_prefs.json"]; ok {
		t.Error("another person's row leaked into the part")
	}
	hands := files["player_hands.json"]
	if strings.Contains(hands, "Bob") || strings.Contains(hands, "bob.png") {
		t.Errorf("an opponent's name or avatar is in the export: %s", hands)
	}
	if !strings.Contains(hands, "bob") || !strings.Contains(hands, "Alice") || !strings.Contains(hands, "fairness-proof") {
		t.Errorf("opaque opponent id, own name and the fairness proof must stay: %s", hands)
	}
	if !strings.Contains(strings.Join(db.queries, ","), "test_poker_player_poker_stats|stats#real#alice") {
		t.Errorf("stats partitions not queried: %v", db.queries)
	}
}

func TestScrubDropsSecretsAtAnyDepth(t *testing.T) {
	got := scrub(map[string]any{"a": 1, "password_hash": "x", "n": map[string]any{"api_token": "y", "ok": true}, "l": []any{map[string]any{"cpf": "1"}}}, "me").(map[string]any)
	if _, ok := got["password_hash"]; ok || got["n"].(map[string]any)["api_token"] != nil || len(got["l"].([]any)[0].(map[string]any)) != 0 || got["n"].(map[string]any)["ok"] != true {
		t.Fatalf("scrub=%v", got)
	}
}
