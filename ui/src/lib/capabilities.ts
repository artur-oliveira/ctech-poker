import type {WalletMode} from '@/lib/api/player';

// Static export bakes public configuration into the bundle. Defaulting to
// false is intentional: the API's REAL_MONEY_ENABLED gate is off by default,
// so a missing UI flag must never advertise or request real-money history.
export const REAL_MONEY_UI_ENABLED = process.env.NEXT_PUBLIC_REAL_MONEY_ENABLED === 'true';

export function availableWalletMode(value: string | null | undefined): WalletMode {
  return value === 'real' && REAL_MONEY_UI_ENABLED ? 'real' : 'sandbox';
}
