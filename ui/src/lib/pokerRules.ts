import {HAND_CATEGORY_LABELS} from './handCategories.ts';

// Single source of truth for the client-authored buy-in window, in big blinds.
// Both lobby entry points (quick-join in StakesGrid and the private-room
// dialog) post these on `createRoom`, and the "N–M BB" display copy derives
// from them. The server clamps/overrides for public rooms, so these are a
// sane default, not an authority. Previously quick-join sent 20 BB and the
// private dialog sent 40 BB — two floors for the same product (issue #102).
export const BUY_IN_MIN_BB = 40;
export const BUY_IN_MAX_BB = 100;

/** Absolute buy-in window (in chips / cents) for a table at the given big
 * blind, from the shared BB multipliers above. */
export function buyInRange(bigBlind: number): {min: number; max: number} {
  return {min: bigBlind * BUY_IN_MIN_BB, max: bigBlind * BUY_IN_MAX_BB};
}

export type HandRankingEntry = {
  key: string;
  label: string;
  description: string;
  example: string[];
};

// Ordered strongest to weakest, matching how players expect to read a
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
    description: 'Nenhuma combinação, vale a carta mais alta.',
    example: ['AH', 'JD', '8C', '5S', '2H']
  }
].map(entry => ({...entry, label: HAND_CATEGORY_LABELS[entry.key] || entry.key}));

// Strongest → weakest as a lookup (0 = royal_flush) so any comparison that
// needs "which category wins" reads off HAND_RANKINGS' order instead of
// re-declaring it (used by HandOutcomeBanner to find the toughest rival hand).
export const HAND_RANK_INDEX: Record<string, number> = Object.fromEntries(
  HAND_RANKINGS.map((entry, index) => [entry.key, index])
);

// How many of a resolved 5-card hand's cards actually make the combination,
// vs. ride along as kickers, for emphasizing the cards that matter (e.g. a
// pair's 2 cards) over the 3 that don't, instead of showing all 5 as equals.
// Categories where every card participates (straights, flushes, full house)
// claim all 5; high_card claims only the top card, since the rest are pure
// tiebreakers. Paired with canonicalOrder, which already sorts the matching
// group(s) first, so slicing the first N cards off bestFiveCardHand's output
// is enough to isolate them.
export const HAND_MATCH_SIZE: Record<string, number> = {
  royal_flush: 5, straight_flush: 5, four_of_a_kind: 4, full_house: 5, flush: 5,
  straight: 5, three_of_a_kind: 3, two_pair: 4, pair: 2, high_card: 1
};

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
// tiebreak vector compared lexicographically, highest first, the same
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
  
  if (isStraight && isFlush) return {
    category: straightHigh === 14 ? 'royal_flush' : 'straight_flush',
    tiebreak: [straightHigh]
  };
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

// Orders a resolved 5-card hand the way a player reads it: the cards making
// the hand first (a pair's two, a trip's three, ...), highest group first,
// then kickers descending, matching HAND_RANKINGS' own example arrays.
function canonicalOrder(cards: string[]): string[] {
  const groups = new Map<number, string[]>();
  for (const card of cards) {
    const value = rankValue(card);
    const group = groups.get(value) || [];
    group.push(card);
    groups.set(value, group);
  }
  // The wheel (A-2-3-4-5): the Ace plays low, so it reads last, not first.
  // Sort by 1 instead of its usual 14 whenever these are exactly the wheel's
  // five distinct values (only possible here when every group is a single
  // card, i.e. this really is that straight and not some other combination
  // that happens to include an Ace and a 5).
  const isWheel = groups.size === 5 && [14, 5, 4, 3, 2].every(v => groups.has(v));
  return [...groups.entries()]
    .sort((a, b) => b[1].length - a[1].length || (isWheel ? valueForOrder(b[0]) - valueForOrder(a[0]) : b[0] - a[0]))
    .flatMap(([, group]) => group);
  
  function valueForOrder(value: number): number {
    return value === 14 ? 1 : value;
  }
}

function bestOf(cards: string[]): { cards: string[]; score: FiveCardScore } {
  if (cards.length <= 5) return {cards, score: scoreFiveCards(cards)};
  let best: { cards: string[]; score: FiveCardScore } | null = null;
  for (const combo of nChooseK(cards, 5)) {
    const score = scoreFiveCards(combo);
    if (!best || compareScores(score, best.score) > 0) best = {cards: combo, score};
  }
  return best!;
}

/** The best 5-card poker hand out of up to 7 cards (2 hole + 5 board), for
 * displaying the actual winning combination rather than just the category
 * name. The server only ever sends a category label (e.g. "two_pair") plus
 * raw hole cards, never the resolved 5-card hand itself, so this evaluates
 * every 5-card subset locally and keeps the strongest one. */
export function bestFiveCardHand(cards: string[]): string[] {
  if (cards.length <= 5) return canonicalOrder(cards);
  return canonicalOrder(bestOf(cards).cards);
}

/** Same evaluation as bestFiveCardHand, but returns the HAND_RANKINGS
 * category key (e.g. "two_pair") instead of the card list, for labeling a
 * player's hand in a hand-history view, client-side, from raw cards only. */
export function bestHandCategory(cards: string[]): string {
  return bestOf(cards).score.category;
}

/** Compares two up-to-7-card hands (hole cards + board) and reports whether
 * the first beats the second: positive when it does, negative when it
 * loses, 0 on a true tie. Used to tell a folded player whether they'd
 * actually have won had they stayed in, not just that someone else did. */
export function compareHands(cardsA: string[], cardsB: string[]): number {
  return compareScores(bestOf(cardsA).score, bestOf(cardsB).score);
}

/** True only when two hands share the same made combination and the first
 * differing rank is outside that combination. Two different pairs/two-pair
 * values are not a kicker decision merely because their category label is
 * the same. */
export function wasDecidedByKicker(cardsA: string[], cardsB: string[]): boolean {
  if (cardsA.length !== 5 || cardsB.length !== 5) return false;
  const a = scoreFiveCards(cardsA);
  const b = scoreFiveCards(cardsB);
  if (a.category !== b.category) return false;
  const firstDifference = a.tiebreak.findIndex((value, index) => value !== b.tiebreak[index]);
  if (firstDifference < 0) return false;
  return firstDifference >= (HAND_MATCH_SIZE[a.category] ?? a.tiebreak.length);
}
