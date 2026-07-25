// Presentation metadata for the achievements catalog — descriptions and
// illustrative playing cards per achievement key. Split out from utils.ts to
// avoid a cycle: pokerRules.ts already imports HAND_CATEGORY_LABELS from
// utils.ts, so this file (not utils.ts) is what pulls in HAND_RANKINGS.
import {HAND_RANKINGS} from '@/lib/pokerRules';
import {ACHIEVEMENT_LABELS, HAND_CATEGORY_LABELS} from '@/lib/utils';

const WIN_CATEGORY_PREFIX = 'win_category_';

const DESCRIPTIONS: Record<string, string> = {
  wins: 'Toda mão vencida conta um ponto.',
  hands_played: 'Toda mão jogada soma, ganhando ou perdendo.',
  comeback: 'Foi all-in, ficou por um fio e ainda assim virou a mesa.',
  bluff: 'Ganhou sem showdown com a mão mais fraca — blefe puro, sem carta na manga.',
  survivor: 'Jogou na mesma mesa por muitas mãos seguidas, sem sair.',
  looser: 'Perdeu no showdown. Faz parte do jogo — ninguém vence sempre.',
  almost_winner: 'Perdeu para alguém com a mesma mão, só que um pouco mais forte.',
  tied: 'Empatou no showdown e dividiu o pote com o adversário.',
  bad_beat: 'Perdeu com trinca ou mais forte — uma mão ótima, mas não o suficiente.',
  cooler: 'Perdeu com full house ou mais forte — quase impossível fugir dessa.',
  cracked_aces: 'Foi ao showdown com par de ases e ainda assim perdeu.',
  fallen_king: 'Foi ao showdown com par de reis e ainda assim perdeu.',
  giant_slayer: 'Ganhou all-in contra um adversário com stack maior que o seu.',
  showdown_warrior: 'Chegou ao showdown. Não teve medo de ver as cartas do adversário.'
};

const EXAMPLES: Record<string, string[]> = {
  wins: ['AH', 'AD'],
  hands_played: ['KC', '7D'],
  comeback: ['5H', '5C'],
  bluff: ['7C', '2D'],
  survivor: ['JD', '8S'],
  looser: ['KH', 'KD'],
  almost_winner: ['QH', 'QD'],
  tied: ['TH', 'TS'],
  bad_beat: ['8H', '8D', '8C'],
  cooler: ['KH', 'KD', 'KC', '5S', '5D'],
  cracked_aces: ['AH', 'AS'],
  fallen_king: ['KD', 'KC'],
  giant_slayer: ['2H', '7D'],
  showdown_warrior: ['JH', 'TD']
};

export function achievementLabel(key: string): string {
  if (key.startsWith(WIN_CATEGORY_PREFIX)) {
    const category = key.slice(WIN_CATEGORY_PREFIX.length);
    return HAND_CATEGORY_LABELS[category] || category;
  }
  return ACHIEVEMENT_LABELS[key] || key.replaceAll('_', ' ');
}

export function achievementDescription(key: string): string {
  if (key.startsWith(WIN_CATEGORY_PREFIX)) {
    const category = key.slice(WIN_CATEGORY_PREFIX.length);
    return `Venceu no showdown com ${(HAND_CATEGORY_LABELS[category] || category).toLowerCase()}.`;
  }
  return DESCRIPTIONS[key] || '';
}

export function achievementExample(key: string): string[] {
  if (key.startsWith(WIN_CATEGORY_PREFIX)) {
    const category = key.slice(WIN_CATEGORY_PREFIX.length);
    return HAND_RANKINGS.find(h => h.key === category)?.example || [];
  }
  return EXAMPLES[key] || [];
}
