import {act, render, screen, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, describe, expect, test, vi} from 'vitest';
import {HandOutcomeBanner, type HandOutcomeState} from './HandOutcome';

vi.mock('@/lib/hooks/useDeckVariant', () => ({useDeckVariant: () => 'four-color'}));

afterEach(() => vi.mocked(window.matchMedia).mockImplementation(query => ({
  matches: false, media: query, onchange: null, addListener: vi.fn(), removeListener: vi.fn(),
  addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
} as unknown as MediaQueryList)));

const renderOutcome = (outcome: HandOutcomeState, props: Partial<{
  holdOpen: boolean; nextHandDeadlineMs: number; nextHandDurationMs: number;
}> = {}) => render(<HandOutcomeBanner outcome={outcome} holdOpen {...props}/>);

describe('HandOutcomeBanner', () => {
  test('renders only the standalone countdown when there is no outcome yet', () => {
    const {container, rerender} = render(<HandOutcomeBanner outcome={null} holdOpen={false}
      nextHandDeadlineMs={Date.now() + 5000} nextHandDurationMs={5000}/>);
    expect(container.querySelector('.hand-outcome-ring-standalone')).toBeInTheDocument();

    rerender(<HandOutcomeBanner outcome={null} holdOpen={false}/>);
    expect(container).toBeEmptyDOMElement();
  });

  test.each([
    ['win', 'Ver resultado: você venceu'],
    ['lose', 'Ver resultado: você perdeu'],
    ['tie', 'Ver resultado: pote dividido'],
    ['mixed', 'Ver resultado misto'],
    ['fold', 'Ver resultado: você desistiu'],
  ])('collapses a %s outcome into a badge that reopens it', async (kind, label) => {
    renderOutcome({key: 1, kind} as HandOutcomeState, {nextHandDeadlineMs: Date.now() + 5000});
    await userEvent.click(screen.getByRole('button', {name: 'Minimizar resultado'}));

    const badge = screen.getByRole('button', {name: label});
    expect(badge).toBeInTheDocument();
    await userEvent.click(badge);
    expect(screen.getByRole('button', {name: 'Minimizar resultado'})).toBeInTheDocument();
  });

  test('names both hands in a split pot and the players sharing it', () => {
    renderOutcome({
      key: 2, kind: 'tie', handCategory: 'pair',
      viewerCards: ['AH', 'AD', 'KC', 'QS', '2D'], viewerHoleCards: ['AH', 'AD'],
      tiedWith: [{name: 'Bia', cards: ['AS', 'AC', 'KH', 'QD', '2C']}, {cards: ['AH', 'AD', 'KC', 'QS', '2D']}],
      wonAmount: 300,
    });
    const rows = document.querySelectorAll('.hand-outcome-comparison-row');
    expect(rows).toHaveLength(3);
    expect(within(rows[0] as HTMLElement).getByText('Você')).toBeInTheDocument();
    expect(within(rows[1] as HTMLElement).getByText('Bia')).toBeInTheDocument();
    expect(within(rows[2] as HTMLElement).getByText('Jogador')).toBeInTheDocument();
    expect(screen.getByText('Mesma combinação. Os naipes não desempatam.')).toBeInTheDocument();
    expect(screen.getByText('300 fichas ganhas')).toBeInTheDocument();
  });

  test('falls back to a single hand when the split has no other named seats', () => {
    renderOutcome({key: 3, kind: 'tie', handCategory: 'pair', viewerCards: ['AH', 'AD', 'KC', 'QS', '2D']});
    expect(document.querySelectorAll('.hand-outcome-comparison-row')).toHaveLength(0);
    expect(screen.getByText('Mesma combinação. Os naipes não desempatam.')).toBeInTheDocument();
  });

  test('labels each contested pot in a mixed result', () => {
    renderOutcome({
      key: 4, kind: 'mixed', handCategory: 'pair',
      viewerCards: ['AH', 'AD', 'KC', 'QS', '2D'], viewerHoleCards: ['AH', 'AD'],
      pots: [
        {won: true},
        {won: false, winnerName: 'Bia', category: 'flush', winningCards: ['2H', '5H', '9H', 'JH', 'KH']},
        {won: false, winningCards: undefined},
      ],
      wonAmount: 100, refundAmount: 25,
    });
    expect(screen.getByText('Pote 1 · Você')).toBeInTheDocument();
    expect(screen.getByText('Pote 2 · Bia')).toBeInTheDocument();
    expect(screen.getByText('Pote 3 · Vencedor')).toBeInTheDocument();
    expect(screen.getByText('100 fichas ganhas e 25 fichas devolvidas')).toBeInTheDocument();
  });

  test('explains side pots the viewer did not contest using only resolved server data', () => {
    renderOutcome({
      key: 41, kind: 'win', wonAmount: 900,
      resolvedPots: [
        {
          amount: 900, payoutAmount: 900, winnerNames: ['Ana'], wonByViewer: true,
          viewerEligible: true, split: false, refund: false
        },
        {
          amount: 800, payoutAmount: 800, winnerNames: ['Bia'], wonByViewer: false,
          viewerEligible: false, split: false, refund: false
        }
      ]
    });
    expect(screen.getByText('Acerto dos potes')).toBeInTheDocument();
    expect(screen.getByText('Pote principal')).toBeInTheDocument();
    expect(screen.getByText('Pote lateral 1')).toBeInTheDocument();
    expect(screen.getByText('Você não disputou este pote')).toBeInTheDocument();
    expect(screen.getByText('Você +900')).toBeInTheDocument();
    expect(screen.getByText('Bia +800')).toBeInTheDocument();
  });

  test('marks a two-board settlement and names split and refund outcomes', () => {
    renderOutcome({
      key: 42, kind: 'tie', runItTwice: true,
      resolvedPots: [
        {
          amount: 600, payoutAmount: 600, viewerPayout: 300, winnerNames: ['Ana', 'Bia'], wonByViewer: true,
          viewerEligible: true, split: true, refund: false, runout: 1
        },
        {
          amount: 50, payoutAmount: 50, winnerNames: [], wonByViewer: false,
          viewerEligible: true, split: false, refund: true, runout: 2
        }
      ]
    });
    expect(screen.getByText('Rodado duas vezes')).toBeInTheDocument();
    expect(screen.getByText('Pote principal · Board 1')).toBeInTheDocument();
    expect(screen.getByText('Pote lateral 1 · Board 2')).toBeInTheDocument();
    expect(screen.getByText('Dividido · Você · 600 fichas distribuídas · sua parte +300')).toBeInTheDocument();
    expect(screen.getByText('Devolução +50')).toBeInTheDocument();
  });

  test('drops the pot label when a mixed result only has one pot to show', () => {
    renderOutcome({key: 5, kind: 'mixed', pots: [{won: true}]});
    expect(screen.getByText('Você')).toBeInTheDocument();
    expect(screen.queryByText(/Pote 1/)).not.toBeInTheDocument();
  });

  test('explains a loss where the higher combination, not a kicker, decided it', () => {
    renderOutcome({
      key: 6, kind: 'lose', winnerName: 'Bia',
      viewerCards: ['5H', '5D', 'KC', 'QS', '2D'], viewerHoleCards: ['5H', '5D'],
      winningCards: ['AH', 'AD', 'KH', 'QD', '3C'],
    });
    expect(screen.getByText('A combinação mais alta venceu.')).toBeInTheDocument();
    expect(screen.queryByText('Mesma combinação, o kicker decidiu.')).not.toBeInTheDocument();
  });

  test('falls back to generic hand labels when no cards were revealed', () => {
    renderOutcome({key: 7, kind: 'lose'});
    expect(screen.getByText('Vencedor')).toBeInTheDocument();
    expect(screen.getByText('Mão vencedora')).toBeInTheDocument();
    expect(screen.getByText('Sua mão')).toBeInTheDocument();
  });

  test('compares the folded hand against the revealed winner when it would have won', () => {
    renderOutcome({
      key: 8, kind: 'fold', couldHaveWon: true, winnerName: 'Bia',
      viewerCards: ['AH', 'AD', 'KC', 'QS', '2D'], viewerHoleCards: ['AH', 'AD'],
      winningCards: ['5H', '5D', 'KH', 'QD', '3C'],
    });
    expect(screen.getByText('Você poderia ter ganhado!')).toBeInTheDocument();
    expect(screen.getByText('Sua mão batia a mão revelada')).toBeInTheDocument();
    expect(screen.getByText('Sua mão (desistida)')).toBeInTheDocument();
  });

  test('shows no comparison for an ordinary fold', () => {
    renderOutcome({key: 9, kind: 'fold', couldHaveWon: false});
    expect(screen.getByText('Você desistiu.')).toBeInTheDocument();
    expect(document.querySelector('.hand-outcome-comparison')).toBeNull();
  });

  test('counts the stack through its base, delta and total phases on a win', () => {
    vi.useFakeTimers();
    try {
      const {container} = render(<HandOutcomeBanner holdOpen outcome={{
        key: 10, kind: 'win', handCategory: 'pair', stackBefore: 900, stackAfter: 1200, wonAmount: 300,
      }}/>);
      expect(container.querySelector('.hand-outcome-chips-base')).toHaveTextContent('900 fichas');

      act(() => void vi.advanceTimersByTime(300));
      expect(container.querySelector('.hand-outcome-chips-delta')).toHaveTextContent('900+300');

      act(() => void vi.advanceTimersByTime(300));
      expect(container.querySelector('.hand-outcome-chips-total')).toBeInTheDocument();
      expect(container.querySelector('.hand-outcome-chips')).toHaveClass('gain');
    } finally {
      vi.useRealTimers();
    }
  });

  test('counts down with a minus sign on a loss', () => {
    vi.useFakeTimers();
    try {
      const {container} = render(<HandOutcomeBanner holdOpen outcome={{
        key: 11, kind: 'lose', stackBefore: 900, stackAfter: 600,
      }}/>);
      act(() => void vi.advanceTimersByTime(300));
      expect(container.querySelector('.hand-outcome-chips-delta')).toHaveTextContent('900−300');
      expect(container.querySelector('.hand-outcome-chips')).toHaveClass('loss');
    } finally {
      vi.useRealTimers();
    }
  });

  test('skips straight to the counted total under reduced motion', () => {
    vi.mocked(window.matchMedia).mockReturnValue({matches: true} as MediaQueryList);
    const {container} = render(<HandOutcomeBanner holdOpen outcome={{
      key: 12, kind: 'win', stackBefore: 900, stackAfter: 1200,
    }}/>);
    expect(container.querySelector('.hand-outcome-chips-total')).toHaveTextContent('1.200 fichas');
    expect(container.querySelector('.hand-outcome-chips-base')).toBeNull();
  });

  test('shows no chip animation when the stack did not move', () => {
    const {container} = renderOutcome({key: 13, kind: 'tie', stackBefore: 900, stackAfter: 900});
    expect(container.querySelector('.hand-outcome-chips')).toBeNull();
  });

  test('announces the result once to assistive tech', () => {
    renderOutcome({
      key: 14, kind: 'win', handCategory: 'pair', viewerCards: ['AH', 'AD', 'KC', 'QS', '2D'],
      beatenCategory: 'pair', beatenCards: ['AS', 'AC', 'KH', 'QD', '3D'], wonAmount: 300,
    });
    expect(screen.getByRole('status')).toHaveTextContent(
      'Você venceu com Par, decidido pelo kicker: 300 fichas ganhas.');
    expect(screen.getByText('Mesma combinação, o kicker decidiu.')).toBeInTheDocument();
  });
});
