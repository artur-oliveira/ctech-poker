'use client';
import {type KeyboardEvent, useRef, useState} from 'react';
import {Check, ChevronLeft, ChevronRight} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  AchievementCompactCard, achievementSummaryLabel, achievementSummaryView
} from '@/components/achievements/AchievementCompactCard';
import type {AchievementSummaryEntry} from '@/lib/api/achievements';

/**
 * The featured-achievement picker as a carousel: one rail, ordered so the
 * highlighted ones lead and the rest follow by star count.
 *
 * It is a multi-select listbox rather than a row of buttons. Sixty-odd
 * achievements would otherwise be sixty-odd tab stops; as a composite widget
 * the rail costs one, and Arrow/Home/End move between options the way a
 * screen-reader user already expects a listbox to behave.
 */
export interface FeaturedAchievementCarouselProps {
  entries: AchievementSummaryEntry[];
  selected: string[];
  max: number;
  onToggleAction: (key: string) => void;
}

/** Highlighted first (in the player's own order), then the strongest. */
export function orderedForCarousel(entries: AchievementSummaryEntry[], selected: string[]): AchievementSummaryEntry[] {
  const byKey = new Map(entries.map(entry => [entry.key, entry]));
  const featured = selected
    .map(key => byKey.get(key))
    .filter((entry): entry is AchievementSummaryEntry => Boolean(entry));
  const chosen = new Set(featured.map(entry => entry.key));
  const rest = entries.filter(entry => !chosen.has(entry.key)).sort((a, b) => {
    const left = achievementSummaryView(a, a.progress);
    const right = achievementSummaryView(b, b.progress);
    return right.starsFilled - left.starsFilled
      || b.progress - a.progress
      || left.label.localeCompare(right.label, 'pt-BR');
  });
  return [...featured, ...rest];
}

export function FeaturedAchievementCarousel({entries, selected, max, onToggleAction}: FeaturedAchievementCarouselProps) {
  const items = orderedForCarousel(entries, selected);
  const track = useRef<HTMLDivElement>(null);
  const options = useRef(new Map<string, HTMLDivElement>());
  const [focusedKey, setFocusedKey] = useState<string | null>(null);
  // Derived during render, never in an effect: a key that left the list (or a
  // first paint with nothing focused yet) falls back to the leading option, so
  // exactly one option is ever in the tab order.
  const rovingKey = focusedKey && items.some(item => item.key === focusedKey) ? focusedKey : items[0]?.key ?? null;

  function focusAt(index: number) {
    const next = items[Math.max(0, Math.min(items.length - 1, index))];
    if (!next) return;
    setFocusedKey(next.key);
    const node = options.current.get(next.key);
    node?.focus();
    node?.scrollIntoView?.({block: 'nearest', inline: 'nearest'});
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>, index: number, key: string) {
    if (event.key === 'ArrowRight') focusAt(index + 1);
    else if (event.key === 'ArrowLeft') focusAt(index - 1);
    else if (event.key === 'Home') focusAt(0);
    else if (event.key === 'End') focusAt(items.length - 1);
    else if (event.key === 'Enter' || event.key === ' ') toggle(key);
    else return;
    event.preventDefault();
  }

  function toggle(key: string) {
    setFocusedKey(key);
    onToggleAction(key);
  }

  function page(direction: -1 | 1) {
    const node = track.current;
    node?.scrollBy?.({left: direction * Math.max(240, node.clientWidth * 0.8), behavior: 'smooth'});
  }

  return <div className="achievement-carousel">
    <Button type="button" variant="ghost" size="icon" className="achievement-carousel-step"
            aria-label="Rolar conquistas para trás" onClick={() => page(-1)}>
      <ChevronLeft aria-hidden="true"/>
    </Button>
    <div className="achievement-carousel-track" ref={track} role="listbox" aria-multiselectable="true"
         aria-label={`Conquistas disponíveis para destaque, no máximo ${max}`}>
      {items.map((entry, index) => {
        const isFeatured = selected.includes(entry.key);
        const view = achievementSummaryView(entry, entry.progress);
        return <div
          key={entry.key}
          ref={node => {
            if (node) options.current.set(entry.key, node);
            else options.current.delete(entry.key);
          }}
          role="option"
          aria-selected={isFeatured}
          aria-label={achievementSummaryLabel(view)}
          tabIndex={entry.key === rovingKey ? 0 : -1}
          className={`achievement-carousel-option${isFeatured ? ' is-featured' : ''}`}
          onClick={() => toggle(entry.key)}
          onKeyDown={event => onKeyDown(event, index, entry.key)}
        >
          <span className="achievement-carousel-badge" aria-hidden="true"><Check/> Em destaque</span>
          <AchievementCompactCard achievement={entry} count={entry.progress}/>
        </div>;
      })}
    </div>
    <Button type="button" variant="ghost" size="icon" className="achievement-carousel-step"
            aria-label="Rolar conquistas para frente" onClick={() => page(1)}>
      <ChevronRight aria-hidden="true"/>
    </Button>
  </div>;
}
