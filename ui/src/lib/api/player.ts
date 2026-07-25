import {apiClient} from './client';

export type WalletMode = 'sandbox' | 'real';

export interface PlayerProfile {
  user_id: string;
  name?: string;
  wallet_mode: WalletMode;
  poker_terms_accepted: boolean;
  poker_terms_accepted_at?: string;
  game_balance?: number;
  sandbox_balance?: number;
}

export async function getMe() {
  return (await apiClient.get<PlayerProfile>('/v1.0/players/me', {silentError: true})).data;
}

export async function acceptPokerTerms() {
  return (await apiClient.post<PlayerProfile>('/v1.0/players/me/terms/accept', {}, {silentError: true})).data;
}

export async function updateMe(input: { name?: string; wallet_mode?: WalletMode }) {
  return (await apiClient.post<PlayerProfile>('/v1.0/players/me', input, {silentError: true})).data;
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
// means that table is still the player's open seat.
export async function getSessions() {
  return (await apiClient.get<PlayerSession[]>('/v1.0/players/me/sessions', {silentError: true})).data;
}

export type HandOutcome = 'won' | 'lost' | 'tied';

export interface OpponentSummary {
  player_id: string;
  name?: string;
  hole_cards?: string[];
  won?: boolean;
}

export interface HandItem {
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

// Most-recent-first (server sorts descending), capped at 50 by the API.
export async function getHands() {
  return (await apiClient.get<HandItem[]>('/v1.0/players/me/hands', {silentError: true})).data;
}

export async function getHand(handId: string) {
  return (await apiClient.get<HandItem>(`/v1.0/players/me/hands/${handId}`, {silentError: true})).data;
}
