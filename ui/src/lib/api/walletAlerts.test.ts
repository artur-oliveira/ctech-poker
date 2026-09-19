import {describe, expect, test, vi} from 'vitest';

const get = vi.fn();
const put = vi.fn();
const del = vi.fn();
vi.mock('./client', () => ({apiClient: {
  get: (...a: unknown[]) => get(...a),
  put: (...a: unknown[]) => put(...a),
  delete: (...a: unknown[]) => del(...a),
}}));

import {deleteWalletAlertPrefs, getWalletAlertPrefs, saveWalletAlertPrefs} from './walletAlerts';

describe('walletAlerts api', () => {
  test('getWalletAlertPrefs GETs the player\'s own thresholds', async () => {
    get.mockResolvedValueOnce({data: {min_sandbox_balance: 500, max_purchase_cents: 2000}});
    const prefs = await getWalletAlertPrefs();
    expect(get).toHaveBeenCalledWith('/v1.0/players/me/wallet-alerts', {silentError: true});
    expect(prefs).toEqual({min_sandbox_balance: 500, max_purchase_cents: 2000});
  });

  test('saveWalletAlertPrefs PUTs both thresholds', async () => {
    put.mockResolvedValueOnce({data: {min_sandbox_balance: 500, max_purchase_cents: 0}});
    const prefs = await saveWalletAlertPrefs({min_sandbox_balance: 500, max_purchase_cents: 0});
    expect(put).toHaveBeenCalledWith('/v1.0/players/me/wallet-alerts', {min_sandbox_balance: 500, max_purchase_cents: 0});
    expect(prefs.min_sandbox_balance).toBe(500);
  });

  test('deleteWalletAlertPrefs DELETEs the player\'s thresholds', async () => {
    del.mockResolvedValueOnce({});
    await deleteWalletAlertPrefs();
    expect(del).toHaveBeenCalledWith('/v1.0/players/me/wallet-alerts');
  });
});
