import {describe, expect, it} from 'vitest';
import {createQueryClient, getQueryClient} from '@/lib/providers/createQueryClient';

describe('createQueryClient', () => {
  it('applies the shared query defaults', () => {
    const {queries, mutations} = createQueryClient().getDefaultOptions();
    expect(queries?.staleTime).toBe(30 * 1000);
    expect(queries?.retry).toBe(false);
    expect(queries?.refetchOnWindowFocus).toBe(true);
    expect(mutations?.retry).toBe(false);
  });

  it('returns a fresh instance every call', () => {
    expect(createQueryClient()).not.toBe(createQueryClient());
  });
});

describe('getQueryClient', () => {
  it('returns the same browser instance across calls so the cache survives a route-group switch', () => {
    // jsdom => window is defined => singleton path
    const first = getQueryClient();
    first.setQueryData(['player', 'me'], {user_id: 'p1'});

    const afterNavigation = getQueryClient();
    expect(afterNavigation).toBe(first);
    expect(afterNavigation.getQueryData(['player', 'me'])).toEqual({user_id: 'p1'});
  });
});
