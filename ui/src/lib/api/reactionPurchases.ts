import {apiClient} from './client';

export type ReactionPurchaseMethod = 'pix' | 'fichas';

export interface ReactionCatalogEntry {
  id: string;
  premium: boolean;
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

export async function listReactionCatalog() {
  return (await apiClient.get<ReactionCatalogEntry[]>('/v1.0/wallet/reaction-purchase/catalog')).data;
}

export async function listReactionPurchases() {
  return (await apiClient.get<ReactionPurchase[]>('/v1.0/wallet/reaction-purchase/')).data;
}

export async function createReactionPurchase(reactionId: string, method: ReactionPurchaseMethod) {
  return (await apiClient.post<ReactionPurchase>(
    '/v1.0/wallet/reaction-purchase/',
    {reaction_id: reactionId, method, idem_key: crypto.randomUUID()},
    {silentError: true},
  )).data;
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

export function ownedReactionIDs(purchases: ReactionPurchase[]) {
  return new Set(purchases.filter(item => item.status === 'confirmed').map(item => item.reaction_id));
}

export function currentReactionPurchase(purchases: ReactionPurchase[], reactionId: string) {
  const priority: Record<string, number> = {confirmed: 5, processing: 4, pending: 3, refunding: 2};
  return purchases
    .filter(item => item.reaction_id === reactionId)
    .sort((a, b) => (priority[b.status] ?? 0) - (priority[a.status] ?? 0) ||
      (b.updated_at || b.created_at || '').localeCompare(a.updated_at || a.created_at || ''))[0];
}
