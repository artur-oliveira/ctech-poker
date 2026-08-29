'use client';
import {type CSSProperties, useState} from 'react';
import {Check, Star} from 'lucide-react';
import {PlayingCard} from '@/components/table/PlayingCard';
import {type Achievement, achievementProgress, type AchievementProgress} from '@/lib/api/achievements';
import {achievementDescription, achievementExample, achievementLabel, achievementValueFormat} from '@/lib/achievements';

// Progress is undefined for the not-logged variant (no player data to merge):
// stars render empty and the tier ladder shows instead of a live count.
export function AchievementCard({achievement, count}: { achievement: Achievement; count?: number }) {
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);
  const progress: AchievementProgress | null = count === undefined ? null : achievementProgress(achievement.tiers, count);
  const example = achievementExample(achievement.key);
  const formatValue = achievementValueFormat(achievement.key);
  
  const previewing = hoverIndex !== null;
  const previewTier = previewing ? achievement.tiers[hoverIndex] : null;
  const previousThreshold = progress
    ? Math.max(0, ...achievement.tiers.filter(tier => tier.threshold <= progress.count).map(tier => tier.threshold))
    : 0;
  const segmentProgress = progress?.nextTier
    ? Math.min(100, Math.round(((progress.count - previousThreshold) / (progress.nextTier.threshold - previousThreshold)) * 100))
    : progress?.maxed ? 100 : 0;
  const stateClass = progress?.maxed ? ' is-complete' : progress && progress.starsFilled > 0 ? ' is-progressing' : '';
  
  return <article className={`achievement-card${stateClass}`}>
    <div className="achievement-card-topline">
      {example.length > 0 && <div className="achievement-card-art" aria-hidden="true">
        {example.map((card, i) => <PlayingCard key={`${card}-${i}`} card={card} index={i} size="hole"/>)}
      </div>}
      {progress?.maxed && <span className="achievement-mastered"><Check aria-hidden="true"/> Dominada</span>}
    </div>
    <div className="achievement-card-body">
      <h3>{achievementLabel(achievement.key)}</h3>
      <p>{achievementDescription(achievement.key)}</p>
      <div className="achievement-stars" onMouseLeave={() => setHoverIndex(null)}
           aria-label={progress ? `${progress.starsFilled} de 5 estrelas, ${formatValue(progress.count)} registrados` : 'Progresso disponível após entrar na conta'}>
        {achievement.tiers.map((tier, i) => {
          const filled = previewing ? i <= hoverIndex! : Boolean(progress && i < progress.starsFilled);
          return <button key={tier.stars} type="button" className={`achievement-star${filled ? ' is-filled' : ''}`}
                         style={{'--tier-index': i} as CSSProperties}
                         onMouseEnter={() => setHoverIndex(i)} onFocus={() => setHoverIndex(i)}
                         onBlur={() => setHoverIndex(null)}
                         aria-label={`Nível ${tier.stars}: ${formatValue(tier.threshold)}`}>
            <Star fill={filled ? 'currentColor' : 'none'} aria-hidden="true"/>
          </button>;
        })}
        {previewTier && <span className="achievement-star-tooltip" role="tooltip">
          {progress ? `${formatValue(progress.count)}/${formatValue(previewTier.threshold)}`
            : formatValue(previewTier.threshold)}
        </span>}
      </div>
      {progress
        ? <div className="achievement-progress-wrap">
          <div className="achievement-progress-copy">
            <span>{progress.maxed ? 'Todos os níveis completos' : 'Rumo à próxima estrela'}</span>
            <strong>{progress.maxed ? `${progress.starsFilled}/${achievement.tiers.length}` : `${formatValue(progress.count)}/${formatValue(progress.nextTier!.threshold)}`}</strong>
          </div>
          <div className="achievement-progress-track" role="progressbar" aria-label={`Progresso de ${achievementLabel(achievement.key)}`}
               aria-valuemin={0} aria-valuemax={100} aria-valuenow={segmentProgress}>
            <span style={{'--fill': segmentProgress / 100} as CSSProperties}/>
          </div>
          {progress.maxed && <p className="achievement-count sr-only">Completo</p>}
        </div>
        : <p
          className="achievement-count achievement-count-locked">{achievement.tiers.map(t => formatValue(t.threshold)).join(' · ')}</p>}
    </div>
  </article>;
}
