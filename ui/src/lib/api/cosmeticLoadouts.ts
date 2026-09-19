import {apiClient, type Page} from './client';

/** Kind is a plain string key here (not CosmeticKind) because the server's
 * `selections` map is generic over every cosmetics.Kind that ever exists —
 * today only "deck"/"felt" are ever populated by this client. */
export interface CosmeticLoadout {
  id: string;
  name: string;
  selections: Record<string, string>;
  created_at: string;
}

// Mirrors the server's per-player cap (internal/cosmeticloadout.maxLoadoutsPerPlayer)
// — the list is always a single, unbounded-free page, so the UI can disable
// "save" locally instead of waiting on a 409.
export const MAX_COSMETIC_LOADOUTS = 5;

export async function listCosmeticLoadouts() {
  return (await apiClient.get<Page<CosmeticLoadout>>(
    '/v1.0/players/me/cosmetic-loadouts', {silentError: true}
  )).data.data;
}

export async function createCosmeticLoadout(name: string, selections: Record<string, string>) {
  return (await apiClient.post<CosmeticLoadout>(
    '/v1.0/players/me/cosmetic-loadouts', {name, selections}
  )).data;
}

export async function deleteCosmeticLoadout(id: string) {
  await apiClient.delete(`/v1.0/players/me/cosmetic-loadouts/${encodeURIComponent(id)}`);
}

// Returns the loadout that was applied — its `selections` is what the
// preview dialog already showed before the player confirmed, so the caller
// does not need to re-derive it from the refreshed profile.
export async function applyCosmeticLoadout(id: string) {
  return (await apiClient.post<CosmeticLoadout>(
    `/v1.0/players/me/cosmetic-loadouts/${encodeURIComponent(id)}/apply`, {}
  )).data;
}
