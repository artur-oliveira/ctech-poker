package achievements

import (
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval/ref"
)

func parseCard(code string) (deck.Card, bool) {
	if len(code) != 2 {
		return deck.Card{}, false
	}
	ranks := map[byte]deck.Rank{
		'2': deck.Two, '3': deck.Three, '4': deck.Four, '5': deck.Five,
		'6': deck.Six, '7': deck.Seven, '8': deck.Eight, '9': deck.Nine,
		'T': deck.Ten, 'J': deck.Jack, 'Q': deck.Queen, 'K': deck.King, 'A': deck.Ace,
	}
	suits := map[byte]deck.Suit{'c': deck.Clubs, 'd': deck.Diamonds, 'h': deck.Hearts, 's': deck.Spades}
	rank, rok := ranks[code[0]]
	suit, sok := suits[code[1]]
	return deck.Card{Rank: rank, Suit: suit}, rok && sok
}

func playerCards(outcome hand.HandOutcome, playerID string) ([7]deck.Card, bool) {
	var cards [7]deck.Card
	info, ok := outcome.PlayerHands[playerID]
	if !ok || len(outcome.Board) != 5 {
		return cards, false
	}
	codes := []string{info.HoleCards[0], info.HoleCards[1]}
	codes = append(codes, outcome.Board...)
	for i, code := range codes {
		card, valid := parseCard(code)
		if !valid {
			return cards, false
		}
		cards[i] = card
	}
	return cards, true
}

func hasCompleteCards(outcome hand.HandOutcome, playerID string) bool {
	_, ok := playerCards(outcome, playerID)
	return ok
}

func fourToRoyalMissed(cards [7]deck.Card) bool {
	if ref.BestN(cards[:]).Category() == ref.RoyalFlush {
		return false
	}
	for suit := deck.Clubs; suit <= deck.Spades; suit++ {
		seen := map[deck.Rank]bool{}
		for _, card := range cards {
			if card.Suit == suit && card.Rank >= deck.Ten {
				seen[card.Rank] = true
			}
		}
		if len(seen) == 4 {
			return true
		}
	}
	return false
}

func straightFlushWindows(cards []deck.Card) bool {
	for suit := deck.Clubs; suit <= deck.Spades; suit++ {
		seen := map[int]bool{}
		for _, card := range cards {
			if card.Suit == suit {
				seen[int(card.Rank)] = true
				if card.Rank == deck.Ace {
					seen[1] = true
				}
			}
		}
		for low := 1; low <= 10; low++ {
			count := 0
			for rank := low; rank < low+5; rank++ {
				if seen[rank] {
					count++
				}
			}
			if count >= 4 {
				return true
			}
		}
	}
	return false
}

func fourToStraightFlushMissed(cards [7]deck.Card) bool {
	return ref.BestN(cards[:]).Category() < ref.StraightFlush && straightFlushWindows(cards[:])
}

func riverDrawMissed(turnCards []deck.Card, river deck.Card) bool {
	suits := [4]int{}
	ranks := map[int]bool{}
	for _, card := range turnCards {
		suits[card.Suit]++
		ranks[int(card.Rank)] = true
		if card.Rank == deck.Ace {
			ranks[1] = true
		}
	}
	flushDraw := suits[river.Suit] != 4
	hadFourFlush := false
	for _, count := range suits {
		if count == 4 {
			hadFourFlush = true
		}
	}
	openEndedMiss := false
	for low := 1; low <= 10; low++ {
		if ranks[low+1] && ranks[low+2] && ranks[low+3] && ranks[low+4] &&
			int(river.Rank) != low && int(river.Rank) != low+5 {
			openEndedMiss = true
		}
	}
	return (hadFourFlush && flushDraw) || openEndedMiss
}

func stageScore(outcome hand.HandOutcome, playerID string, boardCount int) (ref.Score, bool) {
	info, ok := outcome.PlayerHands[playerID]
	if !ok || len(outcome.Board) < boardCount {
		return 0, false
	}
	codes := []string{info.HoleCards[0], info.HoleCards[1]}
	codes = append(codes, outcome.Board[:boardCount]...)
	cards := make([]deck.Card, len(codes))
	for i, code := range codes {
		card, valid := parseCard(code)
		if !valid {
			return 0, false
		}
		cards[i] = card
	}
	return ref.BestN(cards), true
}

func lostRiverAfterLeadingTurn(outcome hand.HandOutcome, playerID string) bool {
	score, ok := stageScore(outcome, playerID, 4)
	if !ok {
		return false
	}
	for opponent := range outcome.ShowdownResults {
		if opponent == playerID {
			continue
		}
		other, valid := stageScore(outcome, opponent, 4)
		if !valid || other > score {
			return false
		}
	}
	return true
}

func wonRunnerRunner(outcome hand.HandOutcome, playerID string) bool {
	for _, boardCount := range []int{3, 4} {
		score, ok := stageScore(outcome, playerID, boardCount)
		if !ok {
			return false
		}
		behind := false
		for opponent := range outcome.ShowdownResults {
			if opponent == playerID {
				continue
			}
			other, valid := stageScore(outcome, opponent, boardCount)
			if valid && other > score {
				behind = true
				break
			}
		}
		if !behind {
			return false
		}
	}
	final, ok := stageScore(outcome, playerID, 5)
	if !ok {
		return false
	}
	for opponent := range outcome.ShowdownResults {
		if opponent == playerID {
			continue
		}
		other, valid := stageScore(outcome, opponent, 5)
		if valid && other >= final {
			return false
		}
	}
	return true
}

func isNuts(outcome hand.HandOutcome, playerID string) bool {
	cards, ok := playerCards(outcome, playerID)
	if !ok {
		return false
	}
	winnerScore := ref.BestN(cards[:])
	dealt := map[deck.Card]bool{}
	for _, code := range outcome.Board {
		if card, valid := parseCard(code); valid {
			dealt[card] = true
		}
	}
	for _, info := range outcome.PlayerHands {
		for _, code := range info.HoleCards {
			if card, valid := parseCard(code); valid {
				dealt[card] = true
			}
		}
	}
	remaining := make([]deck.Card, 0, 52-len(dealt))
	for suit := deck.Clubs; suit <= deck.Spades; suit++ {
		for rank := deck.Two; rank <= deck.Ace; rank++ {
			card := deck.Card{Rank: rank, Suit: suit}
			if !dealt[card] {
				remaining = append(remaining, card)
			}
		}
	}
	board := make([]deck.Card, 5)
	for i, code := range outcome.Board {
		board[i], _ = parseCard(code)
	}
	for i := 0; i < len(remaining); i++ {
		for j := i + 1; j < len(remaining); j++ {
			candidate := append(append([]deck.Card{}, board...), remaining[i], remaining[j])
			if ref.BestN(candidate) > winnerScore {
				return false
			}
		}
	}
	return true
}
