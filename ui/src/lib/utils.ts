import {type ClassValue, clsx} from 'clsx';
import {twMerge} from 'tailwind-merge';
import {getPlayerId} from '@/lib/api/client';
import {MOCK_PLAYER_ID, USE_MOCK} from '@/lib/mock';

export function cn(...values: ClassValue[]) {
  return twMerge(clsx(values));
}

// The single answer to "who is looking at this screen" — the profile's
// user_id (matches seat.player_id / current_player_id server-side) in prod,
// the fixed mock player in mock mode. NOT decodeIdToken: that only ever
// returns username/first_name/last_name, never `sub` — using it here silently
// left every viewer comparison undefined.
export function getViewerId(): string | undefined {
  if (USE_MOCK) return MOCK_PLAYER_ID;
  return getPlayerId() ?? undefined;
}

// Player IDs are opaque (OIDC sub UUIDs in prod) and carry no name — the
// display name comes from the player's persisted profile (GET /players/me),
// broadcast to seats by the table actor, so callers pass whatever name they
// already resolved from a SeatView. Until it arrives, `name` is undefined and
// the seat shows as a not-yet-named placeholder.
export function playerName(id: string, viewerId?: string, name?: string): string {
  if (viewerId && id === viewerId) return 'Você';
  return name || 'Visitante';
}

// Seat CSS position is purely index-driven (Seat.tsx's `seat-${index}` class),
// so the server's seat order must be rotated before rendering — otherwise the
// viewer lands wherever the server happens to seat them instead of always at
// the hero slot (index 0).
export function rotateSeats<T extends {player_id: string}>(seats: T[], viewerId?: string): T[] {
  const at = viewerId ? seats.findIndex(seat => seat.player_id === viewerId) : -1;
  if (at <= 0) return seats;
  return [...seats.slice(at), ...seats.slice(0, at)];
}
