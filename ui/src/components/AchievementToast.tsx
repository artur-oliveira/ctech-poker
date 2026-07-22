'use client';
import {useEffect, useState} from 'react';
import {Star} from 'lucide-react';

const HOLD_MS = 4200;
const EXIT_MS = 350;

export function AchievementToast({unlock}: { unlock: { key: string; stars: number } | null }) {
  const [shown, setShown] = useState(unlock);
  const [leaving, setLeaving] = useState(false);
  const [seen, setSeen] = useState(unlock);

  if (unlock && unlock !== seen) {
    setSeen(unlock);
    setShown(unlock);
    setLeaving(false);
  }

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
  return <div key={`${shown.key}-${shown.stars}`} className={`achievement-toast${leaving ? ' leaving' : ''}`}>
    <Star/>
    <span>
      <small>CONQUISTA DESBLOQUEADA</small>
      <b>{shown.key.replaceAll('_', ' ')}</b>
      <span className="achievement-stars" aria-hidden="true">{Array.from({length: shown.stars}, (_, i) =>
        <span key={i} style={{'--delay': `${Math.min(i, 5) * 70}ms`} as React.CSSProperties}>★</span>)}</span>
      <span className="sr-only">{shown.stars} estrela{shown.stars === 1 ? '' : 's'}</span>
    </span>
  </div>;
}
