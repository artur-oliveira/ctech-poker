import type {ReactNode} from 'react';
import {render, screen, waitFor} from '@testing-library/react';
import {expectNoAxeViolations} from '@/test/axe';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {Page} from '@/lib/api/client';
import type {SocialInboxEvent, SocialPlayer} from '@/lib/api/social';
import People from './page';

const api = vi.hoisted(() => ({
  listFriends: vi.fn(),
  listFriendRequests: vi.fn(),
  listRecentPlayers: vi.fn(),
  listBlockedPlayers: vi.fn(),
  listSocialInbox: vi.fn(),
  acceptFriendRequest: vi.fn(),
  markInboxRead: vi.fn(),
  getMe: vi.fn(),
  notify: vi.fn(),
}));

vi.mock('@/lib/api/social', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api/social')>()),
  listFriends: api.listFriends,
  listFriendRequests: api.listFriendRequests,
  listRecentPlayers: api.listRecentPlayers,
  listBlockedPlayers: api.listBlockedPlayers,
  listSocialInbox: api.listSocialInbox,
  acceptFriendRequest: api.acceptFriendRequest,
  markInboxRead: api.markInboxRead,
}));
vi.mock('@/lib/api/player', () => ({getMe: api.getMe}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: {children: ReactNode}) => children}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
const searchParams = {value: new URLSearchParams()};
vi.mock('next/navigation', () => ({
  useRouter: () => ({push: vi.fn()}),
  useSearchParams: () => searchParams.value,
}));
vi.mock('@/lib/notify', () => ({pushNotification: api.notify}));

function page<T>(items: T[], hasNext = false): Page<T> {
  return {data: items, has_next: hasNext, next_cursor: hasNext ? 'next' : null, has_previous: false,
    previous_cursor: null};
}

function player(overrides: Partial<SocialPlayer>): SocialPlayer {
  return {player_id: 'p1', name: 'Bia', relationship: 'none', muted: false, blocked: false, ...overrides};
}

function inboxEvent(overrides: Partial<SocialInboxEvent> = {}): SocialInboxEvent {
  return {event_id: 'e1', type: 'friend_request', actor_id: 'caio', status: 'pending', unread: false,
    created_at: Date.now(), ...overrides};
}

function renderPeople() {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return render(<QueryClientProvider client={client}><People/></QueryClientProvider>);
}

describe('people page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    searchParams.value = new URLSearchParams();
    api.getMe.mockResolvedValue({user_id: 'me', friend_code: 'PKR-AAAA-BBBB-CCCC'});
    api.listFriends.mockResolvedValue(page([player({player_id: 'bia', relationship: 'friend', presence: 'online'})]));
    api.listFriendRequests.mockResolvedValue(page([player({player_id: 'caio', name: 'Caio',
      relationship: 'incoming'})]));
    api.listRecentPlayers.mockResolvedValue(page([player({player_id: 'vic', name: 'Vic',
      last_played_at: Date.now()})]));
    api.listBlockedPlayers.mockResolvedValue(page([player({player_id: 'spam', name: 'Spam', blocked: true,
      muted: true})]));
    api.listSocialInbox.mockResolvedValue(page([inboxEvent()]));
    api.acceptFriendRequest.mockResolvedValue(player({player_id: 'caio', relationship: 'friend'}));
    api.markInboxRead.mockResolvedValue(undefined);
  });

  test('opens the activity tab when the url asks for it', async () => {
    searchParams.value = new URLSearchParams('tab=activity');
    renderPeople();
    expect(await screen.findByRole('button', {name: 'Atividades'})).toHaveAttribute('aria-pressed', 'true');
  });

  test('falls back to friends for an unknown tab', async () => {
    searchParams.value = new URLSearchParams('tab=garbage');
    renderPeople();
    expect(await screen.findByRole('button', {name: 'Amigos'})).toHaveAttribute('aria-pressed', 'true');
  });

  test('opens on friends and only loads a tab once it is selected', async () => {
    renderPeople();
    expect(await screen.findByText('Bia')).toBeInTheDocument();
    expect(api.listRecentPlayers).not.toHaveBeenCalled();
    expect(api.listSocialInbox).not.toHaveBeenCalled();
    expect(api.listFriendRequests).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole('button', {name: 'Recentes'}));
    expect(await screen.findByText('Vic')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', {name: 'Bloqueados'}));
    expect(await screen.findByText('Spam')).toBeInTheDocument();
  });

  test('switches request direction and answers one from the list', async () => {
    renderPeople();
    await userEvent.click(screen.getByRole('button', {name: 'Solicitações'}));
    expect(await screen.findByText('Caio')).toBeInTheDocument();
    expect(api.listFriendRequests).toHaveBeenCalledWith('incoming', undefined);

    await userEvent.click(screen.getByRole('button', {name: 'Enviadas'}));
    await waitFor(() => expect(api.listFriendRequests).toHaveBeenCalledWith('outgoing', undefined));

    await userEvent.click(screen.getByRole('button', {name: 'Recebidas'}));
    await userEvent.click(await screen.findByRole('button', {name: 'Aceitar Caio'}));
    await waitFor(() => expect(api.acceptFriendRequest).toHaveBeenCalledWith('caio'));
  });

  test('reports a failed action as a notification and keeps the list on screen', async () => {
    const {ApiError} = await import('@/lib/api/client');
    api.acceptFriendRequest.mockRejectedValueOnce(new ApiError('conflict', 409,
      {type: '/problems/relationship-conflict'}));
    renderPeople();
    await userEvent.click(screen.getByRole('button', {name: 'Solicitações'}));
    await userEvent.click(await screen.findByRole('button', {name: 'Aceitar Caio'}));
    await waitFor(() => expect(api.notify).toHaveBeenCalledWith('Não foi possível concluir essa ação agora.'));
    expect(screen.getByText('Caio')).toBeInTheDocument();
  });

  // #210: the inbox resolves actor_name server-side, so Atividades is one
  // request — not inbox + friends + requests to spell a name.
  test('reads the activity feed from the inbox alone', async () => {
    searchParams.value = new URLSearchParams('tab=activity');
    api.listSocialInbox.mockResolvedValue(page([inboxEvent({actor_name: 'Caio'})]));
    renderPeople();
    expect(await screen.findByText('Caio quer ser seu amigo.')).toBeInTheDocument();
    expect(api.listSocialInbox).toHaveBeenCalledTimes(1);
    expect(api.listFriends).not.toHaveBeenCalled();
    expect(api.listFriendRequests).not.toHaveBeenCalled();
  });

  test('pages a list forward with the returned cursor', async () => {
    api.listFriends
      .mockResolvedValueOnce(page([player({player_id: 'bia', relationship: 'friend'})], true))
      .mockResolvedValueOnce(page([player({player_id: 'leo', name: 'Leo', relationship: 'friend'})]));
    renderPeople();
    await userEvent.click(await screen.findByRole('button', {name: 'Carregar mais'}));
    expect(await screen.findByText('Leo')).toBeInTheDocument();
    expect(api.listFriends).toHaveBeenLastCalledWith('next');
  });

  test('surfaces a failing list with a retry', async () => {
    api.listFriends.mockRejectedValue(new Error('offline'));
    renderPeople();
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível carregar esta lista.');

    api.listFriends.mockResolvedValue(page([player({player_id: 'bia', relationship: 'friend'})]));
    await userEvent.click(screen.getByRole('button', {name: /Tentar novamente/}));
    expect(await screen.findByText('Bia')).toBeInTheDocument();
  });

  // Issue #60: an automated floor under the a11y intent in ui/CLAUDE.md — a new
  // serious or critical axe violation on this route fails CI.
  test('is axe-clean', async () => {
    const {container} = renderPeople();
    await screen.findByRole('heading', {level: 1});
    await expectNoAxeViolations(container);
  });

});
