import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
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
  isNotFound: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({
  useQuery: mocks.query,
  useQueryClient: () => ({invalidateQueries: mocks.invalidateQueries}),
}));
vi.mock('@/lib/api/rooms', () => ({
  getRoom: vi.fn(),
  joinRoom: mocks.joinRoom,
  leaveRoom: mocks.leaveRoom,
}));
vi.mock('@/lib/api/client', () => ({isNotFound: mocks.isNotFound}));
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
  beforeEach(() => vi.clearAllMocks());
  
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
    
    await waitFor(() => expect(mocks.joinRoom).toHaveBeenLastCalledWith('room-1', 4_000));
    expect(rebought).toHaveBeenCalledOnce();
  });
});
