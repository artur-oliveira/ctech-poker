import Image from 'next/image';
import type {CSSProperties} from 'react';
import {back, cardLabel, cardPath} from '@/lib/cards';
import {useDeckVariant} from '@/lib/hooks/useDeckVariant';

export function PlayingCard({card, index, size, owner, slow, onReveal, revealPending}: {
  card?: string;
  index: number;
  size: 'board' | 'hole';
  owner?: 'viewer' | 'opponent';
  slow?: boolean;
  onReveal?: () => void;
  revealPending?: boolean;
}) {
  const variant = useDeckVariant();
  const revealed = Boolean(card && card.toLowerCase() !== 'back' && cardPath(card, variant) !== back);
  const dimensions = size === 'board' ? {width: 68, height: 95} : {width: 46, height: 64};
  const style = {'--deal-index': index} as CSSProperties;
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
  return (
    <span className={`playing-card ${size}-card card-reveal${slow ? ' card-flip-slow' : ''}`} role="img"
          aria-label={label} style={style}>
      <span className="card-reveal-inner">
        <Image className="card-back" src={back} alt="" aria-hidden="true" {...dimensions}/>
        <Image className="card-front" src={cardPath(card!, variant)} alt="" aria-hidden="true" {...dimensions}/>
      </span>
    </span>
  );
}
