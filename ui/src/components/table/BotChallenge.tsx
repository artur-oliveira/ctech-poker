'use client';
import {useEffect, useRef, useState} from 'react';
import {LoaderCircle, ShieldCheck} from 'lucide-react';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle} from '@/components/ui/dialog';

const TURNSTILE_SCRIPT_ID = 'cloudflare-turnstile-script';
const TURNSTILE_SCRIPT = 'https://challenges.cloudflare.com/turnstile/v0/api.js';

type TurnstileAPI = {
  render: (element: HTMLElement, options: Record<string, unknown>) => string;
  remove: (widgetId: string) => void;
};

function turnstileAPI() {
  return (window as typeof window & { turnstile?: TurnstileAPI }).turnstile;
}

export function BotChallenge({required, onTokenAction}: {
  required: boolean;
  onTokenAction: (token: string) => boolean;
}) {
  const siteKey = process.env.NEXT_PUBLIC_TURNSTILE_SITE_KEY || '';
  const containerRef = useRef<HTMLDivElement>(null);
  const widgetRef = useRef('');
  const [status, setStatus] = useState<'loading' | 'ready' | 'checking' | 'error'>('loading');
  
  useEffect(() => {
    if (!required || !siteKey) return undefined;
    let cancelled = false;
    const render = () => {
      const api = turnstileAPI();
      if (cancelled || !api || !containerRef.current || widgetRef.current) return;
      widgetRef.current = api.render(containerRef.current, {
        sitekey: siteKey,
        action: 'poker_bot_check',
        appearance: 'interaction-only',
        theme: 'dark',
        language: 'pt-BR',
        callback: (token: string) => {
          setStatus('checking');
          if (!onTokenAction(token)) setStatus('error');
        },
        'error-callback': () => setStatus('error'),
        'expired-callback': () => setStatus('error')
      });
      setStatus('ready');
    };
    const existing = document.getElementById(TURNSTILE_SCRIPT_ID) as HTMLScriptElement | null;
    if (existing) {
      if (turnstileAPI()) render();
      else existing.addEventListener('load', render, {once: true});
    } else {
      const script = document.createElement('script');
      script.id = TURNSTILE_SCRIPT_ID;
      script.src = TURNSTILE_SCRIPT;
      script.async = true;
      script.defer = true;
      script.addEventListener('load', render, {once: true});
      script.addEventListener('error', () => setStatus('error'), {once: true});
      document.head.appendChild(script);
    }
    return () => {
      cancelled = true;
      if (widgetRef.current && turnstileAPI()) turnstileAPI()?.remove(widgetRef.current);
      widgetRef.current = '';
    };
  }, [onTokenAction, required, siteKey]);
  
  if (!required) return null;
  return <Dialog open onOpenChange={() => undefined}>
    <DialogContent className="bot-challenge-dialog">
      <DialogHeader>
        <DialogTitle><ShieldCheck aria-hidden="true"/> Verificação rápida</DialogTitle>
        <DialogDescription>
          Detectamos uma sequência incomum de ações muito rápidas. Confirme que é você para continuar jogando.
        </DialogDescription>
      </DialogHeader>
      {!siteKey ? <p className="bot-challenge-error" role="alert">
        A verificação ainda não foi configurada neste ambiente.
      </p> : <>
        <div ref={containerRef} className="turnstile-slot"/>
        {(status === 'loading' || status === 'checking') && <p className="bot-challenge-status">
            <LoaderCircle className="spin" aria-hidden="true"/>
          {status === 'checking' ? 'Validando…' : 'Preparando verificação…'}
        </p>}
        {status === 'error' && <p className="bot-challenge-error" role="alert">
            Não foi possível validar. Recarregue a página para tentar novamente.
        </p>}
      </>}
      <small>O relógio da mesa continua visível ao fundo e seu Time Bank permanece disponível.</small>
    </DialogContent>
  </Dialog>;
}
