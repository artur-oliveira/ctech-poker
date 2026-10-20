'use client';

import React, {useEffect, useRef, useSyncExternalStore} from 'react';
import {OfflineBanner, type NetworkStatus} from '@aoctech/ui';
import {useQueryClient} from '@tanstack/react-query';
import {
  getApiLivenessSnapshot,
  getServerApiLivenessSnapshot,
  liveness,
  navigateToUnavailable,
  subscribeApiLiveness,
} from './liveness';

export const networkLabels = {
  offline: 'Você está sem internet',
  offlineHint: 'A mesa será reconectada quando sua conexão voltar.',
  unavailable: 'Servidor temporariamente indisponível',
  unavailableHint: 'Seus dados continuam seguros. Estamos verificando a conexão sem sobrecarregar o servidor.',
  retry: 'Verificar agora',
};

export function useApiLiveness() {
  return useSyncExternalStore(subscribeApiLiveness, getApiLivenessSnapshot, getServerApiLivenessSnapshot);
}

/**
 * Starts the app's single probe and shows the package banner. The package's own
 * NetworkProvider would create a second probe, and the api client and sockets
 * must share this one, so the instance is owned by `liveness.ts`.
 */
export function NetworkProvider({children}: { children: React.ReactNode }) {
  const queryClient = useQueryClient();
  const {status} = useSyncExternalStore(liveness.subscribe, liveness.getSnapshot, liveness.getSnapshot);
  // The product only ever showed offline/unavailable: one failed probe (degraded) stays silent.
  const shown: NetworkStatus = status === 'degraded' ? 'online' : status;
  const previous = useRef<NetworkStatus>('online');

  useEffect(() => liveness.start(), []);

  // The strip reserves its own height (--api-bar-h) rather than floating over the
  // page, so the table's Lobby/Sair controls stay reachable. CSS owns the height.
  useEffect(() => {
    const root = document.documentElement;
    if (shown !== 'online') root.dataset.apiOffline = 'true';
    else delete root.dataset.apiOffline;
    return () => { delete root.dataset.apiOffline; };
  }, [shown]);

  useEffect(() => {
    if (shown === 'unavailable') navigateToUnavailable();
    // Back online after an outage (link or server): refresh what is on screen.
    if (shown === 'online' && previous.current !== 'online') void queryClient.refetchQueries({type: 'active'});
    previous.current = shown;
  }, [shown, queryClient]);

  return <>
    {children}
    <div data-ctech-bridge="">
      <OfflineBanner className="network-status" status={shown} labels={networkLabels}
                     onRetry={() => void liveness.check()}/>
    </div>
  </>;
}
