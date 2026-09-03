import {type ClassValue, clsx} from 'clsx';
import {twMerge} from 'tailwind-merge';
import {getPlayerId} from '@/lib/api/client';
import {MOCK_PLAYER_ID, USE_MOCK} from '@/lib/mockConfig';

export {HAND_CATEGORY_LABELS} from './handCategories';

export function cn(...values: ClassValue[]) {
  return twMerge(clsx(values));
}

/** True on devices with a real hover-capable, fine pointer (mouse/trackpad),
 * false on touch — gates hover-to-open affordances so tap doesn't misfire them. */
export function isHoverCapable(): boolean {
  return typeof window !== 'undefined' && window.matchMedia('(hover: hover) and (pointer: fine)').matches;
}

/** True when the key press belongs to a text field (chat input, etc.), never the raise slider. */
export function isTypingTarget(target: EventTarget | null) {
  return target instanceof HTMLElement && !!target.closest('input:not([type=range]), textarea, select, [contenteditable]');
}

export function isPlainKey(event: KeyboardEvent) {
  return !event.metaKey && !event.ctrlKey && !event.altKey && !event.repeat && !isTypingTarget(event.target);
}

export const ACHIEVEMENT_LABELS: Record<string, string> = {
  wins: "Vitórias",
  hands_played: "Mãos Jogadas",
  comeback: "De Volta ao Jogo",
  bluff: "Mestre do Blefe",
  survivor: "Sobrevivente",
  
  looser: "Não Foi Dessa Vez",
  almost_winner: "Por Um Detalhe",
  tied: "Dividindo o Pote",
  
  bad_beat: "Que Azar!",
  cooler: "Sem Escapatória",
  cracked_aces: "Maldito Ás",
  fallen_king: "KKKKKKKKK",
  
  giant_slayer: "Virou o Jogo",
  showdown_warrior: "Paga pra Ver",
  all_in: "Tudo ou Nada",
  
  sandbox_chips_earned: "Montanha de Fichas",
  real_money_earned: "Banca de Verdade",
  
  won_with_pocket_pair: "Par na Mão",
  won_full_table: "Dono da Mesa",
  won_heads_up: "Duelo Vencido",
  won_with_nuts: "Mão Imbatível",
  won_runner_runner: "Turn e River Perfeitos",
  three_bet_won_no_showdown: "Pressão no 3-bet",
  beat_pocket_aces: "Quebrou os Ases",
  beat_trips_or_better: "Passou por Cima",
  first_hand_allin_win: "Chegou Chegando",
  same_pocket_pair_streak: "Par de Estimação",
  
  folded_streak: "Paciência de Pedra",
  all_in_blind: "All-in às Cegas",
  blind_magic: "Magia das Cartas",
  no_rush: "Sem pressa",
  four_to_royal_missed: "Quase Royal",
  four_to_straight_flush_missed: "Quase Straight Flush",
  paid_river_draw_missed: "Pagou e Não Veio",
  lost_river_after_leading_turn: "Perdeu no River",
  lost_straight_flush_to_royal: "Azar Histórico",
  
  win_category_high_card: "Carta Alta",
  win_category_pair: "Um Par",
  win_category_two_pair: "Dois Pares",
  win_category_three_of_a_kind: "Trinca",
  win_category_straight: "Sequência",
  win_category_flush: "Flush",
  win_category_full_house: "Full House",
  win_category_four_of_a_kind: "Quadra",
  win_category_straight_flush: "Straight Flush",
  win_category_royal_flush: "Royal Flush",
};

// The single answer to "who is looking at this screen": the profile's
// user_id (matches seat.player_id / current_player_id server-side) in prod,
// the fixed mock player in mock mode. NOT decodeIdToken: that only ever
// returns username/first_name/last_name, never `sub`. Using it here silently
// left every viewer comparison undefined.
export function getViewerId(): string | undefined {
  if (USE_MOCK) return MOCK_PLAYER_ID;
  return getPlayerId() ?? undefined;
}

// Player IDs are opaque (OIDC sub UUIDs in prod) and carry no name. The
// display name comes from the player's persisted profile (GET /players/me),
// broadcast to seats by the table actor, so callers pass whatever name they
// already resolved from a SeatView. Until it arrives, `name` is undefined and
// the seat shows as a not-yet-named placeholder.
export function playerName(id: string, viewerId?: string, name?: string): string {
  if (viewerId && id === viewerId) return 'Você';
  return name || 'Visitante';
}

// Shared avatar-fallback initials: first + last name initial, uppercased.
export function initials(name?: string): string {
  if (!name) return '?';
  const parts = name.trim().split(/\s+/);
  return ((parts[0]?.[0] || '') + (parts.length > 1 ? parts[parts.length - 1][0] : '')).toUpperCase() || '?';
}

// Relative pt-BR phrasing for an epoch-milliseconds instant, past or future
// ("há 3 dias", "em 6 dias"). Intl.RelativeTimeFormat does the wording, so
// there is no hand-written plural table to keep in sync.
const RELATIVE_UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 3600_000],
  ['month', 30 * 24 * 3600_000],
  ['day', 24 * 3600_000],
  ['hour', 3600_000],
  ['minute', 60_000]
];

export function relativeTime(instantMs: number, nowMs = Date.now()): string {
  const delta = instantMs - nowMs;
  const format = new Intl.RelativeTimeFormat('pt-BR', {numeric: 'auto'});
  for (const [unit, size] of RELATIVE_UNITS) {
    if (Math.abs(delta) >= size) return format.format(Math.round(delta / size), unit);
  }
  return format.format(Math.round(delta / 1000), 'second');
}

// Seat CSS position is purely index-driven (Seat.tsx's `seat-${index}` class),
// so the server's seat order must be rotated before rendering, otherwise the
// viewer lands wherever the server happens to seat them instead of always at
// the hero slot (index 0).
export function rotateSeats<T extends { player_id: string }>(seats: T[], viewerId?: string): T[] {
  const at = viewerId ? seats.findIndex(seat => seat.player_id === viewerId) : -1;
  if (at <= 0) return seats;
  return [...seats.slice(at), ...seats.slice(0, at)];
}
