import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import Store from './page';

const mocks = vi.hoisted(() => ({
  queryState: {} as Record<string, { data: unknown; isLoading: boolean; isError: boolean; refetch: ReturnType<typeof vi.fn> }>,
  invalidateQueries: vi.fn(),
  createPurchase: vi.fn(),
  refundPurchase: vi.fn(),
  getPurchase: vi.fn(),
  getCooldown: vi.fn(),
  spin: vi.fn(),
  notify: vi.fn(),
}));

function queryState(data: unknown, overrides: Record<string, unknown> = {}) {
  return {data, isLoading: false, isError: false, refetch: vi.fn(), ...overrides};
}

vi.mock('@tanstack/react-query', () => ({
  useQuery: ({queryKey}: { queryKey: string[] }) =>
    mocks.queryState[queryKey.join('.')] ?? queryState(undefined),
  useQueryClient: () => ({invalidateQueries: mocks.invalidateQueries}),
}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: { children: React.ReactNode }) => children}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
vi.mock('@/lib/hooks/useLobbyRealtime', () => ({useLobbyRealtime: vi.fn()}));
vi.mock('@/lib/notify', () => ({pushNotification: mocks.notify}));
vi.mock('@/lib/api/wallet', () => ({
  listSkus: vi.fn(),
  listPurchases: vi.fn(),
  createPurchase: mocks.createPurchase,
  refundPurchase: mocks.refundPurchase,
  getPurchase: mocks.getPurchase,
}));
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
      'wallet.sandbox-purchases': queryState(purchases),
      'dailyReward.cooldown': queryState({remaining_time_seconds: 0}),
    };
  });

  test('starts on the Recompensas tab with the daily reward widget ready to claim', () => {
    render(<Store/>);
    expect(screen.getByRole('heading', {name: 'Loja'})).toBeInTheDocument();
    expect(screen.getByText('profile-menu')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Resgatar recompensa'})).toBeEnabled();
    expect(screen.queryByText(/fichas/, {selector: '.store-sku-credits'})).not.toBeInTheDocument();
  });

  test('shows a countdown and a disabled claim button when the reward is on cooldown', () => {
    vi.useFakeTimers();
    mocks.queryState['dailyReward.cooldown'] = queryState({remaining_time_seconds: 3661});
    render(<Store/>);
    expect(screen.getByText(/Próxima recompensa em 1:01:01/)).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Resgatar recompensa'})).toBeDisabled();
    vi.useRealTimers();
  });

  test('claims the daily reward and shows the amount won', async () => {
    mocks.spin.mockResolvedValue({amount: 500, remaining_time_seconds: 86400});
    render(<Store/>);
    fireEvent.click(screen.getByRole('button', {name: 'Resgatar recompensa'}));
    await waitFor(() => expect(screen.getByText('+500 fichas')).toBeInTheDocument());
    expect(mocks.notify).toHaveBeenCalledWith(expect.stringContaining('500'), 'info');
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet', 'balance']});
  });

  test('switches to Compras and renders the SKU grid and purchase history', () => {
    render(<Store/>);
    fireEvent.click(screen.getByRole('button', {name: 'Compras'}));

    expect(screen.getByText('1.000')).toBeInTheDocument();
    expect(screen.getByText('+10% bônus')).toBeInTheDocument();
    expect(screen.getByText(/1\.000 fichas · R\$\s*1,00/)).toBeInTheDocument();
    expect(screen.getByText('Confirmada')).toBeInTheDocument();
    expect(screen.getByText('Pendente')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Estornar/})).toBeInTheDocument();
  });

  test('buying a pack opens the purchase modal with the PIX payload', async () => {
    mocks.createPurchase.mockResolvedValue({
      purchase_id: 'sbxp-new', sku: 'pack_100', status: 'pending',
      pix_copia_e_cola: '00020126...', qr_code_base64: 'iVBORw0...',
      expires_at: new Date(Date.now() + 600_000).toISOString(),
    });
    render(<Store/>);
    fireEvent.click(screen.getByRole('button', {name: 'Compras'}));
    fireEvent.click(screen.getByText('1.000').closest('button')!);

    await waitFor(() => expect(mocks.createPurchase).toHaveBeenCalledWith('pack_100'));
    expect(await screen.findByText('Pague com Pix para concluir')).toBeInTheDocument();
    expect(screen.getByText('00020126...')).toBeInTheDocument();
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet', 'sandbox-purchases']});
  });

  test('refunding a confirmed purchase calls the API and invalidates wallet queries', async () => {
    mocks.refundPurchase.mockResolvedValue({purchase_id: 'sbxp-1', status: 'refunded'});
    render(<Store/>);
    fireEvent.click(screen.getByRole('button', {name: 'Compras'}));
    fireEvent.click(screen.getByRole('button', {name: /Estornar/}));

    await waitFor(() => expect(mocks.refundPurchase).toHaveBeenCalledWith('sbxp-1'));
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet', 'balance']});
    expect(mocks.notify).toHaveBeenCalledWith('Compra estornada.', 'info');
  });

  test('shows an error state with retry when the SKU catalog fails to load', () => {
    mocks.queryState['wallet.skus'] = queryState(undefined, {isError: true});
    render(<Store/>);
    fireEvent.click(screen.getByRole('button', {name: 'Compras'}));
    expect(screen.getByText(/Não foi possível carregar os pacotes/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.queryState['wallet.skus'].refetch).toHaveBeenCalledOnce();
  });
});
