'use client';

import {createContext, useCallback, useContext, useEffect, useMemo, useState, useSyncExternalStore, type ReactNode} from 'react';
import {useRouter} from 'next/navigation';
import {onNavigateMessage, registerServiceWorker, usePushSubscription, type UsePushSubscriptionResult} from '@aoctech/push-client';
import {getAccessToken, getPlayerId, subscribeAccessToken, subscribePlayerId} from '@/lib/api/client';
import {pushCallbacks} from '@/lib/push/callbacks';
import {PUSH_STAMP_KEY, PUSH_USER_KEY} from '@/lib/push/storage';

/** Chromium's install prompt event; not in the DOM lib types. */
interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>;
}

export interface PushContextValue extends UsePushSubscriptionResult {
  /** The browser offered to install the app (Chromium); iPhone never fires this, see the install steps instead. */
  canInstall: boolean;
  install: () => Promise<void>;
}

const PushContext = createContext<PushContextValue | null>(null);

/** Push status and actions for this device; the single instance lives in `PushProvider`. */
export function usePush(): PushContextValue {
  const value = useContext(PushContext);
  if (!value) throw new Error('usePush precisa estar dentro do PushProvider');
  return value;
}

/**
 * Registers the push service worker, keeps this device's subscription fresh once there is a session (`syncOnLoad`: it
 * never prompts) and routes notification clicks. Copy of the DF-e provider without its company handling.
 */
export function PushProvider({children}: {children: ReactNode}) {
  const router = useRouter();
  const token = useSyncExternalStore(subscribeAccessToken, getAccessToken, () => null);
  const playerId = useSyncExternalStore(subscribePlayerId, getPlayerId, () => null);

  useEffect(() => {
    void registerServiceWorker();
  }, []);

  useEffect(() => onNavigateMessage(path => router.push(path)), [router]);

  // One browser, one person. The sync stamp is per browser, so a different player signing in on a shared browser would be
  // told "synced 5 minutes ago" and the subscription would stay on the previous player's row. Dropping the stamp when the
  // signed-in player changes makes `syncOnLoad` resend it, and the API moves the endpoint to the current player.
  // Declared before `usePushSubscription` on purpose: effects run in order, so this one runs before the sync.
  useEffect(() => {
    if (!playerId) return;
    try {
      if (localStorage.getItem(PUSH_USER_KEY) !== playerId) {
        localStorage.removeItem(PUSH_STAMP_KEY);
        localStorage.setItem(PUSH_USER_KEY, playerId);
      }
    } catch {
      // Storage unavailable: the stamp's own 24 h expiry still resends the subscription.
    }
  }, [playerId]);

  // The sync needs the token and the player id (the stamp rule above), so wait for both.
  const push = usePushSubscription({...pushCallbacks, enabled: !!token && !!playerId});

  // `beforeinstallprompt` fires once, early, usually before the notifications screen exists: keep it here.
  const [installEvent, setInstallEvent] = useState<BeforeInstallPromptEvent | null>(null);
  useEffect(() => {
    const onPrompt = (e: Event) => {
      e.preventDefault();
      setInstallEvent(e as BeforeInstallPromptEvent);
    };
    const onInstalled = () => setInstallEvent(null);
    window.addEventListener('beforeinstallprompt', onPrompt);
    window.addEventListener('appinstalled', onInstalled);
    return () => {
      window.removeEventListener('beforeinstallprompt', onPrompt);
      window.removeEventListener('appinstalled', onInstalled);
    };
  }, []);
  const install = useCallback(async () => {
    if (!installEvent) return;
    await installEvent.prompt();
    setInstallEvent(null);
  }, [installEvent]);

  const value = useMemo<PushContextValue>(
    () => ({...push, canInstall: installEvent !== null, install}),
    [push, installEvent, install],
  );

  return <PushContext.Provider value={value}>{children}</PushContext.Provider>;
}
