import {apiClient} from './client';

export type CosmeticKind = 'deck' | 'felt';
export type CosmeticPurchaseMethod = 'pix' | 'fichas';

export interface CosmeticCatalogEntry {
  kind: CosmeticKind;
  id: string;
  premium: boolean;
  price_cents?: number;
  price_fichas?: number;
}

export interface CosmeticPurchase {
  player_id?: string;
  purchase_id: string;
  kind: CosmeticKind;
  item_id: string;
  method: CosmeticPurchaseMethod;
  price_cents?: number;
  price_fichas?: number;
  status: 'processing' | 'pending' | 'confirmed' | 'refunding' | 'refunded' | 'failed' | 'expired' | string;
  pix_copia_e_cola?: string;
  qr_code_base64?: string;
  expires_at?: string;
  created_at?: string;
  updated_at?: string;
}

export async function listCosmeticCatalog(kind: CosmeticKind) {
  return (await apiClient.get<CosmeticCatalogEntry[]>(
    `/v1.0/wallet/cosmetic-purchase/${kind}/catalog`
  )).data;
}

export async function listCosmeticPurchases(kind: CosmeticKind) {
  return (await apiClient.get<CosmeticPurchase[]>(`/v1.0/wallet/cosmetic-purchase/${kind}/`)).data;
}

export async function createCosmeticPurchase(kind: CosmeticKind, itemId: string, method: CosmeticPurchaseMethod) {
  return (await apiClient.post<CosmeticPurchase>(
    `/v1.0/wallet/cosmetic-purchase/${kind}/`,
    {item_id: itemId, method, idem_key: crypto.randomUUID()},
    {silentError: true},
  )).data;
}

export async function getCosmeticPurchase(kind: CosmeticKind, purchaseId: string) {
  return (await apiClient.get<CosmeticPurchase>(
    `/v1.0/wallet/cosmetic-purchase/${kind}/${encodeURIComponent(purchaseId)}`,
    {silentError: true},
  )).data;
}

export async function refundCosmeticPurchase(kind: CosmeticKind, purchaseId: string) {
  return (await apiClient.post<CosmeticPurchase>(
    `/v1.0/wallet/cosmetic-purchase/${kind}/${encodeURIComponent(purchaseId)}/refund`,
    {idem_key: crypto.randomUUID()},
    {silentError: true},
  )).data;
}

export function ownedCosmeticIDs(purchases: CosmeticPurchase[]) {
  return new Set([...purchases].filter(item => item.status === 'confirmed').map(item => item.item_id));
}

export function currentCosmeticPurchase(purchases: CosmeticPurchase[], itemId: string) {
  const priority: Record<string, number> = {confirmed: 5, processing: 4, pending: 3, refunding: 2};
  return purchases
    .filter(item => item.item_id === itemId)
    .sort((a, b) => (priority[b.status] ?? 0) - (priority[a.status] ?? 0) ||
      (b.updated_at || b.created_at || '').localeCompare(a.updated_at || a.created_at || ''))[0];
}
