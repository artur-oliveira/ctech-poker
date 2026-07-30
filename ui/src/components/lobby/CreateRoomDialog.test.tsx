import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {CreateRoomDialog} from './CreateRoomDialog';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  createRoom: vi.fn(),
  invalidateQueries: vi.fn(),
  push: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({
  useQuery: mocks.query,
  useQueryClient: () => ({invalidateQueries: mocks.invalidateQueries}),
}));
vi.mock('next/navigation', () => ({useRouter: () => ({push: mocks.push})}));
vi.mock('@/lib/api/player', () => ({getMe: vi.fn()}));
vi.mock('@/lib/api/rooms', () => ({
  createRoom: mocks.createRoom,
  listStakes: vi.fn(),
}));

const sandboxStakes = [
  {small_blind: 10, big_blind: 20},
  {small_blind: 25, big_blind: 50},
];
const realStakes = [{small_blind: 100, big_blind: 200, fee_cents: 75}];

describe('CreateRoomDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.query.mockImplementation(({queryKey}: { queryKey: string[] }) => {
      if (queryKey[0] === 'player') return {data: {wallet_mode: 'sandbox'}};
      if (queryKey[1] === 'real') return {data: []};
      return {data: sandboxStakes};
    });
  });
  
  test('creates a private sandbox table with the selected stake and seat count', async () => {
    mocks.createRoom.mockResolvedValue({room_id: 'room / private'});
    render(<CreateRoomDialog/>);
    await userEvent.click(screen.getByRole('button', {name: /Mesa privada/}));
    
    await userEvent.click(screen.getByRole('radio', {name: '25 / 50'}));
    await userEvent.click(screen.getByRole('radio', {name: '9 lugares'}));
    await userEvent.click(screen.getByRole('button', {name: 'Criar mesa privada'}));
    
    await waitFor(() => expect(mocks.createRoom).toHaveBeenCalledWith({
      visibility: 'private',
      currency_mode: 'sandbox',
      small_blind: 25,
      big_blind: 50,
      max_seats: 9,
      buy_in_min: 2_000,
      buy_in_max: 5_000,
      run_it_twice_enabled: false,
    }));
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['rooms']});
    expect(mocks.push).toHaveBeenCalledWith('/table?id=room%20%2F%20private');
  });
  
  test('can enable run it twice while creating the room', async () => {
    mocks.createRoom.mockResolvedValue({room_id: 'rit-room'});
    render(<CreateRoomDialog/>);
    await userEvent.click(screen.getByRole('button', {name: /Mesa privada/}));
    await userEvent.click(screen.getByText('Permitir rodar duas vezes'));
    await userEvent.click(screen.getByRole('button', {name: 'Criar mesa privada'}));
    await waitFor(() => expect(mocks.createRoom).toHaveBeenCalledWith(expect.objectContaining({
      run_it_twice_enabled: true,
    })));
  });
  
  test('offers real-money mode only to an eligible wallet and resets the stake selection', async () => {
    mocks.query.mockImplementation(({queryKey}: { queryKey: string[] }) => {
      if (queryKey[0] === 'player') return {data: {wallet_mode: 'real'}};
      if (queryKey[1] === 'real') return {data: realStakes};
      return {data: sandboxStakes};
    });
    mocks.createRoom.mockResolvedValue({id: 'real-room'});
    render(<CreateRoomDialog/>);
    await userEvent.click(screen.getByRole('button', {name: /Mesa privada/}));
    await userEvent.click(screen.getByRole('radio', {name: '25 / 50'}));
    await userEvent.click(screen.getByRole('radio', {name: 'Dinheiro real'}));
    
    expect(screen.getByRole('alert')).toHaveTextContent('Jogue com responsabilidade');
    expect(screen.getByRole('radio', {name: /R\$\s*1,00\s*\/\s*R\$\s*2,00/}))
      .toHaveAttribute('aria-checked', 'true');
    expect(screen.getByText(/taxa R\$\s*0,75/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Criar mesa privada'}));
    await waitFor(() => expect(mocks.createRoom).toHaveBeenCalledWith(expect.objectContaining({
      currency_mode: 'real',
      small_blind: 100,
      big_blind: 200,
      buy_in_min: 8_000,
      buy_in_max: 20_000,
    })));
  });
  
  test('supports roving keyboard selection for stakes and seats', async () => {
    render(<CreateRoomDialog/>);
    await userEvent.click(screen.getByRole('button', {name: /Mesa privada/}));
    const firstStake = screen.getByRole('radio', {name: '10 / 20'});
    firstStake.focus();
    fireEvent.keyDown(firstStake, {key: 'ArrowLeft'});
    expect(screen.getByRole('radio', {name: '25 / 50'})).toHaveAttribute('aria-checked', 'true');
    
    const sixSeats = screen.getByRole('radio', {name: '6 lugares'});
    fireEvent.keyDown(sixSeats, {key: 'ArrowDown'});
    expect(screen.getByRole('radio', {name: '9 lugares'})).toHaveAttribute('aria-checked', 'true');
  });
  
  test('disables creation with no stakes and exposes API errors without navigating', async () => {
    mocks.query.mockImplementation(({queryKey}: { queryKey: string[] }) =>
      queryKey[0] === 'player' ? {data: {wallet_mode: 'sandbox'}} : {data: []});
    const empty = render(<CreateRoomDialog/>);
    await userEvent.click(screen.getByRole('button', {name: /Mesa privada/}));
    expect(screen.getByText('Nenhum stake disponível no momento.')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Criar mesa privada'})).toBeDisabled();
    empty.unmount();
    
    mocks.query.mockImplementation(({queryKey}: { queryKey: string[] }) =>
      queryKey[0] === 'player' ? {data: {wallet_mode: 'sandbox'}} :
        queryKey[1] === 'real' ? {data: []} : {data: sandboxStakes});
    mocks.createRoom.mockRejectedValue(new Error('offline'));
    render(<CreateRoomDialog/>);
    await userEvent.click(screen.getByRole('button', {name: /Mesa privada/}));
    await userEvent.click(screen.getByRole('button', {name: 'Criar mesa privada'}));
    expect(await screen.findByText('Não foi possível criar a mesa. Tente novamente.')).toBeInTheDocument();
    expect(mocks.push).not.toHaveBeenCalled();
  });
  
  test('keeps the dialog open when a malformed create response has no room id', async () => {
    mocks.createRoom.mockResolvedValue({});
    render(<CreateRoomDialog/>);
    await userEvent.click(screen.getByRole('button', {name: /Mesa privada/}));
    await userEvent.click(screen.getByRole('button', {name: 'Criar mesa privada'}));
    
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível criar a mesa');
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(mocks.push).not.toHaveBeenCalled();
  });
});
