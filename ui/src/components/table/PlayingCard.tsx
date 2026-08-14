import Image from 'next/image';
import type {CSSProperties} from 'react';
import {back, cardLabel, cardPath} from '@/lib/cards';
import {useDeckVariant} from '@/lib/hooks/useDeckVariant';

export function PlayingCard({card, index, size, owner, slow, onReveal, revealPending, peekable, peeked, onPeekToggle, onPeekHover}: {
  card?: string;
  index: number;
  size: 'board' | 'hole';
  owner?: 'viewer' | 'opponent';
  slow?: boolean;
  onReveal?: () => void;
  revealPending?: boolean;
  // peekable/peeked/onPeek* are a private, client-side-only visibility gate
  // for the viewer's own hole cards mid-hand — distinct from onReveal, which
  // publicly reveals a card to every other viewer. Both can be relevant to
  // the same card at once (e.g. folded and eligible for public reveal, but
  // still not privately peeked), so peekable is checked first below.
  peekable?: boolean;
  peeked?: boolean;
  onPeekToggle?: () => void;
  onPeekHover?: (hovering: boolean) => void;
}) {
  const variant = useDeckVariant();
  const revealed = Boolean(card && card.toLowerCase() !== 'back' && cardPath(card, variant) !== back);
  const dimensions = size === 'board' ? {width: 68, height: 95} : {width: 46, height: 64};
  const style = {'--deal-index': index} as CSSProperties;
  if (peekable && revealed && !peeked) {
    return <button type="button" className={`playing-card ${size}-card peekable-card`}
                   aria-label={`Ver sua ${index + 1}ª carta`}
                   onClick={onPeekToggle}
                   onMouseEnter={() => onPeekHover?.(true)} onMouseLeave={() => onPeekHover?.(false)}
                   onFocus={() => onPeekHover?.(true)} onBlur={() => onPeekHover?.(false)} style={style}>
      <Image src={back} alt="Carta fechada" aria-hidden="true" {...dimensions}/>
      <span aria-hidden="true">Ver</span>
    </button>;
  }
  if (onReveal && revealed) {
    return <button type="button" className={`playing-card ${size}-card revealable-card`}
                   aria-label={`Mostrar sua ${index + 1}ª carta: ${cardLabel(card!)}`}
                   disabled={revealPending} onClick={onReveal} style={style}>
      <Image src={cardPath(card!, variant)} alt="" aria-hidden="true" {...dimensions}/>
      <span aria-hidden="true">{revealPending ? '…' : 'Mostrar'}</span>
    </button>;
  }
  if (!revealed) return <Image className={`playing-card ${size}-card`} src={back} alt="Carta fechada" {...dimensions}
                               style={style}/>;

  const label = size === 'board'
    ? `Carta comunitária: ${cardLabel(card!)}`
    : owner === 'viewer'
      ? `Sua carta: ${cardLabel(card!)}`
      : `Carta: ${cardLabel(card!)}`;
  const inner = <span className="card-reveal-inner">
    <Image className="card-back" src={back} alt="" aria-hidden="true" {...dimensions}/>
    <Image className="card-front" src={cardPath(card!, variant)} alt="" aria-hidden="true" {...dimensions}/>
  </span>;
  if (peekable) {
    return <button type="button" className={`playing-card ${size}-card card-reveal peekable-card is-peeked${slow ? ' card-flip-slow' : ''}`}
                   aria-label={`Ocultar sua ${index + 1}ª carta: ${cardLabel(card!)}`}
                   onClick={onPeekToggle}
                   onMouseEnter={() => onPeekHover?.(true)} onMouseLeave={() => onPeekHover?.(false)}
                   onFocus={() => onPeekHover?.(true)} onBlur={() => onPeekHover?.(false)} style={style}>
      {inner}
    </button>;
  }
  return (
    <span className={`playing-card ${size}-card card-reveal${slow ? ' card-flip-slow' : ''}`} role="img"
          aria-label={label} style={style}>
      {inner}
    </span>
  );
}
