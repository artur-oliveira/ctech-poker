import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {Page} from '@/lib/api/client';
import type {SocialPlayer} from '@/lib/api/social';
import {InviteDialog} from './InviteDialog';

const api = vi.hoisted(() => ({listFriends: vi.fn(), sendTableInvite: vi.fn()}));

vi.mock('@/lib/api/social', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/social')>(),
  listFriends: api.listFriends,
  sendTableInvite: api.sendTableInvite,
}));

function friend(overrides: Partial<SocialPlayer>): SocialPlayer {
  return {player_id: 'bia', name: 'Bia', relationship: 'friend', muted: false, blocked: false, ...overrides};
}

function page(items: SocialPlayer[]): Page<SocialPlayer> {
  return {data: items, has_next: false, next_cursor: null, has_previous: false, previous_cursor: null};
}

function renderInvite(roomId?: string) {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return render(<QueryClientProvider client={client}>
    <InviteDialog url="https://poker.example/table?id=room-1" roomId={roomId}/>
  </QueryClientProvider>);
}

async function open() {
  await userEvent.click(screen.getByRole('button', {name: 'Convidar para a mesa'}));
  await screen.findByRole('heading', {name: 'Convidar para a mesa'});
}

describe('invite dialog friends section', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    Object.assign(navigator, {clipboard: {writeText: vi.fn().mockResolvedValue(undefined)}, share: undefined});
    api.listFriends.mockResolvedValue(page([friend({}), friend({player_id: 'leo', name: 'Leo'})]));
    api.sendTableInvite.mockResolvedValue({event_id: 'e1'});
  });

  test('only offers friends when the dialog knows the room', async () => {
    renderInvite();
    await open();
    expect(screen.queryByRole('heading', {name: 'Amigos'})).not.toBeInTheDocument();
    expect(api.listFriends).not.toHaveBeenCalled();
  });

  test('invites a friend and keeps the dialog open with a per-recipient status', async () => {
    renderInvite('room-1');
    await open();
    expect(await screen.findByText('Bia')).toBeInTheDocument();

    await userEvent.click(screen.getAllByRole('button', {name: 'Convidar'})[0]);
    await waitFor(() => expect(api.sendTableInvite).toHaveBeenCalledWith('bia', 'room-1'));
    expect(await screen.findByRole('button', {name: 'Convidado'})).toBeDisabled();
    expect(screen.getByRole('heading', {name: 'Convidar para a mesa'})).toBeInTheDocument();
  });

  test('filters the friend list locally by name', async () => {
    renderInvite('room-1');
    await open();
    await screen.findByText('Bia');
    await userEvent.type(screen.getByLabelText('Buscar na sua lista'), 'leo');
    expect(screen.queryByText('Bia')).not.toBeInTheDocument();
    expect(screen.getByText('Leo')).toBeInTheDocument();

    await userEvent.clear(screen.getByLabelText('Buscar na sua lista'));
    await userEvent.type(screen.getByLabelText('Buscar na sua lista'), 'ninguém');
    expect(screen.getByText('Nenhum amigo com esse nome.')).toBeInTheDocument();
  });

  test('reports a rejected invitation next to the copy link that still works', async () => {
    const {ApiError} = await import('@/lib/api/client');
    api.sendTableInvite.mockRejectedValueOnce(new ApiError('pending', 409,
      {type: '/problems/invite-already-pending'}));
    renderInvite('room-1');
    await open();
    await screen.findByText('Bia');
    await userEvent.click(screen.getAllByRole('button', {name: 'Convidar'})[0]);
    expect(await screen.findByRole('alert'))
      .toHaveTextContent('Já existe um convite pendente para esse amigo nesta mesa.');
    expect(screen.getByLabelText('Link de convite')).toHaveValue('https://poker.example/table?id=room-1');
  });

  test('says so when the viewer has no friend to invite', async () => {
    api.listFriends.mockResolvedValue(page([]));
    renderInvite('room-1');
    await open();
    expect(await screen.findByText('Você ainda não tem amigos para convidar.')).toBeInTheDocument();
  });
});
