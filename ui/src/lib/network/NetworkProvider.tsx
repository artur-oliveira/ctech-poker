'use client';

// standards:allow Wrapper around @aoctech/ui NetworkProvider (shared liveness injected). Remaining local gap: the package banner also shows `degraded` (poker keeps one failed probe silent) and does not set the --api-bar-h html flag, so the banner is drawn here with the product's labels.

import React, {useEffect, useRef, useSyncExternalStore} from 'react';
import {NetworkProvider as PackageNetworkProvider, OfflineBanner, useNetwork, type NetworkStatus} from '@aoctech/ui';
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
 * The package provider, fed the app's single probe (api client and sockets
 * share it, owned by `liveness.ts`). It starts the probe and escalates a
 * confirmed outage; the strip and the refetch live in `ApiStatus` below.
 */
export function NetworkProvider({children}: { children: React.ReactNode }) {
  return <PackageNetworkProvider liveness={liveness} banner={false} onUnavailable={navigateToUnavailable}>
    {children}
    <ApiStatus/>
  </PackageNetworkProvider>;
}

function ApiStatus() {
  const queryClient = useQueryClient();
  const {status, retry} = useNetwork();
  // The product only ever showed offline/unavailable: one failed probe (degraded) stays silent.
  const shown: NetworkStatus = status === 'degraded' ? 'online' : status;
  const previous = useRef<NetworkStatus>('online');

  // The strip reserves its own height (--api-bar-h) rather than floating over the
  // page, so the table's Lobby/Sair controls stay reachable. CSS owns the height.
  useEffect(() => {
    const root = document.documentElement;
    if (shown !== 'online') root.dataset.apiOffline = 'true';
    else delete root.dataset.apiOffline;
    return () => { delete root.dataset.apiOffline; };
  }, [shown]);

  // Back online after an outage (link or server): refresh what is on screen.
  useEffect(() => {
    if (shown === 'online' && previous.current !== 'online') void queryClient.refetchQueries({type: 'active'});
    previous.current = shown;
  }, [shown, queryClient]);

  return <div data-ctech-bridge="">
    <OfflineBanner className="network-status" status={shown} labels={networkLabels} onRetry={retry}/>
  </div>;
}
