'use client';
import {PartyPopper, Sparkles, Star} from 'lucide-react';
import {achievementLabel} from '@/lib/achievements';
import type {RecentUnlock} from '@/lib/achievementRecency';
import {relativeTime} from '@/lib/utils';

/**
 * "Recém-desbloqueadas" (#119): the last few unlocks in recency order, so the
 * richest gamification moment survives the 4.2s table toast. `celebrating` is
 * the key the player just unlocked at the table — it gets the gold ring, once,
 * and the ring is animation-only so reduced motion loses nothing but the pulse.
 */
export function RecentUnlocksRail({unlocks, celebrating}: {
  unlocks: RecentUnlock[];
  celebrating?: string | null;
}) {
  if (!unlocks.length) return null;
  const arrived = celebrating && unlocks.some(unlock => unlock.key === celebrating)
    ? achievementLabel(celebrating)
    : null;

  return <section className="achievement-recent" aria-labelledby="achievement-recent-heading">
    <h2 id="achievement-recent-heading"><Sparkles aria-hidden="true"/> Recém-desbloqueadas</h2>
    {arrived && <p className="achievement-arrival-note" role="status">
      <PartyPopper aria-hidden="true"/>
      <span>Boa! <b>{arrived}</b> entrou na sua coleção agora.</span>
    </p>}
    <ul className="achievement-recent-list">
      {unlocks.map(unlock => <li
        key={unlock.key}
        className={`achievement-recent-item${unlock.key === celebrating ? ' is-celebrating' : ''}`}
      >
        <b>{achievementLabel(unlock.key)}</b>
        <span className="achievement-recent-stars">
          <Star fill="currentColor" aria-hidden="true"/>
          {unlock.stars} {unlock.stars === 1 ? 'estrela' : 'estrelas'}
        </span>
        <small>{relativeTime(unlock.unlockedAtMs)}</small>
      </li>)}
    </ul>
  </section>;
}
