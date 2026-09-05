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

/**
 * Request budget for one `/leaderboard` open: **two** GETs — the board page
 * and, for a signed-in viewer, their own rank — and nothing else until this
 * staleTime elapses.
 *
 * It is pinned to the server's rank-mirror TTL (`leaderboard.RankMirrorTTL`,
 * 5 min): a rank served inside that window is materialized from the same
 * snapshot, so the global 30s staleTime plus `refetchOnWindowFocus` was
 * spending requests on answers that could not have changed.
 */
export const LEADERBOARD_STALE_MS = 5 * 60 * 1000;

/** The one spelling of the viewer's-rank query key. `/hands` and
 * `/leaderboard` render the same `myRank(mode)` response; they used to cache
 * it under `['leaderboard','me',mode]` and `['leaderboard-me',mode]`, so
 * walking between the two pages refetched data already in the cache. */
export const myRankKey = (mode: WalletMode) => ['leaderboard', 'me', mode] as const;

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
