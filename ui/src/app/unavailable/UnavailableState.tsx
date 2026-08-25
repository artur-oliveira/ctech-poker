'use client';

import {useState} from 'react';
import {SystemState} from '@/components/SystemState';
import {checkApiLiveness} from '@/lib/network/liveness';

const RETURN_AFTER_OUTAGE = 'poker:return-after-outage';

export function UnavailableState() {
  const [checking, setChecking] = useState(false);

  const retry = async () => {
    if (checking) return;
    setChecking(true);
    const available = await checkApiLiveness();
    if (!available) {
      setChecking(false);
      return;
    }
    let destination = '/lobby';
    try {
      destination = window.sessionStorage.getItem(RETURN_AFTER_OUTAGE) || destination;
      window.sessionStorage.removeItem(RETURN_AFTER_OUTAGE);
    } catch {
      // Keep the safe lobby destination when storage is unavailable.
    }
    window.location.replace(destination);
  };

  return <SystemState
    code="503"
    title="A mesa volta em breve."
    description="O servidor não respondeu, mas suas fichas e seu histórico continuam seguros."
    detail={checking ? 'Verificando se o serviço já voltou…' : 'Você pode verificar novamente sem recarregar a partida às cegas.'}
    onRetryAction={() => void retry()}
  />;
}
