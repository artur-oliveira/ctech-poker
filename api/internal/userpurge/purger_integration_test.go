//go:build integration

package userpurge

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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

// After a purge, no table holds the user's sub or display name, and what is
// kept (other players' rows, reports, leaderboard) is anonymized.
func TestPurgeLeavesNoTraceOfTheUser(t *testing.T) {
	ctx := context.Background()
	db := testClient(t)
	objects := newFakeS3()
	valkey := cache.NewMemoryBackend(100)
	p, env := newTestPurger(t, db, objects, valkey, noBlockers)
	seedSharedHand(t, db, env, objects)
	third := "33333333-3333-4333-8333-333333333333"

	// Rows the user owns outright.
	for table, sk := range map[string]string{
		TableAchievementProgress: "sandbox#_progress", TableDailyReward: "streak", TablePlayerSessions: "1",
		TableHandMeta: "hand#h1", TableSandboxPurchases: "p1", TableReactionEntitlements: "fire", TableReactionPurchases: "p2",
		TableCosmeticEntitlements: "deck#casino", TableCosmeticPurchases: "p3", TableCosmeticLoadouts: "l1",
		TableBotCheckContests: "c1", TablePromoRedemptions: "CODE", TableTableEntitlements: "ent#t1",
		TableRecentPlayers: "hand#h1", TableSocialEvents: "e2",
	} {
		put(t, db, env, table, map[string]any{"pk": testU, "sk": sk, "name": "Alice's"})
	}
	for _, table := range []string{TableChatPrefs, TableWalletAlertPrefs} {
		put(t, db, env, table, map[string]any{"pk": testU})
	}
	put(t, db, env, TablePlayerProfiles, map[string]any{"pk": testU, "name": "Alice", "friend_code": "PKR-ALICE"})
	put(t, db, env, TablePlayerSessions, map[string]any{"pk": "buyinguard#" + testU, "sk": "k1"})
	put(t, db, env, TablePokerStats, map[string]any{"pk": "stats#sandbox#" + testU})
	put(t, db, env, TablePokerStats, map[string]any{"pk": "stats#real#" + testU})
	put(t, db, env, TableHandShares, map[string]any{"pk": "tok1", "owner_id": testU, "created_at": 1})
	put(t, db, env, TablePendingCashouts, map[string]any{"pk": "t1#" + testU + "#cashout", "sk": "pending", "player_id": testU,
		"resolved": true, "recorded_at": "2026-10-01T00:00:00Z"})
	// Rows about the user in other players' partitions.
	putAV(t, db, env, TablePlayerProfiles, map[string]types.AttributeValue{"pk": str(testO), "name": str("Bob"),
		"avatar_reporters": &types.AttributeValueMemberSS{Value: []string{testU, third}}})
	put(t, db, env, TablePlayerProfiles, map[string]any{"pk": "avreport#" + testO + "#" + testU, "target_id": testO, "reporter_id": testU})
	put(t, db, env, TablePlayerNotes, map[string]any{"pk": testU, "sk": testO, "note": "Bob tilts"})
	put(t, db, env, TablePlayerNotes, map[string]any{"pk": testO, "sk": testU, "note": "Alice bluffs"})
	put(t, db, env, TableSocialEdges, map[string]any{"pk": testU, "sk": testO, "relationship": "friends"})
	put(t, db, env, TableSocialEdges, map[string]any{"pk": testO, "sk": testU, "relationship": "friends"})
	put(t, db, env, TableSocialEvents, map[string]any{"pk": testO, "sk": "e1", "actor_id": testU})
	put(t, db, env, TableSocialEvents, map[string]any{"pk": testO, "sk": "invite_guard#" + testU + "#r1"})
	put(t, db, env, TableSocialEvents, map[string]any{"pk": testO, "sk": "e3", "actor_id": third})
	put(t, db, env, TablePlayerReports, map[string]any{"pk": testU, "sk": testO + "#abc", "reporter_id": testO,
		"evidence_message": "Alice xingou", "gsi_reporter_pk": testO, "gsi_reporter_sk": "1#abc"})
	put(t, db, env, TablePlayerReports, map[string]any{"pk": testO, "sk": testU + "#def", "reporter_id": testU,
		"details": "Bob trapaceia, diz Alice", "gsi_reporter_pk": testU, "gsi_reporter_sk": "1#def"})
	put(t, db, env, TableLeaderboardStats, map[string]any{"pk": testU, "sk": "stats#sandbox", "player_name": "Alice", "hands_played": 3})
	objects.put("avatars", "av/"+testU+"/1.jpg", []byte("jpg"))
	objects.put("avatars", "up/"+testU+"/2.jpg", []byte("jpg"))
	objects.put("avatars", "av/"+testO+"/1.jpg", []byte("jpg"))
	for _, k := range []string{"poker:presence:table:" + testU, "reaction-owned:" + testU + ":fire", "chatprefs:extra:" + testU} {
		if err := valkey.Set(ctx, k, []byte("1"), 60); err != nil {
			t.Fatal(err)
		}
	}

	for i := range 2 {
		if ack, err := p.Purge(ctx, purgeMsg()); err != nil || ack.Result != erasure.ResultDone {
			t.Fatalf("run %d: %+v %v", i, ack, err)
		}
	}

	for table := range purgeTables {
		for _, it := range scanTable(t, db, env, table) {
			if mentions(it, testU) || mentions(it, "Alice") {
				t.Fatalf("%s still mentions the user: %v", table, it)
			}
		}
	}
	reports := scanTable(t, db, env, TablePlayerReports)
	if len(reports) != 2 {
		t.Fatalf("reports are retained anonymized: want 2, got %d", len(reports))
	}
	for _, it := range reports {
		if _, ok := it["ttl"]; !ok {
			t.Fatalf("a retained report must expire (1 year): %v", it)
		}
	}
	board := scanTable(t, db, env, TableLeaderboardStats)
	if len(board) != 1 || !strings.HasPrefix(stringAttr(board[0], "pk"), "anon_") {
		t.Fatalf("leaderboard row must be re-keyed: %v", board)
	}
	profiles := scanTable(t, db, env, TablePlayerProfiles)
	if len(profiles) != 1 || stringAttr(profiles[0], "name") != "Bob" {
		t.Fatalf("only Bob's profile must remain: %v", profiles)
	}
	if set := profiles[0]["avatar_reporters"].(*types.AttributeValueMemberSS).Value; len(set) != 1 || set[0] != third {
		t.Fatalf("avatar_reporters: %v", set)
	}
	events := scanTable(t, db, env, TableSocialEvents)
	if len(events) != 1 || stringAttr(events[0], "sk") != "e3" {
		t.Fatalf("only the third party's event must remain: %v", events)
	}
	if n := len(scanTable(t, db, env, TableSocialEdges)); n != 0 {
		t.Fatalf("social edges: %d left", n)
	}
	if left := objects.keys("avatars", ""); len(left) != 1 || left[0] != "av/"+testO+"/1.jpg" {
		t.Fatalf("avatars left: %v", left)
	}
	for _, k := range []string{"poker:presence:table:" + testU, "reaction-owned:" + testU + ":fire", "chatprefs:extra:" + testU} {
		if _, found, _ := valkey.Get(ctx, k); found {
			t.Fatalf("valkey key %s survived", k)
		}
	}
}

// A blocker that appeared since the lock stops the purge before anything is erased.
func TestPurgeBlockedErasesNothing(t *testing.T) {
	db := testClient(t)
	blocked := func(context.Context, string) ([]erasure.Blocker, error) {
		return []erasure.Blocker{{Code: BlockerSeatedAtTable}}, nil
	}
	p, env := newTestPurger(t, db, newFakeS3(), cache.NewMemoryBackend(10), blocked)
	put(t, db, env, TablePlayerProfiles, map[string]any{"pk": testU, "name": "Alice"})
	ack, err := p.Purge(context.Background(), purgeMsg())
	if err != nil || ack.Result != erasure.ResultBlocked || len(ack.Blockers) != 1 {
		t.Fatalf("got %+v %v", ack, err)
	}
	if n := len(scanTable(t, db, env, TablePlayerProfiles)); n != 1 {
		t.Fatal("a blocked purge must not erase anything")
	}
}
