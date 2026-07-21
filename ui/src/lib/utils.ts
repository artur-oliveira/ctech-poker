import {type ClassValue, clsx} from 'clsx';
import {twMerge} from 'tailwind-merge';

export function cn(...values: ClassValue[]) {
  return twMerge(clsx(values))
}

// Player IDs are opaque (OIDC sub UUIDs in prod, slug-like IDs in mocks). Turn
// an id into a readable label without truncating — truncation turns a UUID into
// garbage and provokes overflow. Mock ids like `mock_player_ana` read naturally.
export function playerName(id: string, viewerId?: string): string {
  if (viewerId && id === viewerId) return 'Você';
  return id.replaceAll('_', ' ');
}
