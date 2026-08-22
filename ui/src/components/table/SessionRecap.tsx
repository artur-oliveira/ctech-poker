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
import type {WalletMode} from '@/lib/api/player';
import {getHands} from '@/lib/api/player';

const HANDS_PAGE_CAP = 3; // 3 pages * 50/page = 150 hands; a courtesy summary, not an audit.

function durationLabel(seconds: number) {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return hours ? `${hours}h ${minutes}min` : `${minutes}min`;
}

interface SessionHandStats {
  handsPlayed: number;
  biggestPot?: number;
  capped: boolean;
}

// Pages newest-first through this table's hands, stopping at the first hand
// older than the session (or the 150-hand cap), so it never reads more of
// the player's history than this one session actually needs.
async function loadSessionHandStats(tableId: string, mode: WalletMode, joinedAt: number): Promise<SessionHandStats> {
  let handsPlayed = 0;
  let biggestPot: number | undefined;
  let cursor: string | undefined;
  for (let page = 0; page < HANDS_PAGE_CAP; page++) {
    const result = await getHands({tableId, mode, cursor});
    let reachedSessionStart = false;
    for (const item of result.data) {
      if (item.ended_at < joinedAt) {
        reachedSessionStart = true;
        break;
      }
      handsPlayed++;
      if (item.net_change > 0) biggestPot = Math.max(biggestPot ?? 0, item.net_change);
    }
    if (reachedSessionStart || !result.has_next) return {handsPlayed, biggestPot, capped: false};
    if (page === HANDS_PAGE_CAP - 1) return {handsPlayed, biggestPot, capped: true};
    cursor = result.next_cursor ?? undefined;
  }
  return {handsPlayed, biggestPot, capped: false};
}

// One-time recap shown at the moment of leaving a table — duration/buy-in/result
// render immediately from props (no fetch needed); hands-played/biggest-pot
// fill in once the hand-history page load resolves, or stay absent on failure,
// matching getHands' own "never let a stats fetch block a core flow" contract.
export function SessionRecap({joinedAt, buyIn, finalStack, tableId, mode, onCloseAction}: {
  joinedAt: number;
  buyIn: number;
  finalStack: number;
  tableId: string;
  mode: WalletMode;
  onCloseAction: () => void;
}) {
  const [stats, setStats] = useState<SessionHandStats | null>(null);
  const [openedAt] = useState(() => Date.now());

  useEffect(() => {
    let cancelled = false;
    loadSessionHandStats(tableId, mode, joinedAt).then(result => {
      if (!cancelled) setStats(result);
    }).catch(() => {
    });
    return () => {
      cancelled = true;
    };
  }, [tableId, mode, joinedAt]);

  const sessionSeconds = Math.max(0, Math.floor((openedAt - joinedAt) / 1000));
  const result = finalStack - buyIn;
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
          <dd>{buyIn.toLocaleString('pt-BR')}</dd>
        </div>
        {stats && <div>
          <dt>{stats.capped ? 'Mãos jogadas (últimas 150)' : 'Mãos jogadas'}</dt>
          <dd>{stats.handsPlayed}</dd>
        </div>}
        {stats?.biggestPot !== undefined && <div>
          <dt>Maior pote ganho</dt>
          <dd className="positive">+{stats.biggestPot.toLocaleString('pt-BR')}</dd>
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
