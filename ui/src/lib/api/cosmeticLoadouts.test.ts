import {describe, expect, test, vi} from 'vitest';

const get = vi.fn();
const post = vi.fn();
const del = vi.fn();
vi.mock('./client', () => ({
  apiClient: {
    get: (...a: unknown[]) => get(...a), post: (...a: unknown[]) => post(...a), delete: (...a: unknown[]) => del(...a),
  },
}));

import {applyCosmeticLoadout, createCosmeticLoadout, deleteCosmeticLoadout, listCosmeticLoadouts} from './cosmeticLoadouts';

const page = <T, >(data: T[]) => ({data: {data, has_next: false, next_cursor: null, has_previous: false, previous_cursor: null}});

const loadout = {id: 'l1', name: 'Combo', selections: {deck: 'four-color', felt: 'classic'}, created_at: '2026-09-01T00:00:00Z'};

describe('cosmeticLoadouts api', () => {
  test('listCosmeticLoadouts GETs and unwraps the single page', async () => {
    get.mockResolvedValueOnce(page([loadout]));
    expect(await listCosmeticLoadouts()).toEqual([loadout]);
    expect(get).toHaveBeenCalledWith('/v1.0/players/me/cosmetic-loadouts', {silentError: true});
  });

  test('createCosmeticLoadout POSTs the name and selections', async () => {
    post.mockResolvedValueOnce({data: loadout});
    const result = await createCosmeticLoadout('Combo', {deck: 'four-color', felt: 'classic'});
    expect(post).toHaveBeenCalledWith('/v1.0/players/me/cosmetic-loadouts',
      {name: 'Combo', selections: {deck: 'four-color', felt: 'classic'}});
    expect(result).toEqual(loadout);
  });

  test('deleteCosmeticLoadout DELETEs the encoded id', async () => {
    del.mockResolvedValueOnce({});
    await deleteCosmeticLoadout('l 1');
    expect(del).toHaveBeenCalledWith('/v1.0/players/me/cosmetic-loadouts/l%201');
  });

  test('applyCosmeticLoadout POSTs to the apply route and returns the loadout', async () => {
    post.mockResolvedValueOnce({data: loadout});
    const result = await applyCosmeticLoadout('l1');
    expect(post).toHaveBeenCalledWith('/v1.0/players/me/cosmetic-loadouts/l1/apply', {});
    expect(result).toEqual(loadout);
  });
});
