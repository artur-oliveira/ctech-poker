'use client';
import {useState} from 'react';
import {Star} from 'lucide-react';
import {PlayingCard} from '@/components/table/PlayingCard';
import {achievementProgress, type Achievement, type AchievementProgress} from '@/lib/api/achievements';
import {achievementDescription, achievementExample, achievementLabel} from '@/lib/achievements';

// Progress is undefined for the not-logged variant (no player data to merge):
// stars render empty and the tier ladder shows instead of a live count.
export function AchievementCard({achievement, count}: { achievement: Achievement; count?: number }) {
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);
  const progress: AchievementProgress | null = count === undefined ? null : achievementProgress(achievement.tiers, count);
  const example = achievementExample(achievement.key);

  const previewing = hoverIndex !== null;
  const previewTier = previewing ? achievement.tiers[hoverIndex] : null;

  return <article className="achievement-card">
    {example.length > 0 && <div className="achievement-card-art" aria-hidden="true">
      {example.map((card, i) => <PlayingCard key={`${card}-${i}`} card={card} index={i} size="hole"/>)}
    </div>}
    <div className="achievement-card-body">
      <h3>{achievementLabel(achievement.key)}</h3>
      <p>{achievementDescription(achievement.key)}</p>
      <div className="achievement-stars" onMouseLeave={() => setHoverIndex(null)}
           aria-label={progress ? `${progress.starsFilled} de 5 estrelas, ${progress.count.toLocaleString('pt-BR')} registrados` : 'Progresso disponível após entrar na conta'}>
        {achievement.tiers.map((tier, i) => {
          const filled = previewing ? i <= hoverIndex! : Boolean(progress && i < progress.starsFilled);
          return <button key={tier.stars} type="button" className={`achievement-star${filled ? ' is-filled' : ''}`}
                         onMouseEnter={() => setHoverIndex(i)} onFocus={() => setHoverIndex(i)}
                         onBlur={() => setHoverIndex(null)} aria-label={`Nível ${tier.stars}: ${tier.threshold.toLocaleString('pt-BR')}`}>
            <Star fill={filled ? 'currentColor' : 'none'} aria-hidden="true"/>
          </button>;
        })}
        {previewTier && <span className="achievement-star-tooltip" role="tooltip">
          {progress ? `${progress.count.toLocaleString('pt-BR')}/${previewTier.threshold.toLocaleString('pt-BR')}`
            : previewTier.threshold.toLocaleString('pt-BR')}
        </span>}
      </div>
      {progress
        ? <p className="achievement-count">{progress.maxed ? 'Completo' : `${progress.count.toLocaleString('pt-BR')}/${progress.nextTier!.threshold.toLocaleString('pt-BR')}`}</p>
        : <p className="achievement-count achievement-count-locked">{achievement.tiers.map(t => t.threshold.toLocaleString('pt-BR')).join(' · ')}</p>}
    </div>
  </article>;
}
