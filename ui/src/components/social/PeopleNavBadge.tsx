'use client';
import {useSocialUnread} from '@/lib/hooks/useSocialUnread';

/** Unread social activity, mirrored from the same counter the socket pushes.
 * The number is spelled out for assistive tech: the dot alone would make the
 * badge color-only information. */
export function PeopleNavBadge() {
  const count = useSocialUnread();
  if (count <= 0) return null;
  return <>
    <span className="app-nav-people-badge" aria-hidden="true">{count > 9 ? '9+' : count}</span>
    <span className="sr-only"> — {count} {count === 1 ? 'novidade' : 'novidades'} em Pessoas</span>
  </>;
}
