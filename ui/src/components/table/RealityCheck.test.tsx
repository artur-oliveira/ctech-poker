import {act, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';

import {RealityCheck} from './RealityCheck';

const {useTablePreferences} = vi.hoisted(() => ({
  useTablePreferences: vi.fn(),
}));

vi.mock('@/lib/tablePreferences', () => ({useTablePreferences}));
vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({children, open}: React.PropsWithChildren<{ open: boolean }>) =>
    <div data-testid="dialog" data-open={String(open)}>{children}</div>,
  DialogContent: ({children}: React.PropsWithChildren) => <section>{children}</section>,
  DialogDescription: ({children}: React.PropsWithChildren) => <p>{children}</p>,
  DialogFooter: ({children}: React.PropsWithChildren) => <footer>{children}</footer>,
  DialogHeader: ({children}: React.PropsWithChildren) => <header>{children}</header>,
  DialogTitle: ({children}: React.PropsWithChildren) => <h2>{children}</h2>,
}));

describe('RealityCheck', () => {
  const joinedAt = Date.UTC(2026, 6, 28, 12, 0, 0) / 1000;
  
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-07-28T12:59:45Z'));
    useTablePreferences.mockReturnValue({preferences: {realityCheckMinutes: 60}});
  });
  
  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });
  
  test('opens at the configured boundary and presents a neutral session summary', async () => {
    render(<RealityCheck joinedAt={joinedAt} buyIn={1000} currentStack={1250}
                         handId="hand-1" handComplete isTurn={false}/>);
    
    expect(screen.getByTestId('dialog')).toHaveAttribute('data-open', 'false');
    await act(async () => vi.advanceTimersByTime(15_000));
    
    expect(screen.getByTestId('dialog')).toHaveAttribute('data-open', 'true');
    expect(screen.getByRole('heading', {name: /Pausa consciente/})).toBeInTheDocument();
    expect(screen.getByText('1h 0min')).toBeInTheDocument();
    expect(screen.getByText('Mãos concluídas').nextSibling).toHaveTextContent('1');
    expect(screen.getByText('Entrada acumulada').nextSibling).toHaveTextContent('1.000');
    expect(screen.getByText('Stack atual').nextSibling).toHaveTextContent('1.250');
    const result = screen.getByText('Resultado da sessão').nextSibling;
    expect(result).toHaveTextContent('+250');
    expect(result).toHaveClass('positive');
    
    fireEvent.click(screen.getByRole('button', {name: 'Continuar jogando'}));
    expect(screen.getByTestId('dialog')).toHaveAttribute('data-open', 'false');
  });
  
  test('never opens during the player turn and waits until that turn ends', async () => {
    const {rerender} = render(
      <RealityCheck joinedAt={joinedAt} buyIn={1000} currentStack={800}
                    handComplete={false} isTurn/>,
    );
    
    await act(async () => vi.advanceTimersByTime(30_000));
    expect(screen.getByTestId('dialog')).toHaveAttribute('data-open', 'false');
    
    rerender(<RealityCheck joinedAt={joinedAt} buyIn={1000} currentStack={800}
                           handComplete={false} isTurn={false}/>);
    expect(screen.getByTestId('dialog')).toHaveAttribute('data-open', 'true');
    const result = screen.getByText('Resultado da sessão').nextSibling;
    expect(result).toHaveTextContent('-200');
    expect(result).toHaveClass('negative');
  });
  
  test('counts each completed hand once and disables reminders when configured to zero', () => {
    useTablePreferences.mockReturnValue({preferences: {realityCheckMinutes: 0}});
    const {rerender} = render(
      <RealityCheck joinedAt={joinedAt} buyIn={500} currentStack={500}
                    handId="hand-1" handComplete isTurn={false}/>,
    );
    
    rerender(<RealityCheck joinedAt={joinedAt} buyIn={500} currentStack={500}
                           handId="hand-1" handComplete isTurn={false}/>);
    rerender(<RealityCheck joinedAt={joinedAt} buyIn={500} currentStack={500}
                           handId="hand-2" handComplete isTurn={false}/>);
    act(() => vi.advanceTimersByTime(120_000));
    
    expect(screen.getByText('Mãos concluídas').nextSibling).toHaveTextContent('2');
    expect(screen.getByText('Resultado da sessão').nextSibling).toHaveTextContent('0');
    expect(screen.getByTestId('dialog')).toHaveAttribute('data-open', 'false');
  });
});
