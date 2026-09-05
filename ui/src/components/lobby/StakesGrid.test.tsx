import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {StakesGrid} from './StakesGrid';

const push = vi.fn();
const listStakes = vi.fn();
const listRoomBuckets = vi.fn();
const joinOrCreateRoom = vi.fn();
const createRoom = vi.fn();
const getRoom = vi.fn();
let stakesQuery: Record<string, unknown> = {};
let bucketsQuery: Record<string, unknown> = {};
let bucketsQueryFn: (() => unknown) | null = null;

vi.mock('next/navigation', () => ({useRouter: () => ({push})}));
vi.mock('@tanstack/react-query', () => ({
  useQuery: ({queryKey, queryFn}: { queryKey: readonly string[]; queryFn: () => unknown }) => {
    if (queryKey[0] === 'stakes') return stakesQuery;
    bucketsQueryFn = queryFn;
    return bucketsQuery;
  },
}));
vi.mock('@/lib/hooks/useLobbyRealtime', () => ({useLobbyRealtime: vi.fn()}));
vi.mock('@/lib/api/rooms', () => ({
  listStakes: (...args: unknown[]) => listStakes(...args),
  listRoomBuckets: (...args: unknown[]) => listRoomBuckets(...args),
  joinOrCreateRoom: (...args: unknown[]) => joinOrCreateRoom(...args),
  createRoom: (...args: unknown[]) => createRoom(...args),
  getRoom: (...args: unknown[]) => getRoom(...args),
}));

function bucket(maxSeats: number, openRooms: number) {
  return {
    small_blind: 25, big_blind: 50, max_seats: maxSeats, currency_mode: 'sandbox',
    rooms: openRooms, open_rooms: openRooms, seats_taken: 0, seats_available: openRooms * maxSeats,
  };
}

describe('lobby stakes integration', () => {
  beforeEach(() => {
    stakesQuery = {};
    bucketsQuery = {};
    bucketsQueryFn = null;
    push.mockReset();
    listRoomBuckets.mockReset();
    joinOrCreateRoom.mockReset();
    createRoom.mockReset();
    getRoom.mockReset();
  });

  // The request budget for a lobby load: one aggregate call, no room-list
  // pagination and no per-room reads (#205).
  test('loads availability from the server aggregate and nothing else', () => {
    stakesQuery = {data: [], isLoading: false};
    bucketsQuery = {data: [], isLoading: false};
    render(<StakesGrid/>);
    expect(bucketsQueryFn).not.toBeNull();
    bucketsQueryFn?.();
    expect(listRoomBuckets).toHaveBeenCalledExactlyOnceWith('sandbox');
    expect(getRoom).not.toHaveBeenCalled();
    expect(createRoom).not.toHaveBeenCalled();
  });

  test('renders loading, error and empty contracts', async () => {
    stakesQuery = {data: [], isLoading: true};
    bucketsQuery = {data: [], isLoading: false};
    const {rerender} = render(<StakesGrid/>);
    expect(screen.getByText(/Buscando mesas/)).toBeInTheDocument();

    const refetch = vi.fn();
    const refetchBuckets = vi.fn();
    stakesQuery = {data: [], isLoading: false, isError: true, refetch};
    bucketsQuery = {data: [], isLoading: false, refetch: refetchBuckets};
    rerender(<StakesGrid/>);
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(refetch).toHaveBeenCalledOnce();
    expect(refetchBuckets).toHaveBeenCalledOnce();

    stakesQuery = {data: [], isLoading: false};
    bucketsQuery = {data: [], isLoading: false};
    rerender(<StakesGrid/>);
    expect(screen.getByText('Nenhum stake disponível no momento.')).toBeInTheDocument();
  });

  test('offers no card while the availability aggregate is unavailable', () => {
    stakesQuery = {data: [{small_blind: 10, big_blind: 20}], isLoading: false, refetch: vi.fn()};
    bucketsQuery = {data: [], isLoading: false, isError: true, refetch: vi.fn()};
    render(<StakesGrid/>);

    expect(screen.getByRole('alert')).toHaveTextContent('Nenhuma nova mesa será criada');
    expect(screen.queryByRole('button', {name: /HEADS-UP/})).not.toBeInTheDocument();
  });

  test('shows the aggregate open-room count per format', () => {
    stakesQuery = {data: [{small_blind: 25, big_blind: 50}], isLoading: false};
    // Counts the server reports across the whole directory, not a page of it.
    bucketsQuery = {data: [bucket(6, 3), bucket(9, 1)], isLoading: false};
    render(<StakesGrid/>);

    expect(screen.getByText('3 mesas ativas · até 6 jogadores')).toBeInTheDocument();
    expect(screen.getByText('1 mesa ativa · até 9 jogadores')).toBeInTheDocument();
    expect(screen.getByText('Nenhuma mesa ativa · até 2 jogadores')).toBeInTheDocument();
    expect(screen.getAllByText('Entrar agora')).toHaveLength(2);
    expect(screen.getAllByText('Criar mesa')).toHaveLength(1);
  });

  // The click is navigation only: nothing is read or mutated in the lobby,
  // and the seat is resolved by join-or-create in the buy-in ceremony.
  test('carries the picked bucket to the buy-in ceremony without any room lookup', async () => {
    stakesQuery = {data: [{small_blind: 25, big_blind: 50}], isLoading: false};
    bucketsQuery = {data: [bucket(6, 2)], isLoading: false};
    render(<StakesGrid/>);

    await userEvent.click(screen.getByRole('button', {name: /6-MAX/}));

    expect(push).toHaveBeenCalledExactlyOnceWith('/table?sb=25&bb=50&seats=6');
    expect(getRoom).not.toHaveBeenCalled();
    expect(createRoom).not.toHaveBeenCalled();
    expect(joinOrCreateRoom).not.toHaveBeenCalled();
  });

  test('a second click while one pick is in flight is a no-op', async () => {
    stakesQuery = {data: [{small_blind: 25, big_blind: 50}], isLoading: false};
    bucketsQuery = {data: [bucket(6, 2)], isLoading: false};
    render(<StakesGrid/>);

    await userEvent.click(screen.getByRole('button', {name: /6-MAX/}));
    await userEvent.click(screen.getAllByRole('button', {name: /aguarde, outra mesa está sendo aberta/})[0]);

    expect(push).toHaveBeenCalledTimes(1);
  });

  test('keeps three format choices while blind inventory grows', async () => {
    stakesQuery = {
      data: [
        {small_blind: 5, big_blind: 10},
        {small_blind: 10, big_blind: 20},
        {small_blind: 25, big_blind: 50},
        {small_blind: 50, big_blind: 100},
        {small_blind: 100, big_blind: 200},
      ],
      isLoading: false
    };
    bucketsQuery = {data: [bucket(6, 1)], isLoading: false};
    render(<StakesGrid/>);

    // The pre-selected stake is the one the aggregate reports tables for.
    expect(screen.getByRole('radio', {name: '25 / 50'})).toBeChecked();
    expect(screen.getAllByRole('button')).toHaveLength(3);
    expect(screen.getAllByText('Entrada sandbox: 2.000–5.000 fichas (40–100 BB)')).toHaveLength(3);

    await userEvent.click(screen.getByRole('radio', {name: '100 / 200'}));
    expect(screen.getAllByRole('button')).toHaveLength(3);
    expect(screen.getAllByText('Entrada sandbox: 8.000–20.000 fichas (40–100 BB)')).toHaveLength(3);
    expect(screen.getAllByText('Criar mesa')).toHaveLength(3);
  });
});
