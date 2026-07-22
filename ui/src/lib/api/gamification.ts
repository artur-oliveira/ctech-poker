import {apiClient} from './client';

export interface Entry {
  player_id: string;
  hands_played: number;
  hands_won: number;
  win_rate: number
}

export async function leaderboard() {
  return (await apiClient.get<Entry[]>('/v1.0/leaderboard')).data;
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
