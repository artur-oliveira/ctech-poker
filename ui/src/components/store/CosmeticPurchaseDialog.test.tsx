import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {CosmeticPurchaseDialog} from './CosmeticPurchaseDialog';
import {ApiError} from '@/lib/api/client';
import type {CosmeticCatalogEntry, CosmeticPurchase} from '@/lib/api/cosmeticPurchases';

const {createCosmeticPurchase, getCosmeticPurchase} = vi.hoisted(() => ({
  createCosmeticPurchase: vi.fn(), getCosmeticPurchase: vi.fn(),
}));
vi.mock('@/lib/api/cosmeticPurchases', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/cosmeticPurchases')>(),
  createCosmeticPurchase, getCosmeticPurchase,
}));
vi.mock('next/image', () => ({default: ({alt}: {alt: string}) => <div role="img" aria-label={alt}/>}));

const entry: CosmeticCatalogEntry = {kind: 'deck', id: 'golden', premium: true, price_cents: 2990, price_fichas: 500_000};

const purchase = (overrides: Partial<CosmeticPurchase> = {}): CosmeticPurchase => ({
  purchase_id: 'p1', kind: 'deck', item_id: 'golden', method: 'pix', status: 'confirmed',
  price_cents: 2990, price_fichas: 500_000, ...overrides,
});

const wrapper = ({children}: {children: ReactNode}) => {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

describe('CosmeticPurchaseDialog', () => {
  beforeEach(() => {
    createCosmeticPurchase.mockReset();
    getCosmeticPurchase.mockReset();
  });

  test('renders nothing for an unknown item', () => {
    const {container} = render(<CosmeticPurchaseDialog kind="deck" entry={null} onCloseAction={vi.fn()}/>, {wrapper});
    expect(container).toBeEmptyDOMElement();
    render(<CosmeticPurchaseDialog kind="deck" entry={{kind: 'deck', id: 'nope', premium: true}}
      onCloseAction={vi.fn()}/>, {wrapper});
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  test('buys a deck with fichas and reports the confirmed purchase', async () => {
    const confirmed = purchase({method: 'fichas', status: 'confirmed'});
    createCosmeticPurchase.mockResolvedValue(confirmed);
    const onConfirmedAction = vi.fn();
    render(<CosmeticPurchaseDialog kind="deck" entry={entry} sandboxBalance={900_000} onCloseAction={vi.fn()}
      onConfirmedAction={onConfirmedAction}/>, {wrapper});

    await userEvent.click(screen.getByRole('button', {name: /500.000 fichas/}));
    expect(createCosmeticPurchase).toHaveBeenCalledWith('deck', 'golden', 'fichas');
    expect(await screen.findByText('Pronto para a mesa')).toBeInTheDocument();
    expect(onConfirmedAction).toHaveBeenCalledWith(confirmed);
  });

  test('blocks the fichas option when the sandbox balance is short', () => {
    render(<CosmeticPurchaseDialog kind="deck" entry={entry} sandboxBalance={10} onCloseAction={vi.fn()}/>, {wrapper});
    expect(screen.getByRole('button', {name: /500.000 fichas/})).toBeDisabled();
    expect(screen.getByText('Saldo sandbox insuficiente')).toBeInTheDocument();
  });

  test('starts a Pix purchase and shows the QR code with the cosmetic-only note', async () => {
    createCosmeticPurchase.mockResolvedValue(purchase({
      status: 'pending', qr_code_base64: 'PHN2Zy', pix_copia_e_cola: '000201'
    }));
    render(<CosmeticPurchaseDialog kind="deck" entry={entry} onCloseAction={vi.fn()}/>, {wrapper});

    await userEvent.click(screen.getByRole('button', {name: /via Pix/}));
    expect(await screen.findByText(/Aguardando confirmação do Pix/)).toBeInTheDocument();
    expect(screen.getByLabelText('Pix copia e cola')).toHaveValue('000201');
    expect(screen.getByText(/não altera fichas nem saldo de jogo/)).toBeInTheDocument();
  });

  test('polls a pending felt purchase until the server confirms it', async () => {
    vi.useFakeTimers();
    try {
      getCosmeticPurchase.mockResolvedValue(purchase({kind: 'felt', item_id: 'midnight', status: 'confirmed'}));
      render(<CosmeticPurchaseDialog kind="felt" entry={{kind: 'felt', id: 'midnight', premium: true, price_fichas: 200_000}}
        initialPurchase={purchase({kind: 'felt', item_id: 'midnight', status: 'pending'})}
        onCloseAction={vi.fn()}/>, {wrapper});

      expect(screen.getByText(/Aguardando confirmação do Pix/)).toBeInTheDocument();
      await vi.advanceTimersByTimeAsync(4000);
      expect(getCosmeticPurchase).toHaveBeenCalledWith('felt', 'p1');
      await vi.waitFor(() => expect(screen.getByText('Pronto para a mesa')).toBeInTheDocument());
    } finally {
      vi.useRealTimers();
    }
  });

  test.each([
    [409, /já é seu ou existe uma compra dele em andamento/],
    [400, 'Não foi possível usar este meio de pagamento para este item.'],
    [500, 'Não foi possível iniciar a compra agora. Tente novamente.'],
  ])('explains a %s failure without leaving the dialog stuck', async (status, message) => {
    createCosmeticPurchase.mockRejectedValue(new ApiError('failed', status));
    render(<CosmeticPurchaseDialog kind="deck" entry={entry} onCloseAction={vi.fn()}/>, {wrapper});
    await userEvent.click(screen.getByRole('button', {name: /via Pix/}));
    expect(await screen.findByRole('alert')).toHaveTextContent(message);
    expect(screen.getByRole('button', {name: /via Pix/})).toBeEnabled();
  });

  test('an expired Pix offers a fresh one instead of a dead QR code', async () => {
    createCosmeticPurchase.mockResolvedValue(purchase({status: 'pending'}));
    render(<CosmeticPurchaseDialog kind="deck" entry={entry} initialPurchase={purchase({status: 'expired'})}
      onCloseAction={vi.fn()}/>, {wrapper});
    expect(screen.getByRole('alert')).toHaveTextContent('Este Pix expirou.');
    await userEvent.click(screen.getByRole('button', {name: 'Gerar novo Pix'}));
    expect(createCosmeticPurchase).toHaveBeenCalledWith('deck', 'golden', 'pix');
  });

  test('closes only when no purchase call is in flight', async () => {
    const onCloseAction = vi.fn();
    render(<CosmeticPurchaseDialog kind="deck" entry={entry}
      initialPurchase={purchase({status: 'processing', method: 'fichas'})}
      onCloseAction={onCloseAction}/>, {wrapper});
    expect(screen.getByText(/Confirmando o débito de fichas/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Fechar e acompanhar depois'}));
    expect(onCloseAction).toHaveBeenCalledOnce();
  });
});
