import type {Page} from './client';
import {apiClient} from './client';
import type {WalletMode} from './player';

export interface Tier {
  stars: number;
  threshold: number
}

export interface Achievement {
  key: string;
  metric: string;
  tiers: Tier[];
  secret?: boolean
}

export interface PlayerAchievementProgress {
  key: string;
  count: number
}

// Public: no auth, the full catalog of achievement definitions.
export async function getAchievementCatalog() {
  return (await apiClient.get<Achievement[]>('/v1.0/achievements')).data;
}

// Auth required: the caller's own per-key progress counters. Keys absent
// here simply have no progress yet (count 0), matching the API's
// query-by-partition-key semantics (nothing written == nothing to list).
export async function getMyAchievements(mode: WalletMode = 'sandbox', cursor?: string) {
  return (await apiClient.get<Page<PlayerAchievementProgress>>('/v1.0/players/me/achievements', {
    params: {mode, cursor}, silentError: true
  })).data.data;
}

export interface AchievementProgress {
  count: number;
  starsFilled: number;
  nextTier: Tier | null;
  maxed: boolean
}

// Derives display-ready progress from a catalog entry's tiers + a raw
// counter: how many stars are solidly earned, and what the next tier to
// chase is (null once every tier is cleared).
export function achievementProgress(tiers: Tier[], count: number): AchievementProgress {
  let starsFilled = 0;
  let nextTier: Tier | null = null;
  for (const tier of tiers) {
    if (count >= tier.threshold) starsFilled = Math.max(starsFilled, tier.stars);
    else if (!nextTier || tier.threshold < nextTier.threshold) nextTier = tier;
  }
  return {count, starsFilled, nextTier, maxed: !nextTier};
}
