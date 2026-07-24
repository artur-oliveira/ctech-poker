import {HAND_CATEGORY_LABELS} from '@/lib/utils';

export type HandRankingEntry = {
  key: string;
  label: string;
  description: string;
  example: string[];
};

// Ordered strongest to weakest — matches how players expect to read a
// rankings reference. `example` cards are illustrative only, independent of
// any board shown elsewhere in the app.
export const HAND_RANKINGS: HandRankingEntry[] = [
  {
    key: 'royal_flush',
    description: 'Sequência do 10 ao Ás, todas do mesmo naipe.',
    example: ['AH', 'KH', 'QH', 'JH', 'TH']
  },
  {
    key: 'straight_flush',
    description: 'Cinco cartas em sequência, todas do mesmo naipe.',
    example: ['9S', '8S', '7S', '6S', '5S']
  },
  {key: 'four_of_a_kind', description: 'Quatro cartas do mesmo valor.', example: ['9C', '9D', '9H', '9S', '4C']},
  {key: 'full_house', description: 'Uma trinca mais um par.', example: ['KH', 'KD', 'KC', '5S', '5D']},
  {
    key: 'flush',
    description: 'Cinco cartas do mesmo naipe, fora de sequência.',
    example: ['AH', 'JH', '8H', '5H', '2H']
  },
  {
    key: 'straight',
    description: 'Cinco cartas em sequência, naipes variados.',
    example: ['9S', '8H', '7D', '6C', '5S']
  },
  {key: 'three_of_a_kind', description: 'Três cartas do mesmo valor.', example: ['7C', '7D', '7H', 'KS', '2H']},
  {key: 'two_pair', description: 'Dois pares de valores diferentes.', example: ['JD', 'JH', '4C', '4D', '9S']},
  {key: 'pair', description: 'Duas cartas do mesmo valor.', example: ['AH', 'AD', '9C', '5D', '2S']},
  {
    key: 'high_card',
    description: 'Nenhuma combinação — vale a carta mais alta.',
    example: ['AH', 'JD', '8C', '5S', '2H']
  }
].map(entry => ({...entry, label: HAND_CATEGORY_LABELS[entry.key] || entry.key}));

// Strongest → weakest as a lookup (0 = royal_flush) so any comparison that
// needs "which category wins" reads off HAND_RANKINGS' order instead of
// re-declaring it (used by HandOutcomeBanner to find the toughest rival hand).
export const HAND_RANK_INDEX: Record<string, number> = Object.fromEntries(
  HAND_RANKINGS.map((entry, index) => [entry.key, index])
);

const RANK_ORDER = '23456789TJQKA';

function rankValue(card: string): number {
  return RANK_ORDER.indexOf(card[0].toUpperCase()) + 2;
}

function nChooseK<T>(items: T[], k: number): T[][] {
  if (k === 0) return [[]];
  if (items.length < k) return [];
  const [head, ...rest] = items;
  return [...nChooseK(rest, k - 1).map(combo => [head, ...combo]), ...nChooseK(rest, k)];
}

type FiveCardScore = { category: string; tiebreak: number[] };

// Ranks one 5-card hand: category (matching HAND_RANKINGS' keys) plus a
// tiebreak vector compared lexicographically, highest first — the same
// values a human would cite when explaining why one hand beats another
// (quads' rank, then kicker; two pair's high pair, low pair, then kicker).
function scoreFiveCards(cards: string[]): FiveCardScore {
  const values = cards.map(rankValue).sort((a, b) => b - a);
  const isFlush = cards.every(c => c[1].toLowerCase() === cards[0][1].toLowerCase());
  const uniqueDesc = [...new Set(values)];
  let straightHigh = 0;
  for (let i = 0; i <= uniqueDesc.length - 5; i++) {
    if (uniqueDesc[i] - uniqueDesc[i + 4] === 4) {
      straightHigh = uniqueDesc[i];
      break;
    }
  }
  // The wheel (A-2-3-4-5): the Ace plays low, so the straight's "high card"
  // for comparison purposes is the 5, not the Ace.
  if (!straightHigh && uniqueDesc.includes(14) && [5, 4, 3, 2].every(v => uniqueDesc.includes(v))) straightHigh = 5;
  const isStraight = straightHigh > 0;

  const counts = new Map<number, number>();
  for (const v of values) counts.set(v, (counts.get(v) || 0) + 1);
  const groups = [...counts.entries()]
    .map(([value, count]) => ({value, count}))
    .sort((a, b) => b.count - a.count || b.value - a.value);
  const kickers = groups.flatMap(g => Array(g.count).fill(g.value));

  if (isStraight && isFlush) return {category: straightHigh === 14 ? 'royal_flush' : 'straight_flush', tiebreak: [straightHigh]};
  if (groups[0].count === 4) return {category: 'four_of_a_kind', tiebreak: kickers};
  if (groups[0].count === 3 && groups[1]?.count === 2) return {category: 'full_house', tiebreak: kickers};
  if (isFlush) return {category: 'flush', tiebreak: values};
  if (isStraight) return {category: 'straight', tiebreak: [straightHigh]};
  if (groups[0].count === 3) return {category: 'three_of_a_kind', tiebreak: kickers};
  if (groups[0].count === 2 && groups[1]?.count === 2) return {category: 'two_pair', tiebreak: kickers};
  if (groups[0].count === 2) return {category: 'pair', tiebreak: kickers};
  return {category: 'high_card', tiebreak: values};
}

function compareScores(a: FiveCardScore, b: FiveCardScore): number {
  const byCategory = HAND_RANK_INDEX[b.category] - HAND_RANK_INDEX[a.category];
  if (byCategory !== 0) return byCategory;
  for (let i = 0; i < Math.max(a.tiebreak.length, b.tiebreak.length); i++) {
    const diff = (a.tiebreak[i] || 0) - (b.tiebreak[i] || 0);
    if (diff !== 0) return diff;
  }
  return 0;
}

// Orders a resolved 5-card hand the way a player reads it — the cards making
// the hand first (a pair's two, a trip's three, ...), highest group first,
// then kickers descending — matching HAND_RANKINGS' own example arrays.
function canonicalOrder(cards: string[]): string[] {
  const groups = new Map<number, string[]>();
  for (const card of cards) {
    const value = rankValue(card);
    const group = groups.get(value) || [];
    group.push(card);
    groups.set(value, group);
  }
  return [...groups.entries()]
    .sort((a, b) => b[1].length - a[1].length || b[0] - a[0])
    .flatMap(([, group]) => group);
}

/** The best 5-card poker hand out of up to 7 cards (2 hole + 5 board), for
 * displaying the actual winning combination rather than just the category
 * name. The server only ever sends a category label (e.g. "two_pair") plus
 * raw hole cards, never the resolved 5-card hand itself, so this evaluates
 * every 5-card subset locally and keeps the strongest one. */
export function bestFiveCardHand(cards: string[]): string[] {
  if (cards.length <= 5) return canonicalOrder(cards);
  let best: { cards: string[]; score: FiveCardScore } | null = null;
  for (const combo of nChooseK(cards, 5)) {
    const score = scoreFiveCards(combo);
    if (!best || compareScores(score, best.score) > 0) best = {cards: combo, score};
  }
  return canonicalOrder(best!.cards);
}
