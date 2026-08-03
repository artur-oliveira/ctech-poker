'use client';

import {SystemState} from '@/components/SystemState';

export default function GlobalError({reset}: { error: Error & { digest?: string }; reset: () => void }) {
  return <html lang="pt-BR">
  <body><SystemState
    code="500"
    title="O CTech Poker encontrou um erro."
    description="A aplicação não conseguiu continuar com segurança."
    detail="Atualize a experiência. Se o problema persistir, volte em alguns instantes."
    onRetryAction={reset}
  /></body>
  </html>;
}
