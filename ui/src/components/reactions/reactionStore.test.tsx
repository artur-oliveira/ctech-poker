import {act, render, screen, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {reactionActivityRows, ReactionStoreSection} from './ReactionStoreSection';
import {PurchaseActivityList} from '@/components/store/PurchaseActivityList';
import {ReactionPurchaseDialog} from './ReactionPurchaseDialog';
import {ReactionRefundDialog} from './ReactionRefundDialog';
import {ReactionFavoritesDialog} from './ReactionFavoritesDialog';
import {ApiError} from '@/lib/api/client';
import type {ReactionCatalogEntry, ReactionPurchase} from '@/lib/api/reactionPurchases';
import type {TableReactionID} from '@/lib/reactions';

const {createReactionPurchase, getReactionPurchase} = vi.hoisted(() => ({
  createReactionPurchase: vi.fn(), getReactionPurchase: vi.fn(),
}));
vi.mock('@/lib/api/reactionPurchases', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/reactionPurchases')>(),
  createReactionPurchase, getReactionPurchase,
}));
vi.mock('next/image', () => ({default: ({alt}: {alt: string}) => <div role="img" aria-label={alt}/>}));

const catalog: ReactionCatalogEntry[] = [
  {id: 'fire', premium: true, owned: false, price_cents: 990, price_fichas: 5000},
  {id: 'cold', premium: true, owned: false, price_cents: 490, price_fichas: 2500},
  {id: 'clap', premium: false, owned: true, price_cents: 0, price_fichas: 0},
  {id: 'ghost-reaction', premium: true, owned: false, price_cents: 100},
];

// Ownership is a catalog fact now (the server reads it from entitlements), so
// "owning fire" is a different catalog, not a different purchase list.
const ownedFireCatalog = catalog.map(entry => entry.id === 'fire' ? {...entry, owned: true} : entry);

const purchase = (overrides: Partial<ReactionPurchase> = {}): ReactionPurchase => ({
  purchase_id: 'p1', reaction_id: 'fire', method: 'pix', status: 'confirmed',
  price_cents: 990, price_fichas: 5000, ...overrides,
});

const wrapper = ({children}: {children: ReactNode}) => {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

describe('ReactionStoreSection', () => {
  const actions = {
    onRetryAction: vi.fn(), onBuyAction: vi.fn(), onRefundAction: vi.fn(), onResumeAction: vi.fn(),
  };

  beforeEach(() => Object.values(actions).forEach(action => action.mockReset()));

  const renderSection = (purchases: ReactionPurchase[] = [], overrides = {}) =>
    render(<ReactionStoreSection catalog={catalog} purchases={purchases} isLoading={false} isError={false}
      {...actions} {...overrides}/>);

  test('shows a skeleton while the catalog loads', () => {
    renderSection([], {isLoading: true});
    expect(screen.getByText('Carregando reações premium…')).toBeInTheDocument();
  });

  test('offers a retry when the catalog fails', async () => {
    renderSection([], {isError: true});
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(actions.onRetryAction).toHaveBeenCalledOnce();
  });

  test('falls back to an empty state when nothing premium is on sale', () => {
    renderSection([], {catalog: [{id: 'clap', premium: false, owned: true}]});
    expect(screen.getByText('Nenhuma reação premium disponível no momento.')).toBeInTheDocument();
  });

  test('lists only premium entries the client knows how to render', () => {
    renderSection();
    const items = within(screen.getByRole('list', {name: 'Catálogo de reações premium'})).getAllByRole('listitem');
    expect(items).toHaveLength(2);
    expect(within(items[0]).getByText('Pegando fogo')).toBeInTheDocument();
    expect(within(items[0]).getByText(/R\$\s?9,90/)).toBeInTheDocument();
    expect(within(items[0]).getByText(/5\.000 fichas/)).toBeInTheDocument();
  });

  test('a locked reaction can be bought', async () => {
    renderSection();
    expect(screen.getAllByText('Não liberada')).toHaveLength(2);
    await userEvent.click(screen.getAllByRole('button', {name: 'Liberar'})[0]);
    expect(actions.onBuyAction).toHaveBeenCalledWith(catalog[0], expect.any(HTMLButtonElement));
  });

  test('an owned reaction can be refunded instead of bought again', async () => {
    renderSection([purchase()], {catalog: ownedFireCatalog});
    expect(screen.getByText('Sua')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Estornar/}));
    expect(actions.onRefundAction).toHaveBeenCalledWith(expect.objectContaining({purchase_id: 'p1'}),
      expect.any(HTMLButtonElement));
  });

  test('a pending purchase resumes instead of starting a second one', async () => {
    const pending = purchase({status: 'pending'});
    renderSection([pending]);
    expect(screen.getByText('Aguardando Pix')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Acompanhar/}));
    expect(actions.onResumeAction).toHaveBeenCalledWith(catalog[0], pending, expect.any(HTMLButtonElement));
    expect(actions.onBuyAction).not.toHaveBeenCalled();
  });

  test('a refunding purchase blocks every action until the server settles it', () => {
    renderSection([purchase({status: 'refunding'})]);
    expect(screen.getByText('Estornando')).toBeInTheDocument();
    expect(screen.getByText('Aguarde')).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /Estornar/})).not.toBeInTheDocument();
  });

  test('picks the highest-priority purchase when a reaction has several', async () => {
    renderSection([purchase({purchase_id: 'old', status: 'refunded'}), purchase({purchase_id: 'new'})],
      {catalog: ownedFireCatalog});
    expect(screen.getByText('Sua')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Estornar/}));
    expect(actions.onRefundAction).toHaveBeenCalledWith(expect.objectContaining({purchase_id: 'new'}),
      expect.any(HTMLButtonElement));
  });

  test('an owned reaction whose receipt is not on this page of history offers no refund button', () => {
    // Ownership comes from the catalog and history is paginated, so the two can
    // legitimately disagree; the owned state must still render.
    renderSection([], {catalog: ownedFireCatalog});
    const items = within(screen.getByRole('list', {name: 'Catálogo de reações premium'})).getAllByRole('listitem');
    const owned = items.find(item => within(item).queryByText('Sua'));
    expect(owned).toBeDefined();
    expect(within(owned!).queryByRole('button')).not.toBeInTheDocument();
  });
});

const activity = (purchases: ReactionPurchase[] = [], overrides = {}) =>
  <PurchaseActivityList rows={reactionActivityRows(purchases)}
    loadingLabel="Carregando compras de reações…"
    errorLabel="Não foi possível carregar as compras de reações."
    emptyLabel="Suas compras de reações aparecerão aqui." {...overrides}/>;

describe('reaction purchase activity', () => {
  test('renders loading, error and empty states', async () => {
    const onRetryAction = vi.fn();
    const {rerender} = render(activity([], {isLoading: true}));
    expect(screen.getByText('Carregando compras de reações…')).toBeInTheDocument();

    rerender(activity([], {isError: true, onRetryAction}));
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(onRetryAction).toHaveBeenCalledOnce();

    rerender(activity([], {isError: true}));
    expect(screen.queryByRole('button', {name: 'Tentar novamente'})).not.toBeInTheDocument();

    rerender(activity([]));
    expect(screen.getByText('Suas compras de reações aparecerão aqui.')).toBeInTheDocument();
  });

  test('shows the four most recent purchases newest first', () => {
    render(activity([1, 2, 3, 4, 5].map(index => purchase({
      purchase_id: `p${index}`, reaction_id: 'fire', updated_at: `2026-08-0${index}T10:00:00Z`,
    }))));
    const items = screen.getAllByRole('listitem');
    expect(items).toHaveLength(4);
    expect(within(items[0]).getByText(/05 de ago/)).toBeInTheDocument();
  });

  test('degrades gracefully for an unknown reaction, method and status', () => {
    render(activity([purchase({
      reaction_id: 'unknown-reaction', method: 'fichas', status: 'weird', updated_at: 'not-a-date', created_at: '',
    })]));
    expect(screen.getByText('unknown-reaction')).toBeInTheDocument();
    expect(screen.getByText('Fichas')).toBeInTheDocument();
    expect(screen.getByText('Atualizando')).toBeInTheDocument();
  });
});

describe('ReactionPurchaseDialog', () => {
  beforeEach(() => {
    createReactionPurchase.mockReset();
    getReactionPurchase.mockReset();
  });

  const entry = catalog[0];

  test('renders nothing for an unknown reaction', () => {
    const {container} = render(<ReactionPurchaseDialog entry={null} onCloseAction={vi.fn()}/>, {wrapper});
    expect(container).toBeEmptyDOMElement();
    render(<ReactionPurchaseDialog entry={{id: 'nope', premium: true, owned: false}} onCloseAction={vi.fn()}/>, {wrapper});
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  test('buys with fichas and reports the confirmed purchase', async () => {
    const confirmed = purchase({method: 'fichas', status: 'confirmed'});
    createReactionPurchase.mockResolvedValue(confirmed);
    const onConfirmedAction = vi.fn();
    render(<ReactionPurchaseDialog entry={entry} sandboxBalance={9000} onCloseAction={vi.fn()}
      onConfirmedAction={onConfirmedAction}/>, {wrapper});

    await userEvent.click(screen.getByRole('button', {name: /5.000 fichas/}));
    expect(createReactionPurchase).toHaveBeenCalledWith('fire', 'fichas');
    expect(await screen.findByText('Pronta para a mesa')).toBeInTheDocument();
    expect(onConfirmedAction).toHaveBeenCalledWith(confirmed);
  });

  test('blocks the fichas option when the sandbox balance is short', () => {
    render(<ReactionPurchaseDialog entry={entry} sandboxBalance={10} onCloseAction={vi.fn()}/>, {wrapper});
    expect(screen.getByRole('button', {name: /5.000 fichas/})).toBeDisabled();
    expect(screen.getByText('Saldo sandbox insuficiente')).toBeInTheDocument();
  });

  test('starts a Pix purchase and shows the QR code with the cosmetic-only note', async () => {
    createReactionPurchase.mockResolvedValue(purchase({status: 'pending', qr_code_base64: 'PHN2Zy', pix_copia_e_cola: '000201'}));
    render(<ReactionPurchaseDialog entry={entry} onCloseAction={vi.fn()}/>, {wrapper});

    await userEvent.click(screen.getByRole('button', {name: /via Pix/}));
    expect(await screen.findByText(/Aguardando confirmação do Pix/)).toBeInTheDocument();
    expect(screen.getByLabelText('QR code Pix para pagamento')).toBeInTheDocument();
    expect(screen.getByLabelText('Pix copia e cola')).toHaveValue('000201');
    expect(screen.getByText(/não altera fichas nem saldo de jogo/)).toBeInTheDocument();
  });

  test('polls the pending purchase until the server confirms it', async () => {
    vi.useFakeTimers();
    try {
      getReactionPurchase.mockResolvedValue(purchase({status: 'confirmed'}));
      render(<ReactionPurchaseDialog entry={entry} initialPurchase={purchase({status: 'pending'})}
        onCloseAction={vi.fn()}/>, {wrapper});

      expect(screen.getByText(/Aguardando confirmação do Pix/)).toBeInTheDocument();
      await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
      expect(getReactionPurchase).toHaveBeenCalledWith('p1');
      await vi.waitFor(() => expect(screen.getByText('Pronta para a mesa')).toBeInTheDocument());
    } finally {
      vi.useRealTimers();
    }
  });

  test('surfaces a poll failure with a manual retry', async () => {
    getReactionPurchase.mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(purchase({status: 'confirmed'}));
    vi.useFakeTimers();
    try {
      render(<ReactionPurchaseDialog entry={entry} initialPurchase={purchase({status: 'processing'})}
        onCloseAction={vi.fn()}/>, {wrapper});
      await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
      await vi.waitFor(() => expect(screen.getByText('Não foi possível atualizar a confirmação.')).toBeInTheDocument());
      await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
      await vi.waitFor(() => expect(screen.getByText('Pronta para a mesa')).toBeInTheDocument());
    } finally {
      vi.useRealTimers();
    }
  });

  test.each([
    [409, /já é sua ou existe uma compra dela em andamento/],
    [400, 'Não foi possível usar este meio de pagamento para a reação.'],
    [500, 'Não foi possível iniciar a compra agora. Tente novamente.'],
  ])('explains a %s failure without leaving the dialog stuck', async (status, message) => {
    createReactionPurchase.mockRejectedValue(new ApiError('failed', status));
    render(<ReactionPurchaseDialog entry={entry} onCloseAction={vi.fn()}/>, {wrapper});
    await userEvent.click(screen.getByRole('button', {name: /via Pix/}));
    expect(await screen.findByRole('alert')).toHaveTextContent(message);
    expect(screen.getByRole('button', {name: /via Pix/})).toBeEnabled();
  });

  test('an expired Pix offers a fresh one instead of a dead QR code', async () => {
    createReactionPurchase.mockResolvedValue(purchase({status: 'pending'}));
    render(<ReactionPurchaseDialog entry={entry} initialPurchase={purchase({status: 'expired'})}
      onCloseAction={vi.fn()}/>, {wrapper});
    expect(screen.getByRole('alert')).toHaveTextContent('Este Pix expirou.');
    await userEvent.click(screen.getByRole('button', {name: 'Gerar novo Pix'}));
    expect(createReactionPurchase).toHaveBeenCalledWith('fire', 'pix');
  });

  test('closes only when no purchase call is in flight', async () => {
    const onCloseAction = vi.fn();
    render(<ReactionPurchaseDialog entry={entry} initialPurchase={purchase({status: 'processing', method: 'fichas'})}
      onCloseAction={onCloseAction}/>, {wrapper});
    expect(screen.getByText(/Confirmando o débito de fichas/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Fechar e acompanhar depois'}));
    expect(onCloseAction).toHaveBeenCalledOnce();
  });
});

describe('ReactionRefundDialog', () => {
  test('renders nothing without a purchase', () => {
    const {container} = render(<ReactionRefundDialog purchase={null} onCloseAction={vi.fn()}
      onConfirmAction={vi.fn()}/>);
    expect(container).toBeEmptyDOMElement();
  });

  test('refunds a Pix purchase back through the same payment', async () => {
    const onConfirmAction = vi.fn().mockResolvedValue(undefined);
    render(<ReactionRefundDialog purchase={purchase()} onCloseAction={vi.fn()} onConfirmAction={onConfirmAction}/>);
    expect(screen.getByText('Mesma compra Pix')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Estornar R\$\s?9,90/}));
    expect(onConfirmAction).toHaveBeenCalledWith('p1');
    expect(await screen.findByText('Estorno concluído')).toBeInTheDocument();
  });

  test('refunds a fichas purchase back to the sandbox balance', () => {
    render(<ReactionRefundDialog purchase={purchase({method: 'fichas'})} onCloseAction={vi.fn()}
      onConfirmAction={vi.fn()}/>);
    expect(screen.getByText('Saldo de fichas sandbox')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Estornar 5\.000 fichas/})).toBeInTheDocument();
  });

  test('explains that an already-used reaction can no longer be refunded', async () => {
    render(<ReactionRefundDialog purchase={purchase()} onCloseAction={vi.fn()}
      onConfirmAction={vi.fn().mockRejectedValue(new ApiError('used', 409))}/>);
    await userEvent.click(screen.getByRole('button', {name: /Estornar/}));
    expect(await screen.findByRole('alert')).toHaveTextContent('já foi usada e não pode mais ser estornada');
  });

  test('keeps the purchase untouched when the refund call fails', async () => {
    render(<ReactionRefundDialog purchase={purchase()} onCloseAction={vi.fn()}
      onConfirmAction={vi.fn().mockRejectedValue(new Error('offline'))}/>);
    await userEvent.click(screen.getByRole('button', {name: /Estornar/}));
    expect(await screen.findByRole('alert')).toHaveTextContent('Sua compra não foi alterada');
  });

  test('keeping the reaction closes without calling the server', async () => {
    const onCloseAction = vi.fn();
    const onConfirmAction = vi.fn();
    render(<ReactionRefundDialog purchase={purchase()} onCloseAction={onCloseAction}
      onConfirmAction={onConfirmAction}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Manter reação'}));
    expect(onCloseAction).toHaveBeenCalledOnce();
    expect(onConfirmAction).not.toHaveBeenCalled();
  });
});

describe('ReactionFavoritesDialog', () => {
  const owned = new Set<string>(['fire']);

  test('caps the selection at three and lets a slot be freed', async () => {
    render(<ReactionFavoritesDialog open favorites={['clap', 'laugh'] as TableReactionID[]} owned={owned}
      saving={false} onOpenChangeAction={vi.fn()} onSaveAction={vi.fn()}/>);
    expect(screen.getByRole('status')).toHaveTextContent('2 de 3 selecionadas');

    await userEvent.click(screen.getByRole('button', {name: /Uau/}));
    expect(screen.getByRole('status')).toHaveTextContent('3 de 3 selecionadas');
    expect(screen.getByRole('button', {name: /Raiva/})).toBeDisabled();

    await userEvent.click(screen.getByRole('button', {name: /Uau/}));
    expect(screen.getByRole('status')).toHaveTextContent('2 de 3 selecionadas');
    expect(screen.getByRole('button', {name: /Raiva/})).toBeEnabled();
  });

  test('marks unowned premium reactions as locked but still favoritable', async () => {
    const onSaveAction = vi.fn().mockResolvedValue(undefined);
    const onOpenChangeAction = vi.fn();
    render(<ReactionFavoritesDialog open favorites={[]} owned={owned} saving={false}
      onOpenChangeAction={onOpenChangeAction} onSaveAction={onSaveAction}/>);

    expect(within(screen.getByRole('button', {name: /Pegando fogo/})).queryByLabelText('Premium bloqueada')).toBeNull();
    const locked = screen.getByRole('button', {name: /Frio na mesa/});
    expect(within(locked).getByLabelText('Premium bloqueada')).toBeInTheDocument();

    await userEvent.click(locked);
    await userEvent.click(screen.getByRole('button', {name: 'Salvar atalhos'}));
    expect(onSaveAction).toHaveBeenCalledWith(['cold']);
    expect(onOpenChangeAction).toHaveBeenCalledWith(false);
  });

  test('locks both footer actions while saving', () => {
    render(<ReactionFavoritesDialog open favorites={[]} owned={owned} saving
      onOpenChangeAction={vi.fn()} onSaveAction={vi.fn()}/>);
    expect(screen.getByRole('button', {name: 'Salvando…'})).toBeDisabled();
    expect(screen.getByRole('button', {name: 'Cancelar'})).toBeDisabled();
  });
});
