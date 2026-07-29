import type {Page} from './client';
import {apiClient} from './client';
import type {WalletMode} from './player';

export interface Entry {
  player_id: string;
  player_name?: string;
  hands_played: number;
  hands_won: number;
  win_rate: number
}

export async function leaderboard(mode: WalletMode = 'sandbox', cursor?: string) {
  return (await apiClient.get<Page<Entry>>('/v1.0/leaderboard', {params: {mode, cursor}})).data.data;
}

export async function spin(): Promise<{ amount: number; remaining_time_seconds: number; }> {
  return (await apiClient.post<{
    amount: number;
    remaining_time_seconds: number;
  }>('/v1.0/sandbox-credits', {}, {silentError: true})).data;
}

export async function remainingTime(): Promise<{ remaining_time_seconds: number; }> {
  return (await apiClient.get<{
    remaining_time_seconds: number;
  }>('/v1.0/sandbox-credits', {silentError: true})).data;
}
