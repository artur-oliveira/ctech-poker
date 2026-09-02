'use client';
import {useQuery} from '@tanstack/react-query';
import {getMyAchievementsSummary} from '@/lib/api/achievements';
import type {WalletMode} from '@/lib/api/player';

// Single, non-paginated fetch of the caller's full achievement state (#79).
// Shared by the achievements page and ProfileShowcaseDialog's featured-
// achievement picker so both read the same complete progress map instead of
// each re-deriving it from the paginated endpoint.
export function useAchievementsSummary(mode: WalletMode, enabled: boolean) {
  return useQuery({
    queryKey: ['achievements', 'summary', mode],
    queryFn: () => getMyAchievementsSummary(mode),
    enabled
  });
}
