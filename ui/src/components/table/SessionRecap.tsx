'use client';
import {useEffect, useState} from 'react';
import {Receipt} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog';
import type {SessionRecapData, WalletMode} from '@/lib/api/player';
import {getSessionRecap} from '@/lib/api/player';

function durationLabel(seconds: number) {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return hours ? `${hours}h ${minutes}min` : `${minutes}min`;
}

// The buy-in a session actually risked is the sum of the initial seat and
// every rebuy, which only the server tracks (`AddBuyin`'s atomic ADD). The
// caller's `buyIn` prop comes from whatever page of `getSessions` the client
// last cached, so it is stale by exactly one rebuy whenever the player was
// auto-rebought on the way out — which is the common case, since busting is
// what makes people leave. Prefer the recap's figure whenever it resolved.
//
// The result itself has two sources, and which one is authoritative depends
// on whether the session is already closed. `net_pnl` is only written by
// CloseSession, and the `removed` frame can reach the client before that
// settlement lands, so an open recap still reports 0 — reducing
// finalStack - buyin_amount is correct either way and is what we use until
// the row is closed.
export function sessionResult(recap: SessionRecapData | null, buyIn: number, finalStack: number) {
  if (!recap) return {buyIn, result: finalStack - buyIn};
  if (recap.ended_at !== 0) return {buyIn: recap.buyin_amount, result: recap.net_pnl};
  return {buyIn: recap.buyin_amount, result: finalStack - recap.buyin_amount};
}

// One-time recap shown at the moment of leaving a table. Duration/buy-in/result
// render immediately from props so the dialog is never blank; the hands-played,
// biggest-pot and the authoritative buy-in/result fill in once the session
// recap resolves, or stay on the props on failure — matching the rest of the
// app's "never let a stats fetch block a core flow" contract.
export function SessionRecap({sessionId, joinedAt, buyIn, finalStack, mode, onCloseAction}: {
  sessionId?: string;
  joinedAt: number;
  buyIn: number;
  finalStack: number;
  mode: WalletMode;
  onCloseAction: () => void;
}) {
  const [recap, setRecap] = useState<SessionRecapData | null>(null);
  const [openedAt] = useState(() => Date.now());

  useEffect(() => {
    let cancelled = false;
    if (sessionId) {
      getSessionRecap(sessionId, mode).then(result => {
        if (!cancelled) setRecap(result);
      }).catch(() => {
      });
    }
    return () => {
      cancelled = true;
    };
  }, [sessionId, mode]);

  const sessionSeconds = Math.max(0, Math.floor((openedAt - joinedAt) / 1000));
  const {buyIn: settledBuyIn, result} = sessionResult(recap, buyIn, finalStack);
  const biggestPot = recap?.biggest_win?.net_change;
  return <Dialog open onOpenChange={next => {
    if (!next) onCloseAction();
  }}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle><span className="reality-check-title"><Receipt aria-hidden="true"/> Resumo da sessão</span>
        </DialogTitle>
        <DialogDescription>Como foi sua passagem por essa mesa.</DialogDescription>
      </DialogHeader>
      <dl className="reality-check-stats">
        <div>
          <dt>Tempo na mesa</dt>
          <dd>{durationLabel(sessionSeconds)}</dd>
        </div>
        <div>
          <dt>Entrada</dt>
          <dd>{settledBuyIn.toLocaleString('pt-BR')}</dd>
        </div>
        {recap && <div>
          <dt>{recap.truncated ? 'Mãos jogadas (últimas 200)' : 'Mãos jogadas'}</dt>
          <dd>{recap.hands_played}</dd>
        </div>}
        {biggestPot !== undefined && <div>
          <dt>Maior pote ganho</dt>
          <dd className="positive">+{biggestPot.toLocaleString('pt-BR')}</dd>
        </div>}
        <div>
          <dt>Resultado da sessão</dt>
          <dd className={result > 0 ? 'positive' : result < 0 ? 'negative' : ''}>
            {result > 0 ? '+' : ''}{result.toLocaleString('pt-BR')}
          </dd>
        </div>
      </dl>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCloseAction}>Voltar ao lobby</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
