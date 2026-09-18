import {describe, expect, test, vi} from 'vitest';

const get = vi.fn();
const post = vi.fn();
vi.mock('./client', async importOriginal => ({
  ...await importOriginal<typeof import('./client')>(),
  apiClient: {get: (...a: unknown[]) => get(...a), post: (...a: unknown[]) => post(...a)},
}));

import {ApiError} from './client';
import {
  createPurchase,
  getPurchase,
  listPurchases,
  listSkus,
  promoRedeemErrorMessage,
  refundPurchase,
  welcomePackErrorMessage,
} from './wallet';

describe('wallet api', () => {
  const page = <T, >(data: T[], overrides: Record<string, unknown> = {}) => ({
    data: {data, has_next: false, next_cursor: null, has_previous: false, previous_cursor: null, ...overrides},
  });

  test('listSkus GETs the catalog and unwraps its single page', async () => {
    get.mockResolvedValueOnce(page([{id: 'pack_100', price_cents: 100, base_credits: 1000, bonus_percent: 0, total_credits: 1000}]));
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

  test('listPurchases GETs the history and returns the page envelope', async () => {
    get.mockResolvedValueOnce(page([], {has_next: true, next_cursor: 'c2'}));
    expect((await listPurchases()).next_cursor).toBe('c2');
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/sandbox-purchase/');
  });

  test('listPurchases passes an encoded cursor through as a query param', async () => {
    get.mockResolvedValueOnce(page([]));
    await listPurchases('a+b/c=');
    expect(get).toHaveBeenCalledWith('/v1.0/wallet/sandbox-purchase/?cursor=a%2Bb%2Fc%3D');
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

  test('createPurchase POSTs a promo_code, when given, alongside sku', async () => {
    post.mockResolvedValueOnce({data: {purchase_id: 'sbxp-2', status: 'pending'}});
    await createPurchase('', 'BEMVINDO10');
    expect(post).toHaveBeenCalledWith(
      '/v1.0/wallet/sandbox-purchase/',
      expect.objectContaining({sku: '', promo_code: 'BEMVINDO10', idem_key: expect.any(String)}),
      {silentError: true},
    );
  });

  test('createPurchase omits promo_code entirely when none is given', async () => {
    post.mockResolvedValueOnce({data: {purchase_id: 'sbxp-3', status: 'pending'}});
    await createPurchase('pack_100');
    expect(post).toHaveBeenCalledWith(
      '/v1.0/wallet/sandbox-purchase/',
      expect.objectContaining({sku: 'pack_100', promo_code: undefined}),
      {silentError: true},
    );
  });
});

describe('promoRedeemErrorMessage', () => {
  test('maps each backend promocode error to pt-BR copy', () => {
    expect(promoRedeemErrorMessage(new ApiError('x', 400, {detail: 'promocode: already redeemed by this player'})))
      .toBe('Este código já foi usado na sua conta.');
    expect(promoRedeemErrorMessage(new ApiError('x', 400, {detail: 'promocode: expired'})))
      .toBe('Este código promocional expirou.');
    expect(promoRedeemErrorMessage(new ApiError('x', 400, {detail: 'promocode: redemption limit reached'})))
      .toBe('Este código atingiu o limite de resgates.');
    expect(promoRedeemErrorMessage(new ApiError('x', 400, {detail: 'promocode: not found'})))
      .toBe('Código promocional inválido.');
    expect(promoRedeemErrorMessage(new Error('network down'))).toBe('Não foi possível aplicar o código agora. Tente novamente.');
  });
});

describe('welcomePackErrorMessage', () => {
  test('names the welcome pack specifically for an already-redeemed claim', () => {
    expect(welcomePackErrorMessage(new ApiError('x', 400, {detail: 'promocode: already redeemed by this player'})))
      .toBe('Você já resgatou o pacote de boas-vindas antes.');
    expect(welcomePackErrorMessage(new Error('boom'))).toBe('Não foi possível iniciar o pacote de boas-vindas agora. Tente novamente.');
  });
});
