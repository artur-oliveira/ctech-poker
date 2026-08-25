import {QueryClient} from '@tanstack/react-query';
import {describe, expect, test, vi} from 'vitest';

const get = vi.fn();
const post = vi.fn();
vi.mock('./client', () => ({apiClient: {get: (...a: unknown[]) => get(...a), post: (...a: unknown[]) => post(...a)}}));

import {
  createReactionPurchase, currentReactionPurchase, getReactionPurchase, listReactionCatalog, listReactionPurchases,
  ownedReactionIDs, refundReactionPurchase, REACTION_PURCHASE_FIRST_PAGE_KEY, REACTION_PURCHASE_HISTORY_KEY,
  type ReactionCatalogEntry, type ReactionPurchase
} from './reactionPurchases';
import {WALLET_QUERY_ROOT} from './wallet';

const page = <T, >(data: T[], overrides: Record<string, unknown> = {}) => ({
  data: {data, has_next: false, next_cursor: null, has_previous: false, previous_cursor: null, ...overrides},
});

const purchase = (overrides: Partial<ReactionPurchase> = {}): ReactionPurchase => ({
  purchase_id: 'p1', reaction_id: 'fire', method: 'pix', status: 'confirmed', ...overrides,
});

const entry = (overrides: Partial<ReactionCatalogEntry> = {}): ReactionCatalogEntry => ({
  id: 'fire', premium: true, owned: false, ...overrides,
});

describe('reactionPurchases api', () => {
  test('listReactionCatalog GETs the catalog and unwraps its single page', async () => {
    get.mockResolvedValueOnce(page([entry()]));
    expect(await listReactionCatalog()).toEqual([entry()]);
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/reaction-purchase/catalog');
  });

  test('listReactionPurchases GETs the history and returns the page envelope', async () => {
    get.mockResolvedValueOnce(page([purchase()], {has_next: true, next_cursor: 'c2'}));
    const result = await listReactionPurchases();
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/reaction-purchase/');
    expect(result.next_cursor).toBe('c2');
  });

  test('listReactionPurchases passes an encoded cursor through as a query param', async () => {
    get.mockResolvedValueOnce(page([]));
    await listReactionPurchases('a+b/c=');
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/reaction-purchase/?cursor=a%2Bb%2Fc%3D');
  });

  test('createReactionPurchase POSTs the reaction and method with an idem key', async () => {
    post.mockResolvedValueOnce({data: purchase()});
    await createReactionPurchase('fire', 'fichas');
    expect(post).toHaveBeenCalledWith(
      '/v1.0/wallet/reaction-purchase/',
      expect.objectContaining({reaction_id: 'fire', method: 'fichas', idem_key: expect.any(String)}),
      {silentError: true},
    );
  });

  test('getReactionPurchase GETs one purchase by id', async () => {
    get.mockResolvedValueOnce({data: purchase()});
    await getReactionPurchase('p/1');
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/reaction-purchase/p%2F1', {silentError: true});
  });

  test('refundReactionPurchase POSTs to the refund route', async () => {
    post.mockResolvedValueOnce({data: purchase({status: 'refunding'})});
    await refundReactionPurchase('p1');
    expect(post).toHaveBeenCalledWith(
      '/v1.0/wallet/reaction-purchase/p1/refund',
      expect.objectContaining({idem_key: expect.any(String)}),
      {silentError: true},
    );
  });

  test('ownedReactionIDs reads the catalog entitlement flag, not purchase history', () => {
    expect(ownedReactionIDs([
      entry({id: 'clap', premium: false, owned: true}),
      entry({id: 'fire', owned: true}),
      entry({id: 'cold', owned: false}),
    ])).toEqual(new Set(['clap', 'fire']));
  });

  test('the store history and the table first page never share one cache entry', () => {
    const client = new QueryClient();
    // What the store's useInfiniteQuery writes. Under a shared key the table's
    // plain useQuery would hand this object to TableReactions as `purchases`.
    client.setQueryData(REACTION_PURCHASE_HISTORY_KEY, {pages: [{data: [purchase()]}], pageParams: [undefined]});
    expect(client.getQueryData(REACTION_PURCHASE_FIRST_PAGE_KEY)).toBeUndefined();
    // Both must still be swept by the single wallet-root invalidation.
    client.setQueryData(REACTION_PURCHASE_FIRST_PAGE_KEY, [purchase()]);
    expect(client.getQueryCache().findAll({queryKey: WALLET_QUERY_ROOT})).toHaveLength(2);
  });

  test('currentReactionPurchase prioritizes confirmed over pending, newest first as a tiebreak', () => {
    const purchases = [
      purchase({purchase_id: 'old', status: 'refunded', updated_at: '2026-01-01T00:00:00Z'}),
      purchase({purchase_id: 'new', status: 'confirmed', updated_at: '2026-02-01T00:00:00Z'}),
      purchase({purchase_id: 'other', reaction_id: 'cold'}),
    ];
    expect(currentReactionPurchase(purchases, 'fire')?.purchase_id).toBe('new');
  });
});
