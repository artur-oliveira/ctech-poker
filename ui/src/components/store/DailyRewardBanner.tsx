'use client';
import {useQuery} from '@tanstack/react-query';
import {ArrowDown, Flame} from 'lucide-react';
import {Button} from '@aoctech/ui';
import {getCooldown} from '@/lib/api/dailyReward';

/** Id of the reward panel's section in the Fichas department. */
export const DAILY_REWARD_ANCHOR = 'daily-reward';

/** Sits at the top of the store only while there is a reward to take: the
 * panel that actually claims it lives under "Fichas", a long scroll down. It
 * reads the same cooldown query as the panel, so it costs no extra request. */
export function DailyRewardBanner() {
  const {data: status} = useQuery({queryKey: ['dailyReward', 'cooldown'], queryFn: getCooldown});
  if (!status || status.claimed_today || status.remaining_time_seconds > 0) return null;

  const reward = status.days.find(day => day.today)?.amount ?? 0;
  const streak = status.streak_lost ? 0 : status.current_streak;

  function goToReward() {
    const target = document.getElementById(DAILY_REWARD_ANCHOR);
    if (!target) return;
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    target.scrollIntoView({behavior: reduced ? 'auto' : 'smooth', block: 'center'});
    target.querySelector('button')?.focus({preventScroll: true});
  }

  return <aside className="store-reward-banner" aria-label="Recompensa diária disponível">
    <span className="store-reward-banner-icon"><Flame aria-hidden="true"/></span>
    <p>
      <b>{reward > 0 ? `${reward.toLocaleString('pt-BR')} fichas esperando por você` : 'Sua recompensa diária está pronta'}</b>
      <small>{streak > 0
        ? `Ofensiva de ${streak} ${streak === 1 ? 'dia' : 'dias'} em andamento.`
        : 'Resgate hoje e comece uma nova ofensiva.'}</small>
    </p>
    <Button type="button" variant="brand" onClick={goToReward}>
      Pegar recompensa <ArrowDown aria-hidden="true"/>
    </Button>
  </aside>;
}
