import {apiClient, type Page} from './client';

// Everything the store reads lives under this query root: balance, the
// SKU/cosmetic/reaction catalogs (which now carry the `owned` flag), and
// purchase history. A buy or a refund can move all three at once, so
// mutations invalidate the root instead of naming the subset they think they
// touched — that list is what went stale and left the store showing
// ownership that no longer existed.
export const WALLET_QUERY_ROOT = ['wallet'] as const;

export interface SandboxSKU {
  id: string;
  price_cents: number;
  base_credits: number;
  bonus_percent: number;
  total_credits: number;
}

export interface SandboxPurchase {
  player_id?: string;
  purchase_id: string;
  sku: string;
  price_cents?: number;
  base_credits?: number;
  bonus_percent?: number;
  total_credits?: number;
  status: string;
  pix_copia_e_cola?: string;
  qr_code_base64?: string;
  expires_at?: string;
  created_at?: string;
  updated_at?: string;
}

// The SKU catalog arrives whole on a single page — unwrap it and hand callers
// the plain array.
export async function listSkus() {
  return (await apiClient.get<Page<SandboxSKU>>('/v1.0/wallet/sandbox-purchase/skus')).data.data;
}

export async function createPurchase(sku: string) {
  // idem_key fresh per purchase click, stable across this click's own retries
  // — same convention as rooms.ts's joinRoom/leaveRoom.
  return (await apiClient.post<SandboxPurchase>(
    '/v1.0/wallet/sandbox-purchase/',
    {sku, idem_key: crypto.randomUUID()},
    {silentError: true},
  )).data;
}

export async function listPurchases(cursor?: string) {
  const query = cursor ? `?cursor=${encodeURIComponent(cursor)}` : '';
  return (await apiClient.get<Page<SandboxPurchase>>(`/v1.0/wallet/sandbox-purchase/${query}`)).data;
}

// "The live status of this sandbox purchase", under WALLET_QUERY_ROOT so the
// `sandbox_purchase_update` websocket frame's root invalidation already reaches
// it — the frame is the primary path, the dialog's poll only the fallback.
export function sandboxPurchaseKey(purchaseId: string) {
  return [...WALLET_QUERY_ROOT, 'sandbox-purchase', purchaseId];
}

export async function getPurchase(id: string) {
  return (await apiClient.get<SandboxPurchase>(`/v1.0/wallet/sandbox-purchase/${id}`)).data;
}

export async function refundPurchase(id: string) {
  return (await apiClient.post<SandboxPurchase>(
    `/v1.0/wallet/sandbox-purchase/${id}/refund`,
    {idem_key: crypto.randomUUID()},
    {silentError: true},
  )).data;
}
