import {render, screen, within} from '@testing-library/react';
import {BookOpen} from 'lucide-react';
import {describe, expect, test, vi} from 'vitest';
import {AppPageHeader, AppPageNav} from './AppPageChrome';

vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));

describe('shared app page chrome', () => {
  test('keeps Lobby in authenticated primary navigation and marks it as current', () => {
    render(<AppPageNav authed current="lobby"/>);
    const nav = screen.getByRole('navigation', {name: 'Navegação principal'});
    const lobby = within(nav).getByRole('link', {name: 'Lobby'});

    expect(lobby).toHaveAttribute('href', '/lobby');
    expect(lobby).toHaveAttribute('aria-current', 'page');
    expect(within(nav).getByText('profile-menu')).toBeInTheDocument();
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
});
