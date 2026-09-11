import Image from 'next/image';
import {type CSSProperties, useState} from 'react';
import {back, cardLabel, cardPath} from '@/lib/cards';
import {useDeckVariant} from '@/lib/hooks/useDeckVariant';

// How long a card keeps `data-card-revealing` after it turns face-up — the
// window in which the deal-in / flip keyframes are allowed to run. Must cover
// the longest reveal in renderer.css (board flop card 3: 780ms board-card-reveal
// + 640ms stagger) with margin. It is a cap on jank, never on visibility: a
// card's resting stylesheet state is its final, fully-visible face, so if an
// engine strands the entrance animation `pending` (WebKit) or a face fetch
// fails, the card still paints. See docs/2026-09-10-card-reveal-visibility.md.
export const CARD_REVEAL_MS = 1500;

export function PlayingCard({card, index, size, owner, slow, revealing, onReveal, revealPending, peekable, peeked, onPeekToggle, shortcutKey}: {
  card?: string;
  index: number;
  size: 'board' | 'hole';
  owner?: 'viewer' | 'opponent';
  slow?: boolean;
  // Set by the parent (Board / Seat via useEnteredKeys) for the one render span
  // after this card turned face-up: it gates the deal-in / flip animation. The
  // card is fully visible with or without it.
  revealing?: boolean;
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
  shortcutKey?: string;
}) {
  const variant = useDeckVariant();
  const revealed = Boolean(card && card.toLowerCase() !== 'back' && cardPath(card, variant) !== back);

  // A failed face-SVG fetch (network hiccup at the moment of reveal — the
  // symptom users described, cured by F5) must not leave a transparent hole:
  // fall back to the card back, and clear the flag when the slot goes
  // face-down again so the next hand retries.
  const [faceBroken, setFaceBroken] = useState(false);
  if (!revealed && faceBroken) setFaceBroken(false);

  // Requested well above CSS display size (which every breakpoint sets explicitly):
  // Safari rasterizes an <img>-embedded SVG once at its width/height attributes,
  // ignoring devicePixelRatio, so a 1x-sized source reads blurry on Retina iPhones.
  const dimensions = size === 'board' ? {width: 204, height: 285} : {width: 138, height: 192};
  // The card back is one shared asset, it is above the fold on every surface
  // that shows a table, and Next measured it as the LCP element there. Eager
  // (not `priority`) fetches it without emitting a preload link per instance —
  // twenty seats would otherwise queue twenty preloads for the same file.
  const backImageProps = {...dimensions, loading: 'eager'} as const;
  const style = {'--deal-index': index} as CSSProperties;
  if (!revealed) return <Image className={`playing-card ${size}-card`} src={back} alt="Carta fechada" {...backImageProps}
                               style={style}/>;

  const inner = <span className="card-reveal-inner">
    <Image className="card-back" src={back} alt="" aria-hidden="true" {...backImageProps}/>
    {/* eager, not the default lazy: a card being revealed is on-screen and
        wanted now. `loading="lazy"` deferred the face fetch to an
        IntersectionObserver tick that a saturated reveal frame could delay,
        leaving a blank card in any engine. */}
    {!faceBroken && <Image className="card-front" src={cardPath(card!, variant)} alt="" aria-hidden="true"
                           {...dimensions} loading="eager" onError={() => setFaceBroken(true)}/>}
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
      <span className="peek-hint" aria-hidden="true">Ver{shortcutKey && <kbd>{shortcutKey}</kbd>}</span>
    </button>;
  }
  if (onReveal) {
    return <button type="button" className={`playing-card ${size}-card revealable-card`}
                   aria-label={`Mostrar sua ${index + 1}ª carta: ${cardLabel(card!)}`}
                   disabled={revealPending} onClick={onReveal} style={style}>
      <Image src={cardPath(card!, variant)} alt="" aria-hidden="true" {...dimensions} loading="eager"/>
      <span aria-hidden="true">{revealPending ? '…' : 'Mostrar'}</span>
    </button>;
  }

  const label = size === 'board'
    ? `Carta comunitária: ${cardLabel(card!)}`
    : owner === 'viewer'
      ? `Sua carta: ${cardLabel(card!)}`
      : `Carta: ${cardLabel(card!)}`;
  return (
    <span
      className={`playing-card ${size}-card card-reveal${faceBroken ? ' face-fallback' : ''}${slow ? ' card-flip-slow' : ''}`}
      role="img" aria-label={label} style={style}
      data-card-revealing={revealing && !faceBroken ? '' : undefined}>
      {inner}
    </span>
  );
}
