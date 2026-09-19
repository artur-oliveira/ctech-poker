import {apiClient, type Page} from './api/client';

export type AvatarCosmeticKind = 'frame' | 'badge';
export type AvatarFrameId = 'season-2026-q3' | 'champion-gold';
export type AvatarBadgeId = 'season-2026-q3-top10' | 'season-2026-q3-champion';

/**
 * Seasonal frames/badges (#292) are always premium and always granted (never
 * sold, see api/internal/cosmetics' KindFrame/KindBadge doc comment) — there
 * is no priced catalog endpoint for them, so their small, fixed id→label set
 * lives here, the same way DECK_VARIANTS/TABLE_THEMES keep the deck/felt
 * labels client-side while the server stays the source of truth for which
 * ids exist and who owns them.
 */
export const AVATAR_FRAMES: Record<AvatarFrameId, { label: string }> = {
  'season-2026-q3': {label: 'Moldura da Temporada'},
  'champion-gold': {label: 'Moldura de Campeão'},
};

export const AVATAR_BADGES: Record<AvatarBadgeId, { label: string }> = {
  'season-2026-q3-top10': {label: 'Top 10 da Temporada'},
  'season-2026-q3-champion': {label: 'Campeão da Temporada'},
};

export function avatarFrameLabel(id: string) {
  return AVATAR_FRAMES[id as AvatarFrameId]?.label ?? id;
}

export function avatarBadgeLabel(id: string) {
  return AVATAR_BADGES[id as AvatarBadgeId]?.label ?? id;
}

/** Every id this player currently owns for kind, read from the entitlement
 * table server-side (`GET /players/me/cosmetics/:kind/owned`) — never
 * inferred client-side, same "ownership never lives in the client" rule
 * cosmeticPurchases.ts follows for deck/felt. */
export async function listOwnedAvatarCosmetics(kind: AvatarCosmeticKind) {
  return (await apiClient.get<Page<string>>(
    `/v1.0/players/me/cosmetics/${kind}/owned`, {silentError: true}
  )).data.data;
}
