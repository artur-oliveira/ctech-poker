export const IDENTITY_SCOPES = ['openid', 'profile'] as const;

// Keep this list synchronized with the public, active entries owned by the
// Poker Resource Server manifest. Interactive write operations are authorized
// by the first-party Poker client (`azp=poker`), not by an internal scope: a
// browser SPA cannot safely hold confidential client credentials.
export const POKER_READ_SCOPES = [
  'poker:rooms:read',
  'poker:players:read',
  'poker:sessions:read',
  'poker:hands:read',
  'poker:achievements:read',
  'poker:stats:read',
  'poker:leaderboard:read',
  'poker:daily-reward:read',
  'poker:player-notes:read',
  'poker:sandbox-purchases:read',
  'poker:reaction-purchases:read',
  'poker:cosmetic-purchases:read',
] as const;

export const OAUTH_SCOPE = [...IDENTITY_SCOPES, ...POKER_READ_SCOPES].join(' ');
