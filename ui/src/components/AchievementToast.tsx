'use client';
import React, {useEffect, useRef, useState} from 'react';
import {Star} from 'lucide-react';
import {ACHIEVEMENT_LABELS} from "@/lib/utils";
import {rememberAchievementUnlock} from '@/lib/achievementRecency';


const HOLD_MS = 4200;
const EXIT_MS = 350;

type AchievementUnlock = { key: string; stars: number };

const signature = (unlock: AchievementUnlock | null) =>
  unlock ? `${unlock.key}-${unlock.stars}` : null;

export function AchievementToast({unlock, blocked = false, onConsumed}: {
  unlock: AchievementUnlock | null;
  blocked?: boolean;
  // Called once a toast has finished its full lifecycle so the owner can drop
  // the unlock it is still holding. Without this the parent keeps `unlock` set
  // forever and every later `blocked` toggle (one per resolved hand) re-queues
  // and replays the same, already-celebrated achievement.
  onConsumed?: () => void;
}) {
  const [shown, setShown] = useState(unlock);
  const [leaving, setLeaving] = useState(false);
  const queued = useRef<AchievementUnlock | null>(null);
  // Signature of the unlock that already ran its full course. A completed
  // unlock is never re-shown or re-queued even if the prop still carries it.
  const consumed = useRef<string | null>(null);
  // Held in a ref so an inline `onConsumed` prop cannot restart the lifecycle
  // timers on every parent render.
  const onConsumedRef = useRef(onConsumed);
  useEffect(() => {
    onConsumedRef.current = onConsumed;
  });

  useEffect(() => {
    if (blocked) {
      if (unlock && signature(unlock) !== consumed.current) queued.current = unlock;
      // Outcome and consent cards own the announcement layer. Keep the latest
      // unlock queued instead of stacking a toast over a decision.
      setShown(previous => {
        if (previous && !queued.current) queued.current = previous;
        return null;
      });
      return;
    }
    const candidate = queued.current || unlock;
    queued.current = null;
    const next = candidate && signature(candidate) !== consumed.current ? candidate : null;
    if (!next) return;
    // Retain the last unlock while its exit animation finishes.
    setShown(previous => previous?.key === next.key && previous.stars === next.stars ? previous : next);
    setLeaving(false);
  }, [unlock, blocked]);

  useEffect(() => {
    if (!shown) return () => {
    };
    // #119: the toast is gone in 4.2s, so it hands the unlock to the
    // achievements page, which celebrates it once when the player gets there.
    rememberAchievementUnlock(shown.key);
    const startLeave = setTimeout(() => setLeaving(true), HOLD_MS);
    const clear = setTimeout(() => {
      consumed.current = signature(shown);
      setShown(null);
      onConsumedRef.current?.();
    }, HOLD_MS + EXIT_MS);
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
