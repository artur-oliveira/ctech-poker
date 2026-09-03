import {act, render, screen} from '@testing-library/react';
import {afterEach, describe, expect, test, vi} from 'vitest';
import {IdleWarning} from './IdleWarning';

afterEach(() => {
  vi.useRealTimers();
});

describe('IdleWarning', () => {
  test('says nothing while the deadline is still more than a minute away', () => {
    vi.useFakeTimers();
    render(<IdleWarning deadline={Date.now() + 300_000} onKeepSeat={() => true}/>);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    // Nothing ticks during the quiet part: one timeout waits it out.
    act(() => void vi.advanceTimersByTime(239_000));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    act(() => void vi.advanceTimersByTime(1_000));
    expect(screen.getByRole('alert')).toHaveTextContent('removido por inatividade em 60s');
  });

  test('counts down once per second once armed', () => {
    vi.useFakeTimers();
    render(<IdleWarning deadline={Date.now() + 30_000} onKeepSeat={() => true}/>);

    act(() => void vi.advanceTimersByTime(0));
    expect(screen.getByRole('alert')).toHaveTextContent('em 30s');

    act(() => void vi.advanceTimersByTime(5_000));
    expect(screen.getByRole('alert')).toHaveTextContent('em 25s');

    act(() => void vi.advanceTimersByTime(30_000));
    expect(screen.getByRole('alert')).toHaveTextContent('em 0s');
  });

  test('renders nothing without a deadline', () => {
    render(<IdleWarning onKeepSeat={() => true}/>);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  test('starts the quiet wait over when the server re-arms a fresh deadline', () => {
    vi.useFakeTimers();
    const {rerender} = render(<IdleWarning deadline={Date.now() + 30_000} onKeepSeat={() => true}/>);
    act(() => void vi.advanceTimersByTime(0));
    expect(screen.getByRole('alert')).toBeInTheDocument();

    // A new idle spell: the previous spell's armed state must not carry over.
    rerender(<IdleWarning deadline={Date.now() + 300_000} onKeepSeat={() => true}/>);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  test('offers the keep-seat action', async () => {
    vi.useFakeTimers();
    const onKeepSeat = vi.fn(() => true);
    render(<IdleWarning deadline={Date.now() + 20_000} onKeepSeat={onKeepSeat}/>);
    act(() => void vi.advanceTimersByTime(0));

    act(() => void screen.getByRole('button', {name: 'Continuar na mesa'}).click());
    expect(onKeepSeat).toHaveBeenCalledTimes(1);
  });
});
