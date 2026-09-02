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

/**
 * The viewer's exact global rank + total ranked player count for mode, from
 * `GET /v1.0/leaderboard/me` — computed server-side against the full board,
 * not just whatever page `leaderboard()` happened to fetch. `ranked: false`
 * (with `rank`/`total`/`entry` absent) means the viewer has no stats row yet
 * for this mode — "unranked", not an error.
 */
export interface MyRank {
  ranked: boolean;
  rank?: number;
  total?: number;
  entry?: Entry;
}

export async function myRank(mode: WalletMode = 'sandbox'): Promise<MyRank> {
  return (await apiClient.get<MyRank>('/v1.0/leaderboard/me', {params: {mode}})).data;
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
