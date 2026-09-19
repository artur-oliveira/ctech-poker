'use client';
import {useEffect, useRef, useState} from 'react';
import Link from 'next/link';
import {LoaderCircle, ShieldCheck} from 'lucide-react';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle} from '@/components/ui/dialog';
import {Button} from '@/components/ui/button';
import {Label} from '@/components/ui/label';
import {BOT_CHECK_CONTEST_MAX_REASON_LENGTH, fileBotCheckContest} from '@/lib/api/botCheckContest';

const TURNSTILE_SCRIPT_ID = 'cloudflare-turnstile-script';
const TURNSTILE_SCRIPT = 'https://challenges.cloudflare.com/turnstile/v0/api.js';
// If the script is blocked or the widget renders but never fires a callback, the
// dialog would otherwise sit on "Preparando verificação…" forever with the table
// locked behind it. Flip to an actionable error state after this long.
const LOAD_TIMEOUT_MS = 15_000;

type TurnstileAPI = {
  render: (element: HTMLElement, options: Record<string, unknown>) => string;
  remove: (widgetId: string) => void;
};

function turnstileAPI() {
  return (window as typeof window & { turnstile?: TurnstileAPI }).turnstile;
}

/**
 * A failed or blocked bot-check used to be a dead end: reload or leave, no
 * other path (#322). `BotChallengeContest` gives the player still on this
 * screen a way to say "I'm not a bot" that a human can review — it never
 * reopens the table itself (it doesn't touch `required`/`onTokenAction` at
 * all), only records the claim as `pending` for later review.
 */
function BotChallengeContest({tableId}: { tableId?: string }) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState('');
  const [state, setState] = useState<'idle' | 'sending' | 'sent' | 'error'>('idle');

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setState('sending');
    try {
      await fileBotCheckContest({tableId, reason: reason.trim() || undefined});
      setState('sent');
    } catch {
      setState('error');
    }
  }

  if (state === 'sent') {
    return <p className="bot-challenge-contest-sent" role="status">
      Contestação enviada — status: aguardando revisão. Isso não libera a mesa automaticamente;
      nossa equipe vai analisar o caso.
    </p>;
  }

  if (!open) {
    return <Button type="button" variant="ghost" size="sm" className="bot-challenge-contest-toggle"
                    onClick={() => setOpen(true)}>
      Acha que não é um bot? Conte pra gente
    </Button>;
  }

  return <form className="bot-challenge-contest-form" onSubmit={submit}>
    <Label htmlFor="bot-challenge-contest-reason">O que aconteceu (opcional)</Label>
    <textarea
      id="bot-challenge-contest-reason"
      value={reason}
      onChange={e => setReason(e.target.value.slice(0, BOT_CHECK_CONTEST_MAX_REASON_LENGTH))}
      maxLength={BOT_CHECK_CONTEST_MAX_REASON_LENGTH}
      disabled={state === 'sending'}
      placeholder="Ex.: joguei rápido porque já sabia minha jogada"
    />
    {state === 'error' && <p className="form-error" role="alert">
      Não foi possível enviar sua contestação agora. Tente de novo.
    </p>}
    <div className="bot-challenge-contest-actions">
      <Button type="submit" variant="outline" size="sm" loading={state === 'sending'}>
        Enviar contestação
      </Button>
      <Button type="button" variant="ghost" size="sm" disabled={state === 'sending'}
              onClick={() => setOpen(false)}>
        Cancelar
      </Button>
    </div>
  </form>;
}

export function BotChallenge({required, onTokenAction, tableId}: {
  required: boolean;
  onTokenAction: (token: string) => boolean;
  tableId?: string;
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
    const timeout = window.setTimeout(() => {
      if (!cancelled) setStatus(current => (current === 'loading' ? 'error' : current));
    }, LOAD_TIMEOUT_MS);
    return () => {
      cancelled = true;
      window.clearTimeout(timeout);
      if (widgetRef.current && turnstileAPI()) turnstileAPI()?.remove(widgetRef.current);
      widgetRef.current = '';
    };
  }, [onTokenAction, required, siteKey]);

  if (!required) return null;
  const recovery = <div className="bot-challenge-recovery">
    <Button type="button" onClick={() => window.location.reload()}>Recarregar página</Button>
    <Button type="button" variant="ghost" render={<Link href="/lobby"/>}>Voltar ao lobby</Button>
  </div>;
  return <Dialog open onOpenChange={() => undefined}>
    <DialogContent className="bot-challenge-dialog">
      <DialogHeader>
        <DialogTitle><ShieldCheck aria-hidden="true"/> Verificação rápida</DialogTitle>
        <DialogDescription>
          Detectamos uma sequência incomum de ações muito rápidas. Confirme que é você para continuar jogando.
        </DialogDescription>
      </DialogHeader>
      {!siteKey ? <>
        <p className="bot-challenge-error" role="alert">
          A verificação ainda não foi configurada neste ambiente.
        </p>
        {recovery}
      </> : <>
        <div ref={containerRef} className="turnstile-slot"/>
        {(status === 'loading' || status === 'checking') && <p className="bot-challenge-status">
            <LoaderCircle className="spin" aria-hidden="true"/>
          {status === 'checking' ? 'Validando…' : 'Preparando verificação…'}
        </p>}
        {status === 'error' && <>
          <p className="bot-challenge-error" role="alert">
            Não foi possível validar. Recarregue a página para tentar novamente.
          </p>
          <BotChallengeContest tableId={tableId}/>
        </>}
        {status !== 'ready' && recovery}
      </>}
      <small>O relógio da mesa continua visível ao fundo e seu Time Bank permanece disponível.</small>
    </DialogContent>
  </Dialog>;
}
