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
  return (await apiClient.get<{
    sessions: PlayerSession[]
  }>('/v1.0/players/me/sessions', {silentError: true})).data.sessions;
}
