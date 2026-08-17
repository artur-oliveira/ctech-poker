import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {Page} from '@/lib/api/client';
import type {SocialInboxEvent, SocialPlayer} from '@/lib/api/social';
import {PeopleDrawer} from './PeopleDrawer';
import {RealtimeBridge} from '@/lib/providers/RealtimeBridge';

const api = vi.hoisted(() => ({
  listFriends: vi.fn(),
  listFriendRequests: vi.fn(),
  listRecentPlayers: vi.fn(),
  listSocialInbox: vi.fn(),
  acceptTableInvite: vi.fn(),
  declineTableInvite: vi.fn(),
  getSocialSummary: vi.fn(),
  push: vi.fn(),
  lobbyRealtime: vi.fn(),
}));

vi.mock('@/lib/api/social', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api/social')>()),
  listFriends: api.listFriends,
  listFriendRequests: api.listFriendRequests,
  listRecentPlayers: api.listRecentPlayers,
  listSocialInbox: api.listSocialInbox,
  acceptTableInvite: api.acceptTableInvite,
  declineTableInvite: api.declineTableInvite,
  getSocialSummary: api.getSocialSummary,
}));
vi.mock('next/navigation', () => ({useRouter: () => ({push: api.push})}));
vi.mock('@/lib/hooks/useLobbyRealtime', () => ({useLobbyRealtime: api.lobbyRealtime}));

function page<T>(items: T[]): Page<T> {
  return {data: items, has_next: false, next_cursor: null, has_previous: false, previous_cursor: null};
}

function player(overrides: Partial<SocialPlayer>): SocialPlayer {
  return {player_id: 'p1', name: 'Bia', relationship: 'none', muted: false, blocked: false, ...overrides};
}

function invite(overrides: Partial<SocialInboxEvent> = {}): SocialInboxEvent {
  return {
    event_id: 'e1', type: 'table_invite', actor_id: 'bia', status: 'pending', unread: true,
    created_at: Date.now(), expires_at: Date.now() + 600_000, room_id: 'room-1', ...overrides
  };
}

function renderDrawer() {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return render(<QueryClientProvider client={client}><PeopleDrawer/></QueryClientProvider>);
}

describe('lobby people drawer', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getSocialSummary.mockResolvedValue({unread_count: 2});
    api.listFriends.mockResolvedValue(page([
      player({player_id: 'bia', relationship: 'friend', presence: 'online'}),
      player({player_id: 'zeh', name: 'Zé', relationship: 'friend', presence: 'offline'})
    ]));
    api.listFriendRequests.mockResolvedValue(page([player({player_id: 'caio', name: 'Caio',
      relationship: 'incoming'})]));
    api.listRecentPlayers.mockResolvedValue(page([player({player_id: 'vic', name: 'Vic',
      last_played_at: Date.now()})]));
    api.listSocialInbox.mockResolvedValue(page([invite()]));
    api.acceptTableInvite.mockResolvedValue({event: invite({status: 'accepted'}), room: {room_id: 'room-1'}});
    api.declineTableInvite.mockResolvedValue(undefined);
  });

  test('loads nothing until it is opened, then shows only online friends', async () => {
    renderDrawer();
    expect(api.listFriends).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole('button', {name: /Pessoas/}));
    expect(await screen.findByText('Bia')).toBeInTheDocument();
    expect(screen.queryByText('Zé')).not.toBeInTheDocument();
    expect(screen.getByText('Caio')).toBeInTheDocument();
    expect(screen.getByText('Vic')).toBeInTheDocument();
    expect(screen.getByRole('link', {name: /Ver todas as pessoas/})).toHaveAttribute('href', '/people');
  });

  test('enters a table from an invite and closes the drawer', async () => {
    renderDrawer();
    await userEvent.click(screen.getByRole('button', {name: /Pessoas/}));
    await userEvent.click(await screen.findByRole('button', {name: 'Entrar'}));
    await waitFor(() => expect(api.push).toHaveBeenCalledWith('/table?id=room-1'));
  });

  test('declines an invite in place', async () => {
    renderDrawer();
    await userEvent.click(screen.getByRole('button', {name: /Pessoas/}));
    await userEvent.click(await screen.findByRole('button', {name: 'Recusar'}));
    await waitFor(() => expect(api.declineTableInvite).toHaveBeenCalledWith('e1'));
    expect(api.push).not.toHaveBeenCalled();
  });

  test('says so when there is no invite and no online friend', async () => {
    api.listSocialInbox.mockResolvedValue(page([invite({status: 'declined'})]));
    api.listFriends.mockResolvedValue(page([player({player_id: 'zeh', relationship: 'friend',
      presence: 'offline'})]));
    renderDrawer();
    await userEvent.click(screen.getByRole('button', {name: /Pessoas/}));
    expect(await screen.findByText('Nenhum convite ativo.')).toBeInTheDocument();
    expect(screen.getByText('Nenhum amigo online agora.')).toBeInTheDocument();
  });
});

describe('realtime bridge', () => {
  test('mounts the single lobby socket and renders nothing', () => {
    const {container} = render(<RealtimeBridge/>);
    expect(api.lobbyRealtime).toHaveBeenCalledOnce();
    expect(container).toBeEmptyDOMElement();
  });
});
