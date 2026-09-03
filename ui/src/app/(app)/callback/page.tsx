'use client';
import Link from 'next/link';
import {Suspense, useEffect, useRef, useState} from 'react';
import {useRouter, useSearchParams} from 'next/navigation';
import {exchangeCode, OAuthExchangeError, startOAuthFlow} from '@/lib/auth/oauth';
import {setAccessToken, setUsername} from '@/lib/api/client';
import {navigateToUnavailable} from '@/lib/network/liveness';
import {reportClientError} from '@/lib/telemetry';
import {Button} from '@/components/ui/button';

type CallbackFailure = {
  // 'unavailable' never reaches state: it navigates away instead (see below).
  kind: 'transient' | 'invalid';
  correlationId: string;
};

function Callback() {
  const p = useSearchParams(), r = useRouter();
  const [failure, setFailure] = useState<CallbackFailure | null>(null);
  // The failure screen stays on screen during a manual retry — replacing it
  // with the neutral "Autenticando…" shell would throw away the reference code
  // the player may be reading out. The button carries the progress instead.
  const [retrying, setRetrying] = useState(false);
  const started = useRef(false);
  const codeRef = useRef(''), stateRef = useRef('');

  function attempt() {
    exchangeCode(codeRef.current, stateRef.current).then(x => {
      setAccessToken(x.accessToken);
      setUsername(x.username);
      r.replace(x.returnTo || '/lobby');
    }).catch((error: unknown) => {
      setRetrying(false);
      const kind = error instanceof OAuthExchangeError ? error.kind : 'transient';
      // An IdP-down 5xx isn't this page's problem to retry — the same
      // full-bleed outage screen a dead API gets is more honest than looping
      // the player through a sign-in that cannot succeed right now.
      if (kind === 'unavailable') {
        navigateToUnavailable();
        return;
      }
      const correlationId = reportClientError('OAuth callback exchange failed', {
        stack: error instanceof Error ? error.stack : undefined,
        context: {kind},
      });
      setFailure({kind, correlationId});
    });
  }

  useEffect(() => {
    const c = p.get('code'), s = p.get('state');
    if (!c || !s) {
      r.replace('/');
      return;
    }
    if (started.current) return;
    started.current = true;
    codeRef.current = c;
    stateRef.current = s;
    attempt();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [p, r]);
  // Same shell in every branch, and all of them are landmarks with a real page
  // heading: this route can sit on screen for a while, and an authentication
  // that finishes (or fails) without announcing itself leaves a screen-reader
  // user waiting on silence.
  if (failure?.kind === 'invalid') return (
    <main className="loading-screen">
      <h1>Não foi possível autenticar</h1>
      <p role="alert">O código de acesso expirou ou já foi usado. Entre novamente para continuar.</p>
      <Button onClick={() => startOAuthFlow()}>Tentar novamente</Button>
      <Button variant="ghost" render={<Link href="/"/>}>Voltar ao início</Button>
      <p>Código de referência: {failure.correlationId}</p>
    </main>
  );
  if (failure?.kind === 'transient') return (
    <main className="loading-screen">
      <h1>Não foi possível confirmar seu login</h1>
      <p role="alert">Isso costuma ser uma instabilidade passageira, não o fim do seu código de acesso. Tentar novamente deve resolver.</p>
      <Button loading={retrying} onClick={() => {
        setRetrying(true);
        attempt();
      }}>{retrying ? 'Tentando…' : 'Tentar novamente'}</Button>
      <Button variant="ghost" render={<Link href="/"/>}>Voltar ao início</Button>
      <p>Código de referência: {failure.correlationId}</p>
    </main>
  );
  return (
    <main className="loading-screen">
      <h1 className="sr-only">Autenticando</h1>
      <span className="loader" aria-hidden="true"/>
      <p role="status" aria-live="polite">Autenticando seu lugar…</p>
    </main>
  );
}

export default function Page() {
  return <Suspense><Callback/></Suspense>;
}
