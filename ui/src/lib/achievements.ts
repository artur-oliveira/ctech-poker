// Presentation metadata for the achievements catalog: descriptions and
// illustrative playing cards per achievement key. Split out from utils.ts to
// avoid a cycle: pokerRules.ts already imports HAND_CATEGORY_LABELS from
// utils.ts, so this file (not utils.ts) is what pulls in HAND_RANKINGS.
import type {WalletMode} from '@/lib/api/player';
import {HAND_RANKINGS} from '@/lib/pokerRules';
import {ACHIEVEMENT_LABELS, HAND_CATEGORY_LABELS} from '@/lib/utils';

const WIN_CATEGORY_PREFIX = 'win_category_';

const DESCRIPTIONS: Record<string, string> = {
  wins: 'Toda mão vencida conta um ponto.',
  hands_played: 'Toda mão jogada soma, ganhando ou perdendo.',
  comeback: 'Foi all-in, ficou por um fio e ainda assim virou a mesa.',
  bluff: 'Ganhou sem showdown com a mão mais fraca, blefe puro, sem carta na manga.',
  survivor: 'Jogou na mesma mesa por muitas mãos seguidas, sem sair.',
  looser: 'Perdeu no showdown. Faz parte do jogo, ninguém vence sempre.',
  almost_winner: 'Perdeu para alguém com a mesma mão, só que um pouco mais forte.',
  tied: 'Empatou no showdown e dividiu o pote com o adversário.',
  bad_beat: 'Perdeu com trinca ou mais forte, uma mão ótima, mas não o suficiente.',
  cooler: 'Perdeu com full house ou mais forte, quase impossível fugir dessa.',
  cracked_aces: 'Foi ao showdown com par de ases e ainda assim perdeu.',
  fallen_king: 'Foi ao showdown com par de reis e ainda assim perdeu.',
  giant_slayer: 'Ganhou all-in contra um adversário com stack maior que o seu.',
  showdown_warrior: 'Chegou ao showdown. Não teve medo de ver as cartas do adversário.',
  all_in: 'Empurrou todas as fichas para o meio da mesa.',
  sandbox_chips_earned: 'Soma de todas as fichas de sandbox que você já levou dos potes.',
  real_money_earned: 'Soma de todo o dinheiro real que você já levou dos potes.',
  won_with_pocket_pair: 'Venceu uma mão que começou com um par na mão.',
  won_full_table: 'Venceu com a mesa cheia, contra o máximo de adversários.',
  won_heads_up: 'Venceu no mano a mano, só você e um adversário na mesa.',
  won_with_nuts: 'Venceu com a melhor mão possível para aquele board.',
  won_runner_runner: 'Precisava do turn e do river, e os dois vieram.',
  three_bet_won_no_showdown: 'Deu o terceiro aumento e levou o pote sem mostrar as cartas.',
  beat_pocket_aces: 'Ganhou de um adversário que estava com par de ases.',
  beat_trips_or_better: 'Ganhou de um adversário com trinca ou mais forte.',
  first_hand_allin_win: 'Foi all-in na primeira mão da mesa e venceu.',
  same_pocket_pair_streak: 'Venceu mãos seguidas com o mesmo par na mão.',
  folded_streak: 'Passou muitas mãos seguidas sem colocar uma ficha no pote.',
  four_to_royal_missed: 'Chegou a quatro cartas do royal flush e a quinta não veio.',
  four_to_straight_flush_missed: 'Chegou a quatro cartas do straight flush e a quinta não veio.',
  paid_river_draw_missed: 'Pagou para ver o river atrás de um projeto que não fechou.',
  lost_river_after_leading_turn: 'Estava na frente no turn e perdeu no river.',
  lost_straight_flush_to_royal: 'Perdeu com straight flush para um royal flush.',
  all_in_blind: 'Foi all-in sem ver nenhuma das suas cartas.',
  blind_magic: 'Venceu a mão sem ver nenhuma das suas cartas.',
  no_rush: 'Deixou o relógio correr e usou seu tempo extra para decidir.'
};

// The two "earned" counters are wallet-scoped by definition: the server keeps
// progress per mode, so the sandbox total is meaningless under real money and
// vice-versa. Everything else is tracked in both modes.
const MODE_ONLY: Record<string, WalletMode> = {
  real_money_earned: 'real',
  sandbox_chips_earned: 'sandbox'
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
  showdown_warrior: ['JH', 'TD'],
  all_in: ['AS', 'KS'],
  won_with_pocket_pair: ['9H', '9S'],
  won_full_table: ['AC', 'KC'],
  won_heads_up: ['AD', 'TD'],
  won_with_nuts: ['JH', 'TH'],
  won_runner_runner: ['4C', '5C'],
  three_bet_won_no_showdown: ['AH', 'QS'],
  beat_pocket_aces: ['7S', '8S'],
  beat_trips_or_better: ['6H', '7H', '8H', '9H', 'TH'],
  first_hand_allin_win: ['AC', 'AS'],
  same_pocket_pair_streak: ['8C', '8D'],
  folded_streak: ['3S', '8D'],
  four_to_royal_missed: ['TS', 'JS', 'QS', 'KS'],
  four_to_straight_flush_missed: ['5C', '6C', '7C', '8C'],
  paid_river_draw_missed: ['AH', '4H'],
  lost_river_after_leading_turn: ['KS', 'QS'],
  lost_straight_flush_to_royal: ['9H', 'TH', 'JH', 'QH', 'KH'],
  all_in_blind: ['back', 'back'],
  blind_magic: ['back', 'back'],
  no_rush: ['QS', 'JS']
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

// Undefined means "counts in both wallets"; a mode means the catalog entry only
// belongs on screen while that wallet is selected.
export function achievementWalletMode(key: string): WalletMode | undefined {
  return MODE_ONLY[key];
}

export function achievementExample(key: string): string[] {
  if (key.startsWith(WIN_CATEGORY_PREFIX)) {
    const category = key.slice(WIN_CATEGORY_PREFIX.length);
    return HAND_RANKINGS.find(h => h.key === category)?.example || [];
  }
  return EXAMPLES[key] || [];
}

// Most achievements count events; no_rush counts milliseconds, so its raw
// thresholds ("2.592.000.000") are unreadable. The unit lives here rather
// than in the catalog because it is purely a presentation concern.
const DURATION_UNITS: { ms: number; one: string; many: string }[] = [
  {ms: 2_592_000_000, one: 'mês', many: 'meses'},
  {ms: 604_800_000, one: 'semana', many: 'semanas'},
  {ms: 86_400_000, one: 'dia', many: 'dias'},
  {ms: 3_600_000, one: 'hora', many: 'horas'},
  {ms: 60_000, one: 'minuto', many: 'minutos'},
];

function formatDurationMs(value: number): string {
  for (const unit of DURATION_UNITS) {
    if (value >= unit.ms) {
      const count = Math.floor(value / unit.ms);
      return `${count.toLocaleString('pt-BR')} ${count === 1 ? unit.one : unit.many}`;
    }
  }
  const seconds = Math.max(0, Math.floor(value / 1000));
  return `${seconds.toLocaleString('pt-BR')} ${seconds === 1 ? 'segundo' : 'segundos'}`;
}

const DURATION_KEYS = new Set(['no_rush']);

/** How this achievement's counts and thresholds are written out. */
export function achievementValueFormat(key: string): (value: number) => string {
  return DURATION_KEYS.has(key) ? formatDurationMs : value => value.toLocaleString('pt-BR');
}
