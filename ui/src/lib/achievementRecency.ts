// Recency shaping for the achievements page (#119). Pure, over the summary
// rows already in the react-query cache, plus the one-shot handoff that lets
// the page know the player just came from a table unlock.
import type {AchievementSummaryEntry} from '@/lib/api/achievements';

export const RECENT_UNLOCK_LIMIT = 5;

export interface RecentUnlock {
  key: string;
  stars: number;
  unlockedAtMs: number;
}

/**
 * The most recently unlocked entries, newest first. Rows without an
 * `unlocked_at` are skipped entirely rather than sorted to the bottom: a
 * legacy unlock has no position on a timeline, and inventing one would put
 * "há 56 anos" on the rail.
 */
export function recentUnlocks(
  entries: AchievementSummaryEntry[] | undefined,
  limit = RECENT_UNLOCK_LIMIT
): RecentUnlock[] {
  return (entries || [])
    .flatMap(entry => {
      if (!entry.unlocked || !entry.unlocked_at) return [];
      const unlockedAtMs = Date.parse(entry.unlocked_at);
      return Number.isNaN(unlockedAtMs) ? [] : [{key: entry.key, stars: entry.stars, unlockedAtMs}];
    })
    .sort((a, b) => b.unlockedAtMs - a.unlockedAtMs)
    .slice(0, limit);
}

/** Newest unlock first; everything undated keeps the catalogue's own order
 * after it, so "mais recentes" never hides a legacy unlock. */
export function byRecencyFirst(a?: string, b?: string): number {
  return (b ? Date.parse(b) || 0 : 0) - (a ? Date.parse(a) || 0 : 0);
}

// Session-scoped handoff from the table's AchievementToast to the achievements
// page: the toast lives 4.2s at the table and the celebration belongs on the
// page the player opens next. `sessionStorage`, so it dies with the tab and can
// never re-fire a stale celebration days later.
const CELEBRATION_KEY = 'ctech-poker:achievement-arrival';

export function rememberAchievementUnlock(key: string) {
  if (typeof window === 'undefined') return;
  try {
    window.sessionStorage.setItem(CELEBRATION_KEY, key);
  } catch {
    // Private mode / storage disabled: the rail still lists the unlock, only
    // the celebration is lost. Never break the toast over it.
  }
}

/** Reads and clears the pending celebration — "once" is enforced here, so a
 * remount or a filter change cannot replay it. */
export function takeAchievementUnlock(): string | null {
  if (typeof window === 'undefined') return null;
  try {
    const key = window.sessionStorage.getItem(CELEBRATION_KEY);
    if (key) window.sessionStorage.removeItem(CELEBRATION_KEY);
    return key;
  } catch {
    return null;
  }
}
