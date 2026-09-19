import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, expect, test, vi} from 'vitest';
import {BotReservationScreen} from './BotReservationScreen';

const mocks = vi.hoisted(() => ({
  query: vi.fn(), replace: vi.fn(), setQueryData: vi.fn(), cancel: vi.fn(), refetch: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({
  useQuery: mocks.query,
  useQueryClient: () => ({setQueryData: mocks.setQueryData}),
}));
vi.mock('next/navigation', () => ({useRouter: () => ({replace: mocks.replace})}));
vi.mock('@/lib/api/rooms', () => ({
  getBotReservation: vi.fn(), cancelBotReservation: mocks.cancel,
}));

beforeEach(() => {
  vi.clearAllMocks();
  mocks.query.mockReturnValue({data: {status: 'pending'}, isError: false, refetch: mocks.refetch});
});

test('keeps the reserved player outside the table and explains that no chips were debited', () => {
  render(<BotReservationScreen roomId="room-1" reservationId="reservation-1"/>);
  expect(screen.getByRole('heading', {name: 'Sua vaga está reservada'})).toBeInTheDocument();
  expect(screen.getByRole('status')).toHaveTextContent('suas fichas ainda não foram debitadas');
  expect(screen.queryByText(/pote|cartas/i)).not.toBeInTheDocument();
});

test('cancels without seating and returns to the lobby', async () => {
  mocks.cancel.mockResolvedValue(undefined);
  render(<BotReservationScreen roomId="room-1" reservationId="reservation-1"/>);
  await userEvent.click(screen.getByRole('button', {name: 'Cancelar entrada'}));
  await waitFor(() => expect(mocks.cancel).toHaveBeenCalledWith('room-1', 'reservation-1'));
  expect(mocks.replace).toHaveBeenCalledWith('/lobby');
});

test('moves to the table only after the server confirms the seat', async () => {
  mocks.query.mockReturnValue({data: {status: 'seated'}, isError: false});
  render(<BotReservationScreen roomId="room-1" reservationId="reservation-1"/>);
  await waitFor(() => expect(mocks.setQueryData).toHaveBeenCalledWith(
    ['seated', 'room-1'], {seated: true, stack: 0},
  ));
  expect(mocks.replace).toHaveBeenCalledWith('/table?id=room-1');
});

test('explains expiry without pretending any debit happened', () => {
  mocks.query.mockReturnValue({data: {status: 'expired'}, isError: false});
  render(<BotReservationScreen roomId="room-1" reservationId="reservation-1"/>);
  expect(screen.getByRole('heading', {name: 'A reserva expirou'})).toBeInTheDocument();
  expect(screen.getByRole('status')).toHaveTextContent('Nenhuma ficha foi debitada');
  expect(screen.queryByRole('button', {name: 'Cancelar entrada'})).not.toBeInTheDocument();
});

test('keeps the player outside the table when the buy-in fails', () => {
  mocks.query.mockReturnValue({data: {status: 'failed', reason: 'Saldo insuficiente.'}, isError: false});
  render(<BotReservationScreen roomId="room-1" reservationId="reservation-1"/>);
  expect(screen.getByRole('heading', {name: 'A entrada não foi concluída'})).toBeInTheDocument();
  expect(screen.getByRole('status')).toHaveTextContent('Saldo insuficiente.');
  expect(mocks.replace).not.toHaveBeenCalled();
});

test('offers an explicit retry when reservation lookup fails', async () => {
  mocks.query.mockReturnValue({data: undefined, isError: true, refetch: mocks.refetch});
  render(<BotReservationScreen roomId="room-1" reservationId="reservation-1"/>);
  await userEvent.click(screen.getByRole('button', {name: 'Atualizar agora'}));
  expect(mocks.refetch).toHaveBeenCalled();
});
