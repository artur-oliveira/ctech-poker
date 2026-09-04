package app

import (
	"strings"
	"testing"
	"time"

	dynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// itemBytes approximates DynamoDB's own item-size accounting: every attribute
// costs its name plus its value, strings and numbers cost their raw bytes, and
// each map/list element carries a small structural overhead. It is close
// enough to turn "how big is a hand-history row" into a regression test
// without provisioning a real table.
func itemBytes(av dynamotypes.AttributeValue) int {
	switch v := av.(type) {
	case *dynamotypes.AttributeValueMemberS:
		return len(v.Value)
	case *dynamotypes.AttributeValueMemberN:
		return len(v.Value)
	case *dynamotypes.AttributeValueMemberB:
		return len(v.Value)
	case *dynamotypes.AttributeValueMemberBOOL:
		return 1
	case *dynamotypes.AttributeValueMemberNULL:
		return 1
	case *dynamotypes.AttributeValueMemberL:
		total := 0
		for _, elem := range v.Value {
			total += 1 + itemBytes(elem)
		}
		return total
	case *dynamotypes.AttributeValueMemberM:
		total := 0
		for name, elem := range v.Value {
			total += 1 + len(name) + itemBytes(elem)
		}
		return total
	default:
		return 0
	}
}

// worstCaseHandOutcome builds the largest hand-history row a real table can
// produce for the given number of participants: a full two-board runout, every
// hand shown at showdown (so every opponent summary carries name, avatar and
// hole cards) and a complete per-viewer deck proof over all 52 positions,
// which is what fairnessProofFor emits — it iterates the whole shuffled deck
// (internal/engine/hand/snapshot.go).
func worstCaseHandOutcome(players int) (hand.HandOutcome, map[string]string, map[string]string) {
	const hexLen = 64
	ids := make([]string, players)
	names := make(map[string]string, players)
	avatars := make(map[string]string, players)
	outcome := hand.HandOutcome{
		Board:           []string{"Ah", "Kd", "Qc", "2s", "3h"},
		BoardTwo:        []string{"Ah", "Kd", "Qc", "4c", "5d"},
		SmallBlind:      500,
		BigBlind:        1000,
		ServerSeed:      strings.Repeat("f", hexLen),
		CommitHash:      strings.Repeat("f", hexLen),
		RootCommitHash:  strings.Repeat("f", hexLen),
		Payouts:         map[string]int64{},
		Contributions:   map[string]int64{},
		PlayerHands:     map[string]hand.PlayerHandInfo{},
		ShowdownResults: map[string]hand.ShowdownResult{},
		FairnessProofs:  map[string]hand.FairnessProof{},
	}
	for i := range players {
		id := "01M1HABKV5Z90SGHZ63K44EGH" + string(rune('A'+i))
		ids[i] = id
		names[id] = "player-with-a-long-display-name"
		avatars[id] = "https://cdn.example.com/av/" + strings.Repeat("a", 40) + ".webp"
		outcome.Payouts[id] = 1000
		outcome.Contributions[id] = 1000
		outcome.PlayerHands[id] = hand.PlayerHandInfo{HoleCards: [2]string{"Ah", "Kh"}, Revealed: true}
		outcome.ShowdownResults[id] = hand.ShowdownResult{Category: "full_house"}
		proof := hand.FairnessProof{
			RevealedCardSalts:    make(map[int]hand.RevealedSaltView, 52),
			UnrevealedCardHashes: make(map[int]string, 52),
		}
		for pos := range 52 {
			proof.RevealedCardSalts[pos] = hand.RevealedSaltView{Card: "Ah", SaltHex: strings.Repeat("f", hexLen)}
			proof.UnrevealedCardHashes[pos] = strings.Repeat("f", hexLen)
		}
		outcome.FairnessProofs[id] = proof
	}
	outcome.Participants = ids
	outcome.Winners = ids[:1]
	return outcome, names, avatars
}

// TestHandHistoryItemWriteUnits is the measurement acceptance criterion of
// #200: it pins the worst-case per-participant hand-history row size and the
// WCU one completed hand costs at 2, 6 and 9 players, so a field added to
// sessionlog.HandItem cannot quietly multiply the history table's write bill
// by nine.
//
// Batching the writes (sessionlog.Store.RecordHands) does not change these
// numbers — BatchWriteItem bills per item, exactly like the PutItems it
// replaces — it only collapses nine round trips into one. The WCU ceiling is
// a property of the item shape alone, which is why this test guards the shape
// rather than the call.
func TestHandHistoryItemWriteUnits(t *testing.T) {
	// perRowWCUCeiling covers what the worst-case row measures today — ~9.7 KB
	// (10 WCU), dominated by the 52-position deck proof: 52 revealed salts
	// plus 52 unrevealed hashes at 64 hex chars each, ~8.5 KB of the row on
	// its own — with headroom for a couple of new fields. A hand's total is
	// this times the participant count, since DynamoDB rounds every item up to
	// a whole WCU independently: today's nine-handed worst case is 90 WCU.
	const perRowWCUCeiling = 12
	for _, players := range []int{2, 6, 9} {
		outcome, names, avatars := worstCaseHandOutcome(players)
		handWCU, maxBytes := 0, 0
		for _, id := range outcome.Participants {
			item := handItemForWithAvatars(outcome, id, names, avatars)
			item.PK, item.TableID, item.HandID, item.CurrencyMode, item.EndedAt =
				id, "01M1HABKV5Z90SGHZ63K44EGHB", "01M1HABKV5Z90SGHZ63K44EGHC", "sandbox", time.Now().UnixMilli()
			encoded, err := dynamo.Encode(item)
			if err != nil {
				t.Fatalf("encode hand item: %v", err)
			}
			size := 0
			for name, av := range encoded {
				size += len(name) + itemBytes(av)
			}
			if size > maxBytes {
				maxBytes = size
			}
			wcu := (size + 1023) / 1024
			if wcu > perRowWCUCeiling {
				t.Fatalf("%d-player hand: one participant row is %d bytes (%d WCU), over the %d WCU ceiling",
					players, size, wcu, perRowWCUCeiling)
			}
			handWCU += wcu
		}
		t.Logf("%d players: worst row %d bytes; %d WCU for the whole hand", players, maxBytes, handWCU)
	}
}
