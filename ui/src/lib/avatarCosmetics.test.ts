import {describe, expect, test, vi} from 'vitest';

const get = vi.fn();
vi.mock('./api/client', () => ({apiClient: {get: (...a: unknown[]) => get(...a)}}));

import {AVATAR_ITEMS, avatarBadgeLabel, avatarFrameLabel, avatarItemLabel, listOwnedAvatarCosmetics} from './avatarCosmetics';

describe('avatarCosmetics', () => {
  test('registry has the launch set: 6 frames, 6 hats, 5 accessories, 7 badges with pt-BR labels', () => {
    const labels = (slot: keyof typeof AVATAR_ITEMS) =>
      Object.fromEntries(Object.entries(AVATAR_ITEMS[slot]).map(([id, v]) => [id, v.label]));
    expect(labels('frame')).toEqual({
      'season-2026-q3': 'Moldura da Temporada', 'champion-gold': 'Moldura de Campeão',
      'chip-ring': 'Borda de Fichas', 'felt-ring': 'Borda de Feltro',
      'walnut-rail': 'Borda de Nogueira', 'oxblood-edge': 'Borda Oxblood',
    });
    expect(labels('hat')).toEqual({
      'dealer-visor': 'Viseira do Dealer', crown: 'Coroa', laurel: 'Louros',
      'top-hat': 'Cartola', cowboy: 'Chapéu de Cowboy', beanie: 'Gorro',
    });
    expect(labels('accessory')).toEqual({
      'bandit-mask': 'Máscara de Bandido', shades: 'Óculos Escuros', monocle: 'Monóculo',
      mustache: 'Bigode', headset: 'Headset',
    });
    expect(labels('badge')).toEqual({
      'season-2026-q3-top10': 'Top 10 da Temporada', 'season-2026-q3-champion': 'Campeão da Temporada',
      'all-in-hero': 'Herói do All-in', 'iron-nerve': 'Sangue Frio', rainmaker: 'Chuva de Fichas',
      veteran: 'Veterano', 'social-butterfly': 'Anfitrião',
    });
  });

  test('avatarItemLabel falls back to the id for unknown ids and slots', () => {
    expect(avatarItemLabel('hat', 'crown')).toBe('Coroa');
    expect(avatarItemLabel('hat', 'nope')).toBe('nope');
    expect(avatarItemLabel('frame', 'crown')).toBe('crown');
  });

  test('avatarFrameLabel/avatarBadgeLabel resolve known ids and fall back to the raw id for unknown ones', () => {
    expect(avatarFrameLabel('season-2026-q3')).toBe('Moldura da Temporada');
    expect(avatarFrameLabel('not-a-real-frame')).toBe('not-a-real-frame');
    expect(avatarBadgeLabel('season-2026-q3-champion')).toBe('Campeão da Temporada');
    expect(avatarBadgeLabel('not-a-real-badge')).toBe('not-a-real-badge');
  });

  test('listOwnedAvatarCosmetics GETs the per-kind owned-ids route and unwraps the page', async () => {
    get.mockResolvedValueOnce({
      data: {data: ['season-2026-q3'], has_next: false, next_cursor: null, has_previous: false, previous_cursor: null}
    });
    expect(await listOwnedAvatarCosmetics('frame')).toEqual(['season-2026-q3']);
    expect(get).toHaveBeenCalledWith('/v1.0/players/me/cosmetics/frame/owned', {silentError: true});
  });
});
