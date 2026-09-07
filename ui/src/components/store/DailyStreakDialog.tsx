'use client';
import {Check, Flame, Gift, Shield, ShieldCheck, Trophy} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle} from '@/components/ui/dialog';
import type {DailyRewardStatus, DailyStreakDay} from '@/lib/api/dailyReward';
import {formatDuration} from './useCountdown';

/** Compact chip figure for a trail cell: the exact value never fits 30 times
 * over, and the precise amount is on the claim button and in the cell's
 * accessible label. */
function shortChips(amount: number): string {
  if (amount >= 1_000_000) return `${(amount / 1_000_000).toLocaleString('pt-BR', {maximumFractionDigits: 1})}M`;
  return `${(amount / 1000).toLocaleString('pt-BR', {maximumFractionDigits: 1})}k`;
}

function fullChips(amount: number): string {
  return amount.toLocaleString('pt-BR');
}

function dayState(day: DailyStreakDay, claimable: boolean): 'claimed' | 'today' | 'locked' {
  if (day.claimed) return 'claimed';
  if (day.today && claimable) return 'today';
  return 'locked';
}

function dayLabel(day: DailyStreakDay, state: string): string {
  const value = `${fullChips(day.amount)} fichas`;
  if (state === 'claimed') return `Dia ${day.day}, ${value}, resgatado`;
  if (state === 'today') return `Dia ${day.day}, ${value}, disponível hoje`;
  return `Dia ${day.day}, ${value}, ainda bloqueado`;
}

function TrailCell({day, claimable}: {day: DailyStreakDay; claimable: boolean}) {
  const state = dayState(day, claimable);
  return <li className="streak-cell" data-state={state} data-milestone={day.milestone || undefined}>
    <span className="streak-cell-day">{day.day}</span>
    <span className="streak-cell-mark" aria-hidden="true">
      {state === 'claimed' ? <Check/> : day.milestone ? <Gift/> : <Flame/>}
    </span>
    <span className="streak-cell-amount">{shortChips(day.amount)}</span>
    <span className="sr-only">{dayLabel(day, state)}</span>
  </li>;
}

/** The grand prize closes the trail as a full-width chest rather than a 30th
 * identical cell — it is the one slot on the trail that is worth a headline. */
function GrandPrize({day, claimable}: {day: DailyStreakDay; claimable: boolean}) {
  const state = dayState(day, claimable);
  return <li className="streak-grand" data-state={state}>
    <span className="streak-grand-icon" aria-hidden="true"><Trophy/></span>
    <span className="streak-grand-copy">
      <strong>Dia {day.day} · {fullChips(day.amount)} fichas</strong>
      <span>O baú final da trilha. Depois dele a trilha recomeça no dia 1 e sua ofensiva continua contando.</span>
    </span>
    <span className="sr-only">{dayLabel(day, state)}</span>
  </li>;
}

export interface DailyStreakDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  status: DailyRewardStatus;
  /** Owned by the panel, which reconciles the server's `claimed_today` with
   * the locally ticking cooldown — the dialog must not derive it a second way
   * or the two disagree the moment a claim lands. */
  claimed: boolean;
  remainingMs: number;
  claiming: boolean;
  onClaim: () => void;
  /** Set for the rest of the session once a claim lands, so the dialog can
   * report what was just won instead of only what comes next. */
  wonAmount: number | null;
}

export function DailyStreakDialog(props: DailyStreakDialogProps) {
  const {status, remainingMs, claiming, onClaim, wonAmount} = props;
  const claimable = !props.claimed;
  const todayReward = status.days.find(day => day.today)?.amount ?? 0;
  const trail = status.days.filter(day => day.day < status.cycle_length);
  const grand = status.days.find(day => day.day === status.cycle_length);

  return <Dialog open={props.open} onOpenChange={props.onOpenChange}>
    <DialogContent className="streak-dialog max-w-[min(760px,calc(100%-2rem))]">
      <DialogHeader>
        <DialogTitle>Ofensiva diária</DialogTitle>
        <DialogDescription>
          Resgate fichas sandbox todo dia. Cada dia seguido vale mais que o anterior, e o dia {status.cycle_length} paga
          o baú de {fullChips(grand?.amount ?? 0)} fichas.
        </DialogDescription>
      </DialogHeader>

      <dl className="streak-stats">
        <div className="streak-stat" data-tone="flame">
          <dt><Flame aria-hidden="true"/> Ofensiva atual</dt>
          <dd>{status.current_streak} {status.current_streak === 1 ? 'dia' : 'dias'}</dd>
        </div>
        <div className="streak-stat">
          <dt><Trophy aria-hidden="true"/> Melhor ofensiva</dt>
          <dd>{status.best_streak} {status.best_streak === 1 ? 'dia' : 'dias'}</dd>
        </div>
        <div className="streak-stat" data-tone={status.protection_available ? 'shield' : undefined}>
          <dt>{status.protection_available ? <ShieldCheck aria-hidden="true"/> : <Shield aria-hidden="true"/>} Proteção</dt>
          <dd>{status.protection_available ? 'Guardada' : 'Nenhuma'}</dd>
        </div>
      </dl>

      <p className="streak-rule">
        {status.protection_available
          ? 'Você tem uma proteção guardada: se perder um dia, ela segura a ofensiva por você. Ela volta a cada 7 dias seguidos.'
          : 'Complete 7 dias seguidos para ganhar uma proteção — ela segura sua ofensiva no dia em que você não conseguir jogar.'}
      </p>

      <ol className="streak-trail" aria-label={`Trilha de ${status.cycle_length} dias`}>
        {trail.map(day => <TrailCell key={day.day} day={day} claimable={claimable}/>)}
        {grand && <GrandPrize day={grand} claimable={claimable}/>}
      </ol>

      <div className="streak-claim">
        {claimable
          ? <Button type="button" size="lg" loading={claiming} onClick={onClaim}>
            {claiming ? 'Resgatando…' : `Resgatar ${fullChips(todayReward)} fichas`}
          </Button>
          : <p className="streak-claim-done" role="status">
            {wonAmount !== null
              ? <><strong>+{fullChips(wonAmount)} fichas</strong> creditadas. Próximo resgate em {formatDuration(remainingMs)}.</>
              : <>Dia {status.cycle_day} já resgatado. Próximo resgate em {formatDuration(remainingMs)}.</>}
          </p>}
      </div>
    </DialogContent>
  </Dialog>;
}
