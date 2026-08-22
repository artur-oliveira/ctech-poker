import {HAND_CATEGORY_LABELS} from './handCategories';
import {HAND_MATCH_SIZE, HAND_RANK_INDEX, bestFiveCardHand} from './pokerRules';

const RANK_ORDER = '23456789TJQKA';

function suitOf(card: string) {
  return card[1]?.toLowerCase();
}

function rankValueOf(card: string) {
  return RANK_ORDER.indexOf(card[0]?.toUpperCase());
}

// A rough texture read, not a solver: any 3+ board cards sharing a suit means
// some opponent holding two of that suit already has a flush, or is one card
// from it.
export function boardHasFlushDraw(board: string[]): boolean {
  const counts = new Map<string, number>();
  for (const card of board) {
    const suit = suitOf(card);
    counts.set(suit, (counts.get(suit) || 0) + 1);
  }
  return [...counts.values()].some(count => count >= 3);
}

// Same rough-texture spirit: any two board ranks close enough to share a
// straight (within a 4-gap window) means a connected opponent hand is live.
export function boardHasStraightDraw(board: string[]): boolean {
  const values = [...new Set(board.map(rankValueOf))].filter(value => value >= 0).sort((a, b) => a - b);
  for (let i = 0; i < values.length; i++) {
    for (let j = i + 1; j < values.length; j++) {
      if (values[j] - values[i] <= 4) return true;
    }
  }
  return false;
}

// The specific cards that make up the viewer's own category: the best
// 5-card read of their known hole cards + board, trimmed to however many of
// those cards actually participate in the category (HAND_MATCH_SIZE) rather
// than dragging kickers along.
export function matchingCards(category: string | undefined, holeCards: string[], board: string[]): string[] {
  if (!category) return [];
  const known = [...holeCards, ...board].filter(card => card && card.toLowerCase() !== 'back');
  if (known.length === 0) return known;
  const best = bestFiveCardHand(known);
  return best.slice(0, HAND_MATCH_SIZE[category] ?? best.length);
}

// One plain-language line: what the viewer has, plus whatever the board
// itself threatens for a category weaker than that threat (a made flush
// doesn't need a flush-draw warning; a pair does). Client-side only, reusing
// data already sent to the viewer for their own seat.
export function explainEquity(category: string | undefined, board: string[]): string {
  if (!category) return 'Aguardando cartas suficientes na mesa para avaliar a mão.';
  const label = (HAND_CATEGORY_LABELS[category] || category).toLowerCase();
  const categoryRank = HAND_RANK_INDEX[category] ?? HAND_RANK_INDEX.high_card;
  const dangers: string[] = [];
  if (categoryRank > HAND_RANK_INDEX.flush && boardHasFlushDraw(board)) dangers.push('um flush');
  if (categoryRank > HAND_RANK_INDEX.straight && boardHasStraightDraw(board)) dangers.push('uma sequência');
  if (!dangers.length) return `Você tem ${label}. A mesa não deixa flush nem sequência óbvios para os adversários.`;
  return `Você tem ${label}. A mesa deixa ${dangers.join(' ou ')} em formação para os adversários.`;
}
