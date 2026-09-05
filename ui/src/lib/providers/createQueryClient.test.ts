import {describe, expect, it} from 'vitest';
import {createQueryClient, getQueryClient} from '@/lib/providers/createQueryClient';
import {HISTORY_QUERY, SESSION_QUERY, STATIC_QUERY} from '@/lib/queryFreshness';

describe('createQueryClient', () => {
  it('applies the shared query defaults', () => {
    const {queries, mutations} = createQueryClient().getDefaultOptions();
    expect(queries?.staleTime).toBe(30 * 1000);
    expect(queries?.retry).toBe(false);
    // #233: focus refetching is opt-in per family, not the app-wide default.
    expect(queries?.refetchOnWindowFocus).toBe(false);
    expect(mutations?.retry).toBe(false);
  });

  it('classifies each family by how fresh it has to be', () => {
    const client = createQueryClient();
    const defaultsFor = (key: readonly unknown[]) => client.getQueryDefaults(key);

    // The player's own live standing is the family that keeps focus refetching.
    expect(defaultsFor(['player', 'me'])).toMatchObject(SESSION_QUERY);
    expect(defaultsFor(['sessions', 'me'])).toMatchObject(SESSION_QUERY);

    // Catalogs are shared between Store and Table on one read.
    expect(defaultsFor(['wallet', 'cosmetic-catalog', 'deck'])).toMatchObject(STATIC_QUERY);
    expect(defaultsFor(['wallet', 'reaction-catalog'])).toMatchObject(STATIC_QUERY);
    expect(defaultsFor(['achievements', 'catalog'])).toMatchObject(STATIC_QUERY);

    // Append-only records never re-read every loaded page just because the tab
    // came back.
    expect(defaultsFor(['hands', 'sandbox'])).toMatchObject(HISTORY_QUERY);
    expect(defaultsFor(['wallet', 'cosmetic-purchases', 'felt'])).toMatchObject(HISTORY_QUERY);

    // Anything unclassified inherits the app default: no focus refetch.
    expect(defaultsFor(['leaderboard', 'me', 'sandbox']).refetchOnWindowFocus).toBeUndefined();
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
