import {apiClient} from './client';

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

export async function listSkus() {
  return (await apiClient.get<SandboxSKU[]>('/v1.0/wallet/sandbox-purchase/skus')).data;
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

export async function listPurchases() {
  return (await apiClient.get<SandboxPurchase[]>('/v1.0/wallet/sandbox-purchase/')).data;
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
