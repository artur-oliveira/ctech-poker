'use client';
import Link from 'next/link';
import {Suspense, useEffect, useState} from 'react';
import {useRouter, useSearchParams} from 'next/navigation';
import {exchangeCode, startOAuthFlow} from '@/lib/auth/oauth';
import {setAccessToken, setUsername} from '@/lib/api/client';
import {Button} from '@/components/ui/button';

function Callback() {
  const p = useSearchParams(), r = useRouter();
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    const c = p.get('code'), s = p.get('state');
    if (!c || !s) {
      r.replace('/');
      return;
    }
    exchangeCode(c, s).then(x => {
      setAccessToken(x.accessToken);
      setUsername(x.username);
      r.replace(x.returnTo || '/lobby');
    }).catch(() => setFailed(true));
  }, [p, r]);
  if (failed) return <div className="loading-screen">
    <h2>Não foi possível autenticar</h2>
    <p>O código de acesso expirou ou já foi usado. Entre novamente para continuar.</p>
    <Button onClick={() => startOAuthFlow()}>Tentar novamente</Button>
    <Button variant="ghost" render={<Link href="/"/>}>Voltar ao início</Button>
  </div>;
  return <div className="loading-screen"><span className="loader"/>Autenticando sua cadeira…</div>;
}

export default function Page() {
  return <Suspense><Callback/></Suspense>;
}
