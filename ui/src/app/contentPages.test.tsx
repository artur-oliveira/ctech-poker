import {render, screen, within} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import Guide from './guide/page';
import PokerRules from './poker-rules/page';

const mocks = vi.hoisted(() => ({authed: false}));

vi.mock('@/lib/auth/session', () => ({
  useOptionalSession: () => ({authed: mocks.authed, checking: false}),
}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
vi.mock('@/components/social/PeopleNavBadge', () => ({PeopleNavBadge: () => <span>people-badge</span>}));
vi.mock('next/image', () => ({
  default: ({alt}: { alt: string }) => <div role="img" aria-label={alt}/>,
}));
vi.mock('@/components/HandRankings', () => ({HandRankings: () => <ol aria-label="ranking de mãos"/>}));

describe('static learning pages', () => {
  beforeEach(() => {
    mocks.authed = false;
  });
  
  test('renders the complete guide and public calls to action', () => {
    render(<Guide/>);
    expect(screen.getByRole('heading', {name: 'Aprenda no seu ritmo'})).toBeInTheDocument();
    expect(screen.getByRole('heading', {name: 'Sua primeira mão em três movimentos'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Ver primeiros passos'})).toHaveAttribute('href', '/guide/basics');
    expect(screen.getByRole('link', {name: /Tudo sobre a mesa/})).toHaveAttribute('href', '/guide/table');
    expect(screen.getByRole('link', {name: /Mãos, replay e integridade/})).toHaveAttribute('href', '/guide/hands');
    expect(screen.getByRole('link', {name: /Comunidade e jogo seguro/})).toHaveAttribute('href', '/guide/community');
    expect(screen.getByText(/não podem ser sacadas/)).toBeInTheDocument();
    expect(screen.queryByRole('link', {name: 'Lobby'})).not.toBeInTheDocument();
    expect(document.querySelector('.page-heading-shell a')).not.toBeInTheDocument();
    expect(screen.queryByText('profile-menu')).not.toBeInTheDocument();
  });
  
  test('shows authenticated navigation and sends players back to the lobby', () => {
    mocks.authed = true;
    render(<Guide/>);
    expect(screen.getByText('profile-menu')).toBeInTheDocument();
    const nav = screen.getByRole('navigation', {name: 'Navegação principal'});
    expect(within(nav).getByRole('link', {name: 'Lobby'})).toHaveAttribute('href', '/lobby');
    expect(screen.getByRole('link', {name: 'Guia'})).toHaveAttribute('aria-current', 'page');
  });
  
  test('renders poker rules, hand rankings and provably-fair explanation', () => {
    render(<PokerRules/>);
    expect(screen.getByRole('heading', {name: "Regras do Texas Hold'em"})).toBeInTheDocument();
    expect(screen.getByRole('navigation', {name: 'Seções desta página'}).querySelectorAll('a')).toHaveLength(6);
    expect(screen.getByLabelText('ranking de mãos')).toBeInTheDocument();
    expect(screen.getByText(/Embaralhamento Provably Fair/)).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Ir para o Início'})).toHaveAttribute('href', '/');
  });
  
  test('uses private navigation and lobby CTA in rules for authenticated players', () => {
    mocks.authed = true;
    render(<PokerRules/>);
    expect(screen.getByText('profile-menu')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Ir para o Lobby'})).toHaveAttribute('href', '/lobby');
  });
});
