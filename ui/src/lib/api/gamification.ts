// Leaderboard reads only. The daily-reward wrappers (`spin`/`getCooldown`)
// live in `dailyReward.ts` and are the single spelling of that endpoint —
// `/v1.0/sandbox-credits/`, with the trailing slash the router registers
// (api/internal/api/v1/dailyreward.go). A second, slashless pair used to sit
// here with no callers; it is gone (Issue #104).
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
