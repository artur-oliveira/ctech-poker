'use client';

import React, {useEffect, useRef, useSyncExternalStore} from 'react';
import {CloudOff, RefreshCw, WifiOff} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {useQueryClient} from '@tanstack/react-query';
import {
  checkApiLiveness,
  getApiLivenessSnapshot,
  getServerApiLivenessSnapshot,
  livenessPollDelay,
  markApiOffline,
  subscribeApiLiveness,
} from './liveness';

export function useApiLiveness() {
  return useSyncExternalStore(subscribeApiLiveness, getApiLivenessSnapshot, getServerApiLivenessSnapshot);
}

export function NetworkProvider({children}: { children: React.ReactNode }) {
  const state = useApiLiveness();
  const queryClient = useQueryClient();
  const checkNowRef = useRef<() => void>(() => undefined);
  const banner = state.status === 'unavailable'
    && (typeof window === 'undefined' || window.location.pathname !== '/unavailable');

  // The notice is a strip that reserves its own height rather than a card
  // floating over the page, so navigation and the table's Lobby/Sair controls
  // stay reachable exactly when recovery matters. The height itself lives in
  // CSS (--api-bar-h); this only says whether the strip is up.
  useEffect(() => {
    const root = document.documentElement;
    if (banner) root.dataset.apiOffline = 'true';
    else delete root.dataset.apiOffline;
  }, [banner]);

  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | null = null;
    let failures = 0;
    let cancelled = false;
    async function runCheck() {
      if (timer) clearTimeout(timer);
      const wasUnavailable = getApiLivenessSnapshot().status === 'unavailable';
      const available = await checkApiLiveness();
      if (cancelled) return;
      failures = available ? 0 : failures + 1;
      if (available && wasUnavailable) void queryClient.refetchQueries({type: 'active'});
      timer = setTimeout(() => void runCheck(), livenessPollDelay(failures));
    }
    checkNowRef.current = () => void runCheck();
    void runCheck();
    const onOnline = () => void runCheck();
    const onOffline = () => {
      if (timer) clearTimeout(timer);
      markApiOffline();
      failures += 1;
      timer = setTimeout(() => void runCheck(), livenessPollDelay(failures));
    };
    const onVisibility = () => {
      if (document.visibilityState === 'visible' && getApiLivenessSnapshot().status !== 'available') void runCheck();
    };
    window.addEventListener('online', onOnline);
    window.addEventListener('offline', onOffline);
    document.addEventListener('visibilitychange', onVisibility);
    return () => {
      cancelled = true;
      checkNowRef.current = () => undefined;
      if (timer) clearTimeout(timer);
      window.removeEventListener('online', onOnline);
      window.removeEventListener('offline', onOffline);
      document.removeEventListener('visibilitychange', onVisibility);
    };
  }, [queryClient]);

  return <>
    {children}
    {banner && <NetworkStatusBanner
      offline={state.reason === 'offline'}
      onRetryAction={() => checkNowRef.current()}
    />}
  </>;
}

function NetworkStatusBanner({offline, onRetryAction}: { offline: boolean; onRetryAction: () => void }) {
  const Icon = offline ? WifiOff : CloudOff;
  return <aside className="network-status" role="status" aria-live="polite">
    <Icon aria-hidden="true"/>
    <div>
      <strong>{offline ? 'Você está sem internet' : 'Servidor temporariamente indisponível'}</strong>
      <span>{offline
        ? 'A mesa será reconectada quando sua conexão voltar.'
        : 'Seus dados continuam seguros. Estamos verificando a conexão sem sobrecarregar o servidor.'}</span>
    </div>
    <Button type="button" size="sm" variant="outline" onClick={onRetryAction}>
      <RefreshCw aria-hidden="true"/> Verificar agora
    </Button>
  </aside>;
}
