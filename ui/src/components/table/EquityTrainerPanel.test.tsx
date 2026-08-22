import {render, screen, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import type {SeatView} from '@/lib/api/table';
import {EquityTrainerPanel} from './EquityTrainerPanel';

vi.mock('./PlayingCard', () => ({
  PlayingCard: ({card}: { card?: string }) => <span data-testid="card">{card}</span>,
}));

const seat = (overrides: Partial<SeatView> = {}): SeatView => ({
  player_id: 'viewer', stack: 1000, state: 'active', contributed: 0,
  hole_cards: ['AH', 'AD'], hand_category: 'pair', equity: 0.62, ...overrides,
});

const board = ['KC', '5H', '2D'];

function renderPanel(props: Partial<React.ComponentProps<typeof EquityTrainerPanel>> = {}) {
  return render(<EquityTrainerPanel seat={seat()} isViewer currencyMode="sandbox" board={board} stage="flop"
                                     handComplete={false} isTurn={false} open {...props}/>);
}

describe('EquityTrainerPanel', () => {
  test('renders nothing outside sandbox rooms', () => {
    const {container} = renderPanel({currencyMode: 'real'});
    expect(container).toBeEmptyDOMElement();
  });

  test('renders nothing for a seat that is not the viewer', () => {
    const {container} = renderPanel({isViewer: false});
    expect(container).toBeEmptyDOMElement();
  });

  test('shows the category, matching cards and a plain-language reason once open', () => {
    renderPanel();
    expect(screen.getByText('Par')).toBeInTheDocument();
    expect(screen.getAllByTestId('card').map(el => el.textContent)).toEqual(['AH', 'AD']);
    expect(screen.getByText(/Você tem par/)).toBeInTheDocument();
    expect(screen.getByText(/Chance atual:/)).toBeInTheDocument();
  });

  test('withholds the explanation while it is the viewer\'s own turn, disabling the toggle', () => {
    renderPanel({isTurn: true});
    expect(screen.getByText(/Disponível depois da sua decisão/)).toBeInTheDocument();
    expect(screen.queryByText('Par')).not.toBeInTheDocument();
    expect(screen.getByRole('button', {name: /treinador/i})).toBeDisabled();
  });

  test('toggles open and closed from its own trigger', async () => {
    const user = userEvent.setup();
    render(<EquityTrainerPanel seat={seat()} isViewer currencyMode="sandbox" board={board} stage="flop"
                                handComplete={false} isTurn={false}/>);
    const toggle = screen.getByRole('button', {name: 'Ver treinador'});
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    await user.click(toggle);
    expect(screen.getByRole('button', {name: 'Fechar treinador'})).toHaveAttribute('aria-expanded', 'true');
  });

  test('reports toggle intent without overriding controlled state', async () => {
    const onOpenChangeAction = vi.fn();
    render(<EquityTrainerPanel seat={seat()} isViewer currencyMode="sandbox" board={board} stage="flop"
                                handComplete={false} isTurn={false} open={false}
                                onOpenChangeAction={onOpenChangeAction}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Ver treinador'}));
    expect(onOpenChangeAction).toHaveBeenCalledWith(true);
    expect(screen.getByRole('button', {name: 'Ver treinador'})).toHaveAttribute('aria-expanded', 'false');
  });

  test('shows a waiting reason instead of a false read before a category exists', () => {
    renderPanel({seat: seat({hand_category: undefined})});
    expect(screen.getByText(/Aguardando cartas suficientes/)).toBeInTheDocument();
    expect(screen.queryByTestId('card')).not.toBeInTheDocument();
  });

  test('shows the per-street equity recap only after the hand completes, with more than one street recorded', () => {
    const {rerender} = renderPanel({stage: 'flop', handId: 'hand-1', handComplete: false});
    expect(screen.queryByText('Turn')).not.toBeInTheDocument();

    rerender(<EquityTrainerPanel seat={seat({equity: 0.7})} isViewer currencyMode="sandbox" board={board}
                                  stage="turn" handId="hand-1" handComplete={false} isTurn={false} open/>);
    rerender(<EquityTrainerPanel seat={seat({equity: 0.9})} isViewer currencyMode="sandbox" board={board}
                                  stage="complete" handId="hand-1" handComplete isTurn={false} open/>);

    const history = within(document.querySelector('.equity-trainer-history')!);
    expect(history.getByText('Flop')).toBeInTheDocument();
    expect(history.getByText('Turn')).toBeInTheDocument();
    expect(history.getByText('90%')).toBeInTheDocument();
  });
});
