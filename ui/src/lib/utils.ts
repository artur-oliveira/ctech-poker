import {type ClassValue, clsx} from 'clsx';
import {twMerge} from 'tailwind-merge';
import {getAccessToken} from '@/lib/api/client';
import {decodeIdToken} from '@/lib/auth/oauth';
import {MOCK_PLAYER_ID, USE_MOCK} from '@/lib/mock';

export function cn(...values: ClassValue[]) {
  return twMerge(clsx(values));
}

// The single answer to "who is looking at this screen" — OIDC sub in prod,
// the fixed mock player in mock mode.
export function getViewerId(): string | undefined {
  if (USE_MOCK) return MOCK_PLAYER_ID;
  const token = getAccessToken();
  return token ? (decodeIdToken(token) as { sub?: string } | null)?.sub : undefined;
}

// Player IDs are opaque (OIDC sub UUIDs in prod, slug-like IDs in mocks). Turn
// an id into a readable label without truncating — truncation turns a UUID into
// garbage and provokes overflow. Mock ids like `mock_player_ana` read naturally.
export function playerName(id: string, viewerId?: string): string {
  if (viewerId && id === viewerId) return 'Você';
  return id.replaceAll('_', ' ');
}
