import type {ReactNode} from 'react';
import {render, screen, waitFor, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {SocialInboxEvent, SocialPlayer} from '@/lib/api/social';
import type {SocialActionState} from '@/lib/hooks/useSocialActions';
import {PeopleList} from './PeopleList';
import {PeopleNavBadge} from './PeopleNavBadge';
import {PlayerActionsMenu} from './PlayerActionsMenu';
import {ReportPlayerDialog} from './ReportPlayerDialog';
import {FriendCodeLookup} from './FriendCodeLookup';
import {SocialInbox} from './SocialInbox';

const api = vi.hoisted(() => ({
  getSocialSummary: vi.fn(),
  lookupFriendCode: vi.fn(),
  markInboxRead: vi.fn(),
  reportPlayer: vi.fn(),
  getMe: vi.fn(),
  push: vi.fn(),
}));

vi.mock('@/lib/api/social', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api/social')>()),
  getSocialSummary: api.getSocialSummary,
  lookupFriendCode: api.lookupFriendCode,
  markInboxRead: api.markInboxRead,
  reportPlayer: api.reportPlayer,
}));
vi.mock('@/lib/api/player', () => ({getMe: api.getMe}));
vi.mock('next/navigation', () => ({useRouter: () => ({push: api.push})}));

function renderWithClient(ui: ReactNode) {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  const wrap = (node: ReactNode) => <QueryClientProvider client={client}>{node}</QueryClientProvider>;
  const result = render(wrap(ui));
  return {...result, rerender: (node: ReactNode) => result.rerender(wrap(node))};
}

function actionState(overrides: Partial<SocialActionState> = {}): SocialActionState {
  return {run: vi.fn().mockResolvedValue(true), pending: null, ...overrides};
}

function player(overrides: Partial<SocialPlayer>): SocialPlayer {
  return {player_id: 'p1', name: 'Bia', relationship: 'none', muted: false, blocked: false, ...overrides};
}

function event(overrides: Partial<SocialInboxEvent>): SocialInboxEvent {
  return {
    event_id: 'e1', type: 'table_invite', actor_id: 'p1', status: 'pending', unread: false,
    created_at: Date.now(), expires_at: Date.now() + 600_000, ...overrides
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  api.getSocialSummary.mockResolvedValue({unread_count: 0});
  api.getMe.mockResolvedValue({user_id: 'me', friend_code: 'PKR-AAAA-BBBB-CCCC'});
  api.markInboxRead.mockResolvedValue(undefined);
  api.reportPlayer.mockResolvedValue({report_id: 'rep-1', status: 'open'});
});

describe('PeopleList', () => {
  test('shows the loading, error and empty states of a tab', async () => {
    const actions = actionState();
    const {rerender} = renderWithClient(<PeopleList variant="friends" items={[]} isLoading actions={actions}
                                                    emptyTitle="Sem amigos"/>);
    expect(screen.getByRole('status', {name: ''})).toHaveAttribute('aria-busy', 'true');

    const retry = vi.fn();
    rerender(<PeopleList variant="friends" items={[]} isError onRetryAction={retry} actions={actions}
                         emptyTitle="Sem amigos"/>);
    await userEvent.click(screen.getByRole('button', {name: /Tentar novamente/}));
    expect(retry).toHaveBeenCalled();

    rerender(<PeopleList variant="friends" items={[]} actions={actions} emptyTitle="Sem amigos"
                         emptyHint="Compartilhe seu código"/>);
    expect(screen.getByText('Sem amigos')).toBeInTheDocument();
    expect(screen.getByText('Compartilhe seu código')).toBeInTheDocument();
  });

  test('answers an incoming request from the row itself', async () => {
    const actions = actionState();
    renderWithClient(<PeopleList variant="incoming" items={[player({relationship: 'incoming'})]} actions={actions}
                                 emptyTitle="vazio"/>);
    await userEvent.click(screen.getByRole('button', {name: 'Aceitar Bia'}));
    expect(actions.run).toHaveBeenCalledWith('accept', 'p1');
    await userEvent.click(screen.getByRole('button', {name: 'Recusar Bia'}));
    expect(actions.run).toHaveBeenCalledWith('decline', 'p1');
  });

  test('cancels an outgoing request, unblocks and adds a recent player', async () => {
    const actions = actionState();
    const {rerender} = renderWithClient(
      <PeopleList variant="outgoing" items={[player({relationship: 'outgoing'})]} actions={actions}
                  emptyTitle="vazio"/>);
    await userEvent.click(screen.getByRole('button', {name: 'Cancelar'}));
    expect(actions.run).toHaveBeenCalledWith('cancel', 'p1');

    rerender(<PeopleList variant="blocked" items={[player({blocked: true, muted: true})]} actions={actions}
                         emptyTitle="vazio"/>);
    await userEvent.click(screen.getByRole('button', {name: 'Desbloquear'}));
    expect(actions.run).toHaveBeenCalledWith('unblock', 'p1');

    rerender(<PeopleList variant="recent" items={[player({last_played_at: Date.now(), presence: 'online'})]}
                         actions={actions} emptyTitle="vazio"/>);
    expect(screen.getByText(/Jogaram hoje/)).toBeInTheDocument();
    expect(screen.getByText('Online')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Adicionar Bia'}));
    expect(actions.run).toHaveBeenCalledWith('request', 'p1');
  });

  test('offers to join a friend at a joinable public table', () => {
    renderWithClient(
      <PeopleList variant="friends" emptyTitle="vazio" actions={actionState()}
                  items={[player({relationship: 'friend', presence: 'in_table', room_id: 'room-9'})]}/>);
    expect(screen.getByRole('link', {name: 'Entrar na mesa'})).toHaveAttribute('href', '/table?id=room-9');
  });

  test('offers no join target without a room id', () => {
    renderWithClient(
      <PeopleList variant="friends" emptyTitle="vazio" actions={actionState()}
                  items={[player({relationship: 'friend', presence: 'in_table'})]}/>);
    expect(screen.queryByRole('link', {name: 'Entrar na mesa'})).not.toBeInTheDocument();
  });

  test('invites a friend once, warns when stale and pages forward', async () => {
    const invite = vi.fn();
    const loadMore = vi.fn();
    const actions = actionState();
    const {rerender} = renderWithClient(
      <PeopleList variant="friends" items={[player({relationship: 'friend', presence: 'in_table'})]}
                  actions={actions} emptyTitle="vazio" isStale hasNext onMoreAction={loadMore}
                  onInviteAction={invite} invitedIds={[]}/>);
    expect(screen.getByRole('status')).toHaveTextContent('última lista carregada');
    expect(screen.getByText('Em uma mesa')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Convidar'}));
    expect(invite).toHaveBeenCalledWith(expect.objectContaining({player_id: 'p1'}));
    await userEvent.click(screen.getByRole('button', {name: 'Carregar mais'}));
    expect(loadMore).toHaveBeenCalled();

    rerender(<PeopleList variant="friends" items={[player({relationship: 'friend'})]} actions={actions}
                         emptyTitle="vazio" hasNext loadingMore onMoreAction={loadMore} onInviteAction={invite}
                         invitedIds={['p1']}/>);
    expect(screen.getByRole('button', {name: 'Convidado'})).toBeDisabled();
    expect(screen.getByRole('button', {name: 'Carregando…'})).toBeDisabled();
  });
});

describe('PlayerActionsMenu', () => {
  async function openMenu() {
    await userEvent.click(screen.getByRole('button', {name: 'Ações para Bia'}));
    await screen.findByText('Ver perfil');
  }

  test('offers the friend action that matches the current relationship', async () => {
    const actions = actionState();
    const {rerender} = renderWithClient(
      <PlayerActionsMenu target={player({})} actions={actions} surface="profile"/>);
    await openMenu();
    expect(screen.getByRole('link', {name: /Ver perfil/})).toHaveAttribute('href', '/profile?id=p1');
    await userEvent.click(screen.getByRole('button', {name: 'Adicionar amigo'}));
    expect(actions.run).toHaveBeenCalledWith('request', 'p1');

    rerender(<PlayerActionsMenu target={player({relationship: 'friend'})} actions={actions} surface="profile"/>);
    await openMenu();
    await userEvent.click(screen.getByRole('button', {name: 'Remover amizade'}));
    expect(actions.run).toHaveBeenCalledWith('remove', 'p1');
  });

  test('accepts or declines an incoming request and edits the private note', async () => {
    const actions = actionState();
    const onEditNote = vi.fn();
    renderWithClient(<PlayerActionsMenu target={player({relationship: 'incoming'})} actions={actions}
                                        surface="table_behavior" onEditNoteAction={onEditNote}/>);
    await openMenu();
    await userEvent.click(screen.getByRole('button', {name: 'Recusar solicitação'}));
    expect(actions.run).toHaveBeenCalledWith('decline', 'p1');

    await openMenu();
    await userEvent.click(screen.getByRole('button', {name: 'Editar nota privada'}));
    expect(onEditNote).toHaveBeenCalled();
  });

  test('mutes, unmutes and hides the friend action for a blocked player', async () => {
    const actions = actionState();
    const {rerender} = renderWithClient(
      <PlayerActionsMenu target={player({})} actions={actions} surface="profile"/>);
    await openMenu();
    await userEvent.click(screen.getByRole('button', {name: 'Silenciar'}));
    expect(actions.run).toHaveBeenCalledWith('mute', 'p1');

    rerender(<PlayerActionsMenu target={player({muted: true, blocked: true})} actions={actions} surface="profile"/>);
    await openMenu();
    expect(screen.queryByRole('button', {name: 'Adicionar amigo'})).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Reativar chat e reações'}));
    expect(actions.run).toHaveBeenCalledWith('unmute', 'p1');

    await openMenu();
    await userEvent.click(screen.getByRole('button', {name: 'Desbloquear'}));
    expect(actions.run).toHaveBeenCalledWith('unblock', 'p1');
  });

  test('confirms a block, hides content first and rolls back when it fails', async () => {
    const actions = actionState({run: vi.fn().mockResolvedValue(false)});
    const onBlocked = vi.fn().mockResolvedValue(true);
    renderWithClient(<PlayerActionsMenu target={player({})} actions={actions} surface="table_chat" tableId="t1"
                                        handId="h1" onBlockedAction={onBlocked}/>);
    await openMenu();
    await userEvent.click(screen.getByRole('button', {name: 'Bloquear'}));
    expect(screen.getByText(/desfaz a amizade/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Cancelar'}));
    expect(screen.queryByText(/desfaz a amizade/)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', {name: 'Bloquear'}));
    await userEvent.click(screen.getByRole('button', {name: 'Bloquear'}));
    await waitFor(() => expect(onBlocked).toHaveBeenCalledWith(false));
    expect(actions.run).toHaveBeenCalledWith('block', 'p1');
  });

  test('blocks without a local rollback hook and keeps the menu closed after success', async () => {
    const actions = actionState();
    renderWithClient(<PlayerActionsMenu target={player({})} actions={actions} surface="profile"/>);
    await openMenu();
    await userEvent.click(screen.getByRole('button', {name: 'Bloquear'}));
    await userEvent.click(screen.getByRole('button', {name: 'Bloquear'}));
    await waitFor(() => expect(actions.run).toHaveBeenCalledWith('block', 'p1'));
  });

  test('opens the report dialog from the menu', async () => {
    renderWithClient(<PlayerActionsMenu target={player({})} actions={actionState()} surface="table_chat"/>);
    await openMenu();
    await userEvent.click(screen.getByRole('button', {name: 'Denunciar'}));
    expect(await screen.findByRole('heading', {name: 'Denunciar Bia'})).toBeInTheDocument();
  });
});

describe('ReportPlayerDialog', () => {
  test('sends a category with optional details and confirms', async () => {
    renderWithClient(<ReportPlayerDialog target={{player_id: 'p1', name: 'Bia'}} surface="table_chat" tableId="t1"
                                         handId="h1" actionId="a1" open onOpenChangeAction={vi.fn()}/>);
    await userEvent.click(await screen.findByRole('radio', {name: 'Trapaça ou conluio'}));
    await userEvent.type(screen.getByRole('textbox'), 'Falou combinado com outro jogador');
    await userEvent.click(screen.getByRole('button', {name: /Enviar denúncia/}));

    await waitFor(() => expect(api.reportPlayer).toHaveBeenCalledWith({
      target_player_id: 'p1', category: 'cheating', surface: 'table_chat',
      table_id: 't1', hand_id: 'h1', action_id: 'a1', details: 'Falou combinado com outro jogador'
    }));
    expect(await screen.findByText(/Denúncia registrada/)).toBeInTheDocument();
  });

  test('surfaces a throttled report and can be cancelled', async () => {
    const {ApiError} = await import('@/lib/api/client');
    api.reportPlayer.mockRejectedValueOnce(new ApiError('slow down', 429, {type: '/problems/report-rate-limited'}));
    const onOpenChange = vi.fn();
    renderWithClient(<ReportPlayerDialog target={{player_id: 'p1'}} surface="profile" open
                                         onOpenChangeAction={onOpenChange}/>);
    await userEvent.click(await screen.findByRole('button', {name: /Enviar denúncia/}));
    expect(await screen.findByRole('alert')).toHaveTextContent('muitas denúncias');
    expect(api.reportPlayer).toHaveBeenCalledWith(expect.objectContaining({details: undefined}));

    await userEvent.click(screen.getByRole('button', {name: 'Cancelar'}));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  test('does nothing without a target', async () => {
    renderWithClient(<ReportPlayerDialog target={null} surface="profile" open onOpenChangeAction={vi.fn()}/>);
    await userEvent.click(await screen.findByRole('button', {name: /Enviar denúncia/}));
    expect(api.reportPlayer).not.toHaveBeenCalled();
  });
});

describe('FriendCodeLookup', () => {
  test('copies the own code and resolves an exact code into a request', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, {clipboard: {writeText}});
    api.lookupFriendCode.mockResolvedValue(player({relationship: 'none'}));
    const actions = actionState();
    renderWithClient(<FriendCodeLookup actions={actions}/>);

    await userEvent.click(await screen.findByRole('button', {name: 'Copiar meu código de amizade'}));
    expect(writeText).toHaveBeenCalledWith('PKR-AAAA-BBBB-CCCC');

    await userEvent.type(screen.getByLabelText('Código de um amigo'), 'PKR-1{Enter}');
    expect(await screen.findByText('Bia')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Adicionar amigo/}));
    await waitFor(() => expect(actions.run).toHaveBeenCalledWith('request', 'p1'));
    expect(await screen.findByText('Solicitação enviada')).toBeInTheDocument();
  });

  test('reports an unknown code and recognises an existing friend', async () => {
    const {ApiError} = await import('@/lib/api/client');
    api.lookupFriendCode.mockRejectedValueOnce(new ApiError('missing', 404, {type: '/problems/not-found'}));
    renderWithClient(<FriendCodeLookup actions={actionState()}/>);
    await userEvent.type(await screen.findByLabelText('Código de um amigo'), 'PKR-9');
    await userEvent.click(screen.getByRole('button', {name: /Buscar/}));
    expect(await screen.findByRole('alert')).toHaveTextContent('Jogador não encontrado.');

    api.lookupFriendCode.mockResolvedValueOnce(player({relationship: 'friend'}));
    await userEvent.click(screen.getByRole('button', {name: /Buscar/}));
    expect(await screen.findByText('Já é seu amigo')).toBeInTheDocument();
  });

  test('survives a blocked clipboard and a missing friend code', async () => {
    Object.assign(navigator, {clipboard: {writeText: vi.fn().mockRejectedValue(new Error('denied'))}});
    api.getMe.mockResolvedValue({user_id: 'me'});
    renderWithClient(<FriendCodeLookup actions={actionState()}/>);
    expect(await screen.findByText('—')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Copiar meu código de amizade'})).toBeDisabled();
  });
});

describe('SocialInbox', () => {
  test('marks the visible unread events as read exactly once', async () => {
    renderWithClient(<SocialInbox events={[event({unread: true, type: 'friend_request', actor_name: 'Bia'})]}
                                  actions={actionState()}/>);
    await waitFor(() => expect(api.markInboxRead).toHaveBeenCalledWith(['e1']));
    expect(api.markInboxRead).toHaveBeenCalledTimes(1);
    expect(screen.getByText('Bia quer ser seu amigo.')).toBeInTheDocument();
  });

  test('keeps the badge up when the read receipt fails', async () => {
    api.markInboxRead.mockRejectedValueOnce(new Error('offline'));
    renderWithClient(<SocialInbox events={[event({unread: true})]} actions={actionState()}/>);
    await waitFor(() => expect(api.markInboxRead).toHaveBeenCalled());
  });

  test('enters or declines a pending invite', async () => {
    const actions = actionState();
    renderWithClient(<SocialInbox events={[event({room_id: 'room-1'})]} actions={actions}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Entrar'}));
    await waitFor(() => expect(api.push).toHaveBeenCalledWith('/table?id=room-1'));
    await userEvent.click(screen.getByRole('button', {name: 'Recusar'}));
    expect(actions.run).toHaveBeenCalledWith('decline-invite', 'e1');
  });

  test('hides the actions of an expired invite and pages the feed', async () => {
    const loadMore = vi.fn();
    renderWithClient(<SocialInbox events={[event({expires_at: Date.now() - 1_000, room_id: 'room-1'})]}
                                  actions={actionState()} hasNext onMoreAction={loadMore}/>);
    expect(screen.queryByRole('button', {name: 'Entrar'})).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Carregar mais'}));
    expect(loadMore).toHaveBeenCalled();
  });

  test('names an actor whose profile is gone with the shared placeholder', () => {
    // The server resolves actor_name for every live actor (#73); a row without
    // one is a deleted profile, not a missing list.
    renderWithClient(<SocialInbox events={[event({type: 'friend_request'})]} actions={actionState()}/>);
    expect(screen.getByText('Visitante quer ser seu amigo.')).toBeInTheDocument();
  });

  test('shows the loading, error and empty feed states', async () => {
    const retry = vi.fn();
    const {rerender} = renderWithClient(<SocialInbox events={[]} isLoading actions={actionState()}/>);
    expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true');

    rerender(<SocialInbox events={[]} isError onRetryAction={retry} actions={actionState()}/>);
    await userEvent.click(screen.getByRole('button', {name: /Tentar novamente/}));
    expect(retry).toHaveBeenCalled();

    rerender(<SocialInbox events={[]} actions={actionState()}/>);
    expect(screen.getByText('Nenhuma atividade por aqui.')).toBeInTheDocument();
  });
});

describe('PeopleNavBadge', () => {
  test('spells the unread count out for assistive tech and caps the dot at 9+', async () => {
    api.getSocialSummary.mockResolvedValue({unread_count: 12});
    const {container} = renderWithClient(<PeopleNavBadge/>);
    expect(await screen.findByText('9+')).toBeInTheDocument();
    expect(within(container).getByText('— 12 novidades em Pessoas')).toBeInTheDocument();
  });

  test('renders one singular novelty and nothing at zero', async () => {
    api.getSocialSummary.mockResolvedValue({unread_count: 1});
    const {container, rerender} = renderWithClient(<PeopleNavBadge/>);
    expect(await within(container).findByText('— 1 novidade em Pessoas')).toBeInTheDocument();

    api.getSocialSummary.mockResolvedValue({unread_count: 0});
    rerender(<PeopleNavBadge/>);
    await waitFor(() => expect(screen.queryByText('9+')).not.toBeInTheDocument());
  });
});
