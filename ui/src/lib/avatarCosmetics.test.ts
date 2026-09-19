import {describe, expect, test, vi} from 'vitest';

const get = vi.fn();
vi.mock('./api/client', () => ({apiClient: {get: (...a: unknown[]) => get(...a)}}));

import {avatarBadgeLabel, avatarFrameLabel, listOwnedAvatarCosmetics} from './avatarCosmetics';

describe('avatarCosmetics', () => {
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
