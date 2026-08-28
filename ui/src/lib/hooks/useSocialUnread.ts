'use client';
import {useQuery} from '@tanstack/react-query';
import {getSocialSummary} from '@/lib/api/social';
import {SOCIAL_KEYS} from '@/lib/social';

/** Unread social inbox events. Shared by the nav badge and by the nav link,
 * which points at the Atividades tab precisely when there is something there
 * to read — that tab is the only thing that clears the badge. */
export function useSocialUnread(): number {
  const {data} = useQuery({queryKey: SOCIAL_KEYS.summary, queryFn: getSocialSummary});
  return data?.unread_count ?? 0;
}
