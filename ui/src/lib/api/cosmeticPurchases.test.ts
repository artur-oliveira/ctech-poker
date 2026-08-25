import {describe, expect, test, vi} from 'vitest';

const get = vi.fn();
const post = vi.fn();
vi.mock('./client', () => ({apiClient: {get: (...a: unknown[]) => get(...a), post: (...a: unknown[]) => post(...a)}}));

import {
  createCosmeticPurchase, currentCosmeticPurchase, getCosmeticPurchase, listCosmeticCatalog, listCosmeticPurchases,
  ownedCosmeticIDs, refundCosmeticPurchase, type CosmeticCatalogEntry, type CosmeticPurchase
} from './cosmeticPurchases';

const page = <T, >(data: T[], overrides: Partial<Record<string, unknown>> = {}) => ({
  data: {data, has_next: false, next_cursor: null, has_previous: false, previous_cursor: null, ...overrides},
});

const purchase = (overrides: Partial<CosmeticPurchase> = {}): CosmeticPurchase => ({
  purchase_id: 'p1', kind: 'deck', item_id: 'golden', method: 'pix', status: 'confirmed', ...overrides,
});

const entry = (overrides: Partial<CosmeticCatalogEntry> = {}): CosmeticCatalogEntry => ({
  kind: 'deck', id: 'golden', premium: true, owned: false, ...overrides,
});

describe('cosmeticPurchases api', () => {
  test('listCosmeticCatalog GETs the per-kind catalog and unwraps its single page', async () => {
    get.mockResolvedValueOnce(page([entry()]));
    expect(await listCosmeticCatalog('deck')).toEqual([entry()]);
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/cosmetic-purchase/deck/catalog');
  });

  test('listCosmeticPurchases GETs the per-kind purchase list and returns the page envelope', async () => {
    get.mockResolvedValueOnce(page([purchase()], {has_next: true, next_cursor: 'c2'}));
    const result = await listCosmeticPurchases('felt');
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/cosmetic-purchase/felt/');
    expect(result.next_cursor).toBe('c2');
  });

  test('listCosmeticPurchases passes an encoded cursor through as a query param', async () => {
    get.mockResolvedValueOnce(page([]));
    await listCosmeticPurchases('felt', 'a+b/c=');
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/cosmetic-purchase/felt/?cursor=a%2Bb%2Fc%3D');
  });

  test('createCosmeticPurchase POSTs the item and method with an idem key', async () => {
    post.mockResolvedValueOnce({data: purchase()});
    await createCosmeticPurchase('deck', 'golden', 'fichas');
    expect(post).toHaveBeenCalledWith(
      '/v1.0/wallet/cosmetic-purchase/deck/',
      expect.objectContaining({item_id: 'golden', method: 'fichas', idem_key: expect.any(String)}),
      {silentError: true},
    );
  });

  test('getCosmeticPurchase GETs one purchase by id', async () => {
    get.mockResolvedValueOnce({data: purchase()});
    await getCosmeticPurchase('deck', 'p1');
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/cosmetic-purchase/deck/p1', {silentError: true});
  });

  test('refundCosmeticPurchase POSTs to the refund route', async () => {
    post.mockResolvedValueOnce({data: purchase({status: 'refunding'})});
    await refundCosmeticPurchase('felt', 'p1');
    expect(post).toHaveBeenCalledWith(
      '/v1.0/wallet/cosmetic-purchase/felt/p1/refund',
      expect.objectContaining({idem_key: expect.any(String)}),
      {silentError: true},
    );
  });

  test('ownedCosmeticIDs reads the catalog entitlement flag, not purchase history', () => {
    const owned = ownedCosmeticIDs([
      entry({id: 'four-color', premium: false, owned: true}),
      entry({id: 'golden', owned: true}),
      entry({id: 'pink', owned: false}),
    ]);
    expect(owned).toEqual(new Set(['four-color', 'golden']));
  });

  test('currentCosmeticPurchase prioritizes confirmed over pending, newest first as a tiebreak', () => {
    const purchases = [
      purchase({purchase_id: 'old', item_id: 'golden', status: 'refunded', updated_at: '2026-01-01T00:00:00Z'}),
      purchase({purchase_id: 'new', item_id: 'golden', status: 'confirmed', updated_at: '2026-02-01T00:00:00Z'}),
      purchase({purchase_id: 'other', item_id: 'pink', status: 'confirmed'}),
    ];
    expect(currentCosmeticPurchase(purchases, 'golden')?.purchase_id).toBe('new');
  });
});
