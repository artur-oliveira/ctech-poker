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
  {key: 'royal_flush', description: 'Sequência do 10 ao Ás, todas do mesmo naipe.', example: ['AS', 'KS', 'QS', 'JS', 'TS']},
  {key: 'straight_flush', description: 'Cinco cartas em sequência, todas do mesmo naipe.', example: ['9H', '8H', '7H', '6H', '5H']},
  {key: 'four_of_a_kind', description: 'Quatro cartas do mesmo valor.', example: ['9C', '9D', '9H', '9S', '4C']},
  {key: 'full_house', description: 'Uma trinca mais um par.', example: ['KH', 'KD', 'KC', '5S', '5D']},
  {key: 'flush', description: 'Cinco cartas do mesmo naipe, fora de sequência.', example: ['AH', 'JH', '8H', '5H', '2H']},
  {key: 'straight', description: 'Cinco cartas em sequência, naipes variados.', example: ['9S', '8H', '7D', '6C', '5S']},
  {key: 'three_of_a_kind', description: 'Três cartas do mesmo valor.', example: ['7C', '7D', '7H', 'KS', '2H']},
  {key: 'two_pair', description: 'Dois pares de valores diferentes.', example: ['JD', 'JH', '4C', '4D', '9S']},
  {key: 'pair', description: 'Duas cartas do mesmo valor.', example: ['AH', 'AD', '9C', '5D', '2S']},
  {key: 'high_card', description: 'Nenhuma combinação — vale a carta mais alta.', example: ['AH', 'JD', '8C', '5S', '2H']}
].map(entry => ({...entry, label: HAND_CATEGORY_LABELS[entry.key] || entry.key}));
