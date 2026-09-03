'use client';

import {useEffect} from 'react';
import {SystemState} from '@/components/SystemState';
import {reportBoundaryError} from '@/lib/telemetry';

/** Last-resort boundary for failures in the root layout/provider tree. Route
 * boundaries keep handling ordinary screen errors closer to their source. */
export default function GlobalError({error, reset}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error(error);
    reportBoundaryError(error, 'global');
  }, [error]);

  return <html lang="pt-BR">
  <body>
  <SystemState
    code="500"
    title="O aplicativo encontrou um problema."
    description="A tela foi interrompida de forma segura. Nenhuma jogada deve ser repetida sem a confirmação da mesa."
    detail={error.digest ? `Referência do erro: ${error.digest}` : 'Tente iniciar o aplicativo novamente.'}
    onRetryAction={reset}
  />
  </body>
  </html>;
}
