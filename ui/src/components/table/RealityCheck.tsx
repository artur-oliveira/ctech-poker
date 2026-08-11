'use client';
import {useEffect, useRef, useState} from 'react';
import {Clock3} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog';
import {useTablePreferences} from '@/lib/tablePreferences';

function durationLabel(seconds: number) {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return hours ? `${hours}h ${minutes}min` : `${minutes}min`;
}

export function RealityCheck({
                               joinedAt,
                               buyIn,
                               currentStack,
                               handId,
                               handComplete,
                               isTurn
                             }: {
  joinedAt: number;
  buyIn: number;
  currentStack: number;
  handId?: string;
  handComplete: boolean;
  isTurn: boolean;
}) {
  const {preferences} = useTablePreferences();
  const [open, setOpen] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const shownAt = useRef(0);
  const [completedHands, setCompletedHands] = useState<Set<string>>(() => new Set());
  const intervalMs = preferences.realityCheckMinutes * 60_000;
  
  useEffect(() => {
    if (!handComplete || !handId) return;
    // Record each completed hand once; the set spans subsequent live snapshots.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setCompletedHands(previous => previous.has(handId) ? previous : new Set(previous).add(handId));
  }, [handComplete, handId]);
  
  useEffect(() => {
    if (!intervalMs) return undefined;
    const tick = () => setNow(Date.now());
    const timer = window.setInterval(tick, 15_000);
    return () => window.clearInterval(timer);
  }, [intervalMs]);
  
  useEffect(() => {
    if (!intervalMs || isTurn || open) return;
    const sessionStart = joinedAt > 0 ? joinedAt * 1000 : now;
    const elapsed = Math.max(0, now - sessionStart);
    const dueBoundary = Math.floor(elapsed / intervalMs);
    if (dueBoundary > 0 && dueBoundary > shownAt.current) {
      shownAt.current = dueBoundary;
      setOpen(true);
    }
  }, [intervalMs, isTurn, joinedAt, now, open]);
  
  const sessionSeconds = Math.max(0, Math.floor((now - joinedAt * 1000) / 1000));
  const result = currentStack - buyIn;
  return <Dialog open={open} onOpenChange={setOpen}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle><span className="reality-check-title"><Clock3
          aria-hidden="true"/> Pausa consciente</span></DialogTitle>
        <DialogDescription>Um resumo neutro da sua sessão. O lembrete nunca abre durante sua vez.</DialogDescription>
      </DialogHeader>
      <dl className="reality-check-stats">
        <div>
          <dt>Tempo na mesa</dt>
          <dd>{durationLabel(sessionSeconds)}</dd>
        </div>
        <div>
          <dt>Mãos concluídas</dt>
          <dd>{completedHands.size}</dd>
        </div>
        <div>
          <dt>Entrada acumulada</dt>
          <dd>{buyIn.toLocaleString('pt-BR')}</dd>
        </div>
        <div>
          <dt>Stack atual</dt>
          <dd>{currentStack.toLocaleString('pt-BR')}</dd>
        </div>
        <div>
          <dt>Resultado da sessão</dt>
          <dd className={result > 0 ? 'positive' : result < 0 ? 'negative' : ''}>
            {result > 0 ? '+' : ''}{result.toLocaleString('pt-BR')}
          </dd>
        </div>
      </dl>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={() => setOpen(false)}>Continuar jogando</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
