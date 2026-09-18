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

/* The viewer's-rank key used to be spelled two ways (`['leaderboard','me',mode]`
 * and `['leaderboard-me',mode]`), so walking between `/hands` and `/leaderboard`
 * refetched data already in the cache. Both now go through myRankKey below;
 * `/hands` shows the lifetime standing, which is what the defaults resolve to. */
/** Which window a board covers. `month` is the current calendar month in BRT
 * (the server derives the bucket, the client only names the period); `all` is
 * the lifetime board this page served before monthly rankings existed. */
export type LeaderboardPeriod = 'month' | 'all';

/** The three metrics the board can be ordered by — each backed by its own GSI
 * server-side, so this list cannot grow without a backend change. */
export type LeaderboardMetric = 'hands_won' | 'hands_played' | 'win_rate';

/** A player needs this many hands **within the period** before they appear on
 * the win_rate board at all (`leaderboard.MinHandsForWinRateRank`). Mirrored
 * here only to explain the absence to the player, never to filter rows — the
 * server already leaves sub-floor rows off the board. */
export const MIN_HANDS_FOR_WIN_RATE = 100;

export interface BoardScope {
  period?: LeaderboardPeriod;
  metric?: LeaderboardMetric;
}

/** The one spelling of a board's query key. Period and metric are part of it:
 * two boards of the same mode are different data, and caching them under one
 * key would show September's ranking under October's tab. */
export const leaderboardKey = (mode: WalletMode, {period = 'all', metric = 'hands_won'}: BoardScope = {}) =>
  ['leaderboard', mode, period, metric] as const;

export const myRankKey = (mode: WalletMode, {period = 'all', metric = 'hands_won'}: BoardScope = {}) =>
  ['leaderboard', 'me', mode, period, metric] as const;

export async function leaderboard(mode: WalletMode = 'sandbox', cursor?: string, scope: BoardScope = {}) {
  const {period = 'all', metric = 'hands_won'} = scope;
  return (await apiClient.get<Page<Entry>>('/v1.0/leaderboard', {params: {mode, cursor, period, metric}})).data.data;
}

/**
 * The viewer's exact rank + total ranked player count on one board, from
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

export async function myRank(mode: WalletMode = 'sandbox', scope: BoardScope = {}): Promise<MyRank> {
  const {period = 'all', metric = 'hands_won'} = scope;
  return (await apiClient.get<MyRank>('/v1.0/leaderboard/me', {params: {mode, period, metric}})).data;
}
