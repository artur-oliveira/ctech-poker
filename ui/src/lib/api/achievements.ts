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
//
// Paginated at 100/key; the client never follows the cursor, so a player who
// has touched more distinct keys than one page understates their real
// progress. Superseded for display purposes by getMyAchievementsSummary
// below (#79) — kept only because it's still the underlying data source the
// summary endpoint itself reads from server-side.
export async function getMyAchievements(mode: WalletMode = 'sandbox', cursor?: string) {
  return (await apiClient.get<Page<PlayerAchievementProgress>>('/v1.0/players/me/achievements', {
    params: {mode, cursor}, silentError: true
  })).data.data;
}

export interface AchievementSummaryEntry extends Achievement {
  progress: number;
  stars: number;
  unlocked: boolean;
  completed: boolean;
  next_target: number | null;
  max_target: number;
  // RFC 3339 instant of the last tier this player crossed (backend #72),
  // `omitempty` on the wire: absent on every row unlocked before the field
  // existed, and on rows with no tier cleared at all. Never substitute a
  // fallback date — "unknown when" is the truth for a legacy unlock.
  unlocked_at?: string
}

export interface AchievementTotals {
  revealed: number;
  unlocked: number;
  completed: number;
  stars: number;
  max_stars: number
}

export interface AchievementsSummary {
  mode: WalletMode;
  totals: AchievementTotals;
  achievements: AchievementSummaryEntry[]
}

// Auth required: the full-state achievement summary (#71/#79) in one,
// non-paginated response — every catalog achievement the player may see
// (a still-locked secret is omitted, the same reveal gate the paginated
// endpoint applies), complete progress/stars/unlocked/completed per key, and
// catalog-wide totals computed server-side so the client can't understate
// them from a partial fetch. This is the source of truth the achievements
// page and the showcase picker should build their progress map from instead
// of getMyAchievements above.
export async function getMyAchievementsSummary(mode: WalletMode = 'sandbox') {
  return (await apiClient.get<AchievementsSummary>('/v1.0/players/me/achievements/summary', {
    params: {mode}, silentError: true
  })).data;
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
