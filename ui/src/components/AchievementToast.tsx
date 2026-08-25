'use client';
import React, {useEffect, useRef, useState} from 'react';
import {Star} from 'lucide-react';
import {ACHIEVEMENT_LABELS} from "@/lib/utils";


const HOLD_MS = 4200;
const EXIT_MS = 350;

type AchievementUnlock = { key: string; stars: number };

export function AchievementToast({unlock, blocked = false}: {
  unlock: AchievementUnlock | null;
  blocked?: boolean;
}) {
  const [shown, setShown] = useState(unlock);
  const [leaving, setLeaving] = useState(false);
  const queued = useRef<AchievementUnlock | null>(null);
  
  useEffect(() => {
    if (blocked) {
      if (unlock) queued.current = unlock;
      // Outcome and consent cards own the announcement layer. Keep the latest
      // unlock queued instead of stacking a toast over a decision.
      setShown(previous => {
        if (previous && !queued.current) queued.current = previous;
        return null;
      });
      return;
    }
    const next = queued.current || unlock;
    queued.current = null;
    if (!next) return;
    // Retain the last unlock while its exit animation finishes.
    setShown(previous => previous?.key === next.key && previous.stars === next.stars ? previous : next);
    setLeaving(false);
  }, [unlock, blocked]);
  
  useEffect(() => {
    if (!shown) return () => {
    };
    const startLeave = setTimeout(() => setLeaving(true), HOLD_MS);
    const clear = setTimeout(() => setShown(null), HOLD_MS + EXIT_MS);
    return () => {
      clearTimeout(startLeave);
      clearTimeout(clear);
    };
  }, [shown]);
  
  if (!shown) return null;
  return <div key={`${shown.key}-${shown.stars}`} className={`achievement-toast${leaving ? ' leaving' : ''}`}
              role="status" aria-live="polite">
    <Star/>
    <span>
      <small>CONQUISTA DESBLOQUEADA</small>
      <b>{ACHIEVEMENT_LABELS[shown.key] || shown.key.replaceAll('_', ' ')}</b>
      <span className="achievement-toast-stars" aria-hidden="true">{Array.from({length: shown.stars}, (_, i) =>
        <span key={i} style={{'--delay': `${Math.min(i, 5) * 70}ms`} as React.CSSProperties}>★</span>)}</span>
      <span className="sr-only">{shown.stars} estrela{shown.stars === 1 ? '' : 's'}</span>
    </span>
  </div>;
}
