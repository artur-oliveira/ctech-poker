import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {describe, expect, test} from 'vitest';
import {IDENTITY_SCOPES, OAUTH_SCOPE, POKER_READ_SCOPES} from './scopes';

type Manifest = {
  scopes: Array<{
    name: string;
    visibility: 'public' | 'internal';
    status: 'active' | 'deprecated';
  }>;
};

describe('Poker OAuth scopes', () => {
  test('requests every public active scope declared by the Poker API', () => {
    const manifestPath = resolve(process.cwd(), '../api/internal/oauthresource/scope-manifest.json');
    const manifest = JSON.parse(readFileSync(manifestPath, 'utf8')) as Manifest;
    const publicActiveScopes = manifest.scopes
      .filter((scope) => scope.visibility === 'public' && scope.status === 'active')
      .map((scope) => scope.name)
      .sort();

    expect([...POKER_READ_SCOPES].sort()).toEqual(publicActiveScopes);
  });

  test('requests identity and read-only Poker scopes, never an internal scope', () => {
    expect(OAUTH_SCOPE.split(' ')).toEqual([...IDENTITY_SCOPES, ...POKER_READ_SCOPES]);
    expect(POKER_READ_SCOPES).toHaveLength(11);
    expect(POKER_READ_SCOPES.every((scope) => scope.endsWith(':read'))).toBe(true);
    expect(OAUTH_SCOPE).not.toContain('internal:');
  });
});
