import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {ProfileShowcaseDialog} from './ProfileShowcaseDialog';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  mutate: vi.fn(),
  updateMe: vi.fn(),
  setQueryData: vi.fn(),
  notify: vi.fn(),
  writeText: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({
  useQuery: mocks.query,
  useQueryClient: () => ({setQueryData: mocks.setQueryData}),
  useMutation: ({mutationFn, onSuccess}: {
    mutationFn: () => Promise<unknown>;
    onSuccess: (data: unknown) => void;
  }) => ({
    isPending: false,
    mutate: () => {
      mocks.mutate();
      void mutationFn().then(onSuccess);
    },
  }),
}));
vi.mock('@/lib/api/player', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/player')>(),
  getMe: vi.fn(),
  updateMe: mocks.updateMe,
}));
vi.mock('@/lib/api/achievements', () => ({
  getAchievementCatalog: vi.fn(),
  getMyAchievements: vi.fn(),
  getMyAchievementsSummary: vi.fn(),
}));
vi.mock('@/lib/notify', () => ({pushNotification: mocks.notify}));

const me = {
  user_id: 'player / one',
  name: 'Ada',
  showcase_public: false,
  playstyle_public: false,
  featured_achievements: ['first_hand'],
};
const catalog = [
  {key: 'first_hand'},
  {key: 'hands_10'},
  {key: 'hands_100'},
  {key: 'hands_1000'},
  {key: 'locked'},
];
const mine = {
  achievements: catalog.map((item, index) => ({
    key: item.key, progress: item.key === 'locked' ? 0 : index + 1,
  })),
};

describe('ProfileShowcaseDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {writeText: mocks.writeText},
    });
    mocks.writeText.mockResolvedValue(undefined);
    mocks.query.mockImplementation(({queryKey}: { queryKey: string[] }) => {
      if (queryKey[0] === 'player') return {data: me};
      if (queryKey[1] === 'catalog') return {data: catalog, isLoading: false};
      return {data: mine, isLoading: false};
    });
  });
  
  test('renders only earned achievements and the current selection', () => {
    render(<ProfileShowcaseDialog open onOpenChangeAction={vi.fn()}/>);
    expect(screen.getByText('Sua vitrine')).toBeInTheDocument();
    expect(screen.getByText('1/3')).toBeInTheDocument();
    expect(screen.queryByText('locked')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /Copiar link/})).not.toBeInTheDocument();
  });
  
  test('enforces the three-achievement maximum', () => {
    render(<ProfileShowcaseDialog open onOpenChangeAction={vi.fn()}/>);
    fireEvent.click(screen.getByRole('checkbox', {name: 'hands 102 registrados'}));
    fireEvent.click(screen.getByRole('checkbox', {name: 'hands 1003 registrados'}));
    expect(screen.getByText('3/3')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('checkbox', {name: 'hands 10004 registrados'}));
    expect(mocks.notify).toHaveBeenCalledWith('Escolha no máximo três conquistas.', 'info');
    expect(screen.getByText('3/3')).toBeInTheDocument();
  });
  
  test('keeps sharing unavailable until public visibility is saved', async () => {
    render(<ProfileShowcaseDialog open onOpenChangeAction={vi.fn()}/>);
    fireEvent.click(screen.getByRole('switch', {name: 'Vitrine pública'}));
    fireEvent.click(screen.getByRole('switch', {name: 'Estilo de jogo público'}));
    expect(screen.queryByRole('button', {name: /Copiar link/})).not.toBeInTheDocument();
    expect(screen.queryByRole('link', {name: /Ver perfil/})).not.toBeInTheDocument();
  });
  
  test('saves privacy and selections into the shared profile cache', async () => {
    const updated = {
      ...me, showcase_public: true, playstyle_public: true,
      featured_achievements: ['first_hand', 'hands_10']
    };
    mocks.updateMe.mockResolvedValue(updated);
    render(<ProfileShowcaseDialog open onOpenChangeAction={vi.fn()}/>);
    fireEvent.click(screen.getByRole('switch', {name: 'Vitrine pública'}));
    fireEvent.click(screen.getByRole('switch', {name: 'Estilo de jogo público'}));
    fireEvent.click(screen.getByRole('checkbox', {name: 'hands 102 registrados'}));
    fireEvent.click(screen.getByRole('button', {name: 'Salvar vitrine'}));
    expect(mocks.updateMe).toHaveBeenCalledWith({
      showcase_public: true,
      playstyle_public: true,
      featured_achievements: ['first_hand', 'hands_10'],
      showcase_layout: {order: ['achievements', 'best_hand', 'matchup'], hidden: []},
    });
    await waitFor(() => expect(mocks.setQueryData).toHaveBeenCalledWith(['player', 'me'], updated));
    expect(mocks.notify).toHaveBeenCalledWith('Vitrine do perfil atualizada.', 'info');
  });
  
  test('reorders sections with keyboard-accessible arrows and announces the new position', async () => {
    mocks.updateMe.mockResolvedValue(me);
    render(<ProfileShowcaseDialog open onOpenChangeAction={vi.fn()}/>);
    expect(screen.getByRole('button', {name: 'Mover Conquistas em Destaque para cima'})).toBeDisabled();
    fireEvent.click(screen.getByRole('button', {name: 'Mover Conquistas em Destaque para baixo'}));
    expect(screen.getByText('Conquistas em Destaque agora em 2º lugar de 3.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Salvar vitrine'}));
    expect(mocks.updateMe).toHaveBeenCalledWith(expect.objectContaining({
      showcase_layout: {order: ['best_hand', 'achievements', 'matchup'], hidden: []},
    }));
  });

  test('hides Melhor Vitória / Cara a Cara via a switch, never Conquistas', () => {
    render(<ProfileShowcaseDialog open onOpenChangeAction={vi.fn()}/>);
    expect(screen.queryByRole('switch', {name: /Mostrar Conquistas em Destaque/})).not.toBeInTheDocument();
    const hideBestHand = screen.getByRole('switch', {name: 'Mostrar Melhor Vitória Recente na vitrine'});
    expect(hideBestHand).toHaveAttribute('aria-checked', 'true');
    fireEvent.click(hideBestHand);
    expect(hideBestHand).toHaveAttribute('aria-checked', 'false');
  });

  test('shows a skeleton while achievement sources are unresolved', () => {
    mocks.query.mockImplementation(({queryKey}: { queryKey: string[] }) =>
      queryKey[0] === 'player' ? {data: me} : {isLoading: true});
    render(<ProfileShowcaseDialog open onOpenChangeAction={vi.fn()}/>);
    expect(screen.getByText('Carregando suas conquistas…')).toBeInTheDocument();
    expect(document.querySelectorAll('.skeleton')).toHaveLength(3);
  });
});
