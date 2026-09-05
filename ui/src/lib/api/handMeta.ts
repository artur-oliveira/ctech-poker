import {apiClient} from './client';

export const HAND_META_STREETS = ['preflop', 'flop', 'turn', 'river', 'showdown'] as const;
export type HandMetaStreet = typeof HAND_META_STREETS[number];

export interface HandMeta {
  hand_id: string;
  street_notes?: Partial<Record<HandMetaStreet, string>>;
  review_marked: boolean;
  collections?: string[];
  updated_at?: string;
}

export interface SavedHandFilter {
  name: string;
  outcome: string;
  table_id: string;
}

/** Query key for one hand's metadata (#349/#347: street notes, review
 * marker and collections all live on the same record). */
export const HAND_META_KEY = (handId: string) => ['hand-meta', handId] as const;
export const HAND_COLLECTIONS_KEY = ['hand-collections'] as const;
export const SAVED_HAND_FILTERS_KEY = ['hand-filters'] as const;

export async function getHandMeta(handId: string) {
  return (await apiClient.get<HandMeta>(
    `/v1.0/players/me/hands/${encodeURIComponent(handId)}/meta`, {silentError: true}
  )).data;
}

export async function saveHandMeta(handId: string, input: {
  street_notes?: Partial<Record<HandMetaStreet, string>>;
  review_marked: boolean;
  collections?: string[];
}) {
  return (await apiClient.put<HandMeta>(
    `/v1.0/players/me/hands/${encodeURIComponent(handId)}/meta`, input
  )).data;
}

/** Every hand the player marked for review or filed into a collection —
 * backs the /hands "Coleções" tab. */
export async function listHandCollections() {
  return (await apiClient.get<{ data: HandMeta[] }>('/v1.0/players/me/hand-collections')).data.data;
}

export async function getSavedHandFilters() {
  return (await apiClient.get<{ data: SavedHandFilter[] }>('/v1.0/players/me/hand-filters')).data.data;
}

export async function saveSavedHandFilters(filters: SavedHandFilter[]) {
  return (await apiClient.put<{ data: SavedHandFilter[] }>('/v1.0/players/me/hand-filters', {filters})).data.data;
}
