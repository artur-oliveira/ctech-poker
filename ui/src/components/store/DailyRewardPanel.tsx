'use client';
import {useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Flame, RotateCcw} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {type DailyRewardStatus, getCooldown, spin} from '@/lib/api/dailyReward';
import {BALANCE_QUERY_KEY} from '@/lib/api/player';
import {pushNotification} from '@/lib/notify';
import {formatDuration, useCountdownMs} from './useCountdown';
import {DailyStreakDialog} from './DailyStreakDialog';

function deadlineFromSeconds(seconds: number): number | null {
  return seconds > 0 ? Date.now() + seconds * 1000 : null;
}

function chips(amount: number): string {
  return amount.toLocaleString('pt-BR');
}

/** The teaser that lives in the store: it reports the streak at a glance and
 * opens the trail, which is where the claim itself happens (#293). */
export function DailyRewardPanel() {
  const queryClient = useQueryClient();
  const cooldown = useQuery({queryKey: ['dailyReward', 'cooldown'], queryFn: getCooldown});
  const [deadline, setDeadline] = useState<number | null>(null);
  // Tracks which server-reported cooldown the local deadline was last derived
  // from, so a fresh GET result resets the countdown exactly once (during
  // render, not in an effect — avoids an extra cascading render per fetch).
  const [syncedFrom, setSyncedFrom] = useState<number | undefined>(undefined);
  const [claiming, setClaiming] = useState(false);
  const [wonAmount, setWonAmount] = useState<number | null>(null);
  const [open, setOpen] = useState(false);

  const status = cooldown.data;
  if (status && wonAmount === null && status.remaining_time_seconds !== syncedFrom) {
    setSyncedFrom(status.remaining_time_seconds);
    setDeadline(deadlineFromSeconds(status.remaining_time_seconds));
  }

  const remainingMs = useCountdownMs(deadline);
  const ready = deadline === null || remainingMs <= 0;

  async function claim() {
    setClaiming(true);
    try {
      const result = await spin();
      const {amount, ...next} = result;
      setSyncedFrom(next.remaining_time_seconds);
      setDeadline(deadlineFromSeconds(next.remaining_time_seconds));
      queryClient.setQueryData<DailyRewardStatus>(['dailyReward', 'cooldown'], next);
      void queryClient.invalidateQueries({queryKey: BALANCE_QUERY_KEY});
      if (amount > 0) {
        setWonAmount(amount);
        pushNotification(`Você recebeu ${chips(amount)} fichas sandbox!`, 'info');
      } else {
        pushNotification('Recompensa diária ainda não disponível.', 'info');
      }
    } catch {
      pushNotification('Não foi possível resgatar sua recompensa agora. Tente novamente em instantes.');
    } finally {
      setClaiming(false);
    }
  }

  if (cooldown.isError) {
    return <div className="store-reward store-reward-error">
      <span className="store-reward-icon"><Flame aria-hidden="true"/></span>
      <div className="store-reward-copy">
        <h2 id="daily-reward-title">Ofensiva diária</h2>
        <p>Não foi possível consultar sua ofensiva agora.</p>
      </div>
      <Button type="button" variant="outline" onClick={() => void cooldown.refetch()}>
        <RotateCcw aria-hidden="true"/> Tentar novamente
      </Button>
    </div>;
  }

  const claimed = Boolean(status?.claimed_today) || !ready;
  const todayReward = status?.days.find(day => day.today)?.amount ?? 0;
  const streak = status?.current_streak ?? 0;

  return <>
    <div className="store-reward" data-streak={streak > 0 ? 'live' : undefined}>
      <span className="store-reward-icon" data-lit={!claimed || undefined}>
        <Flame aria-hidden="true"/>
        {streak > 0 && <b className="store-reward-count">{streak}</b>}
      </span>
      <div className="store-reward-copy">
        <h2 id="daily-reward-title">Ofensiva diária</h2>
        <p>{!status
          ? 'Carregando sua trilha de recompensas…'
          : claimed
            ? `Dia ${status.cycle_day} de ${status.cycle_length} garantido. Próximo resgate em ${formatDuration(remainingMs)}.`
            : `Dia ${status.cycle_day} de ${status.cycle_length} liberado: ${chips(todayReward)} fichas esperando por você.`}</p>
      </div>
      <Button type="button" variant={claimed ? 'outline' : 'default'} disabled={!status}
              onClick={() => setOpen(true)}>
        {claimed ? 'Ver trilha' : 'Abrir e resgatar'}
      </Button>
    </div>
    {status && <DailyStreakDialog open={open} onOpenChange={setOpen} status={status} claimed={claimed} remainingMs={remainingMs}
                                  claiming={claiming} onClaim={() => void claim()} wonAmount={wonAmount}/>}
  </>;
}
