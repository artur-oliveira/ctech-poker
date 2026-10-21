import {act, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {liveness} from './liveness';
import {NetworkProvider} from './NetworkProvider';

const mocks = vi.hoisted(() => ({refetchQueries: vi.fn(), replace: vi.fn()}));
vi.mock('@tanstack/react-query', () => ({useQueryClient: () => ({refetchQueries: mocks.refetchQueries})}));

const setOnLine = (value: boolean) => Object.defineProperty(navigator, 'onLine', {configurable: true, value});

async function takeServerDown() {
  vi.mocked(fetch).mockResolvedValue({ok: false} as Response);
  // The first call may join the probe the provider started on mount.
  for (let i = 0; i < 4; i++) await act(() => liveness.check());
}

describe('NetworkProvider', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ok: true} as Response));
    setOnLine(true);
    window.sessionStorage.clear();
    Object.defineProperty(window, 'location', {
      configurable: true, value: {pathname: '/table', search: '?id=room-1', replace: mocks.replace},
    });
  });

  // The probe is a singleton shared with the api client: leave it healthy for the next test.
  afterEach(async () => {
    vi.mocked(fetch).mockResolvedValue({ok: true} as Response);
    setOnLine(true);
    await liveness.check();
    vi.unstubAllGlobals();
  });

  test('stays silent while the API answers, and when a single probe blips', async () => {
    render(<NetworkProvider><main>Cached table</main></NetworkProvider>);
    expect(screen.getByText('Cached table')).toBeInTheDocument();
    await waitFor(() => expect(fetch).toHaveBeenCalled());
    vi.mocked(fetch).mockResolvedValue({ok: false} as Response);
    await act(() => liveness.check());
    expect(screen.getByRole('status')).toBeEmptyDOMElement();
    expect(document.documentElement.dataset.apiOffline).toBeUndefined();
  });

  // The strip reserves its own height instead of floating over the page, so
  // navigation and the table's Lobby/Sair controls stay reachable. The height
  // itself is CSS (--api-bar-h); this flag switches it on.
  test('keeps the app mounted, shows the strip with an immediate check, and clears it on recovery', async () => {
    render(<NetworkProvider><main>Cached table</main></NetworkProvider>);
    await takeServerDown();
    expect(screen.getByText('Cached table')).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('Servidor temporariamente indisponível');
    expect(document.documentElement.dataset.apiOffline).toBe('true');

    vi.mocked(fetch).mockResolvedValue({ok: true} as Response);
    const calls = vi.mocked(fetch).mock.calls.length;
    await userEvent.click(screen.getByRole('button', {name: 'Verificar agora'}));
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(calls + 1));
    await waitFor(() => expect(screen.getByRole('status')).toBeEmptyDOMElement());
    expect(document.documentElement.dataset.apiOffline).toBeUndefined();
  });

  test('refetches active screen data once the probe recovers', async () => {
    render(<NetworkProvider><span>content</span></NetworkProvider>);
    await takeServerDown();
    expect(mocks.refetchQueries).not.toHaveBeenCalled();
    vi.mocked(fetch).mockResolvedValue({ok: true} as Response);
    await act(() => liveness.check());
    expect(mocks.refetchQueries).toHaveBeenCalledWith({type: 'active'});
  });

  // A dead HAProxy surfaces as rejected fetches, not a clean 503: that shape
  // must reach the same full outage screen.
  test('escalates a persistent server outage to the full outage screen', async () => {
    render(<NetworkProvider><span>content</span></NetworkProvider>);
    await takeServerDown();
    expect(mocks.replace).toHaveBeenCalledWith('/unavailable');
    expect(window.sessionStorage.getItem('poker:return-after-outage')).toBe('/table?id=room-1');
  });

  test('distinguishes a device outage, never escalates it, and refetches when the link returns', async () => {
    render(<NetworkProvider><span>content</span></NetworkProvider>);
    await waitFor(() => expect(fetch).toHaveBeenCalled());
    setOnLine(false);
    act(() => window.dispatchEvent(new Event('offline')));
    expect(screen.getByRole('status')).toHaveTextContent('Você está sem internet');
    for (let i = 0; i < 5; i++) await act(() => liveness.check());
    expect(mocks.replace).not.toHaveBeenCalled();

    setOnLine(true);
    act(() => window.dispatchEvent(new Event('online')));
    await waitFor(() => expect(mocks.refetchQueries).toHaveBeenCalledWith({type: 'active'}));
    expect(screen.getByRole('status')).toBeEmptyDOMElement();
  });
});
