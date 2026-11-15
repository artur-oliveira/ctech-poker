import {apiClient, type Page} from './api/client';

export type AvatarSlot = 'frame' | 'hat' | 'accessory' | 'badge';
export type AvatarCosmeticKind = AvatarSlot;

/**
 * Avatar cosmetics' small, fixed id→label set per slot (frames, hats,
 * accessories, badges), kept client-side the same way DECK_VARIANTS/
 * TABLE_THEMES keep the deck/felt labels while the server stays the source
 * of truth for which ids exist and who owns them. Labels are pt-BR (no i18n
 * library). Art for each id lives in components/avatar/*; see DESIGN.md
 * "Avatar cosmetics art contract".
 */
export const AVATAR_ITEMS: Record<AvatarSlot, Record<string, { label: string }>> = {
  frame: {
    'season-2026-q3': {label: 'Moldura da Temporada'},
    'champion-gold': {label: 'Moldura de Campeão'},
    'chip-ring': {label: 'Borda de Fichas'},
    'felt-ring': {label: 'Borda de Feltro'},
    'walnut-rail': {label: 'Borda de Nogueira'},
    'oxblood-edge': {label: 'Borda Oxblood'},
    'silver-filigree': {label: 'Filigrana de Prata'},
    'pearl-beads': {label: 'Colar de Pérolas'},
    'rose-gold': {label: 'Ouro Rosé'},
    'obsidian-edge': {label: 'Borda de Obsidiana'},
    'emerald-cut': {label: 'Esmeralda'},
    'sapphire-deco': {label: 'Art Déco Safira'},
    'olive-branch': {label: 'Ramo de Oliveira'},
    'suit-ring': {label: 'Naipes'},
  },
  hat: {
    'dealer-visor': {label: 'Viseira do Dealer'},
    'crown': {label: 'Coroa'},
    'laurel': {label: 'Louros'},
    'top-hat': {label: 'Cartola'},
    'cowboy': {label: 'Chapéu de Cowboy'},
    'beanie': {label: 'Gorro'},
    'fedora': {label: 'Fedora'},
    'beret': {label: 'Boina'},
    'bowler': {label: 'Chapéu-coco'},
    'captain-cap': {label: 'Boné de Capitão'},
    'wizard': {label: 'Chapéu de Mago'},
    'sun-visor': {label: 'Viseira'},
  },
  accessory: {
    'bandit-mask': {label: 'Máscara de Bandido'},
    'shades': {label: 'Óculos Escuros'},
    'monocle': {label: 'Monóculo'},
    'mustache': {label: 'Bigode'},
    'headset': {label: 'Headset'},
    'round-glasses': {label: 'Óculos Redondos'},
    'eyepatch': {label: 'Tapa-olho'},
    'cigar': {label: 'Charuto'},
    'handlebar': {label: 'Bigode Francês'},
    'earring': {label: 'Brinco'},
  },
  badge: {
    'season-2026-q3-top10': {label: 'Top 10 da Temporada'},
    'season-2026-q3-champion': {label: 'Campeão da Temporada'},
    'all-in-hero': {label: 'Herói do All-in'},
    'iron-nerve': {label: 'Sangue Frio'},
    'rainmaker': {label: 'Chuva de Fichas'},
    'veteran': {label: 'Veterano'},
    'social-butterfly': {label: 'Anfitrião'},
  },
};

/** Unknown id (new server item, old client) falls back to the raw id. */
export function avatarItemLabel(slot: AvatarSlot, id: string): string {
  return AVATAR_ITEMS[slot]?.[id]?.label ?? id;
}

/** The equipped cosmetics phase 1 adds to every REST payload that decorates
 * a player (leaderboard entries, showcase, people, hand opponents,
 * `/players/me`). All optional: an older server or a bare player sends none. */
export interface EquippedCosmetics {
  equipped_frame_id?: string;
  equipped_hat_id?: string;
  equipped_accessory_id?: string;
  equipped_badge_ids?: string[];
}

/** REST `equipped_*` fields to `PlayerAvatar`'s props. `badgeIds` keeps the
 * payload's array reference (query data is structurally shared). */
export const decorProps = (e?: EquippedCosmetics) => ({
  frameId: e?.equipped_frame_id,
  hatId: e?.equipped_hat_id,
  accessoryId: e?.equipped_accessory_id,
  badgeIds: e?.equipped_badge_ids,
});

export const avatarFrameLabel = (id: string) => avatarItemLabel('frame', id);
export const avatarBadgeLabel = (id: string) => avatarItemLabel('badge', id);

/** Every id this player currently owns for kind, read from the entitlement
 * table server-side (`GET /players/me/cosmetics/:kind/owned`) — never
 * inferred client-side, same "ownership never lives in the client" rule
 * cosmeticPurchases.ts follows for deck/felt. */
export async function listOwnedAvatarCosmetics(kind: AvatarCosmeticKind) {
  return (await apiClient.get<Page<string>>(
    `/v1.0/players/me/cosmetics/${kind}/owned`, {silentError: true}
  )).data.data;
}
