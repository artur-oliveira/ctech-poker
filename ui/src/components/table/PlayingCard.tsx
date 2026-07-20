import Image from 'next/image';
import type {CSSProperties} from 'react';
import {back, cardLabel, cardPath} from '@/lib/cards';

export function PlayingCard({card, index, size, owner}: {card?: string; index: number; size: 'board' | 'hole'; owner?: 'viewer' | 'opponent'}) {
  const revealed = Boolean(card && card.toLowerCase() !== 'back' && cardPath(card) !== back);
  const dimensions = size === 'board' ? {width: 68, height: 95} : {width: 46, height: 64};
  const style = {'--deal-index': index} as CSSProperties;
  if (!revealed) return <Image className={`playing-card ${size}-card`} src={back} alt="Carta fechada" {...dimensions} style={style}/>;

  const label = size === 'board'
    ? `Carta comunitária: ${cardLabel(card!)}`
    : owner === 'viewer'
      ? `Sua carta: ${cardLabel(card!)}`
      : `Carta: ${cardLabel(card!)}`;
  return (
    <span className={`playing-card ${size}-card card-reveal`} role="img" aria-label={label} style={style}>
    <span className="card-reveal-inner">
      <Image className="card-back" src={back} alt="" aria-hidden="true" {...dimensions}/>
      <Image className="card-front" src={cardPath(card!)} alt="" aria-hidden="true" {...dimensions}/>
    </span>
  </span>
  );
}
