import {render} from '@testing-library/react';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {describe, expect, test} from 'vitest';
import type {TableSnapshot} from '@/lib/api/table';
import {useTableOutcome} from './useTableOutcome';

const queryClient = new QueryClient({defaultOptions: {queries: {retry: false}}});
const wrapper = ({children}: {children: React.ReactNode}) =>
  <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;

function baseSnapshot(overrides: Partial<TableSnapshot> = {}): TableSnapshot {
  return {stage: 'complete', board: [], seats: [], payouts: {}, ...overrides};
}

function Probe({snapshot, snapshotAt, onDuration}: {
  snapshot: TableSnapshot | null;
  snapshotAt: number;
  onDuration: (durationMs: number) => void;
}) {
  const {nextHandDurationMs} = useTableOutcome({id: 't1', viewer: 'p1', snapshot, snapshotAt});
  onDuration(nextHandDurationMs);
  return null;
}

describe('useTableOutcome next-hand duration', () => {
  test('computes the real span once a next-hand deadline is armed', () => {
    const durations: number[] = [];
    const onDuration = (value: number) => durations.push(value);
    const snapshotAt = 1_000_000;
    const deadline = snapshotAt + 12_000;

    render(<Probe snapshot={baseSnapshot()} snapshotAt={snapshotAt} onDuration={onDuration}/>, {wrapper});
    expect(durations.at(-1)).toBe(0);

    const {rerender} = render(
      <Probe snapshot={baseSnapshot({next_hand_unix_ms: deadline})} snapshotAt={snapshotAt} onDuration={onDuration}/>,
      {wrapper}
    );
    expect(durations.at(-1)).toBe(12_000);

    // A later broadcast repeating the same armed deadline (a reconnect, a
    // show_cards ping) must not recompute against its own later snapshotAt —
    // the duration stays pinned to the original arm.
    rerender(<Probe snapshot={baseSnapshot({next_hand_unix_ms: deadline})} snapshotAt={snapshotAt + 4_000}
                     onDuration={onDuration}/>);
    expect(durations.at(-1)).toBe(12_000);
  });

  // A hand that never armed a next-hand deadline (still WaitingForPlayers,
  // or a snapshot arriving before the countdown starts) must report 0, not
  // carry over whatever the previous hand happened to leave behind.
  test('does not leak a previous hand\'s duration when no deadline is armed', () => {
    const durations: number[] = [];
    const onDuration = (value: number) => durations.push(value);
    const snapshotAt = 1_000_000;
    const deadline = snapshotAt + 12_000;

    const {rerender} = render(
      <Probe snapshot={baseSnapshot({next_hand_unix_ms: deadline})} snapshotAt={snapshotAt} onDuration={onDuration}/>,
      {wrapper}
    );
    expect(durations.at(-1)).toBe(12_000);

    rerender(<Probe snapshot={baseSnapshot({next_hand_unix_ms: undefined})} snapshotAt={snapshotAt + 100}
                     onDuration={onDuration}/>);
    expect(durations.at(-1)).toBe(0);
  });
});
