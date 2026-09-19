import {ApiError, apiClient, type Page} from './client';

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

// The one welcome-pack SKU id, at most one purchase per player ever (#348).
// The catalog carries no "already used" flag — the store page derives it from
// whether a completed purchase of this sku already exists in the loaded
// history, and the purchase itself is the final authority (a stale/absent
// history read is answered with a rejected purchase, not a silent success).
export const WELCOME_PACK_SKU = 'welcome_pack';

export interface SandboxPurchase {
  player_id?: string;
  purchase_id: string;
  sku: string;
  price_cents?: number;
  base_credits?: number;
  bonus_percent?: number;
  total_credits?: number;
  status: string;
  promo_code?: string;
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

// promoCode, when given, is redeemed server-side and its own SKU overrides
// sku entirely (see api/CLAUDE.md's sandboxpurchase — the client never picks
// both a cheaper sku and a promo's discount). Callers that only want to
// redeem a code pass an empty sku.
export async function createPurchase(sku: string, promoCode?: string) {
  // idem_key fresh per purchase click, stable across this click's own retries
  // — same convention as rooms.ts's joinRoom/leaveRoom.
  return (await apiClient.post<SandboxPurchase>(
    '/v1.0/wallet/sandbox-purchase/',
    {sku, promo_code: promoCode || undefined, idem_key: crypto.randomUUID()},
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

// Maps the backend's internal/promocode error strings (see
// api/internal/promocode/store.go) to player-facing pt-BR copy. Falls back to
// a generic message for anything else (network error, unrelated 500, …) so a
// future backend error text never leaks English into the dialog.
export function promoRedeemErrorMessage(error: unknown): string {
  const detail = error instanceof ApiError ? error.problem?.detail ?? error.message : '';
  if (detail.includes('already redeemed')) return 'Este código já foi usado na sua conta.';
  if (detail.includes('expired')) return 'Este código promocional expirou.';
  if (detail.includes('redemption limit')) return 'Este código atingiu o limite de resgates.';
  if (detail.includes('not found')) return 'Código promocional inválido.';
  return 'Não foi possível aplicar o código agora. Tente novamente.';
}

// Same mapping, phrased for the welcome pack's own once-per-player claim
// (same ErrAlreadyRedeemed on the backend, different SKU — internal/promocode
// makes no distinction, so the copy has to).
export function welcomePackErrorMessage(error: unknown): string {
  const detail = error instanceof ApiError ? error.problem?.detail ?? error.message : '';
  if (detail.includes('already redeemed')) return 'Você já resgatou o pacote de boas-vindas antes.';
  return 'Não foi possível iniciar o pacote de boas-vindas agora. Tente novamente.';
}
