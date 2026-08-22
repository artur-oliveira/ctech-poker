import {render, screen, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {BookOpen} from 'lucide-react';
import {describe, expect, test, vi} from 'vitest';
import {AppPageHeader, AppPageNav} from './AppPageChrome';

vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
vi.mock('@/components/social/PeopleNavBadge', () => ({PeopleNavBadge: () => <span>people-badge</span>}));

describe('shared app page chrome', () => {
  test('keeps Lobby in authenticated primary navigation and marks it as current', () => {
    render(<AppPageNav authed current="lobby"/>);
    const nav = screen.getByRole('navigation', {name: 'Navegação principal'});
    const lobby = within(nav).getByRole('link', {name: 'Lobby'});

    expect(lobby).toHaveAttribute('href', '/lobby');
    expect(lobby).toHaveAttribute('aria-current', 'page');
    expect(within(nav).getByText('profile-menu')).toBeInTheDocument();
    expect(within(nav).getByRole('link', {name: /Pessoas/})).toHaveAttribute('href', '/people');
    expect(within(nav).getByText('people-badge')).toBeInTheDocument();
  });

  test('omits Lobby and private navigation for anonymous visitors', () => {
    render(<AppPageNav authed={false}/>);
    const nav = screen.getByRole('navigation', {name: 'Navegação principal'});

    expect(within(nav).queryByRole('link', {name: 'Lobby'})).not.toBeInTheDocument();
    expect(within(nav).queryByText('profile-menu')).not.toBeInTheDocument();
    expect(within(nav).getByRole('link', {name: 'Voltar'})).toHaveAttribute('href', '/');
  });

  test('keeps route navigation out of page headers', () => {
    const {container} = render(<AppPageHeader icon={BookOpen} eyebrow="AJUDA" title="Guia"
      description="Aprenda a jogar."/>);

    expect(screen.getByRole('heading', {name: 'Guia'})).toBeInTheDocument();
    expect(container.querySelector('a')).not.toBeInTheDocument();
  });

  describe('mobile tab bar', () => {
    test('surfaces Lobby, Pessoas and Loja directly, marking the current one', () => {
      render(<AppPageNav authed current="people"/>);
      const tabBar = screen.getByRole('navigation', {name: 'Navegação rápida'});

      expect(within(tabBar).getByRole('link', {name: /Lobby/})).toHaveAttribute('href', '/lobby');
      const people = within(tabBar).getByRole('link', {name: /Pessoas/});
      expect(people).toHaveAttribute('href', '/people');
      expect(people).toHaveAttribute('aria-current', 'page');
      expect(within(tabBar).getByRole('link', {name: /Loja/})).toHaveAttribute('href', '/store');
      expect(within(tabBar).getByText('people-badge')).toBeInTheDocument();
    });

    test('tucks Guia, Ranking, Conquistas and Mãos behind "Mais"', async () => {
      render(<AppPageNav authed current="leaderboard"/>);
      const tabBar = screen.getByRole('navigation', {name: 'Navegação rápida'});

      expect(within(tabBar).queryByRole('link', {name: /Guia/})).not.toBeInTheDocument();
      const more = within(tabBar).getByRole('button', {name: 'Mais'});
      expect(more).toHaveClass('is-active');

      await userEvent.click(more);
      const menu = within(screen.getByLabelText('Mais opções'));
      const rankingLink = menu.getByRole('link', {name: /Ranking/});
      expect(rankingLink).toHaveAttribute('href', '/leaderboard');
      expect(rankingLink).toHaveAttribute('aria-current', 'page');
      expect(menu.getByRole('link', {name: /Guia/})).toHaveAttribute('href', '/guide');
      expect(menu.getByRole('link', {name: /Conquistas/})).toHaveAttribute('href', '/achievements');
      expect(menu.getByRole('link', {name: /Mãos/})).toHaveAttribute('href', '/hands');
    });

    test('does not mark "Mais" active for a primary route, and shows the reward dot on Loja', () => {
      render(<AppPageNav authed current="lobby" rewardReady/>);
      const tabBar = screen.getByRole('navigation', {name: 'Navegação rápida'});

      expect(within(tabBar).getByRole('button', {name: 'Mais'})).not.toHaveClass('is-active');
      expect(within(tabBar).getByText(/recompensa diária disponível/)).toBeInTheDocument();
    });

    test('is not rendered for anonymous visitors', () => {
      render(<AppPageNav authed={false}/>);
      expect(screen.queryByRole('navigation', {name: 'Navegação rápida'})).not.toBeInTheDocument();
    });
  });
});
