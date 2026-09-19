package pokerbot

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/reactions"
)

func TestReactionPolicyIsSparseAndUsesValidCatalogShapes(t *testing.T) {
	for _, moment := range []ReactionMoment{ReactionBotWon, ReactionHumanWon, ReactionAllIn, ReactionUncontested} {
		if _, ok := ChooseReaction(moment, &fixedRandom{floats: []float64{.99}, ints: []int{0}}); ok {
			t.Fatalf("moment %d reacted despite high skip roll", moment)
		}
		seen := map[string]bool{}
		for i := 0; i < 100; i++ {
			selection := float64(i) / 100
			choice, ok := ChooseReaction(moment, &fixedRandom{floats: []float64{0, selection}, ints: []int{0}})
			if !ok || !reactions.IsKnown(choice.ID) || choice.Targeted != reactions.IsTargeted(choice.ID) {
				t.Fatalf("moment %d invalid choice %+v, ok=%v", moment, choice, ok)
			}
			seen[choice.ID] = true
		}
		if len(seen) < 4 {
			t.Fatalf("moment %d has too little variety: %v", moment, seen)
		}
	}
}

func TestReactionPolicyCanUsePremiumAndProvocativeReactions(t *testing.T) {
	seen := map[string]bool{}
	for _, moment := range []ReactionMoment{ReactionBotWon, ReactionHumanWon, ReactionAllIn, ReactionUncontested} {
		for i := 0; i < 100; i++ {
			choice, ok := ChooseReaction(moment, &fixedRandom{floats: []float64{0, float64(i) / 100}, ints: []int{0}})
			if ok {
				seen[choice.ID] = true
			}
		}
	}
	for _, id := range []string{"fire", "poop", "rofl", "knife", "turtle", "tomato", "duck"} {
		if !seen[id] {
			t.Fatalf("bot policy never uses %s", id)
		}
	}
}
