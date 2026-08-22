import {act, fireEvent, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {BuyInPanel, formatBuyIn, midBuyIn} from './BuyInPanel';
import {LeaveDialog} from './LeaveDialog';
import {RebuyDialog} from './RebuyDialog';
import type {Room} from '@/lib/api/rooms';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  joinRoom: vi.fn(),
  leaveRoom: vi.fn(),
  refetch: vi.fn(),
  invalidateQueries: vi.fn(),
  setQueryData: vi.fn(),
  isNotFound: vi.fn(),
  createPurchase: vi.fn(),
  spin: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({
  useQuery: mocks.query,
  useQueryClient: () => ({invalidateQueries: mocks.invalidateQueries, setQueryData: mocks.setQueryData}),
}));
vi.mock('@/lib/api/rooms', () => ({
  getRoom: vi.fn(),
  joinRoom: mocks.joinRoom,
  leaveRoom: mocks.leaveRoom,
}));
vi.mock('@/lib/api/client', () => ({isNotFound: mocks.isNotFound}));
vi.mock('@/lib/api/wallet', () => ({
  listSkus: vi.fn(),
  createPurchase: mocks.createPurchase,
}));
vi.mock('@/lib/api/player', () => ({getMe: vi.fn()}));
vi.mock('@/lib/api/dailyReward', () => ({getCooldown: vi.fn(), spin: mocks.spin}));
vi.mock('axios', () => ({
  default: {isAxiosError: (error: { axios?: boolean }) => Boolean(error?.axios)},
  isAxiosError: (error: { axios?: boolean }) => Boolean(error?.axios),
}));

const sandboxRoom: Room = {
  room_id: 'room-1',
  visibility: 'public',
  currency_mode: 'sandbox',
  small_blind: 25,
  big_blind: 50,
  max_seats: 6,
  buy_in_min: 2_000,
  buy_in_max: 5_000,
  status: 'open',
  seats_taken: 2,
};

describe('buy-in helpers', () => {
  test('rounds the midpoint to a blind and clamps it to the limits', () => {
    expect(midBuyIn(100, 230, 50)).toBe(150);
    expect(midBuyIn(100, 120, 50)).toBe(100);
    expect(midBuyIn(5, 9, 0)).toBe(7);
  });
  
  test('formats sandbox chips and real-money cents', () => {
    expect(formatBuyIn(2_000, false)).toBe('2.000');
    expect(formatBuyIn(12_345, true)).toMatch(/123,45/);
  });
});

describe('BuyInPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.query.mockReturnValue({data: sandboxRoom, isLoading: false, isError: false, refetch: mocks.refetch});
  });
  
  test('renders loading, missing-room and retryable failure states', async () => {
    mocks.query.mockReturnValueOnce({isLoading: true});
    const loading = render(<BuyInPanel roomId="room-1" onSeatedAction={vi.fn()}/>);
    expect(screen.getByText('Preparando a mesa…')).toBeInTheDocument();
    loading.unmount();
    
    mocks.isNotFound.mockReturnValueOnce(true);
    mocks.query.mockReturnValueOnce({isLoading: false, isError: true, error: new Error('gone')});
    const missing = render(<BuyInPanel roomId="room-1" onSeatedAction={vi.fn()}/>);
    expect(screen.getByText('Essa sala não está mais disponível')).toBeInTheDocument();
    missing.unmount();
    
    mocks.isNotFound.mockReturnValue(false);
    mocks.query.mockReturnValueOnce({
      isLoading: false,
      isError: true,
      error: new Error('offline'),
      refetch: mocks.refetch
    });
    render(<BuyInPanel roomId="room-1" onSeatedAction={vi.fn()}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.refetch).toHaveBeenCalledOnce();
  });
  
  test('confirms the selected amount and preserves a private share code', async () => {
    const seated = vi.fn();
    mocks.joinRoom.mockResolvedValue(undefined);
    render(<BuyInPanel roomId="room-1" shareCode="invite-7" onSeatedAction={seated}/>);
    
    expect(screen.getByRole('slider', {name: 'Buy-in'})).toHaveValue('3500');
    fireEvent.change(screen.getByRole('slider', {name: 'Buy-in'}), {target: {value: '4500'}});
    await userEvent.click(screen.getByRole('button', {name: 'Entrar com 4.500'}));
    
    await waitFor(() => expect(mocks.joinRoom).toHaveBeenCalledWith('room-1', 4_500, 'invite-7'));
    expect(seated).toHaveBeenCalledOnce();
  });
  
  test('opts into auto-rebuy and passes it through on confirm', async () => {
    const seated = vi.fn();
    mocks.joinRoom.mockResolvedValue(undefined);
    render(<BuyInPanel roomId="room-1" onSeatedAction={seated}/>);

    await userEvent.click(screen.getByRole('switch', {name: /auto.?rebuy|recompra automática/i}));
    await userEvent.click(screen.getByRole('button', {name: /Entrar com/}));

    await waitFor(() => expect(mocks.joinRoom).toHaveBeenCalledWith('room-1', 3_500, undefined, true));
    expect(seated).toHaveBeenCalledOnce();
  });

  test('shows wallet activation guidance and real-money fee without seating on failure', async () => {
    const seated = vi.fn();
    mocks.query.mockReturnValue({
      data: {...sandboxRoom, currency_mode: 'real', buy_in_min: 10_000, buy_in_max: 20_000, entry_fee_cents: 250},
      isLoading: false, isError: false, refetch: mocks.refetch,
    });
    mocks.joinRoom.mockRejectedValue({
      axios: true,
      response: {data: {detail: 'has not activated gambling on ctech-wallet'}},
    });
    render(<BuyInPanel roomId="room-1" onSeatedAction={seated}/>);
    
    expect(screen.getByText(/Taxa fixa de mesa:/)).toHaveTextContent('R$ 2,50');
    await userEvent.click(screen.getByRole('button', {name: /Entrar com/}));
    expect(await screen.findByRole('alert')).toHaveTextContent('carteira ainda não tem apostas ativadas');
    expect(seated).not.toHaveBeenCalled();
  });
  
  test('explains a full-table race and refreshes room state after the refund', async () => {
    mocks.joinRoom.mockRejectedValue({
      axios: true,
      response: {status: 409, data: {type: '/problems/table-full'}},
    });
    render(<BuyInPanel roomId="room-1" onSeatedAction={vi.fn()}/>);
    
    await userEvent.click(screen.getByRole('button', {name: /Entrar com/}));
    expect(await screen.findByRole('alert')).toHaveTextContent('última vaga foi ocupada');
    await waitFor(() => expect(mocks.invalidateQueries).toHaveBeenCalledTimes(3));
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['rooms']});
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['room', 'room-1']});
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['seated', 'room-1']});
  });
});

describe('table bankroll dialogs', () => {
  let playerData: { sandbox_balance?: number; game_balance?: number } | undefined;
  let cooldownData: { remaining_time_seconds: number } | undefined;

  beforeEach(() => {
    vi.clearAllMocks();
    playerData = {sandbox_balance: 6_000};
    cooldownData = {remaining_time_seconds: 0};
    mocks.query.mockImplementation((options: { queryKey: unknown[] }) => {
      const key = (options.queryKey as string[]).join(':');
      if (key === 'player:me') return {data: playerData};
      if (key === 'dailyReward:cooldown') return {data: cooldownData};
      return {data: undefined, isLoading: false, isError: false, refetch: vi.fn()};
    });
  });

  test('cash-outs the returned stack and closes the leave dialog', async () => {
    const left = vi.fn();
    mocks.leaveRoom.mockResolvedValue({amount: 1_750});
    render(<LeaveDialog roomId="room-1" stack={2_000} onLeftAction={left}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Sair da mesa'}));
    expect(screen.getByText(/2.000 fichas/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Sair e sacar fichas'}));
    await waitFor(() => expect(left).toHaveBeenCalledWith(1_750));
    expect(screen.queryByText('Sair da mesa?')).not.toBeInTheDocument();
  });
  
  test('distinguishes a dealt-in conflict from an already completed leave', async () => {
    const left = vi.fn();
    mocks.leaveRoom.mockRejectedValueOnce({axios: true, response: {status: 409, data: {detail: 'still active'}}});
    render(<LeaveDialog roomId="room-1" stack={900} onLeftAction={left}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Sair da mesa'}));
    await userEvent.click(screen.getByRole('button', {name: 'Sair e sacar fichas'}));
    expect(await screen.findByRole('alert')).toHaveTextContent('Você está na mão atual');
    expect(left).not.toHaveBeenCalled();
    
    mocks.leaveRoom.mockRejectedValueOnce({axios: true, response: {status: 409, data: {detail: 'player not found'}}});
    await userEvent.click(screen.getByRole('button', {name: 'Sair e sacar fichas'}));
    await waitFor(() => expect(left).toHaveBeenCalledWith(900));
  });
  
  test('rebuys a changed amount and reports a retryable failure', async () => {
    const rebought = vi.fn();
    mocks.joinRoom.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(undefined);
    render(<RebuyDialog roomId="room-1" room={sandboxRoom} onRebuyAction={rebought}/>);
    expect(screen.getByText('Você ficou sem fichas')).toBeInTheDocument();
    
    fireEvent.change(screen.getByRole('slider', {name: 'Recompra'}), {target: {value: '4000'}});
    await userEvent.click(screen.getByRole('button', {name: 'Comprar 4.000'}));
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível comprar mais fichas');
    await userEvent.click(screen.getByRole('button', {name: 'Comprar 4.000'}));
    
    await waitFor(() => expect(mocks.joinRoom).toHaveBeenLastCalledWith('room-1', 4_000, undefined, false));
    expect(rebought).toHaveBeenCalledOnce();
  });

  test('opts into auto-rebuy from the rebuy dialog and passes it through on confirm', async () => {
    const rebought = vi.fn();
    mocks.joinRoom.mockResolvedValue(undefined);
    render(<RebuyDialog roomId="room-1" room={sandboxRoom} onRebuyAction={rebought}/>);

    await userEvent.click(screen.getByRole('switch', {name: /auto.?rebuy|recompra automática/i}));
    await userEvent.click(screen.getByRole('button', {name: /Comprar/}));

    await waitFor(() => expect(mocks.joinRoom).toHaveBeenCalledWith('room-1', 3_500, undefined, true));
    expect(rebought).toHaveBeenCalledOnce();
  });

  test('does not offer the auto-rebuy toggle for real-money tables', () => {
    playerData = {game_balance: 20_000};
    render(<RebuyDialog roomId="room-1" room={{...sandboxRoom, currency_mode: 'real', buy_in_min: 10_000}}
                        onRebuyAction={vi.fn()}/>);
    expect(screen.queryByRole('switch')).not.toBeInTheDocument();
  });

  describe('bust recovery without a purchase', () => {
    test('never renders a SKU, QR code or store CTA', () => {
      playerData = {sandbox_balance: 0};
      render(<RebuyDialog roomId="room-1" room={sandboxRoom} onRebuyAction={vi.fn()}/>);
      expect(screen.queryByRole('slider')).not.toBeInTheDocument();
      expect(screen.queryByLabelText(/pix copia e cola/i)).not.toBeInTheDocument();
      expect(screen.queryByRole('link', {name: /Loja/})).not.toBeInTheDocument();
      expect(screen.getByRole('button', {name: 'Voltar ao lobby'})).toHaveAttribute('href', '/lobby');
    });

    test('treats a balance below the minimum buy-in as busted, not only zero', () => {
      playerData = {sandbox_balance: 1_999};
      render(<RebuyDialog roomId="room-1" room={sandboxRoom} onRebuyAction={vi.fn()}/>);
      expect(screen.queryByRole('slider')).not.toBeInTheDocument();
      expect(screen.getByText(/não cobre o buy-in mínimo/)).toBeInTheDocument();
    });

    test('claims the free daily reward and re-reads the balance', async () => {
      playerData = {sandbox_balance: 0};
      mocks.spin.mockResolvedValue({amount: 250, remaining_time_seconds: 3_600});
      render(<RebuyDialog roomId="room-1" room={sandboxRoom} onRebuyAction={vi.fn()}/>);

      await userEvent.click(screen.getByRole('button', {name: /Resgatar fichas grátis/}));
      await waitFor(() => expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['player', 'me']}));
      expect(await screen.findByRole('status')).toHaveTextContent('250 fichas grátis');
    });

    test('keeps explaining instead of pressuring when the reward is still not enough', async () => {
      playerData = {sandbox_balance: 0};
      mocks.spin.mockResolvedValue({amount: 250, remaining_time_seconds: 3_600});
      render(<RebuyDialog roomId="room-1" room={sandboxRoom} onRebuyAction={vi.fn()}/>);
      await userEvent.click(screen.getByRole('button', {name: /Resgatar fichas grátis/}));
      await screen.findByRole('status');
      expect(screen.queryByRole('slider')).not.toBeInTheDocument();
      expect(screen.getByRole('button', {name: 'Voltar ao lobby'})).toBeInTheDocument();
    });

    test('surfaces a failed reward claim', async () => {
      playerData = {sandbox_balance: 0};
      mocks.spin.mockRejectedValue(new Error('cooldown'));
      render(<RebuyDialog roomId="room-1" room={sandboxRoom} onRebuyAction={vi.fn()}/>);
      await userEvent.click(screen.getByRole('button', {name: /Resgatar fichas grátis/}));
      expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível resgatar a recompensa');
    });

    test('offers no claim while the reward is cooling down', () => {
      playerData = {sandbox_balance: 0};
      cooldownData = {remaining_time_seconds: 3_600};
      render(<RebuyDialog roomId="room-1" room={sandboxRoom} onRebuyAction={vi.fn()}/>);
      expect(screen.queryByRole('button', {name: /Resgatar fichas grátis/})).not.toBeInTheDocument();
      expect(screen.getByText(/recompensa diária ainda está em contagem/)).toBeInTheDocument();
    });

    test('sends an underfunded real-money player to the lobby, never to a purchase', () => {
      playerData = {game_balance: 100};
      render(<RebuyDialog roomId="room-1" room={{...sandboxRoom, currency_mode: 'real', entry_fee_cents: 500}}
                          onRebuyAction={vi.fn()}/>);
      expect(screen.getByText(/carteira fica em uma rota separada/)).toBeInTheDocument();
      expect(screen.queryByRole('button', {name: /Resgatar fichas grátis/})).not.toBeInTheDocument();
      expect(screen.getByText(/Taxa fixa de mesa/)).toBeInTheDocument();
    });
  });

  describe('auto-rebuy grace window', () => {
    beforeEach(() => vi.useFakeTimers());
    afterEach(() => vi.useRealTimers());

    test('shows nothing during the grace window when auto_rebuy is on', () => {
      render(<RebuyDialog roomId="room-1" room={sandboxRoom} autoRebuy onRebuyAction={vi.fn()}/>);
      expect(screen.queryByText('Você ficou sem fichas')).not.toBeInTheDocument();
    });

    test('falls back to the manual rebuy dialog after the grace window if still busted with balance', async () => {
      playerData = {sandbox_balance: 3_000};
      render(<RebuyDialog roomId="room-1" room={sandboxRoom} autoRebuy onRebuyAction={vi.fn()}/>);

      await act(async () => {
        vi.advanceTimersByTime(1600);
      });

      expect(screen.getByText('Você ficou sem fichas')).toBeInTheDocument();
      expect(screen.getByRole('slider', {name: 'Recompra'})).toBeInTheDocument();
    });

    test('renders the manual dialog immediately (no grace window) when auto_rebuy is off', () => {
      render(<RebuyDialog roomId="room-1" room={sandboxRoom} onRebuyAction={vi.fn()}/>);
      expect(screen.getByText('Você ficou sem fichas')).toBeInTheDocument();
    });
  });
});
