import type {DeckCard} from '@/lib/deckVerify';

// Continues the real dealing sequence from the current board. Hole cards are
// dealt first (two per participant); each street then consumes one burn.
export function rabbitRunout(deck: DeckCard[], dealtPlayers: number, boardSize: number) {
  let next = dealtPlayers * 2;
  const cards: string[] = [];
  if (boardSize >= 3) {
    next += 4;
  } else {
    next++;
    cards.push(...deck.slice(next, next + 3).map(card => card.code));
    next += 3;
  }
  if (boardSize >= 4) {
    next += 2;
  } else {
    next++;
    cards.push(deck[next].code);
    next++;
  }
  if (boardSize < 5) {
    next++;
    cards.push(deck[next].code);
  }
  return cards;
}
