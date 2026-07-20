import {apiClient} from './client';

export interface Entry {
  player_id: string;
  hands_played: number;
  hands_won: number;
  win_rate: number
}

export async function leaderboard() {
  return (await apiClient.get<Entry[]>('/v1.0/leaderboard')).data
}

export async function spin() {
  return (await apiClient.post<{ amount: number }>('/v1.0/roulette/spin', {}, {silentError: true})).data
}
