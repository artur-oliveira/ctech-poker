'use client';

import {useEffect, useRef, useState} from 'react';
import {SystemState} from '@/components/SystemState';
import {checkApiLiveness, livenessPollDelay} from '@/lib/network/liveness';

const RETURN_AFTER_OUTAGE = 'poker:return-after-outage';

function returnDestination() {
  try {
    const saved = window.sessionStorage.getItem(RETURN_AFTER_OUTAGE);
    window.sessionStorage.removeItem(RETURN_AFTER_OUTAGE);
    return saved || '/lobby';
  } catch {
    // Keep the safe lobby destination when storage is unavailable.
    return '/lobby';
  }
}

/**
 * `NetworkProvider` is mounted by the `(app)` layout, which this ungrouped
 * route deliberately sits outside of — so this screen owns its own liveness
 * loop. It reuses `livenessPollDelay`'s backoff, states the outcome of every
 * check (with the wall-clock time it ran), counts down to the next automatic
 * attempt, and navigates back to the interrupted route the moment one
 * succeeds, whether the player tapped anything or not.
 */
export function UnavailableState() {
  const [checking, setChecking] = useState(true);
  const [failedAt, setFailedAt] = useState<number | null>(null);
  const [retryInSeconds, setRetryInSeconds] = useState<number | null>(null);
  const checkNowRef = useRef<() => void>(() => undefined);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let countdown: ReturnType<typeof setInterval> | null = null;
    let failures = 0;

    function stopTimers() {
      if (timer) clearTimeout(timer);
      if (countdown) clearInterval(countdown);
      timer = null;
      countdown = null;
    }

    async function run() {
      if (cancelled) return;
      stopTimers();
      setChecking(true);
      setRetryInSeconds(null);
      const available = await checkApiLiveness();
      if (cancelled) return;
      setChecking(false);
      if (available) {
        window.location.replace(returnDestination());
        return;
      }
      failures += 1;
      setFailedAt(Date.now());
      const delay = livenessPollDelay(failures);
      setRetryInSeconds(Math.max(1, Math.round(delay / 1_000)));
      countdown = setInterval(
        () => setRetryInSeconds(value => value === null ? null : Math.max(0, value - 1)), 1_000);
      timer = setTimeout(() => void run(), delay);
    }

    checkNowRef.current = () => void run();
    void run();
    return () => {
      cancelled = true;
      checkNowRef.current = () => undefined;
      stopTimers();
    };
  }, []);

  const detail = checking ? 'Verificando se o serviço já voltou…'
    : failedAt !== null
      ? `Ainda fora do ar — última verificação às ${new Date(failedAt).toLocaleTimeString('pt-BR')}${
        retryInSeconds !== null ? ` · nova tentativa automática em ${retryInSeconds}s` : ''}`
      : 'Você pode verificar novamente sem recarregar a partida às cegas.';

  return <SystemState
    code="503"
    title="A mesa volta em breve."
    description="O servidor não respondeu, mas suas fichas e seu histórico continuam seguros."
    detail={detail}
    retryPending={checking}
    onRetryAction={() => checkNowRef.current()}
  />;
}
