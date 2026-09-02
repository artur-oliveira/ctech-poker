'use client';

import {useEffect} from 'react';
import {SystemState} from '@/components/SystemState';
import {ApiError, redirectOnServiceUnavailable} from '@/lib/api/client';

export default function ErrorPage({error, reset}: { error: Error & { digest?: string }; reset: () => void }) {
  const api: ApiError | null = error instanceof ApiError ? error : null;
  const unavailable = api?.status === 503;

  useEffect(() => {
    console.error(error);
    // A render that threw a 503 is the same outage the API interceptor already
    // knows how to handle: hand it to the dedicated flow, which saves the
    // return path and navigates to the full /unavailable screen.
    if (unavailable) redirectOnServiceUnavailable(503);
  }, [error, unavailable]);

  if (unavailable) {
    return <SystemState
      code="503"
      title="A mesa volta em breve."
      description="O servidor não respondeu, mas suas fichas e seu histórico continuam seguros."
      detail="Levando você para a tela de manutenção…"
    />;
  }

  if (api?.status === 404) {
    return <SystemState
      code="404"
      title="Não encontramos esta mesa."
      description="A sala pode ter sido encerrada ou o link expirou, mas suas fichas e seu histórico continuam seguros."
      detail={error.digest ? `Referência do erro: ${error.digest}` : 'Volte ao lobby para escolher outra mesa.'}
      onRetryAction={reset}
    />;
  }

  return <SystemState
    code="500"
    title="Não conseguimos concluir esta jogada."
    description="Algo inesperado interrompeu a tela, mas nenhuma ação deve ser repetida sem confirmação da mesa."
    detail={error.digest ? `Referência do erro: ${error.digest}` : 'Tente carregar esta tela novamente.'}
    onRetryAction={reset}
  />;
}
