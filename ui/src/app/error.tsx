'use client';

import {useEffect} from 'react';
import {SystemState} from '@/components/SystemState';

export default function ErrorPage({error, reset}: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    console.error(error);
  }, [error]);
  
  return <SystemState
    code="500"
    title="Não conseguimos concluir esta jogada."
    description="Algo inesperado interrompeu a tela, mas nenhuma ação deve ser repetida sem confirmação da mesa."
    detail={error.digest ? `Referência do erro: ${error.digest}` : 'Tente carregar esta tela novamente.'}
    onRetry={reset}
  />;
}
