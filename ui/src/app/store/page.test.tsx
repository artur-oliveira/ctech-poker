import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import Store from './page';

const mocks = vi.hoisted(() => ({
  queryState: {} as Record<string, { data: unknown; isLoading: boolean; isError: boolean; refetch: ReturnType<typeof vi.fn> }>,
  invalidateQueries: vi.fn(),
  fetchNextPage: vi.fn(),
  setQueryData: vi.fn(),
  createPurchase: vi.fn(),
  refundPurchase: vi.fn(),
  refundCosmeticPurchase: vi.fn(),
  refundReactionPurchase: vi.fn(),
  getPurchase: vi.fn(),
  getCooldown: vi.fn(),
  spin: vi.fn(),
  notify: vi.fn(),
}));

function queryState(data: unknown, overrides: Record<string, unknown> = {}) {
  return {data, isLoading: false, isError: false, refetch: vi.fn(), ...overrides};
}

/** Purchase history is a cursor-paginated infinite query; its state is a list of pages. */
function pageState(rows: unknown[], overrides: Record<string, unknown> = {}) {
  return queryState({pages: [{data: rows, has_next: false, next_cursor: null}]}, overrides);
}

vi.mock('@tanstack/react-query', () => ({
  useQuery: ({queryKey}: { queryKey: string[] }) =>
    mocks.queryState[queryKey.join('.')] ?? queryState(undefined),
  useInfiniteQuery: ({queryKey}: { queryKey: string[] }) => ({
    ...(mocks.queryState[queryKey.join('.')] ?? pageState([])),
    fetchNextPage: mocks.fetchNextPage,
  }),
  useQueryClient: () => ({invalidateQueries: mocks.invalidateQueries, setQueryData: mocks.setQueryData}),
}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: { children: React.ReactNode }) => children}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
vi.mock('@/lib/hooks/useLobbyRealtime', () => ({useLobbyRealtime: vi.fn()}));
vi.mock('@/lib/notify', () => ({pushNotification: mocks.notify}));
vi.mock('@/lib/api/wallet', () => ({
  WALLET_QUERY_ROOT: ['wallet'],
  listSkus: vi.fn(),
  listPurchases: vi.fn(),
  createPurchase: mocks.createPurchase,
  refundPurchase: mocks.refundPurchase,
  getPurchase: mocks.getPurchase,
}));
vi.mock('@/lib/api/cosmeticPurchases', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/cosmeticPurchases')>(),
  listCosmeticCatalog: vi.fn(),
  listCosmeticPurchases: vi.fn(),
  getCosmeticPurchase: vi.fn(),
  refundCosmeticPurchase: mocks.refundCosmeticPurchase,
}));
vi.mock('@/lib/api/reactionPurchases', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/reactionPurchases')>(),
  listReactionCatalog: vi.fn(),
  listReactionPurchases: vi.fn(),
  getReactionPurchase: vi.fn(),
  refundReactionPurchase: mocks.refundReactionPurchase,
}));
vi.mock('next/image', () => ({default: ({alt}: {alt: string}) => <div role="img" aria-label={alt}/>}));
vi.mock('@/lib/api/dailyReward', () => ({
  getCooldown: mocks.getCooldown,
  spin: mocks.spin,
}));

const skus = [
  {id: 'pack_100', price_cents: 100, base_credits: 1000, bonus_percent: 0, total_credits: 1000},
  {id: 'pack_500', price_cents: 500, base_credits: 5000, bonus_percent: 10, total_credits: 5500},
];

const purchases = [
  {
    purchase_id: 'sbxp-1', sku: 'pack_100', status: 'confirmed', total_credits: 1000, price_cents: 100,
    created_at: '2026-07-30T10:00:00Z',
  },
  {
    purchase_id: 'sbxp-2', sku: 'pack_500', status: 'pending', total_credits: 5500, price_cents: 500,
    created_at: '2026-07-30T11:00:00Z',
  },
];

describe('store page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.queryState = {
      'wallet.skus': queryState(skus),
      'wallet.sandbox-purchases': pageState(purchases),
      'dailyReward.cooldown': queryState({remaining_time_seconds: 0}),
      'player.me': queryState({sandbox_balance: 12_345}),
    };
  });

  test('shows reward, packages, and recent purchases in one continuous page', () => {
    render(<Store/>);
    expect(screen.getByRole('heading', {name: 'Loja'})).toBeInTheDocument();
    expect(screen.getByRole('navigation', {name: 'Seções da loja'})).toBeInTheDocument();
    // Receipts for reactions and cosmetics live in their own top-level
    // department, not nested inside the section that shipped first.
    const activityDepartment = screen.getByRole('heading', {name: 'Compras e estornos'}).closest('section');
    const chipDepartment = screen.getByRole('heading', {name: 'Fichas sandbox'}).closest('section');
    expect(activityDepartment).toHaveAttribute('id', 'activity');
    expect(within(activityDepartment!).getByRole('heading', {name: 'Reações'})).toBeInTheDocument();
    expect(within(activityDepartment!).getByRole('heading', {name: 'Baralhos e feltros'})).toBeInTheDocument();
    expect(within(chipDepartment!).getByRole('heading', {name: 'Compras e estornos de fichas'})).toBeInTheDocument();
    expect(screen.getByText('profile-menu')).toBeInTheDocument();
    expect(screen.getByText('12.345 fichas')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Resgatar fichas grátis'})).toBeEnabled();
    expect(screen.getByText('1.000')).toBeInTheDocument();
    expect(screen.getByText('Confirmada')).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'Comprar via Pix'})).not.toBeInTheDocument();
  });

  test('collapses the reward to a countdown when it is on cooldown', () => {
    vi.useFakeTimers();
    mocks.queryState['dailyReward.cooldown'] = queryState({remaining_time_seconds: 3661});
    render(<Store/>);
    expect(screen.getByRole('heading', {name: 'Recompensa diária'})).toBeInTheDocument();
    expect(screen.getByText('Resgatada · próxima em 1:01:01')).toBeInTheDocument();
    vi.useRealTimers();
  });

  test('claims the daily reward and shows the amount won', async () => {
    mocks.spin.mockResolvedValue({amount: 500, remaining_time_seconds: 86400});
    render(<Store/>);
    fireEvent.click(screen.getByRole('button', {name: 'Resgatar fichas grátis'}));
    await waitFor(() => expect(screen.getByText(/^\+500 fichas recebidas · próxima em/)).toBeInTheDocument());
    expect(mocks.notify).toHaveBeenCalledWith(expect.stringContaining('500'), 'info');
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet', 'balance']});
    expect(mocks.setQueryData).toHaveBeenCalledWith(
      ['dailyReward', 'cooldown'],
      {remaining_time_seconds: 86400},
    );
  });

  test('renders the SKU grid and purchase history without navigation', () => {
    render(<Store/>);

    expect(screen.getByText('1.000')).toBeInTheDocument();
    expect(screen.getByText('5.000 base')).toBeInTheDocument();
    expect(screen.getByText('500 bônus')).toBeInTheDocument();
    expect(screen.getByText('(10%)')).toBeInTheDocument();
    expect(screen.getByText('sem bônus')).toBeInTheDocument();
    const confirmedPurchase = screen.getByText('1.000 fichas').closest('.store-history-item') as HTMLElement;
    expect(within(confirmedPurchase).getByText(/R\$\s*1,00/)).toBeInTheDocument();
    expect(screen.getByText('Confirmada')).toBeInTheDocument();
    expect(screen.getByText('Pendente')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Solicitar estorno'})).toBeInTheDocument();
  });

  test('shows every package ordered by price regardless of API order', () => {
    mocks.queryState['wallet.skus'] = queryState([
      {id: 'pack_10000', price_cents: 10000, base_credits: 1000000, bonus_percent: 100, total_credits: 2000000},
      {id: 'pack_100', price_cents: 100, base_credits: 10000, bonus_percent: 0, total_credits: 10000},
      {id: 'pack_500', price_cents: 500, base_credits: 50000, bonus_percent: 10, total_credits: 55000},
      {id: 'pack_1000', price_cents: 1000, base_credits: 100000, bonus_percent: 20, total_credits: 120000},
      {id: 'pack_2000', price_cents: 2000, base_credits: 200000, bonus_percent: 35, total_credits: 270000},
      {id: 'pack_5000', price_cents: 5000, base_credits: 500000, bonus_percent: 50, total_credits: 750000},
    ]);
    render(<Store/>);

    const packageButtons = screen.getAllByRole('button', {name: /^Escolher .* fichas:/});
    expect(packageButtons.map(button => button.getAttribute('aria-label'))).toEqual([
      expect.stringMatching(/^Escolher 10\.000 fichas:/),
      expect.stringMatching(/^Escolher 55\.000 fichas:/),
      expect.stringMatching(/^Escolher 120\.000 fichas:/),
      expect.stringMatching(/^Escolher 270\.000 fichas:/),
      expect.stringMatching(/^Escolher 750\.000 fichas:/),
      expect.stringMatching(/^Escolher 2\.000\.000 fichas:/),
    ]);
    expect(screen.queryByRole('button', {name: /Ver mais|Mostrar .* opções/})).not.toBeInTheDocument();
  });

  test('buying a pack opens the purchase modal with the PIX payload', async () => {
    mocks.createPurchase.mockResolvedValue({
      purchase_id: 'sbxp-new', sku: 'pack_100', status: 'pending',
      pix_copia_e_cola: '00020126...', qr_code_base64: 'iVBORw0...',
      expires_at: new Date(Date.now() + 600_000).toISOString(),
    });
    render(<Store/>);
    fireEvent.click(screen.getByRole('button', {name: /Escolher 1\.000 fichas/}));

    await waitFor(() => expect(mocks.createPurchase).toHaveBeenCalledWith('pack_100'));
    expect(await screen.findByText('Pague com Pix para concluir')).toBeInTheDocument();
    expect(screen.getByDisplayValue('00020126...')).toBeInTheDocument();
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet']});
  });

  test('regenerates an expired Pix for the same package and restores package focus on close', async () => {
    mocks.createPurchase
      .mockResolvedValueOnce({
        purchase_id: 'sbxp-expired', sku: 'pack_100', status: 'pending',
        pix_copia_e_cola: 'expired-code', expires_at: new Date(Date.now() - 1_000).toISOString(),
      })
      .mockResolvedValueOnce({
        purchase_id: 'sbxp-fresh', sku: 'pack_100', status: 'pending',
        pix_copia_e_cola: 'fresh-code', expires_at: new Date(Date.now() + 600_000).toISOString(),
      });
    render(<Store/>);
    const packageButton = screen.getByRole('button', {name: /Escolher 1\.000 fichas/});
    fireEvent.click(packageButton);

    expect(await screen.findByRole('heading', {name: 'Código Pix expirado'})).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Gerar novo Pix para este pacote'}));

    await waitFor(() => expect(mocks.createPurchase).toHaveBeenNthCalledWith(2, 'pack_100'));
    expect(await screen.findByDisplayValue('fresh-code')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Fechar'}));
    await waitFor(() => expect(packageButton).toHaveFocus());
  });

  test('reopens a pending Pix payment from purchase history', async () => {
    mocks.getPurchase.mockResolvedValue({
      ...purchases[1], pix_copia_e_cola: '00020126-resumed',
      expires_at: new Date(Date.now() + 600_000).toISOString(),
    });
    render(<Store/>);
    fireEvent.click(screen.getByRole('button', {name: /Continuar pagamento/}));

    await waitFor(() => expect(mocks.getPurchase).toHaveBeenCalledWith('sbxp-2'));
    expect(await screen.findByDisplayValue('00020126-resumed')).toBeInTheDocument();
  });

  test('confirms exact sandbox consequences before refunding and shows success', async () => {
    mocks.refundPurchase.mockResolvedValue({purchase_id: 'sbxp-1', status: 'refunded'});
    render(<Store/>);
    fireEvent.click(screen.getByRole('button', {name: 'Solicitar estorno'}));

    expect(screen.getByRole('heading', {name: 'Solicitar estorno desta compra?'})).toBeInTheDocument();
    expect(within(screen.getByRole('dialog')).getByText(/1.000 fichas$/)).toBeInTheDocument();
    expect(screen.getByText('11.345 fichas')).toBeInTheDocument();
    expect(screen.getByText(/fichas já foram usadas em uma mesa/)).toBeInTheDocument();
    expect(screen.getByText(/Não movimenta saldo de dinheiro real/)).toBeInTheDocument();
    expect(mocks.refundPurchase).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', {name: /Solicitar estorno de R/}));
    await waitFor(() => expect(mocks.refundPurchase).toHaveBeenCalledWith('sbxp-1'));
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet']});
    expect(await screen.findByRole('heading', {name: 'Compra estornada'})).toBeInTheDocument();
    expect(screen.getByText(/1.000 fichas sandbox removidas/)).toBeInTheDocument();
  });

  test('keeps the dialog actionable when the server rejects refund eligibility', async () => {
    const {ApiError} = await import('@/lib/api/client');
    mocks.refundPurchase.mockRejectedValue(new ApiError('sandbox purchase credits already used', 409));
    render(<Store/>);
    fireEvent.click(screen.getByRole('button', {name: 'Solicitar estorno'}));
    fireEvent.click(screen.getByRole('button', {name: /Solicitar estorno de R/}));

    expect(await screen.findByRole('alert')).toHaveTextContent('O estorno não é mais elegível');
    expect(screen.getByRole('button', {name: /Solicitar estorno de R/})).toBeEnabled();
  });

  test('shows an error state with retry when the SKU catalog fails to load', () => {
    mocks.queryState['wallet.skus'] = queryState(undefined, {isError: true});
    render(<Store/>);
    expect(screen.getByText(/Não foi possível carregar os pacotes/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.queryState['wallet.skus'].refetch).toHaveBeenCalledOnce();
  });

  test('limits recent purchases to three until the user expands the history', () => {
    mocks.queryState['wallet.sandbox-purchases'] = pageState([
      ...purchases,
      {...purchases[0], purchase_id: 'sbxp-3', total_credits: 3000},
      {...purchases[0], purchase_id: 'sbxp-4', total_credits: 4000},
    ]);
    render(<Store/>);

    expect(screen.queryByText(/4\.000 fichas/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Ver todas as 4 compras'}));
    expect(screen.getByText(/4\.000 fichas/)).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Mostrar menos'})).toHaveAttribute('aria-expanded', 'true');
    expect(screen.queryByRole('button', {name: 'Carregar mais compras'})).not.toBeInTheDocument();
  });

  test('flattens every fetched page and offers to load the next one', () => {
    mocks.queryState['wallet.sandbox-purchases'] = queryState({
      pages: [
        {data: purchases, has_next: true, next_cursor: 'c2'},
        {data: [
          {...purchases[0], purchase_id: 'sbxp-3', total_credits: 3000},
          {...purchases[0], purchase_id: 'sbxp-4', total_credits: 4000},
        ], has_next: true, next_cursor: 'c3'},
      ],
    }, {hasNextPage: true, isFetchingNextPage: false});
    render(<Store/>);

    fireEvent.click(screen.getByRole('button', {name: 'Ver todas as 4+ compras'}));
    expect(screen.getByText(/4\.000 fichas/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Carregar mais compras'}));
    expect(mocks.fetchNextPage).toHaveBeenCalledOnce();
  });

  test('disables the load-more button while the next page is in flight', () => {
    mocks.queryState['wallet.sandbox-purchases'] = queryState(
      {pages: [{data: purchases, has_next: true, next_cursor: 'c2'}]},
      {hasNextPage: true, isFetchingNextPage: true},
    );
    render(<Store/>);

    fireEvent.click(screen.getByRole('button', {name: 'Ver todas as 2+ compras'}));
    expect(screen.getByRole('button', {name: 'Carregando…'})).toBeDisabled();
  });
});

describe('store cosmetics and reactions', () => {
  const deckCatalog = [
    {kind: 'deck', id: 'four-color', premium: false, owned: true},
    {kind: 'deck', id: 'golden', premium: true, owned: true, price_cents: 2990, price_fichas: 500_000},
    {kind: 'deck', id: 'pink', premium: true, owned: false, price_cents: 2990, price_fichas: 500_000},
  ];
  const feltCatalog = [
    {kind: 'felt', id: 'classic', premium: false, owned: true},
    {kind: 'felt', id: 'midnight', premium: true, owned: false, price_cents: 990, price_fichas: 1_000_000},
  ];
  const reactionCatalog = [
    {id: 'clap', premium: false, owned: true},
    {id: 'fire', premium: true, owned: true, price_cents: 990, price_fichas: 5_000},
    {id: 'cold', premium: true, owned: false, price_cents: 490, price_fichas: 2_500},
  ];
  const deckReceipt = {
    purchase_id: 'cp-1', kind: 'deck', item_id: 'golden', method: 'fichas',
    status: 'confirmed', price_fichas: 500_000,
  };
  const reactionReceipt = {
    purchase_id: 'rp-1', reaction_id: 'fire', method: 'fichas', status: 'confirmed', price_fichas: 5_000,
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.queryState = {
      'wallet.skus': queryState(skus),
      'wallet.sandbox-purchases': pageState(purchases),
      'wallet.reaction-catalog': queryState(reactionCatalog),
      'wallet.reaction-purchases.history': pageState([reactionReceipt]),
      'wallet.cosmetic-catalog.deck': queryState(deckCatalog),
      'wallet.cosmetic-purchases.deck': pageState([deckReceipt]),
      'wallet.cosmetic-catalog.felt': queryState(feltCatalog),
      'wallet.cosmetic-purchases.felt': pageState([]),
      'dailyReward.cooldown': queryState({remaining_time_seconds: 0}),
      'player.me': queryState({sandbox_balance: 12_345}),
    };
  });

  // The regression this guards: ownership used to be inferred from purchase
  // history, which counted refunded rows and (for cosmetics) leaked the other
  // kind's purchases in, producing "8 de 6 liberados".
  test('counts released items from the catalog, never from purchase history', () => {
    render(<Store/>);
    const directory = screen.getByRole('navigation', {name: 'Seções da loja'});
    expect(within(directory).getByText('1 de 2 liberadas')).toBeInTheDocument();
    expect(within(directory).getByText('1 de 2 liberados')).toBeInTheDocument();
    expect(within(directory).getByText('0 de 1 liberados')).toBeInTheDocument();
  });

  test('shows a loading label per section while its catalog is still in flight', () => {
    mocks.queryState['wallet.cosmetic-catalog.deck'] = queryState(undefined, {isLoading: true});
    render(<Store/>);
    expect(screen.getAllByText('Carregando catálogo…')).toHaveLength(1);
  });

  test('refunding an owned deck calls the API and invalidates the whole wallet root', async () => {
    mocks.refundCosmeticPurchase.mockResolvedValue({...deckReceipt, status: 'refunding'});
    render(<Store/>);
    const decks = screen.getByRole('list', {name: 'Catálogo de baralhos'});
    fireEvent.click(within(decks).getByRole('button', {name: /Estornar/}));
    fireEvent.click(await screen.findByRole('button', {name: /Estornar 500\.000 fichas/}));

    await waitFor(() => expect(mocks.refundCosmeticPurchase).toHaveBeenCalledWith('deck', 'cp-1'));
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet']});
  });

  test('refunding an owned reaction calls the API and invalidates the whole wallet root', async () => {
    mocks.refundReactionPurchase.mockResolvedValue({...reactionReceipt, status: 'refunding'});
    render(<Store/>);
    const reactions = screen.getByRole('list', {name: 'Catálogo de reações premium'});
    fireEvent.click(within(reactions).getByRole('button', {name: /Estornar/}));
    fireEvent.click(await screen.findByRole('button', {name: /Estornar 5\.000 fichas/}));

    await waitFor(() => expect(mocks.refundReactionPurchase).toHaveBeenCalledWith('rp-1'));
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet']});
  });

  test('a locked felt opens its purchase dialog instead of buying silently', async () => {
    render(<Store/>);
    const felts = screen.getByRole('list', {name: 'Catálogo de feltros'});
    fireEvent.click(within(felts).getByRole('button', {name: 'Liberar'}));
    expect(await screen.findByRole('heading', {name: 'Liberar Meia-noite'})).toBeInTheDocument();
  });

  test('retrying a failed section refetches both its catalog and its history', () => {
    mocks.queryState['wallet.cosmetic-catalog.felt'] = queryState(undefined, {isError: true});
    render(<Store/>);
    fireEvent.click(within(screen.getByRole('heading', {name: 'Feltro'}).closest('section')!)
      .getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.queryState['wallet.cosmetic-catalog.felt'].refetch).toHaveBeenCalledOnce();
    expect(mocks.queryState['wallet.cosmetic-purchases.felt'].refetch).toHaveBeenCalledOnce();
  });
});
