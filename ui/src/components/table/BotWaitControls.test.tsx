import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, beforeEach, expect, test, vi} from 'vitest';
import {BotWaitControls} from './BotWaitControls';

const mocks = vi.hoisted(() => ({query: vi.fn(), start: vi.fn(), refetch: vi.fn()}));
vi.mock('@tanstack/react-query', () => ({useQuery: mocks.query}));
vi.mock('@/lib/api/rooms', () => ({
  getBotWaitStatus: vi.fn(), startBotsNow: mocks.start,
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(Date, 'now').mockReturnValue(1_000_000);
  mocks.refetch.mockResolvedValue(undefined);
  mocks.query.mockReturnValue({
    data: {enabled: true, activate_at: 1_015_000, has_bot: false, reserved: false},
    refetch: mocks.refetch,
  });
});
afterEach(() => vi.restoreAllMocks());

test('shows the server deadline and lets the owner start without another buy-in', async () => {
  mocks.start.mockResolvedValue(undefined);
  render(<BotWaitControls roomId="room-1" connected expected/>);
  expect(screen.getByRole('status')).toHaveTextContent('bots disponíveis em 15 s');
  await userEvent.click(screen.getByRole('button', {name: 'Começar com bots agora'}));
  await waitFor(() => expect(mocks.start).toHaveBeenCalledWith('room-1'));
  expect(mocks.refetch).toHaveBeenCalled();
});

test('offers retry after a stalled fill and explains a failed attempt', async () => {
  vi.spyOn(Date, 'now').mockReturnValue(1_030_000);
  mocks.start.mockRejectedValue(new Error('unavailable'));
  render(<BotWaitControls roomId="room-1" connected expected/>);
  expect(screen.getByRole('status')).toHaveTextContent('Os bots ainda não entraram');
  await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
  expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível preparar os bots');
});

test('hides controls when the server has replaced bots with a human or disabled them', () => {
  mocks.query.mockReturnValue({data: {enabled: false}, refetch: mocks.refetch});
  const view = render(<BotWaitControls roomId="room-1" connected expected/>);
  expect(screen.queryByRole('button')).not.toBeInTheDocument();
  view.unmount();
  mocks.query.mockReturnValue({data: {enabled: true, has_bot: true}, refetch: mocks.refetch});
  render(<BotWaitControls roomId="room-1" connected expected/>);
  expect(screen.queryByRole('button')).not.toBeInTheDocument();
});

test('offers a manual status refresh after a lookup error', async () => {
  mocks.query.mockReturnValue({isError: true, refetch: mocks.refetch});
  render(<BotWaitControls roomId="room-1" connected expected/>);
  expect(screen.getByRole('alert')).toHaveTextContent('Não foi possível confirmar');
  await userEvent.click(screen.getByRole('button', {name: 'Atualizar espera'}));
  expect(mocks.refetch).toHaveBeenCalledOnce();
});
