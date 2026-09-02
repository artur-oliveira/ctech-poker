import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {StakesGrid} from './StakesGrid';

const push = vi.fn();
const replace = vi.fn();
const invalidateQueries = vi.fn();
const createRoom = vi.fn();
const getRoom = vi.fn();
const listStakes = vi.fn();
const listAllRooms = vi.fn();
let stakesQuery: Record<string, unknown> = {};
let roomsQuery: Record<string, unknown> = {};
let roomsQueryFn: (() => unknown) | null = null;

vi.mock('next/navigation', () => ({useRouter: () => ({push, replace})}));
vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({invalidateQueries}),
  useQuery: ({queryKey, queryFn}: { queryKey: string[]; queryFn: () => unknown }) => {
    if (queryKey[0] === 'stakes') return stakesQuery;
    roomsQueryFn = queryFn;
    return roomsQuery;
  },
}));
vi.mock('@/lib/hooks/useLobbyRealtime', () => ({useLobbyRealtime: vi.fn()}));
vi.mock('@/lib/api/rooms', () => ({
  createRoom: (...args: unknown[]) => createRoom(...args),
  getRoom: (...args: unknown[]) => getRoom(...args),
  listStakes: (...args: unknown[]) => listStakes(...args),
  listAllRooms: (...args: unknown[]) => listAllRooms(...args),
}));

// A single open room, used across the join-race tests below.
const openRoom = {
  id: 'open-room', visibility: 'public', small_blind: 25, big_blind: 50, max_seats: 6, seats_taken: 5
};

function withOpenRooms(rooms: unknown[] = [openRoom]) {
  return {data: rooms, isLoading: false, refetch: vi.fn().mockResolvedValue({data: rooms})};
}

describe('lobby stakes integration', () => {
  beforeEach(() => {
    stakesQuery = {};
    roomsQuery = {};
    roomsQueryFn = null;
    push.mockReset();
    replace.mockReset();
    createRoom.mockReset();
    getRoom.mockReset();
    invalidateQueries.mockReset();
    listAllRooms.mockReset();
    window.history.replaceState(null, '', '/lobby');
  });

  test('fetches the room list through the paginated, sandbox-scoped fetch', () => {
    stakesQuery = {data: [], isLoading: false};
    roomsQuery = {data: [], isLoading: false};
    render(<StakesGrid/>);
    expect(roomsQueryFn).not.toBeNull();
    roomsQueryFn?.();
    expect(listAllRooms).toHaveBeenCalledWith('sandbox');
  });
  afterEach(() => {
    window.history.replaceState(null, '', '/lobby');
  });

  test('renders loading, error and empty contracts', async () => {
    stakesQuery = {data: [], isLoading: true};
    roomsQuery = {data: [], isLoading: false};
    const {rerender} = render(<StakesGrid/>);
    expect(screen.getByText(/Buscando mesas/)).toBeInTheDocument();

    const refetch = vi.fn();
    const refetchRooms = vi.fn();
    stakesQuery = {data: [], isLoading: false, isError: true, refetch};
    roomsQuery = {data: [], isLoading: false, refetch: refetchRooms};
    rerender(<StakesGrid/>);
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(refetch).toHaveBeenCalledOnce();
    expect(refetchRooms).toHaveBeenCalledOnce();

    stakesQuery = {data: [], isLoading: false};
    roomsQuery = {data: [], isLoading: false};
    rerender(<StakesGrid/>);
    expect(screen.getByText('Nenhum stake disponível no momento.')).toBeInTheDocument();
  });

  test('does not create a duplicate room when the room inventory is unavailable', async () => {
    const refetchStakes = vi.fn();
    const refetchRooms = vi.fn();
    stakesQuery = {
      data: [{small_blind: 10, big_blind: 20}], isLoading: false, refetch: refetchStakes
    };
    roomsQuery = {data: [], isLoading: false, isError: true, refetch: refetchRooms};
    render(<StakesGrid/>);

    expect(screen.getByRole('alert')).toHaveTextContent('Nenhuma nova mesa será criada');
    expect(screen.queryByRole('button', {name: /HEADS-UP/})).not.toBeInTheDocument();
    expect(createRoom).not.toHaveBeenCalled();
  });

  test('joins an existing compatible room after re-verifying it is still open', async () => {
    stakesQuery = {data: [{small_blind: 25, big_blind: 50}], isLoading: false};
    roomsQuery = withOpenRooms([{
      id: 'open-room',
      visibility: 'public',
      small_blind: 25,
      big_blind: 50,
      max_seats: 6,
      seats_taken: 3
    }]);
    getRoom.mockResolvedValueOnce({room_id: 'open-room', seats_taken: 3, max_seats: 6});
    render(<StakesGrid/>);
    expect(screen.getByText('Agora escolha o tamanho da mesa')).toBeInTheDocument();
    expect(screen.getByText('O tamanho define quantos jogadores podem ocupar a mesa.')).toBeInTheDocument();
    expect(screen.getByText('Entrar agora')).toBeInTheDocument();
    expect(screen.getAllByText('Entrada sandbox: 1.000–5.000 fichas (20–100 BB)')).toHaveLength(3);
    await userEvent.click(screen.getByRole('button', {name: /6-MAX/}));
    await waitFor(() => expect(push).toHaveBeenCalledWith('/table?id=open-room'));
    expect(getRoom).toHaveBeenCalledWith('open-room');
    expect(createRoom).not.toHaveBeenCalled();
  });

  test('joins an open room that only exists past page one of the room list, without creating a new one', async () => {
    stakesQuery = {data: [{small_blind: 25, big_blind: 50}], isLoading: false};
    // Simulates the aggregate the paginated fetch hands back once it has
    // walked every page: a full room from page one and the joinable one that
    // used to be invisible on page two (see #90).
    roomsQuery = {
      data: [
        {id: 'page-1-full-room', visibility: 'public', small_blind: 25, big_blind: 50, max_seats: 6, seats_taken: 6},
        {id: 'page-2-open-room', visibility: 'public', small_blind: 25, big_blind: 50, max_seats: 6, seats_taken: 3},
      ], isLoading: false
    };
    render(<StakesGrid/>);
    expect(screen.getByText('1 mesa ativa · até 6 jogadores')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /6-MAX/}));
    expect(push).toHaveBeenCalledWith('/table?id=page-2-open-room');
    expect(createRoom).not.toHaveBeenCalled();
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
    roomsQuery = {data: [{
      id: 'active-stake-room',
      visibility: 'public',
      small_blind: 25,
      big_blind: 50,
      max_seats: 6,
      seats_taken: 2
    }], isLoading: false};
    render(<StakesGrid/>);

    const blindSelector = screen.getByRole('radio', {name: '25 / 50'});
    expect(blindSelector).toBeChecked();
    expect(screen.getAllByRole('button')).toHaveLength(3);
    expect(screen.getAllByText('Entrada sandbox: 1.000–5.000 fichas (20–100 BB)')).toHaveLength(3);

    await userEvent.click(screen.getByRole('radio', {name: '100 / 200'}));
    expect(screen.getAllByRole('button')).toHaveLength(3);
    expect(screen.getAllByText('Entrada sandbox: 4.000–20.000 fichas (20–100 BB)')).toHaveLength(3);
    expect(screen.getAllByText('Criar mesa')).toHaveLength(3);
  });

  test('creates a room when none is open and reports failures', async () => {
    createRoom.mockResolvedValueOnce({room_id: 'new room'});
    stakesQuery = {data: [{small_blind: 10, big_blind: 20}], isLoading: false};
    roomsQuery = withOpenRooms([]);
    const {unmount} = render(<StakesGrid/>);
    expect(screen.getAllByText('Criar mesa')).toHaveLength(3);
    expect(screen.getAllByText('Entrada sandbox: 400–2.000 fichas (20–100 BB)')).toHaveLength(3);
    await userEvent.click(screen.getByRole('button', {name: /HEADS-UP/}));
    expect(createRoom).toHaveBeenCalledWith(expect.objectContaining({
      small_blind: 10, big_blind: 20, max_seats: 2, buy_in_min: 400, buy_in_max: 2000,
    }));
    expect(invalidateQueries).toHaveBeenCalledWith({queryKey: ['rooms']});
    expect(push).toHaveBeenCalledWith('/table?id=new%20room');
    unmount();

    createRoom.mockRejectedValueOnce(new Error('offline'));
    stakesQuery = {data: [{small_blind: 10, big_blind: 20}], isLoading: false};
    roomsQuery = withOpenRooms([]);
    render(<StakesGrid/>);
    await userEvent.click(screen.getByRole('button', {name: /FULL-RING/}));
    expect(screen.getByRole('alert')).toHaveTextContent('Não foi possível criar a mesa. Tente novamente.');
  });

  test('falls through to a fresh room when the cached "open" room filled during the stale window', async () => {
    stakesQuery = {data: [{small_blind: 25, big_blind: 50}], isLoading: false};
    const staleOpenRoom = {
      id: 'stale-open', visibility: 'public', small_blind: 25, big_blind: 50, max_seats: 6, seats_taken: 5
    };
    // The cached list (still 30s-stale) shows a free seat, but a direct read
    // of that specific room reports it just filled.
    roomsQuery = withOpenRooms([staleOpenRoom]);
    getRoom.mockResolvedValueOnce({room_id: 'stale-open', seats_taken: 6, max_seats: 6});
    createRoom.mockResolvedValueOnce({room_id: 'fresh-room'});
    render(<StakesGrid/>);

    await userEvent.click(screen.getByRole('button', {name: /6-MAX/}));
    await waitFor(() => expect(push).toHaveBeenCalledWith('/table?id=fresh-room'));
    expect(createRoom).toHaveBeenCalledOnce();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  test('tries the next candidate when a verification read fails instead of dead-ending', async () => {
    stakesQuery = {data: [{small_blind: 25, big_blind: 50}], isLoading: false};
    const gone = {id: 'gone-room', visibility: 'public', small_blind: 25, big_blind: 50, max_seats: 6, seats_taken: 5};
    const stillOpen = {
      id: 'still-open', visibility: 'public', small_blind: 25, big_blind: 50, max_seats: 6, seats_taken: 5
    };
    roomsQuery = withOpenRooms([gone, stillOpen]);
    getRoom.mockRejectedValueOnce(new Error('not found'))
      .mockResolvedValueOnce({room_id: 'still-open', seats_taken: 5, max_seats: 6});
    render(<StakesGrid/>);

    await userEvent.click(screen.getByRole('button', {name: /6-MAX/}));
    await waitFor(() => expect(push).toHaveBeenCalledWith('/table?id=still-open'));
    expect(createRoom).not.toHaveBeenCalled();
    expect(getRoom).toHaveBeenCalledTimes(2);
  });

  test('two near-simultaneous joins into the same bucket both land without a raw error', async () => {
    stakesQuery = {data: [{small_blind: 25, big_blind: 50}], isLoading: false};
    roomsQuery = withOpenRooms([openRoom]);
    // First caller verifies the room is still open; by the time the second
    // caller's own verification read lands, the seat is gone.
    getRoom
      .mockResolvedValueOnce({room_id: 'open-room', seats_taken: 5, max_seats: 6})
      .mockResolvedValueOnce({room_id: 'open-room', seats_taken: 6, max_seats: 6});
    createRoom.mockResolvedValueOnce({room_id: 'fallback-room'});

    const containerA = document.createElement('div');
    const containerB = document.createElement('div');
    document.body.append(containerA, containerB);
    render(<StakesGrid/>, {container: containerA});
    render(<StakesGrid/>, {container: containerB});

    const buttonA = within(containerA).getByRole('button', {name: /6-MAX/});
    const buttonB = within(containerB).getByRole('button', {name: /6-MAX/});
    fireEvent.click(buttonA);
    fireEvent.click(buttonB);

    await waitFor(() => expect(push).toHaveBeenCalledTimes(2));
    expect(push).toHaveBeenCalledWith('/table?id=open-room');
    expect(push).toHaveBeenCalledWith('/table?id=fallback-room');
    expect(within(containerA).queryByRole('alert')).not.toBeInTheDocument();
    expect(within(containerB).queryByRole('alert')).not.toBeInTheDocument();
  });

  test('auto-retries the bucket the table page bounced back for, then strips the query', async () => {
    window.history.replaceState(null, '', '/lobby?retrySmallBlind=25&retryBigBlind=50&retrySeats=6');
    stakesQuery = {data: [{small_blind: 25, big_blind: 50}], isLoading: false};
    roomsQuery = withOpenRooms([openRoom]);
    getRoom.mockResolvedValueOnce({room_id: 'open-room', seats_taken: 5, max_seats: 6});

    render(<StakesGrid/>);

    await waitFor(() => expect(replace).toHaveBeenCalledWith('/lobby'));
    await waitFor(() => expect(push).toHaveBeenCalledWith('/table?id=open-room'));
    expect(screen.getByRole('radio', {name: '25 / 50'})).toBeChecked();
  });

  test('ignores an incomplete retry query instead of auto-joining', async () => {
    window.history.replaceState(null, '', '/lobby?retrySmallBlind=25');
    stakesQuery = {data: [{small_blind: 25, big_blind: 50}], isLoading: false};
    roomsQuery = withOpenRooms([]);

    render(<StakesGrid/>);
    await waitFor(() => expect(screen.getByText('Agora escolha o tamanho da mesa')).toBeInTheDocument());
    expect(replace).not.toHaveBeenCalled();
    expect(push).not.toHaveBeenCalled();
  });
});
