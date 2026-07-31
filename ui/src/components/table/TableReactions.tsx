'use client';
import {type CSSProperties, useState} from 'react';
import {SmilePlus, Volume2, VolumeX, X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import type {SeatView} from '@/lib/api/table';
import {TABLE_REACTIONS, type TableReactionEvent, type TableReactionID} from '@/lib/reactions';

const REACTION_MUTE_KEY = 'poker:table-reactions-muted';
const CHIP_PIECES = Array.from({length: 12}, (_, index) => index);
const EFFECT_PIECES = Array.from({length: 8}, (_, index) => index);

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

function ReactionImpact({reactionId}: {reactionId: TableReactionID}) {
  if (reactionId === 'chip') return <span className="reaction-impact reaction-impact-chip" aria-hidden="true">
    {CHIP_PIECES.map(index => <ReactionChipStack key={index} className="reaction-jackpot-stack"
                                      style={{'--piece': index, '--chip-x': `${(index % 6) * 20 + 7}px`,
                                        '--chip-drift': `${((index * 7) % 11) - 5}px`,
                                        '--chip-drift-end': `${5 - ((index * 7) % 11)}px`} as CSSProperties}/>)}</span>;
  if (reactionId === 'tomato') return <span className="reaction-impact reaction-impact-tomato" aria-hidden="true">
    {EFFECT_PIECES.map(index => <span key={index} style={{'--piece': index} as CSSProperties}/>)}</span>;
  if (reactionId === 'coffee') return <span className="reaction-impact reaction-impact-coffee" aria-hidden="true">
    <span>☕</span>{[0, 1, 2].map(index => <i key={index} style={{'--piece': index} as CSSProperties}/>)}</span>;
  if (reactionId === 'clover') return <span className="reaction-impact reaction-impact-clover" aria-hidden="true">
    <span>🍀</span>{EFFECT_PIECES.slice(0, 6).map(index => <i key={index} style={{'--piece': index} as CSSProperties}>
      {index % 2 ? '✦' : '🍀'}</i>)}</span>;
  if (reactionId === 'horseshoe') return <span className="reaction-impact reaction-impact-horseshoe" aria-hidden="true">
    <span className="reaction-star-ring">{EFFECT_PIECES.map(index =>
      <i key={index} style={{'--piece': index} as CSSProperties}>★</i>)}</span></span>;
  if (reactionId === 'tear') return <span className="reaction-impact reaction-impact-tear" aria-hidden="true">
    {EFFECT_PIECES.map(index => <i key={index} style={{'--piece': index} as CSSProperties}>💧</i>)}</span>;
  return null;
}

function ReactionEffect({item}: { item: TableReactionEvent }) {
  const definition = TABLE_REACTIONS[item.reactionId];
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
               className={`table-reaction-effect ${definition.targeted ? `thrown reaction-${item.reactionId}` : 'emote'}`}
               role="img" aria-label={definition.label}>
    <span className="reaction-projectile"><ReactionGlyph reactionId={item.reactionId} glyph={definition.glyph}/></span>
    {definition.targeted && <ReactionImpact reactionId={item.reactionId}/>}
  </span>;
}

export function TableReactions({items, seats, viewerId, connected, coolingDown, pendingReaction, onQuickSend,
                                 onPendingReactionChange, open, onOpenChange}: {
  items: TableReactionEvent[];
  seats: SeatView[];
  viewerId?: string;
  connected: boolean;
  coolingDown: boolean;
  pendingReaction: TableReactionID | null;
  onQuickSend: (reaction: TableReactionID) => void;
  onPendingReactionChange: (reaction: TableReactionID | null) => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [muted, setMuted] = useState(() =>
    typeof window !== 'undefined' && window.localStorage.getItem(REACTION_MUTE_KEY) === 'true');
  const hasOpponents = seats.some(seat => seat.player_id !== viewerId);
  
  function toggleMute() {
    setMuted(value => {
      window.localStorage.setItem(REACTION_MUTE_KEY, String(!value));
      return !value;
    });
  }
  
  return <>
    {!muted && <div className="table-reaction-layer" aria-live="off">
      {items.map(item => <ReactionEffect key={item.id} item={item}/>)}
    </div>}
    <aside className={`table-reactions${open ? ' open' : ''}${pendingReaction ? ' targeting' : ''}`} aria-label="Reações da mesa">
      <Button type="button" variant="ghost" size="icon" className="reaction-toggle"
              aria-label={pendingReaction ? 'Cancelar arremesso' : open ? 'Fechar reações' : 'Abrir reações'}
              aria-expanded={open} aria-pressed={Boolean(pendingReaction)}
              onClick={() => pendingReaction ? onPendingReactionChange(null) : onOpenChange(!open)}>
        {open || pendingReaction ? <X/> : <SmilePlus/>}
      </Button>
      {pendingReaction && <span className="reaction-target-hint" role="status">Escolha um jogador</span>}
      {open && <div className="reaction-panel">
          <header><b>Reagir</b>
              <Button type="button" variant="ghost" size="icon"
                      aria-label={muted ? 'Ativar animações de reações' : 'Silenciar animações de reações'}
                      aria-pressed={muted} onClick={toggleMute}>
                {muted ? <VolumeX/> : <Volume2/>}
              </Button>
          </header>
          <div className="reaction-quick" role="group" aria-label="Emotes rápidos">
            {(Object.entries(TABLE_REACTIONS) as [TableReactionID, typeof TABLE_REACTIONS[TableReactionID]][])
              .filter(([, definition]) => !definition.targeted)
              .map(([id, definition]) => <button type="button" key={id} title={definition.label}
                                                 disabled={!connected || coolingDown} onClick={() => onQuickSend(id)}>
                <span aria-hidden="true">{definition.glyph}</span><span className="sr-only">{definition.label}</span>
              </button>)}
          </div>
          <p className="reaction-object-instruction">Escolha um objeto e toque no jogador.</p>
          <div className="reaction-objects" role="group" aria-label="Objetos">
            {(Object.entries(TABLE_REACTIONS) as [TableReactionID, typeof TABLE_REACTIONS[TableReactionID]][])
              .filter(([, definition]) => definition.targeted)
              .map(([id, definition]) => <button type="button" key={id} title={definition.label}
                                                 disabled={!connected || !hasOpponents || coolingDown}
                                                 onClick={() => {
                                                   onPendingReactionChange(id);
                                                   onOpenChange(false);
                                                 }}>
                <ReactionGlyph reactionId={id} glyph={definition.glyph}/>{definition.label}
              </button>)}
          </div>
      </div>}
    </aside>
  </>;
}
