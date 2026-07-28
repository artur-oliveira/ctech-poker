import {type ClassValue, clsx} from 'clsx';
import {twMerge} from 'tailwind-merge';
import {getPlayerId} from '@/lib/api/client';
import {MOCK_PLAYER_ID, USE_MOCK} from '@/lib/mock';
export {HAND_CATEGORY_LABELS} from './handCategories';

export function cn(...values: ClassValue[]) {
  return twMerge(clsx(values));
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

// Seat CSS position is purely index-driven (Seat.tsx's `seat-${index}` class),
// so the server's seat order must be rotated before rendering, otherwise the
// viewer lands wherever the server happens to seat them instead of always at
// the hero slot (index 0).
export function rotateSeats<T extends { player_id: string }>(seats: T[], viewerId?: string): T[] {
  const at = viewerId ? seats.findIndex(seat => seat.player_id === viewerId) : -1;
  if (at <= 0) return seats;
  return [...seats.slice(at), ...seats.slice(0, at)];
}
