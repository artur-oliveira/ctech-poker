import {describe, expect, test, vi} from 'vitest';

const get = vi.fn();
const post = vi.fn();
vi.mock('./client', () => ({apiClient: {get: (...a: unknown[]) => get(...a), post: (...a: unknown[]) => post(...a)}}));

import {createPurchase, getPurchase, listPurchases, listSkus, refundPurchase} from './wallet';

describe('wallet api', () => {
  test('listSkus GETs the catalog', async () => {
    get.mockResolvedValueOnce({data: [{id: 'pack_100', price_cents: 100, base_credits: 1000, bonus_percent: 0, total_credits: 1000}]});
    const skus = await listSkus();
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/sandbox-purchase/skus');
    expect(skus).toHaveLength(1);
  });

  test('createPurchase POSTs sku with a fresh idem_key', async () => {
    post.mockResolvedValueOnce({data: {purchase_id: 'sbxp-1', status: 'pending'}});
    await createPurchase('pack_100');
    expect(post).toHaveBeenCalledWith(
      '/v1.0/wallet/sandbox-purchase/',
      expect.objectContaining({sku: 'pack_100', idem_key: expect.any(String)}),
      {silentError: true},
    );
  });

  test('listPurchases GETs the history', async () => {
    get.mockResolvedValueOnce({data: []});
    await listPurchases();
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/sandbox-purchase/');
  });

  test('getPurchase GETs by id', async () => {
    get.mockResolvedValueOnce({data: {purchase_id: 'sbxp-1', status: 'confirmed'}});
    await getPurchase('sbxp-1');
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/sandbox-purchase/sbxp-1');
  });

  test('refundPurchase POSTs a fresh idem_key', async () => {
    post.mockResolvedValueOnce({data: {purchase_id: 'sbxp-1', status: 'refunded'}});
    await refundPurchase('sbxp-1');
    expect(post).toHaveBeenCalledWith(
      '/v1.0/wallet/sandbox-purchase/sbxp-1/refund',
      expect.objectContaining({idem_key: expect.any(String)}),
      {silentError: true},
    );
  });
});
