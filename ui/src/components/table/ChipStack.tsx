import {chipTier} from '@/lib/chips';

/** Purely decorative — the numeric chip count next to it already carries the value for
 * screen readers, so every chip here stays out of the accessibility tree. */
export function ChipStack({amount, bigBlind = 25, size = 'seat'}: {
  amount: number;
  bigBlind?: number;
  size?: 'seat' | 'pot';
}) {
  const tier = chipTier(amount, bigBlind);
  if (tier <= 0) return null;
  return <span key={amount} className={`chip-stack chip-stack-${size} tier-${tier}`}
    style={{'--tier': tier} as React.CSSProperties} aria-hidden="true">
    {Array.from({length: tier}, (_, i) => <span key={i} className="chip" style={{'--i': i} as React.CSSProperties}/>)}
  </span>;
}
