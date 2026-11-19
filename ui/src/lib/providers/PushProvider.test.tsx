import {act, cleanup, render} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';

const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  register: vi.fn().mockResolvedValue(null),
  subscription: vi.fn(),
  stampWhenSyncRan: vi.fn(),
  handler: null as null | ((path: string) => void),
}));

vi.mock('next/navigation', () => ({useRouter: () => ({push: mocks.push})}));
vi.mock('@aoctech/push-client', async () => {
  const {useEffect} = await import('react');
  return {
    registerServiceWorker: mocks.register,
    onNavigateMessage: (handler: (path: string) => void) => {
      mocks.handler = handler;
      return () => {
        mocks.handler = null;
      };
    },
    // Like the real hook, the "sync" happens in an effect once `enabled` is true; it reads the stamp the way the client does.
    usePushSubscription: (options: {enabled: boolean}) => {
      mocks.subscription(options);
      useEffect(() => {
        if (options.enabled) mocks.stampWhenSyncRan(localStorage.getItem('ctech-push:last-sync'));
      }, [options.enabled]);
      return {status: 'default', subscribe: vi.fn(), unsubscribe: vi.fn()};
    },
  };
});

import {setAccessToken, setPlayerId} from '@/lib/api/client';
import {PUSH_STAMP_KEY, PUSH_USER_KEY} from '@/lib/push/storage';
import {PushProvider, usePush} from '@/lib/providers/PushProvider';

function Probe() {
  const {status, canInstall} = usePush();
  return <p data-testid="probe">{status}/{String(canInstall)}</p>;
}

function renderProvider() {
  return render(<PushProvider><Probe/></PushProvider>);
}

const STAMP = '{"at":1763400000000,"kid":"k1"}';

beforeEach(() => {
  localStorage.clear();
  setAccessToken('token');
  setPlayerId('p1');
});
afterEach(() => {
  cleanup();
  setAccessToken(null);
  setPlayerId(null);
  vi.clearAllMocks();
});

describe('PushProvider', () => {
  it('registers the service worker and syncs only with a session and a known player', () => {
    renderProvider();
    expect(mocks.register).toHaveBeenCalledTimes(1);
    expect(mocks.subscription).toHaveBeenLastCalledWith(expect.objectContaining({enabled: true}));
    cleanup();
    setPlayerId(null);
    renderProvider();
    expect(mocks.subscription).toHaveBeenLastCalledWith(expect.objectContaining({enabled: false}));
    cleanup();
    setPlayerId('p1');
    setAccessToken(null);
    renderProvider();
    expect(mocks.subscription).toHaveBeenLastCalledWith(expect.objectContaining({enabled: false}));
  });

  it('a click on a notification routes to its link', () => {
    renderProvider();
    act(() => mocks.handler?.('/lobby'));
    expect(mocks.push).toHaveBeenCalledWith('/lobby');
  });

  describe('one browser, one person', () => {
    it('another player signs in on the same browser: the stamp drops so the sync resends', () => {
      localStorage.setItem(PUSH_STAMP_KEY, STAMP);
      localStorage.setItem(PUSH_USER_KEY, 'p-before');
      renderProvider();
      expect(localStorage.getItem(PUSH_STAMP_KEY)).toBeNull();
      expect(localStorage.getItem(PUSH_USER_KEY)).toBe('p1');
    });

    it('the same player again: the stamp stays, nothing is resent for no reason', () => {
      localStorage.setItem(PUSH_STAMP_KEY, STAMP);
      localStorage.setItem(PUSH_USER_KEY, 'p1');
      renderProvider();
      expect(localStorage.getItem(PUSH_STAMP_KEY)).toBe(STAMP);
    });

    it('no record of who synced last (logout cleared it, or first use): resend', () => {
      localStorage.setItem(PUSH_STAMP_KEY, STAMP);
      renderProvider();
      expect(localStorage.getItem(PUSH_STAMP_KEY)).toBeNull();
    });

    it('the stamp is already gone when the sync runs (effect order is what guarantees the resend)', () => {
      localStorage.setItem(PUSH_STAMP_KEY, STAMP);
      localStorage.setItem(PUSH_USER_KEY, 'p-before');
      renderProvider();
      expect(mocks.stampWhenSyncRan).toHaveBeenCalledWith(null);
    });
  });

  describe('install prompt', () => {
    it('is offered after beforeinstallprompt', async () => {
      const prompt = vi.fn().mockResolvedValue(undefined);
      const {getByTestId} = renderProvider();
      expect(getByTestId('probe').textContent).toBe('default/false');
      act(() => {
        const event = new Event('beforeinstallprompt', {cancelable: true});
        Object.assign(event, {prompt});
        window.dispatchEvent(event);
      });
      expect(getByTestId('probe').textContent).toBe('default/true');
    });
  });
});
