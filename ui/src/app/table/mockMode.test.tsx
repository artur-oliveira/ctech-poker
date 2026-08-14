import {render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';

const mocks = vi.hoisted(() => ({
  params: new Map<string, string>(),
  realtimeHook: vi.fn(),
  seated: true,
}));

vi.mock('next/navigation', () => ({
  useRouter: () => ({push: vi.fn()}),
  useSearchParams: () => ({get: (key: string) => mocks.params.get(key) ?? null}),
}));
vi.mock('@tanstack/react-query', () => ({
  useQuery: ({queryKey}: {queryKey: string[]}) => queryKey[0] === 'seated'
    ? {data: {seated: mocks.seated, stack: 500}, isLoading: false} : {data: []},
  useQueryClient: () => ({setQueryData: vi.fn(), invalidateQueries: vi.fn()}),
}));
vi.mock('@/lib/hooks/useTableRealtime', () => ({
  useTableRealtime: (...args: unknown[]) => {
    mocks.realtimeHook(...args);
    return {
      snapshot: null, snapshotAt: 0, status: 'connecting', reconnectAttempt: 0, announcement: '',
      removed: null, retryNow: vi.fn(), chat: [], reactions: [],
    };
  },
}));
vi.mock('@/lib/mockConfig', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/mockConfig')>(), USE_MOCK: true,
}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: {children: React.ReactNode}) => children}));
vi.mock('@/components/table/BuyInPanel', () => ({BuyInPanel: () => <span>buy-in</span>}));
vi.mock('@/components/table/MockControls', () => ({
  MockControls: ({scenario, delay}: {scenario: string; delay: number}) =>
    <span>mock-controls:{scenario}:{delay}</span>,
}));

const ROOM_ID = '01ARZ3NDEKTSV4RRFFQ69G5FAV';

describe('table page in mock mode', () => {
  beforeEach(() => {
    mocks.params = new Map([['id', ROOM_ID]]);
    mocks.seated = true;
  });

  test('loads the mock control panel with the requested scenario and delay', async () => {
    mocks.params.set('scenario', 'side_pot');
    mocks.params.set('delay', '1200');
    const {default: TablePage} = await import('./page');
    render(<TablePage/>);
    expect(await screen.findByText('mock-controls:side_pot:1200')).toBeInTheDocument();
  });

  test('falls back to the default scenario and delay for unknown values', async () => {
    mocks.params.set('scenario', 'not-a-scenario');
    mocks.params.set('delay', '7');
    const {default: TablePage} = await import('./page');
    render(<TablePage/>);
    expect(await screen.findByText('mock-controls:full_hand:350')).toBeInTheDocument();
  });

  test('keeps the mock panel available before the buy-in ceremony', async () => {
    mocks.seated = false;
    const {default: TablePage} = await import('./page');
    render(<TablePage/>);
    expect(screen.getByText('buy-in')).toBeInTheDocument();
    expect(await screen.findByText('mock-controls:full_hand:350')).toBeInTheDocument();
  });
});
