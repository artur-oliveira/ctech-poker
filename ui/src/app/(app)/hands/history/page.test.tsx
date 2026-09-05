import {act, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {HandItem} from '@/lib/api/player';
import HandHistoryPage from './page';

const mocks = vi.hoisted(() => ({
  params: new Map<string, string>(),
  query: vi.fn(),
  viewerId: vi.fn((): string | undefined => 'viewer'),
  setQueryData: vi.fn(),
  reportPlayer: vi.fn().mockResolvedValue({report_id: 'rep-1', status: 'open'}),
  getRelationships: vi.fn().mockResolvedValue([]),
  saveHandMeta: vi.fn().mockResolvedValue({hand_id: 'h1', review_marked: true}),
  noteProps: null as Record<string, unknown> | null,
  queryFns: new Map<string, () => unknown>(),
}));

vi.mock('next/navigation', () => ({useSearchParams: () => ({get: (key: string) => mocks.params.get(key) ?? null})}));
vi.mock('@tanstack/react-query', () => ({
  useQuery: mocks.query,
  useQueryClient: () => ({setQueryData: mocks.setQueryData}),
}));
vi.mock('@/lib/utils', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/utils')>();
  return {...actual, getViewerId: mocks.viewerId};
});
vi.mock('@/lib/api/social', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/social')>(),
  reportPlayer: mocks.reportPlayer,
  getRelationships: mocks.getRelationships,
}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: { children: React.ReactNode }) => children}));
vi.mock('@/components/table/PlayerNoteDialog', () => ({
  PlayerNoteDialog: (props: Record<string, unknown>) => {
    mocks.noteProps = props;
    return props.open ? <span>note-dialog</span> : null;
  },
}));
vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: { card: string }) => <span data-testid="card">{card}</span>,
}));
vi.mock('@/components/hands/OutcomeBadge', () => ({
  OutcomeBadge: ({outcome}: { outcome: string }) => <span>outcome:{outcome}</span>,
}));
vi.mock('@/components/hands/ActionTimeline', () => ({
  ActionTimeline: ({actions, resolveName}: {
    actions: Array<{ seq: number; player_id: string }>,
    resolveName: (id: string) => string,
  }) => <div data-testid="timeline">{actions.map(a => `${a.seq}:${resolveName(a.player_id)}`).join('|')}</div>,
}));
vi.mock('@/lib/api/handMeta', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/handMeta')>(),
  saveHandMeta: mocks.saveHandMeta,
}));
vi.mock('@/components/hands/DeckReveal', () => ({
  DeckReveal: ({serverSeed}: { serverSeed: string }) => <div>proof:{serverSeed}</div>,
}));
vi.mock('@/components/hands/PartialDeckProof', () => ({
  PartialDeckProof: ({rootCommitHash}: { rootCommitHash: string }) => <div>partial:{rootCommitHash}</div>,
}));
vi.mock('@/components/hands/HandExportButton', () => ({
  HandExportButton: ({actionsAvailable}: {actionsAvailable: boolean}) =>
    <button>{actionsAvailable ? 'exportar' : 'exportar resumo'}</button>
}));
vi.mock('@/components/hands/ShareHandDialog', () => ({ShareHandDialog: () => <button>compartilhar</button>}));

const hand: HandItem = {
  pk: 'viewer', sk: 'h1', table_id: 'table-123456', hand_id: 'h1', outcome: 'won',
  net_change: 500, ended_at: 1_700_000_000_000,
  hole_cards: ['AH', 'KH'], board: ['QH', 'JH', 'TH', '2C', '3D'],
  opponents: [
    {player_id: 'p2', name: 'Bia', hole_cards: ['AS', 'AD'], won: false},
    {player_id: 'p3', won: true},
  ],
  server_seed: 'seed-1', commit_hash: 'commit-1',
};

function queryState({
                      handData = hand,
                      handLoading = false,
                      handError = false,
                      historyData = {
                        actions: [
                          {
                            seq: 2,
                            player_id: 'p2',
                            action: 'check',
                            amount: 0,
                            timestamp: 200,
                            frame: {stage: 'flop', pot: 10}
                          },
                          {seq: 1, player_id: 'viewer', action: 'call', amount: 10, timestamp: 100},
                        ]
                      },
                      historyLoading = false,
                      historyError = false,
                      historyRefetch = vi.fn(),
                      relationshipsData = [] as unknown[],
                    }: Record<string, unknown> = {}) {
  mocks.queryFns.clear();
  mocks.query.mockImplementation(({queryKey, queryFn}: { queryKey: string[]; queryFn: () => unknown }) => {
    mocks.queryFns.set(queryKey[0], queryFn);
    if (queryKey[0] === 'hand') return {data: handData, isLoading: handLoading, isError: handError};
    if (queryKey[0] === 'hand-history') {
      return {data: historyData, isLoading: historyLoading, isError: historyError, refetch: historyRefetch};
    }
    if (queryKey[0] === 'social') return {data: relationshipsData, isLoading: false, isError: false};
    return {data: [], isLoading: false, isError: false}; // player-notes and anything else
  });
}

describe('hand detail page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.viewerId.mockReturnValue('viewer');
    mocks.reportPlayer.mockResolvedValue({report_id: 'rep-1', status: 'open'});
    mocks.params = new Map([['table_id', 'table/one'], ['hand_id', 'hand one']]);
    queryState();
  });
  
  test('rejects incomplete links without executing enabled queries', () => {
    mocks.params = new Map();
    render(<HandHistoryPage/>);
    expect(screen.getByRole('heading', {name: 'Este link de mão está incompleto'})).toBeInTheDocument();
    expect(screen.getByText(/Ver minhas mãos/).closest('a')).toHaveAttribute('href', '/hands');
    expect(mocks.query).toHaveBeenCalledWith(expect.objectContaining({enabled: false}));
  });
  
  test('renders loading and missing-hand errors', () => {
    queryState({handLoading: true});
    const view = render(<HandHistoryPage/>);
    expect(screen.getByText(/Carregando detalhes/)).toBeInTheDocument();
    queryState({handData: undefined, handError: true});
    view.rerender(<HandHistoryPage/>);
    expect(screen.getByRole('heading', {name: 'Não foi possível carregar esta mão'})).toBeInTheDocument();
    expect(screen.getByText(/conta certa/)).toBeInTheDocument();
  });
  
  test('integrates hand and chronologically sorted history responses', () => {
    render(<HandHistoryPage/>);
    expect(screen.getByText('outcome:won')).toBeInTheDocument();
    expect(screen.getByText('Resultado líquido')).toBeInTheDocument();
    expect(screen.getByText('+500 fichas')).toBeInTheDocument();
    expect(screen.getByText('Sandbox')).toBeInTheDocument();
    expect(screen.getAllByText('Royal flush').length).toBeGreaterThan(0);
    expect(screen.getByText('Cartas não reveladas')).toBeInTheDocument();
    expect(screen.getByTestId('timeline')).toHaveTextContent('1:Você|2:Bia');
    expect(screen.getByText('proof:seed-1')).toBeInTheDocument();
    expect(screen.getByRole('region', {name: 'Board final da mão'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Assistir replay/})).toHaveAttribute(
      'href', '/hands/replay?table_id=table%2Fone&hand_id=hand%20one&mode=sandbox'
    );
    expect(screen.getByRole('button', {name: 'exportar'})).toBeInTheDocument();
  });
  
  test('falls back to the per-position proof when the seed was withheld', () => {
    queryState({
      handData: {
        ...hand, server_seed: undefined,
        root_commit_hash: 'root-1',
        revealed_card_salts: {0: {card: 'AH', salt_hex: 'aa'}},
        unrevealed_card_hashes: {1: 'bb'},
      },
    });
    render(<HandHistoryPage/>);
    expect(screen.getByText('partial:root-1')).toBeInTheDocument();
    expect(screen.queryByText(/registrada antes da prova criptográfica/)).not.toBeInTheDocument();
  });

  test('does not honor a real-money URL while the UI capability is disabled', () => {
    mocks.params.set('mode', 'real');
    render(<HandHistoryPage/>);
    expect(screen.getByText('Sandbox')).toBeInTheDocument();
    expect(mocks.query).toHaveBeenCalledWith(expect.objectContaining({queryKey: ['hand', 'sandbox', 'hand one']}));
  });
  

  test('copies the table id and confirms it, then reports a clipboard failure', async () => {
    const writeText = vi.fn().mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('denied'));
    Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText}});
    render(<HandHistoryPage/>);

    await userEvent.click(screen.getByRole('button', {name: 'Copiar ID da mesa'}));
    expect(writeText).toHaveBeenCalledWith('table-123456');
    const copiedButton = await screen.findByRole('button', {name: 'ID da mesa copiado'});
    expect(copiedButton).toBeInTheDocument();

    // A second attempt (clipboard permission revoked mid-session) clears the
    // confirmation instead of leaving a stale "copiado" label behind.
    await userEvent.click(copiedButton);
    expect(writeText).toHaveBeenCalledTimes(2);
    expect(await screen.findByRole('button', {name: 'Copiar ID da mesa'})).toBeInTheDocument();
  });

  test('marks a tie with a handshake for the viewer and the sharing opponent', () => {
    queryState({
      handData: {
        ...hand, outcome: 'tied', net_change: 0,
        opponents: [{player_id: 'p2', name: 'Bia', hole_cards: ['AS', 'AD'], won: true}],
      },
    });
    const {container} = render(<HandHistoryPage/>);
    expect(container.querySelectorAll('.tie-mark')).toHaveLength(2);
    expect(screen.getByText('0 fichas')).toBeInTheDocument();
    expect(container.querySelector('.hand-net')).toHaveClass('even');
  });

  test('marks a loss without a crown and names an unidentified opponent', () => {
    queryState({
      handData: {
        ...hand, outcome: 'lost', net_change: -250, board: undefined, hole_cards: undefined,
        opponents: [{player_id: 'p3', won: true}],
      },
    });
    const {container} = render(<HandHistoryPage/>);
    expect(screen.getByText('-250 fichas')).toBeInTheDocument();
    expect(container.querySelector('.hand-net')).toHaveClass('loss');
    expect(screen.getByText('Adversário')).toBeInTheDocument();
    expect(container.querySelectorAll('.hand-category')).toHaveLength(0);
  });

  test('hides the replay launcher while the action history is still loading or frameless', () => {
    queryState({historyLoading: true});
    const view = render(<HandHistoryPage/>);
    expect(screen.queryByRole('button', {name: /Assistir replay/})).not.toBeInTheDocument();
    expect(screen.getByText(/Carregando histórico de ações/)).toBeInTheDocument();

    queryState({historyData: {actions: [{seq: 1, player_id: 'viewer', action: 'call', amount: 10, timestamp: 100}]}});
    view.rerender(<HandHistoryPage/>);
    expect(screen.queryByRole('button', {name: /Assistir replay/})).not.toBeInTheDocument();
  });

  test('tolerates a hand with no action list at all', () => {
    queryState({historyData: {}});
    render(<HandHistoryPage/>);
    expect(screen.getByTestId('timeline')).toBeEmptyDOMElement();
  });

  test('shows independent history error and unavailable fairness proof, and retries on demand', async () => {
    const historyRefetch = vi.fn();
    queryState({
      handData: {...hand, server_seed: undefined, commit_hash: undefined},
      historyError: true,
      historyRefetch,
    });
    render(<HandHistoryPage/>);
    expect(screen.getByText(/sequência de ações/)).toBeInTheDocument();
    // #117: a legacy hand with no proof is neutral prose, never the
    // `mismatch` failure red DeckReveal uses for a genuine hash divergence.
    const legacyProof = screen.getByText(/registrada antes da prova criptográfica/);
    expect(legacyProof).toBeInTheDocument();
    expect(legacyProof).toHaveClass('is-unavailable');
    expect(legacyProof).not.toHaveClass('mismatch');
    expect(screen.getByRole('button', {name: 'exportar resumo'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'compartilhar'})).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', {name: 'Tentar ações novamente'}));
    expect(historyRefetch).toHaveBeenCalledOnce();
  });

  test('links each opponent to their profile and mounts the actions menu, never for the viewer', () => {
    render(<HandHistoryPage/>);
    expect(screen.getByRole('link', {name: /Bia/})).toHaveAttribute('href', '/profile?id=p2');
    expect(screen.getByRole('button', {name: 'Ações para Bia'})).toBeInTheDocument();
    // p3 has no name, so both the row label and the menu fall back consistently.
    expect(screen.getByRole('link', {name: /Adversário/})).toHaveAttribute('href', '/profile?id=p3');
    expect(screen.getByRole('button', {name: 'Ações para Visitante'})).toBeInTheDocument();
    // The viewer's own seat is never wrapped in a profile link or given a menu.
    expect(screen.queryByRole('link', {name: /Você/})).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /Ações para Você/})).not.toBeInTheDocument();
  });

  test('hides the actions menu entirely when logged out but keeps the profile link', () => {
    mocks.viewerId.mockReturnValue(undefined);
    render(<HandHistoryPage/>);
    expect(screen.getByRole('link', {name: /Bia/})).toHaveAttribute('href', '/profile?id=p2');
    expect(screen.queryByRole('button', {name: /Ações para/})).not.toBeInTheDocument();
  });

  test('renders a degenerate opponent with no player_id as plain text, no link or menu', () => {
    queryState({handData: {...hand, opponents: [{player_id: '', name: 'Sem ID'}]}});
    render(<HandHistoryPage/>);
    expect(screen.getByText('Sem ID')).toBeInTheDocument();
    expect(screen.queryByRole('link', {name: /Sem ID/})).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /Ações para/})).not.toBeInTheDocument();
  });

  test('reporting an opponent from hand history carries the hand_id and table_id', async () => {
    const user = userEvent.setup();
    render(<HandHistoryPage/>);
    await user.click(screen.getByRole('button', {name: 'Ações para Bia'}));
    await screen.findByText('Ver perfil');
    await user.click(screen.getByRole('button', {name: 'Denunciar'}));
    await screen.findByRole('heading', {name: 'Denunciar Bia'});
    await user.click(screen.getByRole('button', {name: /Enviar denúncia/}));
    await waitFor(() => expect(mocks.reportPlayer).toHaveBeenCalledWith(expect.objectContaining({
      target_player_id: 'p2', surface: 'table_behavior', table_id: 'table-123456', hand_id: 'h1'
    })));
  });

  test('opens the private note editor for an opponent from the menu and syncs the cache on save', async () => {
    const user = userEvent.setup();
    render(<HandHistoryPage/>);
    await user.click(screen.getByRole('button', {name: 'Ações para Bia'}));
    await screen.findByText('Ver perfil');
    await user.click(screen.getByRole('button', {name: 'Editar nota privada'}));
    expect(mocks.noteProps?.opponent).toEqual({player_id: 'p2', name: 'Bia'});

    const note = {opponent_id: 'p2', note: 'agressivo'};
    act(() => (mocks.noteProps?.onSaved as (note: object) => void)(note));
    const [, updater] = mocks.setQueryData.mock.calls.at(-1)!;
    expect((updater as (current: object[]) => object[])([{opponent_id: 'p2', note: 'velho'}])).toEqual([note]);

    act(() => (mocks.noteProps?.onSaved as (note: object | null) => void)(null));
    const [, clearUpdater] = mocks.setQueryData.mock.calls.at(-1)!;
    expect((clearUpdater as (current: object[]) => object[])([{opponent_id: 'p2', note: 'velho'}])).toEqual([]);
  });

  test('dismissing the note dialog without saving closes it', async () => {
    const user = userEvent.setup();
    render(<HandHistoryPage/>);
    await user.click(screen.getByRole('button', {name: 'Ações para Bia'}));
    await screen.findByText('Ver perfil');
    await user.click(screen.getByRole('button', {name: 'Editar nota privada'}));
    expect(screen.getByText('note-dialog')).toBeInTheDocument();

    act(() => (mocks.noteProps?.onOpenChangeAction as (open: boolean) => void)(false));
    expect(screen.queryByText('note-dialog')).not.toBeInTheDocument();
  });

  test('marks a hand for review via the shared endpoint, aria-pressed reflecting the saved state', async () => {
    const user = userEvent.setup();
    render(<HandHistoryPage/>);
    const toggle = screen.getByRole('button', {name: 'Marcar para revisar'});
    expect(toggle).toHaveAttribute('aria-pressed', 'false');
    await user.click(toggle);
    expect(mocks.saveHandMeta).toHaveBeenCalledWith('hand one', expect.objectContaining({review_marked: true}));
  });

  test('surfaces a role="alert" error when saving the review marker fails, without crashing', async () => {
    mocks.saveHandMeta.mockRejectedValueOnce(new Error('network'));
    const user = userEvent.setup();
    render(<HandHistoryPage/>);
    await user.click(screen.getByRole('button', {name: 'Marcar para revisar'}));
    expect(await screen.findByRole('alert')).toHaveTextContent(/Não foi possível salvar o marcador/);
  });

  test('fetches relationships only for signed-in viewers with real opponent ids', () => {
    render(<HandHistoryPage/>);
    const relationshipsFn = mocks.queryFns.get('social');
    expect(relationshipsFn).toBeDefined();
    void relationshipsFn?.();
    expect(mocks.getRelationships).toHaveBeenCalledWith(['p2', 'p3']);
  });
});
