import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';

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
vi.mock('@/lib/api/player', () => ({
  getMe: vi.fn(),
  updateMe: mocks.updateMe,
}));
vi.mock('@/lib/api/achievements', () => ({
  getAchievementCatalog: vi.fn(),
  getMyAchievements: vi.fn(),
}));
vi.mock('@/lib/notify', () => ({pushNotification: mocks.notify}));

import {ProfileShowcaseDialog} from './ProfileShowcaseDialog';

const me = {
  user_id: 'player / one',
  name: 'Ada',
  showcase_public: false,
  featured_achievements: ['first_hand'],
};
const catalog = [
  {key: 'first_hand'},
  {key: 'hands_10'},
  {key: 'hands_100'},
  {key: 'hands_1000'},
  {key: 'locked'},
];
const mine = catalog.map((item, index) => ({key: item.key, count: item.key === 'locked' ? 0 : index + 1}));

describe('ProfileShowcaseDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {writeText: mocks.writeText},
    });
    mocks.writeText.mockResolvedValue(undefined);
    mocks.query.mockImplementation(({queryKey}: {queryKey: string[]}) => {
      if (queryKey[0] === 'player') return {data: me};
      if (queryKey[1] === 'catalog') return {data: catalog, isLoading: false};
      return {data: mine, isLoading: false};
    });
  });

  test('renders only earned achievements and the current selection', () => {
    render(<ProfileShowcaseDialog open onOpenChange={vi.fn()}/>);
    expect(screen.getByText('Sua vitrine')).toBeInTheDocument();
    expect(screen.getByText('1/3')).toBeInTheDocument();
    expect(screen.queryByText('locked')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /Copiar link/})).not.toBeInTheDocument();
  });

  test('enforces the three-achievement maximum', () => {
    render(<ProfileShowcaseDialog open onOpenChange={vi.fn()}/>);
    fireEvent.click(screen.getByRole('checkbox', {name: 'hands 102 registrados'}));
    fireEvent.click(screen.getByRole('checkbox', {name: 'hands 1003 registrados'}));
    expect(screen.getByText('3/3')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('checkbox', {name: 'hands 10004 registrados'}));
    expect(mocks.notify).toHaveBeenCalledWith('Escolha no máximo três conquistas.', 'info');
    expect(screen.getByText('3/3')).toBeInTheDocument();
  });

  test('makes a public profile shareable and copies its encoded URL', async () => {
    render(<ProfileShowcaseDialog open onOpenChange={vi.fn()}/>);
    fireEvent.click(screen.getByRole('switch', {name: 'Perfil público'}));
    fireEvent.click(screen.getByRole('button', {name: /Copiar link/}));
    await waitFor(() => expect(mocks.writeText)
      .toHaveBeenCalledWith(`${window.location.origin}/profile?id=player%20%2F%20one`));
    expect(mocks.notify).toHaveBeenCalledWith('Link do perfil copiado.', 'info');
    expect(screen.getByRole('button', {name: /Ver perfil/}))
      .toHaveAttribute('href', '/profile?id=player%20%2F%20one');
  });

  test('saves privacy and selections into the shared profile cache', async () => {
    const updated = {...me, showcase_public: true, featured_achievements: ['first_hand', 'hands_10']};
    mocks.updateMe.mockResolvedValue(updated);
    render(<ProfileShowcaseDialog open onOpenChange={vi.fn()}/>);
    fireEvent.click(screen.getByRole('switch', {name: 'Perfil público'}));
    fireEvent.click(screen.getByRole('checkbox', {name: 'hands 102 registrados'}));
    fireEvent.click(screen.getByRole('button', {name: 'Salvar vitrine'}));
    expect(mocks.updateMe).toHaveBeenCalledWith({
      showcase_public: true,
      featured_achievements: ['first_hand', 'hands_10'],
    });
    await waitFor(() => expect(mocks.setQueryData).toHaveBeenCalledWith(['player', 'me'], updated));
    expect(mocks.notify).toHaveBeenCalledWith('Vitrine do perfil atualizada.', 'info');
  });

  test('shows a loader while achievement sources are unresolved', () => {
    mocks.query.mockImplementation(({queryKey}: {queryKey: string[]}) =>
      queryKey[0] === 'player' ? {data: me} : {isLoading: true});
    render(<ProfileShowcaseDialog open onOpenChange={vi.fn()}/>);
    expect(document.querySelector('.loader')).toBeInTheDocument();
  });
});
