import {describe, expect, test, vi} from 'vitest';

const get = vi.fn();
const post = vi.fn();
vi.mock('./client', () => ({apiClient: {get: (...a: unknown[]) => get(...a), post: (...a: unknown[]) => post(...a)}}));

import {
  createCosmeticPurchase, currentCosmeticPurchase, getCosmeticPurchase, listCosmeticCatalog, listCosmeticPurchases,
  ownedCosmeticIDs, refundCosmeticPurchase, type CosmeticPurchase
} from './cosmeticPurchases';

const purchase = (overrides: Partial<CosmeticPurchase> = {}): CosmeticPurchase => ({
  purchase_id: 'p1', kind: 'deck', item_id: 'golden', method: 'pix', status: 'confirmed', ...overrides,
});

describe('cosmeticPurchases api', () => {
  test('listCosmeticCatalog GETs the per-kind catalog', async () => {
    get.mockResolvedValueOnce({data: []});
    await listCosmeticCatalog('deck');
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/cosmetic-purchase/deck/catalog');
  });

  test('listCosmeticPurchases GETs the per-kind purchase list', async () => {
    get.mockResolvedValueOnce({data: []});
    await listCosmeticPurchases('felt');
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/cosmetic-purchase/felt/');
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

  test('ownedCosmeticIDs keeps only confirmed purchases', () => {
    const owned = ownedCosmeticIDs([
      purchase({item_id: 'golden', status: 'confirmed'}),
      purchase({item_id: 'pink', status: 'pending'}),
    ]);
    expect(owned).toEqual(new Set(['golden']));
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
