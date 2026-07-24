'use client';
import {useEffect, useState} from 'react';
import {PartyPopper} from 'lucide-react';
import {HAND_CATEGORY_LABELS} from '@/lib/utils';

export type HandOutcomeState = { key: number; kind: 'win' | 'lose'; amount: number; handCategory?: string };

const HOLD_MS = 2600;
const EXIT_MS = 320;
const CONFETTI_PIECES = Array.from({length: 8}, (_, i) => i);

/** Fires once per resolved hand (keyed by an ever-increasing counter the
 * caller bumps only when payouts first appear for that hand) — never on the
 * repeat broadcasts that follow while the table sits in `complete`. Purely
 * decorative: the sr-only live region already announces the same outcome. */
export function HandOutcomeBanner({outcome}: { outcome: HandOutcomeState | null }) {
  const [shown, setShown] = useState(outcome);
  const [leaving, setLeaving] = useState(false);
  const [seenKey, setSeenKey] = useState(outcome?.key);

  if (outcome && outcome.key !== seenKey) {
    setSeenKey(outcome.key);
    setShown(outcome);
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
  const category = shown.handCategory && (HAND_CATEGORY_LABELS[shown.handCategory] || shown.handCategory);
  return <div className="hand-outcome" aria-hidden="true">
    <div key={shown.key} className={`hand-outcome-card ${shown.kind}${leaving ? ' leaving' : ''}`}>
      {shown.kind === 'win' && <span className="hand-outcome-confetti">{CONFETTI_PIECES.map(i =>
        <span key={i}/>)}</span>}
      {shown.kind === 'win' ? <PartyPopper/> : null}
      <b>{shown.kind === 'win' ? 'Você venceu a mão!' : 'Não foi dessa vez.'}</b>
      {shown.kind === 'win' ?
        <span className="hand-outcome-amount">+{shown.amount.toLocaleString('pt-BR')} fichas</span> :
        <small>A próxima mão já está a caminho.</small>}
      {category && <small>{category}</small>}
    </div>
  </div>;
}
