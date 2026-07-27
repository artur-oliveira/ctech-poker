import type {Page} from './client';
import {apiClient} from './client';
import type {DeckVariantId} from '../cardVariants';

export type WalletMode = 'sandbox' | 'real';

export interface PlayerProfile {
  user_id: string;
  name?: string;
  wallet_mode: WalletMode;
  poker_terms_accepted: boolean;
  poker_terms_accepted_at?: string;
  game_balance?: number;
  sandbox_balance?: number;
  // Not sent by the backend yet — reserved so the deck color variant can be
  // wired in without another PlayerProfile shape change.
  deck_variant?: DeckVariantId;
  showcase_public: boolean;
  featured_achievements?: string[];
}

export async function getMe() {
  return (await apiClient.get<PlayerProfile>('/v1.0/players/me', {silentError: true})).data;
}

export async function acceptPokerTerms() {
  return (await apiClient.post<PlayerProfile>('/v1.0/players/me/terms/accept', {}, {silentError: true})).data;
}

export async function updateMe(input: {
  name?: string;
  wallet_mode?: WalletMode;
  deck_variant?: DeckVariantId;
  showcase_public?: boolean;
  featured_achievements?: string[];
}) {
  return (await apiClient.post<PlayerProfile>('/v1.0/players/me', input, {silentError: false})).data;
}

export interface ProfileShowcase {
  player_id: string;
  name?: string;
  featured_achievements: Array<{key: string; count: number}>;
  best_hand?: Pick<HandItem, 'hand_id' | 'table_id' | 'net_change' | 'ended_at' | 'board' | 'hole_cards'>;
}

export async function getProfileShowcase(playerId: string) {
  return (await apiClient.get<ProfileShowcase>(
    `/v1.0/players/${encodeURIComponent(playerId)}/showcase`,
    {silentError: true}
  )).data;
}

export interface PlayerSession {
  table_id: string;
  buyin_amount: number;
  cashout_amount: number;
  net_pnl: number;
  joined_at: number;
  ended_at: number;
}

// Most-recent-first (server sorts descending) — sessions[0].ended_at === 0
// means that table is still the player's open seat. cursor pages backward
// through history; omit it for the first (most recent) page.
export async function getSessions(cursor?: string) {
  return (await apiClient.get<Page<PlayerSession>>('/v1.0/players/me/sessions', {
    params: {cursor}, silentError: true
  })).data.data;
}

export type HandOutcome = 'won' | 'lost' | 'tied';

export interface OpponentSummary {
  player_id: string;
  name?: string;
  hole_cards?: string[];
  won?: boolean;
}

export interface HandItem {
  pk: string;
  sk: string;
  table_id: string;
  hand_id: string;
  outcome: HandOutcome;
  net_change: number;
  ended_at: number;
  board?: string[];
  hole_cards?: string[];
  opponents?: OpponentSummary[];
  server_seed?: string;
  commit_hash?: string;
}

// Most-recent-first (server sorts descending), capped at 50 per page.
// `tableId` scopes it to one table's hands (e.g. the live table's own
// "last winners" strip) instead of the viewer's whole history.
export async function getHands({cursor, tableId}: { cursor?: string; tableId?: string } = {}) {
  return (await apiClient.get<Page<HandItem>>('/v1.0/players/me/hands', {
    params: {cursor, table_id: tableId}, silentError: true
  })).data.data;
}

export async function getHand(handId: string) {
  return (await apiClient.get<HandItem>(`/v1.0/players/me/hands/${encodeURIComponent(handId)}`, {silentError: true})).data;
}
