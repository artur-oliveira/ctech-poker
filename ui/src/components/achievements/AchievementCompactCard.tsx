'use client';
import {Star} from 'lucide-react';
import {PlayingCard} from '@/components/table/PlayingCard';
import {type Achievement, achievementProgress} from '@/lib/api/achievements';
import {achievementExample, achievementLabel, achievementValueFormat} from '@/lib/achievements';

export interface AchievementSummaryView {
  label: string;
  /** The current counter, written the way this achievement writes numbers. */
  value: string;
  starsFilled: number;
  starsTotal: number;
  example: string[];
  maxed: boolean;
}

/**
 * One derivation shared by the compact card and by whatever names it (the
 * featured-achievement carousel's options). It goes through the same
 * `achievementProgress` / `achievementValueFormat` / `achievementExample` the
 * full `AchievementCard` uses, so the compact vocabulary can never drift from
 * the catalogue's: same card art, same label, same count, same star ladder.
 */
export function achievementSummaryView(achievement: Achievement, count: number): AchievementSummaryView {
  const progress = achievementProgress(achievement.tiers, count);
  return {
    label: achievementLabel(achievement.key),
    value: achievementValueFormat(achievement.key)(progress.count),
    starsFilled: progress.starsFilled,
    starsTotal: achievement.tiers.length,
    example: achievementExample(achievement.key),
    maxed: progress.maxed
  };
}

/** The accessible name for a control whose visible content is a compact card. */
export function achievementSummaryLabel(view: AchievementSummaryView): string {
  return `${view.label}, ${view.value} registrados, ${view.starsFilled} de ${view.starsTotal} estrelas`;
}

/**
 * The catalogue card reduced to what identifies an achievement at a glance:
 * the cards that stand for it, its name, the current value, and the stars.
 * Rendered inside listbox options, so every element here is a `span` and the
 * name lives on the owning control.
 */
export function AchievementCompactCard({achievement, count}: {achievement: Achievement; count: number}) {
  const view = achievementSummaryView(achievement, count);
  return <span className={`achievement-compact${view.maxed ? ' is-complete' : ''}`}>
    {view.example.length > 0 && <span className="achievement-compact-art" aria-hidden="true">
      {view.example.map((card, index) =>
        <PlayingCard key={`${card}-${index}`} card={card} index={index} size="hole"/>)}
    </span>}
    <span className="achievement-compact-name">{view.label}</span>
    <span className="achievement-compact-readout" aria-hidden="true">
      <span className="achievement-compact-value">{view.value}</span>
      <span className="achievement-compact-stars">
        <Star fill="currentColor"/>{view.starsFilled}/{view.starsTotal}
      </span>
    </span>
  </span>;
}
