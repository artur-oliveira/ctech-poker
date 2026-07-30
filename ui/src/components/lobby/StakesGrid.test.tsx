import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {StakesGrid} from './StakesGrid';

const push = vi.fn();
const invalidateQueries = vi.fn();
const createRoom = vi.fn();
const listStakes = vi.fn();
const listRooms = vi.fn();
const notify = vi.fn();
let stakesQuery: Record<string, unknown> = {};
let roomsQuery: Record<string, unknown> = {};

vi.mock('next/navigation', () => ({useRouter: () => ({push})}));
vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({invalidateQueries}),
  useQuery: ({queryKey}: { queryKey: string[] }) => queryKey[0] === 'stakes' ? stakesQuery : roomsQuery,
}));
vi.mock('@/lib/hooks/useLobbyRealtime', () => ({useLobbyRealtime: vi.fn()}));
vi.mock('@/lib/api/rooms', () => ({
  createRoom: (...args: unknown[]) => createRoom(...args),
  listStakes: (...args: unknown[]) => listStakes(...args),
  listRooms: (...args: unknown[]) => listRooms(...args),
}));
vi.mock('@/lib/notify', () => ({pushNotification: (...args: unknown[]) => notify(...args)}));

describe('lobby stakes integration', () => {
  beforeEach(() => {
    stakesQuery = {};
    roomsQuery = {};
    push.mockReset();
    createRoom.mockReset();
    invalidateQueries.mockReset();
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
  
  test('joins an existing compatible room', async () => {
    stakesQuery = {data: [{small_blind: 25, big_blind: 50}], isLoading: false};
    roomsQuery = {
      data: [{
        id: 'open-room',
        visibility: 'public',
        small_blind: 25,
        big_blind: 50,
        max_seats: 6,
        seats_taken: 3
      }], isLoading: false
    };
    render(<StakesGrid/>);
    await userEvent.click(screen.getByRole('button', {name: /6-MAX/}));
    expect(push).toHaveBeenCalledWith('/table?id=open-room');
    expect(createRoom).not.toHaveBeenCalled();
  });
  
  test('creates a room when none is open and reports failures', async () => {
    createRoom.mockResolvedValueOnce({room_id: 'new room'});
    stakesQuery = {data: [{small_blind: 10, big_blind: 20}], isLoading: false};
    roomsQuery = {data: [], isLoading: false};
    const {unmount} = render(<StakesGrid/>);
    await userEvent.click(screen.getByRole('button', {name: /HEADS-UP/}));
    expect(createRoom).toHaveBeenCalledWith(expect.objectContaining({
      small_blind: 10, big_blind: 20, max_seats: 2, buy_in_min: 400, buy_in_max: 2000,
    }));
    expect(invalidateQueries).toHaveBeenCalledWith({queryKey: ['rooms']});
    expect(push).toHaveBeenCalledWith('/table?id=new%20room');
    unmount();
    
    createRoom.mockRejectedValueOnce(new Error('offline'));
    stakesQuery = {data: [{small_blind: 10, big_blind: 20}], isLoading: false};
    roomsQuery = {data: [], isLoading: false};
    render(<StakesGrid/>);
    await userEvent.click(screen.getByRole('button', {name: /FULL-RING/}));
    expect(notify).toHaveBeenCalledWith('Não foi possível entrar na mesa. Tente novamente.', 'error');
  });
});
