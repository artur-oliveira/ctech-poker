'use client'
import {Suspense, useEffect} from 'react';
import {useRouter, useSearchParams} from 'next/navigation';
import {exchangeCode} from '@/lib/auth/oauth';
import {setAccessToken} from '@/lib/api/client'

function Callback() {
  const p = useSearchParams(), r = useRouter();
  useEffect(() => {
    const c = p.get('code'), s = p.get('state');
    if (!c || !s) {
      r.replace('/');
      return
    }
    exchangeCode(c, s).then(x => {
      setAccessToken(x.accessToken);
      r.replace(x.returnTo || '/lobby')
    })
  }, [p, r]);
  return <div className="loading-screen"><span className="loader"/>Autenticando sua cadeira…</div>
}

export default function Page() {
  return <Suspense><Callback/></Suspense>
}
