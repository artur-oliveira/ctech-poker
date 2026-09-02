'use client';
import Link from 'next/link';
import {Suspense, useEffect, useRef, useState} from 'react';
import {useRouter, useSearchParams} from 'next/navigation';
import {exchangeCode, startOAuthFlow} from '@/lib/auth/oauth';
import {setAccessToken, setUsername} from '@/lib/api/client';
import {Button} from '@/components/ui/button';

function Callback() {
  const p = useSearchParams(), r = useRouter();
  const [failed, setFailed] = useState(false);
  const started = useRef(false);
  useEffect(() => {
    const c = p.get('code'), s = p.get('state');
    if (!c || !s) {
      r.replace('/');
      return;
    }
    if (started.current) return;
    started.current = true;
    exchangeCode(c, s).then(x => {
      setAccessToken(x.accessToken);
      setUsername(x.username);
      r.replace(x.returnTo || '/lobby');
    }).catch(() => setFailed(true));
  }, [p, r]);
  // Same shell in both branches, and both are landmarks with a real page
  // heading: this route can sit on screen for a while, and an authentication
  // that finishes (or fails) without announcing itself leaves a screen-reader
  // user waiting on silence.
  if (failed) return (
    <main className="loading-screen">
      <h1>Não foi possível autenticar</h1>
      <p role="alert">O código de acesso expirou ou já foi usado. Entre novamente para continuar.</p>
      <Button onClick={() => startOAuthFlow()}>Tentar novamente</Button>
      <Button variant="ghost" render={<Link href="/"/>}>Voltar ao início</Button>
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
