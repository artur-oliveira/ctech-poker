import {act, render, waitFor} from '@testing-library/react';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {afterEach, expect, test, vi} from 'vitest';
import Store from './page';

// The store's initial-load budget measured at the wire: a real QueryClient,
// every request recorded at the API client. page.test.tsx asserts the same
// budget through `enabled`; this one counts the GETs.
const {get} = vi.hoisted(() => ({get: vi.fn()}));
vi.mock('@/lib/api/client', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/client')>(),
  apiClient: {get, post: vi.fn(), put: vi.fn(), delete: vi.fn()},
}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: { children: React.ReactNode }) => children}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => null}));
vi.mock('@/lib/hooks/useLobbyRealtime', () => ({useLobbyRealtime: vi.fn()}));
vi.mock('@/components/store/DailyRewardBanner', () => ({DAILY_REWARD_ANCHOR: 'daily', DailyRewardBanner: () => null}));
vi.mock('@/components/store/DailyRewardPanel', () => ({DailyRewardPanel: () => null}));
vi.mock('next/image', () => ({default: ({alt}: {alt: string}) => <div role="img" aria-label={alt}/>}));

afterEach(() => vi.unstubAllGlobals());

const catalogGets = (kind: string) =>
  get.mock.calls.filter(([url]) => url === `/v1.0/wallet/cosmetic-purchase/${kind}/catalog`).length;

test('first paint requests no avatar catalog; a department in view requests its own exactly once', async () => {
  get.mockResolvedValue({data: {data: [], has_next: false, next_cursor: null}});
  const observed: {callback: IntersectionObserverCallback; ids: string[]}[] = [];
  vi.stubGlobal('IntersectionObserver', class {
    ids: string[] = [];
    constructor(public callback: IntersectionObserverCallback) {
      observed.push({callback, ids: this.ids});
    }
    observe = (node: Element) => void this.ids.push(node.id);
    disconnect = vi.fn();
    unobserve = vi.fn();
  });
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  render(<QueryClientProvider client={client}><Store/></QueryClientProvider>);

  await waitFor(() => expect(catalogGets('deck')).toBe(1));
  expect(catalogGets('felt')).toBe(1);
  for (const kind of ['frame', 'hat', 'accessory']) expect(catalogGets(kind)).toBe(0);
  expect(get.mock.calls.some(([url]) => /cosmetic-purchase\/(frame|hat|accessory)\//.test(url))).toBe(false);

  const enter = (id: string) => act(() => observed.filter(entry => entry.ids.includes(id))
    .forEach(entry => entry.callback([{isIntersecting: true} as IntersectionObserverEntry], {} as IntersectionObserver)));
  enter('chapeus');
  await waitFor(() => expect(catalogGets('hat')).toBe(1));
  enter('molduras');
  enter('acessorios');
  await waitFor(() => expect(catalogGets('frame')).toBe(1));
  await waitFor(() => expect(catalogGets('accessory')).toBe(1));
  expect(catalogGets('hat')).toBe(1);
});
