//go:build integration

package userpurge

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

// Other players keep their history of a hand they shared with the erased
// user, under one pseudonym and without the user's name, avatar or chat.
// Purge runs twice: a redelivered message must finish cleanly.
func TestPurgeAnonymizesSharedHands(t *testing.T) {
	db := testClient(t)
	objects := newFakeS3()
	p, env := newTestPurger(t, db, objects, cache.NewMemoryBackend(10), noBlockers)
	seedSharedHand(t, db, env, objects)

	for i := range 2 {
		ack, err := p.Purge(context.Background(), purgeMsg())
		if err != nil || ack.Result != erasure.ResultDone {
			t.Fatalf("run %d: %+v %v", i, ack, err)
		}
		if i == 0 && ack.Counts[TablePlayerHands+"_erased"] != 1 {
			t.Fatalf("counts: %+v", ack.Counts)
		}
	}

	hands := scanTable(t, db, env, TablePlayerHands)
	if len(hands) != 1 {
		t.Fatalf("want only the opponent's hand row, got %d", len(hands))
	}
	var bobs sessionlog.HandItem
	if err := attributevalue.UnmarshalMap(hands[0], &bobs); err != nil {
		t.Fatal(err)
	}
	pseudo := bobs.Opponents[0].PlayerID
	if !strings.HasPrefix(pseudo, "anon_") || bobs.Opponents[0].Name != "" || bobs.Opponents[0].AvatarURL != "" || len(bobs.Opponents[0].HoleCards) != 2 {
		t.Fatalf("opponent summary: %+v", bobs.Opponents[0])
	}
	logRow := scanTable(t, db, env, TableActionLog)[0]
	if stringAttr(logRow, "player_id") != pseudo {
		t.Fatalf("one pseudonym per run: hand row %s, action log %s", pseudo, stringAttr(logRow, "player_id"))
	}
	if _, ok := logRow["message"]; ok {
		t.Fatal("the erased user's chat survived")
	}
	if !mentions(logRow, "Bob") {
		t.Fatal("the opponent's seat must stay")
	}
	for _, table := range []string{TablePlayerHands, TableActionLog, TableTableStateHistory, TableTableHighlights,
		TableHandReveals, TableHandRevealPayments, TableRecentPlayers, TableMatchups} {
		for _, it := range scanTable(t, db, env, table) {
			if mentions(it, testU) || mentions(it, "Alice") {
				t.Fatalf("%s still mentions the user: %v", table, it)
			}
		}
	}
	for _, table := range []string{TableHandRevealPayments, TableMatchups} {
		if n := len(scanTable(t, db, env, table)); n != 0 {
			t.Fatalf("%s: want erased, %d rows left", table, n)
		}
	}
	archived := string(objects.objects["archive/t1/h1/1-e1.jsonl"])
	if strings.Contains(archived, testU) || strings.Contains(archived, "Alice") || !strings.Contains(archived, pseudo) || !strings.Contains(archived, "Bob") {
		t.Fatalf("archive: %s", archived)
	}
}
