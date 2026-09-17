import {render, screen, waitFor, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {CosmeticCatalogEntry} from '@/lib/api/cosmeticPurchases';
import type {AchievementSummaryEntry} from '@/lib/api/achievements';
import {expectNoAxeViolations} from '@/test/axe';
import PlayerProfilePage from './page';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  mutate: vi.fn(),
  updateMe: vi.fn(),
  uploadAvatar: vi.fn(),
  deleteAvatar: vi.fn(),
  setQueryData: vi.fn(),
  invalidateQueries: vi.fn(),
  notify: vi.fn(),
  writeText: vi.fn(),
  saveShouldFail: false,
  realMoney: {enabled: false},
  state: {
    player: undefined as unknown,
    summary: undefined as unknown,
    summaryLoading: false,
    catalog: [] as CosmeticCatalogEntry[],
  },
}));

vi.mock('@/lib/capabilities', () => ({
  get REAL_MONEY_UI_ENABLED() {
    return mocks.realMoney.enabled;
  },
  availableWalletMode: (value: string | null | undefined) =>
    value === 'real' && mocks.realMoney.enabled ? 'real' : 'sandbox',
}));
vi.mock('@tanstack/react-query', () => ({
  useQuery: ({queryKey, enabled}: {queryKey: unknown[]; enabled?: boolean}) => {
    if (enabled === false) return {data: undefined, isLoading: false};
    mocks.query(queryKey);
    if (queryKey[0] === 'player') return {data: mocks.state.player, isLoading: !mocks.state.player};
    if (queryKey[0] === 'achievements') return {data: mocks.state.summary, isLoading: mocks.state.summaryLoading};
    if (queryKey[1] === 'cosmetic-catalog') return {data: mocks.state.catalog, isLoading: false};
    return {data: undefined, isLoading: false};
  },
  useQueryClient: () => ({setQueryData: mocks.setQueryData, invalidateQueries: mocks.invalidateQueries}),
  useMutation: ({mutationFn, onSuccess, onError}: {
    mutationFn: (input: unknown) => unknown;
    onSuccess?: (data: unknown, input: unknown) => void;
    onError?: (error: unknown, input: unknown) => void;
  }) => ({
    isPending: false,
    mutate: (input: unknown) => {
      mocks.mutate(input);
      if (mocks.saveShouldFail) {
        onError?.(new Error('rejected'), input);
        return;
      }
      void Promise.resolve(mutationFn(input)).then(data => onSuccess?.(data, input));
    },
  }),
}));
vi.mock('@/lib/api/player', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/player')>(),
  getMe: vi.fn(),
  updateMe: mocks.updateMe,
}));
vi.mock('@/lib/avatar', () => ({uploadAvatar: mocks.uploadAvatar, deleteAvatar: mocks.deleteAvatar}));
vi.mock('@/lib/notify', () => ({pushNotification: mocks.notify}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: {children: React.ReactNode}) => children}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
vi.mock('@/lib/hooks/useSocialUnread', () => ({useSocialUnread: () => 0}));
vi.mock('@/components/social/PeopleNavBadge', () => ({PeopleNavBadge: () => null}));
vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: {card: string}) => <span data-testid="playing-card">{card}</span>,
}));
// Decorative images keep an empty alt rather than an unnamed img role, which
// is what the real next/image emits and what axe expects to see here.
vi.mock('next/image', () => ({
  default: (props: {src: string; alt: string}) => props.alt
    ? <span role="img" aria-label={props.alt} data-src={props.src}/>
    : <span data-src={props.src}/>,
}));

const player = {
  user_id: 'player / one',
  name: 'Ana Silva',
  wallet_mode: 'sandbox',
  poker_terms_accepted: true,
  sandbox_balance: 12_345,
  game_balance: 98_760,
  showcase_public: false,
  playstyle_public: false,
  table_public: false,
  featured_achievements: [] as string[],
};

function entry(key: string, progress: number, thresholds: number[]): AchievementSummaryEntry {
  return {
    key, metric: key, progress,
    tiers: thresholds.map((threshold, index) => ({stars: index + 1, threshold})),
    stars: 0, unlocked: progress > 0, completed: false, next_target: null,
    max_target: thresholds[thresholds.length - 1],
  };
}

const summary = {
  achievements: [
    entry('bluff', 12, [1, 10, 100]),
    entry('wins', 40, [1, 10, 25, 100]),
    entry('tied', 4, [1, 10, 100]),
    entry('all_in', 30, [1, 25, 500]),
    entry('cooler', 0, [1, 10]),
  ],
};

/** The "N/3" counter, which shares its shape with a compact card's star readout. */
function featuredCount() {
  return document.querySelector('.player-profile-featured-count')?.textContent;
}

describe('/player-profile', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText: mocks.writeText}});
    mocks.writeText.mockResolvedValue(undefined);
    mocks.state.player = player;
    mocks.state.summary = summary;
    mocks.state.summaryLoading = false;
    mocks.state.catalog = [];
    mocks.saveShouldFail = false;
    mocks.realMoney.enabled = false;
    mocks.updateMe.mockImplementation(async (input: object) => ({...player, ...input}));
    mocks.uploadAvatar.mockResolvedValue({...player, avatar_url: '/avatars/one.jpg'});
    mocks.deleteAvatar.mockResolvedValue({...player, avatar_url: undefined});
  });

  test('gathers identity, showcase, table and balances on one page', () => {
    render(<PlayerProfilePage/>);

    expect(screen.getByRole('heading', {level: 1, name: 'Seu perfil'})).toBeInTheDocument();
    expect(screen.getAllByRole('heading', {level: 2}).map(node => node.textContent))
      .toEqual(['Identidade', 'Sua vitrine', 'Sua mesa', 'Seus saldos']);
    expect(screen.getByRole('textbox', {name: /Nome de exibição/})).toHaveValue('Ana Silva');
    expect(screen.getByRole('combobox', {name: 'Baralho'})).toBeInTheDocument();
    expect(screen.getByRole('definition')).toHaveTextContent('12.345');
    expect(screen.getByRole('button', {name: /Loja/})).toHaveAttribute('href', '/store');
  });

  test('keeps the landmark and the h1 while the profile is still loading', () => {
    mocks.state.player = undefined;
    render(<PlayerProfilePage/>);
    expect(screen.getByRole('heading', {level: 1, name: 'Seu perfil'})).toBeInTheDocument();
    expect(screen.getByText('Carregando seu perfil…')).toBeInTheDocument();
    expect(screen.queryByRole('heading', {level: 2})).not.toBeInTheDocument();
  });

  test('has no serious accessibility violations', async () => {
    const {container} = render(<PlayerProfilePage/>);
    await expectNoAxeViolations(container);
  });

  test('saves a trimmed display name only once it actually changed', async () => {
    render(<PlayerProfilePage/>);
    const save = screen.getByRole('button', {name: 'Salvar nome'});
    expect(save).toBeDisabled();

    const input = screen.getByRole('textbox', {name: /Nome de exibição/});
    await userEvent.clear(input);
    expect(screen.getByRole('button', {name: 'Salvar nome'})).toBeDisabled();
    await userEvent.type(input, '  Nova Ana  ');
    await userEvent.click(screen.getByRole('button', {name: 'Salvar nome'}));

    expect(mocks.mutate).toHaveBeenCalledWith({name: 'Nova Ana'});
    await waitFor(() => expect(mocks.setQueryData)
      .toHaveBeenCalledWith(['player', 'me'], expect.objectContaining({name: 'Nova Ana'})));
    expect(mocks.notify).toHaveBeenCalledWith('Agora você joga como Nova Ana.', 'info');
  });

  test('uploads a photo, then offers to remove it', async () => {
    const view = render(<PlayerProfilePage/>);
    expect(screen.getByRole('button', {name: 'Adicionar foto de perfil'})).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /Remover foto/})).not.toBeInTheDocument();

    const file = new File(['photo'], 'photo.png', {type: 'image/png'});
    await userEvent.upload(screen.getByLabelText('Selecionar foto de perfil'), file);
    expect(mocks.uploadAvatar).toHaveBeenCalledWith(file);
    await waitFor(() => expect(mocks.notify).toHaveBeenCalledWith('Foto de perfil atualizada.', 'info'));

    view.unmount();
    mocks.state.player = {...player, avatar_url: '/avatars/one.jpg'};
    render(<PlayerProfilePage/>);
    expect(screen.getByRole('button', {name: 'Trocar foto de perfil'})).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Remover foto/}));
    await waitFor(() => expect(mocks.notify).toHaveBeenCalledWith('Foto de perfil removida.', 'info'));
  });

  test('reports a photo the server would not take', async () => {
    mocks.saveShouldFail = true;
    render(<PlayerProfilePage/>);
    await userEvent.upload(screen.getByLabelText('Selecionar foto de perfil'),
      new File(['x'], 'x.png', {type: 'image/png'}));
    expect(mocks.notify).toHaveBeenCalledWith('Não foi possível atualizar a foto. Tente outra imagem.');
  });

  test('highlights an achievement, which jumps to the front of the rail', async () => {
    render(<PlayerProfilePage/>);
    expect(featuredCount()).toBe('0/3');
    expect(screen.getAllByRole('option')[0]).toHaveAccessibleName(/^Vitórias/);

    await userEvent.click(screen.getByRole('option', {name: /^Dividindo o Pote/}));
    expect(featuredCount()).toBe('1/3');
    const options = screen.getAllByRole('option');
    expect(options[0]).toHaveAccessibleName(/^Dividindo o Pote/);
    expect(options[0]).toHaveAttribute('aria-selected', 'true');

    // A second press takes it back out.
    await userEvent.click(options[0]);
    expect(featuredCount()).toBe('0/3');
  });

  test('stops at three highlights and says so', async () => {
    render(<PlayerProfilePage/>);
    for (const name of [/^Vitórias/, /^Tudo ou Nada/, /^Mestre do Blefe/]) {
      await userEvent.click(screen.getByRole('option', {name}));
    }
    expect(featuredCount()).toBe('3/3');

    await userEvent.click(screen.getByRole('option', {name: /^Dividindo o Pote/}));
    expect(mocks.notify).toHaveBeenCalledWith('Escolha no máximo 3 conquistas.', 'info');
    expect(featuredCount()).toBe('3/3');
  });

  test('only offers achievements the player has actually scored', () => {
    render(<PlayerProfilePage/>);
    expect(screen.getAllByRole('option')).toHaveLength(4);
    expect(screen.queryByRole('option', {name: /Sem Escapatória/})).not.toBeInTheDocument();
  });

  test('shows a skeleton while the achievement summary is unresolved', () => {
    mocks.state.summary = undefined;
    mocks.state.summaryLoading = true;
    render(<PlayerProfilePage/>);
    expect(screen.getByText('Carregando suas conquistas…')).toBeInTheDocument();
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  });

  test('teaches the way out when nothing has been scored yet', () => {
    mocks.state.summary = {achievements: [entry('cooler', 0, [1, 10])]};
    render(<PlayerProfilePage/>);
    expect(screen.getByText(/Você ainda não pontuou em nenhuma conquista/)).toBeInTheDocument();
    expect(screen.getByRole('link', {name: 'Ver o catálogo'})).toHaveAttribute('href', '/achievements');
  });

  test('saves privacy, highlights and layout in one request', async () => {
    render(<PlayerProfilePage/>);
    await userEvent.click(screen.getByRole('switch', {name: 'Vitrine pública'}));
    await userEvent.click(screen.getByRole('switch', {name: 'Estilo de jogo público'}));
    await userEvent.click(screen.getByRole('switch', {name: 'Mesa visível para amigos'}));
    await userEvent.click(screen.getByRole('option', {name: /^Vitórias/}));
    await userEvent.click(screen.getByRole('button', {name: 'Mover Conquistas em Destaque para baixo'}));
    await userEvent.click(screen.getByRole('button', {name: 'Salvar vitrine'}));

    expect(mocks.updateMe).toHaveBeenCalledWith({
      showcase_public: true,
      playstyle_public: true,
      table_public: true,
      featured_achievements: ['wins'],
      showcase_layout: {order: ['best_hand', 'achievements', 'matchup'], hidden: []},
    });
    await waitFor(() => expect(mocks.notify).toHaveBeenCalledWith('Vitrine do perfil atualizada.', 'info'));
  });

  test('reports a rejected showcase save without losing the draft', async () => {
    mocks.saveShouldFail = true;
    render(<PlayerProfilePage/>);
    await userEvent.click(screen.getByRole('switch', {name: 'Vitrine pública'}));
    await userEvent.click(screen.getByRole('button', {name: 'Salvar vitrine'}));

    expect(mocks.notify).toHaveBeenCalledWith('Não foi possível salvar a vitrine. Tente novamente.', 'error');
    expect(screen.getByRole('switch', {name: 'Vitrine pública'})).toHaveAttribute('aria-checked', 'true');
  });

  test('keeps the playstyle switch off while the showcase is private', async () => {
    render(<PlayerProfilePage/>);
    const playstyle = screen.getByRole('switch', {name: 'Estilo de jogo público'});
    expect(playstyle).toHaveAttribute('aria-disabled', 'true');

    await userEvent.click(screen.getByRole('switch', {name: 'Vitrine pública'}));
    expect(playstyle).not.toHaveAttribute('aria-disabled');
  });

  test('reorders sections with keyboard-reachable arrows and announces the move', async () => {
    render(<PlayerProfilePage/>);
    expect(screen.getByRole('button', {name: 'Mover Conquistas em Destaque para cima'})).toBeDisabled();
    expect(screen.getByRole('button', {name: 'Mover Cara a Cara para baixo'})).toBeDisabled();

    await userEvent.click(screen.getByRole('button', {name: 'Mover Conquistas em Destaque para baixo'}));
    expect(screen.getByText('Conquistas em Destaque agora em 2º lugar de 3.')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Mover Conquistas em Destaque para cima'}));
    expect(screen.getByText('Conquistas em Destaque agora em 1º lugar de 3.')).toBeInTheDocument();
  });

  test('hides Melhor Vitória and Cara a Cara, never Conquistas', async () => {
    render(<PlayerProfilePage/>);
    expect(screen.queryByRole('switch', {name: /Mostrar Conquistas em Destaque/})).not.toBeInTheDocument();
    const bestHand = screen.getByRole('switch', {name: 'Mostrar Melhor Vitória Recente na vitrine'});
    expect(bestHand).toHaveAttribute('aria-checked', 'true');
    await userEvent.click(bestHand);
    expect(bestHand).toHaveAttribute('aria-checked', 'false');

    await userEvent.click(screen.getByRole('button', {name: 'Salvar vitrine'}));
    expect(mocks.updateMe).toHaveBeenCalledWith(expect.objectContaining({
      showcase_layout: {order: ['achievements', 'best_hand', 'matchup'], hidden: ['best_hand']},
    }));
    await userEvent.click(bestHand);
    expect(bestHand).toHaveAttribute('aria-checked', 'true');
  });

  test('always offers the visitor preview, and the share link only once public', async () => {
    const view = render(<PlayerProfilePage/>);
    expect(screen.getByRole('button', {name: /Ver como visitante/}))
      .toHaveAttribute('href', '/profile?id=player%20%2F%20one&preview=1');
    expect(screen.queryByRole('button', {name: /Copiar link/})).not.toBeInTheDocument();

    view.unmount();
    mocks.state.player = {...player, showcase_public: true};
    render(<PlayerProfilePage/>);
    await userEvent.click(screen.getByRole('button', {name: /Copiar link/}));
    expect(mocks.writeText).toHaveBeenCalledWith(`${window.location.origin}/profile?id=player%20%2F%20one`);
    expect(mocks.notify).toHaveBeenCalledWith('Link do perfil copiado.', 'info');
  });

  test('switches the deck and confirms it for the next hand', async () => {
    render(<PlayerProfilePage/>);
    await userEvent.click(screen.getByRole('combobox', {name: 'Baralho'}));
    await userEvent.click(await screen.findByRole('option', {name: /Clássico/}));
    expect(mocks.mutate).toHaveBeenCalledWith({deck_variant: 'two-color'});
    await waitFor(() => expect(mocks.notify).toHaveBeenCalledWith('Baralho pronto para a próxima mão.', 'info'));
  });

  test('locks an unowned premium deck behind the store instead of selecting it', async () => {
    mocks.state.catalog = [{kind: 'deck', id: 'golden', premium: true, owned: false, price_fichas: 500_000}];
    render(<PlayerProfilePage/>);
    await userEvent.click(screen.getByRole('combobox', {name: 'Baralho'}));

    const golden = await screen.findByRole('option', {name: /Dourado/});
    expect(golden.tagName).toBe('A');
    expect(golden).toHaveAttribute('href', '/store#decks');
    expect(golden.querySelector('svg[aria-label*="500.000 fichas"]')).not.toBeNull();
    expect(mocks.mutate).not.toHaveBeenCalledWith(expect.objectContaining({deck_variant: 'golden'}));
  });

  test('an owned premium deck selects normally', async () => {
    mocks.state.catalog = [{kind: 'deck', id: 'golden', premium: true, owned: true, price_fichas: 500_000}];
    render(<PlayerProfilePage/>);
    await userEvent.click(screen.getByRole('combobox', {name: 'Baralho'}));
    const golden = await screen.findByRole('option', {name: 'Dourado'});
    expect(golden.tagName).not.toBe('A');
    await userEvent.click(golden);
    expect(mocks.mutate).toHaveBeenCalledWith({deck_variant: 'golden'});
  });

  test('hides the mode switch and the real-money balance while real money is off', () => {
    mocks.state.player = {...player, wallet_mode: 'real'};
    render(<PlayerProfilePage/>);
    expect(screen.queryByText('Modo de jogo')).not.toBeInTheDocument();
    expect(screen.queryByText(/R\$/)).not.toBeInTheDocument();
    expect(within(screen.getByRole('heading', {level: 2, name: 'Seus saldos'}).closest('section')!)
      .getByText('12.345')).toBeInTheDocument();
  });

  test('switches wallet mode when real money is on', async () => {
    mocks.realMoney.enabled = true;
    const view = render(<PlayerProfilePage/>);
    expect(screen.getByText(/R\$\s*987,60/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('switch', {name: 'Fichas'}));
    expect(mocks.mutate).toHaveBeenCalledWith({wallet_mode: 'real'});
    await waitFor(() => expect(mocks.notify).toHaveBeenCalledWith('Modo dinheiro real selecionado.', 'info'));

    view.unmount();
    mocks.state.player = {...player, wallet_mode: 'real'};
    render(<PlayerProfilePage/>);
    await userEvent.click(screen.getByRole('switch', {name: 'Dinheiro real'}));
    expect(mocks.mutate).toHaveBeenCalledWith({wallet_mode: 'sandbox'});
    await waitFor(() => expect(mocks.notify).toHaveBeenCalledWith('Modo fichas selecionado.', 'info'));
  });

  test('keeps the current mode when the server refuses the change', async () => {
    mocks.realMoney.enabled = true;
    mocks.saveShouldFail = true;
    render(<PlayerProfilePage/>);
    await userEvent.click(screen.getByRole('switch', {name: 'Fichas'}));
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['player', 'me']});
    expect(mocks.notify).toHaveBeenCalledWith(
      'Não foi possível trocar o modo de jogo. Seu modo atual foi mantido.');
  });

  test('a rejected name save reports nothing about the wallet', async () => {
    mocks.saveShouldFail = true;
    render(<PlayerProfilePage/>);
    const input = screen.getByRole('textbox', {name: /Nome de exibição/});
    await userEvent.type(input, 'x');
    await userEvent.click(screen.getByRole('button', {name: 'Salvar nome'}));
    expect(mocks.invalidateQueries).not.toHaveBeenCalled();
  });
});
