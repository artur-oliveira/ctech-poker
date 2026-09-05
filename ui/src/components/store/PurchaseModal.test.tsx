import {act, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {focusManager, QueryClient, QueryClientProvider} from '@tanstack/react-query';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {PurchaseModal} from './PurchaseModal';
import type {SandboxPurchase} from '@/lib/api/wallet';

const {getPurchase} = vi.hoisted(() => ({getPurchase: vi.fn()}));
vi.mock('@/lib/api/wallet', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/wallet')>(), getPurchase,
}));
vi.mock('next/image', () => ({default: ({alt}: {alt: string}) => <div role="img" aria-label={alt}/>}));

const purchase = (overrides: Partial<SandboxPurchase> = {}): SandboxPurchase => ({
  purchase_id: 'sbxp-1', sku: 'pack_100', status: 'pending',
  qr_code_base64: 'aGVsbG8=', pix_copia_e_cola: '00020126',
  expires_at: new Date(Date.now() + 5 * 60_000).toISOString(), ...overrides,
});

const wrapper = ({children}: {children: ReactNode}) => {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

const renderModal = (value: SandboxPurchase | null, props: Partial<{
  onCloseAction: () => void; onUpdateAction: (purchase: SandboxPurchase) => void;
  onRegenerateAction: (sku: string) => Promise<void>;
}> = {}) => render(<PurchaseModal purchase={value} onCloseAction={vi.fn()} onUpdateAction={vi.fn()}
  onRegenerateAction={vi.fn().mockResolvedValue(undefined)} {...props}/>, {wrapper});

describe('PurchaseModal', () => {
  beforeEach(() => getPurchase.mockReset());

  test('stays closed without a purchase', () => {
    renderModal(null);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  test('shows the Pix instructions while the payment is pending', () => {
    renderModal(purchase());
    expect(screen.getByText('Pague com Pix para concluir')).toBeInTheDocument();
    expect(screen.getByLabelText('Pix copia e cola')).toHaveValue('00020126');
  });

  test('celebrates a confirmed payment with the credited amount', () => {
    renderModal(purchase({status: 'confirmed', total_credits: 12_000}));
    expect(screen.getByText('Fichas adicionadas')).toBeInTheDocument();
    expect(screen.getByText('12.000 fichas sandbox já estão no seu saldo.')).toBeInTheDocument();
  });

  test('confirms without an amount when the server did not publish one', () => {
    renderModal(purchase({status: 'confirmed'}));
    expect(screen.getByText('Suas fichas sandbox já estão no saldo.')).toBeInTheDocument();
  });

  test.each([
    ['refunded', 'estornada'],
    ['failed', 'falhou'],
    ['something_new', 'status desconhecido'],
  ])('closes out a %s purchase without offering payment', (status, label) => {
    renderModal(purchase({status, expires_at: undefined}));
    expect(screen.getByText('Compra encerrada')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent(`(${label})`);
    expect(screen.queryByLabelText('Pix copia e cola')).not.toBeInTheDocument();
  });

  test('offers a fresh Pix once the code expires and reports a failure to mint one', async () => {
    const onRegenerateAction = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(undefined);
    renderModal(purchase({status: 'expired', expires_at: undefined}), {onRegenerateAction});
    expect(screen.getByText('Código Pix expirado')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', {name: 'Gerar novo Pix para este pacote'}));
    expect(onRegenerateAction).toHaveBeenCalledWith('pack_100');
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível gerar um novo Pix.');

    await userEvent.click(screen.getByRole('button', {name: 'Gerar novo Pix para este pacote'}));
    expect(onRegenerateAction).toHaveBeenCalledTimes(2);
  });

  test('treats a countdown that already elapsed as expired', () => {
    renderModal(purchase({expires_at: new Date(Date.now() - 1000).toISOString()}));
    expect(screen.getByText('Código Pix expirado')).toBeInTheDocument();
  });

  test('cannot regenerate a purchase that carries no sku', () => {
    renderModal(purchase({status: 'expired', sku: '', expires_at: undefined}));
    expect(screen.getByRole('button', {name: 'Gerar novo Pix para este pacote'})).toBeDisabled();
  });

  test('closes back to the packages list', async () => {
    const onCloseAction = vi.fn();
    renderModal(purchase({status: 'expired', expires_at: undefined}), {onCloseAction});
    await userEvent.click(screen.getByRole('button', {name: 'Voltar aos pacotes'}));
    expect(onCloseAction).toHaveBeenCalledOnce();
  });

  test('polls the pending purchase and reports the confirmation upwards', async () => {
    vi.useFakeTimers();
    try {
      const onUpdateAction = vi.fn();
      const confirmed = purchase({status: 'confirmed'});
      getPurchase.mockResolvedValue(confirmed);
      renderModal(purchase(), {onUpdateAction});

      await vi.advanceTimersByTimeAsync(5000);
      expect(getPurchase).toHaveBeenCalledWith('sbxp-1');
      expect(onUpdateAction).toHaveBeenCalledWith(confirmed);
    } finally {
      vi.useRealTimers();
    }
  });

  test('spends nothing while the tab is hidden, and one read on coming back', async () => {
    vi.useFakeTimers();
    try {
      getPurchase.mockResolvedValue(purchase());
      renderModal(purchase());

      focusManager.setFocused(false);
      await vi.advanceTimersByTimeAsync(60_000);
      expect(getPurchase).not.toHaveBeenCalled();

      focusManager.setFocused(true);
      await vi.waitFor(() => expect(getPurchase).toHaveBeenCalledTimes(1));
      expect(getPurchase).toHaveBeenCalledTimes(1);
    } finally {
      focusManager.setFocused(undefined);
      vi.useRealTimers();
    }
  });

  test('offers a manual check when the poll fails, and recovers from it', async () => {
    vi.useFakeTimers();
    try {
      const onUpdateAction = vi.fn();
      getPurchase.mockRejectedValueOnce(new Error('offline'));
      renderModal(purchase(), {onUpdateAction});

      await vi.advanceTimersByTimeAsync(5000);
      await vi.waitFor(() => expect(screen.getByRole('alert'))
        .toHaveTextContent('Seu pagamento não foi alterado.'));

      const confirmed = purchase({status: 'confirmed'});
      getPurchase.mockResolvedValueOnce(confirmed);
      act(() => void screen.getByRole('button', {name: 'Verificar pagamento'}).click());
      await vi.waitFor(() => expect(onUpdateAction).toHaveBeenCalledWith(confirmed));
    } finally {
      vi.useRealTimers();
    }
  });

  test('keeps the failure visible when the manual check also fails', async () => {
    vi.useFakeTimers();
    try {
      getPurchase.mockRejectedValueOnce(new Error('offline')).mockRejectedValueOnce(new Error('offline'));
      renderModal(purchase());

      await vi.advanceTimersByTimeAsync(5000);
      await vi.waitFor(() => expect(screen.getByRole('button', {name: 'Verificar pagamento'})).toBeInTheDocument());
      act(() => void screen.getByRole('button', {name: 'Verificar pagamento'}).click());
      await vi.waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());
    } finally {
      vi.useRealTimers();
    }
  });
});
