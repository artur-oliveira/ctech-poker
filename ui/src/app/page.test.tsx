import {fireEvent, render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import Home from './page';

const mocks = vi.hoisted(() => ({
  startOAuthFlow: vi.fn(),
}));

vi.mock('@/lib/auth/oauth', () => ({startOAuthFlow: mocks.startOAuthFlow}));
vi.mock('next/image', () => ({
  default: ({src, alt}: { src: string; alt: string }) =>
    <span role="img" aria-label={alt || 'decorative image'} data-src={src}/>,
}));
vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: { card: string }) => <span data-testid="achievement-card">{card}</span>,
}));

describe('landing page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });
  
  test('renders the complete public navigation and product proposition', () => {
    render(<Home/>);
    
    expect(screen.getByRole('heading', {name: 'Chame seus amigos para jogar poker.'})).toBeInTheDocument();
    expect(screen.getByRole('link', {name: 'Novidades'})).toHaveAttribute('href', '#novidades');
    expect(screen.getByRole('link', {name: 'Por que jogar'})).toHaveAttribute('href', '#experience');
    expect(screen.getByRole('link', {name: 'Conquistas'})).toHaveAttribute('href', '#achievements');
    expect(screen.getByRole('link', {name: 'Regras'})).toHaveAttribute('href', '/poker-rules');
    expect(screen.getByRole('link', {name: 'Guia'})).toHaveAttribute('href', '/guide');
    expect(screen.getByRole('link', {name: 'Ranking'})).toHaveAttribute('href', '/leaderboard');
    expect(screen.getByText('Fichas sandbox')).toBeInTheDocument();
    expect(screen.getByText('2–9 jogadores')).toBeInTheDocument();
  });
  
  test('starts authentication with the correct destination from every call to action', () => {
    render(<Home/>);
    
    fireEvent.click(screen.getByRole('button', {name: 'Entrar'}));
    const playButtons = screen.getAllByRole('button', {name: /Jogar agora/});
    fireEvent.click(playButtons[0]);
    fireEvent.click(playButtons[1]);
    
    expect(mocks.startOAuthFlow).toHaveBeenNthCalledWith(1);
    expect(mocks.startOAuthFlow).toHaveBeenNthCalledWith(2, '/lobby');
    expect(mocks.startOAuthFlow).toHaveBeenNthCalledWith(3, '/lobby');
  });
  
  test('shows all feature, achievement and table preview content without API data', () => {
    render(<Home/>);
    
    expect(screen.getAllByRole('article')).toHaveLength(12);
    expect(screen.getByText('Mesa ao Vivo')).toBeInTheDocument();
    expect(screen.getByText('Conquistas e recompensas')).toBeInTheDocument();
    expect(screen.getByLabelText('Prévia de uma mesa de poker')).toBeInTheDocument();
    expect(screen.getByText('POTE')).toHaveTextContent('2.450');
    expect(screen.getByText('Você')).toBeInTheDocument();
    expect(screen.getAllByTestId('achievement-card')).toHaveLength(11);
    expect(screen.getByRole('img', {name: 'Mesa real do CTech Poker em andamento, com cartas comunitárias e barra de ações'}))
      .toHaveAttribute('data-src', '/guide/table-flop.webp');
    expect(screen.getByRole('img', {name: /Lobby do CTech Poker/}))
      .toHaveAttribute('data-src', '/guide/lobby.webp');
  });
  
  test('exposes the catalog, guide and legal destinations', () => {
    render(<Home/>);
    
    expect(screen.getByRole('link', {name: /Ver catálogo/})).toHaveAttribute('href', '/achievements');
    expect(screen.getByRole('link', {name: /Acessar o guia completo/})).toHaveAttribute('href', '/guide');
    expect(screen.getByRole('link', {name: 'Termos de Uso'})).toHaveAttribute(
      'href', 'https://accounts.aoctech.app/products/poker'
    );
    expect(screen.getByRole('link', {name: 'Política de privacidade'})).toHaveAttribute(
      'href', 'https://accounts.aoctech.app/products/poker-privacy'
    );
  });
});
