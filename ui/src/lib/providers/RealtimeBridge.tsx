'use client';
import {useLobbyRealtime} from '@/lib/hooks/useLobbyRealtime';

/** The lobby/user gateway is a single per-session socket: social pushes,
 * presence and the unread badge have to arrive on every route, not only on the
 * lobby or the store. Mounted once by QueryProvider; nothing else may mount
 * useLobbyRealtime again or the same account opens two connections. */
export function RealtimeBridge() {
  useLobbyRealtime();
  return null;
}
