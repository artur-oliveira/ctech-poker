import {fireEvent, render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {ActiveTableBanner} from './ActiveTableBanner';
import {SelfHudDialog} from './SelfHudDialog';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  push: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({
  useQuery: mocks.query,
  useQueryClient: () => ({setQueryData: vi.fn()}),
  useMutation: () => ({mutate: vi.fn(), isPending: false}),
}));
vi.mock('next/navigation', () => ({useRouter: () => ({push: mocks.push})}));

describe('lobby player components', () => {
  beforeEach(() => vi.clearAllMocks());
  
  test('hides the active-table banner without an open session', () => {
    mocks.query.mockReturnValue({data: [{table_id: 'old', ended_at: 10}]});
    const {container} = render(<ActiveTableBanner/>);
    expect(container).toBeEmptyDOMElement();
  });
  
  test('routes the player back to their still-open table', () => {
    mocks.query.mockReturnValue({
      data: [
        {table_id: 'table / 1', buyin_amount: 100, ended_at: 0},
        {table_id: 'old', ended_at: 10},
      ],
    });
    render(<ActiveTableBanner/>);
    expect(screen.getByRole('heading', {name: 'Sua mesa continua aberta'})).toBeInTheDocument();
    expect(screen.getByText('Você ainda está sentado · entrada de 100 fichas sandbox')).toBeInTheDocument();
    expect(screen.queryByText('MESA EM ANDAMENTO')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Retomar mesa'}));
    expect(mocks.push).toHaveBeenCalledWith('/table?id=table%20%2F%201');
  });
  
  test('renders HUD loading, failure and zero-hand states', () => {
    mocks.query.mockReturnValueOnce({isLoading: true});
    const loading = render(<SelfHudDialog open onOpenChange={vi.fn()}/>);
    expect(screen.getByText('Calculando tendências…')).toBeInTheDocument();
    loading.unmount();
    
    mocks.query.mockReturnValueOnce({isLoading: false, data: undefined});
    const failed = render(<SelfHudDialog open onOpenChange={vi.fn()}/>);
    expect(screen.getByText('Não foi possível carregar agora.')).toBeInTheDocument();
    failed.unmount();
    
    mocks.query.mockReturnValueOnce({
      isLoading: false,
      data: {
        hands: 0, vpip_hands: 0, pfr_hands: 0, three_bet_hands: 0, three_bet_chances: 0,
        vpip_rate: 0, pfr_rate: 0, three_bet_rate: 0,
      },
    });
    render(<SelfHudDialog open onOpenChange={vi.fn()}/>);
    expect(screen.getByText('Suas tendências aparecem depois da primeira mão.')).toBeInTheDocument();
  });
  
  test('formats an initial HUD sample and explains that it is not representative yet', () => {
    mocks.query.mockReturnValue({
      isLoading: false,
      data: {
        hands: 12, vpip_hands: 4, pfr_hands: 2, three_bet_hands: 1, three_bet_chances: 8,
        vpip_rate: 1 / 3, pfr_rate: 1 / 6, three_bet_rate: .125,
      },
    });
    render(<SelfHudDialog open onOpenChange={vi.fn()}/>);
    expect(screen.getByText('12 mãos')).toBeInTheDocument();
    expect(screen.getByText('33,3%')).toBeInTheDocument();
    expect(screen.getByText(/Amostra inicial/)).toBeInTheDocument();
    expect(screen.queryByText('Seu estilo pré-flop')).not.toBeInTheDocument();
  });
  
  test('shows style classification and accessible radar for a mature sample', () => {
    mocks.query.mockReturnValue({
      isLoading: false,
      data: {
        hands: 100, vpip_hands: 45, pfr_hands: 32, three_bet_hands: 12, three_bet_chances: 50,
        vpip_rate: .45, pfr_rate: .32, three_bet_rate: .24,
        playstyle: [
          {key: 'explorer'}, {key: 'initiative'}, {key: 'counter'},
        ],
      },
    });
    render(<SelfHudDialog open onOpenChange={vi.fn()}/>);
    expect(screen.getByText('Explorador')).toBeInTheDocument();
    expect(screen.getAllByText('Iniciativa')).toHaveLength(2);
    expect(screen.getByText('Contra-ataque')).toBeInTheDocument();
    expect(screen.getByRole('img', {name: /Radar de estilo/})).toBeInTheDocument();
  });
});
