import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {WalletAlertsSection} from './WalletAlertsSection';
import type {WalletAlertPrefs} from '@/lib/api/walletAlerts';

const {getWalletAlertPrefs, saveWalletAlertPrefs, deleteWalletAlertPrefs} = vi.hoisted(() => ({
  getWalletAlertPrefs: vi.fn(),
  saveWalletAlertPrefs: vi.fn(),
  deleteWalletAlertPrefs: vi.fn(),
}));
vi.mock('@/lib/api/walletAlerts', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/walletAlerts')>(),
  getWalletAlertPrefs, saveWalletAlertPrefs, deleteWalletAlertPrefs,
}));
const {notify} = vi.hoisted(() => ({notify: vi.fn()}));
vi.mock('@/lib/notify', () => ({pushNotification: notify}));

function wrapper({children}: {children: ReactNode}) {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

const empty: WalletAlertPrefs = {min_sandbox_balance: 0, max_purchase_cents: 0};

describe('WalletAlertsSection', () => {
  beforeEach(() => {
    getWalletAlertPrefs.mockReset();
    saveWalletAlertPrefs.mockReset();
    deleteWalletAlertPrefs.mockReset();
    notify.mockReset();
  });

  test('shows a loading state, then the empty form with no preference configured', async () => {
    getWalletAlertPrefs.mockResolvedValue(empty);
    render(<WalletAlertsSection/>, {wrapper});

    expect(screen.getByText('Carregando seus alertas…')).toBeInTheDocument();
    expect(await screen.findByRole('spinbutton', {name: /saldo de fichas/})).toHaveValue(null);
    // Nothing configured yet — no "remove" affordance to offer.
    expect(screen.queryByRole('button', {name: /Remover alertas/})).not.toBeInTheDocument();
  });

  test('shows an error state with a retry affordance when the load fails', async () => {
    getWalletAlertPrefs.mockRejectedValue(new Error('network'));
    render(<WalletAlertsSection/>, {wrapper});

    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível carregar seus alertas agora.');
    expect(screen.getByRole('button', {name: 'Tentar novamente'})).toBeInTheDocument();
  });

  test('saves both thresholds and offers to remove them once configured', async () => {
    getWalletAlertPrefs.mockResolvedValue({min_sandbox_balance: 500, max_purchase_cents: 2000});
    saveWalletAlertPrefs.mockResolvedValue({min_sandbox_balance: 500, max_purchase_cents: 5000});
    render(<WalletAlertsSection/>, {wrapper});

    const maxPurchase = await screen.findByRole('spinbutton', {name: /uma compra passar/});
    expect(maxPurchase).toHaveValue(20);
    await userEvent.clear(maxPurchase);
    await userEvent.type(maxPurchase, '50');
    await userEvent.click(screen.getByRole('button', {name: 'Salvar alertas'}));

    expect(saveWalletAlertPrefs).toHaveBeenCalledWith(
      {min_sandbox_balance: 500, max_purchase_cents: 5000}, expect.anything());
    expect(await screen.findByRole('button', {name: /Remover alertas/})).toBeInTheDocument();
  });

  test('removes a configured alert', async () => {
    getWalletAlertPrefs.mockResolvedValue({min_sandbox_balance: 500, max_purchase_cents: 0});
    deleteWalletAlertPrefs.mockResolvedValue(undefined);
    render(<WalletAlertsSection/>, {wrapper});

    await userEvent.click(await screen.findByRole('button', {name: /Remover alertas/}));

    expect(deleteWalletAlertPrefs).toHaveBeenCalled();
  });
});
