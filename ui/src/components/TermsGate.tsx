'use client'
import {useEffect, useState} from 'react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {ShieldCheck} from 'lucide-react';
import {acceptPokerTerms, getMe} from '@/lib/api/player';
import {doRefresh, startOAuthFlow} from '@/lib/auth/oauth';
import {getAccessToken, setAccessToken, subscribeAccessToken} from '@/lib/api/client';
import {MOCK_PLAYER_ID, USE_MOCK} from '@/lib/mock';
import {Button} from '@/components/ui/button';
import {Checkbox} from '@/components/ui/checkbox';

const POKER_TERMS_URL = 'https://accounts.aoctech.app/products/poker';
const POKER_PRIVACY_URL = 'https://accounts.aoctech.app/products/poker-privacy';

export function TermsGate({children}: {children: React.ReactNode}) {
  const [token, setToken] = useState<string | null>(() => getAccessToken());
  const [checked, setChecked] = useState(false);
  const [booting, setBooting] = useState(() => !USE_MOCK && !getAccessToken());
  const queryClient = useQueryClient();

  useEffect(() => {
    const unsubscribe = subscribeAccessToken(setToken);
    if (USE_MOCK) {
      setAccessToken(MOCK_PLAYER_ID);
    } else if (!getAccessToken()) {
      void doRefresh().then(result => {
        if (result) setAccessToken(result.accessToken);
      }).finally(() => setBooting(false));
    }
    return unsubscribe;
  }, []);

  const me = useQuery({queryKey: ['player', 'me'], queryFn: getMe, enabled: Boolean(token)});
  const accept = useMutation({mutationFn: acceptPokerTerms, onSuccess: data => queryClient.setQueryData(['player', 'me'], data)});

  if (booting || me.isLoading) return <div className="loading-screen"><span className="loader"/>Verificando sua conta…</div>;
  if (!token) return <div className="terms-gate"><div>
    <ShieldCheck/><h1>Entre para continuar</h1>
    <p>Use sua conta CTech para acessar as mesas e manter suas preferências.</p>
    <Button className="w-full" size="lg" onClick={() => startOAuthFlow(typeof window === 'undefined' ? '/lobby' : window.location.pathname + window.location.search)}>Entrar com CTech Account</Button>
  </div></div>;
  if (me.isError) return <div className="terms-gate"><div>
    <h1>Não foi possível carregar seu perfil</h1><p>Tente novamente em alguns instantes.</p>
    <Button variant="outline" onClick={() => void me.refetch()}>Tentar novamente</Button>
  </div></div>;
  if (!me.data?.poker_terms_accepted) return <div className="terms-gate"><div>
    <ShieldCheck/><p className="gate-eyebrow">ANTES DE JOGAR</p><h1>Confirme os termos do CTech Poker</h1>
    <p>Leia os documentos publicados na Central Jurídica CTech. O aceite é necessário para acessar mesas sandbox ou de dinheiro real.</p>
    <label className="gate-check"><Checkbox checked={checked} onCheckedChange={value => setChecked(value === true)}/><span>Li e aceito os <a href={POKER_TERMS_URL} target="_blank" rel="noreferrer">Termos do CTech Poker</a> e a <a href={POKER_PRIVACY_URL} target="_blank" rel="noreferrer">Política de Privacidade do CTech Poker</a>.</span></label>
    {accept.isError && <p className="form-error">Não foi possível registrar o aceite.</p>}
    <Button className="w-full" size="lg" disabled={!checked || accept.isPending} onClick={() => accept.mutate()}>{accept.isPending ? 'Registrando…' : 'Aceitar e continuar'}</Button>
  </div></div>;
  return children;
}
