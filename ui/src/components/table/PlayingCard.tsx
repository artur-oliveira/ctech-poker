import Image from 'next/image';
import type {CSSProperties} from 'react';
import {back, cardLabel, cardPath} from '@/lib/cards';
import {useDeckVariant} from '@/lib/hooks/useDeckVariant';

export function PlayingCard({card, index, size, owner, slow, onReveal, revealPending, peekable, peeked, onPeekToggle}: {
  card?: string;
  index: number;
  size: 'board' | 'hole';
  owner?: 'viewer' | 'opponent';
  slow?: boolean;
  onReveal?: () => void;
  revealPending?: boolean;
  // peekable/peeked/onPeekToggle are a private, client-side-only visibility
  // gate for the viewer's own hole cards mid-hand — distinct from onReveal,
  // which publicly reveals a card to every other viewer. Both can be relevant
  // to the same card at once (e.g. folded and eligible for public reveal, but
  // still not privately peeked), so peekable is checked first below.
  peekable?: boolean;
  peeked?: boolean;
  onPeekToggle?: () => void;
}) {
  const variant = useDeckVariant();
  const revealed = Boolean(card && card.toLowerCase() !== 'back' && cardPath(card, variant) !== back);
  const dimensions = size === 'board' ? {width: 68, height: 95} : {width: 46, height: 64};
  const style = {'--deal-index': index} as CSSProperties;
  if (!revealed) return <Image className={`playing-card ${size}-card`} src={back} alt="Carta fechada" {...dimensions}
                               style={style}/>;

  const inner = <span className="card-reveal-inner">
    <Image className="card-back" src={back} alt="" aria-hidden="true" {...dimensions}/>
    <Image className="card-front" src={cardPath(card!, variant)} alt="" aria-hidden="true" {...dimensions}/>
  </span>;
  if (peekable) {
    // Both faces stay mounted in either state and the flip is a CSS transition
    // off .is-peeked, not a mount animation: an animation only ever played on
    // the way in, so hiding a card again snapped back with no motion at all.
    return <button type="button"
                   className={`playing-card ${size}-card card-reveal peekable-card${peeked ? ' is-peeked' : ''}`}
                   aria-label={peeked
                     ? `Ocultar sua ${index + 1}ª carta: ${cardLabel(card!)}`
                     : `Ver sua ${index + 1}ª carta`}
                   onClick={onPeekToggle} style={style}>
      {inner}
      <span className="peek-hint" aria-hidden="true">Ver</span>
    </button>;
  }
  if (onReveal) {
    return <button type="button" className={`playing-card ${size}-card revealable-card`}
                   aria-label={`Mostrar sua ${index + 1}ª carta: ${cardLabel(card!)}`}
                   disabled={revealPending} onClick={onReveal} style={style}>
      <Image src={cardPath(card!, variant)} alt="" aria-hidden="true" {...dimensions}/>
      <span aria-hidden="true">{revealPending ? '…' : 'Mostrar'}</span>
    </button>;
  }

  const label = size === 'board'
    ? `Carta comunitária: ${cardLabel(card!)}`
    : owner === 'viewer'
      ? `Sua carta: ${cardLabel(card!)}`
      : `Carta: ${cardLabel(card!)}`;
  return (
    <span className={`playing-card ${size}-card card-reveal${slow ? ' card-flip-slow' : ''}`} role="img"
          aria-label={label} style={style}>
      {inner}
    </span>
  );
}
