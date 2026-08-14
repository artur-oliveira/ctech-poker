'use client';
import {type CSSProperties, useRef, useState} from 'react';
import {LockKeyhole, SmilePlus, Sparkles, Star, Volume2, VolumeX, X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {ReactionFavoritesDialog} from '@/components/reactions/ReactionFavoritesDialog';
import type {SeatView} from '@/lib/api/table';
import type {ReactionCatalogEntry, ReactionPurchase} from '@/lib/api/reactionPurchases';
import {ownedReactionIDs} from '@/lib/api/reactionPurchases';
import {isHoverCapable} from '@/lib/utils';
import {useDismiss} from '@/lib/hooks/useDismiss';
import {
  PREMIUM_REACTION_IDS, TABLE_REACTIONS, type TableReactionEvent, type TableReactionID
} from '@/lib/reactions';

const REACTION_MUTE_KEY = 'poker:table-reactions-muted';
const CHIP_PIECES = Array.from({length: 12}, (_, index) => index);
const EFFECT_PIECES = Array.from({length: 8}, (_, index) => index);
// Self reactions (cold, fire) decorate the floating emote itself rather than
// hitting an opponent, so they get a satellite effect with no throw delay.
const SELF_IMPACT_REACTIONS = new Set<TableReactionID>(['cold', 'fire', 'respect', 'sleepy']);

function ReactionChipStack({className = '', style}: {className?: string; style?: CSSProperties}) {
  return <span className={`reaction-table-chip-stack ${className}`} style={style} aria-hidden="true">
    {[0, 1, 2].map(index => <span key={index} className="chip"
                                  style={{'--i': index} as CSSProperties}/>)}</span>;
}

function ReactionGlyph({reactionId, glyph}: {reactionId: TableReactionID; glyph: string}) {
  return reactionId === 'chip'
    ? <ReactionChipStack/>
    : <span aria-hidden="true">{glyph}</span>;
}

function chipBurstStyle(index: number): CSSProperties {
  const angle = (index / CHIP_PIECES.length) * 360 + (index % 2 === 0 ? -9 : 9);
  const rad = (angle * Math.PI) / 180;
  const burstDist = 30 + (index % 4) * 9;
  const burstX = Math.cos(rad) * burstDist;
  const burstY = Math.sin(rad) * burstDist * 0.55 - 6;
  const fallExtra = 46 + (index % 3) * 20;
  return {
    '--piece': index,
    '--chip-burst-x': `${burstX.toFixed(1)}px`,
    '--chip-burst-y': `${burstY.toFixed(1)}px`,
    '--chip-fall-x': `${(burstX + ((index % 5) - 2) * 6).toFixed(1)}px`,
    '--chip-fall-y': `${(burstY + fallExtra).toFixed(1)}px`,
    '--chip-rot0': `${index % 2 === 0 ? -70 : 70}deg`,
    '--chip-rot1': `${(index % 2 === 0 ? -18 : 18) + index * 3}deg`,
  } as CSSProperties;
}

function ReactionImpact({reactionId}: {reactionId: TableReactionID}) {
  if (reactionId === 'chip') return <span className="reaction-impact reaction-impact-chip" aria-hidden="true">
    <i className="reaction-chip-flash"/>
    {CHIP_PIECES.map(index => <ReactionChipStack key={index} className="reaction-jackpot-stack"
                                                 style={chipBurstStyle(index)}/>)}</span>;
  if (reactionId === 'tomato') return <span className="reaction-impact reaction-impact-tomato" aria-hidden="true">
    {EFFECT_PIECES.map(index => <span key={index} style={{'--piece': index} as CSSProperties}/>)}</span>;
  if (reactionId === 'coffee') return <span className="reaction-impact reaction-impact-coffee" aria-hidden="true">
    <span>☕</span><b aria-hidden="true">✨</b>
    {[0, 1, 2].map(index => <i key={index} style={{'--piece': index} as CSSProperties}/>)}</span>;
  if (reactionId === 'clover') return <span className="reaction-impact reaction-impact-clover" aria-hidden="true">
    <i className="reaction-clover-glow"/>
    <span>🍀</span>{EFFECT_PIECES.slice(0, 6).map(index => <i key={index} style={{'--piece': index} as CSSProperties}>
      {index % 2 ? '✦' : '🍀'}</i>)}</span>;
  if (reactionId === 'horseshoe') return <span className="reaction-impact reaction-impact-horseshoe" aria-hidden="true">
    <i className="reaction-chip-flash reaction-horseshoe-flash"/>
    <span className="reaction-star-ring">{EFFECT_PIECES.map(index =>
      <i key={index} style={{'--piece': index} as CSSProperties}>★</i>)}</span></span>;
  if (reactionId === 'tear') return <span className="reaction-impact reaction-impact-tear" aria-hidden="true">
    {EFFECT_PIECES.map(index => <i key={index} style={{'--piece': index} as CSSProperties}>💧</i>)}</span>;
  if (reactionId === 'poop') return <span className="reaction-impact reaction-impact-poop" aria-hidden="true">
    {[0, 1].map(index => <i key={index} className="reaction-poop-stink" style={{'--piece': index} as CSSProperties}/>)}
    {EFFECT_PIECES.slice(0, 5).map(index => <span key={index} style={{'--piece': index} as CSSProperties}/>)}</span>;
  if (reactionId === 'rofl') return <span className="reaction-impact reaction-impact-rofl" aria-hidden="true">
    {[0, 1, 2].map(index => <i key={index} style={{'--piece': index} as CSSProperties}>🤣</i>)}</span>;
  if (reactionId === 'duck') return <span className="reaction-impact reaction-impact-duck" aria-hidden="true">
    {EFFECT_PIECES.slice(0, 6).map(index => <i key={index} style={{'--piece': index} as CSSProperties}>🪶</i>)}</span>;
  if (reactionId === 'turtle') return <span className="reaction-impact reaction-impact-turtle" aria-hidden="true">
    {[0, 1, 2].map(index => <i key={index} style={{'--piece': index} as CSSProperties}>💤</i>)}</span>;
  if (reactionId === 'cold') return <span className="reaction-impact reaction-impact-cold" aria-hidden="true">
    {[0, 1, 2].map(index => <i key={index} style={{'--piece': index} as CSSProperties}>❄</i>)}</span>;
  if (reactionId === 'fire') return <span className="reaction-impact reaction-impact-fire" aria-hidden="true">
    {[0, 1, 2].map(index => <i key={index} style={{'--piece': index} as CSSProperties}/>)}</span>;
  if (reactionId === 'respect') return <span className="reaction-impact reaction-impact-respect" aria-hidden="true">
    <i className="reaction-respect-glow"/>
    {[0, 1, 2].map(index => <i key={index} style={{'--piece': index} as CSSProperties}>✦</i>)}</span>;
  if (reactionId === 'sleepy') return <span className="reaction-impact reaction-impact-sleepy" aria-hidden="true">
    {[0, 1, 2].map(index => <i key={index} style={{'--piece': index} as CSSProperties}>z</i>)}</span>;
  if (reactionId === 'knife') return <span className="reaction-impact reaction-impact-knife" aria-hidden="true">
    {EFFECT_PIECES.map(index => <i key={index} style={{'--piece': index} as CSSProperties}/>)}</span>;
  if (reactionId === 'flowers') return <span className="reaction-impact reaction-impact-flowers" aria-hidden="true">
    {EFFECT_PIECES.map(index => <i key={index} style={{'--piece': index} as CSSProperties}>🌸</i>)}</span>;
  return null;
}

function ReactionEffect({item}: { item: TableReactionEvent }) {
  const definition = TABLE_REACTIONS[item.reactionId];
  const showImpact = definition.targeted || SELF_IMPACT_REACTIONS.has(item.reactionId);
  const positionEffect = (node: HTMLSpanElement | null) => {
    if (!node) return;
    const seats = Array.from(document.querySelectorAll<HTMLElement>('.game-seat[data-player-id]'));
    const source = seats.find(node => node.dataset.playerId === item.playerId);
    const target = item.targetPlayerId ? seats.find(node => node.dataset.playerId === item.targetPlayerId) : undefined;
    if (!source || (definition.targeted && !target)) return;
    const from = source.getBoundingClientRect();
    const to = target?.getBoundingClientRect();
    const fromX = from.left + from.width / 2;
    const fromY = from.top + from.height / 2;
    node.style.setProperty('--reaction-x', `${fromX}px`);
    node.style.setProperty('--reaction-y', `${fromY}px`);
    node.style.setProperty('--reaction-dx', `${to ? to.left + to.width / 2 - fromX : 0}px`);
    node.style.setProperty('--reaction-dy', `${to ? to.top + to.height / 2 - fromY : -72}px`);
    node.style.visibility = 'visible';
  };

  return <span ref={positionEffect}
               className={`table-reaction-effect reaction-${item.reactionId} ${definition.targeted ? 'thrown' : 'emote'}`}
               role="img" aria-label={definition.label}>
    <span className="reaction-projectile"><ReactionGlyph reactionId={item.reactionId} glyph={definition.glyph}/></span>
    {showImpact && <ReactionImpact reactionId={item.reactionId}/>}
  </span>;
}

export function TableReactions({items, seats, viewerId, connected, coolingDown, pendingReaction, onQuickSendAction,
                                 onPendingReactionChangeAction, open, onOpenChangeAction, premiumEnabled = false,
                                 premiumLoading = false, catalog = [], purchases = [], favorites = [],
                                 favoritesSaving = false, onLockedReactionAction, onFavoriteReactionsChangeAction}: {
  items: TableReactionEvent[];
  seats: SeatView[];
  viewerId?: string;
  connected: boolean;
  coolingDown: boolean;
  pendingReaction: TableReactionID | null;
  onQuickSendAction: (reaction: TableReactionID) => void;
  onPendingReactionChangeAction: (reaction: TableReactionID | null) => void;
  open: boolean;
  onOpenChangeAction: (open: boolean) => void;
  premiumEnabled?: boolean;
  premiumLoading?: boolean;
  catalog?: ReactionCatalogEntry[];
  purchases?: ReactionPurchase[];
  favorites?: TableReactionID[];
  favoritesSaving?: boolean;
  onLockedReactionAction?: (entry: ReactionCatalogEntry) => void;
  onFavoriteReactionsChangeAction?: (favorites: TableReactionID[]) => Promise<void> | void;
}) {
  const [muted, setMuted] = useState(() =>
    typeof window !== 'undefined' && window.localStorage.getItem(REACTION_MUTE_KEY) === 'true');
  const hasOpponents = seats.some(seat => seat.player_id !== viewerId);
  const [favoritesOpen, setFavoritesOpen] = useState(false);
  const asideRef = useRef<HTMLElement>(null);
  useDismiss(asideRef, open, () => onOpenChangeAction(false));
  const entries = new Map(catalog.map(entry => [entry.id, entry]));
  const owned = ownedReactionIDs(purchases);

  function toggleMute() {
    setMuted(value => {
      window.localStorage.setItem(REACTION_MUTE_KEY, String(!value));
      return !value;
    });
  }

  function premiumState(id: TableReactionID) {
    if (!premiumEnabled || !PREMIUM_REACTION_IDS.has(id)) return 'free' as const;
    if (premiumLoading) return 'loading' as const;
    if (owned.has(id)) return 'owned' as const;
    if (purchases.some(item => item.reaction_id === id && item.status === 'refunding')) return 'unavailable' as const;
    return entries.has(id) ? 'locked' as const : 'unavailable' as const;
  }

  function chooseReaction(id: TableReactionID) {
    const state = premiumState(id);
    if (state === 'loading' || state === 'unavailable') return;
    if (state === 'locked') {
      const entry = entries.get(id);
      if (entry) {
        onOpenChangeAction(false);
        onLockedReactionAction?.(entry);
      }
      return;
    }
    if (TABLE_REACTIONS[id].targeted) {
      onPendingReactionChangeAction(id);
      onOpenChangeAction(false);
    } else {
      onQuickSendAction(id);
    }
  }
  
  return <>
    {!muted && <div className="table-reaction-layer" aria-live="off">
      {items.map(item => <ReactionEffect key={item.id} item={item}/>)}
    </div>}
    <aside ref={asideRef} className={`table-reactions${open ? ' open' : ''}${pendingReaction ? ' targeting' : ''}`} aria-label="Reações da mesa"
           onMouseEnter={() => isHoverCapable() && !pendingReaction && onOpenChangeAction(true)}
           onMouseLeave={() => isHoverCapable() && !pendingReaction && onOpenChangeAction(false)}>
      <Button type="button" variant="ghost" size="icon" className="reaction-toggle"
              aria-label={pendingReaction ? 'Cancelar arremesso' : open ? 'Fechar reações' : 'Abrir reações'}
              aria-expanded={open} aria-pressed={Boolean(pendingReaction)}
              onClick={() => pendingReaction ? onPendingReactionChangeAction(null) : onOpenChangeAction(!open)}>
        {open || pendingReaction ? <X/> : <SmilePlus/>}
      </Button>
      {pendingReaction && <span className="reaction-target-hint" role="status">Escolha um jogador</span>}
      {open && <div className="reaction-panel">
          <header><b>Reagir</b><span>
              {onFavoriteReactionsChangeAction && <Button type="button" variant="ghost" size="icon"
                      aria-label="Editar reações favoritas" onClick={() => setFavoritesOpen(true)}>
                <Star aria-hidden="true"/>
              </Button>}
              <Button type="button" variant="ghost" size="icon"
                      aria-label={muted ? 'Ativar animações de reações' : 'Silenciar animações de reações'}
                      aria-pressed={muted} onClick={toggleMute}>
                {muted ? <VolumeX/> : <Volume2/>}
              </Button></span>
          </header>
          {favorites.length > 0 && <div className="reaction-favorites" role="group" aria-label="Reações favoritas">
            <span><Star aria-hidden="true"/> Atalhos</span>
            <div>{favorites.map(id => {
              const definition = TABLE_REACTIONS[id];
              if (!definition) return null;
              const state = premiumState(id);
              return <button type="button" key={id} title={definition.label}
                             className={`${state}${state === 'owned' ? ' premium-owned' : ''}`}
                             disabled={!connected || coolingDown || state === 'loading' || state === 'unavailable' ||
                               (definition.targeted && !hasOpponents)} onClick={() => chooseReaction(id)}>
                <span aria-hidden="true">{definition.glyph}</span>
                {state === 'locked' && <LockKeyhole aria-label="Premium bloqueada"/>}
                {state === 'owned' && <Sparkles aria-label="Premium liberada"/>}
              </button>;
            })}</div>
          </div>}
          <div className="reaction-quick" role="group" aria-label="Emotes rápidos">
            {(Object.entries(TABLE_REACTIONS) as [TableReactionID, typeof TABLE_REACTIONS[TableReactionID]][])
              .filter(([, definition]) => !definition.targeted)
              .map(([id, definition]) => {
                const state = premiumState(id);
                return <button type="button" key={id} title={definition.label} className={`reaction-choice ${state}`}
                               disabled={!connected || coolingDown || state === 'loading' || state === 'unavailable'}
                               onClick={() => chooseReaction(id)}>
                  <span aria-hidden="true">{definition.glyph}</span><span className="sr-only">{definition.label}</span>
                  {state === 'locked' && <LockKeyhole className="reaction-choice-state" aria-label="Premium bloqueada"/>}
                  {state === 'owned' && <Sparkles className="reaction-choice-state" aria-label="Premium liberada"/>}
                </button>;
              })}
          </div>
          <p className="reaction-object-instruction">Escolha um objeto e toque no jogador.</p>
          <div className="reaction-objects" role="group" aria-label="Objetos">
            {(Object.entries(TABLE_REACTIONS) as [TableReactionID, typeof TABLE_REACTIONS[TableReactionID]][])
              .filter(([, definition]) => definition.targeted)
              .map(([id, definition]) => {
                const state = premiumState(id);
                return <button type="button" key={id} title={definition.label} className={`reaction-choice ${state}`}
                               disabled={!connected || !hasOpponents || coolingDown || state === 'loading' || state === 'unavailable'}
                               onClick={() => chooseReaction(id)}>
                  <ReactionGlyph reactionId={id} glyph={definition.glyph}/>{definition.label}
                  {state === 'locked' && <LockKeyhole className="reaction-choice-state" aria-label="Premium bloqueada"/>}
                  {state === 'owned' && <Sparkles className="reaction-choice-state" aria-label="Premium liberada"/>}
                </button>;
              })}
          </div>
      </div>}
    </aside>
    {onFavoriteReactionsChangeAction && favoritesOpen && <ReactionFavoritesDialog
      key={favorites.join(':') || 'no-favorites'} open favorites={favorites}
      owned={owned} saving={favoritesSaving} onOpenChangeAction={setFavoritesOpen}
      onSaveAction={onFavoriteReactionsChangeAction}/>}
  </>;
}
