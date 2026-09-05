import {apiClient, type Page} from './client';

export type ReactionPurchaseMethod = 'pix' | 'fichas';

export interface ReactionCatalogEntry {
  id: string;
  premium: boolean;
  // Server-computed from the entitlement table. Always true for free reactions.
  owned?: boolean;
  price_cents?: number;
  price_fichas?: number;
}

export interface ReactionPurchase {
  player_id?: string;
  purchase_id: string;
  reaction_id: string;
  method: ReactionPurchaseMethod;
  price_cents?: number;
  price_fichas?: number;
  status: 'processing' | 'pending' | 'confirmed' | 'refunding' | 'refunded' | 'failed' | 'expired' | string;
  pix_copia_e_cola?: string;
  qr_code_base64?: string;
  expires_at?: string;
  created_at?: string;
  updated_at?: string;
}

// The catalog is a fixed set, so it arrives on a single page — unwrap it and
// hand callers the plain array.
export async function listReactionCatalog() {
  return (await apiClient.get<Page<ReactionCatalogEntry>>('/v1.0/wallet/reaction-purchase/catalog')).data.data;
}

// TanStack caches by key hash alone — it does not distinguish a useQuery from
// a useInfiniteQuery. The store paginates this list while the table reads only
// its first page, so the two MUST NOT share a key: whichever mounts second
// renders the other's shape ({pages, pageParams} instead of an array) until its
// own fetch resolves, which crashed the table for anyone arriving from the
// store (and stayed crashed while the API was unreachable).
export const REACTION_PURCHASE_HISTORY_KEY = ['wallet', 'reaction-purchases', 'history'] as const;
export const REACTION_PURCHASE_FIRST_PAGE_KEY = ['wallet', 'reaction-purchases', 'first-page'] as const;

export async function listReactionPurchases(cursor?: string) {
  const query = cursor ? `?cursor=${encodeURIComponent(cursor)}` : '';
  return (await apiClient.get<Page<ReactionPurchase>>(`/v1.0/wallet/reaction-purchase/${query}`)).data;
}

export async function createReactionPurchase(reactionId: string, method: ReactionPurchaseMethod) {
  return (await apiClient.post<ReactionPurchase>(
    '/v1.0/wallet/reaction-purchase/',
    {reaction_id: reactionId, method, idem_key: crypto.randomUUID()},
    {silentError: true},
  )).data;
}

// "The live status of this reaction purchase". Under the same `['wallet', …]`
// root as the lists, so the `reaction_purchase_update` websocket frame's root
// invalidation already reaches it — the frame is the primary path, the dialog's
// poll only the fallback.
export function reactionPurchaseKey(purchaseId: string) {
  return ['wallet', 'reaction-purchase', purchaseId];
}

export async function getReactionPurchase(id: string) {
  return (await apiClient.get<ReactionPurchase>(
    `/v1.0/wallet/reaction-purchase/${encodeURIComponent(id)}`,
    {silentError: true},
  )).data;
}

export async function refundReactionPurchase(id: string) {
  return (await apiClient.post<ReactionPurchase>(
    `/v1.0/wallet/reaction-purchase/${encodeURIComponent(id)}/refund`,
    {idem_key: crypto.randomUUID()},
    {silentError: true},
  )).data;
}

// Ownership comes from the catalog's server-computed `owned` flag, which is
// backed by the entitlement table — never from purchase history. See
// ownedCosmeticIDs for the failure this avoids.
export function ownedReactionIDs(catalog: ReactionCatalogEntry[]) {
  return new Set(catalog.filter(entry => entry.owned).map(entry => entry.id));
}

export function currentReactionPurchase(purchases: ReactionPurchase[], reactionId: string) {
  const priority: Record<string, number> = {confirmed: 5, processing: 4, pending: 3, refunding: 2};
  return purchases
    .filter(item => item.reaction_id === reactionId)
    .sort((a, b) => (priority[b.status] ?? 0) - (priority[a.status] ?? 0) ||
      (b.updated_at || b.created_at || '').localeCompare(a.updated_at || a.created_at || ''))[0];
}
